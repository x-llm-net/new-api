package router

import (
	"github.com/QuantumNous/new-api/controller"
	"github.com/QuantumNous/new-api/middleware"
	"github.com/gin-gonic/gin"
)

func registerInvoiceRoutes(apiRouter *gin.RouterGroup) {
	invoiceRoute := apiRouter.Group("/invoice")
	invoiceRoute.Use(middleware.UserAuth())
	{
		invoiceRoute.GET("/summary", controller.GetInvoiceSummary)
		invoiceRoute.GET("/applications", controller.ListMyInvoiceApplications)
		invoiceRoute.POST("/applications", middleware.CriticalRateLimit(), controller.CreateInvoiceApplication)
		invoiceRoute.POST("/applications/:id/cancel", controller.CancelMyInvoiceApplication)
		invoiceRoute.GET("/applications/:id/download", controller.DownloadMyInvoice)
	}

	operatorRoute := apiRouter.Group("/invoice/operator")
	operatorRoute.Use(middleware.UserAuth(), middleware.InvoiceOperatorAuth())
	{
		operatorRoute.GET("/applications", controller.ListOperatorInvoiceApplications)
		operatorRoute.POST("/applications/:id/issue", controller.IssueInvoiceApplication)
		operatorRoute.POST("/applications/:id/reject", controller.RejectInvoiceApplication)
		operatorRoute.POST("/applications/:id/red-flush", controller.RedFlushInvoiceApplication)
	}

	configRoute := apiRouter.Group("/invoice/config")
	configRoute.Use(middleware.RootAuth())
	{
		configRoute.GET("", controller.GetInvoiceConfig)
		configRoute.PUT("", controller.UpdateInvoiceConfig)
	}
}
