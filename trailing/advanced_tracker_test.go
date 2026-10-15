package trailing

import (
	"sync"
	"testing"
	"time"

	"github.com/YuanJey/go-strike"
	"github.com/shopspring/decimal"
)

func TestAdvancedTracker_LongPriceBoundary(t *testing.T) {
	startTime := time.Date(2026, 10, 15, 10, 0, 0, 0, time.UTC)

	// 多头开仓价 100，激活价 110，做多保护下限（硬止损）95
	cfg := AdvancedTrackerConfig{
		Side:            strike.PositionSideLong,
		CallbackType:    strike.CallbackTypeRate,
		InitialCallback: decimal.NewFromFloat(0.05), // 5%
		ActivationPrice: decimal.NewFromFloat(110),
		LowerBoundPrice: decimal.NewFromFloat(95),
		StartTime:       startTime,
	}

	tracker, err := NewAdvancedTracker(cfg)
	if err != nil {
		t.Fatalf("failed to create tracker: %v", err)
	}

	// 价格跌破 95 -> 立即触发硬边界止损
	res, err := tracker.Update(startTime.Add(1*time.Minute), decimal.NewFromFloat(94.5))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.IsTriggered || res.Reason != TriggerReasonPriceBoundary {
		t.Errorf("expected TriggerReasonPriceBoundary, got %+v", res)
	}
}

func TestAdvancedTracker_ShortPriceBoundary(t *testing.T) {
	startTime := time.Date(2026, 10, 15, 10, 0, 0, 0, time.UTC)

	// 空头开仓价 100，激活价 90，做空保护上限（硬止损）105
	cfg := AdvancedTrackerConfig{
		Side:            strike.PositionSideShort,
		CallbackType:    strike.CallbackTypeRate,
		InitialCallback: decimal.NewFromFloat(0.05),
		ActivationPrice: decimal.NewFromFloat(90),
		UpperBoundPrice: decimal.NewFromFloat(105),
		StartTime:       startTime,
	}

	tracker, err := NewAdvancedTracker(cfg)
	if err != nil {
		t.Fatalf("failed to create tracker: %v", err)
	}

	// 价格涨破 105 -> 立即触发硬边界止损
	res, err := tracker.Update(startTime.Add(1*time.Minute), decimal.NewFromFloat(105.1))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.IsTriggered || res.Reason != TriggerReasonPriceBoundary {
		t.Errorf("expected TriggerReasonPriceBoundary, got %+v", res)
	}
}

func TestAdvancedTracker_CoolingOff(t *testing.T) {
	startTime := time.Date(2026, 10, 15, 10, 0, 0, 0, time.UTC)

	// 立即激活，冷却期 5 分钟
	cfg := AdvancedTrackerConfig{
		Side:               strike.PositionSideLong,
		CallbackType:       strike.CallbackTypeRate,
		InitialCallback:    decimal.NewFromFloat(0.02), // 2%
		ActivationPrice:    decimal.Zero,               // 立即激活
		LowerBoundPrice:    decimal.NewFromFloat(90),
		StartTime:          startTime,
		CoolingOffDuration: 5 * time.Minute,
	}

	tracker, err := NewAdvancedTracker(cfg)
	if err != nil {
		t.Fatalf("failed to create tracker: %v", err)
	}

	// 1. 开仓 1 分钟后，价格跌 2.5% (从 100 跌至 97.5)，由于在冷却期内，不触发移动止损
	res1, err := tracker.Update(startTime.Add(1*time.Minute), decimal.NewFromFloat(100))
	if err != nil || !res1.InCoolingOff {
		t.Fatalf("res1 should be in cooling off: %+v", res1)
	}

	res2, err := tracker.Update(startTime.Add(2*time.Minute), decimal.NewFromFloat(97.5))
	if err != nil || res2.IsTriggered {
		t.Fatalf("cooling off should shield pullback: %+v", res2)
	}

	// 2. 但若在冷却期内跌破 LowerBoundPrice (90)，仍应触发硬止损！
	res3, err := tracker.Update(startTime.Add(3*time.Minute), decimal.NewFromFloat(89.9))
	if err != nil || !res3.IsTriggered || res3.Reason != TriggerReasonPriceBoundary {
		t.Fatalf("cooling off must not shield lower bound breach: %+v", res3)
	}
}

func TestAdvancedTracker_Timeout(t *testing.T) {
	startTime := time.Date(2026, 10, 15, 10, 0, 0, 0, time.UTC)

	// 最大持仓时长 30 分钟
	cfg := AdvancedTrackerConfig{
		Side:            strike.PositionSideLong,
		CallbackType:    strike.CallbackTypeRate,
		InitialCallback: decimal.NewFromFloat(0.05),
		StartTime:       startTime,
		MaxHoldDuration: 30 * time.Minute,
	}

	tracker, err := NewAdvancedTracker(cfg)
	if err != nil {
		t.Fatalf("failed to create tracker: %v", err)
	}

	// 29 分钟，未超时
	res1, err := tracker.Update(startTime.Add(29*time.Minute), decimal.NewFromFloat(100))
	if err != nil || res1.IsTriggered {
		t.Fatalf("should not timeout at 29min: %+v", res1)
	}

	// 30 分钟，超时触发强平
	res2, err := tracker.Update(startTime.Add(30*time.Minute), decimal.NewFromFloat(100))
	if err != nil || !res2.IsTriggered || res2.Reason != TriggerReasonTimeout {
		t.Fatalf("expected TriggerReasonTimeout: %+v", res2)
	}
}

func TestAdvancedTracker_TimeDecayCallback(t *testing.T) {
	startTime := time.Date(2026, 10, 15, 10, 0, 0, 0, time.UTC)

	// 初始回调 5% (0.05)；满 1 小时后收紧为 2% (0.02)
	cfg := AdvancedTrackerConfig{
		Side:            strike.PositionSideLong,
		CallbackType:    strike.CallbackTypeRate,
		InitialCallback: decimal.NewFromFloat(0.05),
		StartTime:       startTime,
		DecayTiers: []TimeDecayTier{
			{AfterDuration: 1 * time.Hour, CallbackValue: decimal.NewFromFloat(0.02)},
		},
	}

	tracker, err := NewAdvancedTracker(cfg)
	if err != nil {
		t.Fatalf("failed to create tracker: %v", err)
	}

	// 10 分钟时，最高价 100，回调 5%，止损线 95
	res1, _ := tracker.Update(startTime.Add(10*time.Minute), decimal.NewFromFloat(100))
	if !res1.StopPrice.Equal(decimal.NewFromFloat(95)) {
		t.Errorf("expected stop 95, got %s", res1.StopPrice)
	}

	// 70 分钟时，价格仍为 100，触发时间衰减收紧 (2%)，止损线自动向上推进至 98！
	res2, _ := tracker.Update(startTime.Add(70*time.Minute), decimal.NewFromFloat(100))
	if !res2.StopPrice.Equal(decimal.NewFromFloat(98)) {
		t.Errorf("expected decayed stop 98, got %s", res2.StopPrice)
	}
	if !res2.ActiveCallback.Equal(decimal.NewFromFloat(0.02)) {
		t.Errorf("expected active callback 0.02, got %s", res2.ActiveCallback)
	}
}

func TestAdvancedTracker_Concurrency(t *testing.T) {
	startTime := time.Date(2026, 10, 15, 10, 0, 0, 0, time.UTC)
	cfg := AdvancedTrackerConfig{
		Side:            strike.PositionSideLong,
		CallbackType:    strike.CallbackTypeRate,
		InitialCallback: decimal.NewFromFloat(0.03),
		StartTime:       startTime,
	}

	tracker, err := NewAdvancedTracker(cfg)
	if err != nil {
		t.Fatalf("failed to create tracker: %v", err)
	}

	var wg sync.WaitGroup
	// 40 个并发任务推进快照与只读查询
	for i := 0; i < 40; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			_ = tracker.GetSnapshot()
		}(i)
	}
	wg.Wait()
}
