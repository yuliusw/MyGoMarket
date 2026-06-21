package kafka

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/segmentio/kafka-go"
)

type KafkaClient struct {
	brokers []string
	writers map[string]*kafka.Writer
	mu      sync.RWMutex
}

var (
	instance *KafkaClient
	once     sync.Once
)

// Init 初始化基础配置
func Init(brokers []string) {
	once.Do(func() {
		instance = &KafkaClient{
			brokers: brokers,
			writers: make(map[string]*kafka.Writer),
		}
	})
}

func GetClient() *KafkaClient {
	return instance
}

// getWriter 获取或创建指定 Topic 的 Writer
func (k *KafkaClient) getWriter(topic string) *kafka.Writer {
	k.mu.RLock()
	w, ok := k.writers[topic]
	k.mu.RUnlock()
	if ok {
		return w
	}

	k.mu.Lock()
	defer k.mu.Unlock()
	// 双重检查
	if w, ok = k.writers[topic]; ok {
		return w
	}

	newWriter := &kafka.Writer{
		Addr:                   kafka.TCP(k.brokers...),
		Topic:                  topic,
		Balancer:               &kafka.LeastBytes{}, // 负载均衡策略
		WriteTimeout:           10 * time.Second,
		RequiredAcks:           kafka.RequireAll, // 保证高可用
		AllowAutoTopicCreation: true,
	}
	k.writers[topic] = newWriter
	return newWriter
}

// Produce 发送消息
func (k *KafkaClient) Produce(ctx context.Context, topic string, key, value []byte) error {
	w := k.getWriter(topic)
	return w.WriteMessages(ctx, kafka.Message{
		Key:   key,
		Value: value,
	})
}

// ConsumerHandler 定义消费逻辑回调
type ConsumerHandler func(ctx context.Context, msg kafka.Message) error

// Consume 启动一个消费者组监听
func (k *KafkaClient) Consume(ctx context.Context, groupID, topic string, handler ConsumerHandler) {
	reader := kafka.NewReader(kafka.ReaderConfig{
		Brokers:        k.brokers,
		GroupID:        groupID,
		Topic:          topic,
		MinBytes:       10e3,        // 10KB
		MaxBytes:       10e6,        // 10MB
		CommitInterval: time.Second, // 自动提交间隔
	})

	defer reader.Close()

	for {
		// 监听 ctx 退出，防止 goroutine 泄露
		select {
		case <-ctx.Done():
			return
		default:
			msg, err := reader.FetchMessage(ctx) // 使用 Fetch + Commit 保证不丢消息
			if err != nil {
				fmt.Printf("Read message error: %v\n", err)
				continue
			}

			// 执行业务逻辑
			if err := handler(ctx, msg); err == nil {
				// 逻辑执行成功后手动提交偏移量
				_ = reader.CommitMessages(ctx, msg)
			}
		}
	}
}

// Close 关闭所有生产者连接
func (k *KafkaClient) Close() {
	k.mu.Lock()
	defer k.mu.Unlock()
	for _, w := range k.writers {
		_ = w.Close()
	}
}
