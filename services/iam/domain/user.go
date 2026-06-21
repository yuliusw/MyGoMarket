// services/iam/domain/user.go
package domain

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/yuliusw/RPA-market/common/utils"
)

type User struct {
	// 使用 column 指定数据库字段名，type 指定 UUID 映射
	UserID       string `gorm:"primaryKey;column:user_id;type:uuid"`
	Username     string `gorm:"column:username;unique;not null"`
	Email        string `gorm:"column:email;unique;not null"`
	PasswordHash string `gorm:"column:password_hash;not null"`
	AvatarURL    string `gorm:"column:avatar_url"`
	// autoCreateTime 会让 GORM 在插入时自动处理时间戳
	CreatedAt time.Time `gorm:"column:create_at;autoCreateTime"`
	// autoUpdateTime 会让 GORM 在更新记录时自动刷新该字段
	UpdatedAt time.Time `gorm:"column:update_at;autoUpdateTime"`
	IsActive  bool      `gorm:"column:is_active;default:true"`
}

// NewUser 领域工厂函数：确保创建出的实体是合法的
func NewUser(username, email, password string) (*User, error) {
	hash, err := hashPassword(password)
	if err != nil {
		return nil, err
	}

	// 生成 UUID v7
	u7, _ := uuid.NewV7()
	return &User{
		UserID:       u7.String(),
		Username:     username,
		Email:        email,
		PasswordHash: hash,
		IsActive:     true,
	}, nil
}

// CheckPassword 充血模型：实体自带的验证行为
func (u *User) CheckPassword(password string) bool {
	return utils.CheckPasswordHash(password, u.PasswordHash)
}

// 内部逻辑，不暴露给外部直接操作 Hash
func hashPassword(password string) (string, error) {
	// 这里的 cost 建议设置为 10 或 12，14 在某些环境下性能开销较大
	return utils.HashPassword(password)
}

// UserRepository 仓储接口
type UserRepository interface {
	Save(user *User) error
	FindByEmail(email string) (*User, error)
	FindByID(id string) (*User, error)

	// Session 相关：Redis 长时会话，用于自动登录与顶号判定
	SetSession(ctx context.Context, userID string, sessionID string, expiration time.Duration) error
	GetSession(ctx context.Context, userID string) (string, error)
	DeleteSession(ctx context.Context, userID string) error
}
