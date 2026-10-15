package execution

import (
	"errors"

	"github.com/YuanJey/go-strike"
	"github.com/shopspring/decimal"
)

var (
	ErrInvalidMultiplier = errors.New("execution: 加仓倍数必须大于 0")
	ErrInvalidRounds     = errors.New("execution: 加仓轮数必须在 1 到 50 之间")
)

// MartingaleStep 单轮马丁格尔加仓状态
type MartingaleStep struct {
	Round       int             // 轮次 (从 1 开始)
	OrderPrice  decimal.Decimal // 该轮挂单委托价
	OrderQty    decimal.Decimal // 该轮委托数量
	OrderCost   decimal.Decimal // 该轮所需资金 (Price * Qty)
	CumQty      decimal.Decimal // 累计持仓数量
	CumCost     decimal.Decimal // 累计投入总资金
	AvgPosition decimal.Decimal // 累计持仓加权平均成本价 (WAP = CumCost / CumQty)
}

// GenerateMartingalePlan 生成多轮马丁格尔 (或反马丁) 加仓执行计划
// 参数:
//   - side: PositionSideLong (逢跌加仓) 或 PositionSideShort (逢涨加仓)
//   - startPrice: 首仓成交价格
//   - baseQty: 首仓数量
//   - priceDropRate: 每轮加仓的价格跌幅/涨幅间隔 (例如 0.02 表示每跌 2% 加仓一次)
//   - volumeMultiplier: 加仓倍数 (例如 1.5 或 2.0 代表倍投)
//   - rounds: 最大总轮数 (含首仓)
func GenerateMartingalePlan(
	side strike.PositionSide,
	startPrice decimal.Decimal,
	baseQty decimal.Decimal,
	priceStepRate decimal.Decimal,
	volumeMultiplier decimal.Decimal,
	rounds int,
) ([]MartingaleStep, error) {
	if startPrice.LessThanOrEqual(decimal.Zero) {
		return nil, strike.ErrInvalidPrice
	}
	if baseQty.LessThanOrEqual(decimal.Zero) {
		return nil, strike.ErrInvalidQuantity
	}
	if priceStepRate.LessThanOrEqual(decimal.Zero) || priceStepRate.GreaterThanOrEqual(decimal.NewFromInt(1)) {
		return nil, strike.ErrInvalidCallbackRate
	}
	if volumeMultiplier.LessThanOrEqual(decimal.Zero) {
		return nil, ErrInvalidMultiplier
	}
	if rounds < 1 || rounds > 50 {
		return nil, ErrInvalidRounds
	}

	steps := make([]MartingaleStep, rounds)

	currentPrice := startPrice
	currentQty := baseQty
	cumQty := decimal.Zero
	cumCost := decimal.Zero

	one := decimal.NewFromInt(1)

	for i := 1; i <= rounds; i++ {
		if i > 1 {
			// 更新下一轮委托价
			if side == strike.PositionSideLong {
				currentPrice = currentPrice.Mul(one.Sub(priceStepRate))
			} else {
				currentPrice = currentPrice.Mul(one.Add(priceStepRate))
			}
			// 更新下一轮委托量
			currentQty = currentQty.Mul(volumeMultiplier)
		}

		orderCost := currentPrice.Mul(currentQty)
		cumQty = cumQty.Add(currentQty)
		cumCost = cumCost.Add(orderCost)
		avgPosition := cumCost.Div(cumQty)

		steps[i-1] = MartingaleStep{
			Round:       i,
			OrderPrice:  currentPrice,
			OrderQty:    currentQty,
			OrderCost:   orderCost,
			CumQty:      cumQty,
			CumCost:     cumCost,
			AvgPosition: avgPosition,
		}
	}

	return steps, nil
}
