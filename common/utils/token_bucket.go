package utils

import (
	"sync"
	"time"
)

// TokenBucket 令牌桶结构体
type TokenBucket struct {
	rate         float64    // 令牌放入速率 (个/秒)
	capacity     float64    // 桶的最大容量
	tokens       float64    // 当前桶中的令牌数
	lastActivity time.Time  // 上次放入令牌的时间
	mu           sync.Mutex // 互斥锁，保证并发安全
}

// NewTokenBucket 创建一个新的令牌桶
func NewTokenBucket(rate float64, capacity float64) *TokenBucket {
	return &TokenBucket{
		rate:         rate,
		capacity:     capacity,
		tokens:       capacity, // 初始化时桶是满的
		lastActivity: time.Now(),
	}
}

// Allow 判断是否允许通过 (取走1个令牌)
func (tb *TokenBucket) Allow() bool {
	tb.mu.Lock()
	defer tb.mu.Unlock()

	now := time.Now()
	// 计算距离上次请求经过的时间，并计算这段时间内产生的令牌数
	elapsed := now.Sub(tb.lastActivity).Seconds()
	newTokens := elapsed * tb.rate

	// 更新令牌数，但不超过桶的容量
	if tb.tokens+newTokens > tb.capacity {
		tb.tokens = tb.capacity
	} else {
		tb.tokens += newTokens
	}

	tb.lastActivity = now

	// 判断是否有足够的令牌
	if tb.tokens >= 1 {
		tb.tokens--
		return true
	}

	return false
}

// GetLastActivity 获取上次访问时间，用于清理过期 IP
func (tb *TokenBucket) GetLastActivity() time.Time {
	tb.mu.Lock()
	defer tb.mu.Unlock()
	return tb.lastActivity
}
