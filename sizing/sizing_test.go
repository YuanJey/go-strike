package sizing

import (
	"testing"

	"github.com/YuanJey/go-strike"
	"github.com/shopspring/decimal"
)

func TestFixedFractionalSizing(t *testing.T) {
	// 账户资金 10,000 USDT，单笔承受 1% 风险 (100 USDT)
	// 入场价 100，止损价 95 (单币风险 5 USDT) -> 应买入 100 / 5 = 20 个币
	equity := decimal.NewFromFloat(10000)
	riskRatio := decimal.NewFromFloat(0.01)
	entryPrice := decimal.NewFromFloat(100)
	stopLoss := decimal.NewFromFloat(95)
	maxLeverage := decimal.NewFromFloat(5)

	res, err := FixedFractionalSizing(strike.PositionSideLong, equity, riskRatio, entryPrice, stopLoss, maxLeverage)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	expectedQty := decimal.NewFromFloat(20)
	if !res.Quantity.Equal(expectedQty) {
		t.Errorf("expected quantity %s, got %s", expectedQty, res.Quantity)
	}

	expectedRisk := decimal.NewFromFloat(100)
	if !res.RiskAmount.Equal(expectedRisk) {
		t.Errorf("expected risk amount %s, got %s", expectedRisk, res.RiskAmount)
	}

	// 杠杆超出保护测试: 设置最大杠杆为 0.1 (名义价值 2000 / 10000 = 0.2 > 0.1)
	_, err = FixedFractionalSizing(strike.PositionSideLong, equity, riskRatio, entryPrice, stopLoss, decimal.NewFromFloat(0.1))
	if err == nil {
		t.Errorf("expected leverage exceeded error, got nil")
	}

	// 止损方向错误测试: 多头止损高于入场
	_, err = FixedFractionalSizing(strike.PositionSideLong, equity, riskRatio, entryPrice, decimal.NewFromFloat(105), maxLeverage)
	if err == nil {
		t.Errorf("expected invalid stop loss error for long, got nil")
	}
}

func TestCalculateKelly(t *testing.T) {
	// 胜率 60% (0.6)，盈亏比 1.5，半凯利 (0.5)
	// Edge = 1.5 * 0.6 - 0.4 = 0.9 - 0.4 = 0.5
	// Full Kelly = 0.5 / 1.5 = 1/3 ≈ 0.3333333333
	// Half Kelly = (1/3) * 0.5 = 1/6 ≈ 0.1666666667
	winRate := decimal.NewFromFloat(0.6)
	winLossRatio := decimal.NewFromFloat(1.5)
	fraction := decimal.NewFromFloat(0.5)

	res, err := CalculateKelly(winRate, winLossRatio, fraction)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.HasPositiveExpectancy {
		t.Errorf("expected positive expectancy")
	}

	expectedFull := decimal.NewFromFloat(1).Div(decimal.NewFromFloat(3))
	if !res.FullKellyFraction.Round(4).Equal(expectedFull.Round(4)) {
		t.Errorf("expected full kelly %s, got %s", expectedFull.Round(4), res.FullKellyFraction.Round(4))
	}

	// 负期望测试：胜率 30%，盈亏比 1.0 (Edge = 0.3 - 0.7 = -0.4 < 0)
	negRes, err := CalculateKelly(decimal.NewFromFloat(0.3), decimal.NewFromFloat(1.0), fraction)
	if err != nil {
		t.Fatalf("unexpected error on negative edge: %v", err)
	}
	if negRes.HasPositiveExpectancy {
		t.Errorf("expected negative expectancy")
	}
	if !negRes.AdjustedFraction.IsZero() {
		t.Errorf("expected zero fraction on negative expectancy, got %s", negRes.AdjustedFraction)
	}
}

func TestATRPositionSizing(t *testing.T) {
	// 资金 10,000，风险 2% (200)，ATR = 5，Multiplier = 2 -> 止损距离 10 -> 数量 20
	equity := decimal.NewFromFloat(10000)
	riskRatio := decimal.NewFromFloat(0.02)
	atr := decimal.NewFromFloat(5)
	mult := decimal.NewFromFloat(2)

	qty, err := ATRPositionSizing(equity, riskRatio, atr, mult)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	expected := decimal.NewFromFloat(20)
	if !qty.Equal(expected) {
		t.Errorf("expected %s, got %s", expected, qty)
	}
}
