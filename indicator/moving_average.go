package indicator

import "github.com/shopspring/decimal"

// SMA 计算简单移动平均线。
func SMA(values []decimal.Decimal, period int) (Series, error) {
	if err := validatePeriod(len(values), period); err != nil {
		return Series{}, err
	}

	result := newSeries(len(values), period-1)
	windowSum := decimal.Zero
	periodDecimal := decimal.NewFromInt(int64(period))

	for i, value := range values {
		windowSum = windowSum.Add(value)
		if i >= period {
			windowSum = windowSum.Sub(values[i-period])
		}
		if i >= result.Start {
			result.Values[i] = windowSum.Div(periodDecimal)
		}
	}

	return result, nil
}

// EMA 计算指数移动平均线。
// 首个有效值使用前 period 个输入值的 SMA，后续平滑系数为 2/(period+1)。
func EMA(values []decimal.Decimal, period int) (Series, error) {
	if err := validatePeriod(len(values), period); err != nil {
		return Series{}, err
	}

	result := newSeries(len(values), period-1)
	seed := decimal.Zero
	for i := 0; i < period; i++ {
		seed = seed.Add(values[i])
	}
	seed = seed.Div(decimal.NewFromInt(int64(period)))
	result.Values[result.Start] = seed

	alpha := decimal.NewFromInt(2).Div(decimal.NewFromInt(int64(period + 1)))
	oneMinusAlpha := decimal.NewFromInt(1).Sub(alpha)
	previous := seed

	for i := period; i < len(values); i++ {
		previous = values[i].Mul(alpha).Add(previous.Mul(oneMinusAlpha))
		result.Values[i] = previous
	}

	return result, nil
}

// RMA 计算 Wilder 移动平均线。
// 首个有效值使用 SMA，后续公式为 (previous*(period-1)+current)/period。
func RMA(values []decimal.Decimal, period int) (Series, error) {
	if err := validatePeriod(len(values), period); err != nil {
		return Series{}, err
	}

	result := newSeries(len(values), period-1)
	seed := decimal.Zero
	for i := 0; i < period; i++ {
		seed = seed.Add(values[i])
	}

	periodDecimal := decimal.NewFromInt(int64(period))
	periodMinusOne := decimal.NewFromInt(int64(period - 1))
	seed = seed.Div(periodDecimal)
	result.Values[result.Start] = seed

	previous := seed
	for i := period; i < len(values); i++ {
		previous = previous.Mul(periodMinusOne).Add(values[i]).Div(periodDecimal)
		result.Values[i] = previous
	}

	return result, nil
}

// WMA 计算线性加权移动平均线，窗口内越新的数据权重越高。
func WMA(values []decimal.Decimal, period int) (Series, error) {
	if err := validatePeriod(len(values), period); err != nil {
		return Series{}, err
	}

	result := newSeries(len(values), period-1)
	periodDecimal := decimal.NewFromInt(int64(period))
	weightSum := periodDecimal.
		Mul(decimal.NewFromInt(int64(period) + 1)).
		Div(decimal.NewFromInt(2))

	windowSum := decimal.Zero
	weightedSum := decimal.Zero
	for i := 0; i < period; i++ {
		windowSum = windowSum.Add(values[i])
		weight := decimal.NewFromInt(int64(i + 1))
		weightedSum = weightedSum.Add(values[i].Mul(weight))
	}
	result.Values[result.Start] = weightedSum.Div(weightSum)

	for i := period; i < len(values); i++ {
		weightedSum = weightedSum.Sub(windowSum).Add(values[i].Mul(periodDecimal))
		windowSum = windowSum.Sub(values[i-period]).Add(values[i])
		result.Values[i] = weightedSum.Div(weightSum)
	}

	return result, nil
}
