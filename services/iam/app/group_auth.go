package app

import (
	"log"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/yuliusw/RPA-market/common/middleware"
	"github.com/yuliusw/RPA-market/common/queue/rocketmq" // 💡 新增：引入你的 RocketMQ 广播包
	"github.com/yuliusw/RPA-market/common/response"
	"github.com/yuliusw/RPA-market/common/utils"
	service "github.com/yuliusw/RPA-market/services/iam/app/services"
	"github.com/yuliusw/RPA-market/services/iam/domain"
)

// 定义请求参数
type CreateGroupRequest struct {
	Name string `json:"name" binding:"required"`
}

type InviteMemberRequest struct {
	UserID string `json:"user_id" binding:"required"`
	RoleID int    `json:"role_id" binding:"required"`
}

type UpdateRoleRequest struct {
	RoleID int `json:"role_id" binding:"required"`
}

type UpdateGroupRequest struct {
	Name string `json:"name" binding:"required"`
}

var gservice *service.GroupService

// RegisterGroup POST /groups
func RegisterGroup(c *gin.Context) {
	var req CreateGroupRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, http.StatusBadRequest, "INVALID_REQUEST", "参数错误")
		return
	}

	userID := middleware.GetUserID(c)
	group, err := gservice.RegisterGroup(req.Name, userID)
	if err != nil {
		response.Error(c, http.StatusInternalServerError, "GROUP_CREATE_FAILED", err.Error())
		return
	}

	// 注：新注册的 GroupID 在集群任何节点的 LRU 中都未曾缓存过，
	// 伴随第一次 API 请求进入中间件时会自动懒加载，因此这里无需广播驱逐。

	c.JSON(http.StatusCreated, group)
}

// GetGroups GET /groups (获取当前 user 加入的所有团体)
func GetMyGroups(c *gin.Context) {
	userID := middleware.GetUserID(c)
	groups, err := gservice.ListUserGroups(userID)
	if err != nil {
		response.Error(c, http.StatusInternalServerError, "GROUP_LIST_FAILED", err.Error())
		return
	}
	c.JSON(http.StatusOK, groups)
}

// GetGroupDetail GET /groups/:id
func GetGroupDetail(c *gin.Context) {
	groupID := c.Param("id")

	detail, err := gservice.GetGroupFullDetail(groupID)
	if err != nil {
		response.Error(c, http.StatusForbidden, "GROUP_ACCESS_DENIED", err.Error())
		return
	}
	c.JSON(http.StatusOK, detail)
}

// UpdateGroup PUT /groups/:id
func UpdateGroup(c *gin.Context) {
	groupID := c.Param("id")
	var req UpdateGroupRequest

	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, http.StatusBadRequest, "INVALID_REQUEST", err.Error())
		return
	}

	if err := gservice.UpdateGroupInfo(groupID, req.Name); err != nil {
		response.Error(c, http.StatusForbidden, "GROUP_UPDATE_DENIED", err.Error())
		return
	}

	// 注：修改群组名称不影响 Casbin 的角色与权限映射，故无需广播。

	c.JSON(http.StatusOK, gin.H{"message": "更新成功"})
}

// InviteMember POST /groups/:id/members
func InviteMember(c *gin.Context) {
	groupID := c.Param("id")
	var req InviteMemberRequest

	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, http.StatusBadRequest, "INVALID_REQUEST", err.Error())
		return
	}

	err := gservice.InviteMember(groupID, req.UserID, req.RoleID)
	if err != nil {
		response.Error(c, http.StatusForbidden, "GROUP_INVITE_DENIED", err.Error())
		return
	}

	invalidateAndBroadcast(groupID)

	c.JSON(http.StatusOK, gin.H{"message": "邀请成功"})
}

// KickMember DELETE /groups/:id/members/:user_id
func KickMember(c *gin.Context) {
	groupID := c.Param("id")
	targetUserID := c.Param("user_id")

	err := gservice.KickMember(groupID, targetUserID)
	if err != nil {
		response.Error(c, http.StatusForbidden, "GROUP_KICK_DENIED", err.Error())
		return
	}

	invalidateAndBroadcast(groupID)

	c.JSON(http.StatusOK, gin.H{"message": "移除成功"})
}

// LeaveGroup POST /groups/:id/leave
func LeaveGroup(c *gin.Context) {
	groupID := c.Param("id")
	userID := middleware.GetUserID(c)

	if err := gservice.LeaveGroup(userID, groupID); err != nil {
		response.Error(c, http.StatusBadRequest, "GROUP_LEAVE_FAILED", err.Error())
		return
	}

	invalidateAndBroadcast(groupID)

	c.JSON(http.StatusOK, gin.H{"message": "已退出团体"})
}

// DissolveGroup DELETE /groups/:id
func DissolveGroup(c *gin.Context) {
	groupID := c.Param("id")

	if err := gservice.DissolveGroup(groupID); err != nil {
		response.Error(c, http.StatusForbidden, "GROUP_DISSOLVE_DENIED", err.Error())
		return
	}

	invalidateAndBroadcast(groupID)

	c.JSON(http.StatusOK, gin.H{"message": "团体已解散"})
}

func UpdateMemberRole(c *gin.Context) {
	groupID := c.Param("id")
	targetUserID := c.Param("user_id")
	var req UpdateRoleRequest

	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, http.StatusBadRequest, "INVALID_REQUEST", err.Error())
		return
	}

	if err := gservice.UpdateMemberRole(groupID, targetUserID, req.RoleID); err != nil {
		response.Error(c, http.StatusForbidden, "GROUP_MEMBER_ROLE_UPDATE_DENIED", err.Error())
		return
	}

	invalidateAndBroadcast(groupID)
	c.JSON(http.StatusOK, gin.H{"message": "成员角色已更新"})
}

func invalidateAndBroadcast(groupID string) {
	if utils.EnforcerPool != nil {
		utils.EnforcerPool.InvalidateDomain(groupID)
	}
	if err := rocketmq.PublishCasbinSync("invalidate_domain", groupID); err != nil {
		log.Printf("failed to broadcast casbin domain invalidation for %s: %v", groupID, err)
	}
}

func InitGroup(groupRepo domain.GroupRepository) {
	gservice = service.NewGroupService(groupRepo)
}
