package indicator

import (
	"fmt"

	"github.com/shopspring/decimal"
)

const standardDeviationPrecision int32 = 16

var squareRootExponent = decimal.New(5, -1)

// TrueRange 计算每根 K 线的真实波幅。
// 第一根 K 线使用 High-Low，后续取 High-Low、|High-PrevClose|、|Low-PrevClose| 的最大值。
func TrueRange(candles []Candle) (Series, error) {
	if err := validateCandles(candles); err != nil {
		return Series{}, err
	}

	result := newSeries(len(candles), 0)
	for i, candle := range candles {
		value := candle.High.Sub(candle.Low)
		if i > 0 {
			highGap := candle.High.Sub(candles[i-1].Close).Abs()
			lowGap := candle.Low.Sub(candles[i-1].Close).Abs()
			if highGap.GreaterThan(value) {
				value = highGap
			}
			if lowGap.GreaterThan(value) {
				value = lowGap
			}
		}
		result.Values[i] = value
	}

	return result, nil
}

// ATR 使用 Wilder RMA 计算平均真实波幅。
func ATR(candles []Candle, period int) (Series, error) {
	trueRange, err := TrueRange(candles)
	if err != nil {
		return Series{}, err
	}
	return RMA(trueRange.Values, period)
}

// StandardDeviation 计算滚动总体标准差，分母为 period。
func StandardDeviation(values []decimal.Decimal, period int) (Series, error) {
	if err := validatePeriod(len(values), period); err != nil {
		return Series{}, err
	}

	result := newSeries(len(values), period-1)
	periodDecimal := decimal.NewFromInt(int64(period))
	periodSquared := periodDecimal.Mul(periodDecimal)
	windowSum := decimal.Zero
	windowSquaredSum := decimal.Zero

	for i := 0; i < period; i++ {
		windowSum = windowSum.Add(values[i])
		windowSquaredSum = windowSquaredSum.Add(values[i].Mul(values[i]))
	}

	for i := result.Start; i < len(values); i++ {
		if i > result.Start {
			outgoing := values[i-period]
			incoming := values[i]
			windowSum = windowSum.Sub(outgoing).Add(incoming)
			windowSquaredSum = windowSquaredSum.
				Sub(outgoing.Mul(outgoing)).
				Add(incoming.Mul(incoming))
		}

		// Var(X) = (N*sum(x^2)-sum(x)^2)/N^2，先完成精确整数运算再除法。
		varianceNumerator := periodDecimal.
			Mul(windowSquaredSum).
			Sub(windowSum.Mul(windowSum))
		variance := varianceNumerator.Div(periodSquared)
		standardDeviation, err := variance.PowWithPrecision(squareRootExponent, standardDeviationPrecision)
		if err != nil {
			return Series{}, fmt.Errorf("indicator: 标准差开方失败: %w", err)
		}
		result.Values[i] = standardDeviation
	}

	return result, nil
}

// BollingerBandsResult 保存布林带上轨、中轨和下轨。
type BollingerBandsResult struct {
	Upper  Series
	Middle Series
	Lower  Series
}

// BollingerBands 计算布林带。
// Middle 为 SMA，Upper/Lower 为 Middle +/- multiplier*总体标准差。
func BollingerBands(
	values []decimal.Decimal,
	period int,
	multiplier decimal.Decimal,
) (BollingerBandsResult, error) {
	if multiplier.IsNegative() {
		return BollingerBandsResult{}, ErrInvalidMultiplier
	}

	middle, err := SMA(values, period)
	if err != nil {
		return BollingerBandsResult{}, err
	}
	deviation, err := StandardDeviation(values, period)
	if err != nil {
		return BollingerBandsResult{}, err
	}

	upper := newSeries(len(values), middle.Start)
	lower := newSeries(len(values), middle.Start)
	for i := middle.Start; i < len(values); i++ {
		offset := deviation.Values[i].Mul(multiplier)
		upper.Values[i] = middle.Values[i].Add(offset)
		lower.Values[i] = middle.Values[i].Sub(offset)
	}

	return BollingerBandsResult{
		Upper:  upper,
		Middle: middle,
		Lower:  lower,
	}, nil
}
