package trailing

import (
	"errors"
	"sort"
	"sync"
	"time"

	"github.com/YuanJey/go-strike"
	"github.com/shopspring/decimal"
)

// TriggerReason 描述触发平仓/防守的业务原因
type TriggerReason string

const (
	TriggerReasonNone          TriggerReason = "NONE"
	TriggerReasonTrailing      TriggerReason = "TRAILING"       // 移动止盈止损线回调击穿
	TriggerReasonPriceBoundary TriggerReason = "PRICE_BOUNDARY" // 触发硬价格边界 (多头低于下界/空头高于上界)
	TriggerReasonTimeout       TriggerReason = "TIMEOUT"        // 超过最大持仓时限触发退出
)

var (
	ErrInvalidTimeSequence = errors.New("trailing: 行情时间戳不能倒流")
)

// TimeDecayTier 时间衰减档位：持仓达到指定时长后，动态收紧回调阈值
type TimeDecayTier struct {
	AfterDuration time.Duration   // 达到或超过此持仓时长
	CallbackValue decimal.Decimal // 切换采用的收紧回调参数 (更小，锁住更多浮盈)
}

// AdvancedTrackerConfig 高级时空双重约束移动止盈止损配置
type AdvancedTrackerConfig struct {
	Side            strike.PositionSide         // 持仓方向：Long 或 Short
	CallbackType    strike.TrailingCallbackType // 回调类型：Rate 或 Distance
	InitialCallback decimal.Decimal             // 初始回调数值 (如 0.03 代表 3%)
	ActivationPrice decimal.Decimal             // 动态激活价格 (可选，为 0 时立即激活)

	// 价格条件: 做多低于该价格触发防守，做空高于该价格触发防守
	LowerBoundPrice decimal.Decimal // 多头绝对保护下界：Price <= LowerBoundPrice 触发防守 (硬止损)
	UpperBoundPrice decimal.Decimal // 空头绝对保护上界：Price >= UpperBoundPrice 触发防守 (硬止损)

	// 时间条件:
	StartTime          time.Time       // 开仓入场时间
	CoolingOffDuration time.Duration   // 入场防震荡冷却期 (此期间内仅硬边界生效，不触发移动止损)
	MaxHoldDuration    time.Duration   // 最大持仓容忍时长 (可选，超时直接平仓，为 0 则不限制)
	DecayTiers         []TimeDecayTier // 随时间衰减的回调档位序列 (可选)
}

// AdvancedUpdateResult 单次时间与行情 Tick 推进后的计算结果
type AdvancedUpdateResult struct {
	CurrentTime      time.Time       // 当前推进的行情时间
	CurrentPrice     decimal.Decimal // 当前价格
	IsActivated      bool            // 是否已激活移动追踪
	IsTriggered      bool            // 是否触发平仓防守
	Reason           TriggerReason   // 触发平仓的具体归因
	StopPrice        decimal.Decimal // 当前综合有效止损/止盈线
	ExtremePrice     decimal.Decimal // 历史极值价
	ActiveCallback   decimal.Decimal // 当前生效的回调值
	PriceRatchetMove bool            // 止损线是否单向推进
	InCoolingOff     bool            // 当前是否处于入场冷却期
}

// AdvancedTracker 融合时间生命周期与多空价格边界的高级移动止盈止损状态机
type AdvancedTracker struct {
	mu sync.RWMutex

	cfg              AdvancedTrackerConfig
	isActivated      bool
	isTriggered      bool
	triggerReason    TriggerReason
	lastTime         time.Time
	extremePrice     decimal.Decimal
	currentStopPrice decimal.Decimal
	activeCallback   decimal.Decimal
}

// NewAdvancedTracker 构建高级追踪器并校验配置合法性
func NewAdvancedTracker(cfg AdvancedTrackerConfig) (*AdvancedTracker, error) {
	if cfg.Side != strike.PositionSideLong && cfg.Side != strike.PositionSideShort {
		return nil, strike.ErrorWrap(strike.ErrInvalidStopLossPrice, "持仓方向仅支持 Long 或 Short")
	}

	switch cfg.CallbackType {
	case strike.CallbackTypeRate:
		if cfg.InitialCallback.LessThanOrEqual(decimal.Zero) || cfg.InitialCallback.GreaterThanOrEqual(decimal.NewFromInt(1)) {
			return nil, strike.ErrInvalidCallbackRate
		}
	case strike.CallbackTypeDistance:
		if cfg.InitialCallback.LessThanOrEqual(decimal.Zero) {
			return nil, strike.ErrInvalidDistance
		}
	default:
		return nil, strike.ErrInvalidCallbackRate
	}

	// 价格边界合理性校验
	if cfg.Side == strike.PositionSideLong {
		if !cfg.LowerBoundPrice.IsZero() && !cfg.ActivationPrice.IsZero() {
			if cfg.LowerBoundPrice.GreaterThan(cfg.ActivationPrice) {
				return nil, strike.ErrorWrap(strike.ErrInvalidPrice, "多头保护下界必须低于激活目标价")
			}
		}
	} else {
		if !cfg.UpperBoundPrice.IsZero() && !cfg.ActivationPrice.IsZero() {
			if cfg.UpperBoundPrice.LessThan(cfg.ActivationPrice) {
				return nil, strike.ErrorWrap(strike.ErrInvalidPrice, "空头保护上界必须高于激活目标价")
			}
		}
	}

	// 按照 AfterDuration 从大到小排序时间衰减梯度，方便后续降序匹配
	if len(cfg.DecayTiers) > 0 {
		sort.Slice(cfg.DecayTiers, func(i, j int) bool {
			return cfg.DecayTiers[i].AfterDuration > cfg.DecayTiers[j].AfterDuration
		})
	}

	tr := &AdvancedTracker{
		cfg:            cfg,
		activeCallback: cfg.InitialCallback,
		lastTime:       cfg.StartTime,
		triggerReason:  TriggerReasonNone,
	}

	// 若未指定激活价格，或者激活价格为 0，则初始化即进入激活追踪状态
	if cfg.ActivationPrice.LessThanOrEqual(decimal.Zero) {
		tr.isActivated = true
	}

	return tr, nil
}

// Update 接收撮合时间戳与最新价格，驱动时空状态机
func (t *AdvancedTracker) Update(currentTime time.Time, currentPrice decimal.Decimal) (AdvancedUpdateResult, error) {
	if currentPrice.LessThanOrEqual(decimal.Zero) {
		return AdvancedUpdateResult{}, strike.ErrInvalidPrice
	}

	t.mu.Lock()
	defer t.mu.Unlock()

	// 时间单调性校验
	if !t.lastTime.IsZero() && currentTime.Before(t.lastTime) {
		return AdvancedUpdateResult{}, ErrInvalidTimeSequence
	}
	t.lastTime = currentTime

	elapsed := currentTime.Sub(t.cfg.StartTime)
	inCoolingOff := t.cfg.CoolingOffDuration > 0 && elapsed < t.cfg.CoolingOffDuration

	res := AdvancedUpdateResult{
		CurrentTime:    currentTime,
		CurrentPrice:   currentPrice,
		IsActivated:    t.isActivated,
		IsTriggered:    t.isTriggered,
		Reason:         t.triggerReason,
		StopPrice:      t.currentStopPrice,
		ExtremePrice:   t.extremePrice,
		ActiveCallback: t.activeCallback,
		InCoolingOff:   inCoolingOff,
	}

	if t.isTriggered {
		return res, nil
	}

	// ==========================================
	// 1. 价格条件硬边界检查 (无视冷却期，绝对安全兜底)
	// ==========================================
	if t.cfg.Side == strike.PositionSideLong {
		// 做多：价格低于等于保护下界，必须立刻平仓止损
		if !t.cfg.LowerBoundPrice.IsZero() && currentPrice.LessThanOrEqual(t.cfg.LowerBoundPrice) {
			t.isTriggered = true
			t.triggerReason = TriggerReasonPriceBoundary
			res.IsTriggered = true
			res.Reason = TriggerReasonPriceBoundary
			return res, nil
		}
	} else {
		// 做空：价格高于等于保护上界，必须立刻平仓止损
		if !t.cfg.UpperBoundPrice.IsZero() && currentPrice.GreaterThanOrEqual(t.cfg.UpperBoundPrice) {
			t.isTriggered = true
			t.triggerReason = TriggerReasonPriceBoundary
			res.IsTriggered = true
			res.Reason = TriggerReasonPriceBoundary
			return res, nil
		}
	}

	// ==========================================
	// 2. 时间条件超时检查 (MaxHoldDuration)
	// ==========================================
	if t.cfg.MaxHoldDuration > 0 && elapsed >= t.cfg.MaxHoldDuration {
		t.isTriggered = true
		t.triggerReason = TriggerReasonTimeout
		res.IsTriggered = true
		res.Reason = TriggerReasonTimeout
		return res, nil
	}

	// ==========================================
	// 3. 计算当前生效的回调数值 (时间衰减梯度)
	// ==========================================
	currentCallback := t.cfg.InitialCallback
	for _, tier := range t.cfg.DecayTiers {
		if elapsed >= tier.AfterDuration {
			currentCallback = tier.CallbackValue
			break
		}
	}
	t.activeCallback = currentCallback
	res.ActiveCallback = currentCallback

	// ==========================================
	// 4. 激活状态判断
	// ==========================================
	if !t.isActivated {
		activated := false
		if t.cfg.Side == strike.PositionSideLong {
			if currentPrice.GreaterThanOrEqual(t.cfg.ActivationPrice) {
				activated = true
			}
		} else {
			if currentPrice.LessThanOrEqual(t.cfg.ActivationPrice) {
				activated = true
			}
		}

		if !activated {
			// 未激活状态下的有效止损线即为硬边界
			if t.cfg.Side == strike.PositionSideLong {
				res.StopPrice = t.cfg.LowerBoundPrice
			} else {
				res.StopPrice = t.cfg.UpperBoundPrice
			}
			return res, nil
		}

		// 触发激活
		t.isActivated = true
		res.IsActivated = true
		t.extremePrice = currentPrice
		res.ExtremePrice = currentPrice

		stop, err := CalculateStopPrice(t.cfg.Side, t.extremePrice, t.activeCallback, t.cfg.CallbackType)
		if err != nil {
			return res, err
		}

		t.currentStopPrice = t.ratchet(t.currentStopPrice, stop)
		res.StopPrice = t.currentStopPrice
		res.PriceRatchetMove = true
		return res, nil
	}

	// ==========================================
	// 5. 处于已激活状态：极值刷新与单向棘轮推进
	// ==========================================
	if t.cfg.Side == strike.PositionSideLong {
		if t.extremePrice.IsZero() || currentPrice.GreaterThan(t.extremePrice) {
			t.extremePrice = currentPrice
		}
	} else {
		if t.extremePrice.IsZero() || currentPrice.LessThan(t.extremePrice) {
			t.extremePrice = currentPrice
		}
	}
	res.ExtremePrice = t.extremePrice

	// 如果极值移动，或者时间衰减导致回调率变小，则重新计算移动止损线
	newStop, err := CalculateStopPrice(t.cfg.Side, t.extremePrice, t.activeCallback, t.cfg.CallbackType)
	if err != nil {
		return res, err
	}

	oldStop := t.currentStopPrice
	t.currentStopPrice = t.ratchet(t.currentStopPrice, newStop)
	if !t.currentStopPrice.Equal(oldStop) {
		res.PriceRatchetMove = true
	}
	res.StopPrice = t.currentStopPrice

	// ==========================================
	// 6. 移动止损触发判定 (冷却期内不因微震触发移动平仓)
	// ==========================================
	if !inCoolingOff {
		if IsTriggered(t.cfg.Side, t.currentStopPrice, currentPrice) {
			t.isTriggered = true
			t.triggerReason = TriggerReasonTrailing
			res.IsTriggered = true
			res.Reason = TriggerReasonTrailing
			return res, nil
		}
	}

	return res, nil
}

// ratchet 保持单向棘轮推进，并与硬边界对齐
func (t *AdvancedTracker) ratchet(oldStop, newStop decimal.Decimal) decimal.Decimal {
	if t.cfg.Side == strike.PositionSideLong {
		// 多头：只升不降，且不能低于保底下限
		target := newStop
		if target.LessThan(t.cfg.LowerBoundPrice) {
			target = t.cfg.LowerBoundPrice
		}
		if oldStop.IsZero() || target.GreaterThan(oldStop) {
			return target
		}
		return oldStop
	}

	// 空头：只降不升，且不能高于保底上限
	target := newStop
	if !t.cfg.UpperBoundPrice.IsZero() && target.GreaterThan(t.cfg.UpperBoundPrice) {
		target = t.cfg.UpperBoundPrice
	}
	if oldStop.IsZero() || target.LessThan(oldStop) {
		return target
	}
	return oldStop
}

// GetSnapshot 并发安全快照
func (t *AdvancedTracker) GetSnapshot() AdvancedUpdateResult {
	t.mu.RLock()
	defer t.mu.RUnlock()

	return AdvancedUpdateResult{
		CurrentTime:    t.lastTime,
		IsActivated:    t.isActivated,
		IsTriggered:    t.isTriggered,
		Reason:         t.triggerReason,
		StopPrice:      t.currentStopPrice,
		ExtremePrice:   t.extremePrice,
		ActiveCallback: t.activeCallback,
	}
}
