package indicator

import "github.com/shopspring/decimal"

// TypicalPrice 计算单根 K 线的典型价格：(High+Low+Close)/3。
func TypicalPrice(candle Candle) decimal.Decimal {
	return candle.High.Add(candle.Low).Add(candle.Close).Div(decimal.NewFromInt(3))
}

// VWAP 计算从输入序列起点开始累计的成交量加权平均价。
// 调用方负责按交易时段切分输入，以决定 VWAP 的重置边界。
func VWAP(candles []Candle) (Series, error) {
	if err := validateCandles(candles); err != nil {
		return Series{}, err
	}

	result := newSeries(len(candles), len(candles))
	cumulativePriceVolume := decimal.Zero
	cumulativeVolume := decimal.Zero

	for i, candle := range candles {
		cumulativePriceVolume = cumulativePriceVolume.Add(TypicalPrice(candle).Mul(candle.Volume))
		cumulativeVolume = cumulativeVolume.Add(candle.Volume)
		if cumulativeVolume.IsZero() {
			continue
		}
		if result.Start == len(candles) {
			result.Start = i
		}
		result.Values[i] = cumulativePriceVolume.Div(cumulativeVolume)
	}

	if result.Start == len(candles) {
		return Series{}, ErrZeroVolume
	}
	return result, nil
}

// OBV 计算能量潮指标。
// 首个值为 0；收盘价上涨时累加成交量，下跌时扣减成交量，持平时保持不变。
func OBV(candles []Candle) (Series, error) {
	if err := validateCandles(candles); err != nil {
		return Series{}, err
	}

	result := newSeries(len(candles), 0)
	for i := 1; i < len(candles); i++ {
		result.Values[i] = result.Values[i-1]
		if candles[i].Close.GreaterThan(candles[i-1].Close) {
			result.Values[i] = result.Values[i].Add(candles[i].Volume)
		} else if candles[i].Close.LessThan(candles[i-1].Close) {
			result.Values[i] = result.Values[i].Sub(candles[i].Volume)
		}
	}

	return result, nil
}
