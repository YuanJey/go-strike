package sizing

import (
	"errors"

	"github.com/YuanJey/go-strike"
	"github.com/shopspring/decimal"
)

var (
	ErrInvalidATR = errors.New("sizing: ATR 波动率值必须大于 0")
)

// ATRPositionSizing 基于真实波幅 ATR 计算波动率风险平价开仓数量
// 原理: 市场波动越大，每单位承担风险越高，开仓数量自适应调小；市场平静时，开仓数量自适应调大。
// 数量 Quantity = (Equity * RiskRatio) / (ATR * Multiplier)
func ATRPositionSizing(
	equity decimal.Decimal,
	riskRatio decimal.Decimal,
	atr decimal.Decimal,
	multiplier decimal.Decimal,
) (decimal.Decimal, error) {
	if equity.LessThanOrEqual(decimal.Zero) {
		return decimal.Zero, strike.ErrInvalidPrice
	}
	if riskRatio.LessThanOrEqual(decimal.Zero) || riskRatio.GreaterThan(decimal.NewFromInt(1)) {
		return decimal.Zero, strike.ErrInvalidRiskRatio
	}
	if atr.LessThanOrEqual(decimal.Zero) {
		return decimal.Zero, ErrInvalidATR
	}
	if multiplier.LessThanOrEqual(decimal.Zero) {
		multiplier = decimal.NewFromInt(1)
	}

	riskBudget := equity.Mul(riskRatio)
	stopDistance := atr.Mul(multiplier)

	return riskBudget.Div(stopDistance), nil
}
