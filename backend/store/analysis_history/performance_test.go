package analysis_history

import (
	"testing"
	"time"
)

func TestBuildSignalPerformances_UsesPreviousSameSignal(t *testing.T) {
	records := []AnalysisHistoryRecord{
		{ID: "buy-2", Signal: "buy", CreatedAt: time.Date(2026, 4, 13, 10, 0, 0, 0, time.UTC)},
		{ID: "sell-1", Signal: "sell", CreatedAt: time.Date(2026, 4, 12, 10, 0, 0, 0, time.UTC)},
		{ID: "buy-1", Signal: "buy", CreatedAt: time.Date(2026, 4, 11, 10, 0, 0, 0, time.UTC)},
	}
	resolved := map[string]ResolvedPrice{
		"buy-1":  {Price: 10, Basis: PriceBasisAnalysis},
		"sell-1": {Price: 9, Basis: PriceBasisAnalysis},
		"buy-2":  {Price: 12, Basis: PriceBasisAnalysis},
	}

	performances := BuildSignalPerformances(records, resolved)
	perf := performances["buy-2"]
	if perf == nil {
		t.Fatal("expected performance for latest buy signal")
	}
	if perf.ReturnPct == nil || *perf.ReturnPct != 20 {
		t.Fatalf("return pct = %+v", perf.ReturnPct)
	}
	if perf.DirectionStatus != DirectionStatusAligned {
		t.Fatalf("direction status = %q", perf.DirectionStatus)
	}
	if perf.PreviousAnalysisAt != "2026-04-11T10:00:00Z" {
		t.Fatalf("previous analysis at = %q", perf.PreviousAnalysisAt)
	}
	if perf.PreviousAnalysisPrice == nil || *perf.PreviousAnalysisPrice != 10 {
		t.Fatalf("previous price = %+v", perf.PreviousAnalysisPrice)
	}
}

func TestBuildSignalPerformances_SellSignalTreatsDeclineAsAligned(t *testing.T) {
	records := []AnalysisHistoryRecord{
		{ID: "sell-2", Signal: "sell", CreatedAt: time.Date(2026, 4, 13, 10, 0, 0, 0, time.UTC)},
		{ID: "sell-1", Signal: "sell", CreatedAt: time.Date(2026, 4, 11, 10, 0, 0, 0, time.UTC)},
	}
	resolved := map[string]ResolvedPrice{
		"sell-1": {Price: 15, Basis: PriceBasisAnalysis},
		"sell-2": {Price: 12, Basis: PriceBasisEstimatedClose},
	}

	performances := BuildSignalPerformances(records, resolved)
	perf := performances["sell-2"]
	if perf == nil || perf.ReturnPct == nil {
		t.Fatal("expected sell signal performance")
	}
	if *perf.ReturnPct != -20 {
		t.Fatalf("return pct = %v", *perf.ReturnPct)
	}
	if perf.DirectionStatus != DirectionStatusAligned {
		t.Fatalf("direction status = %q", perf.DirectionStatus)
	}
	if perf.PriceBasis != PriceBasisMixed {
		t.Fatalf("price basis = %q", perf.PriceBasis)
	}
}

func TestBuildSignalPerformances_FirstSignalOnlyKeepsCurrentPrice(t *testing.T) {
	records := []AnalysisHistoryRecord{{ID: "hold-1", Signal: "hold", CreatedAt: time.Date(2026, 4, 11, 10, 0, 0, 0, time.UTC)}}
	resolved := map[string]ResolvedPrice{"hold-1": {Price: 8.5, Basis: PriceBasisEstimatedClose}}

	performances := BuildSignalPerformances(records, resolved)
	perf := performances["hold-1"]
	if perf == nil {
		t.Fatal("expected current price to be preserved even without previous signal")
	}
	if perf.AnalysisPrice == nil || *perf.AnalysisPrice != 8.5 {
		t.Fatalf("analysis price = %+v", perf.AnalysisPrice)
	}
	if perf.ReturnPct != nil {
		t.Fatalf("expected no return pct, got %+v", perf.ReturnPct)
	}
	if perf.PriceBasis != PriceBasisEstimatedClose {
		t.Fatalf("price basis = %q", perf.PriceBasis)
	}
}
