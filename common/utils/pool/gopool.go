package pool

import (
	"log"
	"sync"
	"time"

	"github.com/panjf2000/ants/v2"
)

var (
	// 定义不同的业务池
	defaultPool *ants.Pool
	ioPool      *ants.Pool
	requestPool *ants.Pool

	// 确保只初始化一次
	defaultOnce sync.Once
	ioOnce      sync.Once
	requestOnce sync.Once
)

// GetDefaultPool 获取普通任务池（单例）
func InitGetDefaultPool() *ants.Pool {
	defaultOnce.Do(func() {
		var err error
		// 普通池：容量适中，满了就阻塞等待
		defaultPool, err = ants.NewPool(200,
			ants.WithExpiryDuration(30*time.Second),
			ants.WithPanicHandler(func(err interface{}) {
				log.Printf("[Gopool-Default] Panic captured: %v", err)
			}),
		)
		if err != nil {
			panic("init default goroutine pool failed: " + err.Error())
		}
	})
	return defaultPool
}

// GetIOPool 获取高并发I/O慢任务池（单例）
func InitGetIOPool() *ants.Pool {
	ioOnce.Do(func() {
		var err error
		// I/O池：容量大，开启非阻塞模式
		// 如果第三方接口挂了导致堆积，池子满了直接拒绝新任务，实现快速失败（Circuit Break）
		ioPool, err = ants.NewPool(1000,
			ants.WithExpiryDuration(15*time.Second),
			ants.WithNonblocking(true),
			ants.WithPanicHandler(func(err interface{}) {
				log.Printf("[Gopool-IO] Panic captured: %v", err)
			}),
		)
		if err != nil {
			panic("init io goroutine pool failed: " + err.Error())
		}
	})
	return ioPool
}

// InitGetRequestPool 获取 HTTP 请求池。池满时 Submit 立即返回 ErrPoolOverload，用于快速失败。
func InitGetRequestPool(capacity int) *ants.Pool {
	requestOnce.Do(func() {
		var err error
		if capacity <= 0 {
			capacity = 1000
		}
		requestPool, err = ants.NewPool(capacity,
			ants.WithExpiryDuration(30*time.Second),
			ants.WithNonblocking(true),
			ants.WithPanicHandler(func(err interface{}) {
				log.Printf("[Gopool-Request] Panic captured: %v", err)
			}),
		)
		if err != nil {
			panic("init request goroutine pool failed: " + err.Error())
		}
	})
	return requestPool
}

// CloseAll 进程退出时优雅关闭所有池子
func CloseAll() {
	if defaultPool != nil {
		defaultPool.Release()
	}
	if ioPool != nil {
		ioPool.Release()
	}
	if requestPool != nil {
		requestPool.Release()
	}
}

// CloseAllWithTimeout 等待池内任务完成，超时后释放资源。
func CloseAllWithTimeout(timeout time.Duration) {
	if defaultPool != nil {
		_ = defaultPool.ReleaseTimeout(timeout)
	}
	if ioPool != nil {
		_ = ioPool.ReleaseTimeout(timeout)
	}
	if requestPool != nil {
		_ = requestPool.ReleaseTimeout(timeout)
	}
}
