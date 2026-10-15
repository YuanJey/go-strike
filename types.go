package strike

import (
	"errors"
	"fmt"
)

// Side 表示订单买卖方向
type Side string

const (
	SideBuy  Side = "BUY"
	SideSell Side = "SELL"
)

// PositionSide 表示持仓多空方向
type PositionSide string

const (
	PositionSideLong  PositionSide = "LONG"
	PositionSideShort PositionSide = "SHORT"
	PositionSideBoth  PositionSide = "BOTH"
)

// TrailingCallbackType 表示移动止盈止损的回调计算模式
type TrailingCallbackType string

const (
	// CallbackTypeRate 百分比回调模式 (例如 0.02 表示 2%)
	CallbackTypeRate TrailingCallbackType = "RATE"
	// CallbackTypeDistance 固定点数/价差回调模式 (例如 50 USDT)
	CallbackTypeDistance TrailingCallbackType = "DISTANCE"
)

// RoundMode 精度对齐舍入模式
type RoundMode int

const (
	// RoundDown 向下截断（金融交易最保守模式，防止超出最大下单限制）
	RoundDown RoundMode = iota
	// RoundUp 向上进位
	RoundNearest
	// RoundHalfUp 四舍五入
	RoundHalfUp
)

// 基础异常错误定义
var (
	ErrInvalidPrice           = errors.New("strike: 价格参数无效，必须大于 0")
	ErrInvalidQuantity        = errors.New("strike: 数量参数无效，必须大于 0")
	ErrInvalidCallbackRate    = errors.New("strike: 回调率无效，必须在 (0, 1) 区间内")
	ErrInvalidDistance        = errors.New("strike: 回调价差无效，必须大于 0")
	ErrInvalidActivationPrice = errors.New("strike: 激活价格无效")
	ErrMinNotionalNotMet      = errors.New("strike: 名义价值低于交易所最低限制")
	ErrInvalidRiskRatio       = errors.New("strike: 风险比例无效，必须在 (0, 1] 区间内")
	ErrInvalidStopLossPrice   = errors.New("strike: 止损价设置不符合持仓方向")
	ErrInvalidSlices          = errors.New("strike: 拆单片数无效，必须大于 0")
	ErrInvalidGridParameters  = errors.New("strike: 网格参数无效，上限价必须高于下限价且网格数需大于等于 2")
)

// ErrorWrap 提供带上下文的标准错误封装
func ErrorWrap(err error, context string) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("%w: %s", err, context)
}
