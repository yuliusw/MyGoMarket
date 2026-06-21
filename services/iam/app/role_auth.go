package app

import (
	"log"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
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

	role, err := roleRepo.ReplaceRolePermissions(roleID, req.PermissionIDs)
	if err != nil {
		response.Error(c, http.StatusBadRequest, "ROLE_PERMISSION_UPDATE_FAILED", err.Error())
		return
	}

	purgeCasbinAndBroadcast()
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
