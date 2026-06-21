package market

import (
	"github.com/gin-gonic/gin"
	"github.com/yuliusw/RPA-market/common/database"
	"github.com/yuliusw/RPA-market/common/middleware"
	"github.com/yuliusw/RPA-market/services/market/app"
	"github.com/yuliusw/RPA-market/services/market/respository"
)

func RegisterMarketHandlers(r *gin.Engine) {
	app.InitMarket(respository.NewAppRepository(database.DB), database.GlobalMinio, database.RedisClient)

	// 1. 公开接口：列表、详情、下载（无需任何凭证）
	public := r.Group("/api/v1/market", middleware.ConfiguredRateLimit())
	{
		public.GET("/apps", app.ListApps)
		public.GET("/rankings", app.GetRankings)
		public.GET("/apps/:app_id", app.GetAppDetail)
		public.GET("/apps/:app_id/download", app.DownloadApp)
	}

	// 2. 私有接口：发布、修改、下架、删除（需要 JWT 登录状态，以及特定的 Casbin 权限）
	private := r.Group("/api/v1/market", middleware.OptionalJWTAuth(), middleware.OptionalCors())
	{
		// 发布应用（全局级权限，不需要指定资源ID）
		private.POST("/apps", middleware.OptionalCasbinRequire("app:create"), app.PublishApp)

		// 修改应用信息（利用 :app_id 捕获应用ID，校验是否有该应用的编辑权限）
		private.PUT("/apps/:app_id", middleware.OptionalCasbinRequire("app:edit"), app.UpdateApp)

		// 下架应用（利用 :app_id 校验是否有该应用的下架权限）
		private.PUT("/apps/:app_id/offshelf", middleware.OptionalCasbinRequire("app:offshelf"), app.OffShelfApp)

		// 删除应用及其文件（利用 :app_id 校验是否有该应用的删除权限）
		private.DELETE("/apps/:app_id", middleware.OptionalCasbinRequire("app:delete"), app.DeleteApp)
	}
}
