package routes

import (
	"ecosystem/handlers"
	"ecosystem/middleware"
	"ecosystem/utils"
	"net/http"
	"os"

	"github.com/gin-gonic/gin"
)

func Setup(router *gin.Engine) {
	router.GET("/api/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"success": true, "data": gin.H{"status": "ok"}, "message": "healthy"})
	})

	router.GET("/api", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"success": true,
			"data":    gin.H{"message": "Welcome to the ecosystem backend"},
			"message": "ok",
		})
	})

	registerDocs(router)

	uploadPath := utils.GetEnv("UPLOAD_DIR", "./uploads")
	_ = os.MkdirAll(uploadPath, 0o755)
	router.Static("/uploads", uploadPath)

	api := router.Group("/api")
	{
		api.POST("/register", handlers.Register)
		api.GET("/verify-email", handlers.VerifyEmail)
		api.POST("/verify-email", handlers.VerifyEmail)
		api.POST("/login", handlers.Login)
		api.POST("/forgot-password", handlers.ForgotPassword)
		api.POST("/reset-password", handlers.ResetPassword)

		api.GET("/reports/public", handlers.GetPublicReports)
		api.GET("/reports/:id/comments", handlers.GetComments)

		auth := api.Group("")
		auth.Use(middleware.AuthRequired())
		{
			auth.POST("/logout", handlers.Logout)
			auth.POST("/change-password", handlers.ChangePassword)
			auth.GET("/profile", handlers.GetProfile)

			auth.POST("/reports", handlers.CreateReport)
			auth.GET("/reports/user", handlers.GetUserReports)
			auth.GET("/reports/:id", handlers.GetReport)
			auth.PUT("/reports/:id", handlers.UpdateReport)
			auth.DELETE("/reports/:id", handlers.DeleteReport)
			auth.POST("/reports/:id/like", handlers.ToggleLike)
			auth.POST("/reports/:id/comments", handlers.AddComment)

			admin := auth.Group("/admin")
			admin.Use(middleware.AdminRequired())
			{
				admin.GET("/reports", handlers.GetAdminReports)
				admin.PUT("/reports/:id/status", handlers.UpdateReportStatus)
				admin.GET("/stats", handlers.GetAdminStats)
				admin.GET("/auditlogs", handlers.GetAuditLogs)
    			admin.GET("/auditlogs/actions", handlers.GetAuditLogActions)
			}
		}
	}
}
