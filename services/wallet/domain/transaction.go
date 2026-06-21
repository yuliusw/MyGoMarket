package domain

import (
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

type TxType string

const (
	TxTypeRecharge TxType = "recharge"
	TxTypeConsume  TxType = "consume"
	TxTypeRefund   TxType = "refund"
)

// Transaction 流水实体 (通常作为不可变对象处理)
type Transaction struct {
	TxID         uuid.UUID
	WalletID     uuid.UUID
	TxType       TxType
	Amount       decimal.Decimal // 正数表示收入，负数表示支出
	BalanceAfter decimal.Decimal // 交易后快照余额，用于对账
	ReferenceID  *uuid.UUID      // 关联的业务单据号（如订单ID、订阅ID）
	Description  string
	CreatedAt    time.Time
}

// NewTransaction 工厂方法，生成一笔流水记录
// amount: 变动金额 (可正可负)
// balanceAfter: 变动后的钱包余额快照
func NewTransaction(walletID uuid.UUID, txType TxType, amount, balanceAfter decimal.Decimal, refID *uuid.UUID, desc string) *Transaction {
	return &Transaction{
		TxID:         uuid.New(),
		WalletID:     walletID,
		TxType:       txType,
		Amount:       amount,
		BalanceAfter: balanceAfter,
		ReferenceID:  refID,
		Description:  desc,
		CreatedAt:    time.Time{}, // 交由 DB Default 或者在此处显式赋值 time.Now()
	}
}
