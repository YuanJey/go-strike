package execution

import (
	"testing"
	"time"

	"github.com/YuanJey/go-strike"
	"github.com/shopspring/decimal"
)

func TestGenerateArithmeticGrid(t *testing.T) {
	upper := decimal.NewFromFloat(100)
	lower := decimal.NewFromFloat(80)
	grids := 5
	totalQty := decimal.NewFromFloat(10)

	orders, err := GenerateArithmeticGrid(upper, lower, grids, totalQty)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(orders) != grids {
		t.Fatalf("expected %d orders, got %d", grids, len(orders))
	}

	// 阶梯价应该为 80, 85, 90, 95, 100
	expectedPrices := []float64{80, 85, 90, 95, 100}
	for i, o := range orders {
		exp := decimal.NewFromFloat(expectedPrices[i])
		if !o.Price.Equal(exp) {
			t.Errorf("order %d expected price %s, got %s", i, exp, o.Price)
		}
	}
}

func TestGenerateGeometricGrid(t *testing.T) {
	upper := decimal.NewFromFloat(160)
	lower := decimal.NewFromFloat(10)
	grids := 5 // 10 -> 20 -> 40 -> 80 -> 160 (ratio = 2)
	totalQty := decimal.NewFromFloat(5)

	orders, err := GenerateGeometricGrid(upper, lower, grids, totalQty)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	expectedPrices := []float64{10, 20, 40, 80, 160}
	for i, o := range orders {
		exp := decimal.NewFromFloat(expectedPrices[i])
		if !o.Price.Round(2).Equal(exp.Round(2)) {
			t.Errorf("grid %d expected price %s, got %s", i, exp, o.Price)
		}
	}
}

func TestGenerateMartingalePlan(t *testing.T) {
	// 多头首仓 100，基础量 1，每跌 10% 补仓，数量 2 倍，共 3 轮
	// Round 1: Price 100, Qty 1, Cost 100, CumQty 1, CumCost 100, AvgPos 100
	// Round 2: Price 90,  Qty 2, Cost 180, CumQty 3, CumCost 280, AvgPos 280/3 ≈ 93.3333
	// Round 3: Price 81,  Qty 4, Cost 324, CumQty 7, CumCost 604, AvgPos 604/7 ≈ 86.2857
	startPrice := decimal.NewFromFloat(100)
	baseQty := decimal.NewFromFloat(1)
	stepRate := decimal.NewFromFloat(0.1)
	multiplier := decimal.NewFromFloat(2)
	rounds := 3

	steps, err := GenerateMartingalePlan(strike.PositionSideLong, startPrice, baseQty, stepRate, multiplier, rounds)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(steps) != rounds {
		t.Fatalf("expected %d rounds, got %d", rounds, len(steps))
	}

	// 验证第 2 轮
	if !steps[1].OrderPrice.Equal(decimal.NewFromFloat(90)) {
		t.Errorf("round 2 price expected 90, got %s", steps[1].OrderPrice)
	}
	if !steps[1].CumQty.Equal(decimal.NewFromFloat(3)) {
		t.Errorf("round 2 cum qty expected 3, got %s", steps[1].CumQty)
	}
	expectedAvg2 := decimal.NewFromFloat(280).Div(decimal.NewFromFloat(3))
	if !steps[1].AvgPosition.Round(4).Equal(expectedAvg2.Round(4)) {
		t.Errorf("round 2 avg pos expected %s, got %s", expectedAvg2.Round(4), steps[1].AvgPosition.Round(4))
	}
}

func TestSliceIceberg(t *testing.T) {
	totalQty := decimal.NewFromFloat(100)
	slices := 10
	variance := decimal.NewFromFloat(0.2)

	slicesRes, err := SliceIceberg(totalQty, slices, variance)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(slicesRes) != slices {
		t.Fatalf("expected %d slices, got %d", slices, len(slicesRes))
	}

	// 必须严格校验所有切片数量相加精确等于 100
	sum := decimal.Zero
	for _, s := range slicesRes {
		sum = sum.Add(s.Quantity)
	}
	if !sum.Equal(totalQty) {
		t.Errorf("iceberg sum mismatch! expected %s, got %s", totalQty, sum)
	}
}

func TestGenerateTWAPPlan(t *testing.T) {
	totalQty := decimal.NewFromFloat(60)
	totalDuration := 10 * time.Minute
	interval := 2 * time.Minute // 5 slices

	plan, err := GenerateTWAPPlan(totalQty, totalDuration, interval)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(plan) != 5 {
		t.Fatalf("expected 5 slices, got %d", len(plan))
	}

	sum := decimal.Zero
	for _, p := range plan {
		sum = sum.Add(p.Quantity)
	}
	if !sum.Equal(totalQty) {
		t.Errorf("twap sum mismatch! expected %s, got %s", totalQty, sum)
	}
}
