package middleware

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/yuliusw/RPA-market/common/response"
	"github.com/yuliusw/RPA-market/common/utils" // 请替换为你项目中 utils 的真实包导入路径
)

// CasbinRequire 定义按域隔离的分布式高性能权限校验中间件
func CasbinRequire(requirePermission string) gin.HandlerFunc {
	return func(c *gin.Context) {
		userID := GetUserID(c)
		if userID == "" {
			response.Error(c, http.StatusUnauthorized, "UNAUTHORIZED", "未登录")
			c.Abort()
			return
		}

		groupID := resourceDomain(c)
		// 如果路由不含具体域，自动归属到你的系统初始化全局域 UUID
		if groupID == "" {
			groupID = "11111111-1111-1111-1111-111111111111"
		}

		// 从 LRU 池获取 Enforcer，免去全局锁，相互之间互不影响
		enforcer, err := utils.EnforcerPool.GetEnforcer(groupID)
		if err != nil {
			response.Error(c, http.StatusInternalServerError, "AUTHZ_LOAD_FAILED", "系统鉴权加载失败")
			c.Abort()
			return
		}

		// 提问小 Enforcer 进行校验
		ok, err := enforcer.Enforce(userID, groupID, requirePermission)
		if err != nil {
			response.Error(c, http.StatusInternalServerError, "AUTHZ_ENFORCE_FAILED", "鉴权内部错误")
			c.Abort()
			return
		}

		if !ok {
			response.Error(c, http.StatusForbidden, "FORBIDDEN", "无权访问")
			c.Abort()
			return
		}

		c.Next()
	}
}

func resourceDomain(c *gin.Context) string {
	for _, name := range []string{"id", "app_id", "group_id"} {
		if value := strings.TrimSpace(c.Param(name)); value != "" {
			return value
		}
	}
	return ""
}
