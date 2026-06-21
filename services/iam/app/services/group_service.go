package service

import (
	"errors"

	"github.com/yuliusw/RPA-market/services/iam/domain"
)

type GroupService struct {
	repo domain.GroupRepository
}

func NewGroupService(groupRepo domain.GroupRepository) *GroupService {
	return &GroupService{
		repo: groupRepo,
	}
}

// 1. 团体注册
func (s *GroupService) RegisterGroup(name string, ownerID string) (*domain.Group, error) {
	group := domain.NewGroup(name, ownerID)

	if err := s.repo.Save(group); err != nil {
		return nil, err
	}

	// 角色种子数据中 owner 的固定 role_id 为 2。
	err := s.repo.AddMember(group.GroupID, ownerID, 2)
	return group, err
}

// 2. 修改团体信息
func (s *GroupService) UpdateGroupInfo(groupID string, newName string) error {
	// 业务逻辑：先从 repo 取出原 group
	group, err := s.repo.FindByID(groupID)
	if err != nil {
		return err
	}
	if group == nil {
		return errors.New("团体不存在")
	}

	// 仅修改需要变动的字段
	group.GroupName = newName
	return s.repo.Save(group)
}

// 3. 成员管理：添加成员
func (s *GroupService) InviteMember(groupID string, targetUserID string, roleID int) error {
	return s.repo.AddMember(groupID, targetUserID, roleID)
}

// 4. 修改成员角色
func (s *GroupService) UpdateMemberRole(groupID string, targetUserID string, newRoleID int) error {
	// 核心业务校验：不能修改所有者(Owner)的角色
	group, err := s.repo.FindByID(groupID)
	if err != nil {
		return err
	}
	if group == nil {
		return errors.New("团体不存在")
	}
	if group.OwnerID == targetUserID {
		return errors.New("禁止操作：不能修改所有者的角色")
	}

	// 校验目标成员是否存在
	_, err = s.repo.GetMember(groupID, targetUserID)
	if err != nil {
		return errors.New("目标成员不存在于该团体")
	}

	return s.repo.UpdateMemberRole(groupID, targetUserID, newRoleID)
}

// 5. 退出团体
func (s *GroupService) LeaveGroup(userID string, groupID string) error {
	group, err := s.repo.FindByID(groupID)
	if err != nil {
		return err
	}
	if group == nil {
		return errors.New("团体不存在")
	}

	// 核心业务校验：如果是 Owner，强制要求先转让或解散
	if group.OwnerID == userID {
		return errors.New("所有者不能直接退出，请先转让所有权或解散团体")
	}

	return s.repo.RemoveMember(groupID, userID)
}

// GroupFullDetail 聚合实体，用于返回给前端
type GroupFullDetail struct {
	Group   *domain.Group
	Members []*domain.Member
}

// 6. 获取团体完整详情（包含成员列表）
func (s *GroupService) GetGroupFullDetail(groupID string) (*GroupFullDetail, error) {
	group, err := s.repo.FindByID(groupID)
	if err != nil {
		return nil, err
	}

	members, err := s.repo.FindMembersByGroupID(groupID)
	if err != nil {
		return nil, err
	}

	return &GroupFullDetail{
		Group:   group,
		Members: members,
	}, nil
}

// 7. 解散团体
func (s *GroupService) DissolveGroup(groupID string) error {
	// 检查团体是否存在
	group, err := s.repo.FindByID(groupID)
	if err != nil || group == nil {
		return errors.New("团体不存在")
	}

	// 执行删除（在 repository 实现中可以是软删除）
	return s.repo.Delete(groupID)
}

// 8. 移除成员
func (s *GroupService) KickMember(groupID string, targetUserID string) error {
	return s.repo.RemoveMember(groupID, targetUserID)
}

// 9. 获取团体详情
func (s *GroupService) GetGroupDetail(groupID string) (*domain.Group, error) {
	return s.repo.FindByID(groupID)
}

// 10. 获取用户加入的所有团体
func (s *GroupService) ListUserGroups(userID string) ([]*domain.Group, error) {
	return s.repo.FindGroupsByUserID(userID)
}
