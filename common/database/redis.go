package database

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/yuliusw/RPA-market/common/config"
)

var RedisClient *redis.Client

// InitRedis 初始化 Redis 连接
func InitRedis() {
	redisConf := config.AppConfig.Redis

	RedisClient = redis.NewClient(&redis.Options{
		Addr:         fmt.Sprintf("%s:%d", redisConf.Host, redisConf.Port),
		Password:     redisConf.Password,
		DB:           redisConf.DB,
		PoolSize:     intOrDefault(redisConf.PoolSize, 100),
		MinIdleConns: intOrDefault(redisConf.MinIdleConns, 10),
		DialTimeout:  durationSecondsOrDefault(redisConf.DialTimeoutSeconds, 5*time.Second),
		ReadTimeout:  durationSecondsOrDefault(redisConf.ReadTimeoutSeconds, 3*time.Second),
		WriteTimeout: durationSecondsOrDefault(redisConf.WriteTimeoutSeconds, 3*time.Second),
		PoolTimeout:  durationSecondsOrDefault(redisConf.PoolTimeoutSeconds, 4*time.Second),
	})

	// 使用带有超时的 context 测试连接
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err := RedisClient.Ping(ctx).Result()
	if err != nil {
		log.Fatalf("Failed to connect to Redis: %v", err)
	}

	log.Println("Redis connected successfully")
}

func CloseRedis() error {
	if RedisClient == nil {
		return nil
	}
	return RedisClient.Close()
}
