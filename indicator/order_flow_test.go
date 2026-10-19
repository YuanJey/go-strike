package indicator

import (
	"errors"
	"testing"

	"github.com/shopspring/decimal"
)

func TestCVD(t *testing.T) {
	points := []TradeFlowPoint{
		{
			Timestamp:  1000,
			BuyVolume:  decimalValue("10"),
			SellVolume: decimalValue("4"),
		},
		{
			Timestamp:  2000,
			BuyVolume:  decimalValue("3"),
			SellVolume: decimalValue("7"),
		},
	}

	result, err := CVD(points, decimalValue("2"))
	if err != nil {
		t.Fatalf("CVD returned error: %v", err)
	}

	if result.Timestamps[0] != 1000 || result.Timestamps[1] != 2000 {
		t.Fatalf("unexpected timestamps: %v", result.Timestamps)
	}
	assertDecimalEqual(t, result.Delta.Values[0], decimalValue("6"))
	assertDecimalEqual(t, result.Delta.Values[1], decimalValue("-4"))
	assertDecimalEqual(t, result.Cumulative.Values[0], decimalValue("8"))
	assertDecimalEqual(t, result.Cumulative.Values[1], decimalValue("4"))
	if result.Contributors[0] != 1 || result.Contributors[1] != 1 {
		t.Fatalf("unexpected contributors: %v", result.Contributors)
	}
}

func TestMultiExchangeCVD(t *testing.T) {
	series := []ExchangeTradeFlow{
		{
			Exchange: "binance",
			Points: []TradeFlowPoint{
				{
					Timestamp:  1000,
					BuyVolume:  decimalValue("10"),
					SellVolume: decimalValue("4"),
				},
				{
					Timestamp:  2000,
					BuyVolume:  decimalValue("3"),
					SellVolume: decimalValue("7"),
				},
			},
		},
		{
			Exchange: "okx",
			Points: []TradeFlowPoint{
				{
					Timestamp:  1000,
					BuyVolume:  decimalValue("2"),
					SellVolume: decimalValue("5"),
				},
				{
					Timestamp:  3000,
					BuyVolume:  decimalValue("8"),
					SellVolume: decimalValue("1"),
				},
			},
		},
	}

	result, err := MultiExchangeCVD(series, decimal.Zero)
	if err != nil {
		t.Fatalf("MultiExchangeCVD returned error: %v", err)
	}

	expectedTimestamps := []int64{1000, 2000, 3000}
	expectedDelta := decimalValues("3", "-4", "7")
	expectedCumulative := decimalValues("3", "-1", "6")
	expectedContributors := []int{2, 1, 1}

	for i := range expectedTimestamps {
		if result.Timestamps[i] != expectedTimestamps[i] {
			t.Fatalf("index %d expected timestamp %d, got %d", i, expectedTimestamps[i], result.Timestamps[i])
		}
		assertDecimalEqual(t, result.Delta.Values[i], expectedDelta[i])
		assertDecimalEqual(t, result.Cumulative.Values[i], expectedCumulative[i])
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

func TestCVDValidation(t *testing.T) {
	_, err := CVD([]TradeFlowPoint{{
		Timestamp: 1000,
		BuyVolume: decimalValue("-1"),
	}}, decimal.Zero)
	if !errors.Is(err, ErrInvalidTradeFlow) {
		t.Fatalf("expected ErrInvalidTradeFlow, got %v", err)
	}

	_, err = CVD([]TradeFlowPoint{
		{Timestamp: 1000},
		{Timestamp: 1000},
	}, decimal.Zero)
	if !errors.Is(err, ErrNonIncreasingTimestamp) {
		t.Fatalf("expected ErrNonIncreasingTimestamp, got %v", err)
	}

	_, err = MultiExchangeCVD([]ExchangeTradeFlow{
		{Exchange: "binance", Points: []TradeFlowPoint{{Timestamp: 1000}}},
		{Exchange: "binance", Points: []TradeFlowPoint{{Timestamp: 2000}}},
	}, decimal.Zero)
	if !errors.Is(err, ErrInvalidExchangeSeries) {
		t.Fatalf("expected ErrInvalidExchangeSeries, got %v", err)
	}

	_, err = MultiExchangeCVD([]ExchangeTradeFlow{{
		Exchange: "binance",
		Points: []TradeFlowPoint{
			{Timestamp: 2000},
			{Timestamp: 1000},
		},
	}}, decimal.Zero)
	if !errors.Is(err, ErrInvalidExchangeSeries) ||
		!errors.Is(err, ErrNonIncreasingTimestamp) {
		t.Fatalf("expected wrapped timestamp error, got %v", err)
	}
}
