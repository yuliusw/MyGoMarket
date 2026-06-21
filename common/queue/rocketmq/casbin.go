package rocketmq

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"time"

	"github.com/apache/rocketmq-client-go/v2/consumer"
	"github.com/apache/rocketmq-client-go/v2/primitive"
	"github.com/yuliusw/RPA-market/common/config"
	"github.com/yuliusw/RPA-market/common/utils"
	// 引入你的 utils 路径
)

type CasbinSyncMessage struct {
	Action string `json:"action"` // 支持 "invalidate_domain" 或 "purge_all"
	Domain string `json:"domain"` // 变更的 group_id (UUID 字符串)
}

// InitCasbinSyncConsumer 注册并启动 Casbin 同步广播消费者
func InitCasbinSyncConsumer(topic string, groupName string) error {
	client := GetClient()
	if client == nil {
		return errors.New("rocketmq client is not initialized")
	}

	err := client.RegisterBroadcastConsumer(groupName, topic, func(ctx context.Context, ext ...*primitive.MessageExt) (consumer.ConsumeResult, error) {
		for _, msg := range ext {
			var syncMsg CasbinSyncMessage
			if err := json.Unmarshal(msg.Body, &syncMsg); err != nil {
				log.Printf("[MQ Casbin] JSON unmarshal error: %v", err)
				continue
			}

			switch syncMsg.Action {
			case "invalidate_domain":
				if syncMsg.Domain != "" && utils.EnforcerPool != nil {
					utils.EnforcerPool.InvalidateDomain(syncMsg.Domain)
				}
			case "purge_all":
				if utils.EnforcerPool != nil {
					utils.EnforcerPool.PurgeAll()
				}
			default:
				log.Printf("[MQ Casbin] Received unknown action: %s", syncMsg.Action)
			}
		}
		return consumer.ConsumeSuccess, nil
	})

	if err != nil {
		return fmt.Errorf("failed to register casbin broadcast consumer: %v", err)
	}

	log.Println("[MQ Casbin] Broadcast consumer for synchronization started successfully.")
	return nil
}

const CasbinSyncTopic = "Topic_Casbin_Sync"

// PublishCasbinSync 发送端：当用户加入退出群组、或修改角色权限时，由业务层主动调用
func PublishCasbinSync(action string, domainID string) error {
	if config.AppConfig == nil || !config.AppConfig.Features.CasbinAuthz {
		log.Printf("[MQ Send] Casbin sync skipped because casbin_authz is disabled, action=%s domain=%s", action, domainID)
		return nil
	}
	client := GetClient()
	if client == nil {
		return errors.New("rocketmq client is not initialized")
	}

	msg := CasbinSyncMessage{
		Action: action,   // "invalidate_domain" 或 "purge_all"
		Domain: domainID, // 对应的 group_id
	}

	body, err := json.Marshal(msg)
	if err != nil {
		log.Printf("[MQ Send] Marshal casbin message failed: %v", err)
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	// 借助注入好的全局 RocketMQ 客户端实例同步发送
	res, err := client.SendSync(ctx, CasbinSyncTopic, body)
	if err != nil {
		log.Printf("[MQ Send] Broadcast casbin sync failed: %v", err)
		return err
	}
	log.Printf("[MQ Send] Broadcast casbin sync success, MsgID: %s", res.MsgID)
	return nil
}
