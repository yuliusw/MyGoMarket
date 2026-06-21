package grpcserver

import (
	walletv1 "github.com/yuliusw/RPA-market/gen/go/wallet/v1"
	walletapp "github.com/yuliusw/RPA-market/services/wallet/app"
	walletrepo "github.com/yuliusw/RPA-market/services/wallet/repository"
	"google.golang.org/grpc"
	"gorm.io/gorm"
)

func New(db *gorm.DB) *grpc.Server {
	server := grpc.NewServer()
	walletv1.RegisterWalletServiceServer(server, walletapp.NewWalletService(walletrepo.NewWalletRepository(db)))
	return server
}
