package domain

import (
	"time"

	"github.com/google/uuid"
)

type EntitlementStatus string

const (
	EntitlementStatusActive  EntitlementStatus = "active"
	EntitlementStatusRevoked EntitlementStatus = "revoked" // 例如退款后撤销权益
)

// GroupEntitlement 团队权益实例
type GroupEntitlement struct {
	EntitlementID uuid.UUID
	GroupID       uuid.UUID
	BenefitCode   string     // 例如 "app:premium", "max_projects:50"
	SourceType    string     // 例如 "subscription", "system_grant"
	SourceID      *uuid.UUID // 关联的源单据(订阅ID等)
	ExpiredAt     time.Time
	Status        EntitlementStatus
}

// NewGroupEntitlement 工厂方法，发放团队权益
func NewGroupEntitlement(groupID uuid.UUID, code, sourceType string, sourceID *uuid.UUID, validity time.Duration) *GroupEntitlement {
	return &GroupEntitlement{
		EntitlementID: uuid.New(),
		GroupID:       groupID,
		BenefitCode:   code,
		SourceType:    sourceType,
		SourceID:      sourceID,
		ExpiredAt:     time.Now().Add(validity),
		Status:        EntitlementStatusActive,
	}
}

// IsValid 判断当前团队是否真实享有该权益（未撤销且未过期）
func (e *GroupEntitlement) IsValid() bool {
	if e.Status != EntitlementStatusActive {
		return false
	}
	if time.Now().After(e.ExpiredAt) {
		return false
	}
	return true
}

// Revoke 撤销权益
func (e *GroupEntitlement) Revoke() {
	e.Status = EntitlementStatusRevoked
}
