package middleware

import (
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/yuliusw/RPA-market/common/utils"
)

// IPRateLimiter 管理每个 IP 的令牌桶
type IPRateLimiter struct {
	ips      map[string]*utils.TokenBucket
	mu       sync.RWMutex
	rate     float64
	capacity float64
}

func GinRateLimitMiddleware(limiter *IPRateLimiter) gin.HandlerFunc {
	return func(c *gin.Context) {
		// 获取客户端 IP
		ip := c.ClientIP()

		// 获取该 IP 的令牌桶
		bucket := limiter.GetLimiter(ip)

		// 判断是否允许请求
		if !bucket.Allow() {
			c.AbortWithStatusJSON(http.StatusTooManyRequests, gin.H{
				"code":    http.StatusTooManyRequests,
				"message": "请求过于频繁，请稍后再试",
			})
			return
		}

		// 允许通过，执行下一个 Handler
		c.Next()
	}
}

// NewIPRateLimiter 创建一个 IP 限流管理器，并启动后台清理协程
// rate: 令牌放入速率 (个/秒)
// capacity: 桶的最大容量
// cleanupInterval: 定时清理的执行间隔
// ttl: 令牌桶的存活时间 (超过此时长未访问的 IP 将被清理)
func NewIPRateLimiter(rate, capacity float64, cleanupInterval, ttl time.Duration) *IPRateLimiter {
	limiter := &IPRateLimiter{
		ips:      make(map[string]*utils.TokenBucket),
		rate:     rate,
		capacity: capacity,
	}

	// 启动后台协程定期清理过期 IP
	go limiter.cleanupStaleBuckets(cleanupInterval, ttl)

	return limiter
}

// GetLimiter 获取指定 IP 的令牌桶，如果不存在则创建一个
func (i *IPRateLimiter) GetLimiter(ip string) *utils.TokenBucket {
	i.mu.RLock()
	limiter, exists := i.ips[ip]
	i.mu.RUnlock()

	if !exists {
		i.mu.Lock()
		defer i.mu.Unlock()

		// 双重检查
		limiter, exists = i.ips[ip]
		if !exists {
			limiter = utils.NewTokenBucket(i.rate, i.capacity)
			i.ips[ip] = limiter
		}
	}

	return limiter
}

// cleanupStaleBuckets 定期遍历 map，清理长时间未访问的 IP
func (i *IPRateLimiter) cleanupStaleBuckets(cleanupInterval, ttl time.Duration) {
	ticker := time.NewTicker(cleanupInterval)
	defer ticker.Stop()

	for {
		<-ticker.C // 阻塞等待 ticker 触发
		i.mu.Lock()

		now := time.Now()
		for ip, bucket := range i.ips {
			// 如果当前时间距离最后一次活动的间隔 > ttl，则删除该 IP 记录
			if now.Sub(bucket.GetLastActivity()) > ttl {
				delete(i.ips, ip)
			}
		}

		i.mu.Unlock()
	}
}
