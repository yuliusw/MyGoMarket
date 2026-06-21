package domain

import (
	"time"

	"github.com/google/uuid"
)

// Group 团体实体
type Group struct {
	GroupID   string    `gorm:"primaryKey;column:group_id;type:uuid"`
	GroupName string    `gorm:"column:group_name;not null"`
	OwnerID   string    `gorm:"column:owner_id;type:uuid"`
	Type      string    `gorm:"column:group_type"`
	CreatedAt time.Time `gorm:"column:created_at"`
}

// Member 团体成员模型 (对应 group_members 表)
type Member struct {
	GroupID string `gorm:"primaryKey;column:group_id;type:uuid"`
	UserID  string `gorm:"primaryKey;column:user_id;type:uuid"`
	RoleID  int    `gorm:"column:role_id"`
	// 关联加载：成员所属的角色及其权限
	Role *Role `gorm:"foreignKey:RoleID"`
}

func (Member) TableName() string {
	return "group_members"
}

// NewGroup 团体工厂
func NewGroup(name string, ownerID string) *Group {
	id, _ := uuid.NewV7()
	return &Group{
		GroupID:   id.String(),
		GroupName: name,
		OwnerID:   ownerID,
		Type:      "standard",
		CreatedAt: time.Now(),
	}
}

// GroupRepository 接口
type GroupRepository interface {
	Save(group *Group) error
	Delete(groupID string) error             // 新增：物理/逻辑删除
	FindByID(groupID string) (*Group, error) // 新增：根据ID获取团体
	AddMember(groupID, userID string, roleID int) error
	RemoveMember(groupID, userID string) error // 新增：移除成员
	GetMember(groupID, userID string) (*Member, error)
	FindGroupsByUserID(userID string) ([]*Group, error)
	UpdateMemberRole(groupID, userID string, newRoleID int) error // 修改成员角色
	FindMembersByGroupID(groupID string) ([]*Member, error)
}
