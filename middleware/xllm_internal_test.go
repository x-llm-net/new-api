package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestXLLMRelaySyncAuth(t *testing.T) {
	gin.SetMode(gin.TestMode)
	t.Setenv("LLMHUB_RELAY_SYNC_TOKEN", "sync-secret")
	router := gin.New()
	router.GET("/", XLLMRelaySyncAuth(), func(c *gin.Context) {
		c.Status(http.StatusNoContent)
	})

	unauthorized := httptest.NewRecorder()
	router.ServeHTTP(unauthorized, httptest.NewRequest(http.MethodGet, "/", nil))
	assert.Equal(t, http.StatusUnauthorized, unauthorized.Code)

	request := httptest.NewRequest(http.MethodGet, "/", nil)
	request.Header.Set("Authorization", "Bearer sync-secret")
	authorized := httptest.NewRecorder()
	router.ServeHTTP(authorized, request)
	assert.Equal(t, http.StatusNoContent, authorized.Code)
}

func TestXLLMRelaySpecificChannelPreparesExistingRelayAuth(t *testing.T) {
	gin.SetMode(gin.TestMode)
	t.Setenv("LLMHUB_RELAY_NEWAPI_TOKEN", "sk-internal")
	router := gin.New()
	router.POST("/channels/:channelId/chat/completions", XLLMRelaySpecificChannel(), func(c *gin.Context) {
		require.Equal(t, "Bearer sk-internal-42", c.GetHeader("Authorization"))
		require.Equal(t, "/v1/chat/completions", c.Request.URL.Path)
		c.Status(http.StatusNoContent)
	})

	response := httptest.NewRecorder()
	router.ServeHTTP(
		response,
		httptest.NewRequest(http.MethodPost, "/channels/42/chat/completions", nil),
	)
	assert.Equal(t, http.StatusNoContent, response.Code)
}

func TestXLLMRelayNoBillingAllowsDynamicModelsOnlyOnInternalRoute(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.POST("/", func(c *gin.Context) {
		common.SetContextKey(c, constant.ContextKeyUserSetting, dto.UserSetting{})
		c.Next()
	}, XLLMRelayNoBilling(), func(c *gin.Context) {
		setting, ok := common.GetContextKeyType[dto.UserSetting](c, constant.ContextKeyUserSetting)
		require.True(t, ok)
		require.True(t, setting.AcceptUnsetRatioModel)
		require.True(t, common.GetContextKeyBool(c, constant.ContextKeyXLLMRelayNoBilling))
		c.Status(http.StatusNoContent)
	})

	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/", nil))
	assert.Equal(t, http.StatusNoContent, response.Code)
}

func TestGlobalAPIRateLimitDoesNotConsumeInternalXLLMRequests(t *testing.T) {
	gin.SetMode(gin.TestMode)
	previousEnabled := common.GlobalApiRateLimitEnable
	previousLimit := common.GlobalApiRateLimitNum
	previousDuration := common.GlobalApiRateLimitDuration
	previousRedisEnabled := common.RedisEnabled
	t.Cleanup(func() {
		common.GlobalApiRateLimitEnable = previousEnabled
		common.GlobalApiRateLimitNum = previousLimit
		common.GlobalApiRateLimitDuration = previousDuration
		common.RedisEnabled = previousRedisEnabled
	})
	common.GlobalApiRateLimitEnable = true
	common.GlobalApiRateLimitNum = 1
	common.GlobalApiRateLimitDuration = 60
	common.RedisEnabled = false

	router := gin.New()
	router.Use(GlobalAPIRateLimit())
	router.Any("/*path", func(c *gin.Context) {
		c.Status(http.StatusNoContent)
	})

	request := func(path string) int {
		recorder := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, path, nil)
		req.RemoteAddr = "203.0.113.91:12345"
		router.ServeHTTP(recorder, req)
		return recorder.Code
	}

	assert.Equal(t, http.StatusNoContent, request("/api/internal/xllm/channels/1"))
	assert.Equal(t, http.StatusNoContent, request("/api/internal/xllm/channels/1"))
	assert.Equal(t, http.StatusNoContent, request("/api/status"))
	assert.Equal(t, http.StatusTooManyRequests, request("/api/status"))
}
