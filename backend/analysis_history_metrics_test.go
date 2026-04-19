package main

import (
	"testing"
	"time"

	"github.com/woodyyan/pumpkin-pro/backend/store/live"
)

func TestEstimateAnalysisPriceFromDailyBars_UsesNearestPreviousTradingDay(t *testing.T) {
	bars := []live.DailyBar{
		{Date: "2026-04-16", Close: 10.5},
		{Date: "2026-04-17", Close: 11.2},
	}
	createdAt := time.Date(2026, 4, 18, 3, 0, 0, 0, time.UTC) // 周六，CST 为 11:00
	price, ok := estimateAnalysisPriceFromDailyBars(createdAt, bars)
	if !ok {
		t.Fatal("expected fallback price")
	}
	if price != 11.2 {
		t.Fatalf("price = %.2f", price)
	}
}

func TestEstimateAnalysisPriceFromDailyBars_UsesSameDayWhenAvailable(t *testing.T) {
	bars := []live.DailyBar{
		{Date: "2026-04-18", Close: 18.6},
	}
	createdAt := time.Date(2026, 4, 18, 7, 30, 0, 0, time.UTC)
	price, ok := estimateAnalysisPriceFromDailyBars(createdAt, bars)
	if !ok || price != 18.6 {
		t.Fatalf("same-day price = %.2f ok=%v", price, ok)
	}
}

func TestAsPositiveFloat(t *testing.T) {
	if got := asPositiveFloat(12.5); got != 12.5 {
		t.Fatalf("float = %.2f", got)
	}
	if got := asPositiveFloat("18.8"); got != 18.8 {
		t.Fatalf("string = %.2f", got)
	}
	if got := asPositiveFloat(0); got != 0 {
		t.Fatalf("zero = %.2f", got)
	}
	if got := asPositiveFloat("bad"); got != 0 {
		t.Fatalf("bad string = %.2f", got)
	}
}
