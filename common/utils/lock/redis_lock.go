package lock

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"github.com/yuliusw/RPA-market/common/database" // 确保引用了你的 redis 实例
)

var (
	ErrLockFailed = errors.New("failed to acquire lock")
)

// RedisLock 分布式锁结构体
type RedisLock struct {
	key        string
	value      string        // 锁的唯一标识（UUID）
	expiration time.Duration // 过期时间
	client     *redis.Client
}

// NewRedisLock 初始化锁
func NewRedisLock(key string, expiration time.Duration) *RedisLock {
	return &RedisLock{
		key:        "lock:" + key,
		value:      uuid.New().String(), // 生成唯一 ID 防止误解锁
		expiration: expiration,
		client:     database.RedisClient,
	}
}

// Lock 尝试加锁（非阻塞）
func (l *RedisLock) Lock(ctx context.Context) (bool, error) {
	// 使用 SET key value NX PX expiration
	success, err := l.client.SetNX(ctx, l.key, l.value, l.expiration).Result()
	if err != nil {
		return false, err
	}
	return success, nil
}

// Unlock 释放锁（使用 Lua 脚本保证原子性）
func (l *RedisLock) Unlock(ctx context.Context) error {
	// Lua 脚本：判断 value 是否一致，一致则删除
	luaScript := `
		if redis.call("get", KEYS[1]) == ARGV[1] then
			return redis.call("del", KEYS[1])
		else
			return 0
		end
	`
	_, err := l.client.Eval(ctx, luaScript, []string{l.key}, l.value).Result()
	return err
}

// SpinLock 自旋锁（阻塞式获取锁）
func (l *RedisLock) SpinLock(ctx context.Context, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		success, err := l.Lock(ctx)
		if err == nil && success {
			return nil
		}
		// 休息一小会儿再试，避免高频请求压垮 Redis
		time.Sleep(100 * time.Millisecond)
	}
	return ErrLockFailed
}
