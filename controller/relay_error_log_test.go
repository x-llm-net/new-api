package controller

import (
	"bufio"
	"compress/gzip"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"testing/iotest"
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
	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestGeminiStreamMissingCompletionReachesClient(t *testing.T) {
	gin.SetMode(gin.TestMode)
	previousTimeout := constant.StreamingTimeout
	previousRanges := operation_setting.AutomaticRetryStatusCodeRanges
	constant.StreamingTimeout = 300
	operation_setting.AutomaticRetryStatusCodeRanges = []operation_setting.StatusCodeRange{{Start: 500, End: 503}}
	t.Cleanup(func() {
		constant.StreamingTimeout = previousTimeout
		operation_setting.AutomaticRetryStatusCodeRanges = previousRanges
	})
	const partial = "data: {\"candidates\":[{\"content\":{\"parts\":[{\"text\":\"partial answer\"}]}}],\"usageMetadata\":{\"promptTokenCount\":10,\"candidatesTokenCount\":3,\"totalTokenCount\":13}}\n\n"
	const notice = `{"candidates":[{"content":{"parts":[{"text":"The prompt could not be submitted. The prompt contains sensitive words that violate Google's [Generative AI Prohibited Use policy](https://policies.google.com/terms/generative-ai/use-policy). Try rephrasing the prompt. If you think this was an error, [send feedback](https://aistudio.google.com/docs/troubleshooting)."}]}}],"modelVersion":"gemini-test"}`
	for _, format := range []types.RelayFormat{types.RelayFormatOpenAI, types.RelayFormatClaude, types.RelayFormatGemini, types.RelayFormatOpenAIResponses} {
		for _, tc := range []struct {
			name      string
			body      string
			ping      bool
			cancelled bool
			readError bool
			failure   bool
			blocked   bool
		}{
			{name: "partial_eof", body: partial, failure: true},
			{name: "partial_read_error", body: partial, readError: true, failure: true},
			{name: "empty_eof", failure: true},
			{name: "heartbeat_empty_eof", ping: true, failure: true},
			{name: "empty_done", body: "data: [DONE]\n\n", failure: true},
			{name: "role_eof", body: "data: {\"candidates\":[{\"content\":{\"role\":\"model\",\"parts\":[]}}]}\n\n", failure: true},
			{name: "thinking_eof", body: "data: {\"candidates\":[{\"content\":{\"parts\":[{\"thought\":true,\"text\":\"thinking\"}]}}]}\n\n", failure: true},
			{name: "tool_eof", body: "data: {\"candidates\":[{\"content\":{\"parts\":[{\"functionCall\":{\"name\":\"lookup\",\"args\":{\"id\":1}}}]}}]}\n\n", failure: true},
			{name: "normal_stop", body: partial + "data: {\"candidates\":[{\"finishReason\":\"STOP\"}]}\n\n"},
			{name: "max_tokens", body: partial + "data: {\"candidates\":[{\"finishReason\":\"MAX_TOKENS\"}]}\n\n"},
			{name: "compatible_done", body: partial + "data: [DONE]\n\n"},
			{name: "content_blocked", body: "data: {\"candidates\":[{\"finishReason\":\"PROHIBITED_CONTENT\"}]}\n\n", blocked: true},
			{name: "prompt_blocked", body: "data: {\"promptFeedback\":{\"blockReason\":\"PROHIBITED_CONTENT\"}}\n\n", blocked: true},
			{name: "unspecified_prompt_block", body: "data: {\"promptFeedback\":{\"blockReason\":\"BLOCK_REASON_UNSPECIFIED\"}}\n\n", failure: true},
			{name: "known_policy_notice", body: "data: " + notice + "\n\n", blocked: true},
			{name: "notice_words_in_partial_text", body: "data: {\"candidates\":[{\"content\":{\"parts\":[{\"text\":\"The policy mentions sensitive words, but\"}]}}]}\n\n", failure: true},
			{name: "notice_after_partial", body: partial + "data: " + notice + "\n\n", failure: true},
			{name: "client_cancelled", body: partial, cancelled: true},
		} {
			t.Run(string(format)+"/"+tc.name, func(t *testing.T) {
				recorder := httptest.NewRecorder()
				ctx, _ := gin.CreateTestContext(recorder)
				ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
				if tc.cancelled {
					requestCtx, cancel := context.WithCancel(ctx.Request.Context())
					ctx.Request = ctx.Request.WithContext(requestCtx)
					cancel()
				}
				info := &relaycommon.RelayInfo{
					RelayFormat: format, IsStream: true, DisablePing: true,
					OriginModelName: "gemini-test", ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: "gemini-test"},
					ClaudeConvertInfo: &relaycommon.ClaudeConvertInfo{LastMessagesType: relaycommon.LastMessageTypeNone},
				}
				if tc.ping {
					helper.SetEventStreamHeaders(ctx)
					require.NoError(t, helper.PingData(ctx))
				}
				var reader io.Reader = strings.NewReader(tc.body)
				if tc.readError {
					reader = io.MultiReader(reader, iotest.ErrReader(io.ErrUnexpectedEOF))
				}
				resp := &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(reader)}
				var apiErr *types.NewAPIError
				switch format {
				case types.RelayFormatGemini:
					_, apiErr = gemini.GeminiTextGenerationStreamHandler(ctx, info, resp)
				case types.RelayFormatOpenAIResponses:
					_, apiErr = gemini.GeminiResponsesStreamHandler(ctx, info, resp)
				default:
					_, apiErr = gemini.GeminiChatStreamHandler(ctx, info, resp)
				}
				if !tc.failure {
					require.Nil(t, apiErr)
					assert.False(t, info.StreamStatus.ResponseFailed())
					assert.Equal(t, tc.blocked, info.PerformanceBusinessRejection)
					assert.False(t, common.GetContextKeyBool(ctx, constant.ContextKeyUpstreamStreamError))
					return
				}
				require.NotNil(t, apiErr)
				assert.Equal(t, http.StatusBadGateway, apiErr.StatusCode)
				assert.Equal(t, types.ErrorCodeBadResponseBody, apiErr.GetErrorCode())
				assert.True(t, info.StreamStatus.ResponseFailed())
				assert.Equal(t, http.StatusBadGateway, info.StreamStatus.OutcomeSnapshot().ErrorStatus)
				started := info.ReceivedResponseCount > 0
				assert.Equal(t, started, types.IsSkipRetryError(apiErr))
				if started {
					assert.Equal(t, "stop", service.DecideRelayRetry(ctx, apiErr, 3).Action)
				} else {
					assert.Equal(t, "retry", service.DecideRelayRetry(ctx, apiErr, 3).Action)
				}
				apiErr.SetMessage(common.MessageWithRequestId(apiErr.Error(), "stream-completion-regression"))
				writeRelayErrorResponse(ctx, format, apiErr)
				got := recorder.Body.String()
				assert.Contains(t, got, "stream-completion-regression")
				assert.Contains(t, got, "Response ended before completion.")
				assert.NotContains(t, got, "[DONE]")
				assert.NotContains(t, got, `"finish_reason":"stop"`)
				assert.NotContains(t, got, `"finishReason":"STOP"`)
				assert.NotContains(t, got, "event: message_stop")
				assert.NotContains(t, got, "event: response.completed")
				if started || tc.ping {
					assert.Equal(t, http.StatusOK, recorder.Code)
					assert.Equal(t, "text/event-stream", recorder.Header().Get("Content-Type"))
					assert.Contains(t, got, "data: ")
				} else {
					assert.Equal(t, http.StatusBadGateway, recorder.Code)
					var failure map[string]any
					require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &failure))
					assert.NotNil(t, failure["error"])
				}
			})
		}
	}
}

func TestGeminiRecordedStreamCompletion(t *testing.T) {
	path := os.Getenv("XLLM_GEMINI_STREAM_REPLAY")
	if path == "" {
		t.Skip("private response captures are provided only for local replay")
	}
	file, err := os.Open(path)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, file.Close()) })
	reader, err := gzip.NewReader(file)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, reader.Close()) })
	previousTimeout := constant.StreamingTimeout
	constant.StreamingTimeout = 300
	t.Cleanup(func() { constant.StreamingTimeout = previousTimeout })
	counts := make(map[string]int)
	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, 64*1024), 16*1024*1024)
	for scanner.Scan() {
		var sample struct {
			RequestID  string `json:"request_id"`
			Kind       string `json:"kind"`
			ClientGone bool   `json:"client_gone"`
			Body       []byte `json:"body"`
		}
		require.NoError(t, common.Unmarshal(scanner.Bytes(), &sample))
		t.Run(sample.RequestID, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			ctx, _ := gin.CreateTestContext(recorder)
			ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
			if sample.ClientGone {
				requestCtx, cancel := context.WithCancel(ctx.Request.Context())
				ctx.Request = ctx.Request.WithContext(requestCtx)
				cancel()
			}
			info := &relaycommon.RelayInfo{
				RelayFormat: types.RelayFormatOpenAI, IsStream: true, DisablePing: true,
				OriginModelName: "gemini-test", ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: "gemini-test"},
			}
			_, apiErr := gemini.GeminiChatStreamHandler(ctx, info, &http.Response{
				StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(string(sample.Body))),
			})
			failure := sample.Kind == "eof_text_without_terminal" || sample.Kind == "explicit_error"
			if failure {
				require.NotNil(t, apiErr)
				assert.True(t, types.IsSkipRetryError(apiErr))
				assert.True(t, info.StreamStatus.ResponseFailed())
				writeRelayErrorResponse(ctx, types.RelayFormatOpenAI, apiErr)
				assert.Contains(t, recorder.Body.String(), `"error":`)
				assert.NotContains(t, recorder.Body.String(), "[DONE]")
				assert.NotContains(t, recorder.Body.String(), `"finish_reason":"stop"`)
			} else {
				require.Nil(t, apiErr)
				assert.False(t, info.StreamStatus.ResponseFailed())
				if !sample.ClientGone {
					assert.Equal(t, sample.Kind == "policy_notice" || sample.Kind == "content_blocked", info.PerformanceBusinessRejection)
				}
			}
		})
		counts[sample.Kind]++
	}
	require.NoError(t, scanner.Err())
	require.NotEmpty(t, counts)
	t.Logf("recorded response categories: %v", counts)
}

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
			{name: "role_then_error", prefix: `{"candidates":[{"content":{"role":"model","parts":[]}}]}`, last: upstreamError, status: 500, message: "Internal error encountered."},
			{name: "thinking_then_error", prefix: `{"candidates":[{"content":{"role":"model","parts":[{"thought":true,"text":"thinking"}]}}]}`, last: upstreamError, status: 500, message: "Internal error encountered."},
			{name: "tool_then_error", prefix: `{"candidates":[{"content":{"role":"model","parts":[{"functionCall":{"name":"lookup","args":{"id":1}}}]}}]}`, last: upstreamError, status: 500, message: "Internal error encountered."},
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
				assert.Equal(t, tc.prefix != "", types.IsSkipRetryError(apiErr))
				if tc.prefix != "" {
					assert.Equal(t, "stop", service.DecideRelayRetry(ctx, apiErr, 3).Action)
				} else {
					assert.Equal(t, tc.ping, ctx.Writer.Written())
					assert.Equal(t, "retry", service.DecideRelayRetry(ctx, apiErr, 3).Action)
					assert.Equal(t, "attempt_budget_exhausted", service.DecideRelayRetry(ctx, apiErr, 0).Reason)
				}
				if tc.prefix == partial {
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

func TestGeminiStreamErrorUsesExistingRetryAndSettlesOnce(t *testing.T) {
	const upstreamError = `{"error":{"message":"Internal error encountered.","type":"bad_response_body","code":"bad_response_body"}}`
	const completed = `{"candidates":[{"content":{"role":"model","parts":[{"text":"Recovered answer."}]},"finishReason":"STOP"}],"usageMetadata":{"promptTokenCount":10,"candidatesTokenCount":3,"totalTokenCount":13}}`
	for _, tc := range []struct {
		name        string
		failures    int64
		wantCalls   int64
		partial     bool
		ping        bool
		silent      bool
		wantSuccess bool
	}{
		{name: "first_retry_recovers", failures: 1, wantCalls: 2, wantSuccess: true},
		{name: "third_retry_recovers", failures: 3, wantCalls: 4, wantSuccess: true},
		{name: "budget_exhausted", failures: 4, wantCalls: 4},
		{name: "partial_response_is_not_retried", failures: 1, wantCalls: 1, partial: true},
		{name: "heartbeat_then_recovery", failures: 1, wantCalls: 2, ping: true, wantSuccess: true},
		{name: "heartbeat_then_exhaustion", failures: 4, wantCalls: 4, ping: true},
		{name: "empty_eof_retry_recovers", failures: 1, wantCalls: 2, silent: true, wantSuccess: true},
		{name: "empty_eof_budget_exhausted", failures: 4, wantCalls: 4, silent: true},
		{name: "partial_eof_is_not_retried", failures: 1, wantCalls: 1, partial: true, silent: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fixture := newResponsesWSBillingTest(t, `tier("request", fixed(0.002))`, func(*websocket.Conn, *http.Request) {})
			previousRetries := common.RetryTimes
			previousRanges := operation_setting.AutomaticRetryStatusCodeRanges
			general := operation_setting.GetGeneralSetting()
			previousPing, previousInterval := general.PingIntervalEnabled, general.PingIntervalSeconds
			common.RetryTimes = 3
			operation_setting.AutomaticRetryStatusCodeRanges = []operation_setting.StatusCodeRange{{Start: 500, End: 503}}
			general.PingIntervalEnabled, general.PingIntervalSeconds = tc.ping, 1
			t.Cleanup(func() {
				common.RetryTimes = previousRetries
				operation_setting.AutomaticRetryStatusCodeRanges = previousRanges
				general.PingIntervalEnabled, general.PingIntervalSeconds = previousPing, previousInterval
			})
			var calls atomic.Int64
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				call := calls.Add(1)
				w.Header().Set("Content-Type", "text/event-stream")
				w.(http.Flusher).Flush()
				if tc.ping && call == 1 {
					time.Sleep(1200 * time.Millisecond)
				}
				if call <= tc.failures {
					if tc.partial {
						_, _ = fmt.Fprint(w, "data: {\"candidates\":[{\"content\":{\"role\":\"model\",\"parts\":[{\"text\":\"Partial answer.\"}]}}]}\n\n")
					}
					if !tc.silent {
						_, _ = fmt.Fprintf(w, "data: %s\n\n", upstreamError)
					}
					return
				}
				_, _ = fmt.Fprintf(w, "data: %s\n\n", completed)
			}))
			t.Cleanup(upstream.Close)
			require.NoError(t, model.DB.Model(fixture.channel).Updates(map[string]any{
				"type": constant.ChannelTypeGemini, "base_url": upstream.URL,
				"model_mapping": `{"ws-billing":"gemini-2.5-flash"}`,
			}).Error)
			request, err := http.NewRequest(http.MethodPost, fixture.gatewayURL+"/v1/responses", strings.NewReader(`{"model":"ws-billing","input":"hello","stream":true}`))
			require.NoError(t, err)
			request.Header.Set("Authorization", "Bearer sk-"+fixture.token.Key)
			request.Header.Set("Content-Type", "application/json")
			client := &http.Client{Timeout: 10 * time.Second}
			response, err := client.Do(request)
			require.NoError(t, err)
			body, err := io.ReadAll(response.Body)
			require.NoError(t, err)
			require.NoError(t, response.Body.Close())
			select {
			case <-fixture.httpDone:
			case <-time.After(3 * time.Second):
				t.Fatal("relay did not finish")
			}
			fixture.closeAndWait(t)
			assert.Equal(t, tc.wantCalls, calls.Load())
			if tc.ping {
				assert.Contains(t, string(body), ": PING")
			}
			if tc.wantSuccess {
				assert.Equal(t, http.StatusOK, response.StatusCode)
				assert.Contains(t, string(body), "Recovered answer.")
				assert.Equal(t, 1, strings.Count(string(body), "event: response.completed"))
				assert.NotContains(t, string(body), "Internal error encountered.")
				assertResponsesWSAccounting(t, fixture, []int{1000})
			} else {
				if tc.silent {
					assert.Contains(t, string(body), "Response ended before completion.")
				} else {
					assert.Contains(t, string(body), "Internal error encountered.")
				}
				assert.NotContains(t, string(body), "response.completed")
				assert.NotContains(t, string(body), "Recovered answer.")
				if tc.partial || tc.ping {
					assert.Equal(t, http.StatusOK, response.StatusCode)
					assert.Contains(t, string(body), "event: error")
				} else {
					if tc.silent {
						assert.Equal(t, http.StatusBadGateway, response.StatusCode)
					} else {
						assert.Equal(t, http.StatusInternalServerError, response.StatusCode)
					}
				}
				assertResponsesWSAccounting(t, fixture, nil)
			}
		})
	}
}

func TestRelayFinalErrorAfterHeartbeatRetryIsSSE(t *testing.T) {
	for _, format := range []types.RelayFormat{types.RelayFormatOpenAI, types.RelayFormatClaude, types.RelayFormatGemini, types.RelayFormatOpenAIResponses} {
		t.Run(string(format), func(t *testing.T) {
			recorder := httptest.NewRecorder()
			ctx, _ := gin.CreateTestContext(recorder)
			ctx.Request = httptest.NewRequest(http.MethodPost, "/", nil)
			helper.SetEventStreamHeaders(ctx)
			require.NoError(t, helper.PingData(ctx))
			// The next attempt may fail before opening another upstream stream.
			common.SetContextKey(ctx, constant.ContextKeyUpstreamStreamError, false)
			apiErr := types.NewErrorWithStatusCode(errors.New("Service unavailable."), types.ErrorCodeBadResponseStatusCode, 503)
			writeRelayErrorResponse(ctx, format, apiErr)
			assert.Contains(t, recorder.Body.String(), "data: ")
			assert.Equal(t, 1, strings.Count(recorder.Body.String(), "Service unavailable."))
			assert.NotContains(t, recorder.Body.String(), "[DONE]")
			for line := range strings.SplitSeq(recorder.Body.String(), "\n") {
				assert.True(t, line == "" || strings.HasPrefix(line, ":") || strings.HasPrefix(line, "event:") || strings.HasPrefix(line, "data:"), line)
			}
		})
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
