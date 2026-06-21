package rocketmq

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/apache/rocketmq-client-go/v2"
	"github.com/apache/rocketmq-client-go/v2/consumer"
	"github.com/apache/rocketmq-client-go/v2/primitive"
	"github.com/apache/rocketmq-client-go/v2/producer"
	"github.com/yuliusw/RPA-market/common/config"
)

type MQClient struct {
	addr      []string
	producer  rocketmq.Producer
	consumers map[string]rocketmq.PushConsumer
	mu        sync.RWMutex
}

var (
	instance *MQClient
	once     sync.Once
)

func Init() error {
	var err error

	// 确保全局配置已经初始化
	if config.AppConfig == nil {
		return errors.New("application configuration is not initialized")
	}

	once.Do(func() {
		instance = &MQClient{
			// 直接从全局配置中读取 NameServer 地址列表
			addr:      config.AppConfig.RocketMQ.Addrs,
			consumers: make(map[string]rocketmq.PushConsumer),
		}
	})
	return err
}

func (m *MQClient) StartProducer(groupName string) error {
	if m == nil {
		return errors.New("rocketmq client is not initialized")
	}
	p, err := rocketmq.NewProducer(
		producer.WithNameServer(m.addr),
		producer.WithGroupName(groupName),
		producer.WithRetry(2),
	)
	if err != nil {
		return fmt.Errorf("create producer error: %v", err)
	}

	if err := p.Start(); err != nil {
		return fmt.Errorf("start producer error: %v", err)
	}

	m.producer = p
	return nil
}

func (m *MQClient) SendSync(ctx context.Context, topic string, body []byte) (*primitive.SendResult, error) {
	if m == nil {
		return nil, errors.New("rocketmq client is not initialized")
	}
	if m.producer == nil {
		return nil, errors.New("rocketmq producer is not started")
	}
	msg := &primitive.Message{
		Topic: topic,
		Body:  body,
	}
	return m.producer.SendSync(ctx, msg)
}

func (m *MQClient) SendAsync(ctx context.Context, topic string, body []byte, callback func(ctx context.Context, result *primitive.SendResult, err error)) error {
	if m == nil {
		return errors.New("rocketmq client is not initialized")
	}
	if m.producer == nil {
		return errors.New("rocketmq producer is not started")
	}
	msg := &primitive.Message{
		Topic: topic,
		Body:  body,
	}
	return m.producer.SendAsync(ctx, callback, msg)
}

// RegisterConsumer 注册并启动消费者（集群多实例轮询模式 - 适合普通业务消息）
func (m *MQClient) RegisterConsumer(groupName string, topic string, handler func(context.Context, ...*primitive.MessageExt) (consumer.ConsumeResult, error)) error {
	return m.registerConsumerWithModel(groupName, topic, consumer.Clustering, handler)
}

// RegisterBroadcastConsumer 【新增】注册广播模式消费者（多实例同时消费 - 专用于集群缓存热更新）
func (m *MQClient) RegisterBroadcastConsumer(groupName string, topic string, handler func(context.Context, ...*primitive.MessageExt) (consumer.ConsumeResult, error)) error {
	return m.registerConsumerWithModel(groupName, topic, consumer.BroadCasting, handler)
}

// 内部封装：支持指定消费模型
func (m *MQClient) registerConsumerWithModel(groupName string, topic string, model consumer.MessageModel, handler func(context.Context, ...*primitive.MessageExt) (consumer.ConsumeResult, error)) error {
	if m == nil {
		return errors.New("rocketmq client is not initialized")
	}
	c, err := rocketmq.NewPushConsumer(
		consumer.WithNameServer(m.addr),
		consumer.WithGroupName(groupName),
		consumer.WithConsumeFromWhere(consumer.ConsumeFromLastOffset),
		consumer.WithConsumerModel(model), // 设置集群模式或广播模式
	)
	if err != nil {
		return err
	}

	err = c.Subscribe(topic, consumer.MessageSelector{}, handler)
	if err != nil {
		return err
	}

	if err := c.Start(); err != nil {
		return err
	}

	m.mu.Lock()
	m.consumers[groupName] = c
	m.mu.Unlock()
	return nil
}

func (m *MQClient) Close() {
	if m.producer != nil {
		_ = m.producer.Shutdown()
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, c := range m.consumers {
		_ = c.Shutdown()
	}
}

func GetClient() *MQClient {
	return instance
}
