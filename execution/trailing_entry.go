package execution

import (
	"errors"
	"sync"
	"time"

	"github.com/YuanJey/go-strike"
	"github.com/shopspring/decimal"
)

// TrailingEntryState 移动追踪开仓状态机生命周期
type TrailingEntryState string

const (
	// EntryStatePending 处于延时等待、价格未进入目标区间或观察期未满
	EntryStatePending TrailingEntryState = "PENDING"
	// EntryStateTracking 价格已满足入场门槛，正在持续追踪阶段极值（寻底或摸顶）
	EntryStateTracking TrailingEntryState = "TRACKING"
	// EntryStateTriggered 极值点反弹或回落达到回调幅度，正式触发开仓信号
	EntryStateTriggered TrailingEntryState = "TRIGGERED"
	// EntryStateExpired 超出最大允许开仓时间窗口，状态机失效
	EntryStateExpired TrailingEntryState = "EXPIRED"
)

var (
	ErrInvalidEntrySide    = errors.New("execution: 追踪开仓方向必须为 Long (做多) 或 Short (做空)")
	ErrTimeSequenceReveral = errors.New("execution: 行情时间戳不能倒流")
)

// TrailingEntryConfig 移动追踪开仓参数配置
type TrailingEntryConfig struct {
	Side          strike.PositionSide         // Long: 逢跌反弹做多开仓; Short: 逢涨回落做空开仓
	TriggerPrice  decimal.Decimal             // 价格门槛：做多市价必须低于该价格；做空市价必须高于该价格
	CallbackType  strike.TrailingCallbackType // 回调类型：Rate (百分比) 或 Distance (固定价差)
	CallbackValue decimal.Decimal             // 反弹/回落判定阈值

	// 时间约束条件
	StartTime    time.Time     // 策略启动时间戳
	ValidAfter   time.Duration // 生效准备缓冲时长 (可选，在此时间内保持 Pending)
	ExpireAfter  time.Duration // 最长追踪有效窗口 (可选，超时未触发则置为 Expired，为 0 时永不超时)
	MinDwellTime time.Duration // 最小驻留时间 (可选，价格进入门槛区间后需持续维持的最小时长，防瞬时插针伪突破)
}

// TrailingEntryResult 单次更新返回的开仓决策结果
type TrailingEntryResult struct {
	State               TrailingEntryState // 当前状态
	Triggered           bool               // 本次 Tick 是否触发开仓下单
	CurrentTime         time.Time          // 当前行情时间戳
	CurrentPrice        decimal.Decimal    // 当前行情价格
	TargetOrderSide     strike.Side        // 触发开仓时的报单方向 (SideBuy 做多 / SideSell 做空)
	ExtremePrice        decimal.Decimal    // 追踪期内的极值价格 (做多最低价 LWM / 做空最高价 HWM)
	CurrentTriggerPrice decimal.Decimal    // 当前实时计算的反弹/回落开仓触发线
	Elapsed             time.Duration      // 自策略启动以来的累计耗时
}

// TrailingEntryTracker 高性能并发安全移动追踪开仓跟踪器
type TrailingEntryTracker struct {
	mu sync.RWMutex

	cfg                 TrailingEntryConfig
	state               TrailingEntryState
	lastTime            time.Time
	dwellStartTime      time.Time       // 首次进入目标价格区间的时间
	extremePrice        decimal.Decimal // 极值价 (多头 LWM / 空头 HWM)
	currentTriggerPrice decimal.Decimal // 动态触发线
	targetOrderSide     strike.Side
}

// NewTrailingEntryTracker 创建并校验移动追踪开仓实例
func NewTrailingEntryTracker(cfg TrailingEntryConfig) (*TrailingEntryTracker, error) {
	if cfg.Side != strike.PositionSideLong && cfg.Side != strike.PositionSideShort {
		return nil, ErrInvalidEntrySide
	}
	if cfg.TriggerPrice.LessThanOrEqual(decimal.Zero) {
		return nil, strike.ErrInvalidPrice
	}

	switch cfg.CallbackType {
	case strike.CallbackTypeRate:
		if cfg.CallbackValue.LessThanOrEqual(decimal.Zero) || cfg.CallbackValue.GreaterThanOrEqual(decimal.NewFromInt(1)) {
			return nil, strike.ErrInvalidCallbackRate
		}
	case strike.CallbackTypeDistance:
		if cfg.CallbackValue.LessThanOrEqual(decimal.Zero) {
			return nil, strike.ErrInvalidDistance
		}
	default:
		return nil, strike.ErrInvalidCallbackRate
	}

	targetSide := strike.SideBuy
	if cfg.Side == strike.PositionSideShort {
		targetSide = strike.SideSell
	}

	return &TrailingEntryTracker{
		cfg:             cfg,
		state:           EntryStatePending,
		lastTime:        cfg.StartTime,
		targetOrderSide: targetSide,
	}, nil
}

// Update 接收最新撮合时间与行情价格，推进追踪开仓状态机
func (t *TrailingEntryTracker) Update(currentTime time.Time, currentPrice decimal.Decimal) (TrailingEntryResult, error) {
	if currentPrice.LessThanOrEqual(decimal.Zero) {
		return TrailingEntryResult{}, strike.ErrInvalidPrice
	}

	t.mu.Lock()
	defer t.mu.Unlock()

	// 时间单调性校验
	if !t.lastTime.IsZero() && currentTime.Before(t.lastTime) {
		return TrailingEntryResult{}, ErrTimeSequenceReveral
	}
	t.lastTime = currentTime

	elapsed := currentTime.Sub(t.cfg.StartTime)

	res := TrailingEntryResult{
		State:               t.state,
		Triggered:           false,
		CurrentTime:         currentTime,
		CurrentPrice:        currentPrice,
		TargetOrderSide:     t.targetOrderSide,
		ExtremePrice:        t.extremePrice,
		CurrentTriggerPrice: t.currentTriggerPrice,
		Elapsed:             elapsed,
	}

	// 终止态判定：已触发或已超时则直接返回快照
	if t.state == EntryStateTriggered || t.state == EntryStateExpired {
		return res, nil
	}

	// 1. 超时时间判定 (ExpireAfter)
	if t.cfg.ExpireAfter > 0 && elapsed >= t.cfg.ExpireAfter {
		t.state = EntryStateExpired
		res.State = EntryStateExpired
		return res, nil
	}

	// 2. 准备期判定 (ValidAfter)
	if t.cfg.ValidAfter > 0 && elapsed < t.cfg.ValidAfter {
		t.state = EntryStatePending
		res.State = EntryStatePending
		return res, nil
	}

	// 3. 价格条件区间判定
	inRange := false
	if t.cfg.Side == strike.PositionSideLong {
		// 做多开仓必须低于设定价格门槛
		if currentPrice.LessThan(t.cfg.TriggerPrice) {
			inRange = true
		}
	} else {
		// 做空开仓必须高于设定价格门槛
		if currentPrice.GreaterThan(t.cfg.TriggerPrice) {
			inRange = true
		}
	}

	if !inRange {
		// 未进入价格门槛，重置驻留计时
		t.dwellStartTime = time.Time{}
		// 若此前在追踪中但价格被拉出安全区间，退回 Pending 保护
		if t.state == EntryStateTracking {
			t.state = EntryStatePending
		}
		res.State = t.state
		return res, nil
	}

	// 4. 最小驻留时间判定 (MinDwellTime)
	if t.cfg.MinDwellTime > 0 {
		if t.dwellStartTime.IsZero() {
			t.dwellStartTime = currentTime
		}
		if currentTime.Sub(t.dwellStartTime) < t.cfg.MinDwellTime {
			t.state = EntryStatePending
			res.State = EntryStatePending
			return res, nil
		}
	}

	// 5. 正式进入/保持 TRACKING 状态
	t.state = EntryStateTracking
	res.State = EntryStateTracking

	// 6. 极值更新与反弹/回落开仓判定
	if t.cfg.Side == strike.PositionSideLong {
		// 做多逻辑：寻底 (记录最低价 LWM)，反弹触发买入
		if t.extremePrice.IsZero() || currentPrice.LessThan(t.extremePrice) {
			t.extremePrice = currentPrice
			// 重新计算做多反弹开仓触发线：LWM + Callback
			t.currentTriggerPrice = t.calcTriggerPrice(t.extremePrice)
		}
		res.ExtremePrice = t.extremePrice
		res.CurrentTriggerPrice = t.currentTriggerPrice

		// 判定反弹触发：市价回升达到或突破触发线
		if currentPrice.GreaterThanOrEqual(t.currentTriggerPrice) {
			// 确保触发时依然未高于上限门槛
			if currentPrice.LessThanOrEqual(t.cfg.TriggerPrice) {
				t.state = EntryStateTriggered
				res.State = EntryStateTriggered
				res.Triggered = true
				return res, nil
			}
		}

	} else {
		// 做空逻辑：摸顶 (记录最高价 HWM)，回落触发卖出
		if t.extremePrice.IsZero() || currentPrice.GreaterThan(t.extremePrice) {
			t.extremePrice = currentPrice
			// 重新计算做空回落开仓触发线：HWM - Callback
			t.currentTriggerPrice = t.calcTriggerPrice(t.extremePrice)
		}
		res.ExtremePrice = t.extremePrice
		res.CurrentTriggerPrice = t.currentTriggerPrice

		// 判定回落触发：市价下跌达到或跌穿触发线
		if currentPrice.LessThanOrEqual(t.currentTriggerPrice) {
			// 确保触发时依然未低于下限门槛
			if currentPrice.GreaterThanOrEqual(t.cfg.TriggerPrice) {
				t.state = EntryStateTriggered
				res.State = EntryStateTriggered
				res.Triggered = true
				return res, nil
			}
		}
	}

	return res, nil
}

// calcTriggerPrice 根据极值与回调模式计算开仓触发线
func (t *TrailingEntryTracker) calcTriggerPrice(extreme decimal.Decimal) decimal.Decimal {
	one := decimal.NewFromInt(1)
	if t.cfg.Side == strike.PositionSideLong {
		// 做多反弹线: LWM * (1 + Rate) 或 LWM + Distance
		if t.cfg.CallbackType == strike.CallbackTypeRate {
			return extreme.Mul(one.Add(t.cfg.CallbackValue))
		}
		return extreme.Add(t.cfg.CallbackValue)
	}

	// 做空回落线: HWM * (1 - Rate) 或 HWM - Distance
	if t.cfg.CallbackType == strike.CallbackTypeRate {
		return extreme.Mul(one.Sub(t.cfg.CallbackValue))
	}
	return extreme.Sub(t.cfg.CallbackValue)
}

// GetSnapshot 获取追踪开仓当前状态快照（并发安全）
func (t *TrailingEntryTracker) GetSnapshot() TrailingEntryResult {
	t.mu.RLock()
	defer t.mu.RUnlock()

	return TrailingEntryResult{
		State:               t.state,
		TargetOrderSide:     t.targetOrderSide,
		ExtremePrice:        t.extremePrice,
		CurrentTriggerPrice: t.currentTriggerPrice,
	}
}

// Reset 重置跟踪器状态
func (t *TrailingEntryTracker) Reset() {
	t.mu.Lock()
	defer t.mu.Unlock()

	t.state = EntryStatePending
	t.extremePrice = decimal.Zero
	t.currentTriggerPrice = decimal.Zero
	t.dwellStartTime = time.Time{}
}
