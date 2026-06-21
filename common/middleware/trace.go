package middleware

import (
	"time"

	"github.com/gin-gonic/gin"
	"github.com/yuliusw/RPA-market/common/response"
	"github.com/yuliusw/RPA-market/common/utils/logs"
	"go.uber.org/zap"
)

func GinLogger() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		traceID := response.TraceID(c)

		path := c.Request.URL.Path
		query := c.Request.URL.RawQuery

		c.Next()

		// 2. 请求结束后记录日志
		latency := time.Since(start)
		if logs.Log == nil {
			return
		}
		logs.Log.Info("API_REQUEST",
			zap.Int("status", c.Writer.Status()),
			zap.String("method", c.Request.Method),
			zap.String("path", path),
			zap.String("query", query),
			zap.String("ip", c.ClientIP()),
			zap.String("trace_id", traceID),
			zap.Duration("latency", latency),
		)
	}
}
