package iam

import (
	"github.com/gin-gonic/gin"
	"github.com/yuliusw/RPA-market/common/middleware"
	"github.com/yuliusw/RPA-market/common/utils/logs"
	"github.com/yuliusw/RPA-market/services/iam/app"
	"go.uber.org/zap"
)

func initLogger() {
	// 实例化你的 zap logger 并赋值给 logs.Log
	logger, _ := zap.NewProduction()
	logs.Log = logger
}

func RegisterHandlers(r *gin.Engine) {
	// 1. 公开路由 (无需任何凭证)
	public := r.Group("/api/v1/iam", middleware.ConfiguredRateLimit())
	{
		public.POST("/register", app.Register)
		public.POST("/login", app.Login)
	}

	// 2. 私有路由 (挂载 JWT 中间件，保证这些接口必须登录)
	private := r.Group("/api/v1/iam", middleware.OptionalJWTAuth(), middleware.OptionalCors())
	{
		// -------------------------------------------------------------
		// A. 基础登录操作，不需要特定 Casbin 权限，只需 JWT 即可
		// -------------------------------------------------------------
		private.POST("/groups", app.RegisterGroup)        // 假设所有登录用户都能创建团体
		private.GET("/groups", app.GetMyGroups)           // 查自己的团体，属于数据过滤，不走 API 鉴权
		private.POST("/groups/:id/leave", app.LeaveGroup) // 主动退群，自己决定即可

		// ==========================================
		// 用户个人中心路由 (新增)
		// ==========================================
		private.GET("/profile", app.GetProfile)              // 获取当前登录用户信息
		private.PUT("/profile", app.UpdateProfile)           // 更新基础信息 (用户名/邮箱)
		private.POST("/profile/avatar", app.UploadAvatar)    // 上传并更新头像
		private.PUT("/profile/password", app.UpdatePassword) // 修改密码
		// private.POST("/logout", app.Logout)               // 登出 (视需求添加)

		// ... 原有的 Group 路由保持不变 ...
		// -------------------------------------------------------------
		// B. 需要 Casbin 校验的群组操作
		// 注意：这里的 ":id" 会被你的中间件 c.Param("id") 精准捕获作为 groupID
		// -------------------------------------------------------------

		// 只有在群组内拥有 'group:view' 权限的角色才能查看群详情（防止非群成员偷窥）
		private.GET("/groups/:id", middleware.OptionalCasbinRequire("group:view"), app.GetGroupDetail)

		// 只有拥有 'group:edit' 权限的角色才能修改群信息
		private.PUT("/groups/:id", middleware.OptionalCasbinRequire("group:edit"), app.UpdateGroup)

		// 只有拥有 'group:delete' 权限的角色（如 owner）才能解散群
		private.DELETE("/groups/:id", middleware.OptionalCasbinRequire("group:delete"), app.DissolveGroup)

		// -------------------------------------------------------------
		// C. 需要 Casbin 校验的成员管理操作
		// -------------------------------------------------------------

		// 只有拥有 'group:invite' 权限的角色才能拉人进群
		private.POST("/groups/:id/members", middleware.OptionalCasbinRequire("group:invite"), app.InviteMember)

		// 只有拥有 'group:kick' 权限的角色才能踢人
		private.DELETE("/groups/:id/members/:user_id", middleware.OptionalCasbinRequire("group:kick"), app.KickMember)

		// 只有拥有 'group:edit' 权限的角色才能调整成员角色
		private.PUT("/groups/:id/members/:user_id/role", middleware.OptionalCasbinRequire("group:edit"), app.UpdateMemberRole)

		// D. 全局角色/权限管理，需要系统全局域的 role:manage 权限
		private.GET("/roles", middleware.OptionalCasbinRequire("role:manage"), app.ListRoles)
		private.GET("/roles/:role_id", middleware.OptionalCasbinRequire("role:manage"), app.GetRole)
		private.PUT("/roles/:role_id/permissions", middleware.OptionalCasbinRequire("role:manage"), app.ReplaceRolePermissions)
		private.GET("/permissions", middleware.OptionalCasbinRequire("role:manage"), app.ListPermissions)
	}
}
