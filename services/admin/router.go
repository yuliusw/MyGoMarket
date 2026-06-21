package admin

import (
	"github.com/gin-gonic/gin"
	"github.com/yuliusw/RPA-market/common/audit"
	"github.com/yuliusw/RPA-market/common/database"
	"github.com/yuliusw/RPA-market/common/middleware"
	"github.com/yuliusw/RPA-market/services/admin/app"
)

func RegisterAdminHandlers(r *gin.Engine) {
	service := app.NewAdminHTTPService(database.DB)

	private := r.Group("/api/v1/admin", middleware.OptionalJWTAuth(), middleware.OptionalCors(), middleware.OptionalCasbinRequire("role:manage"))
	{
		private.GET("/virtual-orders", service.ListVirtualOrders)
		private.GET("/wallet-transactions", service.ListWalletTransactions)
		private.GET("/orders", service.ListOrders)
		private.GET("/change-logs", service.ListChangeLogs)
		private.GET("/change-logs/export", audit.ExportCSVHandler(database.DB))
	}
}
