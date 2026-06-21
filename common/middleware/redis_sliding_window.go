package middleware

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
)

// Redis 中执行的 Lua 脚本，保证滑动窗口操作的原子性
// KEYS[1]: 限流的 Key (例如: rate_limit:192.168.1.1)
// ARGV[1]: 当前时间戳 (毫秒)
// ARGV[2]: 窗口大小 (毫秒)
// ARGV[3]: 窗口内允许的最大请求数
// ARGV[4]: 唯一请求标识 (member)
const slidingWindowLua = `
local key = KEYS[1]
local now = tonumber(ARGV[1])
local window = tonumber(ARGV[2])
local limit = tonumber(ARGV[3])
local member = ARGV[4]

-- 1. 清除窗口外的过期请求记录 (时间戳小于 now - window 的)
local clearBefore = now - window
redis.call('ZREMRANGEBYSCORE', key, '-inf', clearBefore)

-- 2. 获取当前窗口内的请求总数
local count = redis.call('ZCARD', key)

-- 3. 判断是否超过限流阈值
if count < limit then
    -- 未超限：将当前请求加入 ZSET，score 和 member 都用时间戳/唯一ID
    redis.call('ZADD', key, now, member)
    -- 设置 Key 的过期时间，防止冷门 IP 占用 Redis 内存
    redis.call('PEXPIRE', key, window)
    return 1 -- 允许通过
else
    return 0 -- 拒绝通过
end
`

// RedisLimiter 限流器结构体
type RedisLimiter struct {
	client *redis.Client
	script *redis.Script
}

// NewRedisLimiter 创建全局 Redis 限流器
func NewRedisLimiter(client *redis.Client) *RedisLimiter {
	return &RedisLimiter{
		client: client,
		script: redis.NewScript(slidingWindowLua),
	}
}

// AllowIP 判断特定 IP 是否允许通过
func (rl *RedisLimiter) AllowIP(ctx context.Context, ip string, windowSize time.Duration, limit int) (bool, error) {
	key := fmt.Sprintf("rpa:ratelimit:ip:%s", ip)
	now := time.Now().UnixMilli()
	windowMs := windowSize.Milliseconds()

	// 使用 UUID 保证每次请求在 ZSET 中的 member 唯一 (防止同一毫秒内的并发请求被覆盖)
	member := uuid.New().String()

	// 执行 Lua 脚本
	result, err := rl.script.Run(ctx, rl.client, []string{key}, now, windowMs, limit, member).Result()
	if err != nil {
		return false, err
	}

	// 1 表示允许通过，0 表示限流
	return result.(int64) == 1, nil
}

// RedisSlidingWindowMiddleware Gin 的全局限流中间件
func RedisSlidingWindowMiddleware(redisClient *redis.Client, windowSize time.Duration, limit int) gin.HandlerFunc {
	limiter := NewRedisLimiter(redisClient)

	return func(c *gin.Context) {
		clientIP := c.ClientIP()

		allowed, err := limiter.AllowIP(c.Request.Context(), clientIP, windowSize, limit)
		if err != nil {
			// Redis 挂了的情况，通常有两种策略：
			// 1. 降级放行 (这里采用降级放行，保证核心业务不被 Redis 拖垮)
			// 2. 严格拦截 (返回 500)
			// fmt.Printf("Redis 限流器异常: %v\n", err)
			c.Next()
			return
		}

		if !allowed {
			c.JSON(http.StatusTooManyRequests, gin.H{
				"code": 429,
				"msg":  "访问过于频繁，请稍后再试",
			})
			c.Abort()
			return
		}

		c.Next()
	}
}
