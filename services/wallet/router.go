package wallet

import (
	"github.com/gin-gonic/gin"
	"github.com/yuliusw/RPA-market/common/database"
	"github.com/yuliusw/RPA-market/common/middleware"
	"github.com/yuliusw/RPA-market/services/wallet/app"
	"github.com/yuliusw/RPA-market/services/wallet/repository"
)

func RegisterWalletHandlers(r *gin.Engine) {
	service := app.NewWalletHTTPService(repository.NewWalletRepository(database.DB))

	private := r.Group("/api/v1/wallets", middleware.OptionalJWTAuth(), middleware.OptionalCors())
	{
		private.GET("/me", service.GetMyWallet)
		private.GET("/:wallet_id/transactions", service.ListTransactions)
		private.POST("/:wallet_id/credit", service.Credit)
		private.POST("/:wallet_id/debit", service.Debit)
		private.POST("/transfer", service.Transfer)
	}
}
