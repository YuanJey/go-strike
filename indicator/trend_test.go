package indicator

import (
	"errors"
	"testing"

	"github.com/shopspring/decimal"
)

func TestMACD(t *testing.T) {
	result, err := MACD(decimalValues("1", "2", "3", "4", "5"), 2, 3, 2)
	if err != nil {
		t.Fatalf("MACD returned error: %v", err)
	}

	if result.Line.Start != 2 {
		t.Fatalf("expected line Start 2, got %d", result.Line.Start)
	}
	if result.Signal.Start != 3 || result.Histogram.Start != 3 {
		t.Fatalf(
			"expected signal and histogram Start 3, got %d and %d",
			result.Signal.Start,
			result.Histogram.Start,
		)
	}

	tolerance := decimalValue("0.000000000000001")
	for i := result.Line.Start; i < len(result.Line.Values); i++ {
		assertDecimalClose(t, result.Line.Values[i], decimalValue("0.5"), tolerance)
	}
	for i := result.Signal.Start; i < len(result.Signal.Values); i++ {
		assertDecimalClose(t, result.Signal.Values[i], decimalValue("0.5"), tolerance)
		assertDecimalClose(t, result.Histogram.Values[i], decimalValue("0"), tolerance)
	}
}

func TestMACDValidation(t *testing.T) {
	_, err := MACD(decimalValues("1", "2", "3"), 3, 2, 1)
	if !errors.Is(err, ErrInvalidMACDPeriods) {
		t.Fatalf("expected ErrInvalidMACDPeriods, got %v", err)
	}

	_, err = MACD(decimalValues("1", "2", "3"), 2, 3, 2)
	if !errors.Is(err, ErrInsufficientData) {
		t.Fatalf("expected ErrInsufficientData, got %v", err)
	}
}

func TestDonchianChannel(t *testing.T) {
	candles := []Candle{
		{High: decimalValue("10"), Low: decimalValue("5")},
		{High: decimalValue("12"), Low: decimalValue("7")},
		{High: decimalValue("11"), Low: decimalValue("6")},
		{High: decimalValue("15"), Low: decimalValue("9")},
	}

	result, err := DonchianChannel(candles, 3)
	if err != nil {
		t.Fatalf("DonchianChannel returned error: %v", err)
	}

	assertDecimalEqual(t, result.Upper.Values[2], decimalValue("12"))
	assertDecimalEqual(t, result.Lower.Values[2], decimalValue("5"))
	assertDecimalEqual(t, result.Middle.Values[2], decimalValue("8.5"))

	assertDecimalEqual(t, result.Upper.Values[3], decimalValue("15"))
	assertDecimalEqual(t, result.Lower.Values[3], decimalValue("6"))
	assertDecimalEqual(t, result.Middle.Values[3], decimalValue("10.5"))
}

func TestSupertrend(t *testing.T) {
	candles := []Candle{
		{High: decimalValue("10"), Low: decimalValue("8"), Close: decimalValue("9")},
		{High: decimalValue("11"), Low: decimalValue("9"), Close: decimalValue("10")},
		{High: decimalValue("13"), Low: decimalValue("11"), Close: decimalValue("12.5")},
		{High: decimalValue("14"), Low: decimalValue("12"), Close: decimalValue("13")},
		{High: decimalValue("10"), Low: decimalValue("8"), Close: decimalValue("8.5")},
	}

	result, err := Supertrend(candles, 2, decimal.NewFromInt(1))
	if err != nil {
		t.Fatalf("Supertrend returned error: %v", err)
	}
	if result.Value.Start != 1 {
		t.Fatalf("expected Supertrend Start 1, got %d", result.Value.Start)
	}
	if result.Direction[0] != SupertrendDirectionUnknown {
		t.Fatalf("expected pre-warmup direction unknown, got %d", result.Direction[0])
	}

	expectedValues := decimalValues("12", "9.5", "10.75", "12.625")
	expectedUpperBands := decimalValues("12", "12", "15.25", "12.625")
	expectedLowerBands := decimalValues("8", "9.5", "10.75", "10.75")
	expectedDirections := []SupertrendDirection{
		SupertrendDirectionDown,
		SupertrendDirectionUp,
		SupertrendDirectionUp,
		SupertrendDirectionDown,
	}

	for i := result.Value.Start; i < len(candles); i++ {
		expectedIndex := i - result.Value.Start
		assertDecimalEqual(t, result.Value.Values[i], expectedValues[expectedIndex])
		assertDecimalEqual(t, result.UpperBand.Values[i], expectedUpperBands[expectedIndex])
		assertDecimalEqual(t, result.LowerBand.Values[i], expectedLowerBands[expectedIndex])
		if result.Direction[i] != expectedDirections[expectedIndex] {
			t.Fatalf(
				"index %d expected direction %d, got %d",
				i,
				expectedDirections[expectedIndex],
				result.Direction[i],
			)
		}
	}
}

func TestSupertrendValidation(t *testing.T) {
	candles := []Candle{
		{High: decimalValue("10"), Low: decimalValue("8"), Close: decimalValue("9")},
		{High: decimalValue("11"), Low: decimalValue("9"), Close: decimalValue("10")},
	}

	_, err := Supertrend(candles, 2, decimal.Zero)
	if !errors.Is(err, ErrInvalidMultiplier) {
		t.Fatalf("expected ErrInvalidMultiplier, got %v", err)
	}

	_, err = Supertrend(candles, 3, decimal.NewFromInt(1))
	if !errors.Is(err, ErrInsufficientData) {
		t.Fatalf("expected ErrInsufficientData, got %v", err)
	}
}
