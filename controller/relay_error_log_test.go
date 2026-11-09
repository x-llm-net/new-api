package controller

import (
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relay/channel/gemini"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relay/helper"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/operation_setting"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestGeminiStreamErrorReachesClient(t *testing.T) {
	gin.SetMode(gin.TestMode)
	previousTimeout := constant.StreamingTimeout
	previousRetryRanges := operation_setting.AutomaticRetryStatusCodeRanges
	constant.StreamingTimeout = 300
	operation_setting.AutomaticRetryStatusCodeRanges = []operation_setting.StatusCodeRange{{Start: 500, End: 503}}
	t.Cleanup(func() {
		constant.StreamingTimeout = previousTimeout
		operation_setting.AutomaticRetryStatusCodeRanges = previousRetryRanges
	})

	const partial = `{"candidates":[{"content":{"role":"model","parts":[{"text":"partial answer"}]}}],"usageMetadata":{"promptTokenCount":10,"candidatesTokenCount":3,"totalTokenCount":13}}`
	const upstreamError = `{"error":{"message":"Internal error encountered.","type":"bad_response_body","param":"","code":"bad_response_body"}}`
	const googleError = `{"error":{"code":503,"message":"Service unavailable.","status":"UNAVAILABLE"}}`
	const normal = `{"error":null,"candidates":[{"content":{"role":"model","parts":[{"text":"The word error is ordinary text."}]},"finishReason":"STOP"}],"usageMetadata":{"promptTokenCount":10,"candidatesTokenCount":8,"totalTokenCount":18}}`

	for _, format := range []types.RelayFormat{types.RelayFormatOpenAI, types.RelayFormatClaude, types.RelayFormatGemini, types.RelayFormatOpenAIResponses} {
		for _, tc := range []struct {
			name    string
			prefix  string
			last    string
			ping    bool
			status  int
			message string
		}{
			{name: "partial_then_error", prefix: partial, last: upstreamError, status: 500, message: "Internal error encountered."},
			{name: "error_first", last: upstreamError, status: 500, message: "Internal error encountered."},
			{name: "ping_then_error", last: upstreamError, ping: true, status: 500, message: "Internal error encountered."},
			{name: "numeric_google_error", last: googleError, status: 503, message: "Service unavailable."},
			{name: "partial_then_numeric_error", prefix: partial, last: googleError, status: 503, message: "Service unavailable."},
			{name: "normal_stop", last: normal},
		} {
			t.Run(string(format)+"/"+tc.name, func(t *testing.T) {
				recorder := httptest.NewRecorder()
				ctx, _ := gin.CreateTestContext(recorder)
				ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
				ctx.Set(common.RequestIdKey, "stream-error-regression")
				info := &relaycommon.RelayInfo{
					RelayFormat: format, IsStream: true, DisablePing: true,
					OriginModelName:   "gemini-test",
					ChannelMeta:       &relaycommon.ChannelMeta{UpstreamModelName: "gemini-test"},
					ClaudeConvertInfo: &relaycommon.ClaudeConvertInfo{LastMessagesType: relaycommon.LastMessageTypeNone},
				}
				if tc.ping {
					helper.SetEventStreamHeaders(ctx)
					require.NoError(t, helper.PingData(ctx))
				}
				body := ""
				if tc.prefix != "" {
					body = "data: " + tc.prefix + "\n\n"
				}
				body += "data: " + tc.last + "\n\n"
				resp := &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(body))}
				var usage *dto.Usage
				var apiErr *types.NewAPIError
				switch format {
				case types.RelayFormatGemini:
					usage, apiErr = gemini.GeminiTextGenerationStreamHandler(ctx, info, resp)
				case types.RelayFormatOpenAIResponses:
					usage, apiErr = gemini.GeminiResponsesStreamHandler(ctx, info, resp)
				default:
					usage, apiErr = gemini.GeminiChatStreamHandler(ctx, info, resp)
				}
				if tc.status == 0 {
					require.Nil(t, apiErr)
					assert.False(t, info.StreamStatus.ResponseFailed())
					assert.Contains(t, recorder.Body.String(), "The word error is ordinary text.")
					assert.False(t, common.GetContextKeyBool(ctx, constant.ContextKeyUpstreamStreamError))
					return
				}
				require.NotNil(t, apiErr)
				assert.Equal(t, tc.status, apiErr.StatusCode)
				assert.Equal(t, tc.message, apiErr.Error())
				assert.JSONEq(t, tc.last, string(apiErr.DiagnosticResponseBody()))
				assert.True(t, info.StreamStatus.ResponseFailed())
				assert.Equal(t, tc.status, info.StreamStatus.OutcomeSnapshot().ErrorStatus)
				started := tc.prefix != "" || tc.ping
				assert.Equal(t, started, types.IsSkipRetryError(apiErr))
				if started {
					assert.Equal(t, "stop", service.DecideRelayRetry(ctx, apiErr, 3).Action)
				} else {
					assert.False(t, ctx.Writer.Written())
					if tc.status == 503 {
						assert.Equal(t, "retry", service.DecideRelayRetry(ctx, apiErr, 3).Action)
					} else {
						assert.Equal(t, "system_retry_exclusion", service.DecideRelayRetry(ctx, apiErr, 3).Reason)
					}
				}
				if tc.prefix != "" {
					require.NotNil(t, usage)
					assert.Equal(t, 13, usage.TotalTokens)
				}
				apiErr.SetMessage(common.MessageWithRequestId(apiErr.Error(), "stream-error-regression"))
				writeRelayErrorResponse(ctx, format, apiErr)
				got := recorder.Body.String()
				assert.Contains(t, got, "stream-error-regression")
				assert.NotContains(t, got, "[DONE]")
				assert.NotContains(t, got, `"finish_reason":"stop"`)
				assert.NotContains(t, got, `"finishReason":"STOP"`)
				assert.NotContains(t, got, "event: message_stop")
				assert.NotContains(t, got, "event: response.completed")
				if !started {
					assert.Equal(t, tc.status, recorder.Code)
					assert.Equal(t, "application/json; charset=utf-8", recorder.Header().Get("Content-Type"))
					assert.Empty(t, recorder.Header().Get("Transfer-Encoding"))
					var failure map[string]any
					require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &failure))
					assert.NotNil(t, failure["error"])
					return
				}
				assert.Equal(t, 200, recorder.Code)
				assert.Equal(t, "text/event-stream", recorder.Header().Get("Content-Type"))
				assert.Equal(t, 1, strings.Count(got, tc.message))
				assert.Contains(t, got, "stream-error-regression")
				if format == types.RelayFormatClaude || format == types.RelayFormatOpenAIResponses {
					assert.Equal(t, 1, strings.Count(got, "event: error"))
				}
				for line := range strings.SplitSeq(got, "\n") {
					if line == "" || strings.HasPrefix(line, "event:") || strings.HasPrefix(line, ":") {
						continue
					}
					payload, ok := strings.CutPrefix(line, "data: ")
					require.True(t, ok, "non-SSE data: %s", line)
					var event map[string]any
					require.NoError(t, common.UnmarshalJsonStr(payload, &event))
				}
			})
		}
	}
}

func TestProcessChannelErrorUsesSnapshotWithoutLeakingChannelMetadata(t *testing.T) {
	gin.SetMode(gin.TestMode)
	previousDB, previousLogDB := model.DB, model.LOG_DB
	previousRedisEnabled := common.RedisEnabled
	previousMainDatabaseType := common.MainDatabaseType()
	previousLogDatabaseType := common.LogDatabaseType()
	previousErrorLogEnabled := constant.ErrorLogEnabled

	database, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := database.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	require.NoError(t, database.AutoMigrate(&model.User{}, &model.Log{}))
	model.DB, model.LOG_DB = database, database
	common.RedisEnabled = false
	common.SetDatabaseTypes(common.DatabaseTypeSQLite, common.DatabaseTypeSQLite)
	constant.ErrorLogEnabled = true
	t.Cleanup(func() {
		model.DB, model.LOG_DB = previousDB, previousLogDB
		common.RedisEnabled = previousRedisEnabled
		common.SetDatabaseTypes(previousMainDatabaseType, previousLogDatabaseType)
		constant.ErrorLogEnabled = previousErrorLogEnabled
		require.NoError(t, sqlDB.Close())
	})

	require.NoError(t, database.Create(&model.User{Id: 7, Username: "log-owner", Group: "default"}).Error)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	ctx.Set("id", 7)
	ctx.Set("username", "log-owner")
	ctx.Set("token_name", "test-token")
	ctx.Set("token_id", 11)
	ctx.Set("original_model", "gpt-test")
	ctx.Set("group", "default")
	ctx.Set("channel_id", 202)
	ctx.Set("channel_name", "mutable-context-channel")
	ctx.Set("channel_type", 9)
	ctx.Set("use_channel", []string{"101"})
	common.SetContextKey(ctx, constant.ContextKeyRequestStartTime, time.Now().Add(-time.Second))

	channelSnapshot := types.ChannelError{
		ChannelId:   101,
		ChannelType: 1,
		ChannelName: "snapshot-channel",
		AutoBan:     false,
	}
	apiErr := types.NewOpenAIError(errors.New("upstream failed"), types.ErrorCodeBadResponseStatusCode, http.StatusBadGateway)

	processChannelError(ctx, channelSnapshot, apiErr, nil)

	var stored model.Log
	require.NoError(t, database.First(&stored).Error)
	assert.Equal(t, channelSnapshot.ChannelId, stored.ChannelId)
	storedOther, err := common.StrToMap(stored.Other)
	require.NoError(t, err)
	assert.Equal(t, float64(http.StatusBadGateway), storedOther["status_code"])
	for _, key := range []string{"channel_id", "channel_name", "channel_type"} {
		assert.NotContains(t, storedOther, key)
	}
	adminInfo, ok := storedOther["admin_info"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, []any{"101"}, adminInfo["use_channel"])

	logs, total, err := model.GetUserLogs(7, model.LogTypeError, 0, 0, "", "", 0, 10, "", "", "")
	require.NoError(t, err)
	require.Equal(t, int64(1), total)
	require.Len(t, logs, 1)
	assert.Equal(t, channelSnapshot.ChannelId, logs[0].ChannelId)
	assert.Empty(t, logs[0].ChannelName)
	userOther, err := common.StrToMap(logs[0].Other)
	require.NoError(t, err)
	assert.NotContains(t, userOther, "admin_info")
	for _, key := range []string{"channel_id", "channel_name", "channel_type"} {
		assert.NotContains(t, userOther, key)
	}
}
