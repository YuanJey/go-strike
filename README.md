# ⚡ go-strike: 交易开仓与动态风控算法公共 SDK

`go-strike` 是一个为量化交易系统、高频交易机器人和衍生品策略引擎打造的高性能、无浮点漂移、并发安全的 Golang 公共算法 SDK。

---

## 📦 架构概览

```
github.com/YuanJey/go-strike
├── types.go               # 核心类型（Side、PositionSide、RoundMode、基础错误）
├── indicator/             # 常见技术指标与基础滚动算子
│   ├── moving_average.go  # SMA/EMA/RMA/WMA
│   ├── volatility.go      # True Range/ATR/标准差/布林带
│   ├── momentum.go        # RSI/ROC
│   ├── trend.go           # MACD/唐奇安通道/超级趋势
│   ├── volume.go          # VWAP/OBV
│   ├── order_flow.go      # 单交易所/多交易所 CVD
│   └── open_interest.go   # 单交易所/多交易所 OI
├── precision/             # 金融级精度对齐模块
│   ├── round.go           # TickSize/StepSize 步长规范化与 MinNotional 校验
│   └── round_test.go
├── trailing/              # 移动止盈止损模块
│   ├── trailing.go        # 无状态移动止损线与触发计算
│   ├── tracker.go         # 并发安全、支持单向棘轮推进的有状态追踪器
│   ├── chandelier.go      # ATR 吊灯移动止盈止损 (Chandelier Exit)
│   └── trailing_test.go
├── sizing/                # 开仓数量与仓位管理算法
│   ├── risk_fractional.go # 固定风险比例模型 (Fixed Fractional Risk Sizing)
│   ├── kelly.go           # 经典凯利公式与分数凯利 (Fractional Kelly)
│   ├── atr_sizing.go      # ATR 波动率自适应平价仓位
│   └── sizing_test.go
└── execution/             # 分批开仓与委托执行算法
    ├── grid.go            # 算术等差网格与几何等比网格生成器
    ├── martingale.go      # 马丁格尔加仓阶梯与动态持仓均价计算 (WAP)
    ├── iceberg.go         # 伪随机离散冰山委托拆单
    ├── twap.go            # TWAP 时间加权平均拆单计划
    └── execution_test.go
```

---

## 🚀 快速上手

### 1. 移动止盈止损跟踪器 (Trailing Stop Tracker)

```go
package main

import (
	"fmt"
	"github.com/YuanJey/go-strike"
	"github.com/YuanJey/go-strike/trailing"
	"github.com/shopspring/decimal"
)

func main() {
	// 创建多头追踪止盈器：激活价 110，回调 5%，保底止损 95
	cfg := trailing.TrackerConfig{
		Side:             strike.PositionSideLong,
		CallbackType:     strike.CallbackTypeRate,
		CallbackValue:    decimal.NewFromFloat(0.05), // 5% 回调
		ActivationPrice:  decimal.NewFromFloat(110.0),
		InitialStopPrice: decimal.NewFromFloat(95.0),
	}

	tracker, err := trailing.NewTracker(cfg)
	if err != nil {
		panic(err)
	}

	// 模拟行情 Tick 推进
	ticks := []float64{105, 110, 120, 116, 113.5}
	for _, p := range ticks {
		res, _ := tracker.Update(decimal.NewFromFloat(p))
		fmt.Printf("价格: %.2f | 激活: %v | 止损线: %s | 触发平仓: %v\n",
			p, res.IsActivated, res.StopPrice, res.IsTriggered)
	}
}
```

### 2. 精度对齐与合规校验 (Precision)

```go
// 规范化挂单价至交易所 tickSize (0.1)，向下截断避免超限
alignedPrice, err := precision.AlignTickSize(
    decimal.NewFromFloat(123.456), 
    decimal.NewFromFloat(0.1), 
    strike.RoundDown,
) // 123.4

// 规范化报单量至 stepSize (0.001)
alignedQty, err := precision.AlignStepSize(
    decimal.NewFromFloat(1.2349), 
    decimal.NewFromFloat(0.001), 
    strike.RoundDown,
) // 1.234
```

### 3. 开仓风险管理 (Fixed Fractional Sizing)

```go
// 账户 10,000 USDT，单笔承受 1% (100 USDT) 风险，入场价 100，止损价 95
res, err := sizing.FixedFractionalSizing(
    strike.PositionSideLong,
    decimal.NewFromFloat(10000),
    decimal.NewFromFloat(0.01),
    decimal.NewFromFloat(100),
    decimal.NewFromFloat(95),
    decimal.NewFromFloat(5), // 最大允许杠杆倍数
)
// res.Quantity = 20
```

### 4. 高级时空双重约束移动止盈止损 (Advanced Trailing Stop)

```go
// 支持做多保护下限 (LowerBoundPrice) 与做空保护上限 (UpperBoundPrice)
// 叠加开仓防震荡冷却期 (CoolingOffDuration)、超时自动平仓 (MaxHoldDuration) 以及随时间收紧的回调阶梯 (DecayTiers)
cfg := trailing.AdvancedTrackerConfig{
    Side:               strike.PositionSideLong,
    CallbackType:       strike.CallbackTypeRate,
    InitialCallback:    decimal.NewFromFloat(0.05), // 初始 5% 回调
    ActivationPrice:    decimal.NewFromFloat(110.0),// 涨至 110 激活追踪
    LowerBoundPrice:    decimal.NewFromFloat(95.0), // 做多硬保护下界：跌破 95 立即触发防守
    StartTime:          openTime,
    CoolingOffDuration: 5 * time.Minute,           // 前 5 分钟冷却期
    MaxHoldDuration:    2 * time.Hour,             // 持仓上限 2 小时，超时强平
    DecayTiers: []trailing.TimeDecayTier{
        {AfterDuration: 30 * time.Minute, CallbackValue: decimal.NewFromFloat(0.02)}, // 30分钟后收紧至 2%
    },
}

advTracker, _ := trailing.NewAdvancedTracker(cfg)
res, _ := advTracker.Update(tickTime, currentPrice)
if res.IsTriggered {
    fmt.Printf("触发平仓，原因: %s (有效止损价: %s)\n", res.Reason, res.StopPrice)
}
```

### 5. 移动追踪开仓算法 (Trailing Entry Engine)

```go
// 做多追踪开仓：市价必须低于 100 允许启动，持续寻底，自最低点反弹 2% 时触发开仓买入
// 杜绝高位追多与逆势接飞刀，并在超过 30 分钟未触发时自动失效
cfg := execution.TrailingEntryConfig{
    Side:          strike.PositionSideLong,
    TriggerPrice:  decimal.NewFromFloat(100.0), // 做多价格门槛：市价必须 < 100
    CallbackType:  strike.CallbackTypeRate,
    CallbackValue: decimal.NewFromFloat(0.02), // 反弹 2% 触发开仓
    StartTime:     time.Now(),
    ValidAfter:    1 * time.Minute,            // 准备缓冲期
    ExpireAfter:   30 * time.Minute,           // 30 分钟有效期
    MinDwellTime:  10 * time.Second,           // 价格进入门槛后至少稳定 10 秒
}

entryTracker, _ := execution.NewTrailingEntryTracker(cfg)

// 随行情 Tick 驱动
result, err := entryTracker.Update(tickTime, currentPrice)
if result.Triggered {
    fmt.Printf("触发开仓！报单方向: %s，极值低点: %s，触发点位: %s\n",
        result.TargetOrderSide, result.ExtremePrice, result.CurrentTriggerPrice)
}
```

### 6. 常见技术指标 (Indicator)

```go
closes := []decimal.Decimal{
    decimal.NewFromInt(100),
    decimal.NewFromInt(102),
    decimal.NewFromInt(101),
    decimal.NewFromInt(105),
    decimal.NewFromInt(108),
}

rsi, err := indicator.RSI(closes, 3)
if err != nil {
    panic(err)
}

// Series 与输入等长，Start 表示首个有效值下标。
latestRSI, ready := rsi.At(len(closes) - 1)
if ready {
    fmt.Printf("RSI: %s\n", latestRSI)
}

bands, _ := indicator.BollingerBands(
    closes,
    3,
    decimal.NewFromInt(2),
)
latestUpper, _ := bands.Upper.At(len(closes) - 1)
fmt.Printf("Bollinger upper: %s\n", latestUpper)
```

### 7. CVD 与 OI

```go
cvd, err := indicator.CVD([]indicator.TradeFlowPoint{
    {
        Timestamp:  1710000000000,
        BuyVolume:  decimal.NewFromInt(120),
        SellVolume: decimal.NewFromInt(80),
    },
}, decimal.Zero)
if err != nil {
    panic(err)
}
fmt.Printf("CVD: %s\n", cvd.Cumulative.Values[0])

globalOI, err := indicator.MultiExchangeOI(
    []indicator.ExchangeOISeries{
        {
            Exchange: "binance",
            Points: []indicator.OIPoint{
                {Timestamp: 1710000000000, Value: decimal.NewFromInt(1_000_000)},
            },
        },
        {
            Exchange: "okx",
            Points: []indicator.OIPoint{
                {Timestamp: 1710000000000, Value: decimal.NewFromInt(500_000)},
            },
        },
    },
    indicator.OIAggregationOptions{},
)
if err != nil {
    panic(err)
}
fmt.Printf("Global OI: %s\n", globalOI.Value.Values[0])
```

多交易所计算要求调用方先将成交量和 OI 转换到同一单位。CVD 缺失桶不做前向填充；OI 使用最近快照，并可通过 `MaxStalenessMillis` 限制快照陈旧时间。

---

## 📝 变更日志 (Changelog & Traceability)

### [v0.4.0] - 2026-10-19
- **新增指标模块**:
  - `indicator/moving_average.go`: 提供 SMA、EMA、Wilder RMA 与线性 WMA。
  - `indicator/volatility.go`: 提供 True Range、ATR、滚动总体标准差与布林带。
  - `indicator/momentum.go`: 提供 RSI 与百分比 ROC。
  - `indicator/trend.go`: 提供 MACD、唐奇安通道与基于 Wilder ATR 的超级趋势。
  - `indicator/volume.go`: 提供累计 VWAP 与 OBV。
  - `indicator/order_flow.go`: 提供单交易所 CVD 与多交易所主动成交量聚合。
  - `indicator/open_interest.go`: 提供单交易所 OI 变化率与支持陈旧时间控制的多交易所 OI 聚合。
- **统一序列语义**:
  - 所有批量指标返回与输入等长的 `Series`，通过 `Start` 和 `At` 明确预热期。
  - 全部计算使用 `decimal.Decimal`，不依赖交易所 API 或交易执行状态。
- **质量保证**:
  - 覆盖公式结果、预热下标、无涨跌 RSI、零成交量、非法周期与非法 K 线等边界测试。

### [v0.3.0] - 2026-10-15
- **核心算法升级**:
  - `execution/trailing_entry.go`: 正式落地 `TrailingEntryTracker` 移动追踪开仓引擎，满足纯开仓场景下的反弹/回落捕获。
  - **价格门槛控制**：做多必须低于 `TriggerPrice`、做空必须高于 `TriggerPrice` 方可启动追踪，避免追高追空。
  - **动态极值与回调**：做多自动寻底（LWM）并在反弹达到阈值时触发买入（`SideBuy`）；做空自动摸顶（HWM）并在回落达到阈值时触发卖出（`SideSell`）。
  - **时间生命周期**：支持开仓有效期窗口（`ExpireAfter`）、生效缓冲（`ValidAfter`）及区间最小观察驻留时间（`MinDwellTime`，防插针伪突破）。
- **质量保证**:
  - `execution/trailing_entry_test.go`: 完备的多空寻底反弹流程、时间约束与并发竞态测试 100% 通过。
- **影响范围**: 独立丰富 `execution` 模块开仓算法族，完全解耦出场与平仓逻辑。

### [v0.2.0] - 2026-10-15
- **功能升级**:
  - `trailing/advanced_tracker.go`: 落地 `AdvancedTracker` 高级时空约束移动止盈止损状态机。
  - 价格硬边界防守：做多低于 `LowerBoundPrice` 立即触发、做空高于 `UpperBoundPrice` 立即触发。
  - 时间生命周期：引入 `CoolingOffDuration`（入场冷却防震）、`MaxHoldDuration`（持仓超时强平）与 `DecayTiers`（随持仓时间递减动态收紧回调阈值，锁住高位利润）。
  - 触发归因体系：提供 `TriggerReasonPriceBoundary`、`TriggerReasonTrailing` 与 `TriggerReasonTimeout` 清晰区分平仓原因。
- **质量保证**:
  - `trailing/advanced_tracker_test.go`: 100% 测试通过，无竞态隐患。
- **影响范围**: 向后兼容拓展 `trailing` 模块高级能力。

### [v0.1.0] - 2026-10-15
- **初始化工程**: 完成 `github.com/YuanJey/go-strike` Go 1.26 模块创建，引入 `github.com/shopspring/decimal` 进行高精度定点计算。
- **新增模块**:
  - `types.go`: 核心枚举、交易方向、舍入模式及标准业务错误。
  - `precision`: 价格与数量步长精准舍入对齐（RoundDown/RoundNearest/RoundHalfUp）及最小名义价值校验。
  - `trailing`: 有状态高并发移动止盈止损状态机、无状态单步计算函数以及 ATR 吊灯止损算法。
  - `sizing`: 固定风险比例仓位模型、凯利公式/分数凯利、ATR 波动率平价仓位管理。
  - `execution`: 算术/几何网格挂单计算、马丁格尔阶梯与持仓加权成本、防探测冰山拆单、TWAP 时间加权拆单。
- **质量保证**:
  - 全模块单元测试覆盖与边界值校验。
  - 启用 `-race` 竞态检测器验证多协程行情推送下的并发安全性。
- **影响范围**: 新增全库基础代码与算法引擎，无历史破坏性变更。
