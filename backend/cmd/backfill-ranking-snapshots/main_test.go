package main

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/woodyyan/pumpkin-pro/backend/store/live"
	"github.com/woodyyan/pumpkin-pro/backend/store/quadrant"
	"github.com/woodyyan/pumpkin-pro/backend/tests/testutil"
)

type stubDailyBarFetcher struct {
	barsBySymbol map[string][]live.DailyBar
	errBySymbol  map[string]error
}

func (s stubDailyBarFetcher) FetchSymbolDailyBars(ctx context.Context, symbol string, lookbackDays int) ([]live.DailyBar, error) {
	if err := s.errBySymbol[symbol]; err != nil {
		return nil, err
	}
	return s.barsBySymbol[symbol], nil
}

func TestBuildSnapshotSymbol(t *testing.T) {
	tests := []struct {
		name     string
		code     string
		exchange string
		want     string
		wantErr  bool
	}{
		{name: "SSE", code: "600519", exchange: "SSE", want: "600519.SH"},
		{name: "SZSE", code: "000001", exchange: "SZSE", want: "000001.SZ"},
		{name: "HKEX padded", code: "700", exchange: "HKEX", want: "00700.HK"},
		{name: "Infer by code", code: "300750", exchange: "", want: "300750.SZ"},
		{name: "Invalid exchange", code: "600519", exchange: "NYSE", wantErr: true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := buildSnapshotSymbol(tc.code, tc.exchange)
			if (err != nil) != tc.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tc.wantErr)
			}
			if got != tc.want {
				t.Fatalf("got %s, want %s", got, tc.want)
			}
		})
	}
}

func TestComputeLookbackDays(t *testing.T) {
	now := time.Date(2026, 4, 18, 9, 0, 0, 0, shanghaiLocation)
	rows := []quadrant.RankingSnapshot{{SnapshotDate: "2026-04-16"}}

	lookback, err := computeLookbackDays(rows, now, 10)
	if err != nil {
		t.Fatalf("computeLookbackDays failed: %v", err)
	}
	if lookback != 120 {
		t.Fatalf("lookback = %d, want 120 minimum", lookback)
	}
}

func TestResolveClosePrice(t *testing.T) {
	closeByDate := map[string]float64{
		"2026-04-17": 11,
		"2026-04-16": 10,
	}

	matchedDate, price := resolveClosePrice("2026-04-18", closeByDate, 3)
	if matchedDate != "2026-04-17" || price != 11 {
		t.Fatalf("fallback match = (%s, %.2f), want (2026-04-17, 11)", matchedDate, price)
	}

	matchedDate, price = resolveClosePrice("2026-04-18", closeByDate, 0)
	if matchedDate != "" || price != 0 {
		t.Fatalf("exact-only match = (%s, %.2f), want empty", matchedDate, price)
	}
}

func TestBuildPlansForSymbol(t *testing.T) {
	now := time.Date(2026, 4, 18, 9, 0, 0, 0, shanghaiLocation)
	fetcher := stubDailyBarFetcher{
		barsBySymbol: map[string][]live.DailyBar{
			"600519.SH": {
				{Date: "2026-04-16", Close: 10},
				{Date: "2026-04-17", Close: 11},
			},
		},
	}
	rows := []quadrant.RankingSnapshot{
		{ID: 1, Code: "600519", Exchange: "SSE", SnapshotDate: "2026-04-16", ClosePrice: 0},
		{ID: 2, Code: "600519", Exchange: "SSE", SnapshotDate: "2026-04-17", ClosePrice: 0},
	}

	plans, unresolved, err := buildPlansForSymbol(context.Background(), fetcher, "600519.SH", rows, now, 10, 3)
	if err != nil {
		t.Fatalf("buildPlansForSymbol failed: %v", err)
	}
	if len(unresolved) != 0 {
		t.Fatalf("expected no unresolved rows, got %d", len(unresolved))
	}
	if len(plans) != 2 {
		t.Fatalf("expected 2 plans, got %d", len(plans))
	}
	if plans[0].NewPrice != 10 || plans[1].NewPrice != 11 {
		t.Fatalf("unexpected prices: %+v", plans)
	}
	if plans[1].MatchedTradeDate != "2026-04-17" {
		t.Fatalf("matched trade date = %s, want 2026-04-17", plans[1].MatchedTradeDate)
	}
}

func TestBuildPlansForSymbol_FallsBackToPreviousTradingDate(t *testing.T) {
	now := time.Date(2026, 4, 19, 9, 0, 0, 0, shanghaiLocation)
	fetcher := stubDailyBarFetcher{
		barsBySymbol: map[string][]live.DailyBar{
			"600519.SH": {
				{Date: "2026-04-17", Close: 11},
			},
		},
	}
	rows := []quadrant.RankingSnapshot{{ID: 1, Code: "600519", Exchange: "SSE", SnapshotDate: "2026-04-18", ClosePrice: 0}}

	plans, unresolved, err := buildPlansForSymbol(context.Background(), fetcher, "600519.SH", rows, now, 10, 3)
	if err != nil {
		t.Fatalf("buildPlansForSymbol failed: %v", err)
	}
	if len(unresolved) != 0 {
		t.Fatalf("expected no unresolved rows, got %d", len(unresolved))
	}
	if len(plans) != 1 {
		t.Fatalf("expected 1 plan, got %d", len(plans))
	}
	if plans[0].MatchedTradeDate != "2026-04-17" || plans[0].NewPrice != 11 {
		t.Fatalf("unexpected fallback plan: %+v", plans[0])
	}
}

func TestBuildPlansForSymbol_UnresolvedAndFetcherError(t *testing.T) {
	now := time.Date(2026, 4, 18, 9, 0, 0, 0, shanghaiLocation)
	rows := []quadrant.RankingSnapshot{{ID: 1, Code: "00700", Exchange: "HKEX", SnapshotDate: "2026-04-16", ClosePrice: 0}}

	plans, unresolved, err := buildPlansForSymbol(context.Background(), stubDailyBarFetcher{
		barsBySymbol: map[string][]live.DailyBar{"00700.HK": {{Date: "2026-04-15", Close: 100}}},
	}, "00700.HK", rows, now, 10, 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(plans) != 0 || len(unresolved) != 1 {
		t.Fatalf("expected 0 plans and 1 unresolved, got plans=%d unresolved=%d", len(plans), len(unresolved))
	}

	_, _, err = buildPlansForSymbol(context.Background(), stubDailyBarFetcher{
		errBySymbol: map[string]error{"00700.HK": errors.New("network down")},
	}, "00700.HK", rows, now, 10, 3)
	if err == nil {
		t.Fatal("expected fetcher error")
	}
}

func TestQuerySnapshotCandidatesAndApplyBackfillPlans(t *testing.T) {
	db := testutil.InMemoryDB(t)
	testutil.AutoMigrateModels(t, db, &quadrant.RankingSnapshot{})
	ctx := context.Background()

	seed := []quadrant.RankingSnapshot{
		{ID: 1, Code: "600519", Exchange: "SSE", SnapshotDate: "2026-04-16", ClosePrice: 0},
		{ID: 2, Code: "000001", Exchange: "SZSE", SnapshotDate: "2026-04-16", ClosePrice: 0},
		{ID: 3, Code: "00700", Exchange: "HKEX", SnapshotDate: "2026-04-16", ClosePrice: 10},
	}
	if err := db.WithContext(ctx).Create(&seed).Error; err != nil {
		t.Fatalf("seed failed: %v", err)
	}

	rows, err := querySnapshotCandidates(ctx, db, cliOptions{Exchange: "ASHARE"})
	if err != nil {
		t.Fatalf("querySnapshotCandidates failed: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("expected 2 A-share zero-price rows, got %d", len(rows))
	}

	updated, err := applyBackfillPlans(ctx, db, []backfillPlan{{SnapshotID: 1, NewPrice: 123.45}})
	if err != nil {
		t.Fatalf("applyBackfillPlans failed: %v", err)
	}
	if updated != 1 {
		t.Fatalf("updated = %d, want 1", updated)
	}

	var row quadrant.RankingSnapshot
	if err := db.WithContext(ctx).First(&row, 1).Error; err != nil {
		t.Fatalf("reload row failed: %v", err)
	}
	if row.ClosePrice != 123.45 {
		t.Fatalf("close_price = %.2f, want 123.45", row.ClosePrice)
	}
}
