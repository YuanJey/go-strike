package execution

import (
	"math"

	"github.com/YuanJey/go-strike"
	"github.com/shopspring/decimal"
)

// GridOrder 单个网格委托挂单要素
type GridOrder struct {
	Index    int             // 网格索引 (0 到 N-1)
	Price    decimal.Decimal // 挂单价格
	Quantity decimal.Decimal // 挂单数量
	Notional decimal.Decimal // 挂单名义价值 (Price * Quantity)
}

// GenerateArithmeticGrid 生成等差算术网格挂单计划
// upperPrice: 网格最高价
// lowerPrice: 网格最低价
// grids: 网格阶梯总数 (>= 2)
// totalQty: 计划在所有网格分配的总数量 (若为等量分配)
func GenerateArithmeticGrid(
	upperPrice, lowerPrice decimal.Decimal,
	grids int,
	totalQty decimal.Decimal,
) ([]GridOrder, error) {
	if grids < 2 {
		return nil, strike.ErrInvalidGridParameters
	}
	if upperPrice.LessThanOrEqual(lowerPrice) || lowerPrice.LessThanOrEqual(decimal.Zero) {
		return nil, strike.ErrInvalidGridParameters
	}
	if totalQty.LessThanOrEqual(decimal.Zero) {
		return nil, strike.ErrInvalidQuantity
	}

	step := upperPrice.Sub(lowerPrice).Div(decimal.NewFromInt(int64(grids - 1)))
	qtyPerGrid := totalQty.Div(decimal.NewFromInt(int64(grids)))

	orders := make([]GridOrder, grids)
	for i := 0; i < grids; i++ {
		price := lowerPrice.Add(step.Mul(decimal.NewFromInt(int64(i))))
		orders[i] = GridOrder{
			Index:    i,
			Price:    price,
			Quantity: qtyPerGrid,
			Notional: price.Mul(qtyPerGrid),
		}
	}

	return orders, nil
}

// GenerateGeometricGrid 生成等比几何网格挂单计划 (每格价差百分比恒定)
// ratio = (upperPrice / lowerPrice) ^ (1 / (grids - 1))
func GenerateGeometricGrid(
	upperPrice, lowerPrice decimal.Decimal,
	grids int,
	totalQty decimal.Decimal,
) ([]GridOrder, error) {
	if grids < 2 {
		return nil, strike.ErrInvalidGridParameters
	}
	if upperPrice.LessThanOrEqual(lowerPrice) || lowerPrice.LessThanOrEqual(decimal.Zero) {
		return nil, strike.ErrInvalidGridParameters
	}
	if totalQty.LessThanOrEqual(decimal.Zero) {
		return nil, strike.ErrInvalidQuantity
	}

	upperF, _ := upperPrice.Float64()
	lowerF, _ := lowerPrice.Float64()
	power := 1.0 / float64(grids-1)
	ratioF := math.Pow(upperF/lowerF, power)
	ratio := decimal.NewFromFloat(ratioF)

	qtyPerGrid := totalQty.Div(decimal.NewFromInt(int64(grids)))
	orders := make([]GridOrder, grids)

	currentPrice := lowerPrice
	for i := 0; i < grids; i++ {
		if i == grids-1 {
			currentPrice = upperPrice // 防止浮点累乘尾数漂移
		}
		orders[i] = GridOrder{
			Index:    i,
			Price:    currentPrice,
			Quantity: qtyPerGrid,
			Notional: currentPrice.Mul(qtyPerGrid),
		}
		currentPrice = currentPrice.Mul(ratio)
	}

	return orders, nil
}
