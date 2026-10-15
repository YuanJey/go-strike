package sizing

import (
	"errors"

	"github.com/YuanJey/go-strike"
	"github.com/shopspring/decimal"
)

var (
	ErrStopLossPriceEqualEntry = errors.New("sizing: 止损价不能等于入场价")
	ErrLeverageExceeded        = errors.New("sizing: 计算仓位名义价值超出最大允许杠杆")
)

// RiskFractionalResult 固定风险模型计算结果
type RiskFractionalResult struct {
	Quantity    decimal.Decimal // 推荐开仓数量
	RiskAmount  decimal.Decimal // 预估最大承担风险金额 (Equity * RiskRatio)
	Notional    decimal.Decimal // 对应开仓的名义价值 (Quantity * EntryPrice)
	LeverageReq decimal.Decimal // 名义价值对应的实际杠杆倍数 (Notional / Equity)
}

// FixedFractionalSizing 基于账户净值固定风险比例 (Fixed Fractional Risk) 计算开仓数量
// 公式: Quantity = (Equity * RiskRatio) / |EntryPrice - StopLossPrice|
func FixedFractionalSizing(
	side strike.PositionSide,
	equity decimal.Decimal,
	riskRatio decimal.Decimal,
	entryPrice decimal.Decimal,
	stopLossPrice decimal.Decimal,
	maxLeverage decimal.Decimal,
) (RiskFractionalResult, error) {
	if equity.LessThanOrEqual(decimal.Zero) {
		return RiskFractionalResult{}, strike.ErrInvalidPrice
	}
	if riskRatio.LessThanOrEqual(decimal.Zero) || riskRatio.GreaterThan(decimal.NewFromInt(1)) {
		return RiskFractionalResult{}, strike.ErrInvalidRiskRatio
	}
	if entryPrice.LessThanOrEqual(decimal.Zero) || stopLossPrice.LessThanOrEqual(decimal.Zero) {
		return RiskFractionalResult{}, strike.ErrInvalidPrice
	}

	// 校验止损方向合规性
	if entryPrice.Equal(stopLossPrice) {
		return RiskFractionalResult{}, ErrStopLossPriceEqualEntry
	}

	if side == strike.PositionSideLong {
		if stopLossPrice.GreaterThan(entryPrice) {
			return RiskFractionalResult{}, strike.ErrorWrap(strike.ErrInvalidStopLossPrice, "多头止损价必须低于入场价")
		}
	} else if side == strike.PositionSideShort {
		if stopLossPrice.LessThan(entryPrice) {
			return RiskFractionalResult{}, strike.ErrorWrap(strike.ErrInvalidStopLossPrice, "空头止损价必须高于入场价")
		}
	}

	// 单币单点风险 = |EntryPrice - StopLossPrice|
	priceRisk := entryPrice.Sub(stopLossPrice).Abs()

	// 账户允许最大损失金额
	riskAmount := equity.Mul(riskRatio)

	// 开仓数量 = 最大允许损失 / 单币风险
	quantity := riskAmount.Div(priceRisk)

	notional := quantity.Mul(entryPrice)
	actualLeverage := notional.Div(equity)

	// 校验最大杠杆上限保护
	if !maxLeverage.IsZero() && actualLeverage.GreaterThan(maxLeverage) {
		return RiskFractionalResult{}, strike.ErrorWrap(ErrLeverageExceeded, actualLeverage.String())
	}

	return RiskFractionalResult{
		Quantity:    quantity,
		RiskAmount:  riskAmount,
		Notional:    notional,
		LeverageReq: actualLeverage,
	}, nil
}
