package indicator

import (
	"errors"
	"testing"

	"github.com/shopspring/decimal"
)

func TestSeriesAccess(t *testing.T) {
	series := Series{
		Values: decimalValues("0", "0", "3", "4"),
		Start:  2,
	}

	if _, ready := series.At(1); ready {
		t.Fatal("value before Start must not be ready")
	}
	value, ready := series.At(2)
	if !ready {
		t.Fatal("value at Start must be ready")
	}
	assertDecimalEqual(t, value, decimal.NewFromInt(3))

	valid := series.ValidValues()
	if len(valid) != 2 {
		t.Fatalf("expected 2 valid values, got %d", len(valid))
	}
	valid[0] = decimal.Zero
	assertDecimalEqual(t, series.Values[2], decimal.NewFromInt(3))
}

func TestSMA(t *testing.T) {
	result, err := SMA(decimalValues("1", "2", "3", "4", "5"), 3)
	if err != nil {
		t.Fatalf("SMA returned error: %v", err)
	}

	if result.Start != 2 {
		t.Fatalf("expected Start 2, got %d", result.Start)
	}
	expected := decimalValues("2", "3", "4")
	for i, want := range expected {
		assertDecimalEqual(t, result.Values[result.Start+i], want)
	}
}

func TestEMA(t *testing.T) {
	result, err := EMA(decimalValues("1", "2", "3", "4", "5"), 3)
	if err != nil {
		t.Fatalf("EMA returned error: %v", err)
	}

	expected := decimalValues("2", "3", "4")
	for i, want := range expected {
		assertDecimalEqual(t, result.Values[result.Start+i], want)
	}
}

func TestRMA(t *testing.T) {
	result, err := RMA(decimalValues("1", "2", "3", "4", "5"), 3)
	if err != nil {
		t.Fatalf("RMA returned error: %v", err)
	}

	assertDecimalEqual(t, result.Values[2], decimalValue("2"))
	assertDecimalClose(
		t,
		result.Values[3],
		decimalValue("2.6666666666666667"),
		decimalValue("0.0000000000000001"),
	)
	assertDecimalClose(
		t,
		result.Values[4],
		decimalValue("3.4444444444444445"),
		decimalValue("0.0000000000000001"),
	)
}

func TestWMA(t *testing.T) {
	result, err := WMA(decimalValues("1", "2", "3", "4", "5"), 3)
	if err != nil {
		t.Fatalf("WMA returned error: %v", err)
	}

	expected := decimalValues(
		"2.3333333333333333",
		"3.3333333333333333",
		"4.3333333333333333",
	)
	for i, want := range expected {
		assertDecimalClose(
			t,
			result.Values[result.Start+i],
			want,
			decimalValue("0.0000000000000001"),
		)
	}
}

func TestMovingAverageValidation(t *testing.T) {
	_, err := SMA(decimalValues("1"), 0)
	if !errors.Is(err, ErrInvalidPeriod) {
		t.Fatalf("expected ErrInvalidPeriod, got %v", err)
	}

	_, err = EMA(decimalValues("1", "2"), 3)
	if !errors.Is(err, ErrInsufficientData) {
		t.Fatalf("expected ErrInsufficientData, got %v", err)
	}
}
