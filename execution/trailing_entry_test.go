package execution

import (
	"sync"
	"testing"
	"time"

	"github.com/YuanJey/go-strike"
	"github.com/shopspring/decimal"
)

func TestTrailingLongEntryFlow(t *testing.T) {
	startTime := time.Date(2026, 10, 15, 10, 0, 0, 0, time.UTC)

	// 做多追踪开仓：必须低于 100 允许启动，回调 2% 反弹买入
	cfg := TrailingEntryConfig{
		Side:          strike.PositionSideLong,
		TriggerPrice:  decimal.NewFromFloat(100),
		CallbackType:  strike.CallbackTypeRate,
		CallbackValue: decimal.NewFromFloat(0.02), // 2% 反弹
		StartTime:     startTime,
	}

	tracker, err := NewTrailingEntryTracker(cfg)
	if err != nil {
		t.Fatalf("failed to create tracker: %v", err)
	}

	// 1. 价格在 105 (高于 100)，不满足做多必须低于 100 的门槛 -> PENDING
	res1, err := tracker.Update(startTime.Add(1*time.Minute), decimal.NewFromFloat(105))
	if err != nil || res1.State != EntryStatePending {
		t.Fatalf("step 1 should be pending, got %+v", res1)
	}

	// 2. 价格跌到 98 (低于 100)，正式激活追踪！当前最低价 98，触发反弹线 98 * 1.02 = 99.96
	res2, err := tracker.Update(startTime.Add(2*time.Minute), decimal.NewFromFloat(98))
	if err != nil || res2.State != EntryStateTracking {
		t.Fatalf("step 2 should be tracking, got %+v", res2)
	}
	expectedTrigger1 := decimal.NewFromFloat(98).Mul(decimal.NewFromFloat(1.02))
	if !res2.CurrentTriggerPrice.Equal(expectedTrigger1) {
		t.Errorf("step 2 trigger line mismatch, expected %s, got %s", expectedTrigger1, res2.CurrentTriggerPrice)
	}

	// 3. 价格进一步探底至 90 (连续暴跌，不接飞刀！)，最低价刷新为 90，反弹线降至 90 * 1.02 = 91.8
	res3, err := tracker.Update(startTime.Add(3*time.Minute), decimal.NewFromFloat(90))
	if err != nil || res3.State != EntryStateTracking || res3.Triggered {
		t.Fatalf("step 3 should be tracking without trigger: %+v", res3)
	}
	expectedTrigger2 := decimal.NewFromFloat(91.8)
	if !res3.CurrentTriggerPrice.Equal(expectedTrigger2) {
		t.Errorf("step 3 trigger line mismatch, expected %s, got %s", expectedTrigger2, res3.CurrentTriggerPrice)
	}

	// 4. 价格轻微反弹到 91 (未达 91.8)，保持追踪
	res4, err := tracker.Update(startTime.Add(4*time.Minute), decimal.NewFromFloat(91))
	if err != nil || res4.Triggered {
		t.Fatalf("step 4 should not trigger: %+v", res4)
	}

	// 5. 价格强力反弹至 92 (突破 91.8 反弹线，且仍低于 100 上限) -> 正式触发做多开仓买入！
	res5, err := tracker.Update(startTime.Add(5*time.Minute), decimal.NewFromFloat(92))
	if err != nil || !res5.Triggered || res5.State != EntryStateTriggered {
		t.Fatalf("step 5 long entry should trigger: %+v", res5)
	}
	if res5.TargetOrderSide != strike.SideBuy {
		t.Errorf("expected target order side BUY, got %s", res5.TargetOrderSide)
	}
}

func TestTrailingShortEntryFlow(t *testing.T) {
	startTime := time.Date(2026, 10, 15, 10, 0, 0, 0, time.UTC)

	// 做空追踪开仓：必须高于 100 允许启动，冲高回落 2% 卖出
	cfg := TrailingEntryConfig{
		Side:          strike.PositionSideShort,
		TriggerPrice:  decimal.NewFromFloat(100),
		CallbackType:  strike.CallbackTypeRate,
		CallbackValue: decimal.NewFromFloat(0.02), // 2% 回落
		StartTime:     startTime,
	}

	tracker, err := NewTrailingEntryTracker(cfg)
	if err != nil {
		t.Fatalf("failed to create short tracker: %v", err)
	}

	// 1. 价格 95 (低于 100)，不满足做空必须高于 100 的门槛 -> PENDING
	res1, _ := tracker.Update(startTime.Add(1*time.Minute), decimal.NewFromFloat(95))
	if res1.State != EntryStatePending {
		t.Fatalf("step 1 should be pending: %+v", res1)
	}

	// 2. 价格冲高至 110 (高于 100)，进入追踪，最高价 110，回落触发线 110 * 0.98 = 107.8
	res2, _ := tracker.Update(startTime.Add(2*time.Minute), decimal.NewFromFloat(110))
	if res2.State != EntryStateTracking {
		t.Fatalf("step 2 should be tracking: %+v", res2)
	}

	// 3. 价格冲高至 120，最高价刷新至 120，回落触发线抬升至 120 * 0.98 = 117.6
	res3, _ := tracker.Update(startTime.Add(3*time.Minute), decimal.NewFromFloat(120))
	expectedTrigger := decimal.NewFromFloat(117.6)
	if !res3.CurrentTriggerPrice.Equal(expectedTrigger) {
		t.Errorf("expected trigger 117.6, got %s", res3.CurrentTriggerPrice)
	}

	// 4. 价格冲高回落至 117 (跌破 117.6，且仍高于 100 底线) -> 触发做空开仓卖出！
	res4, _ := tracker.Update(startTime.Add(4*time.Minute), decimal.NewFromFloat(117))
	if !res4.Triggered || res4.State != EntryStateTriggered {
		t.Fatalf("step 4 short entry should trigger: %+v", res4)
	}
	if res4.TargetOrderSide != strike.SideSell {
		t.Errorf("expected target order side SELL, got %s", res4.TargetOrderSide)
	}
}

func TestTrailingEntryTimeConditions(t *testing.T) {
	startTime := time.Date(2026, 10, 15, 10, 0, 0, 0, time.UTC)

	// 包含 ValidAfter (5分钟), MinDwellTime (2分钟), ExpireAfter (30分钟)
	cfg := TrailingEntryConfig{
		Side:          strike.PositionSideLong,
		TriggerPrice:  decimal.NewFromFloat(100),
		CallbackType:  strike.CallbackTypeRate,
		CallbackValue: decimal.NewFromFloat(0.01),
		StartTime:     startTime,
		ValidAfter:    5 * time.Minute,
		ExpireAfter:   30 * time.Minute,
		MinDwellTime:  2 * time.Minute,
	}

	tracker, err := NewTrailingEntryTracker(cfg)
	if err != nil {
		t.Fatalf("create error: %v", err)
	}

	// 1. 第 3 分钟：价格 95 (已低于 100)，但仍在 ValidAfter 5 分钟准备期内 -> PENDING
	res1, _ := tracker.Update(startTime.Add(3*time.Minute), decimal.NewFromFloat(95))
	if res1.State != EntryStatePending {
		t.Errorf("res1 should be pending during validAfter: %+v", res1)
	}

	// 2. 第 6 分钟：价格 95，ValidAfter 已过，首次进入价格门槛，MinDwellTime 计时开始 -> PENDING
	res2, _ := tracker.Update(startTime.Add(6*time.Minute), decimal.NewFromFloat(95))
	if res2.State != EntryStatePending {
		t.Errorf("res2 should be pending during minDwellTime: %+v", res2)
	}

	// 3. 第 8 分钟 (驻留满 2 分钟) -> 正式进入 TRACKING
	res3, _ := tracker.Update(startTime.Add(8*time.Minute), decimal.NewFromFloat(95))
	if res3.State != EntryStateTracking {
		t.Errorf("res3 should be tracking after dwell: %+v", res3)
	}

	// 4. 第 31 分钟：超过 ExpireAfter 30 分钟 -> EXPIRED
	res4, _ := tracker.Update(startTime.Add(31*time.Minute), decimal.NewFromFloat(95))
	if res4.State != EntryStateExpired {
		t.Errorf("res4 should be expired after 31min: %+v", res4)
	}
}

func TestTrailingEntryConcurrency(t *testing.T) {
	startTime := time.Date(2026, 10, 15, 10, 0, 0, 0, time.UTC)
	cfg := TrailingEntryConfig{
		Side:          strike.PositionSideLong,
		TriggerPrice:  decimal.NewFromFloat(100),
		CallbackType:  strike.CallbackTypeRate,
		CallbackValue: decimal.NewFromFloat(0.02),
		StartTime:     startTime,
	}

	tracker, err := NewTrailingEntryTracker(cfg)
	if err != nil {
		t.Fatalf("create error: %v", err)
	}

	var wg sync.WaitGroup
	for i := 0; i < 40; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			_ = tracker.GetSnapshot()
		}(i)
	}
	wg.Wait()
}
