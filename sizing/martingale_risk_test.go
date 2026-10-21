package sizing

import (
	"errors"
	"testing"

	"github.com/YuanJey/go-strike"
	"github.com/shopspring/decimal"
)

func sizingDecimal(value string) decimal.Decimal {
	return decimal.RequireFromString(value)
}

func assertSizingDecimalEqual(t *testing.T, got, want decimal.Decimal) {
	t.Helper()
	if !got.Equal(want) {
		t.Fatalf("expected %s, got %s", want, got)
	}
}

func assertSizingDecimalClose(
	t *testing.T,
	got, want, tolerance decimal.Decimal,
) {
	t.Helper()
	if got.Sub(want).Abs().GreaterThan(tolerance) {
		t.Fatalf("expected %s +/- %s, got %s", want, tolerance, got)
	}
}

func TestMartingaleRiskSizingLong(t *testing.T) {
	result, err := MartingaleRiskSizing(MartingaleRiskConfig{
		Side:             strike.PositionSideLong,
		EntryPrice:       sizingDecimal("100"),
		StopLossPrice:    sizingDecimal("85"),
		PriceStep:        sizingDecimal("5"),
		VolumeMultiplier: sizingDecimal("2"),
		MaxLoss:          sizingDecimal("1000"),
	})
	if err != nil {
		t.Fatalf("MartingaleRiskSizing returned error: %v", err)
	}

	if result.Rounds != 3 {
		t.Fatalf("expected 3 rounds, got %d", result.Rounds)
	}
	assertSizingDecimalEqual(t, result.RiskCoefficient, sizingDecimal("55"))
	assertSizingDecimalClose(
		t,
		result.BaseQuantity,
		sizingDecimal("18.1818181818181818"),
		sizingDecimal("0.0000000000000001"),
	)

	expectedPrices := []decimal.Decimal{
		sizingDecimal("100"),
		sizingDecimal("95"),
		sizingDecimal("90"),
	}
	expectedMultipliers := []decimal.Decimal{
		sizingDecimal("1"),
		sizingDecimal("2"),
		sizingDecimal("4"),
	}
	for i, step := range result.Steps {
		assertSizingDecimalEqual(t, step.OrderPrice, expectedPrices[i])
		assertSizingDecimalEqual(t, step.QuantityMultiplier, expectedMultipliers[i])
	}

	if result.EstimatedMaxLoss.GreaterThan(sizingDecimal("1000")) {
		t.Fatalf("estimated loss exceeds budget: %s", result.EstimatedMaxLoss)
	}
	assertSizingDecimalClose(
		t,
		result.EstimatedMaxLoss,
		sizingDecimal("1000"),
		sizingDecimal("0.00000000000001"),
	)
	assertSizingDecimalClose(
		t,
		result.AveragePrice,
		sizingDecimal("92.8571428571428571"),
		sizingDecimal("0.0000000000000001"),
	)
}

func TestMartingaleRiskSizingShort(t *testing.T) {
	result, err := MartingaleRiskSizing(MartingaleRiskConfig{
		Side:             strike.PositionSideShort,
		EntryPrice:       sizingDecimal("100"),
		StopLossPrice:    sizingDecimal("115"),
		PriceStep:        sizingDecimal("5"),
		VolumeMultiplier: sizingDecimal("2"),
		MaxLoss:          sizingDecimal("550"),
	})
	if err != nil {
		t.Fatalf("MartingaleRiskSizing returned error: %v", err)
	}

	assertSizingDecimalEqual(t, result.BaseQuantity, sizingDecimal("10"))
	assertSizingDecimalEqual(t, result.EstimatedMaxLoss, sizingDecimal("550"))
	assertSizingDecimalEqual(t, result.TotalQuantity, sizingDecimal("70"))

	expectedPrices := []decimal.Decimal{
		sizingDecimal("100"),
		sizingDecimal("105"),
		sizingDecimal("110"),
	}
	expectedQuantities := []decimal.Decimal{
		sizingDecimal("10"),
		sizingDecimal("20"),
		sizingDecimal("40"),
	}
	for i, step := range result.Steps {
		assertSizingDecimalEqual(t, step.OrderPrice, expectedPrices[i])
		assertSizingDecimalEqual(t, step.Quantity, expectedQuantities[i])
	}
}

func TestMartingaleRiskSizingMaxRounds(t *testing.T) {
	result, err := MartingaleRiskSizing(MartingaleRiskConfig{
		Side:             strike.PositionSideLong,
		EntryPrice:       sizingDecimal("100"),
		StopLossPrice:    sizingDecimal("85"),
		PriceStep:        sizingDecimal("5"),
		VolumeMultiplier: sizingDecimal("2"),
		MaxLoss:          sizingDecimal("700"),
		MaxRounds:        2,
	})
	if err != nil {
		t.Fatalf("MartingaleRiskSizing returned error: %v", err)
	}

	if result.Rounds != 2 {
		t.Fatalf("expected 2 rounds, got %d", result.Rounds)
	}
	assertSizingDecimalEqual(t, result.RiskCoefficient, sizingDecimal("35"))
	assertSizingDecimalEqual(t, result.BaseQuantity, sizingDecimal("20"))

	truncated, err := MartingaleRiskSizing(MartingaleRiskConfig{
		Side:             strike.PositionSideLong,
		EntryPrice:       sizingDecimal("100"),
		StopLossPrice:    sizingDecimal("1"),
		PriceStep:        sizingDecimal("1"),
		VolumeMultiplier: sizingDecimal("2"),
		MaxLoss:          sizingDecimal("590"),
		MaxRounds:        2,
	})
	if err != nil {
		t.Fatalf("explicit MaxRounds should cap a large derived ladder: %v", err)
	}
	if truncated.Rounds != 2 {
		t.Fatalf("expected explicit cap of 2 rounds, got %d", truncated.Rounds)
	}
	assertSizingDecimalEqual(t, truncated.RiskCoefficient, sizingDecimal("295"))
	assertSizingDecimalEqual(t, truncated.BaseQuantity, sizingDecimal("2"))
}

func TestMartingaleRiskSizingCosts(t *testing.T) {
	result, err := MartingaleRiskSizing(MartingaleRiskConfig{
		Side:             strike.PositionSideLong,
		EntryPrice:       sizingDecimal("100"),
		StopLossPrice:    sizingDecimal("90"),
		PriceStep:        sizingDecimal("20"),
		VolumeMultiplier: sizingDecimal("2"),
		MaxLoss:          sizingDecimal("129"),
		EntryFeeRate:     sizingDecimal("0.01"),
		ExitFeeRate:      sizingDecimal("0.01"),
		SlippagePerUnit:  sizingDecimal("1"),
	})
	if err != nil {
		t.Fatalf("MartingaleRiskSizing returned error: %v", err)
	}

	if result.Rounds != 1 {
		t.Fatalf("expected 1 round, got %d", result.Rounds)
	}
	assertSizingDecimalEqual(t, result.Steps[0].PriceLossPerUnit, sizingDecimal("10"))
	assertSizingDecimalEqual(t, result.Steps[0].EntryFeePerUnit, sizingDecimal("1"))
	assertSizingDecimalEqual(t, result.Steps[0].ExitFeePerUnit, sizingDecimal("0.9"))
	assertSizingDecimalEqual(t, result.Steps[0].RiskPerUnit, sizingDecimal("12.9"))
	assertSizingDecimalEqual(t, result.BaseQuantity, sizingDecimal("10"))
	assertSizingDecimalEqual(t, result.EstimatedMaxLoss, sizingDecimal("129"))
}

func TestMartingaleRiskSizingQuantityStep(t *testing.T) {
	result, err := MartingaleRiskSizing(MartingaleRiskConfig{
		Side:             strike.PositionSideLong,
		EntryPrice:       sizingDecimal("100"),
		StopLossPrice:    sizingDecimal("85"),
		PriceStep:        sizingDecimal("5"),
		VolumeMultiplier: sizingDecimal("2"),
		MaxLoss:          sizingDecimal("1000"),
		QuantityStep:     sizingDecimal("1"),
	})
	if err != nil {
		t.Fatalf("MartingaleRiskSizing returned error: %v", err)
	}

	assertSizingDecimalEqual(t, result.BaseQuantity, sizingDecimal("18"))
	assertSizingDecimalEqual(t, result.Steps[0].Quantity, sizingDecimal("18"))
	assertSizingDecimalEqual(t, result.Steps[1].Quantity, sizingDecimal("36"))
	assertSizingDecimalEqual(t, result.Steps[2].Quantity, sizingDecimal("72"))
	assertSizingDecimalEqual(t, result.EstimatedMaxLoss, sizingDecimal("990"))
	assertSizingDecimalEqual(t, result.UnusedRisk, sizingDecimal("10"))
}

func TestMartingaleRiskSizingValidation(t *testing.T) {
	valid := MartingaleRiskConfig{
		Side:             strike.PositionSideLong,
		EntryPrice:       sizingDecimal("100"),
		StopLossPrice:    sizingDecimal("90"),
		PriceStep:        sizingDecimal("5"),
		VolumeMultiplier: sizingDecimal("2"),
		MaxLoss:          sizingDecimal("100"),
	}

	tests := []struct {
		name   string
		mutate func(*MartingaleRiskConfig)
		target error
	}{
		{
			name: "invalid side",
			mutate: func(cfg *MartingaleRiskConfig) {
				cfg.Side = strike.PositionSideBoth
			},
			target: strike.ErrInvalidStopLossPrice,
		},
		{
			name: "invalid stop direction",
			mutate: func(cfg *MartingaleRiskConfig) {
				cfg.StopLossPrice = sizingDecimal("101")
			},
			target: strike.ErrInvalidStopLossPrice,
		},
		{
			name: "zero price step",
			mutate: func(cfg *MartingaleRiskConfig) {
				cfg.PriceStep = decimal.Zero
			},
			target: strike.ErrInvalidDistance,
		},
		{
			name: "zero multiplier",
			mutate: func(cfg *MartingaleRiskConfig) {
				cfg.VolumeMultiplier = decimal.Zero
			},
			target: ErrInvalidMartingaleMultiplier,
		},
		{
			name: "zero max loss",
			mutate: func(cfg *MartingaleRiskConfig) {
				cfg.MaxLoss = decimal.Zero
			},
			target: ErrInvalidMartingaleMaxLoss,
		},
		{
			name: "negative rounds",
			mutate: func(cfg *MartingaleRiskConfig) {
				cfg.MaxRounds = -1
			},
			target: ErrInvalidMartingaleRounds,
		},
		{
			name: "round limit",
			mutate: func(cfg *MartingaleRiskConfig) {
				cfg.MaxRounds = MaxSupportedMartingaleRounds + 1
			},
			target: ErrMartingaleRoundsExceeded,
		},
		{
			name: "invalid fee",
			mutate: func(cfg *MartingaleRiskConfig) {
				cfg.EntryFeeRate = sizingDecimal("1")
			},
			target: ErrInvalidFeeRate,
		},
		{
			name: "invalid slippage",
			mutate: func(cfg *MartingaleRiskConfig) {
				cfg.SlippagePerUnit = sizingDecimal("-1")
			},
			target: ErrInvalidSlippage,
		},
		{
			name: "invalid quantity step",
			mutate: func(cfg *MartingaleRiskConfig) {
				cfg.QuantityStep = sizingDecimal("-1")
			},
			target: strike.ErrInvalidQuantity,
		},
		{
			name: "derived rounds exceeded",
			mutate: func(cfg *MartingaleRiskConfig) {
				cfg.StopLossPrice = sizingDecimal("1")
				cfg.PriceStep = sizingDecimal("1")
			},
			target: ErrMartingaleRoundsExceeded,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			cfg := valid
			test.mutate(&cfg)
			_, err := MartingaleRiskSizing(cfg)
			if !errors.Is(err, test.target) {
				t.Fatalf("expected %v, got %v", test.target, err)
			}
		})
	}
}

func TestMartingaleRiskSizingQuantityBelowStep(t *testing.T) {
	_, err := MartingaleRiskSizing(MartingaleRiskConfig{
		Side:             strike.PositionSideLong,
		EntryPrice:       sizingDecimal("100"),
		StopLossPrice:    sizingDecimal("90"),
		PriceStep:        sizingDecimal("20"),
		VolumeMultiplier: sizingDecimal("2"),
		MaxLoss:          sizingDecimal("5"),
		QuantityStep:     sizingDecimal("1"),
	})
	if !errors.Is(err, ErrBaseQuantityTooSmall) {
		t.Fatalf("expected ErrBaseQuantityTooSmall, got %v", err)
	}

	_, err = MartingaleRiskSizing(MartingaleRiskConfig{
		Side:             strike.PositionSideLong,
		EntryPrice:       sizingDecimal("100"),
		StopLossPrice:    sizingDecimal("80"),
		PriceStep:        sizingDecimal("10"),
		VolumeMultiplier: sizingDecimal("0.1"),
		MaxLoss:          sizingDecimal("21"),
		QuantityStep:     sizingDecimal("1"),
	})
	if !errors.Is(err, ErrMartingaleQuantityBelowStep) {
		t.Fatalf("expected ErrMartingaleQuantityBelowStep, got %v", err)
	}

	_, err = MartingaleRiskSizing(MartingaleRiskConfig{
		Side:             strike.PositionSideLong,
		EntryPrice:       sizingDecimal("100000000000000000000"),
		StopLossPrice:    sizingDecimal("1"),
		PriceStep:        sizingDecimal("200000000000000000000"),
		VolumeMultiplier: sizingDecimal("1"),
		MaxLoss:          sizingDecimal("0.00000000000000000001"),
	})
	if !errors.Is(err, ErrBaseQuantityTooSmall) {
		t.Fatalf("expected precision-limited ErrBaseQuantityTooSmall, got %v", err)
	}
}
