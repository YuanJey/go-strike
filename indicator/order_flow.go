package indicator

import (
	"fmt"
	"slices"

	"github.com/shopspring/decimal"
)

// TradeFlowPoint 表示一个时间桶内按主动方分类的成交量。
// BuyVolume 与 SellVolume 必须使用相同单位，例如基础币数量或 USD 名义价值。
type TradeFlowPoint struct {
	Timestamp  int64
	BuyVolume  decimal.Decimal
	SellVolume decimal.Decimal
}

// ExchangeTradeFlow 表示一个交易所的主动成交量时间序列。
type ExchangeTradeFlow struct {
	Exchange string
	Points   []TradeFlowPoint
}

// CVDResult 保存每个时间桶的主动买卖量、净差和累计成交量差。
type CVDResult struct {
	Timestamps   []int64
	BuyVolume    Series
	SellVolume   Series
	Delta        Series
	Cumulative   Series
	Contributors []int
}

// CVD 计算单交易所累计成交量差。
//
// Delta = BuyVolume - SellVolume
// Cumulative[i] = initialValue + sum(Delta[0:i])
func CVD(
	points []TradeFlowPoint,
	initialValue decimal.Decimal,
) (CVDResult, error) {
	if err := validateTradeFlowPoints(points); err != nil {
		return CVDResult{}, err
	}

	contributors := make([]int, len(points))
	for i := range contributors {
		contributors[i] = 1
	}
	return buildCVDResult(points, contributors, initialValue), nil
}

// MultiExchangeCVD 按时间戳汇总多个交易所的 CVD。
//
// 输入序列必须已经转换到同一计量单位。CVD 是流量数据，缺失时间桶不会
// 使用前值填充；调用方应为完整但无成交的时间桶显式传入零成交量。
func MultiExchangeCVD(
	series []ExchangeTradeFlow,
	initialValue decimal.Decimal,
) (CVDResult, error) {
	if len(series) == 0 {
		return CVDResult{}, ErrInsufficientData
	}

	type aggregate struct {
		buy          decimal.Decimal
		sell         decimal.Decimal
		contributors int
	}

	exchanges := make(map[string]struct{}, len(series))
	aggregates := make(map[int64]aggregate)
	for i, exchangeSeries := range series {
		if exchangeSeries.Exchange == "" {
			return CVDResult{}, fmt.Errorf("%w: index %d 的交易所名称为空", ErrInvalidExchangeSeries, i)
		}
		if _, exists := exchanges[exchangeSeries.Exchange]; exists {
			return CVDResult{}, fmt.Errorf(
				"%w: 交易所 %q 重复",
				ErrInvalidExchangeSeries,
				exchangeSeries.Exchange,
			)
		}
		exchanges[exchangeSeries.Exchange] = struct{}{}

		if err := validateTradeFlowPoints(exchangeSeries.Points); err != nil {
			return CVDResult{}, fmt.Errorf(
				"%w: exchange %s: %w",
				ErrInvalidExchangeSeries,
				exchangeSeries.Exchange,
				err,
			)
		}

		for _, point := range exchangeSeries.Points {
			current := aggregates[point.Timestamp]
			current.buy = current.buy.Add(point.BuyVolume)
			current.sell = current.sell.Add(point.SellVolume)
			current.contributors++
			aggregates[point.Timestamp] = current
		}
	}

	timestamps := make([]int64, 0, len(aggregates))
	for timestamp := range aggregates {
		timestamps = append(timestamps, timestamp)
	}
	slices.Sort(timestamps)

	points := make([]TradeFlowPoint, len(timestamps))
	contributors := make([]int, len(timestamps))
	for i, timestamp := range timestamps {
		current := aggregates[timestamp]
		points[i] = TradeFlowPoint{
			Timestamp:  timestamp,
			BuyVolume:  current.buy,
			SellVolume: current.sell,
		}
		contributors[i] = current.contributors
	}

	return buildCVDResult(points, contributors, initialValue), nil
}

func validateTradeFlowPoints(points []TradeFlowPoint) error {
	if len(points) == 0 {
		return ErrInsufficientData
	}
	for i, point := range points {
		if point.BuyVolume.IsNegative() || point.SellVolume.IsNegative() {
			return fmt.Errorf("%w: index %d", ErrInvalidTradeFlow, i)
		}
		if i > 0 && point.Timestamp <= points[i-1].Timestamp {
			return fmt.Errorf("%w: index %d", ErrNonIncreasingTimestamp, i)
		}
	}
	return nil
}

func buildCVDResult(
	points []TradeFlowPoint,
	contributors []int,
	initialValue decimal.Decimal,
) CVDResult {
	length := len(points)
	timestamps := make([]int64, length)
	buyVolume := newSeries(length, 0)
	sellVolume := newSeries(length, 0)
	delta := newSeries(length, 0)
	cumulative := newSeries(length, 0)
	resultContributors := make([]int, length)
	currentCumulative := initialValue

	for i, point := range points {
		timestamps[i] = point.Timestamp
		buyVolume.Values[i] = point.BuyVolume
		sellVolume.Values[i] = point.SellVolume
		delta.Values[i] = point.BuyVolume.Sub(point.SellVolume)
		currentCumulative = currentCumulative.Add(delta.Values[i])
		cumulative.Values[i] = currentCumulative
		resultContributors[i] = contributors[i]
	}

	return CVDResult{
		Timestamps:   timestamps,
		BuyVolume:    buyVolume,
		SellVolume:   sellVolume,
		Delta:        delta,
		Cumulative:   cumulative,
		Contributors: resultContributors,
	}
}
