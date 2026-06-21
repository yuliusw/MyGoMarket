package middleware

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/yuliusw/RPA-market/common/utils"
)

func Cors() gin.HandlerFunc {
	return func(c *gin.Context) {
		method := c.Request.Method
		origin := c.Request.Header.Get("Origin")

		allow := false
		if origin != "" {
			// 遍历后缀切片进行模糊匹配
			for _, suffix := range utils.AllowDomainSuffixes {
				// 匹配规则：或者是完全相等，或者是该域名的子域名
				if origin == "http://"+suffix || origin == "https://"+suffix ||
					strings.HasSuffix(origin, suffix) {
					allow = true
					break
				}
			}
		}

		if allow {
			c.Header("Access-Control-Allow-Origin", origin)
			c.Header("Access-Control-Allow-Credentials", "true")
			c.Header("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
			c.Header("Access-Control-Allow-Headers", "Content-Type, Authorization, Token")
		}

		if method == "OPTIONS" {
			if allow {
				c.AbortWithStatus(http.StatusNoContent)
			} else {
				c.AbortWithStatus(http.StatusForbidden)
			}
			return
		}

		c.Next()
	}
}
