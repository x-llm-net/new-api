package controller

import (
	"net/http"

	"github.com/QuantumNous/new-api/service"

	"github.com/gin-gonic/gin"
)

func GetXLLMHomeMonitorSummary(c *gin.Context) {
	summary, err := service.GetXLLMHomeMonitorSummary()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    summary,
	})
}
