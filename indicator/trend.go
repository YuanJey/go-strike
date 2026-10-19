package indicator

import (
	"fmt"

	"github.com/shopspring/decimal"
)

// MACDResult 保存 MACD 线、信号线和柱状图序列。
type MACDResult struct {
	Line      Series
	Signal    Series
	Histogram Series
}

// MACD 计算指数平滑异同移动平均线。
// 常用周期为 fast=12、slow=26、signal=9。
func MACD(values []decimal.Decimal, fast, slow, signal int) (MACDResult, error) {
	if fast <= 0 || slow <= 0 || signal <= 0 || fast >= slow {
		return MACDResult{}, ErrInvalidMACDPeriods
	}

	required := slow + signal - 1
	if len(values) < required {
		return MACDResult{}, fmt.Errorf(
			"%w: MACD 需要至少 %d 个数据点，当前为 %d",
			ErrInsufficientData,
			required,
			len(values),
		)
	}

	fastEMA, err := EMA(values, fast)
	if err != nil {
		return MACDResult{}, err
	}
	slowEMA, err := EMA(values, slow)
	if err != nil {
		return MACDResult{}, err
	}

	line := newSeries(len(values), slow-1)
	validLineValues := make([]decimal.Decimal, len(values)-line.Start)
	for i := line.Start; i < len(values); i++ {
		line.Values[i] = fastEMA.Values[i].Sub(slowEMA.Values[i])
		validLineValues[i-line.Start] = line.Values[i]
	}

	compactSignal, err := EMA(validLineValues, signal)
	if err != nil {
		return MACDResult{}, err
	}
	signalStart := line.Start + compactSignal.Start
	signalLine := newSeries(len(values), signalStart)
	histogram := newSeries(len(values), signalStart)

	for i := signalStart; i < len(values); i++ {
		compactIndex := i - line.Start
		signalLine.Values[i] = compactSignal.Values[compactIndex]
		histogram.Values[i] = line.Values[i].Sub(signalLine.Values[i])
	}

	return MACDResult{
		Line:      line,
		Signal:    signalLine,
		Histogram: histogram,
	}, nil
}

// DonchianChannelResult 保存唐奇安通道的上轨、中轨和下轨。
type DonchianChannelResult struct {
	Upper  Series
	Middle Series
	Lower  Series
}

// DonchianChannel 按滚动窗口的最高价和最低价计算唐奇安通道。
func DonchianChannel(candles []Candle, period int) (DonchianChannelResult, error) {
	if err := validateCandles(candles); err != nil {
		return DonchianChannelResult{}, err
	}
	if err := validatePeriod(len(candles), period); err != nil {
		return DonchianChannelResult{}, err
	}

	start := period - 1
	upper := newSeries(len(candles), start)
	middle := newSeries(len(candles), start)
	lower := newSeries(len(candles), start)
	two := decimal.NewFromInt(2)
	highestIndices := make([]int, 0, period)
	lowestIndices := make([]int, 0, period)

	for i, candle := range candles {
		expiredIndex := i - period
		if len(highestIndices) > 0 && highestIndices[0] <= expiredIndex {
			highestIndices = highestIndices[1:]
		}
		if len(lowestIndices) > 0 && lowestIndices[0] <= expiredIndex {
			lowestIndices = lowestIndices[1:]
		}

		for len(highestIndices) > 0 &&
			candles[highestIndices[len(highestIndices)-1]].High.LessThanOrEqual(candle.High) {
			highestIndices = highestIndices[:len(highestIndices)-1]
		}
		highestIndices = append(highestIndices, i)

		for len(lowestIndices) > 0 &&
			candles[lowestIndices[len(lowestIndices)-1]].Low.GreaterThanOrEqual(candle.Low) {
			lowestIndices = lowestIndices[:len(lowestIndices)-1]
		}
		lowestIndices = append(lowestIndices, i)

		if i < start {
			continue
		}

		highest := candles[highestIndices[0]].High
		lowest := candles[lowestIndices[0]].Low
		upper.Values[i] = highest
		lower.Values[i] = lowest
		middle.Values[i] = highest.Add(lowest).Div(two)
	}

	return DonchianChannelResult{
		Upper:  upper,
		Middle: middle,
		Lower:  lower,
	}, nil
}

// SupertrendDirection 表示超级趋势当前方向。
type SupertrendDirection int8

const (
	SupertrendDirectionUnknown SupertrendDirection = 0
	SupertrendDirectionUp      SupertrendDirection = 1
	SupertrendDirectionDown    SupertrendDirection = -1
)

// SupertrendResult 保存超级趋势线、最终上下轨和趋势方向。
type SupertrendResult struct {
	Value     Series
	UpperBand Series
	LowerBand Series
	Direction []SupertrendDirection
}

// Supertrend 使用 Wilder ATR 计算超级趋势指标。
//
// 首个 ATR 有效点初始化为下降趋势并采用上轨作为趋势线。后续下降趋势在
// Close > UpperBand 时切换为上升趋势，上升趋势在 Close < LowerBand 时
// 切换为下降趋势。
func Supertrend(
	candles []Candle,
	period int,
	multiplier decimal.Decimal,
) (SupertrendResult, error) {
	if multiplier.LessThanOrEqual(decimal.Zero) {
		return SupertrendResult{}, fmt.Errorf("%w: Supertrend 倍数必须大于 0", ErrInvalidMultiplier)
	}

	atr, err := ATR(candles, period)
	if err != nil {
		return SupertrendResult{}, err
	}

	start := atr.Start
	value := newSeries(len(candles), start)
	upperBand := newSeries(len(candles), start)
	lowerBand := newSeries(len(candles), start)
	direction := make([]SupertrendDirection, len(candles))
	two := decimal.NewFromInt(2)

	for i := start; i < len(candles); i++ {
		midpoint := candles[i].High.Add(candles[i].Low).Div(two)
		offset := atr.Values[i].Mul(multiplier)
		basicUpperBand := midpoint.Add(offset)
		basicLowerBand := midpoint.Sub(offset)

		if i == start {
			upperBand.Values[i] = basicUpperBand
			lowerBand.Values[i] = basicLowerBand
			direction[i] = SupertrendDirectionDown
			value.Values[i] = basicUpperBand
			continue
		}

		previousUpperBand := upperBand.Values[i-1]
		previousLowerBand := lowerBand.Values[i-1]
		previousClose := candles[i-1].Close

		if basicUpperBand.LessThan(previousUpperBand) ||
			previousClose.GreaterThan(previousUpperBand) {
			upperBand.Values[i] = basicUpperBand
		} else {
			upperBand.Values[i] = previousUpperBand
		}

		if basicLowerBand.GreaterThan(previousLowerBand) ||
			previousClose.LessThan(previousLowerBand) {
			lowerBand.Values[i] = basicLowerBand
		} else {
			lowerBand.Values[i] = previousLowerBand
		}

		direction[i] = direction[i-1]
		if direction[i-1] == SupertrendDirectionDown &&
			candles[i].Close.GreaterThan(upperBand.Values[i]) {
			direction[i] = SupertrendDirectionUp
		} else if direction[i-1] == SupertrendDirectionUp &&
			candles[i].Close.LessThan(lowerBand.Values[i]) {
			direction[i] = SupertrendDirectionDown
		}

		if direction[i] == SupertrendDirectionUp {
			value.Values[i] = lowerBand.Values[i]
		} else {
			value.Values[i] = upperBand.Values[i]
		}
	}

	return SupertrendResult{
		Value:     value,
		UpperBand: upperBand,
		LowerBand: lowerBand,
		Direction: direction,
	}, nil
}
