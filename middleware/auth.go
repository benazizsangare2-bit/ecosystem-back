package middleware

import (
	"ecosystem/utils"
	"strings"

	"github.com/gin-gonic/gin"
)

const (
	ContextUserID    = "user_id"
	ContextEmail     = "email"
	ContextRole      = "role"
	ContextJTI       = "jti"
	ContextTokenExp  = "token_exp"
)

func AuthRequired() gin.HandlerFunc {
	return func(c *gin.Context) {
		tokenString := extractBearer(c)
		if tokenString == "" {
			utils.Unauthorized(c, "Authorization header required")
			c.Abort()
			return
		}

		claims, err := utils.ParseToken(tokenString)
		if err != nil || claims.Purpose != utils.PurposeAccess {
			utils.Unauthorized(c, "Invalid or expired token")
			c.Abort()
			return
		}

		blacklisted, err := utils.IsTokenBlacklisted(claims.JTI)
		if err != nil {
			utils.InternalError(c, "Authentication check failed")
			c.Abort()
			return
		}
		if blacklisted {
			utils.Unauthorized(c, "Token has been revoked")
			c.Abort()
			return
		}

		c.Set(ContextUserID, claims.UserID)
		c.Set(ContextEmail, claims.Email)
		c.Set(ContextRole, claims.Role)
		c.Set(ContextJTI, claims.JTI)
		if claims.ExpiresAt != nil {
			c.Set(ContextTokenExp, claims.ExpiresAt.Time)
		}
		c.Next()
	}
}

func AdminRequired() gin.HandlerFunc {
	return func(c *gin.Context) {
		role, _ := c.Get(ContextRole)
		roleStr, _ := role.(string)
		if roleStr != "admin" && roleStr != "authority" {
			utils.Forbidden(c, "Admin access required")
			c.Abort()
			return
		}
		c.Next()
	}
}

func extractBearer(c *gin.Context) string {
	header := c.GetHeader("Authorization")
	if header == "" {
		return ""
	}
	parts := strings.SplitN(header, " ", 2)
	if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
		return ""
	}
	return strings.TrimSpace(parts[1])
}

func GetUserID(c *gin.Context) int {
	id, _ := c.Get(ContextUserID)
	userID, _ := id.(int)
	return userID
}

func GetRole(c *gin.Context) string {
	role, _ := c.Get(ContextRole)
	roleStr, _ := role.(string)
	return roleStr
}

func GetJTI(c *gin.Context) string {
	jti, _ := c.Get(ContextJTI)
	jtiStr, _ := jti.(string)
	return jtiStr
}
