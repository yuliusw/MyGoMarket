package utils

import (
	"errors"

	"github.com/casbin/casbin/v3/model"
	"github.com/casbin/casbin/v3/persist"
	"gorm.io/gorm"
)

type CustomDBAdapter struct {
	db       *gorm.DB
	filtered bool
}

type DomainFilter struct {
	DomainID string
}

func NewCustomDBAdapter(db *gorm.DB) *CustomDBAdapter {
	return &CustomDBAdapter{db: db, filtered: false}
}

func (a *CustomDBAdapter) LoadPolicy(model model.Model) error {
	return a.LoadFilteredPolicy(model, nil)
}

// LoadFilteredPolicy 实现按 DomainID 动态按需加载
func (a *CustomDBAdapter) LoadFilteredPolicy(model model.Model, filter interface{}) error {
	var domainFilter *DomainFilter
	if filter != nil {
		if f, ok := filter.(*DomainFilter); ok {
			domainFilter = f
		} else {
			return errors.New("invalid filter type")
		}
	}

	// 1. 加载所有 P 规则 (全局角色-权限映射)
	type pRule struct {
		RoleName       string `gorm:"column:role_name"`
		PermissionCode string `gorm:"column:permission_code"`
	}
	var pRules []pRule
	err := a.db.Table("role_permissions rp").
		Select("r.role_name, p.permission_code").
		Joins("JOIN roles r ON rp.role_id = r.role_id").
		Joins("JOIN permissions p ON rp.permission_id = p.permission_id").
		Scan(&pRules).Error
	if err != nil {
		return err
	}
	for _, rule := range pRules {
		persist.LoadPolicyLine("p, "+rule.RoleName+", "+rule.PermissionCode, model)
	}

	// 2. 加载 G 规则 (用户-角色-团体映射)
	type gRule struct {
		UserID   string `gorm:"column:user_id"`
		RoleName string `gorm:"column:role_name"`
		GroupID  string `gorm:"column:group_id"`
	}
	var gRules []gRule
	query := a.db.Table("group_members gm").
		Select("CAST(gm.user_id AS VARCHAR) AS user_id, r.role_name, CAST(gm.group_id AS VARCHAR) AS group_id").
		Joins("JOIN roles r ON gm.role_id = r.role_id")

	// 核心：如果传入了指定的域 ID，则 SQL 只拉取该域下的成员关系
	if domainFilter != nil && domainFilter.DomainID != "" {
		query = query.Where("gm.group_id = ?", domainFilter.DomainID)
		a.filtered = true
	}

	err = query.Scan(&gRules).Error
	if err != nil {
		return err
	}
	for _, rule := range gRules {
		persist.LoadPolicyLine("g, "+rule.UserID+", "+rule.RoleName+", "+rule.GroupID, model)
	}

	return nil
}

func (a *CustomDBAdapter) IsFiltered() bool { return a.filtered }
