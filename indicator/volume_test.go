package indicator

import (
	"errors"
	"testing"

	"github.com/shopspring/decimal"
)

func TestVWAP(t *testing.T) {
	candles := []Candle{
		{
			High:   decimalValue("2"),
			Low:    decimalValue("0"),
			Close:  decimalValue("1"),
			Volume: decimalValue("1"),
		},
		{
			High:   decimalValue("4"),
			Low:    decimalValue("2"),
			Close:  decimalValue("3"),
			Volume: decimalValue("3"),
		},
		{
			High:   decimalValue("6"),
			Low:    decimalValue("4"),
			Close:  decimalValue("5"),
			Volume: decimalValue("2"),
		},
	}

	result, err := VWAP(candles)
	if err != nil {
		t.Fatalf("VWAP returned error: %v", err)
	}

	assertDecimalEqual(t, result.Values[0], decimalValue("1"))
	assertDecimalEqual(t, result.Values[1], decimalValue("2.5"))
	assertDecimalClose(
		t,
		result.Values[2],
		decimalValue("3.3333333333333333"),
		decimalValue("0.0000000000000001"),
	)
}

func TestVWAPStartsAtFirstPositiveVolume(t *testing.T) {
	candles := []Candle{
		{
			High:   decimalValue("2"),
			Low:    decimalValue("0"),
			Close:  decimalValue("1"),
			Volume: decimal.Zero,
		},
		{
			High:   decimalValue("4"),
			Low:    decimalValue("2"),
			Close:  decimalValue("3"),
			Volume: decimalValue("2"),
		},
	}

	result, err := VWAP(candles)
	if err != nil {
		t.Fatalf("VWAP returned error: %v", err)
	}
	if result.Start != 1 {
		t.Fatalf("expected Start 1, got %d", result.Start)
	}
	assertDecimalEqual(t, result.Values[1], decimalValue("3"))
}

func TestOBV(t *testing.T) {
	candles := []Candle{
		{Close: decimalValue("1"), Volume: decimalValue("10")},
		{Close: decimalValue("2"), Volume: decimalValue("20")},
		{Close: decimalValue("2"), Volume: decimalValue("30")},
		{Close: decimalValue("1"), Volume: decimalValue("40")},
	}

	result, err := OBV(candles)
	if err != nil {
		t.Fatalf("OBV returned error: %v", err)
	}

	for i, want := range decimalValues("0", "20", "20", "-20") {
		assertDecimalEqual(t, result.Values[i], want)
	}
}

func TestVolumeValidation(t *testing.T) {
	_, err := VWAP([]Candle{{Volume: decimal.Zero}})
	if !errors.Is(err, ErrZeroVolume) {
		t.Fatalf("expected ErrZeroVolume, got %v", err)
	}

	_, err = OBV([]Candle{{Volume: decimalValue("-1")}})
	if !errors.Is(err, ErrInvalidCandle) {
		t.Fatalf("expected ErrInvalidCandle, got %v", err)
	}
}
