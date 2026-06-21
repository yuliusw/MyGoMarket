package app

import (
	"log"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/yuliusw/RPA-market/common/audit"
	"github.com/yuliusw/RPA-market/common/queue/rocketmq"
	"github.com/yuliusw/RPA-market/common/response"
	"github.com/yuliusw/RPA-market/common/utils"
	"github.com/yuliusw/RPA-market/services/iam/domain"
)

type ReplaceRolePermissionsRequest struct {
	PermissionIDs []int `json:"permission_ids" binding:"required"`
}

var roleRepo domain.RoleRepository

func InitRole(repo domain.RoleRepository) {
	roleRepo = repo
}

func ListRoles(c *gin.Context) {
	roles, err := roleRepo.ListRoles()
	if err != nil {
		response.Error(c, http.StatusInternalServerError, "ROLE_LIST_FAILED", "Failed to list roles")
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": roles})
}

func ListPermissions(c *gin.Context) {
	permissions, err := roleRepo.ListPermissions()
	if err != nil {
		response.Error(c, http.StatusInternalServerError, "PERMISSION_LIST_FAILED", "Failed to list permissions")
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": permissions})
}

func GetRole(c *gin.Context) {
	roleID, err := strconv.Atoi(c.Param("role_id"))
	if err != nil {
		response.Error(c, http.StatusBadRequest, "INVALID_ROLE_ID", "Invalid role ID")
		return
	}

	role, err := roleRepo.FindByID(roleID)
	if err != nil {
		response.Error(c, http.StatusNotFound, "ROLE_NOT_FOUND", "Role not found")
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": role})
}

func ReplaceRolePermissions(c *gin.Context) {
	roleID, err := strconv.Atoi(c.Param("role_id"))
	if err != nil {
		response.Error(c, http.StatusBadRequest, "INVALID_ROLE_ID", "Invalid role ID")
		return
	}

	var req ReplaceRolePermissionsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, http.StatusBadRequest, "INVALID_REQUEST", err.Error())
		return
	}

	before, _ := roleRepo.FindByID(roleID)
	role, err := roleRepo.ReplaceRolePermissions(roleID, req.PermissionIDs)
	if err != nil {
		emitRolePermissionAudit(c, "role_permissions_update_failed", roleID, before, nil, req.PermissionIDs, err)
		response.Error(c, http.StatusBadRequest, "ROLE_PERMISSION_UPDATE_FAILED", err.Error())
		return
	}

	purgeCasbinAndBroadcast()
	emitRolePermissionAudit(c, "role_permissions_updated", roleID, before, role, req.PermissionIDs, nil)
	c.JSON(http.StatusOK, gin.H{"message": "角色权限已更新", "data": role})
}

func purgeCasbinAndBroadcast() {
	if utils.EnforcerPool != nil {
		utils.EnforcerPool.PurgeAll()
	}
	if err := rocketmq.PublishCasbinSync("purge_all", ""); err != nil {
		log.Printf("failed to broadcast casbin purge all: %v", err)
	}
}

func emitRolePermissionAudit(c *gin.Context, event string, roleID int, before, after *domain.Role, requested []int, err error) {
	actorID, _ := c.Get("user_id")
	metadata := map[string]interface{}{
		"role_id":                  roleID,
		"requested_permission_ids": requested,
	}
	if before != nil {
		metadata["before_permissions"] = permissionCodes(before.Permissions)
		metadata["role_name"] = before.Name
	}
	if after != nil {
		metadata["after_permissions"] = permissionCodes(after.Permissions)
		metadata["role_name"] = after.Name
	}
	auditEvent := audit.Event{EventType: event, TraceID: response.TraceID(c), ActorID: stringify(actorID), Resource: strconv.Itoa(roleID), Metadata: metadata}
	if err != nil {
		auditEvent.Error = err.Error()
	}
	audit.Emit(auditEvent)
}

func permissionCodes(permissions []domain.Permission) []string {
	codes := make([]string, 0, len(permissions))
	for _, permission := range permissions {
		codes = append(codes, permission.Code)
	}
	return codes
}

func stringify(value interface{}) string {
	if s, ok := value.(string); ok {
		return s
	}
	return ""
}
