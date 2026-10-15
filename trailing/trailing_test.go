package trailing

import (
	"sync"
	"testing"

	"github.com/YuanJey/go-strike"
	"github.com/shopspring/decimal"
)

func TestCalculateStopPrice(t *testing.T) {
	// 多头百分比回撤 2%
	hwm := decimal.NewFromFloat(100.0)
	rate := decimal.NewFromFloat(0.02)
	stopLong, err := CalculateStopPrice(strike.PositionSideLong, hwm, rate, strike.CallbackTypeRate)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	expectedLong := decimal.NewFromFloat(98.0)
	if !stopLong.Equal(expectedLong) {
		t.Errorf("expected %s, got %s", expectedLong, stopLong)
	}

	// 空头百分比反弹 2%
	lwm := decimal.NewFromFloat(100.0)
	stopShort, err := CalculateStopPrice(strike.PositionSideShort, lwm, rate, strike.CallbackTypeRate)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	expectedShort := decimal.NewFromFloat(102.0)
	if !stopShort.Equal(expectedShort) {
		t.Errorf("expected %s, got %s", expectedShort, stopShort)
	}

	// 固定点数模式
	dist := decimal.NewFromFloat(5.0)
	stopDist, err := CalculateStopPrice(strike.PositionSideLong, hwm, dist, strike.CallbackTypeDistance)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	expectedDist := decimal.NewFromFloat(95.0)
	if !stopDist.Equal(expectedDist) {
		t.Errorf("expected %s, got %s", expectedDist, stopDist)
	}
}

func TestLongTrackerFlow(t *testing.T) {
	// 多头开仓价 100，激活价 110，初始止损 95，回调 5%
	cfg := TrackerConfig{
		Side:             strike.PositionSideLong,
		CallbackType:     strike.CallbackTypeRate,
		CallbackValue:    decimal.NewFromFloat(0.05), // 5%
		ActivationPrice:  decimal.NewFromFloat(110.0),
		InitialStopPrice: decimal.NewFromFloat(95.0),
	}

	tracker, err := NewTracker(cfg)
	if err != nil {
		t.Fatalf("failed to create tracker: %v", err)
	}

	// 1. 价格在 105，未到激活价 110，未触及止损 95
	res, err := tracker.Update(decimal.NewFromFloat(105.0))
	if err != nil || res.IsActivated || res.IsTriggered {
		t.Errorf("step 1 failed, res: %+v", res)
	}

	// 2. 价格跌到 94，应触发初始止损
	resStop, err := tracker.Update(decimal.NewFromFloat(94.0))
	if err != nil || !resStop.IsTriggered {
		t.Errorf("step 2 initial stop loss should trigger, res: %+v", resStop)
	}

	// 重置追踪器测试正常激活流程
	tracker.Reset()

	// 3. 价格涨到 110，触发激活！
	// 止损线应为 110 * 0.95 = 104.5
	resAct, err := tracker.Update(decimal.NewFromFloat(110.0))
	if err != nil || !resAct.IsActivated || resAct.IsTriggered {
		t.Fatalf("step 3 activation failed, res: %+v", resAct)
	}
	expectedStop := decimal.NewFromFloat(104.5)
	if !resAct.StopPrice.Equal(expectedStop) {
		t.Errorf("expected stop %s, got %s", expectedStop, resAct.StopPrice)
	}

	// 4. 价格进一步上涨至 120 (创新高)，止损线抬高至 120 * 0.95 = 114
	resHigh, err := tracker.Update(decimal.NewFromFloat(120.0))
	if err != nil || !resHigh.IsActivated {
		t.Fatalf("step 4 failed: %v", err)
	}
	expectedStop2 := decimal.NewFromFloat(114.0)
	if !resHigh.StopPrice.Equal(expectedStop2) {
		t.Errorf("expected stop %s, got %s", expectedStop2, resHigh.StopPrice)
	}

	// 5. 价格小幅回落至 116 (未击穿 114)，止损线必须保持 114 (单向棘轮，不能回退)
	resPullback, err := tracker.Update(decimal.NewFromFloat(116.0))
	if err != nil || resPullback.IsTriggered {
		t.Fatalf("step 5 pullback should not trigger: %v", err)
	}
	if !resPullback.StopPrice.Equal(expectedStop2) {
		t.Errorf("ratchet failed! stop should remain %s, got %s", expectedStop2, resPullback.StopPrice)
	}

	// 6. 价格击穿止损线至 113.5 -> 触发移动止损平仓！
	resExit, err := tracker.Update(decimal.NewFromFloat(113.5))
	if err != nil || !resExit.IsTriggered {
		t.Fatalf("step 6 exit should trigger: %+v", resExit)
	}
}

func TestShortTrackerFlow(t *testing.T) {
	// 空头追踪止盈：激活价 90，回调 5%
	cfg := TrackerConfig{
		Side:            strike.PositionSideShort,
		CallbackType:    strike.CallbackTypeRate,
		CallbackValue:   decimal.NewFromFloat(0.05),
		ActivationPrice: decimal.NewFromFloat(90.0),
	}

	tracker, err := NewTracker(cfg)
	if err != nil {
		t.Fatalf("create short tracker error: %v", err)
	}

	// 价格跌到 90，激活，当前最低 90，止损价 = 90 * 1.05 = 94.5
	res, err := tracker.Update(decimal.NewFromFloat(90.0))
	if err != nil || !res.IsActivated {
		t.Fatalf("activation failed: %+v", res)
	}
	if !res.StopPrice.Equal(decimal.NewFromFloat(94.5)) {
		t.Errorf("expected 94.5, got %s", res.StopPrice)
	}

	// 价格创新低至 80，止损线降低至 80 * 1.05 = 84
	res2, err := tracker.Update(decimal.NewFromFloat(80.0))
	if err != nil || !res2.StopPrice.Equal(decimal.NewFromFloat(84.0)) {
		t.Errorf("expected 84.0, got %s", res2.StopPrice)
	}

	// 价格反弹至 84.1 -> 击穿止损线，触发平仓
	res3, err := tracker.Update(decimal.NewFromFloat(84.1))
	if err != nil || !res3.IsTriggered {
		t.Errorf("short exit should trigger: %+v", res3)
	}
}

func TestTrackerConcurrency(t *testing.T) {
	cfg := TrackerConfig{
		Side:          strike.PositionSideLong,
		CallbackType:  strike.CallbackTypeRate,
		CallbackValue: decimal.NewFromFloat(0.02),
	}
	tracker, err := NewTracker(cfg)
	if err != nil {
		t.Fatalf("tracker create error: %v", err)
	}

	var wg sync.WaitGroup
	// 50 个并发 goroutine 模拟行情并发推送
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(val float64) {
			defer wg.Done()
			_, _ = tracker.Update(decimal.NewFromFloat(val))
			_ = tracker.GetSnapshot()
		}(100.0 + float64(i))
	}
	wg.Wait()
}

func TestChandelierExit(t *testing.T) {
	bars := []Bar{
		{High: decimal.NewFromFloat(105), Low: decimal.NewFromFloat(95), Close: decimal.NewFromFloat(100)},
		{High: decimal.NewFromFloat(108), Low: decimal.NewFromFloat(98), Close: decimal.NewFromFloat(106)},
		{High: decimal.NewFromFloat(112), Low: decimal.NewFromFloat(102), Close: decimal.NewFromFloat(110)},
	}

	atr, err := CalculateATR(bars, 2)
	if err != nil {
		t.Fatalf("atr calculation error: %v", err)
	}
	if atr.LessThanOrEqual(decimal.Zero) {
		t.Errorf("atr must be > 0, got %s", atr)
	}

	stop, err := ChandelierStop(strike.PositionSideLong, decimal.NewFromFloat(112), atr, decimal.NewFromFloat(2))
	if err != nil {
		t.Fatalf("chandelier stop error: %v", err)
	}
	if stop.GreaterThanOrEqual(decimal.NewFromFloat(112)) {
		t.Errorf("long chandelier stop should be below extreme price, got %s", stop)
	}
}
