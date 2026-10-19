package indicator

import (
	"errors"
	"testing"
)

func TestOI(t *testing.T) {
	points := []OIPoint{
		{Timestamp: 1000, Value: decimalValue("100")},
		{Timestamp: 2000, Value: decimalValue("110")},
		{Timestamp: 3000, Value: decimalValue("99")},
	}

	result, err := OI(points)
	if err != nil {
		t.Fatalf("OI returned error: %v", err)
	}

	if result.Change.Start != 1 || result.ChangeRate.Start != 1 {
		t.Fatalf(
			"expected change series Start 1, got %d and %d",
			result.Change.Start,
			result.ChangeRate.Start,
		)
	}
	assertDecimalEqual(t, result.Change.Values[1], decimalValue("10"))
	assertDecimalEqual(t, result.Change.Values[2], decimalValue("-11"))

	rate, defined := result.ChangeRateAt(1)
	if !defined {
		t.Fatal("expected change rate at index 1 to be defined")
	}
	assertDecimalEqual(t, rate, decimalValue("10"))
	rate, defined = result.ChangeRateAt(2)
	if !defined {
		t.Fatal("expected change rate at index 2 to be defined")
	}
	assertDecimalEqual(t, rate, decimalValue("-10"))

	if _, defined = result.ChangeRateAt(0); defined {
		t.Fatal("first change rate must not be defined")
	}
}

func TestOIChangeRateWithZeroReference(t *testing.T) {
	result, err := OI([]OIPoint{
		{Timestamp: 1000, Value: decimalValue("0")},
		{Timestamp: 2000, Value: decimalValue("10")},
	})
	if err != nil {
		t.Fatalf("OI returned error: %v", err)
	}

	assertDecimalEqual(t, result.Change.Values[1], decimalValue("10"))
	if _, defined := result.ChangeRateAt(1); defined {
		t.Fatal("change rate with zero reference must not be defined")
	}
}

func TestMultiExchangeOIDefaultRequiresAllExchanges(t *testing.T) {
	series := []ExchangeOISeries{
		{
			Exchange: "binance",
			Points: []OIPoint{
				{Timestamp: 1000, Value: decimalValue("100")},
				{Timestamp: 3000, Value: decimalValue("120")},
			},
		},
		{
			Exchange: "okx",
			Points: []OIPoint{
				{Timestamp: 2000, Value: decimalValue("50")},
				{Timestamp: 3000, Value: decimalValue("60")},
			},
		},
	}

	result, err := MultiExchangeOI(series, OIAggregationOptions{})
	if err != nil {
		t.Fatalf("MultiExchangeOI returned error: %v", err)
	}

	if len(result.Timestamps) != 2 {
		t.Fatalf("expected 2 timestamps, got %d", len(result.Timestamps))
	}
	if result.Timestamps[0] != 2000 || result.Timestamps[1] != 3000 {
		t.Fatalf("unexpected timestamps: %v", result.Timestamps)
	}
	assertDecimalEqual(t, result.Value.Values[0], decimalValue("150"))
	assertDecimalEqual(t, result.Value.Values[1], decimalValue("180"))
	assertDecimalEqual(t, result.Change.Values[1], decimalValue("30"))
	rate, defined := result.ChangeRateAt(1)
	if !defined {
		t.Fatal("expected aggregate change rate to be defined")
	}
	assertDecimalEqual(t, rate, decimalValue("20"))
}

func TestMultiExchangeOIStalenessAndMinimumContributors(t *testing.T) {
	series := []ExchangeOISeries{
		{
			Exchange: "binance",
			Points: []OIPoint{
				{Timestamp: 1, Value: decimalValue("100")},
				{Timestamp: 4, Value: decimalValue("130")},
			},
		},
		{
			Exchange: "okx",
			Points: []OIPoint{
				{Timestamp: 2, Value: decimalValue("50")},
				{Timestamp: 3, Value: decimalValue("60")},
			},
		},
	}

	result, err := MultiExchangeOI(series, OIAggregationOptions{
		MaxStalenessMillis: 1,
		MinContributors:    1,
	})
	if err != nil {
		t.Fatalf("MultiExchangeOI returned error: %v", err)
	}

	expectedValues := decimalValues("100", "150", "60", "190")
	expectedContributors := []int{1, 2, 1, 2}
	for i := range expectedValues {
		assertDecimalEqual(t, result.Value.Values[i], expectedValues[i])
		if result.Contributors[i] != expectedContributors[i] {
			t.Fatalf(
				"index %d expected %d contributors, got %d",
				i,
				expectedContributors[i],
				result.Contributors[i],
			)
		}
	}
}

func TestOIValidation(t *testing.T) {
	_, err := OI([]OIPoint{{Timestamp: 1, Value: decimalValue("-1")}})
	if !errors.Is(err, ErrInvalidOpenInterest) {
		t.Fatalf("expected ErrInvalidOpenInterest, got %v", err)
	}

	_, err = OI([]OIPoint{
		{Timestamp: 2, Value: decimalValue("1")},
		{Timestamp: 1, Value: decimalValue("2")},
	})
	if !errors.Is(err, ErrNonIncreasingTimestamp) {
		t.Fatalf("expected ErrNonIncreasingTimestamp, got %v", err)
	}

	_, err = MultiExchangeOI(
		[]ExchangeOISeries{{
			Exchange: "binance",
			Points:   []OIPoint{{Timestamp: 1, Value: decimalValue("1")}},
		}},
		OIAggregationOptions{MaxStalenessMillis: -1},
	)
	if !errors.Is(err, ErrInvalidAggregationOptions) {
		t.Fatalf("expected ErrInvalidAggregationOptions, got %v", err)
	}

	_, err = MultiExchangeOI(
		[]ExchangeOISeries{
			{
				Exchange: "binance",
				Points:   []OIPoint{{Timestamp: 1, Value: decimalValue("100")}},
			},
			{
				Exchange: "okx",
				Points:   []OIPoint{{Timestamp: 100, Value: decimalValue("50")}},
			},
		},
		OIAggregationOptions{MaxStalenessMillis: 1},
	)
	if !errors.Is(err, ErrInsufficientContributors) {
		t.Fatalf("expected ErrInsufficientContributors, got %v", err)
	}
}
