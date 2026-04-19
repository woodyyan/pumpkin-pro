package analysis_history

import (
	"math"
	"testing"
	"time"

	"github.com/woodyyan/pumpkin-pro/backend/store/live"
)

func assertFloatPtrAlmostEqual(t *testing.T, got *float64, want float64, label string) {
	t.Helper()
	if got == nil {
		t.Fatalf("%s = nil", label)
	}
	if math.Abs(*got-want) > 1e-9 {
		t.Fatalf("%s = %.10f want %.10f", label, *got, want)
	}
}

func TestBuildQualityValidations_BuySignalHitOnPrimaryWindow(t *testing.T) {
	records := []AnalysisHistoryRecord{{
		ID:        "buy-1",
		Signal:    "buy",
		CreatedAt: time.Date(2026, 4, 10, 2, 30, 0, 0, time.UTC),
	}}
	resolved := map[string]ResolvedPrice{
		"buy-1": {Price: 10, Basis: PriceBasisAnalysis},
	}
	bars := []live.DailyBar{
		{Date: "2026-04-10", Close: 10.1, High: 10.3, Low: 9.9},
		{Date: "2026-04-11", Close: 10.2, High: 10.4, Low: 10.0},
		{Date: "2026-04-14", Close: 10.5, High: 10.7, Low: 10.1},
		{Date: "2026-04-15", Close: 10.8, High: 11.0, Low: 10.4},
		{Date: "2026-04-16", Close: 10.7, High: 10.9, Low: 10.5},
		{Date: "2026-04-17", Close: 10.9, High: 11.1, Low: 10.6},
		{Date: "2026-04-18", Close: 11.0, High: 11.2, Low: 10.7},
	}

	validations := BuildQualityValidations(records, resolved, bars)
	validation := validations["buy-1"]
	if validation == nil {
		t.Fatal("expected quality validation")
	}
	if validation.PrimaryWindowDays != 5 {
		t.Fatalf("primary window = %d", validation.PrimaryWindowDays)
	}
	if validation.AvailableDays != 6 {
		t.Fatalf("available days = %d", validation.AvailableDays)
	}
	if validation.SummaryStatus != QualitySummaryHit || validation.SummaryLabel != QualityLabelHit {
		t.Fatalf("summary = %s / %s", validation.SummaryStatus, validation.SummaryLabel)
	}
	assertFloatPtrAlmostEqual(t, validation.PrimaryReturnPct, 9, "primary return")
	if len(validation.Windows) != 4 {
		t.Fatalf("window count = %d", len(validation.Windows))
	}
	fiveDay := validation.Windows[2]
	if !fiveDay.Ready {
		t.Fatalf("5-day window should be ready: %+v", fiveDay)
	}
	assertFloatPtrAlmostEqual(t, fiveDay.ReturnPct, 9, "5-day return")
	if fiveDay.DirectionStatus != QualitySummaryHit {
		t.Fatalf("5-day status = %q", fiveDay.DirectionStatus)
	}
	assertFloatPtrAlmostEqual(t, fiveDay.MaxUpPct, 11, "5-day max up")
	assertFloatPtrAlmostEqual(t, fiveDay.MaxDownPct, 0, "5-day max down")
	if tenDay := validation.Windows[3]; tenDay.Ready || tenDay.DirectionStatus != QualitySummaryPending {
		t.Fatalf("10-day window = %+v", tenDay)
	}
}

func TestBuildQualityValidations_SellSignalTreatsRiseAsMiss(t *testing.T) {
	records := []AnalysisHistoryRecord{{
		ID:        "sell-1",
		Signal:    "sell",
		CreatedAt: time.Date(2026, 4, 10, 3, 0, 0, 0, time.UTC),
	}}
	resolved := map[string]ResolvedPrice{
		"sell-1": {Price: 10, Basis: PriceBasisEstimatedClose},
	}
	bars := []live.DailyBar{
		{Date: "2026-04-11", Close: 10.3, High: 10.6, Low: 10.1},
		{Date: "2026-04-14", Close: 10.4, High: 10.8, Low: 10.2},
		{Date: "2026-04-15", Close: 10.6, High: 10.9, Low: 10.3},
		{Date: "2026-04-16", Close: 10.8, High: 11.0, Low: 10.5},
		{Date: "2026-04-17", Close: 11.0, High: 11.2, Low: 10.7},
	}

	validation := BuildQualityValidations(records, resolved, bars)["sell-1"]
	if validation == nil {
		t.Fatal("expected validation")
	}
	if validation.SummaryStatus != QualitySummaryMiss || validation.SummaryLabel != QualityLabelMiss {
		t.Fatalf("summary = %s / %s", validation.SummaryStatus, validation.SummaryLabel)
	}
	if validation.PriceBasis != PriceBasisEstimatedClose {
		t.Fatalf("price basis = %q", validation.PriceBasis)
	}
	assertFloatPtrAlmostEqual(t, validation.PrimaryReturnPct, 10, "primary return")
}

func TestBuildQualityValidations_HoldSignalUsesNeutralSummary(t *testing.T) {
	records := []AnalysisHistoryRecord{{
		ID:        "hold-1",
		Signal:    "hold",
		CreatedAt: time.Date(2026, 4, 10, 2, 0, 0, 0, time.UTC),
	}}
	resolved := map[string]ResolvedPrice{
		"hold-1": {Price: 10, Basis: PriceBasisAnalysis},
	}
	bars := []live.DailyBar{
		{Date: "2026-04-11", Close: 10.1},
		{Date: "2026-04-14", Close: 10.0},
		{Date: "2026-04-15", Close: 10.2},
		{Date: "2026-04-16", Close: 10.1},
		{Date: "2026-04-17", Close: 10.3},
	}

	validation := BuildQualityValidations(records, resolved, bars)["hold-1"]
	if validation == nil {
		t.Fatal("expected validation")
	}
	if validation.SummaryStatus != QualitySummaryUnknown || validation.SummaryLabel != QualityLabelUnknown {
		t.Fatalf("summary = %s / %s", validation.SummaryStatus, validation.SummaryLabel)
	}
	assertFloatPtrAlmostEqual(t, validation.PrimaryReturnPct, 3, "primary return")
}

func TestBuildQualityValidations_ReturnsPendingWhenFutureBarsInsufficient(t *testing.T) {
	records := []AnalysisHistoryRecord{{
		ID:        "buy-pending",
		Signal:    "buy",
		CreatedAt: time.Date(2026, 4, 17, 2, 0, 0, 0, time.UTC),
	}}
	resolved := map[string]ResolvedPrice{
		"buy-pending": {Price: 10, Basis: PriceBasisAnalysis},
	}
	bars := []live.DailyBar{
		{Date: "2026-04-18", Close: 10.1},
		{Date: "2026-04-21", Close: 10.2},
	}

	validation := BuildQualityValidations(records, resolved, bars)["buy-pending"]
	if validation == nil {
		t.Fatal("expected validation")
	}
	if validation.AvailableDays != 2 {
		t.Fatalf("available days = %d", validation.AvailableDays)
	}
	if validation.SummaryStatus != QualitySummaryPending || validation.SummaryLabel != QualityLabelPending {
		t.Fatalf("summary = %s / %s", validation.SummaryStatus, validation.SummaryLabel)
	}
	if validation.PrimaryReturnPct != nil {
		t.Fatalf("primary return should be nil, got %+v", validation.PrimaryReturnPct)
	}
}

func TestBuildQualityValidations_WeekendAnalysisStartsFromNextTradingDay(t *testing.T) {
	records := []AnalysisHistoryRecord{{
		ID:        "weekend-buy",
		Signal:    "buy",
		CreatedAt: time.Date(2026, 4, 18, 2, 0, 0, 0, time.UTC),
	}}
	resolved := map[string]ResolvedPrice{
		"weekend-buy": {Price: 10, Basis: PriceBasisEstimatedClose},
	}
	bars := []live.DailyBar{
		{Date: "2026-04-17", Close: 9.8},
		{Date: "2026-04-20", Close: 10.5},
		{Date: "2026-04-21", Close: 10.6},
		{Date: "2026-04-22", Close: 10.7},
		{Date: "2026-04-23", Close: 10.8},
		{Date: "2026-04-24", Close: 10.9},
	}

	validation := BuildQualityValidations(records, resolved, bars)["weekend-buy"]
	if validation == nil {
		t.Fatal("expected validation")
	}
	firstWindow := validation.Windows[0]
	if !firstWindow.Ready || firstWindow.EndDate != "2026-04-20" {
		t.Fatalf("first window = %+v", firstWindow)
	}
}
