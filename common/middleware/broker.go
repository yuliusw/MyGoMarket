package middleware

import (
	"errors"
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

type State int

const (
	StateClosed State = iota
	StateOpen
	StateHalfOpen
)

type CircuitBreaker struct {
	mu sync.RWMutex

	state        State
	failureCount int
	maxFailures  int           // 触发熔断的连续失败阈值
	timeout      time.Duration // 熔断后冷却时间 (多久后进入半开状态)
	lastFailure  time.Time     // 上次失败时间
}

func NewCircuitBreaker(maxFailures int, timeout time.Duration) *CircuitBreaker {
	return &CircuitBreaker{
		state:       StateClosed,
		maxFailures: maxFailures,
		timeout:     timeout,
	}
}

// Allow 检查当前请求是否允许通过
func (cb *CircuitBreaker) Allow() bool {
	cb.mu.RLock()
	state := cb.state
	lastFailure := cb.lastFailure
	cb.mu.RUnlock()

	switch state {
	case StateClosed:
		return true
	case StateOpen:
		// 判断冷却时间是否已过，如果过了则尝试进入半开状态
		if time.Since(lastFailure) > cb.timeout {
			cb.mu.Lock()
			// Double check
			if cb.state == StateOpen {
				cb.state = StateHalfOpen
			}
			cb.mu.Unlock()
			return true // 允许一个探针请求通过
		}
		return false
	case StateHalfOpen:
		// 半开状态下，只允许非常有限的请求通过（这里简化处理，交由并发锁控制探针）
		return true
	}
	return false
}

// ReportResult 请求结束后汇报结果 (成功或失败)
func (cb *CircuitBreaker) ReportResult(err error) {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	if err != nil {
		// 请求失败
		cb.failureCount++
		cb.lastFailure = time.Now()
		if cb.failureCount >= cb.maxFailures {
			cb.state = StateOpen
		}
	} else {
		// 请求成功
		cb.failureCount = 0
		cb.state = StateClosed
	}
}

// CircuitBreakerMiddleware 熔断器中间件
func CircuitBreakerMiddleware(maxFailures int, timeout time.Duration) gin.HandlerFunc {
	cb := NewCircuitBreaker(maxFailures, timeout)

	return func(c *gin.Context) {
		if !cb.Allow() {
			c.JSON(http.StatusServiceUnavailable, gin.H{
				"code": 503,
				"msg":  "服务繁忙，已触发熔断保护，请稍后重试",
			})
			c.Abort()
			return
		}

		c.Next()

		// 检查是否有错误发生 (可以通过 c.Errors 或者特定的 HTTP 状态码来判断)
		// 这里假设 5xx 错误算作服务失败
		var err error
		if c.Writer.Status() >= 500 {
			err = errors.New("server internal error")
		} else if len(c.Errors) > 0 {
			err = c.Errors.Last()
		}

		// 汇报结果以更新状态机
		cb.ReportResult(err)
	}
}
