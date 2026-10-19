package indicator

import (
	"fmt"

	"github.com/shopspring/decimal"
)

var oneHundred = decimal.NewFromInt(100)

// RSI 使用 Wilder 平滑计算相对强弱指标，结果范围为 [0, 100]。
// 当窗口内既无上涨也无下跌时返回中性值 50。
func RSI(values []decimal.Decimal, period int) (Series, error) {
	if period <= 0 {
		return Series{}, ErrInvalidPeriod
	}
	required := period + 1
	if len(values) < required {
		return Series{}, fmt.Errorf(
			"%w: RSI 需要至少 %d 个数据点，当前为 %d",
			ErrInsufficientData,
			required,
			len(values),
		)
	}

	result := newSeries(len(values), period)
	gainSum := decimal.Zero
	lossSum := decimal.Zero

	for i := 1; i <= period; i++ {
		change := values[i].Sub(values[i-1])
		if change.IsPositive() {
			gainSum = gainSum.Add(change)
		} else if change.IsNegative() {
			lossSum = lossSum.Add(change.Abs())
		}
	}

	periodDecimal := decimal.NewFromInt(int64(period))
	periodMinusOne := decimal.NewFromInt(int64(period - 1))
	averageGain := gainSum.Div(periodDecimal)
	averageLoss := lossSum.Div(periodDecimal)
	result.Values[result.Start] = calculateRSI(averageGain, averageLoss)

	for i := period + 1; i < len(values); i++ {
		change := values[i].Sub(values[i-1])
		gain := decimal.Zero
		loss := decimal.Zero
		if change.IsPositive() {
			gain = change
		} else if change.IsNegative() {
			loss = change.Abs()
		}

		averageGain = averageGain.Mul(periodMinusOne).Add(gain).Div(periodDecimal)
		averageLoss = averageLoss.Mul(periodMinusOne).Add(loss).Div(periodDecimal)
		result.Values[i] = calculateRSI(averageGain, averageLoss)
	}

	return result, nil
}

func calculateRSI(averageGain, averageLoss decimal.Decimal) decimal.Decimal {
	if averageGain.IsZero() && averageLoss.IsZero() {
		return decimal.NewFromInt(50)
	}
	if averageLoss.IsZero() {
		return oneHundred
	}
	if averageGain.IsZero() {
		return decimal.Zero
	}

	relativeStrength := averageGain.Div(averageLoss)
	return oneHundred.Sub(oneHundred.Div(decimal.NewFromInt(1).Add(relativeStrength)))
}

// ROC 计算价格变化率，返回百分数；20 表示相对 period 个周期前上涨 20%。
func ROC(values []decimal.Decimal, period int) (Series, error) {
	if period <= 0 {
		return Series{}, ErrInvalidPeriod
	}
	required := period + 1
	if len(values) < required {
		return Series{}, fmt.Errorf(
			"%w: ROC 需要至少 %d 个数据点，当前为 %d",
			ErrInsufficientData,
			required,
			len(values),
		)
	}

	result := newSeries(len(values), period)
	for i := period; i < len(values); i++ {
		reference := values[i-period]
		if reference.IsZero() {
			return Series{}, fmt.Errorf("%w: index %d", ErrZeroReference, i-period)
		}
		result.Values[i] = values[i].Sub(reference).Div(reference).Mul(oneHundred)
	}

	return result, nil
}
