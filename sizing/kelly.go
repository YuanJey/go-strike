package sizing

import (
	"errors"

	"github.com/shopspring/decimal"
)

var (
	ErrInvalidWinRate     = errors.New("sizing: 胜率必须在 (0, 1) 区间内")
	ErrInvalidWinLossRate = errors.New("sizing: 盈亏比必须大于 0")
	ErrInvalidFraction    = errors.New("sizing: 凯利系数必须在 (0, 1] 区间内")
)

// KellyResult 凯利公式测算结果
type KellyResult struct {
	FullKellyFraction       decimal.Decimal // 全凯利最优下注比例 f* = (b*p - q) / b
	AdjustedFraction        decimal.Decimal // 经过分数凯利 (如 Half-Kelly) 调节后的仓位比例
	ExpectedGrowthRateEdge  decimal.Decimal // 期望优势 Edge = b*p - q
	HasPositiveExpectancy   bool            // 是否具备正数学期望
}

// CalculateKelly 计算凯利公式推荐仓位比例
// 参数:
//   - winRate (p): 历史胜率 (例如 0.55 代表 55%)
//   - winLossRatio (b): 平均盈亏比 (平均盈利 / 平均亏损，例如 1.5)
//   - fraction: 凯利保守调节系数 (例如 0.5 代表 Half-Kelly，0.25 代表 Quarter-Kelly；为 1 代表全凯利)
func CalculateKelly(winRate, winLossRatio, fraction decimal.Decimal) (KellyResult, error) {
	one := decimal.NewFromInt(1)

	if winRate.LessThanOrEqual(decimal.Zero) || winRate.GreaterThanOrEqual(one) {
		return KellyResult{}, ErrInvalidWinRate
	}
	if winLossRatio.LessThanOrEqual(decimal.Zero) {
		return KellyResult{}, ErrInvalidWinLossRate
	}
	if fraction.LessThanOrEqual(decimal.Zero) || fraction.GreaterThan(one) {
		return KellyResult{}, ErrInvalidFraction
	}

	// 亏损率 q = 1 - p
	lossRate := one.Sub(winRate)

	// 策略数学期望/优势: Edge = b * p - q
	edge := winLossRatio.Mul(winRate).Sub(lossRate)

	// 若期望为负或零，无统计套利优势，推荐下注比例直接归零
	if edge.LessThanOrEqual(decimal.Zero) {
		return KellyResult{
			FullKellyFraction:      decimal.Zero,
			AdjustedFraction:       decimal.Zero,
			ExpectedGrowthRateEdge: edge,
			HasPositiveExpectancy:  false,
		}, nil
	}

	// 全凯利比例 f* = (b*p - q) / b
	fullKelly := edge.Div(winLossRatio)

	// 依照分数因子降噪调节
	adjusted := fullKelly.Mul(fraction)

	return KellyResult{
		FullKellyFraction:      fullKelly,
		AdjustedFraction:       adjusted,
		ExpectedGrowthRateEdge: edge,
		HasPositiveExpectancy:  true,
	}, nil
}
