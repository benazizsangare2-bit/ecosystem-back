package routes

import (
	"ecosystem/docs"
	"io/fs"
	"net/http"

	"github.com/gin-gonic/gin"
)

func registerDocs(router *gin.Engine) {
	router.GET("/api/docs/openapi.yaml", serveEmbedded("openapi.yaml", "application/yaml; charset=utf-8"))
	router.GET("/api/docs", serveEmbedded("swagger.html", "text/html; charset=utf-8"))
	router.GET("/api/docs/", func(c *gin.Context) {
		c.Redirect(http.StatusMovedPermanently, "/api/docs")
	})
}

func serveEmbedded(filename, contentType string) gin.HandlerFunc {
	return func(c *gin.Context) {
		data, err := fs.ReadFile(docs.Content, filename)
		if err != nil {
			c.JSON(http.StatusNotFound, gin.H{
				"success": false,
				"error":   "documentation file not found",
				"code":    404,
			})
			return
		}
		c.Data(http.StatusOK, contentType, data)
	}
}
