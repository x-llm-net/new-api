package middleware

import (
	"crypto/subtle"
	"net/http"
	"os"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"

	"github.com/gin-gonic/gin"
)

func XLLMRelaySyncAuth() gin.HandlerFunc {
	return xllmInternalAuth("LLMHUB_RELAY_SYNC_TOKEN")
}

func XLLMRelayRequestAuth() gin.HandlerFunc {
	return xllmInternalAuth("LLMHUB_RELAY_REQUEST_TOKEN")
}

func xllmInternalAuth(envName string) gin.HandlerFunc {
	return func(c *gin.Context) {
		expected := strings.TrimSpace(os.Getenv(envName))
		if expected == "" {
			xllmInternalError(c, http.StatusServiceUnavailable, "not_configured", "Internal relay authentication is not configured")
			return
		}
		authorization := c.GetHeader("Authorization")
		supplied := ""
		if strings.HasPrefix(authorization, "Bearer ") {
			supplied = strings.TrimSpace(strings.TrimPrefix(authorization, "Bearer "))
		}
		if len(supplied) != len(expected) || subtle.ConstantTimeCompare([]byte(supplied), []byte(expected)) != 1 {
			xllmInternalError(c, http.StatusUnauthorized, "unauthorized", "Unauthorized")
			return
		}
		c.Next()
	}
}

func XLLMRelaySpecificChannel() gin.HandlerFunc {
	return func(c *gin.Context) {
		channelID := c.Param("channelId")
		parsedChannelID, err := strconv.Atoi(channelID)
		if err != nil || parsedChannelID <= 0 {
			xllmInternalError(c, http.StatusBadRequest, "invalid_channel_id", "Invalid channel id")
			return
		}
		token := strings.TrimSpace(os.Getenv("LLMHUB_RELAY_NEWAPI_TOKEN"))
		token = strings.TrimSpace(strings.TrimPrefix(token, "Bearer "))
		if token == "" {
			xllmInternalError(c, http.StatusServiceUnavailable, "not_configured", "Internal New API token is not configured")
			return
		}
		c.Request.Header.Set("Authorization", "Bearer "+token+"-"+channelID)
		c.Request.URL.Path = "/v1/chat/completions"
		c.Request.URL.RawPath = ""
		c.Next()
	}
}

func XLLMRelayBindingAuth() gin.HandlerFunc {
	return func(c *gin.Context) {
		channelID, err := strconv.Atoi(c.Param("channelId"))
		if err != nil || channelID <= 0 {
			xllmInternalError(c, http.StatusBadRequest, "invalid_channel_id", "Invalid channel id")
			return
		}
		active, err := model.IsActiveXLLMRelayChannel(channelID)
		if err != nil {
			xllmInternalError(c, http.StatusInternalServerError, "binding_lookup_failed", "Channel binding lookup failed")
			return
		}
		if !active {
			xllmInternalError(c, http.StatusNotFound, "binding_not_found", "Active LLMHub channel binding not found")
			return
		}
		c.Next()
	}
}

func XLLMRelayNoBilling() gin.HandlerFunc {
	return func(c *gin.Context) {
		userSetting, _ := common.GetContextKeyType[dto.UserSetting](c, constant.ContextKeyUserSetting)
		userSetting.AcceptUnsetRatioModel = true
		common.SetContextKey(c, constant.ContextKeyUserSetting, userSetting)
		common.SetContextKey(c, constant.ContextKeyXLLMRelayNoBilling, true)
		c.Next()
	}
}

func xllmInternalError(c *gin.Context, status int, code string, message string) {
	c.AbortWithStatusJSON(status, gin.H{
		"error": gin.H{
			"code":    code,
			"message": message,
		},
	})
}
