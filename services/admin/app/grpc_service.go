package app

import (
	"context"
	"strings"
	"time"

	"github.com/google/uuid"
	adminv1 "github.com/yuliusw/RPA-market/gen/go/admin/v1"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"gorm.io/gorm"
)

type AdminQueryService struct {
	adminv1.UnimplementedAdminQueryServiceServer
	db *gorm.DB
}

func NewAdminQueryService(db *gorm.DB) *AdminQueryService {
	return &AdminQueryService{db: db}
}

func (s *AdminQueryService) ListVirtualOrders(ctx context.Context, req *adminv1.ListVirtualOrdersRequest) (*adminv1.ListVirtualOrdersResponse, error) {
	page, pageSize, offset := protoPagination(req.GetPage(), req.GetPageSize())
	query := s.db.WithContext(ctx).Model(&walletModel{})
	var err error
	if query, err = protoWhereUUID(query, "owner_id", req.GetOwnerId()); err != nil {
		return nil, err
	}
	query = protoWhereEqual(query, "owner_type", req.GetOwnerType())
	query = protoWhereEqual(query, "currency_code", req.GetCurrencyCode())
	query = protoWhereEqual(query, "status", req.GetStatus())

	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, status.Error(codes.Internal, "failed to count virtual orders")
	}
	var rows []walletModel
	if err := query.Order("updated_at DESC").Limit(pageSize).Offset(offset).Find(&rows).Error; err != nil {
		return nil, status.Error(codes.Internal, "failed to list virtual orders")
	}
	items := make([]*adminv1.VirtualOrder, 0, len(rows))
	for _, row := range rows {
		items = append(items, toProtoVirtualOrder(row))
	}
	return &adminv1.ListVirtualOrdersResponse{Items: items, Total: total, Page: int32(page), PageSize: int32(pageSize)}, nil
}

func (s *AdminQueryService) ListWalletTransactions(ctx context.Context, req *adminv1.ListWalletTransactionsRequest) (*adminv1.ListWalletTransactionsResponse, error) {
	page, pageSize, offset := protoPagination(req.GetPage(), req.GetPageSize())
	query := s.db.WithContext(ctx).Model(&transactionModel{})
	var err error
	if query, err = protoWhereUUID(query, "wallet_id", req.GetWalletId()); err != nil {
		return nil, err
	}
	if query, err = protoWhereUUID(query, "reference_id", req.GetReferenceId()); err != nil {
		return nil, err
	}
	query = protoWhereEqual(query, "tx_type", req.GetTxType())
	if query, err = protoWhereTimeRange(query, "created_at", req.GetFrom(), req.GetTo()); err != nil {
		return nil, err
	}

	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, status.Error(codes.Internal, "failed to count wallet transactions")
	}
	var rows []transactionModel
	if err := query.Order("created_at DESC").Limit(pageSize).Offset(offset).Find(&rows).Error; err != nil {
		return nil, status.Error(codes.Internal, "failed to list wallet transactions")
	}
	items := make([]*adminv1.WalletTransaction, 0, len(rows))
	for _, row := range rows {
		items = append(items, toProtoWalletTransaction(row))
	}
	return &adminv1.ListWalletTransactionsResponse{Items: items, Total: total, Page: int32(page), PageSize: int32(pageSize)}, nil
}

func (s *AdminQueryService) ListOrders(ctx context.Context, req *adminv1.ListOrdersRequest) (*adminv1.ListOrdersResponse, error) {
	page, pageSize, offset := protoPagination(req.GetPage(), req.GetPageSize())
	query := s.db.WithContext(ctx).Model(&orderModel{})
	var err error
	if query, err = protoWhereUUID(query, "user_id", req.GetUserId()); err != nil {
		return nil, err
	}
	if query, err = protoWhereUUID(query, "app_id", req.GetAppId()); err != nil {
		return nil, err
	}
	if query, err = protoWhereUUID(query, "wallet_id", req.GetWalletId()); err != nil {
		return nil, err
	}
	query = protoWhereEqual(query, "status", req.GetStatus())
	query = protoWhereEqual(query, "currency_code", req.GetCurrencyCode())
	if query, err = protoWhereTimeRange(query, "created_at", req.GetFrom(), req.GetTo()); err != nil {
		return nil, err
	}

	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, status.Error(codes.Internal, "failed to count orders")
	}
	var rows []orderModel
	if err := query.Order("created_at DESC").Limit(pageSize).Offset(offset).Find(&rows).Error; err != nil {
		return nil, status.Error(codes.Internal, "failed to list orders")
	}
	items := make([]*adminv1.Order, 0, len(rows))
	for _, row := range rows {
		items = append(items, toProtoOrder(row))
	}
	return &adminv1.ListOrdersResponse{Items: items, Total: total, Page: int32(page), PageSize: int32(pageSize)}, nil
}

func (s *AdminQueryService) ListChangeLogs(ctx context.Context, req *adminv1.ListChangeLogsRequest) (*adminv1.ListChangeLogsResponse, error) {
	page, pageSize, offset := protoPagination(req.GetPage(), req.GetPageSize())
	query := s.db.WithContext(ctx).Model(&auditEventModel{})
	query = protoWhereEqual(query, "event_type", req.GetEventType())
	query = protoWhereEqual(query, "trace_id", req.GetTraceId())
	query = protoWhereEqual(query, "actor_id", req.GetActorId())
	query = protoWhereEqual(query, "resource", req.GetResource())
	var err error
	if query, err = protoWhereTimeRange(query, "created_at", req.GetFrom(), req.GetTo()); err != nil {
		return nil, err
	}

	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, status.Error(codes.Internal, "failed to count change logs")
	}
	var rows []auditEventModel
	if err := query.Order("created_at DESC").Limit(pageSize).Offset(offset).Find(&rows).Error; err != nil {
		return nil, status.Error(codes.Internal, "failed to list change logs")
	}
	items := make([]*adminv1.ChangeLog, 0, len(rows))
	for _, row := range rows {
		items = append(items, toProtoChangeLog(row))
	}
	return &adminv1.ListChangeLogsResponse{Items: items, Total: total, Page: int32(page), PageSize: int32(pageSize)}, nil
}

func protoPagination(pageValue, pageSizeValue int32) (int, int, int) {
	page := int(pageValue)
	pageSize := int(pageSizeValue)
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 20
	}
	if pageSize > 100 {
		pageSize = 100
	}
	return page, pageSize, (page - 1) * pageSize
}

func protoWhereEqual(query *gorm.DB, column, value string) *gorm.DB {
	value = strings.TrimSpace(value)
	if value == "" {
		return query
	}
	return query.Where(column+" = ?", value)
}

func protoWhereUUID(query *gorm.DB, column, value string) (*gorm.DB, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return query, nil
	}
	if _, err := uuid.Parse(value); err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid %s", column)
	}
	return query.Where(column+" = ?", value), nil
}

func protoWhereTimeRange(query *gorm.DB, column, fromValue, toValue string) (*gorm.DB, error) {
	from, ok, err := parseProtoTime(fromValue)
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "invalid from")
	}
	if ok {
		query = query.Where(column+" >= ?", from)
	}
	to, ok, err := parseProtoTime(toValue)
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "invalid to")
	}
	if ok {
		query = query.Where(column+" <= ?", to)
	}
	return query, nil
}

func parseProtoTime(value string) (time.Time, bool, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return time.Time{}, false, nil
	}
	if parsed, err := time.Parse(time.RFC3339Nano, value); err == nil {
		return parsed, true, nil
	}
	parsed, err := time.Parse("2006-01-02", value)
	return parsed, err == nil, err
}

func toProtoVirtualOrder(row walletModel) *adminv1.VirtualOrder {
	return &adminv1.VirtualOrder{WalletId: row.WalletID.String(), OwnerId: row.OwnerID.String(), OwnerType: row.OwnerType, Balance: row.Balance.StringFixed(4), CurrencyCode: row.CurrencyCode, Status: row.Status, UpdatedAt: formatTime(row.UpdatedAt)}
}

func toProtoWalletTransaction(row transactionModel) *adminv1.WalletTransaction {
	return &adminv1.WalletTransaction{TxId: row.TxID.String(), WalletId: row.WalletID.String(), TxType: row.TxType, Amount: row.Amount.StringFixed(4), BalanceAfter: row.BalanceAfter.StringFixed(4), ReferenceId: uuidPtrString(row.ReferenceID), IdempotencyKey: stringPtrValue(row.IdempotencyKey), Description: row.Description, CreatedAt: formatTime(row.CreatedAt)}
}

func toProtoOrder(row orderModel) *adminv1.Order {
	return &adminv1.Order{OrderId: row.OrderID.String(), UserId: row.UserID.String(), AppId: row.AppID.String(), WalletId: row.WalletID.String(), Amount: row.Amount.StringFixed(4), CurrencyCode: row.CurrencyCode, Status: row.Status, TxId: uuidPtrString(row.TxID), SubscriptionId: uuidPtrString(row.SubscriptionID), IdempotencyKey: stringPtrValue(row.IdempotencyKey), Description: row.Description, CreatedAt: formatTime(row.CreatedAt), PaidAt: timePtrString(row.PaidAt), UpdatedAt: formatTime(row.UpdatedAt)}
}

func toProtoChangeLog(row auditEventModel) *adminv1.ChangeLog {
	return &adminv1.ChangeLog{EventId: row.EventID.String(), EventType: row.EventType, TraceId: row.TraceID, ActorId: row.ActorID, Resource: row.Resource, Metadata: string(row.Metadata), Error: row.Error, CreatedAt: formatTime(row.CreatedAt)}
}
