package respository

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/yuliusw/RPA-market/services/market/domain"
	"gorm.io/gorm"
)

type appRepository struct {
	db *gorm.DB
}

// NewAppRepository 实例化仓储
func NewAppRepository(db *gorm.DB) domain.AppRepository {
	return &appRepository{db: db}
}

func (r *appRepository) Create(ctx context.Context, app *domain.App) error {
	return r.db.WithContext(ctx).Create(app).Error
}

func (r *appRepository) GetByID(ctx context.Context, id uuid.UUID) (*domain.App, error) {
	var app domain.App
	err := r.db.WithContext(ctx).First(&app, "app_id = ?", id).Error
	if err != nil {
		return nil, err
	}
	return &app, nil
}

func (r *appRepository) GetByDeveloperIDAndIdempotencyKey(ctx context.Context, developerID uuid.UUID, idempotencyKey string) (*domain.App, error) {
	var app domain.App
	err := r.db.WithContext(ctx).
		Where("developer_id = ? AND metadata ->> 'idempotency_key' = ?", developerID, idempotencyKey).
		Order("create_at DESC").
		First(&app).Error
	if err != nil {
		return nil, err
	}
	return &app, nil
}

func (r *appRepository) Update(ctx context.Context, app *domain.App) error {
	// Save 会更新所有字段，包括零值
	return r.db.WithContext(ctx).Save(app).Error
}

func (r *appRepository) UpdateFields(ctx context.Context, id uuid.UUID, updates map[string]interface{}) (*domain.App, error) {
	result := r.db.WithContext(ctx).Model(&domain.App{}).Where("app_id = ?", id).Updates(updates)
	if result.Error != nil {
		return nil, result.Error
	}
	if result.RowsAffected == 0 {
		return nil, gorm.ErrRecordNotFound
	}
	return r.GetByID(ctx, id)
}

func (r *appRepository) Delete(ctx context.Context, id uuid.UUID) error {
	return r.db.WithContext(ctx).Delete(&domain.App{}, "app_id = ?", id).Error
}

func (r *appRepository) GetByIDs(ctx context.Context, ids []uuid.UUID) ([]*domain.App, error) {
	var apps []*domain.App
	if len(ids) == 0 {
		return apps, nil
	}
	if err := r.db.WithContext(ctx).Where("app_id IN ?", ids).Find(&apps).Error; err != nil {
		return nil, err
	}
	return apps, nil
}

func (r *appRepository) IncrementDownloadMetric(ctx context.Context, id uuid.UUID, day time.Time) error {
	metricDate := time.Date(day.Year(), day.Month(), day.Day(), 0, 0, 0, 0, time.UTC)
	return r.db.WithContext(ctx).Exec(`
		INSERT INTO app_download_metrics (app_id, metric_date, download_count, updated_at)
		VALUES (?, ?, 1, CURRENT_TIMESTAMP)
		ON CONFLICT (app_id, metric_date)
		DO UPDATE SET download_count = app_download_metrics.download_count + 1, updated_at = CURRENT_TIMESTAMP
	`, id, metricDate).Error
}

func (r *appRepository) List(ctx context.Context, page, pageSize int, keyword, category, status string) ([]*domain.App, int64, error) {
	var apps []*domain.App
	var total int64

	query := r.db.WithContext(ctx).Model(&domain.App{})

	// 1. 关键词模糊搜索 (匹配名称)
	if keyword != "" {
		// 注意：不同数据库模糊查询语法不同，PostgreSQL 建议用 ILIKE 忽略大小写
		query = query.Where("name ILIKE ?", "%"+keyword+"%")
	}
	// 2. 分类过滤
	if category != "" {
		query = query.Where("category = ?", category)
	}
	// 3. 状态过滤 (默认可能只展示 'published')
	if status != "" {
		query = query.Where("status = ?", status)
	}

	// 统计总数
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	// 分页查询
	offset := (page - 1) * pageSize
	err := query.Offset(offset).Limit(pageSize).Order("create_at DESC").Find(&apps).Error
	if err != nil {
		return nil, 0, err
	}

	return apps, total, nil
}
