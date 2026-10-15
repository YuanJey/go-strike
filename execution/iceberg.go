package execution

import (
	"math/rand"
	"time"

	"github.com/YuanJey/go-strike"
	"github.com/shopspring/decimal"
)

// IcebergSlice 冰山委托单切片
type IcebergSlice struct {
	Index    int             // 切片序号
	Quantity decimal.Decimal // 该片委托数量
	Ratio    decimal.Decimal // 该片占总量的权重比例
}

// SliceIceberg 将大单拆解为若干个具备离散随机浮动的冰山子单
// 参数:
//   - totalQty: 总下单量
//   - slices: 计划拆分片数 (必须 >= 2)
//   - variance: 随机离散浮动系数 (0 到 0.5 之间，例如 0.2 表示单笔在基准均值的 80% ~ 120% 之间随机波动)
func SliceIceberg(
	totalQty decimal.Decimal,
	slices int,
	variance decimal.Decimal,
) ([]IcebergSlice, error) {
	if totalQty.LessThanOrEqual(decimal.Zero) {
		return nil, strike.ErrInvalidQuantity
	}
	if slices < 2 {
		return nil, strike.ErrInvalidSlices
	}
	if variance.IsNegative() || variance.GreaterThan(decimal.NewFromFloat(0.5)) {
		variance = decimal.NewFromFloat(0.2) // 默认 20% 浮动
	}

	r := rand.New(rand.NewSource(time.Now().UnixNano()))
	result := make([]IcebergSlice, slices)

	baseSliceQty := totalQty.Div(decimal.NewFromInt(int64(slices)))
	varianceF, _ := variance.Float64()

	allocated := decimal.Zero

	for i := 0; i < slices-1; i++ {
		// 生成 [-variance, +variance] 的随机因子
		factor := 1.0 + (r.Float64()*2-1)*varianceF
		sliceQty := baseSliceQty.Mul(decimal.NewFromFloat(factor))

		// 防止拆分出 <= 0 的极端值
		if sliceQty.LessThanOrEqual(decimal.Zero) {
			sliceQty = baseSliceQty.Mul(decimal.NewFromFloat(0.5))
		}

		allocated = allocated.Add(sliceQty)
		result[i] = IcebergSlice{
			Index:    i + 1,
			Quantity: sliceQty,
			Ratio:    sliceQty.Div(totalQty),
		}
	}

	// 最后一个子单直接填充剩余量，确保总和精准无偏差
	lastSliceQty := totalQty.Sub(allocated)
	if lastSliceQty.LessThanOrEqual(decimal.Zero) {
		// 若因随机正偏差过大导致尾单亏空，按等额重分配保底
		equalQty := totalQty.Div(decimal.NewFromInt(int64(slices)))
		for i := 0; i < slices; i++ {
			result[i] = IcebergSlice{
				Index:    i + 1,
				Quantity: equalQty,
				Ratio:    equalQty.Div(totalQty),
			}
		}
		return result, nil
	}

	result[slices-1] = IcebergSlice{
		Index:    slices,
		Quantity: lastSliceQty,
		Ratio:    lastSliceQty.Div(totalQty),
	}

	return result, nil
}
