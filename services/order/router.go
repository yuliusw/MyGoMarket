package order

import (
	"github.com/gin-gonic/gin"
	"github.com/yuliusw/RPA-market/common/database"
	"github.com/yuliusw/RPA-market/common/middleware"
	"github.com/yuliusw/RPA-market/services/order/app"
	"github.com/yuliusw/RPA-market/services/order/repository"
	walletrepo "github.com/yuliusw/RPA-market/services/wallet/repository"
)

func RegisterOrderHandlers(r *gin.Engine) {
	walletRepository := walletrepo.NewWalletRepository(database.DB)
	service := app.NewOrderHTTPService(repository.NewOrderRepository(database.DB, walletRepository))

	private := r.Group("/api/v1/orders", middleware.OptionalJWTAuth(), middleware.OptionalCors())
	{
		private.POST("/purchase", service.PurchaseApp)
		private.POST("", service.CreateOrder)
		private.GET("", service.ListOrders)
		private.GET("/:order_id", service.GetOrder)
		private.POST("/:order_id/pay", service.PayOrder)
		private.POST("/:order_id/cancel", service.CancelOrder)
	}
}
