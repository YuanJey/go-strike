package trailing

import (
	"sync"

	"github.com/YuanJey/go-strike"
	"github.com/shopspring/decimal"
)

// TrackerConfig 移动止盈止损追踪器配置
type TrackerConfig struct {
	Side             strike.PositionSide         // 持仓方向：Long 或 Short
	CallbackType     strike.TrailingCallbackType // 回调类型：Rate 或 Distance
	CallbackValue    decimal.Decimal             // 回调数值（比例或固定价差）
	ActivationPrice  decimal.Decimal             // 激活价格（可选，为 0 时立即激活）
	InitialStopPrice decimal.Decimal             // 初始保底止损价（可选，在追踪激活前起到传统硬止损保护作用）
}

// UpdateResult 单次行情 Tick 推进后的计算结果
type UpdateResult struct {
	IsActivated      bool            // 是否已激活追踪
	IsTriggered      bool            // 是否触发平仓信号
	CurrentPrice     decimal.Decimal // 当前传入价格
	StopPrice        decimal.Decimal // 当前有效止盈/止损线
	ExtremePrice     decimal.Decimal // 历史极值价（多头最高价，空头最低价）
	PriceRatchetMove bool            // 本次止损线是否单向抬高/降低（棘轮推进）
}

// Tracker 有状态高并发移动止盈止损跟踪器
type Tracker struct {
	mu sync.RWMutex

	cfg              TrackerConfig
	isActivated      bool
	extremePrice     decimal.Decimal
	currentStopPrice decimal.Decimal
	isTriggered      bool
}

// NewTracker 创建并校验追踪器实例
func NewTracker(cfg TrackerConfig) (*Tracker, error) {
	if cfg.Side != strike.PositionSideLong && cfg.Side != strike.PositionSideShort {
		return nil, strike.ErrorWrap(strike.ErrInvalidStopLossPrice, "持仓方向仅支持 Long 或 Short")
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

	tr := &Tracker{
		cfg:              cfg,
		currentStopPrice: cfg.InitialStopPrice,
	}

	// 若未指定激活价格，或者激活价格为 0，则初始化即进入激活追踪状态
	if cfg.ActivationPrice.LessThanOrEqual(decimal.Zero) {
		tr.isActivated = true
	}

	return tr, nil
}

// Update 接收最新行情 Tick 价格，驱动状态机更新并判定平仓
func (t *Tracker) Update(currentPrice decimal.Decimal) (UpdateResult, error) {
	if currentPrice.LessThanOrEqual(decimal.Zero) {
		return UpdateResult{}, strike.ErrInvalidPrice
	}

	t.mu.Lock()
	defer t.mu.Unlock()

	res := UpdateResult{
		CurrentPrice: currentPrice,
		IsActivated:  t.isActivated,
		IsTriggered:  t.isTriggered,
		StopPrice:    t.currentStopPrice,
		ExtremePrice: t.extremePrice,
	}

	// 已触发则无需重复计算
	if t.isTriggered {
		res.IsTriggered = true
		return res, nil
	}

	// 1. 状态判断：尚未激活追踪
	if !t.isActivated {
		// 先判定是否触碰初始保底止损价
		if !t.cfg.InitialStopPrice.IsZero() {
			if IsTriggered(t.cfg.Side, t.cfg.InitialStopPrice, currentPrice) {
				t.isTriggered = true
				res.IsTriggered = true
				return res, nil
			}
		}

		// 判定是否达到激活价
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
			// 仍未激活，返回当前状态
			return res, nil
		}

		// 触发激活
		t.isActivated = true
		res.IsActivated = true
		t.extremePrice = currentPrice
		res.ExtremePrice = currentPrice

		stop, err := CalculateStopPrice(t.cfg.Side, t.extremePrice, t.cfg.CallbackValue, t.cfg.CallbackType)
		if err != nil {
			return res, err
		}

		// 如果初始止损比计算出的追踪止损更优，则取更高/更低者保护收益
		t.currentStopPrice = t.ratchetStopPrice(t.currentStopPrice, stop)
		res.StopPrice = t.currentStopPrice
		res.PriceRatchetMove = true
		return res, nil
	}

	// 2. 已处于激活状态：更新极值价格与移动止损线
	extremeMoved := false
	if t.cfg.Side == strike.PositionSideLong {
		// 多头创新高
		if t.extremePrice.IsZero() || currentPrice.GreaterThan(t.extremePrice) {
			t.extremePrice = currentPrice
			extremeMoved = true
		}
	} else {
		// 空头创新低
		if t.extremePrice.IsZero() || currentPrice.LessThan(t.extremePrice) {
			t.extremePrice = currentPrice
			extremeMoved = true
		}
	}
	res.ExtremePrice = t.extremePrice

	if extremeMoved {
		newStop, err := CalculateStopPrice(t.cfg.Side, t.extremePrice, t.cfg.CallbackValue, t.cfg.CallbackType)
		if err != nil {
			return res, err
		}

		oldStop := t.currentStopPrice
		t.currentStopPrice = t.ratchetStopPrice(t.currentStopPrice, newStop)
		if !t.currentStopPrice.Equal(oldStop) {
			res.PriceRatchetMove = true
		}
	}
	res.StopPrice = t.currentStopPrice

	// 3. 检查当前价格是否击穿移动止损线
	if IsTriggered(t.cfg.Side, t.currentStopPrice, currentPrice) {
		t.isTriggered = true
		res.IsTriggered = true
	}

	return res, nil
}

// ratchetStopPrice 棘轮单向推进：多头止损线只升不降，空头止损线只降不升
func (t *Tracker) ratchetStopPrice(oldStop, newStop decimal.Decimal) decimal.Decimal {
	if oldStop.IsZero() {
		return newStop
	}
	if t.cfg.Side == strike.PositionSideLong {
		if newStop.GreaterThan(oldStop) {
			return newStop
		}
		return oldStop
	}

	// 空头
	if newStop.LessThan(oldStop) {
		return newStop
	}
	return oldStop
}

// GetSnapshot 获取当前追踪器状态快照（并发安全）
func (t *Tracker) GetSnapshot() UpdateResult {
	t.mu.RLock()
	defer t.mu.RUnlock()

	return UpdateResult{
		IsActivated:  t.isActivated,
		IsTriggered:  t.isTriggered,
		StopPrice:    t.currentStopPrice,
		ExtremePrice: t.extremePrice,
	}
}

// Reset 重置追踪器状态
func (t *Tracker) Reset() {
	t.mu.Lock()
	defer t.mu.Unlock()

	t.isTriggered = false
	t.extremePrice = decimal.Zero
	t.currentStopPrice = t.cfg.InitialStopPrice
	if t.cfg.ActivationPrice.LessThanOrEqual(decimal.Zero) {
		t.isActivated = true
	} else {
		t.isActivated = false
	}
}
