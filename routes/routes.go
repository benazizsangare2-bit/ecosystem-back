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
			auth.DELETE("/profile", handlers.DeleteAccount)

			auth.POST("/reports", handlers.CreateReport)
			auth.GET("/reports/user", handlers.GetUserReports)
			auth.GET("/reports/:id", handlers.GetReport)
			auth.PUT("/reports/:id", handlers.UpdateReport)
			auth.DELETE("/reports/:id", handlers.DeleteReport)
			auth.POST("/reports/:id/like", handlers.ToggleLike)
			auth.POST("/reports/:id/comments", handlers.AddComment)
			 // NEW: Report data for printing/export
   			auth.GET("/reports/:id/full", handlers.GetReportData)
   			auth.GET("/reports/:id/statistics", handlers.GetReportStatistics)
			auth.GET("/reports/:id/history", handlers.GetReportHistory)
    	    auth.GET("/reports/:id/attachments", handlers.GetAttachments)
    	    auth.GET("/reports/:id/printable", handlers.GetPrintableReport)
    	    auth.POST("/reports/:id/print-preview", handlers.PrintPreview)

			admin := auth.Group("/admin")
			admin.Use(middleware.AdminRequired())
			{
				admin.GET("/reports", handlers.GetAdminReports)
				admin.PUT("/reports/:id/status", handlers.UpdateReportStatus)
				admin.GET("/stats", handlers.GetAdminStats)
				admin.GET("/auditlogs", handlers.GetAuditLogs)
    			admin.GET("/auditlogs/actions", handlers.GetAuditLogActions)
    			admin.GET("/users", handlers.ListUsers)
    			admin.DELETE("/users/:id", handlers.AdminDeleteUser)
                admin.GET("/system-report", handlers.GetSystemReport)
                admin.GET("/system-report/pdf", handlers.GetSystemReportPDF)
			// NEW: Admin dashboard with charts
   			admin.GET("/dashboard/stats", handlers.GetDashboardStats)
   			admin.GET("/dashboard/trends", handlers.GetWeeklyTrends)
   			admin.GET("/dashboard/categories", handlers.GetCategoryDistribution)
			// New download as pdf
			admin.GET("/reports/:id/pdf", handlers.DownloadReportPDF)
			}
		}
	}
}
