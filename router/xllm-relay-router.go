package router

import (
	"github.com/QuantumNous/new-api/controller"
	"github.com/QuantumNous/new-api/middleware"
	"github.com/QuantumNous/new-api/types"

	"github.com/gin-gonic/gin"
)

func registerXLLMRelayRoutes(apiRouter *gin.RouterGroup) {
	xllmRoute := apiRouter.Group("/internal/xllm")
	xllmRoute.PUT(
		"/channels/:sourceRef",
		middleware.XLLMRelaySyncAuth(),
		controller.UpsertXLLMRelayChannel,
	)
	xllmRoute.POST(
		"/channels/:channelId/chat/completions",
		middleware.XLLMRelayRequestAuth(),
		middleware.XLLMRelayBindingAuth(),
		middleware.XLLMRelaySpecificChannel(),
		middleware.SystemPerformanceCheck(),
		middleware.TokenAuth(),
		middleware.XLLMRelayNoBilling(),
		middleware.Distribute(),
		func(c *gin.Context) {
			controller.Relay(c, types.RelayFormatOpenAI)
		},
	)
}
