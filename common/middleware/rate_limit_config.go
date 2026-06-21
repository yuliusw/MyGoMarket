package middleware

import (
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/yuliusw/RPA-market/common/config"
	"github.com/yuliusw/RPA-market/common/database"
)

func ConfiguredRateLimit() gin.HandlerFunc {
	if config.AppConfig == nil || !config.AppConfig.Features.RateLimit.Enabled {
		return func(c *gin.Context) {
			c.Next()
		}
	}

	rateLimit := config.AppConfig.Features.RateLimit
	if strings.EqualFold(rateLimit.Backend, "redis") {
		return RedisSlidingWindowMiddleware(
			database.RedisClient,
			durationFromSeconds(rateLimit.WindowSeconds, time.Second),
			intFromConfig(rateLimit.Limit, 100),
		)
	}

	limiter := NewIPRateLimiter(
		floatFromConfig(rateLimit.Rate, 5.0),
		floatFromConfig(rateLimit.Capacity, 10.0),
		durationFromSeconds(rateLimit.CleanupSeconds, 5*time.Minute),
		durationFromSeconds(rateLimit.TTLSeconds, 10*time.Minute),
	)
	return GinRateLimitMiddleware(limiter)
}

func durationFromSeconds(seconds int, defaultValue time.Duration) time.Duration {
	if seconds <= 0 {
		return defaultValue
	}
	return time.Duration(seconds) * time.Second
}

func intFromConfig(value, defaultValue int) int {
	if value <= 0 {
		return defaultValue
	}
	return value
}

func floatFromConfig(value, defaultValue float64) float64 {
	if value <= 0 {
		return defaultValue
	}
	return value
}
