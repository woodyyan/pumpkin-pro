package quadrant

import (
	"context"
	"fmt"
	"testing"
	"time"
)

func setupSimPortfolioV2Test(t *testing.T) (*Repository, *SimPortfolioV2Service) {
	t.Helper()
	repo, cleanup := setupQuadrantTest(t)
	t.Cleanup(cleanup)
	if err := repo.DB().AutoMigrate(&MarketCalendar{}, &SimPortfolioV2Definition{}, &SimPortfolioV2PipelineRun{}, &SimPortfolioV2PipelineDayStatus{}, &SimPortfolioV2SignalBatch{}, &SimPortfolioV2SignalItem{}, &SimPortfolioV2SelectionBatch{}, &SimPortfolioV2SelectionItem{}, &SimPortfolioV2PriceRequirement{}, &SimPortfolioV2Daily{}, &SimPortfolioV2Position{}, &SimPortfolioV2Trade{}, &SimPortfolioV2Metrics{}, &SimPortfolioV2Watermark{}); err != nil {
		t.Fatalf("migrate v2: %v", err)
	}
	svc := NewSimPortfolioV2Service(repo)
	return repo, svc
}

func seedV2RankingSnapshots(t *testing.T, repo *Repository, market string, date string, count int) {
	t.Helper()
	now := time.Now().UTC()
	rows := []RankingSnapshot{}
	for i := 0; i < count; i++ {
		code := fmt.Sprintf("600%03d", i)
		ex := "SSE"
		if market == SimPortfolioV2MarketHKEX {
			code = fmt.Sprintf("00%03d", i+1)
			ex = "HKEX"
		}
		rows = append(rows, RankingSnapshot{Code: code, Name: code, Exchange: ex, Rank: i + 1, Opportunity: 90 - float64(i), Risk: 10 + float64(i), ClosePrice: 10 + float64(i), PriceTradeDate: date, SnapshotDate: date, CreatedAt: now})
	}
	if err := repo.UpsertSnapshots(context.Background(), rows); err != nil {
		t.Fatalf("seed snapshots: %v", err)
	}
}

func TestSimPortfolioV2CalendarSkipsHKEXHoliday(t *testing.T) {
	_, svc := setupSimPortfolioV2Test(t)
	ctx := context.Background()
	resp, err := svc.Run(ctx, SimPortfolioV2RunRequest{Market: SimPortfolioV2MarketHKEX, FromDate: "2026-07-01", ToDate: "2026-07-01"})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if resp.BlockedDays != 0 || resp.GeneratedFacts != 0 {
		t.Fatalf("unexpected resp: %+v", resp)
	}
	days, err := svc.GetAdminDays(ctx, SimPortfolioV2MarketHKEX, "2026-07-01", "2026-07-01")
	if err != nil {
		t.Fatalf("GetAdminDays: %v", err)
	}
	if len(days.Items) != 1 || days.Items[0].Stage != SimPortfolioV2StageCalendar || days.Items[0].Status != SimPortfolioV2StatusSkipped {
		t.Fatalf("unexpected days: %+v", days.Items)
	}
}

func TestSimPortfolioV2StrictModeBlocksMissingSignal(t *testing.T) {
	_, svc := setupSimPortfolioV2Test(t)
	ctx := context.Background()
	resp, err := svc.Run(ctx, SimPortfolioV2RunRequest{Market: SimPortfolioV2MarketAShare, FromDate: "2026-07-02", ToDate: "2026-07-02"})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if resp.BlockedDays == 0 {
		t.Fatalf("expected blocked day, got %+v", resp)
	}
	days, _ := svc.GetAdminDays(ctx, SimPortfolioV2MarketAShare, "2026-07-02", "2026-07-02")
	found := false
	for _, item := range days.Items {
		if item.Stage == SimPortfolioV2StageSignal && item.Status == SimPortfolioV2StatusBlocked {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected blocked signal status, got %+v", days.Items)
	}
}

func TestSimPortfolioV2GeneratesVerifiedFacts(t *testing.T) {
	repo, svc := setupSimPortfolioV2Test(t)
	ctx := context.Background()
	seedV2RankingSnapshots(t, repo, SimPortfolioV2MarketAShare, "2026-07-02", 4)
	svc.SetOpenPriceResolver(func(ctx context.Context, code string, exchange string, tradeDate string) float64 {
		if tradeDate == "2026-07-03" {
			return 10
		}
		return 0
	})
	svc.SetPriceLookupResolver(func(ctx context.Context, code string, exchange string, tradeDate string) PriceLookupResult {
		if tradeDate == "2026-07-03" {
			return PriceLookupResult{ClosePrice: 11, TradeDate: tradeDate}
		}
		return PriceLookupResult{}
	})
	resp, err := svc.Run(ctx, SimPortfolioV2RunRequest{Market: SimPortfolioV2MarketAShare, FromDate: "2026-07-02", ToDate: "2026-07-02"})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if resp.GeneratedFacts < 1 {
		t.Fatalf("generated facts = %d, want at least 1", resp.GeneratedFacts)
	}
	overview, err := svc.GetPortfolioOverview(ctx)
	if err != nil {
		t.Fatalf("overview: %v", err)
	}
	if len(overview.Items) != 4 {
		t.Fatalf("overview items = %d", len(overview.Items))
	}
	if overview.AsOfTradeDate != "2026-07-03" {
		t.Fatalf("as_of = %s", overview.AsOfTradeDate)
	}
}
