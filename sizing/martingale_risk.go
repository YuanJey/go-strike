package sizing

import (
	"errors"
	"fmt"

	"github.com/YuanJey/go-strike"
	"github.com/shopspring/decimal"
)

// MaxSupportedMartingaleRounds 是未显式截断时允许自动推导的最大马丁层数。
const MaxSupportedMartingaleRounds = 50

var (
	ErrInvalidMartingaleMultiplier = errors.New("sizing: 马丁加仓倍数必须大于 0")
	ErrInvalidMartingaleMaxLoss    = errors.New("sizing: 马丁最大亏损必须大于 0")
	ErrInvalidMartingaleRounds     = errors.New("sizing: 马丁最大层数不能小于 0")
	ErrMartingaleRoundsExceeded    = errors.New("sizing: 马丁加仓层数超过安全上限")
	ErrInvalidFeeRate              = errors.New("sizing: 手续费率必须在 [0, 1) 区间内")
	ErrInvalidSlippage             = errors.New("sizing: 单位滑点成本不能小于 0")
	ErrBaseQuantityTooSmall        = errors.New("sizing: 基础仓位为 0 或小于数量步长")
	ErrMartingaleQuantityBelowStep = errors.New("sizing: 加仓层数量按步长对齐后为 0")
)

// MartingaleRiskConfig 定义固定点数间距的马丁风险反推参数。
type MartingaleRiskConfig struct {
	Side             strike.PositionSide
	EntryPrice       decimal.Decimal // 首仓价格
	StopLossPrice    decimal.Decimal // 所有层级共用的止损价格
	PriceStep        decimal.Decimal // 固定加仓间距（点数）
	VolumeMultiplier decimal.Decimal // 每层数量相对上一层的倍数
	MaxLoss          decimal.Decimal // 所有层级成交后触发止损的最大亏损预算

	// MaxRounds 表示最大总层数，包含首仓。0 表示由止损距离自动推导。
	MaxRounds int

	// 可选成本参数。费率按成交名义价值计算，SlippagePerUnit 为每单位数量
	// 在止损场景下额外预留的绝对亏损金额。
	EntryFeeRate    decimal.Decimal
	ExitFeeRate     decimal.Decimal
	SlippagePerUnit decimal.Decimal

	// QuantityStep 为可选数量步长。大于 0 时，基础仓位和每层数量都会向下对齐。
	QuantityStep decimal.Decimal
}

// MartingaleRiskStep 描述单层加仓及其在止损价处的风险。
type MartingaleRiskStep struct {
	Round              int
	OrderPrice         decimal.Decimal
	QuantityMultiplier decimal.Decimal
	Quantity           decimal.Decimal
	Notional           decimal.Decimal
	PriceLossPerUnit   decimal.Decimal
	EntryFeePerUnit    decimal.Decimal
	ExitFeePerUnit     decimal.Decimal
	SlippagePerUnit    decimal.Decimal
	RiskPerUnit        decimal.Decimal
	LossAtStop         decimal.Decimal
	CumulativeQuantity decimal.Decimal
	CumulativeNotional decimal.Decimal
	CumulativeLoss     decimal.Decimal
	AveragePrice       decimal.Decimal
}

// MartingaleRiskResult 保存反推的基础仓位和完整风险阶梯。
type MartingaleRiskResult struct {
	RawBaseQuantity  decimal.Decimal // 未按 QuantityStep 对齐的连续基础仓位
	BaseQuantity     decimal.Decimal // 实际采用的基础仓位
	Rounds           int
	RiskCoefficient  decimal.Decimal
	TotalQuantity    decimal.Decimal
	TotalNotional    decimal.Decimal
	AveragePrice     decimal.Decimal
	EstimatedMaxLoss decimal.Decimal
	UnusedRisk       decimal.Decimal
	Steps            []MartingaleRiskStep
}

type martingaleRiskLevel struct {
	price              decimal.Decimal
	quantityMultiplier decimal.Decimal
	priceLossPerUnit   decimal.Decimal
	entryFeePerUnit    decimal.Decimal
	exitFeePerUnit     decimal.Decimal
	riskPerUnit        decimal.Decimal
}

// MartingaleRiskSizing 根据止损价、固定加仓间距、加仓倍数和最大亏损预算，
// 反推出首仓基础数量，并生成完整的加仓与止损风险计划。
//
// 本函数适用于盈亏与价格变动线性相关的现货和线性合约。币本位反向合约
// 需要由调用方使用对应的非线性盈亏模型。
func MartingaleRiskSizing(cfg MartingaleRiskConfig) (MartingaleRiskResult, error) {
	if err := validateMartingaleRiskConfig(cfg); err != nil {
		return MartingaleRiskResult{}, err
	}

	rounds, err := calculateMartingaleRounds(cfg)
	if err != nil {
		return MartingaleRiskResult{}, err
	}

	levels := make([]martingaleRiskLevel, rounds)
	riskCoefficient := decimal.Zero
	quantityMultiplier := decimal.NewFromInt(1)

	for i := 0; i < rounds; i++ {
		price := martingaleLevelPrice(cfg, i)
		priceLossPerUnit := price.Sub(cfg.StopLossPrice).Abs()
		entryFeePerUnit := price.Mul(cfg.EntryFeeRate)
		exitFeePerUnit := cfg.StopLossPrice.Mul(cfg.ExitFeeRate)
		riskPerUnit := priceLossPerUnit.
			Add(entryFeePerUnit).
			Add(exitFeePerUnit).
			Add(cfg.SlippagePerUnit)

		levels[i] = martingaleRiskLevel{
			price:              price,
			quantityMultiplier: quantityMultiplier,
			priceLossPerUnit:   priceLossPerUnit,
			entryFeePerUnit:    entryFeePerUnit,
			exitFeePerUnit:     exitFeePerUnit,
			riskPerUnit:        riskPerUnit,
		}
		riskCoefficient = riskCoefficient.Add(quantityMultiplier.Mul(riskPerUnit))
		quantityMultiplier = quantityMultiplier.Mul(cfg.VolumeMultiplier)
	}

	rawBaseQuantity := conservativeDivide(cfg.MaxLoss, riskCoefficient)
	if rawBaseQuantity.LessThanOrEqual(decimal.Zero) {
		return MartingaleRiskResult{}, ErrBaseQuantityTooSmall
	}
	baseQuantity := rawBaseQuantity
	if cfg.QuantityStep.GreaterThan(decimal.Zero) {
		baseQuantity = alignQuantityDown(baseQuantity, cfg.QuantityStep)
		if baseQuantity.LessThanOrEqual(decimal.Zero) {
			return MartingaleRiskResult{}, ErrBaseQuantityTooSmall
		}
	}

	steps := make([]MartingaleRiskStep, rounds)
	totalQuantity := decimal.Zero
	totalNotional := decimal.Zero
	estimatedMaxLoss := decimal.Zero

	for i, level := range levels {
		quantity := baseQuantity.Mul(level.quantityMultiplier)
		if cfg.QuantityStep.GreaterThan(decimal.Zero) {
			quantity = alignQuantityDown(quantity, cfg.QuantityStep)
			if quantity.LessThanOrEqual(decimal.Zero) {
				return MartingaleRiskResult{}, fmt.Errorf(
					"%w: round %d",
					ErrMartingaleQuantityBelowStep,
					i+1,
				)
			}
		}

		notional := level.price.Mul(quantity)
		lossAtStop := level.riskPerUnit.Mul(quantity)
		totalQuantity = totalQuantity.Add(quantity)
		totalNotional = totalNotional.Add(notional)
		estimatedMaxLoss = estimatedMaxLoss.Add(lossAtStop)
		averagePrice := totalNotional.Div(totalQuantity)

		steps[i] = MartingaleRiskStep{
			Round:              i + 1,
			OrderPrice:         level.price,
			QuantityMultiplier: level.quantityMultiplier,
			Quantity:           quantity,
			Notional:           notional,
			PriceLossPerUnit:   level.priceLossPerUnit,
			EntryFeePerUnit:    level.entryFeePerUnit,
			ExitFeePerUnit:     level.exitFeePerUnit,
			SlippagePerUnit:    cfg.SlippagePerUnit,
			RiskPerUnit:        level.riskPerUnit,
			LossAtStop:         lossAtStop,
			CumulativeQuantity: totalQuantity,
			CumulativeNotional: totalNotional,
			CumulativeLoss:     estimatedMaxLoss,
			AveragePrice:       averagePrice,
		}
	}

	unusedRisk := cfg.MaxLoss.Sub(estimatedMaxLoss)
	if unusedRisk.IsNegative() {
		return MartingaleRiskResult{}, fmt.Errorf(
			"sizing: 计算后的止损亏损 %s 超过最大亏损 %s",
			estimatedMaxLoss,
			cfg.MaxLoss,
		)
	}

	return MartingaleRiskResult{
		RawBaseQuantity:  rawBaseQuantity,
		BaseQuantity:     baseQuantity,
		Rounds:           rounds,
		RiskCoefficient:  riskCoefficient,
		TotalQuantity:    totalQuantity,
		TotalNotional:    totalNotional,
		AveragePrice:     totalNotional.Div(totalQuantity),
		EstimatedMaxLoss: estimatedMaxLoss,
		UnusedRisk:       unusedRisk,
		Steps:            steps,
	}, nil
}

func validateMartingaleRiskConfig(cfg MartingaleRiskConfig) error {
	if cfg.Side != strike.PositionSideLong && cfg.Side != strike.PositionSideShort {
		return strike.ErrorWrap(strike.ErrInvalidStopLossPrice, "持仓方向仅支持 Long 或 Short")
	}
	if cfg.EntryPrice.LessThanOrEqual(decimal.Zero) ||
		cfg.StopLossPrice.LessThanOrEqual(decimal.Zero) {
		return strike.ErrInvalidPrice
	}
	if cfg.PriceStep.LessThanOrEqual(decimal.Zero) {
		return strike.ErrInvalidDistance
	}
	if cfg.VolumeMultiplier.LessThanOrEqual(decimal.Zero) {
		return ErrInvalidMartingaleMultiplier
	}
	if cfg.MaxLoss.LessThanOrEqual(decimal.Zero) {
		return ErrInvalidMartingaleMaxLoss
	}
	if cfg.MaxRounds < 0 {
		return ErrInvalidMartingaleRounds
	}
	if cfg.MaxRounds > MaxSupportedMartingaleRounds {
		return ErrMartingaleRoundsExceeded
	}
	if cfg.EntryFeeRate.IsNegative() ||
		cfg.EntryFeeRate.GreaterThanOrEqual(decimal.NewFromInt(1)) ||
		cfg.ExitFeeRate.IsNegative() ||
		cfg.ExitFeeRate.GreaterThanOrEqual(decimal.NewFromInt(1)) {
		return ErrInvalidFeeRate
	}
	if cfg.SlippagePerUnit.IsNegative() {
		return ErrInvalidSlippage
	}
	if cfg.QuantityStep.IsNegative() {
		return strike.ErrInvalidQuantity
	}

	if cfg.Side == strike.PositionSideLong &&
		cfg.StopLossPrice.GreaterThanOrEqual(cfg.EntryPrice) {
		return strike.ErrorWrap(strike.ErrInvalidStopLossPrice, "多头止损价必须低于首仓价")
	}
	if cfg.Side == strike.PositionSideShort &&
		cfg.StopLossPrice.LessThanOrEqual(cfg.EntryPrice) {
		return strike.ErrorWrap(strike.ErrInvalidStopLossPrice, "空头止损价必须高于首仓价")
	}
	return nil
}

func calculateMartingaleRounds(cfg MartingaleRiskConfig) (int, error) {
	stopDistance := cfg.EntryPrice.Sub(cfg.StopLossPrice).Abs()
	derivedRoundsDecimal := stopDistance.Div(cfg.PriceStep).Ceil()
	if cfg.MaxRounds > 0 {
		configuredRounds := decimal.NewFromInt(int64(cfg.MaxRounds))
		if derivedRoundsDecimal.GreaterThan(configuredRounds) {
			return cfg.MaxRounds, nil
		}
		return int(derivedRoundsDecimal.IntPart()), nil
	}

	maxSupported := decimal.NewFromInt(MaxSupportedMartingaleRounds)
	if derivedRoundsDecimal.GreaterThan(maxSupported) {
		return 0, ErrMartingaleRoundsExceeded
	}
	return int(derivedRoundsDecimal.IntPart()), nil
}

func martingaleLevelPrice(cfg MartingaleRiskConfig, index int) decimal.Decimal {
	offset := cfg.PriceStep.Mul(decimal.NewFromInt(int64(index)))
	if cfg.Side == strike.PositionSideLong {
		return cfg.EntryPrice.Sub(offset)
	}
	return cfg.EntryPrice.Add(offset)
}

func alignQuantityDown(quantity, step decimal.Decimal) decimal.Decimal {
	return quantity.Div(step).Floor().Mul(step)
}

func conservativeDivide(numerator, denominator decimal.Decimal) decimal.Decimal {
	result := numerator.Div(denominator)
	if result.Mul(denominator).GreaterThan(numerator) {
		result = result.Sub(decimal.New(1, result.Exponent()))
	}
	return result
}
