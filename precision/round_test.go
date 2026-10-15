package precision

import (
	"testing"

	"github.com/YuanJey/go-strike"
	"github.com/shopspring/decimal"
)

func TestAlignTickSize(t *testing.T) {
	tickSize := decimal.NewFromFloat(0.1)

	// RoundDown
	price := decimal.NewFromFloat(123.456)
	aligned, err := AlignTickSize(price, tickSize, strike.RoundDown)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	expected := decimal.NewFromFloat(123.4)
	if !aligned.Equal(expected) {
		t.Errorf("RoundDown expected %s, got %s", expected, aligned)
	}

	// RoundNearest
	price2 := decimal.NewFromFloat(123.46)
	aligned2, err := AlignTickSize(price2, tickSize, strike.RoundNearest)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	expected2 := decimal.NewFromFloat(123.5)
	if !aligned2.Equal(expected2) {
		t.Errorf("RoundNearest expected %s, got %s", expected2, aligned2)
	}

	// 错误输入
	_, err = AlignTickSize(price, decimal.Zero, strike.RoundDown)
	if err != ErrZeroStep {
		t.Errorf("expected ErrZeroStep, got %v", err)
	}
}

func TestAlignStepSize(t *testing.T) {
	stepSize := decimal.NewFromFloat(0.001)

	// RoundDown (防止超额报单)
	qty := decimal.NewFromFloat(1.23499)
	aligned, err := AlignStepSize(qty, stepSize, strike.RoundDown)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	expected := decimal.NewFromFloat(1.234)
	if !aligned.Equal(expected) {
		t.Errorf("RoundDown expected %s, got %s", expected, aligned)
	}

	// 零值与负步长
	_, err = AlignStepSize(qty, decimal.NewFromFloat(-0.01), strike.RoundDown)
	if err != ErrZeroStep {
		t.Errorf("expected ErrZeroStep on negative, got %v", err)
	}
}

func TestCheckMinNotional(t *testing.T) {
	price := decimal.NewFromFloat(65000)
	qty := decimal.NewFromFloat(0.001) // 65 USDT
	minNotional := decimal.NewFromFloat(5)

	if !CheckMinNotional(price, qty, minNotional) {
		t.Errorf("expected true, notional 65 is >= 5")
	}

	tooSmallQty := decimal.NewFromFloat(0.00005) // 3.25 USDT
	if CheckMinNotional(price, tooSmallQty, minNotional) {
		t.Errorf("expected false, notional 3.25 is < 5")
	}
}
