package indicator

import (
	"errors"
	"testing"
)

func TestRSI(t *testing.T) {
	result, err := RSI(decimalValues("1", "2", "3", "2", "4"), 2)
	if err != nil {
		t.Fatalf("RSI returned error: %v", err)
	}

	if result.Start != 2 {
		t.Fatalf("expected RSI Start 2, got %d", result.Start)
	}
	assertDecimalEqual(t, result.Values[2], decimalValue("100"))
	assertDecimalEqual(t, result.Values[3], decimalValue("50"))
	assertDecimalClose(
		t,
		result.Values[4],
		decimalValue("83.3333333333333333"),
		decimalValue("0.0000000000000001"),
	)
}

func TestRSIFlatSeries(t *testing.T) {
	result, err := RSI(decimalValues("5", "5", "5"), 2)
	if err != nil {
		t.Fatalf("RSI returned error: %v", err)
	}
	assertDecimalEqual(t, result.Values[2], decimalValue("50"))
}

func TestROC(t *testing.T) {
	result, err := ROC(decimalValues("10", "12", "15"), 1)
	if err != nil {
		t.Fatalf("ROC returned error: %v", err)
	}

	assertDecimalEqual(t, result.Values[1], decimalValue("20"))
	assertDecimalEqual(t, result.Values[2], decimalValue("25"))
}

func TestMomentumValidation(t *testing.T) {
	_, err := RSI(decimalValues("1", "2"), 2)
	if !errors.Is(err, ErrInsufficientData) {
		t.Fatalf("expected ErrInsufficientData, got %v", err)
	}

	_, err = ROC(decimalValues("0", "1"), 1)
	if !errors.Is(err, ErrZeroReference) {
		t.Fatalf("expected ErrZeroReference, got %v", err)
	}
}
