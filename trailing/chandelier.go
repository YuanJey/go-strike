package trailing

import (
	"errors"

	"github.com/YuanJey/go-strike"
	"github.com/shopspring/decimal"
)

var (
	ErrInsufficientBars = errors.New("trailing: K线数据量少于设定的 ATR 周期")
)

// Bar 表示单根 K 线的价格要素
type Bar struct {
	High  decimal.Decimal
	Low   decimal.Decimal
	Close decimal.Decimal
}

// TrueRange 计算单根 K 线的真实波幅 (True Range)
// TR = max(High - Low, |High - PrevClose|, |Low - PrevClose|)
func TrueRange(high, low, prevClose decimal.Decimal) decimal.Decimal {
	hl := high.Sub(low)
	if prevClose.IsZero() {
		return hl
	}

	hpc := high.Sub(prevClose).Abs()
	lpc := low.Sub(prevClose).Abs()

	maxTR := hl
	if hpc.GreaterThan(maxTR) {
		maxTR = hpc
	}
	if lpc.GreaterThan(maxTR) {
		maxTR = lpc
	}
	return maxTR
}

// CalculateATR 计算指定周期 (period) 的平均真实波幅 (ATR - RMA/SMA 模式)
func CalculateATR(bars []Bar, period int) (decimal.Decimal, error) {
	if period <= 0 || len(bars) < period {
		return decimal.Zero, ErrInsufficientBars
	}

	// 1. 计算初始 SMA 作为基准
	sumTR := decimal.Zero
	prevClose := decimal.Zero
	trValues := make([]decimal.Decimal, len(bars))

	for i, bar := range bars {
		tr := TrueRange(bar.High, bar.Low, prevClose)
		trValues[i] = tr
		prevClose = bar.Close
	}

	// 初始前 period 个 TR 的均值
	for i := 0; i < period; i++ {
		sumTR = sumTR.Add(trValues[i])
	}
	atr := sumTR.Div(decimal.NewFromInt(int64(period)))

	// Wilder's 平滑移动平均: ATR_t = (ATR_{t-1} * (period - 1) + TR_t) / period
	periodDec := decimal.NewFromInt(int64(period))
	periodMinusOne := decimal.NewFromInt(int64(period - 1))

	for i := period; i < len(bars); i++ {
		atr = atr.Mul(periodMinusOne).Add(trValues[i]).Div(periodDec)
	}

	return atr, nil
}

// ChandelierStop 计算吊灯移动止盈止损线 (Chandelier Exit)
// 多头: 阶段最高点 - ATR * Multiplier
// 空头: 阶段最低点 + ATR * Multiplier
func ChandelierStop(
	side strike.PositionSide,
	extremePrice decimal.Decimal,
	atr decimal.Decimal,
	multiplier decimal.Decimal,
) (decimal.Decimal, error) {
	if extremePrice.LessThanOrEqual(decimal.Zero) {
		return decimal.Zero, strike.ErrInvalidPrice
	}
	if atr.LessThanOrEqual(decimal.Zero) || multiplier.LessThanOrEqual(decimal.Zero) {
		return decimal.Zero, strike.ErrInvalidDistance
	}

	offset := atr.Mul(multiplier)

	if side == strike.PositionSideLong {
		return extremePrice.Sub(offset), nil
	}
	return extremePrice.Add(offset), nil
}
