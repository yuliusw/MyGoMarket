package repository

import (
	"github.com/yuliusw/RPA-market/common/database"
	"github.com/yuliusw/RPA-market/services/iam/domain"
	"gorm.io/gorm"
)

type groupRepository struct {
	db *gorm.DB
}

func NewGroupRepository() domain.GroupRepository {
	return &groupRepository{db: database.DB}
}

// --- Group 相关操作 ---

func (r *groupRepository) Save(group *domain.Group) error {
	return r.db.Save(group).Error
}

func (r *groupRepository) FindByID(groupID string) (*domain.Group, error) {
	var group domain.Group
	err := r.db.Where("group_id = ?", groupID).First(&group).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, err
		}
		return nil, err
	}
	return &group, nil
}

func (r *groupRepository) Delete(groupID string) error {
	// 如果 domain.Group 包含 gorm.DeletedAt 则为逻辑删除，否则为物理删除
	return r.db.Where("group_id = ?", groupID).Delete(&domain.Group{}).Error
}

// --- Member 相关操作 ---

func (r *groupRepository) AddMember(groupID, userID string, roleID int) error {
	member := &domain.Member{
		GroupID: groupID,
		UserID:  userID,
		RoleID:  roleID,
	}
	return r.db.Create(member).Error
}

func (r *groupRepository) RemoveMember(groupID, userID string) error {
	return r.db.Where("group_id = ? AND user_id = ?", groupID, userID).
		Delete(&domain.Member{}).Error
}

func (r *groupRepository) UpdateMemberRole(groupID, userID string, newRoleID int) error {
	return r.db.Model(&domain.Member{}).
		Where("group_id = ? AND user_id = ?", groupID, userID).
		Update("role_id", newRoleID).Error
}

func (r *groupRepository) GetMember(groupID, userID string) (*domain.Member, error) {
	var member domain.Member
	err := r.db.Preload("Role").
		Preload("Role.Permissions").
		Where("group_id = ? AND user_id = ?", groupID, userID).
		First(&member).Error

	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, err
		}
		return nil, err
	}
	return &member, nil
}

func (r *groupRepository) FindMembersByGroupID(groupID string) ([]*domain.Member, error) {
	var members []*domain.Member
	err := r.db.Preload("Role").
		Where("group_id = ?", groupID).
		Find(&members).Error
	return members, err
}

func (r *groupRepository) FindGroupsByUserID(userID string) ([]*domain.Group, error) {
	var groups []*domain.Group
	// 使用 Joins 关联中间表进行查询
	err := r.db.Table("groups").
		Joins("JOIN group_members ON group_members.group_id = groups.group_id").
		Where("group_members.user_id = ?", userID).
		Find(&groups).Error
	return groups, err
}
