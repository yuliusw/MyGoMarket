package domain

import "time"

// Permission 权限实体
type Permission struct {
	ID          int       `gorm:"primaryKey;column:permission_id"`
	Code        string    `gorm:"column:permission_code;unique;not null"`
	Description string    `gorm:"column:description"`
	CreatedAt   time.Time `gorm:"column:created_at"`
}

// Role 角色实体
type Role struct {
	ID          int    `gorm:"primaryKey;column:role_id"`
	Name        string `gorm:"column:role_name;unique;not null"`
	Description string `gorm:"column:description"`
	// Many2Many 关联：通过 role_permissions 中间表连接
	Permissions []Permission `gorm:"many2many:role_permissions;foreignKey:ID;joinForeignKey:RoleID;References:ID;joinReferences:PermissionID"`
}

// HasPermission 检查该角色是否包含特定权限码
func (r *Role) HasPermission(code string) bool {
	for _, p := range r.Permissions {
		if p.Code == code {
			return true
		}
	}
	return false
}

// RoleRepository 接口
type RoleRepository interface {
	FindByID(id int) (*Role, error)
	FindByName(name string) (*Role, error)
	ListRoles() ([]Role, error)
	ListPermissions() ([]Permission, error)
	ReplaceRolePermissions(roleID int, permissionIDs []int) (*Role, error)
}
