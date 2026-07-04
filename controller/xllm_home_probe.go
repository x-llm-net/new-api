package controller

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/middleware"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relay"
	"github.com/QuantumNous/new-api/relay/channel"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/relay/helper"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/QuantumNous/new-api/types"

	"github.com/gin-gonic/gin"
)

type xllmHomeProbeSummary struct {
	Tested    int `json:"tested"`
	Succeeded int `json:"succeeded"`
	Failed    int `json:"failed"`
}

type xllmHomeProbeResult struct {
	info              *relaycommon.RelayInfo
	selectedChannelID int
	success           bool
	errorCode         string
	totalMs           int64
}

func runXLLMHomeProbeTask(ctx context.Context, taskID string, report func(processed, total int)) (xllmHomeProbeSummary, error) {
	entries := service.GetXLLMHomeMonitorEntries()
	summary := xllmHomeProbeSummary{}
	total := len(entries)
	for index, entry := range entries {
		if ctx != nil && ctx.Err() != nil {
			break
		}
		if report != nil {
			report(index, total)
		}

		result := runXLLMHomeProbeEntry(ctx, entry)
		if result.success {
			summary.Succeeded++
		} else {
			summary.Failed++
		}
		summary.Tested++

		if err := service.RecordXLLMProbeSample(service.XLLMProbeRecordInput{
			TaskID:            taskID,
			Entry:             entry,
			SelectedChannelID: result.selectedChannelID,
			Info:              result.info,
			StreamProbe:       true,
			Success:           result.success,
			ErrorCode:         result.errorCode,
			TotalMs:           result.totalMs,
			TestedAt:          time.Now().Unix(),
		}); err != nil {
			return summary, err
		}
	}
	if report != nil && (ctx == nil || ctx.Err() == nil) {
		report(total, total)
	}
	if ctx != nil && ctx.Err() != nil {
		return summary, ctx.Err()
	}
	return summary, nil
}

func runXLLMHomeProbeEntry(ctx context.Context, entry service.XLLMHomeMonitorEntry) xllmHomeProbeResult {
	if ctx == nil {
		ctx = context.Background()
	}
	startedAt := time.Now()
	result := xllmHomeProbeResult{errorCode: "unknown"}
	retryParam := &service.RetryParam{
		TokenGroup:  entry.GroupName,
		ModelName:   entry.ModelName,
		RequestPath: xllmHomeProbeSelectionPath(entry.ModelName),
		Retry:       common.GetPointer(0),
	}

	for ; retryParam.GetRetry() <= common.RetryTimes; retryParam.IncreaseRetry() {
		if ctx.Err() != nil {
			result.errorCode = "context_canceled"
			break
		}

		c, _ := newXLLMHomeProbeContext(ctx, entry, retryParam.RequestPath)
		retryParam.Ctx = c
		channel, _, err := service.CacheGetRandomSatisfiedChannel(retryParam)
		if err != nil {
			result.errorCode = "get_channel_failed"
			break
		}
		if channel == nil {
			result.errorCode = "no_channel"
			break
		}
		result.selectedChannelID = channel.Id

		attempt := executeXLLMHomeProbeAttempt(ctx, entry, channel)
		result.info = attempt.info
		result.errorCode = attempt.errorCode
		if attempt.success {
			result.success = true
			break
		}
		if attempt.newAPIError == nil || !shouldRetry(attempt.context, attempt.newAPIError, common.RetryTimes-retryParam.GetRetry()) {
			break
		}
	}
	result.totalMs = time.Since(startedAt).Milliseconds()
	if result.totalMs < 0 {
		result.totalMs = 0
	}
	return result
}

type xllmHomeProbeAttempt struct {
	context     *gin.Context
	info        *relaycommon.RelayInfo
	success     bool
	errorCode   string
	newAPIError *types.NewAPIError
}

func executeXLLMHomeProbeAttempt(ctx context.Context, entry service.XLLMHomeMonitorEntry, channel *model.Channel) xllmHomeProbeAttempt {
	endpointType := normalizeChannelTestEndpoint(channel, entry.ModelName, "")
	requestPath := xllmHomeProbeRequestPath(entry.ModelName, endpointType)
	c, w := newXLLMHomeProbeContext(ctx, entry, requestPath)

	newAPIError := middleware.SetupContextForSelectedChannel(c, channel, entry.ModelName)
	if newAPIError != nil {
		return xllmHomeProbeAttempt{context: c, errorCode: string(newAPIError.GetErrorCode()), newAPIError: newAPIError}
	}

	request := buildTestRequest(entry.ModelName, endpointType, channel, true)
	relayFormat := xllmHomeProbeRelayFormat(requestPath)
	info, err := relaycommon.GenRelayInfo(c, relayFormat, request, nil)
	if err != nil {
		newAPIError = types.NewError(err, types.ErrorCodeGenRelayInfoFailed)
		return xllmHomeProbeAttempt{context: c, info: info, errorCode: string(newAPIError.GetErrorCode()), newAPIError: newAPIError}
	}
	info.IsChannelTest = true
	info.InitChannelMeta(c)

	if err = helper.ModelMappedHelper(c, info, request); err != nil {
		newAPIError = types.NewError(err, types.ErrorCodeChannelModelMappedError)
		return xllmHomeProbeAttempt{context: c, info: info, errorCode: string(newAPIError.GetErrorCode()), newAPIError: newAPIError}
	}
	request.SetModelName(info.UpstreamModelName)

	newAPIError = sendXLLMHomeProbeRequest(c, info, request)
	if newAPIError != nil {
		return xllmHomeProbeAttempt{context: c, info: info, errorCode: string(newAPIError.GetErrorCode()), newAPIError: newAPIError}
	}

	httpResult := w.Result()
	defer httpResult.Body.Close()
	respBody, err := readTestResponseBody(httpResult.Body, true)
	if err != nil {
		newAPIError = types.NewOpenAIError(err, types.ErrorCodeReadResponseBodyFailed, http.StatusInternalServerError)
		return xllmHomeProbeAttempt{context: c, info: info, errorCode: string(newAPIError.GetErrorCode()), newAPIError: newAPIError}
	}
	if err = validateTestResponseBody(respBody, true); err != nil {
		newAPIError = types.NewOpenAIError(err, types.ErrorCodeBadResponseBody, http.StatusInternalServerError)
		return xllmHomeProbeAttempt{context: c, info: info, errorCode: string(newAPIError.GetErrorCode()), newAPIError: newAPIError}
	}
	if !info.HasSendResponse() {
		newAPIError = types.NewOpenAIError(errors.New("stream probe did not capture first token"), types.ErrorCodeBadResponseBody, http.StatusInternalServerError)
		return xllmHomeProbeAttempt{context: c, info: info, errorCode: string(newAPIError.GetErrorCode()), newAPIError: newAPIError}
	}
	return xllmHomeProbeAttempt{context: c, info: info, success: true}
}

func newXLLMHomeProbeContext(ctx context.Context, entry service.XLLMHomeMonitorEntry, requestPath string) (*gin.Context, *httptest.ResponseRecorder) {
	if ctx == nil {
		ctx = context.Background()
	}
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequestWithContext(ctx, http.MethodPost, requestPath, nil)
	c.Request.Header.Set("Content-Type", "application/json")
	c.Set("group", entry.GroupName)
	common.SetContextKey(c, constant.ContextKeyRequestStartTime, time.Now())
	common.SetContextKey(c, constant.ContextKeyTokenGroup, entry.GroupName)
	common.SetContextKey(c, constant.ContextKeyUserGroup, entry.GroupName)
	common.SetContextKey(c, constant.ContextKeyUsingGroup, entry.GroupName)
	return c, w
}

func sendXLLMHomeProbeRequest(c *gin.Context, info *relaycommon.RelayInfo, request dto.Request) *types.NewAPIError {
	apiType, _ := common.ChannelType2APIType(info.ChannelType)
	if info.RelayMode == relayconstant.RelayModeResponsesCompact &&
		apiType != constant.APITypeOpenAI &&
		apiType != constant.APITypeCodex {
		return types.NewError(fmt.Errorf("unsupported api type: %d", apiType), types.ErrorCodeInvalidApiType)
	}
	adaptor := relay.GetAdaptor(apiType)
	if adaptor == nil {
		return types.NewError(fmt.Errorf("invalid api type: %d, adaptor is nil", apiType), types.ErrorCodeInvalidApiType)
	}
	adaptor.Init(info)

	convertedRequest, err := convertXLLMHomeProbeRequest(c, info, adaptor, request)
	if err != nil {
		return types.NewError(err, types.ErrorCodeConvertRequestFailed)
	}
	jsonData, err := common.Marshal(convertedRequest)
	if err != nil {
		return types.NewError(err, types.ErrorCodeJsonMarshalFailed)
	}
	if len(info.ParamOverride) > 0 {
		jsonData, err = relaycommon.ApplyParamOverrideWithRelayInfo(jsonData, info)
		if err != nil {
			if fixedErr, ok := relaycommon.AsParamOverrideReturnError(err); ok {
				return relaycommon.NewAPIErrorFromParamOverride(fixedErr)
			}
			return types.NewError(err, types.ErrorCodeChannelParamOverrideInvalid)
		}
	}

	requestBody := bytes.NewBuffer(jsonData)
	c.Request.Body = io.NopCloser(bytes.NewBuffer(jsonData))
	resp, err := adaptor.DoRequest(c, info, requestBody)
	if err != nil {
		return types.NewOpenAIError(err, types.ErrorCodeDoRequestFailed, http.StatusInternalServerError)
	}

	var httpResp *http.Response
	if resp != nil {
		httpResp = resp.(*http.Response)
		info.IsStream = info.IsStream || strings.HasPrefix(httpResp.Header.Get("Content-Type"), "text/event-stream")
		if httpResp.StatusCode != http.StatusOK {
			return types.NewOpenAIError(service.RelayErrorHandler(c.Request.Context(), httpResp, true), types.ErrorCodeBadResponse, http.StatusInternalServerError)
		}
	}
	_, newAPIError := adaptor.DoResponse(c, httpResp, info)
	return newAPIError
}

func convertXLLMHomeProbeRequest(c *gin.Context, info *relaycommon.RelayInfo, adaptor channel.Adaptor, request dto.Request) (any, error) {
	switch info.RelayMode {
	case relayconstant.RelayModeResponses:
		responseReq, ok := request.(*dto.OpenAIResponsesRequest)
		if !ok {
			return nil, errors.New("invalid response request type")
		}
		return adaptor.ConvertOpenAIResponsesRequest(c, info, *responseReq)
	default:
		generalReq, ok := request.(*dto.GeneralOpenAIRequest)
		if !ok {
			return nil, errors.New("invalid general request type")
		}
		return adaptor.ConvertOpenAIRequest(c, info, generalReq)
	}
}

func xllmHomeProbeSelectionPath(modelName string) string {
	if strings.HasSuffix(modelName, ratio_setting.CompactModelSuffix) || strings.Contains(strings.ToLower(modelName), "codex") {
		return "/v1/responses"
	}
	return "/v1/chat/completions"
}

func xllmHomeProbeRequestPath(modelName string, endpointType string) string {
	if strings.HasPrefix(endpointType, string(constant.EndpointTypeOpenAIResponseCompact)) {
		return "/v1/responses/compact"
	}
	if strings.HasPrefix(endpointType, string(constant.EndpointTypeOpenAIResponse)) ||
		strings.HasSuffix(modelName, ratio_setting.CompactModelSuffix) ||
		strings.Contains(strings.ToLower(modelName), "codex") {
		return "/v1/responses"
	}
	return "/v1/chat/completions"
}

func xllmHomeProbeRelayFormat(requestPath string) types.RelayFormat {
	if strings.HasPrefix(requestPath, "/v1/responses/compact") {
		return types.RelayFormatOpenAIResponsesCompaction
	}
	if strings.HasPrefix(requestPath, "/v1/responses") {
		return types.RelayFormatOpenAIResponses
	}
	return types.RelayFormatOpenAI
}
