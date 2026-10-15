package precision

import (
	"errors"

	"github.com/YuanJey/go-strike"
	"github.com/shopspring/decimal"
)

var (
	ErrZeroStep = errors.New("precision: 步长 (step/tick size) 必须大于 0")
)

// AlignTickSize 将价格依照交易所规定的 tickSize 步长进行精准对齐
func AlignTickSize(price, tickSize decimal.Decimal, mode strike.RoundMode) (decimal.Decimal, error) {
	if tickSize.LessThanOrEqual(decimal.Zero) {
		return decimal.Zero, ErrZeroStep
	}
	if price.IsZero() {
		return decimal.Zero, nil
	}

	steps := price.Div(tickSize)
	var alignedSteps decimal.Decimal

	switch mode {
	case strike.RoundDown:
		alignedSteps = steps.Floor()
	case strike.RoundNearest:
		alignedSteps = steps.Round(0)
	case strike.RoundHalfUp:
		// 银行家舍入法 (Round to nearest even)
		alignedSteps = steps.RoundBank(0)
	default:
		alignedSteps = steps.Floor()
	}

	return alignedSteps.Mul(tickSize), nil
}

// AlignStepSize 将下单数量依照交易所规定的 stepSize (即 LotSize) 步长进行精准对齐
// 在订单风控中，数量通常推荐使用 RoundDown，避免对齐后超出可用保证金或账户余额
func AlignStepSize(quantity, stepSize decimal.Decimal, mode strike.RoundMode) (decimal.Decimal, error) {
	if stepSize.LessThanOrEqual(decimal.Zero) {
		return decimal.Zero, ErrZeroStep
	}
	if quantity.IsZero() {
		return decimal.Zero, nil
	}

	steps := quantity.Div(stepSize)
	var alignedSteps decimal.Decimal

	switch mode {
	case strike.RoundDown:
		alignedSteps = steps.Floor()
	case strike.RoundNearest:
		alignedSteps = steps.Round(0)
	case strike.RoundHalfUp:
		alignedSteps = steps.RoundBank(0)
	default:
		alignedSteps = steps.Floor()
	}

	return alignedSteps.Mul(stepSize), nil
}

// CheckMinNotional 校验委托订单的名义价值 (Price * Quantity) 是否满足最小限制
func CheckMinNotional(price, quantity, minNotional decimal.Decimal) bool {
	if price.IsNegative() || quantity.IsNegative() {
		return false
	}
	notional := price.Mul(quantity)
	return notional.GreaterThanOrEqual(minNotional)
}

// CalculateNotional 计算名义价值
func CalculateNotional(price, quantity decimal.Decimal) decimal.Decimal {
	return price.Mul(quantity)
}
