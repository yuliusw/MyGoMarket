package middleware

import (
	"github.com/gin-gonic/gin"
	"github.com/yuliusw/RPA-market/common/config"
)

func OptionalJWTAuth() gin.HandlerFunc {
	if config.AppConfig == nil || config.AppConfig.Features.JWTAuth {
		return JWTAuth()
	}

	return func(c *gin.Context) {
		userID := config.AppConfig.Features.AuthBypassUserID
		if userID == "" {
			userID = "k6-test-user"
		}
		c.Set(ContextUserIDKey, userID)
		c.Next()
	}
}

func OptionalCasbinRequire(requirePermission string) gin.HandlerFunc {
	if config.AppConfig == nil || config.AppConfig.Features.CasbinAuthz {
		return CasbinRequire(requirePermission)
	}

	return func(c *gin.Context) {
		c.Next()
	}
}

func OptionalCors() gin.HandlerFunc {
	if config.AppConfig == nil || config.AppConfig.Features.CORS {
		return Cors()
	}

	return func(c *gin.Context) {
		c.Next()
	}
}
