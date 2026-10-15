package trailing

import (
	"github.com/YuanJey/go-strike"
	"github.com/shopspring/decimal"
)

// CalculateStopPrice 无状态函数：根据持仓方向、极值价格与回调参数，计算当前移动止损/止盈线
func CalculateStopPrice(
	side strike.PositionSide,
	extremePrice decimal.Decimal,
	callbackVal decimal.Decimal,
	callbackType strike.TrailingCallbackType,
) (decimal.Decimal, error) {
	if extremePrice.LessThanOrEqual(decimal.Zero) {
		return decimal.Zero, strike.ErrInvalidPrice
	}

	var stopPrice decimal.Decimal

	switch callbackType {
	case strike.CallbackTypeRate:
		// 校验回调率: 必须在 (0, 1) 区间内，例如 0.01 代表 1%
		if callbackVal.LessThanOrEqual(decimal.Zero) || callbackVal.GreaterThanOrEqual(decimal.NewFromInt(1)) {
			return decimal.Zero, strike.ErrInvalidCallbackRate
		}
		if side == strike.PositionSideLong {
			// 多头追踪止盈：极值最高价回撤，Stop = High * (1 - CallbackRate)
			stopPrice = extremePrice.Mul(decimal.NewFromInt(1).Sub(callbackVal))
		} else {
			// 空头追踪止盈：极值最低价反弹，Stop = Low * (1 + CallbackRate)
			stopPrice = extremePrice.Mul(decimal.NewFromInt(1).Add(callbackVal))
		}

	case strike.CallbackTypeDistance:
		// 校验回调点数/价差: 必须大于 0
		if callbackVal.LessThanOrEqual(decimal.Zero) {
			return decimal.Zero, strike.ErrInvalidDistance
		}
		if side == strike.PositionSideLong {
			// 多头追踪：Stop = High - Distance
			stopPrice = extremePrice.Sub(callbackVal)
		} else {
			// 空头追踪：Stop = Low + Distance
			stopPrice = extremePrice.Add(callbackVal)
		}

	default:
		return decimal.Zero, strike.ErrInvalidCallbackRate
	}

	return stopPrice, nil
}

// IsTriggered 判定当前行情最新价格是否击穿止损线
func IsTriggered(side strike.PositionSide, stopPrice, currentPrice decimal.Decimal) bool {
	if stopPrice.IsZero() || currentPrice.IsZero() {
		return false
	}
	if side == strike.PositionSideLong {
		// 多头持仓：当价格下跌击穿或等于止损价时触发
		return currentPrice.LessThanOrEqual(stopPrice)
	}
	// 空头持仓：当价格上涨突破或等于止损价时触发
	return currentPrice.GreaterThanOrEqual(stopPrice)
}
