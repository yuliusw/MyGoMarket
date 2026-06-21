package domain

import (
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

var (
	ErrCouponNotUnused    = errors.New("coupon: is not in unused status")
	ErrCouponExpired      = errors.New("coupon: has expired")
	ErrAppNotMatch        = errors.New("coupon: target app does not match")
	ErrMinSpendNotReached = errors.New("coupon: minimum spend requirement not met")
)

type CouponStatus string

const (
	CouponStatusUnused  CouponStatus = "unused"
	CouponStatusUsed    CouponStatus = "used"
	CouponStatusExpired CouponStatus = "expired"
)

type DiscountType string

const (
	DiscountTypeFixed   DiscountType = "fixed"
	DiscountTypePercent DiscountType = "percent"
)

// CouponTemplate 优惠券模板实体
type CouponTemplate struct {
	TemplateID    uuid.UUID
	Name          string
	DiscountType  DiscountType
	DiscountValue decimal.Decimal
	MinSpend      decimal.Decimal
	ValidDays     int
	TargetAppID   *uuid.UUID
}

// Coupon 优惠券实例
type Coupon struct {
	CouponID  uuid.UUID
	Template  CouponTemplate // 领域层推荐直接嵌套模板实体，方便计算规则
	OwnerID   uuid.UUID
	OwnerType string // "user" or "group"
	Status    CouponStatus
	ExpiredAt time.Time
	UsedAt    *time.Time
	UsedTxID  *uuid.UUID
	CreatedAt time.Time
}

// CalculateDiscount 根据订单金额和应用ID，计算实际可抵扣的金额
func (c *Coupon) CalculateDiscount(orderAmount decimal.Decimal, targetAppID uuid.UUID) (decimal.Decimal, error) {
	if c.Status != CouponStatusUnused {
		return decimal.Zero, ErrCouponNotUnused
	}
	if time.Now().After(c.ExpiredAt) {
		return decimal.Zero, ErrCouponExpired
	}
	// 校验适用应用
	if c.Template.TargetAppID != nil && *c.Template.TargetAppID != targetAppID {
		return decimal.Zero, ErrAppNotMatch
	}
	// 校验满减门槛
	if orderAmount.LessThan(c.Template.MinSpend) {
		return decimal.Zero, ErrMinSpendNotReached
	}

	// 计算折扣额度
	var discount decimal.Decimal
	if c.Template.DiscountType == DiscountTypeFixed {
		discount = c.Template.DiscountValue
	} else if c.Template.DiscountType == DiscountTypePercent {
		// orderAmount * (DiscountValue / 100)
		rate := c.Template.DiscountValue.Div(decimal.NewFromInt(100))
		discount = orderAmount.Mul(rate)
	}

	// 抵扣金额不能超过订单总金额
	if discount.GreaterThan(orderAmount) {
		return orderAmount, nil
	}
	return discount, nil
}

// MarkAsUsed 标记优惠券为已核销
func (c *Coupon) MarkAsUsed(txID uuid.UUID) error {
	if c.Status != CouponStatusUnused {
		return ErrCouponNotUnused
	}
	if time.Now().After(c.ExpiredAt) {
		return ErrCouponExpired
	}

	now := time.Now()
	c.Status = CouponStatusUsed
	c.UsedAt = &now
	c.UsedTxID = &txID
	return nil
}
