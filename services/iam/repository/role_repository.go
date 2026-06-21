// services/iam/infrastructure/repository/role_repository.go
package repository

import (
	"errors"

	"github.com/yuliusw/RPA-market/common/database"
	"github.com/yuliusw/RPA-market/services/iam/domain"
	"gorm.io/gorm"
)

type roleRepository struct {
	db *gorm.DB
}

func NewRoleRepository() domain.RoleRepository {
	return &roleRepository{db: database.DB}
}

// FindByID 获取角色及其拥有的所有权限
func (r *roleRepository) FindByID(id int) (*domain.Role, error) {
	var role domain.Role
	// 使用 Preload 自动关联查询 role_permissions 对应的 permissions
	err := r.db.Preload("Permissions").First(&role, id).Error
	if err != nil {
		return nil, err
	}
	return &role, nil
}

func (r *roleRepository) FindByName(name string) (*domain.Role, error) {
	var role domain.Role
	err := r.db.Preload("Permissions").Where("role_name = ?", name).First(&role).Error
	if err != nil {
		return nil, err
	}
	return &role, nil
}

func (r *roleRepository) ListPermissionsByRoleID(roleID int) ([]domain.Permission, error) {
	var permissions []domain.Permission
	// 联表查询：通过 role_permissions 中间表找 permissions
	err := r.db.Joins("JOIN role_permissions ON role_permissions.permission_id = permissions.permission_id").
		Where("role_permissions.role_id = ?", roleID).
		Find(&permissions).Error
	return permissions, err
}

func (r *roleRepository) ListRoles() ([]domain.Role, error) {
	var roles []domain.Role
	err := r.db.Preload("Permissions").Order("role_id ASC").Find(&roles).Error
	return roles, err
}

func (r *roleRepository) ListPermissions() ([]domain.Permission, error) {
	var permissions []domain.Permission
	err := r.db.Order("permission_id ASC").Find(&permissions).Error
	return permissions, err
}

func (r *roleRepository) ReplaceRolePermissions(roleID int, permissionIDs []int) (*domain.Role, error) {
	err := r.db.Transaction(func(tx *gorm.DB) error {
		var role domain.Role
		if err := tx.First(&role, roleID).Error; err != nil {
			return err
		}

		uniqueIDs := make([]int, 0, len(permissionIDs))
		seen := make(map[int]struct{}, len(permissionIDs))
		for _, id := range permissionIDs {
			if id <= 0 {
				return errors.New("invalid permission id")
			}
			if _, ok := seen[id]; ok {
				continue
			}
			seen[id] = struct{}{}
			uniqueIDs = append(uniqueIDs, id)
		}

		if len(uniqueIDs) > 0 {
			var count int64
			if err := tx.Model(&domain.Permission{}).Where("permission_id IN ?", uniqueIDs).Count(&count).Error; err != nil {
				return err
			}
			if count != int64(len(uniqueIDs)) {
				return errors.New("permission id not found")
			}
		}

		if role.Name == "superadmin" {
			var roleManage domain.Permission
			if err := tx.Where("permission_code = ?", "role:manage").First(&roleManage).Error; err != nil {
				return err
			}
			if _, ok := seen[roleManage.ID]; !ok {
				return errors.New("superadmin must keep role:manage permission")
			}
		}

		if err := tx.Exec("DELETE FROM role_permissions WHERE role_id = ?", roleID).Error; err != nil {
			return err
		}

		for _, permissionID := range uniqueIDs {
			if err := tx.Exec(
				"INSERT INTO role_permissions (role_id, permission_id) VALUES (?, ?) ON CONFLICT DO NOTHING",
				roleID, permissionID,
			).Error; err != nil {
				return err
			}
		}

		return nil
	})
	if err != nil {
		return nil, err
	}
	return r.FindByID(roleID)
}
