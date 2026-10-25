// web/router.go
package web

import (
	"github.com/gin-gonic/gin"
	"io"
)

// SetupRouter initializes the Gin router with all necessary routes, using the provided handlers.
// It also applies CORS middleware.
func SetupRouter(handlers *Handlers) *gin.Engine { // Accept Handlers struct
	// URLs and panic dumps can contain addresses, coordinates or keys.
	router := gin.New()
	policy := loadTransportPolicy()
	if err := router.SetTrustedProxies(policy.proxies); err != nil {
		panic("invalid trusted proxy configuration")
	}
	router.RemoteIPHeaders = []string{"X-Forwarded-For"}
	router.Use(gin.CustomRecoveryWithWriter(io.Discard, func(c *gin.Context, _ interface{}) {
		c.AbortWithStatusJSON(500, gin.H{"error": "服务暂时不可用，请稍后重试。"})
	}))
	router.Use(transportSecurity(policy))

	router.Use(handlers.protectAPI())
	registerUI(router)
	registerAuthRoutes(router, handlers)
	registerLocationRoutes(router)
	registerAutoSignRoutes(router, handlers)

	// 定义API路由
	api := router.Group("/api")
	{
		// Use methods from the handlers instance
		api.POST("/login", handlers.handleLogin)
		api.GET("/query", handlers.handleQuery)
		api.POST("/sign", handlers.handleSign)
		api.GET("/sign/remarks", handlers.handleSignRemarks)
		api.POST("/sign/photos", handlers.handleSignPhoto)
		api.GET("/search_school", handlers.handleSearchSchoolID)
		api.GET("/reports", handlers.handleReportList)
		api.GET("/reports/template", handlers.handleReportTemplate)
		api.GET("/reports/detail", handlers.handleReportDetail)
		api.POST("/reports", handlers.handleReportSubmit)
		api.POST("/reports/attachments", handlers.handleReportUpload)
		// 根据需要添加更多API端点
	}

	return router
}
