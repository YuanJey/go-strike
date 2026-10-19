package indicator

import (
	"errors"
	"testing"

	"github.com/shopspring/decimal"
)

func TestTrueRangeAndATR(t *testing.T) {
	candles := []Candle{
		{
			High:  decimalValue("10"),
			Low:   decimalValue("8"),
			Close: decimalValue("9"),
		},
		{
			High:  decimalValue("12"),
			Low:   decimalValue("9"),
			Close: decimalValue("11"),
		},
		{
			High:  decimalValue("13"),
			Low:   decimalValue("10"),
			Close: decimalValue("12"),
		},
	}

	trueRange, err := TrueRange(candles)
	if err != nil {
		t.Fatalf("TrueRange returned error: %v", err)
	}
	for i, want := range decimalValues("2", "3", "3") {
		assertDecimalEqual(t, trueRange.Values[i], want)
	}

	atr, err := ATR(candles, 2)
	if err != nil {
		t.Fatalf("ATR returned error: %v", err)
	}
	if atr.Start != 1 {
		t.Fatalf("expected ATR Start 1, got %d", atr.Start)
	}
	assertDecimalEqual(t, atr.Values[1], decimalValue("2.5"))
	assertDecimalEqual(t, atr.Values[2], decimalValue("2.75"))
}

func TestStandardDeviation(t *testing.T) {
	result, err := StandardDeviation(decimalValues("1", "2", "3", "4"), 3)
	if err != nil {
		t.Fatalf("StandardDeviation returned error: %v", err)
	}

	want := decimalValue("0.816496580927726")
	tolerance := decimalValue("0.000000000000001")
	assertDecimalClose(t, result.Values[2], want, tolerance)
	assertDecimalClose(t, result.Values[3], want, tolerance)
}

func TestBollingerBands(t *testing.T) {
	result, err := BollingerBands(
		decimalValues("1", "2", "3", "4", "5"),
		3,
		decimal.NewFromInt(2),
	)
	if err != nil {
		t.Fatalf("BollingerBands returned error: %v", err)
	}

	tolerance := decimalValue("0.00000000000001")
	assertDecimalEqual(t, result.Middle.Values[2], decimalValue("2"))
	assertDecimalClose(t, result.Upper.Values[2], decimalValue("3.632993161855452"), tolerance)
	assertDecimalClose(t, result.Lower.Values[2], decimalValue("0.367006838144548"), tolerance)
}

func TestVolatilityValidation(t *testing.T) {
	_, err := TrueRange([]Candle{{
		High: decimalValue("1"),
		Low:  decimalValue("2"),
	}})
	if !errors.Is(err, ErrInvalidCandle) {
		t.Fatalf("expected ErrInvalidCandle, got %v", err)
	}

	_, err = BollingerBands(
		decimalValues("1", "2", "3"),
		3,
		decimal.NewFromInt(-1),
	)
	if !errors.Is(err, ErrInvalidMultiplier) {
		t.Fatalf("expected ErrInvalidMultiplier, got %v", err)
	}
}
