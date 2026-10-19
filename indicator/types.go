package indicator

import (
	"errors"
	"fmt"

	"github.com/shopspring/decimal"
)

var (
	ErrInvalidPeriod             = errors.New("indicator: 周期必须大于 0")
	ErrInsufficientData          = errors.New("indicator: 数据量不足")
	ErrInvalidMultiplier         = errors.New("indicator: 倍数不能小于 0")
	ErrInvalidCandle             = errors.New("indicator: K 线数据无效")
	ErrInvalidMACDPeriods        = errors.New("indicator: MACD 周期必须满足 fast < slow 且 signal > 0")
	ErrZeroReference             = errors.New("indicator: 参考值不能为 0")
	ErrZeroVolume                = errors.New("indicator: 累计成交量必须大于 0")
	ErrInvalidTradeFlow          = errors.New("indicator: 主动成交量不能为负数")
	ErrInvalidOpenInterest       = errors.New("indicator: OI 值不能为负数")
	ErrInvalidExchangeSeries     = errors.New("indicator: 交易所数据序列无效")
	ErrNonIncreasingTimestamp    = errors.New("indicator: 时间戳必须严格递增")
	ErrInvalidAggregationOptions = errors.New("indicator: 聚合参数无效")
	ErrInsufficientContributors  = errors.New("indicator: 有效贡献交易所数量不足")
)

// Candle 表示指标计算使用的标准 OHLCV K 线。
type Candle struct {
	Open   decimal.Decimal
	High   decimal.Decimal
	Low    decimal.Decimal
	Close  decimal.Decimal
	Volume decimal.Decimal
}

// Series 保存与输入等长的指标序列，Start 表示首个有效值下标。
// Start 之前的 Values 为零值，不应参与业务判断。
type Series struct {
	Values []decimal.Decimal
	Start  int
}

// Ready 判断指定下标是否已经越过指标预热期。
func (s Series) Ready(index int) bool {
	return index >= s.Start && index >= 0 && index < len(s.Values)
}

// At 返回指定下标的指标值以及该值是否有效。
func (s Series) At(index int) (decimal.Decimal, bool) {
	if !s.Ready(index) {
		return decimal.Zero, false
	}
	return s.Values[index], true
}

// ValidValues 返回有效区间的副本。
func (s Series) ValidValues() []decimal.Decimal {
	if s.Start < 0 || s.Start >= len(s.Values) {
		return nil
	}
	values := make([]decimal.Decimal, len(s.Values)-s.Start)
	copy(values, s.Values[s.Start:])
	return values
}

func newSeries(length, start int) Series {
	return Series{
		Values: make([]decimal.Decimal, length),
		Start:  start,
	}
}

func validatePeriod(length, period int) error {
	if period <= 0 {
		return ErrInvalidPeriod
	}
	if length < period {
		return fmt.Errorf("%w: 需要至少 %d 个数据点，当前为 %d", ErrInsufficientData, period, length)
	}
	return nil
}

func validateCandles(candles []Candle) error {
	if len(candles) == 0 {
		return ErrInsufficientData
	}
	for i, candle := range candles {
		if candle.High.LessThan(candle.Low) || candle.Volume.IsNegative() {
			return fmt.Errorf("%w: index %d", ErrInvalidCandle, i)
		}
	}
	return nil
}
