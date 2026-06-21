package domain

import (
	"context"
	"database/sql/driver"
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/lib/pq"
)

// Metadata 定义 JSONB 对应的 Go 结构体（根据业务可自行扩展）
type Metadata map[string]interface{}

func (m Metadata) Value() (driver.Value, error) {
	return json.Marshal(m)
}

func (m *Metadata) Scan(value interface{}) error {
	b, ok := value.([]byte)
	if !ok {
		return errors.New("type assertion to []byte failed")
	}
	return json.Unmarshal(b, m)
}

// App 对应应用市场表实体
type App struct {
	AppID       uuid.UUID      `gorm:"type:uuid;primaryKey;default:uuidv7()" json:"app_id"`
	Name        string         `gorm:"type:varchar(255);not null" json:"name"`
	DeveloperID uuid.UUID      `gorm:"type:uuid;not null;index" json:"developer_id"`
	Category    string         `gorm:"type:varchar(50)" json:"category"`
	Tags        pq.StringArray `gorm:"type:text[]" json:"tags"`
	Metadata    Metadata       `gorm:"type:jsonb;default:'{}'" json:"metadata"`
	Status      string         `gorm:"type:varchar(20);default:'published'" json:"status"`
	CreateAt    time.Time      `gorm:"type:timestamp with time zone;default:CURRENT_TIMESTAMP" json:"create_at"`
	UpdateAt    time.Time      `gorm:"type:timestamp with time zone;default:CURRENT_TIMESTAMP" json:"update_at"`
}

// TableName 指定 GORM 表名
func (App) TableName() string {
	return "apps"
}

// AppRepository 仓储接口定义（解耦底层数据库实现）
type AppRepository interface {
	Create(ctx context.Context, app *App) error
	GetByID(ctx context.Context, id uuid.UUID) (*App, error)
	GetByDeveloperIDAndIdempotencyKey(ctx context.Context, developerID uuid.UUID, idempotencyKey string) (*App, error)
	Update(ctx context.Context, app *App) error
	UpdateFields(ctx context.Context, id uuid.UUID, updates map[string]interface{}) (*App, error)
	Delete(ctx context.Context, id uuid.UUID) error
	GetByIDs(ctx context.Context, ids []uuid.UUID) ([]*App, error)
	IncrementDownloadMetric(ctx context.Context, id uuid.UUID, day time.Time) error
	// 扩展 List 方法，支持 keyword 和 status
	List(ctx context.Context, page, pageSize int, keyword, category, status string) ([]*App, int64, error)
}
