package app

import (
	"context"
	"strings"
	"time"

	"github.com/google/uuid"
	walletv1 "github.com/yuliusw/RPA-market/gen/go/wallet/v1"
	"github.com/yuliusw/RPA-market/services/wallet/domain"
	"github.com/yuliusw/RPA-market/services/wallet/repository"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type WalletService struct {
	walletv1.UnimplementedWalletServiceServer
	repo *repository.WalletRepository
}

func NewWalletService(repo *repository.WalletRepository) *WalletService {
	return &WalletService{repo: repo}
}

func (s *WalletService) GetWallet(ctx context.Context, req *walletv1.GetWalletRequest) (*walletv1.Wallet, error) {
	ownerID, ownerType, currencyCode, err := parseOwnerWalletRequest(req.GetOwnerId(), req.GetOwnerType(), req.GetCurrencyCode())
	if err != nil {
		return nil, err
	}
	wallet, err := s.repo.FindByOwner(ctx, ownerID, ownerType, currencyCode)
	if err != nil {
		if repository.IsNotFound(err) {
			return nil, status.Error(codes.NotFound, "wallet not found")
		}
		return nil, status.Error(codes.Internal, "failed to get wallet")
	}
	return toProtoWallet(wallet), nil
}

func (s *WalletService) GetOrCreateWallet(ctx context.Context, req *walletv1.GetOrCreateWalletRequest) (*walletv1.Wallet, error) {
	ownerID, ownerType, currencyCode, err := parseOwnerWalletRequest(req.GetOwnerId(), req.GetOwnerType(), req.GetCurrencyCode())
	if err != nil {
		return nil, err
	}
	wallet, err := s.repo.GetOrCreateByOwner(ctx, ownerID, ownerType, currencyCode)
	if err != nil {
		return nil, status.Error(codes.Internal, "failed to get or create wallet")
	}
	return toProtoWallet(wallet), nil
}

func parseOwnerWalletRequest(ownerIDValue, ownerTypeValue, currencyCode string) (uuid.UUID, domain.OwnerType, string, error) {
	ownerID, err := uuid.Parse(strings.TrimSpace(ownerIDValue))
	if err != nil {
		return uuid.Nil, "", "", status.Error(codes.InvalidArgument, "invalid owner_id")
	}

	ownerType := domain.OwnerType(strings.TrimSpace(strings.ToLower(ownerTypeValue)))
	switch ownerType {
	case domain.OwnerTypeUser, domain.OwnerTypeGroup:
	default:
		return uuid.Nil, "", "", status.Error(codes.InvalidArgument, "owner_type must be user or group")
	}

	return ownerID, ownerType, currencyCode, nil
}

func toProtoWallet(wallet *domain.Wallet) *walletv1.Wallet {
	return &walletv1.Wallet{
		WalletId:     wallet.ID.String(),
		OwnerId:      wallet.OwnerID.String(),
		OwnerType:    string(wallet.OwnerType),
		Balance:      wallet.Balance.StringFixed(4),
		CurrencyCode: wallet.CurrencyCode,
		Status:       string(wallet.Status),
		UpdatedAt:    wallet.UpdatedAt.UTC().Format(time.RFC3339Nano),
	}
}
