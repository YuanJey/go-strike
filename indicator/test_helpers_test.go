package indicator

import (
	"testing"

	"github.com/shopspring/decimal"
)

func decimalValue(value string) decimal.Decimal {
	return decimal.RequireFromString(value)
}

func decimalValues(values ...string) []decimal.Decimal {
	result := make([]decimal.Decimal, len(values))
	for i, value := range values {
		result[i] = decimalValue(value)
	}
	return result
}

func assertDecimalEqual(t *testing.T, got, want decimal.Decimal) {
	t.Helper()
	if !got.Equal(want) {
		t.Fatalf("expected %s, got %s", want, got)
	}
}

func assertDecimalClose(t *testing.T, got, want, tolerance decimal.Decimal) {
	t.Helper()
	if got.Sub(want).Abs().GreaterThan(tolerance) {
		t.Fatalf("expected %s +/- %s, got %s", want, tolerance, got)
	}
}
