package indicator

import (
	"fmt"
	"slices"

	"github.com/shopspring/decimal"
)

// OIPoint 表示一个时间点的未平仓量快照。
// 多交易所聚合前，Value 必须被调用方转换到统一单位。
type OIPoint struct {
	Timestamp int64
	Value     decimal.Decimal
}

// ExchangeOISeries 表示一个交易所的 OI 时间序列。
type ExchangeOISeries struct {
	Exchange string
	Points   []OIPoint
}

// OIAggregationOptions 控制多交易所 OI 聚合。
type OIAggregationOptions struct {
	// MaxStalenessMillis 是允许前向使用最近快照的最长时间。
	// 0 表示不限制陈旧时间。
	MaxStalenessMillis int64
	// MinContributors 是输出聚合点所需的最少有效交易所数量。
	// 0 表示要求所有输入交易所均有效。
	MinContributors int
}

// OIResult 保存 OI 原值、绝对变化量和百分比变化率。
type OIResult struct {
	Timestamps        []int64
	Value             Series
	Change            Series
	ChangeRate        Series
	ChangeRateDefined []bool
	Contributors      []int
}

// ChangeRateAt 返回指定下标的 OI 百分比变化率。
// 当下标仍在预热期或上一期 OI 为 0 时，defined 为 false。
func (r OIResult) ChangeRateAt(index int) (value decimal.Decimal, defined bool) {
	if !r.ChangeRate.Ready(index) ||
		index < 0 ||
		index >= len(r.ChangeRateDefined) ||
		!r.ChangeRateDefined[index] {
		return decimal.Zero, false
	}
	return r.ChangeRate.Values[index], true
}

// OI 计算单交易所 OI 的绝对变化量和百分比变化率。
// ChangeRate 返回百分数；10 表示相比上一期增长 10%。
func OI(points []OIPoint) (OIResult, error) {
	if err := validateOIPoints(points); err != nil {
		return OIResult{}, err
	}

	contributors := make([]int, len(points))
	for i := range contributors {
		contributors[i] = 1
	}
	return buildOIResult(points, contributors), nil
}

// MultiExchangeOI 聚合多个交易所的 OI。
//
// OI 是存量数据，算法会在后续时间点使用每个交易所最近一次有效快照。
// MaxStalenessMillis 可限制快照陈旧时间，Contributors 记录每个结果实际包含
// 的交易所数量。
func MultiExchangeOI(
	series []ExchangeOISeries,
	options OIAggregationOptions,
) (OIResult, error) {
	if len(series) == 0 {
		return OIResult{}, ErrInsufficientData
	}
	if options.MaxStalenessMillis < 0 ||
		options.MinContributors < 0 ||
		options.MinContributors > len(series) {
		return OIResult{}, ErrInvalidAggregationOptions
	}

	minContributors := options.MinContributors
	if minContributors == 0 {
		minContributors = len(series)
	}

	exchanges := make(map[string]struct{}, len(series))
	timestampSet := make(map[int64]struct{})
	for i, exchangeSeries := range series {
		if exchangeSeries.Exchange == "" {
			return OIResult{}, fmt.Errorf("%w: index %d 的交易所名称为空", ErrInvalidExchangeSeries, i)
		}
		if _, exists := exchanges[exchangeSeries.Exchange]; exists {
			return OIResult{}, fmt.Errorf(
				"%w: 交易所 %q 重复",
				ErrInvalidExchangeSeries,
				exchangeSeries.Exchange,
			)
		}
		exchanges[exchangeSeries.Exchange] = struct{}{}

		if err := validateOIPoints(exchangeSeries.Points); err != nil {
			return OIResult{}, fmt.Errorf(
				"%w: exchange %s: %w",
				ErrInvalidExchangeSeries,
				exchangeSeries.Exchange,
				err,
			)
		}
		for _, point := range exchangeSeries.Points {
			timestampSet[point.Timestamp] = struct{}{}
		}
	}

	timestamps := make([]int64, 0, len(timestampSet))
	for timestamp := range timestampSet {
		timestamps = append(timestamps, timestamp)
	}
	slices.Sort(timestamps)

	indices := make([]int, len(series))
	latest := make([]OIPoint, len(series))
	hasLatest := make([]bool, len(series))
	aggregatedPoints := make([]OIPoint, 0, len(timestamps))
	contributors := make([]int, 0, len(timestamps))

	for _, timestamp := range timestamps {
		for i, exchangeSeries := range series {
			if indices[i] < len(exchangeSeries.Points) &&
				exchangeSeries.Points[indices[i]].Timestamp == timestamp {
				latest[i] = exchangeSeries.Points[indices[i]]
				hasLatest[i] = true
				indices[i]++
			}
		}

		total := decimal.Zero
		contributorCount := 0
		for i := range series {
			if !hasLatest[i] {
				continue
			}
			if options.MaxStalenessMillis > 0 &&
				timestamp-latest[i].Timestamp > options.MaxStalenessMillis {
				continue
			}
			total = total.Add(latest[i].Value)
			contributorCount++
		}

		if contributorCount < minContributors {
			continue
		}
		aggregatedPoints = append(aggregatedPoints, OIPoint{
			Timestamp: timestamp,
			Value:     total,
		})
		contributors = append(contributors, contributorCount)
	}

	if len(aggregatedPoints) == 0 {
		return OIResult{}, ErrInsufficientContributors
	}
	return buildOIResult(aggregatedPoints, contributors), nil
}

func validateOIPoints(points []OIPoint) error {
	if len(points) == 0 {
		return ErrInsufficientData
	}
	for i, point := range points {
		if point.Value.IsNegative() {
			return fmt.Errorf("%w: index %d", ErrInvalidOpenInterest, i)
		}
		if i > 0 && point.Timestamp <= points[i-1].Timestamp {
			return fmt.Errorf("%w: index %d", ErrNonIncreasingTimestamp, i)
		}
	}
	return nil
}

func buildOIResult(points []OIPoint, contributors []int) OIResult {
	length := len(points)
	timestamps := make([]int64, length)
	value := newSeries(length, 0)
	change := newSeries(length, 1)
	changeRate := newSeries(length, 1)
	changeRateDefined := make([]bool, length)
	resultContributors := make([]int, length)

	for i, point := range points {
		timestamps[i] = point.Timestamp
		value.Values[i] = point.Value
		resultContributors[i] = contributors[i]
		if i == 0 {
			continue
		}

		change.Values[i] = point.Value.Sub(points[i-1].Value)
		if points[i-1].Value.IsZero() {
			continue
		}
		changeRate.Values[i] = change.Values[i].
			Div(points[i-1].Value).
			Mul(oneHundred)
		changeRateDefined[i] = true
	}

	return OIResult{
		Timestamps:        timestamps,
		Value:             value,
		Change:            change,
		ChangeRate:        changeRate,
		ChangeRateDefined: changeRateDefined,
		Contributors:      resultContributors,
	}
}
