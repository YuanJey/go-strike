package execution

import (
	"errors"
	"time"

	"github.com/YuanJey/go-strike"
	"github.com/shopspring/decimal"
)

var (
	ErrInvalidTWAPDuration = errors.New("execution: TWAP 执行总时长必须大于等于间隔时长")
)

// TWAPSlice TWAP 拆单单笔计划
type TWAPSlice struct {
	Index        int             // 切片序号 (从 1 开始)
	TriggerAfter time.Duration   // 距离起跑时间的偏移执行延迟
	Quantity     decimal.Decimal // 本批次推荐执行数量
	Ratio        decimal.Decimal // 占总体量比例
}

// GenerateTWAPPlan 生成时间加权平均价格算法 (TWAP) 执行切片计划
// 参数:
//   - totalQty: 计划执行的总数量
//   - totalDuration: 总执行窗口时长 (例如 1 小时)
//   - interval: 单次切片执行间隔 (例如 1 分钟)
func GenerateTWAPPlan(
	totalQty decimal.Decimal,
	totalDuration time.Duration,
	interval time.Duration,
) ([]TWAPSlice, error) {
	if totalQty.LessThanOrEqual(decimal.Zero) {
		return nil, strike.ErrInvalidQuantity
	}
	if interval <= 0 || totalDuration < interval {
		return nil, ErrInvalidTWAPDuration
	}

	slicesCount := int(totalDuration / interval)
	if slicesCount <= 0 {
		slicesCount = 1
	}

	slices := make([]TWAPSlice, slicesCount)
	qtyPerSlice := totalQty.Div(decimal.NewFromInt(int64(slicesCount)))
	allocated := decimal.Zero

	for i := 0; i < slicesCount-1; i++ {
		offset := time.Duration(i) * interval
		allocated = allocated.Add(qtyPerSlice)
		slices[i] = TWAPSlice{
			Index:        i + 1,
			TriggerAfter: offset,
			Quantity:     qtyPerSlice,
			Ratio:        qtyPerSlice.Div(totalQty),
		}
	}

	// 最后一个切片消化整除尾差
	lastQty := totalQty.Sub(allocated)
	slices[slicesCount-1] = TWAPSlice{
		Index:        slicesCount,
		TriggerAfter: time.Duration(slicesCount-1) * interval,
		Quantity:     lastQty,
		Ratio:        lastQty.Div(totalQty),
	}

	return slices, nil
}
