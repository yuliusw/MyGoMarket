package repository

import (
	"context"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/yuliusw/RPA-market/services/iam/domain"
	"gorm.io/gorm"
)

// 定义一个结构体实现 domain.UserRepository 接口
type userRepository struct {
	db    *gorm.DB
	redis *redis.Client
}

// NewUserRepository 构造函数：返回接口类型
func NewUserRepository(db *gorm.DB, redis *redis.Client) domain.UserRepository {
	// 这里的 database.DB 是你在 database 包里定义的全局变量
	return &userRepository{db: db, redis: redis}
}

// 实现 Save 方法
func (r *userRepository) Save(user *domain.User) error {
	return r.db.Save(user).Error
}

// 实现 FindByEmail 方法
func (r *userRepository) FindByEmail(email string) (*domain.User, error) {
	var user domain.User
	if err := r.db.Where("email = ?", email).First(&user).Error; err != nil {
		return nil, err
	}
	return &user, nil
}

// 实现 FindByID 方法
func (r *userRepository) FindByID(id string) (*domain.User, error) {
	var user domain.User
	if err := r.db.First(&user, "user_id = ?", id).Error; err != nil {
		return nil, err
	}
	return &user, nil
}

const UserSessionPrefix = "user:session:"

// SetSession 存储用户的长时会话标识（自动登录凭证 + 顶号白名单）
// expiration 为过期时间，例如 7 * 24 * time.Hour
func (r *userRepository) SetSession(ctx context.Context, userID string, sessionID string, expiration time.Duration) error {
	key := UserSessionPrefix + userID
	return r.redis.Set(ctx, key, sessionID, expiration).Err()
}

// GetSession 获取存储的会话标识
func (r *userRepository) GetSession(ctx context.Context, userID string) (string, error) {
	key := UserSessionPrefix + userID
	return r.redis.Get(ctx, key).Result()
}

// DeleteSession 登出 / 改密 时删除会话
func (r *userRepository) DeleteSession(ctx context.Context, userID string) error {
	key := UserSessionPrefix + userID
	return r.redis.Del(ctx, key).Err()
}
