package quadrant

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"
)

func setupSimPortfolioV2Test(t *testing.T) (*Repository, *SimPortfolioV2Service) {
	t.Helper()
	repo, cleanup := setupQuadrantTest(t)
	t.Cleanup(cleanup)
	if err := repo.DB().AutoMigrate(&MarketCalendar{}, &SimPortfolioV2Definition{}, &SimPortfolioV2PipelineRun{}, &SimPortfolioV2PipelineDayStatus{}, &SimPortfolioV2SignalBatch{}, &SimPortfolioV2SignalItem{}, &SimPortfolioV2SelectionBatch{}, &SimPortfolioV2SelectionItem{}, &SimPortfolioV2PriceRequirement{}, &SimPortfolioV2PriceRepairAudit{}, &SimPortfolioV2PriceOverride{}, &SimPortfolioV2Daily{}, &SimPortfolioV2Position{}, &SimPortfolioV2Trade{}, &SimPortfolioV2Metrics{}, &SimPortfolioV2MarketConfig{}, &SimPortfolioV2Watermark{}); err != nil {
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

func TestSimPortfolioV2AdminCalendarsExposeMarketSplitAndPriceGap(t *testing.T) {
	repo, svc := setupSimPortfolioV2Test(t)
	ctx := context.Background()
	seedV2RankingSnapshots(t, repo, SimPortfolioV2MarketAShare, "2026-07-02", 4)
	svc.SetOpenPriceResolver(func(ctx context.Context, code string, exchange string, tradeDate string) float64 {
		if code == "600000" {
			return 0
		}
		return 10
	})
	svc.SetPriceLookupResolver(func(ctx context.Context, code string, exchange string, tradeDate string) PriceLookupResult {
		return PriceLookupResult{ClosePrice: 11, TradeDate: tradeDate}
	})
	if _, err := svc.Run(ctx, SimPortfolioV2RunRequest{Market: SimPortfolioV2MarketAShare, FromDate: "2026-07-02", ToDate: "2026-07-02"}); err != nil {
		t.Fatalf("Run: %v", err)
	}
	cal, err := svc.GetAdminCalendars(ctx, "2026-07")
	if err != nil {
		t.Fatalf("GetAdminCalendars: %v", err)
	}
	if len(cal.Markets) != 2 {
		t.Fatalf("markets = %d, want 2", len(cal.Markets))
	}
	var ashareDay *SimPortfolioV2CalendarDay
	for i := range cal.Markets {
		if cal.Markets[i].Market == SimPortfolioV2MarketAShare {
			for j := range cal.Markets[i].Days {
				if cal.Markets[i].Days[j].Date == "2026-07-02" {
					ashareDay = &cal.Markets[i].Days[j]
				}
			}
		}
	}
	if ashareDay == nil || ashareDay.OverallStatus != SimPortfolioV2StatusBlocked || ashareDay.BlockingCount == 0 {
		t.Fatalf("unexpected ashare day: %+v", ashareDay)
	}
	detail, err := svc.GetAdminCalendarDay(ctx, SimPortfolioV2MarketAShare, "2026-07-02")
	if err != nil {
		t.Fatalf("GetAdminCalendarDay: %v", err)
	}
	foundMissing := false
	for _, p := range detail.Portfolios {
		if p.EntryOpen.MissingCount > 0 && len(p.RepairSuggestions) > 0 {
			foundMissing = true
		}
	}
	if !foundMissing {
		t.Fatalf("expected missing price details, got %+v", detail.Portfolios)
	}
}

// TestSimPortfolioV2CalendarDaySuggestsBackfillWhenCandidatesExistButPriceMissing
// covers the regression where candidates exist (CandidateCount > 0) but their
// close price is missing/misaligned (MissingPriceCount > 0). Before the fix,
// GetAdminCalendarDay only surfaced the "recompute_quadrant" repair suggestion
// when CandidateCount == 0, so this common case ("缺收盘价:N" with no button)
// left the admin UI with no actionable repair suggestion at all.
func TestSimPortfolioV2CalendarDaySuggestsBackfillWhenCandidatesExistButPriceMissing(t *testing.T) {
	repo, svc := setupSimPortfolioV2Test(t)
	ctx := context.Background()
	date := "2026-07-06"
	now := time.Now().UTC()
	rows := []RankingSnapshot{}
	for i := 0; i < 7; i++ {
		// ClosePrice <= 0 simulates the "缺收盘价" scenario: candidates exist
		// but their close price has not been resolved yet.
		rows = append(rows, RankingSnapshot{Code: fmt.Sprintf("600%03d", i), Name: fmt.Sprintf("600%03d", i), Exchange: "SSE", Rank: i + 1, Opportunity: 90 - float64(i), Risk: 10 + float64(i), ClosePrice: 0, PriceTradeDate: "", SnapshotDate: date, CreatedAt: now})
	}
	if err := repo.UpsertSnapshots(ctx, rows); err != nil {
		t.Fatalf("seed snapshots: %v", err)
	}
	detail, err := svc.GetAdminCalendarDay(ctx, SimPortfolioV2MarketAShare, date)
	if err != nil {
		t.Fatalf("GetAdminCalendarDay: %v", err)
	}
	if detail.Signal.CandidateCount != 7 {
		t.Fatalf("candidate_count = %d, want 7", detail.Signal.CandidateCount)
	}
	if detail.Signal.MissingPriceCount != 7 {
		t.Fatalf("missing_price_count = %d, want 7", detail.Signal.MissingPriceCount)
	}
	if detail.Signal.Status != SimPortfolioV2StatusBlocked {
		t.Fatalf("signal status = %s, want blocked", detail.Signal.Status)
	}
	found := false
	for _, s := range detail.RepairSuggestions {
		if s.Type == "backfill_signal_close_price" {
			found = true
		}
		if s.Type == "recompute_quadrant" {
			t.Fatalf("did not expect recompute_quadrant when candidates already exist: %+v", detail.RepairSuggestions)
		}
	}
	if !found {
		t.Fatalf("expected backfill_signal_close_price repair suggestion, got %+v", detail.RepairSuggestions)
	}
}

func TestSimPortfolioV2StartDatePreviewRejectsFutureAndHoliday(t *testing.T) {
	_, svc := setupSimPortfolioV2Test(t)
	ctx := context.Background()
	future, err := svc.PreviewStartDate(ctx, SimPortfolioV2StartDatePreviewRequest{Market: SimPortfolioV2MarketAShare, StartSignalDate: "2099-01-01"})
	if err != nil {
		t.Fatalf("future preview: %v", err)
	}
	if future.CanApply {
		t.Fatalf("future date should not be applicable: %+v", future)
	}
	holiday, err := svc.PreviewStartDate(ctx, SimPortfolioV2StartDatePreviewRequest{Market: SimPortfolioV2MarketHKEX, StartSignalDate: "2026-07-01"})
	if err != nil {
		t.Fatalf("holiday preview: %v", err)
	}
	if holiday.CanApply {
		t.Fatalf("holiday should not be applicable: %+v", holiday)
	}
}

func TestSimPortfolioV2RetryResolvePricesUpdatesMissingRequirement(t *testing.T) {
	repo, svc := setupSimPortfolioV2Test(t)
	ctx := context.Background()
	seedV2RankingSnapshots(t, repo, SimPortfolioV2MarketAShare, "2026-07-02", 4)
	svc.SetOpenPriceResolver(func(ctx context.Context, code string, exchange string, tradeDate string) float64 { return 0 })
	svc.SetPriceLookupResolver(func(ctx context.Context, code string, exchange string, tradeDate string) PriceLookupResult {
		return PriceLookupResult{ClosePrice: 11, TradeDate: tradeDate}
	})
	if _, err := svc.Run(ctx, SimPortfolioV2RunRequest{Market: SimPortfolioV2MarketAShare, FromDate: "2026-07-02", ToDate: "2026-07-02"}); err != nil {
		t.Fatalf("Run: %v", err)
	}
	svc.SetOpenPriceResolver(func(ctx context.Context, code string, exchange string, tradeDate string) float64 {
		if tradeDate == "2026-07-03" {
			return 10
		}
		return 0
	})
	resp, err := svc.RetryResolvePrices(ctx, SimPortfolioV2PriceRepairRequest{Market: SimPortfolioV2MarketAShare, SignalDate: "2026-07-02", PriceType: SimPortfolioV2PriceTypeEntryOpen, OnlyMissing: true})
	if err != nil {
		t.Fatalf("RetryResolvePrices: %v", err)
	}
	if resp.Updated == 0 || resp.StillMissing != 0 || resp.AuditID == 0 {
		t.Fatalf("unexpected repair resp: %+v", resp)
	}
	reqs, err := repo.ListSimPortfolioV2PriceRequirementsForRepair(ctx, SimPortfolioV2MarketAShare, "2026-07-02", "", SimPortfolioV2PriceTypeEntryOpen, true)
	if err != nil {
		t.Fatalf("list reqs: %v", err)
	}
	if len(reqs) != 0 {
		t.Fatalf("expected no missing entry open after retry, got %d", len(reqs))
	}
}

func TestSimPortfolioV2BackfillDailyBarsUpdatesPriceRequirement(t *testing.T) {
	repo, svc := setupSimPortfolioV2Test(t)
	ctx := context.Background()
	seedV2RankingSnapshots(t, repo, SimPortfolioV2MarketAShare, "2026-07-02", 4)
	svc.SetOpenPriceResolver(func(ctx context.Context, code string, exchange string, tradeDate string) float64 { return 0 })
	svc.SetPriceLookupResolver(func(ctx context.Context, code string, exchange string, tradeDate string) PriceLookupResult {
		return PriceLookupResult{ClosePrice: 11, TradeDate: tradeDate}
	})
	if _, err := svc.Run(ctx, SimPortfolioV2RunRequest{Market: SimPortfolioV2MarketAShare, FromDate: "2026-07-02", ToDate: "2026-07-02"}); err != nil {
		t.Fatalf("Run: %v", err)
	}
	svc.SetDailyBarFetcher(func(ctx context.Context, code string, exchange string, lookbackDays int) ([]SimPortfolioV2DailyBar, error) {
		return []SimPortfolioV2DailyBar{{Date: "2026-07-03", Open: 9.8, Close: 10.8}}, nil
	})
	resp, err := svc.BackfillDailyBars(ctx, SimPortfolioV2PriceBackfillRequest{Market: SimPortfolioV2MarketAShare, SignalDate: "2026-07-02", PriceType: SimPortfolioV2PriceTypeEntryOpen, OnlyMissing: true})
	if err != nil {
		t.Fatalf("BackfillDailyBars: %v", err)
	}
	if resp.Backfilled == 0 || resp.StillMissing != 0 || resp.AuditID == 0 {
		t.Fatalf("unexpected backfill resp: %+v", resp)
	}
	reqs, _ := repo.ListSimPortfolioV2PriceRequirementsForRepair(ctx, SimPortfolioV2MarketAShare, "2026-07-02", "", SimPortfolioV2PriceTypeEntryOpen, false)
	for _, req := range reqs {
		if req.Status != SimPortfolioV2PriceStatusSatisfied || req.Source != "daily_bar_backfill" || req.Price != 9.8 {
			t.Fatalf("unexpected req after backfill: %+v", req)
		}
	}
}

func TestSimPortfolioV2ManualOverrideRequiresAuditAndUpdatesRequirement(t *testing.T) {
	repo, svc := setupSimPortfolioV2Test(t)
	ctx := context.Background()
	seedV2RankingSnapshots(t, repo, SimPortfolioV2MarketAShare, "2026-07-02", 4)
	svc.SetOpenPriceResolver(func(ctx context.Context, code string, exchange string, tradeDate string) float64 { return 0 })
	svc.SetPriceLookupResolver(func(ctx context.Context, code string, exchange string, tradeDate string) PriceLookupResult {
		return PriceLookupResult{ClosePrice: 11, TradeDate: tradeDate}
	})
	if _, err := svc.Run(ctx, SimPortfolioV2RunRequest{Market: SimPortfolioV2MarketAShare, FromDate: "2026-07-02", ToDate: "2026-07-02"}); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if _, err := svc.OverridePrice(ctx, SimPortfolioV2PriceOverrideRequest{Market: SimPortfolioV2MarketAShare, SignalDate: "2026-07-02", Code: "600000", Exchange: "SSE", TradeDate: "2026-07-03", PriceType: SimPortfolioV2PriceTypeEntryOpen, Price: 9.9, Confirm: true}); err == nil {
		t.Fatalf("expected reason/evidence validation error")
	}
	resp, err := svc.OverridePrice(ctx, SimPortfolioV2PriceOverrideRequest{Market: SimPortfolioV2MarketAShare, SignalDate: "2026-07-02", Code: "600000", Exchange: "SSE", TradeDate: "2026-07-03", PriceType: SimPortfolioV2PriceTypeEntryOpen, Price: 9.9, Reason: "行情源短缺，人工核对", Evidence: "broker-ticket-1", Confirm: true})
	if err != nil {
		t.Fatalf("OverridePrice: %v", err)
	}
	if resp.Updated == 0 || resp.AuditID == 0 {
		t.Fatalf("unexpected override resp: %+v", resp)
	}
	override, err := repo.GetSimPortfolioV2PriceOverride(ctx, SimPortfolioV2MarketAShare, "600000", "SSE", "2026-07-03", SimPortfolioV2PriceTypeEntryOpen)
	if err != nil || override == nil || override.Price != 9.9 || override.AuditID == 0 {
		t.Fatalf("unexpected override row: %+v err=%v", override, err)
	}
	reqs, _ := repo.ListSimPortfolioV2PriceRequirementsForRepair(ctx, SimPortfolioV2MarketAShare, "2026-07-02", "", SimPortfolioV2PriceTypeEntryOpen, false)
	found := false
	for _, req := range reqs {
		if req.Code == "600000" {
			found = req.Status == SimPortfolioV2PriceStatusSatisfied && req.Source == "admin_override" && req.Price == 9.9
		}
	}
	if !found {
		t.Fatalf("expected overridden requirement in %+v", reqs)
	}
}

// TestSimPortfolioV2RebuildPreservesBackfilledPrices verifies that after
// BackfillDailyBars satisfies a price requirement, a subsequent Run (e.g.
// triggered by ApplyStartDate) does NOT regress those rows back to "missing".
// This is the core regression test for the "rebuild discards backfilled
// prices" bug.
func TestSimPortfolioV2RebuildPreservesBackfilledPrices(t *testing.T) {
	repo, svc := setupSimPortfolioV2Test(t)
	ctx := context.Background()
	seedV2RankingSnapshots(t, repo, SimPortfolioV2MarketAShare, "2026-07-02", 4)

	// First run: open price resolver returns 0 → entry_open blocked.
	// priceLookupResolver returns a valid close → valuation_close satisfied.
	svc.SetOpenPriceResolver(func(ctx context.Context, code string, exchange string, tradeDate string) float64 { return 0 })
	svc.SetPriceLookupResolver(func(ctx context.Context, code string, exchange string, tradeDate string) PriceLookupResult {
		return PriceLookupResult{ClosePrice: 11, TradeDate: tradeDate}
	})
	if _, err := svc.Run(ctx, SimPortfolioV2RunRequest{Market: SimPortfolioV2MarketAShare, FromDate: "2026-07-02", ToDate: "2026-07-02"}); err != nil {
		t.Fatalf("first Run: %v", err)
	}

	// Backfill entry_open prices via daily bars.
	svc.SetDailyBarFetcher(func(ctx context.Context, code string, exchange string, lookbackDays int) ([]SimPortfolioV2DailyBar, error) {
		return []SimPortfolioV2DailyBar{{Date: "2026-07-03", Open: 9.8, Close: 10.8}}, nil
	})
	if _, err := svc.BackfillDailyBars(ctx, SimPortfolioV2PriceBackfillRequest{Market: SimPortfolioV2MarketAShare, SignalDate: "2026-07-02", PriceType: SimPortfolioV2PriceTypeEntryOpen, OnlyMissing: true}); err != nil {
		t.Fatalf("BackfillDailyBars: %v", err)
	}

	// Verify all entry_open rows are now satisfied.
	reqs, _ := repo.ListSimPortfolioV2PriceRequirementsForRepair(ctx, SimPortfolioV2MarketAShare, "2026-07-02", "", SimPortfolioV2PriceTypeEntryOpen, false)
	for _, req := range reqs {
		if req.Status != SimPortfolioV2PriceStatusSatisfied {
			t.Fatalf("expected satisfied after backfill, got %+v", req)
		}
	}

	// Second run (simulates ApplyStartDate → Run rebuild).
	// openPriceResolver still returns 0 — WITHOUT the fix, the rebuild would
	// delete the backfilled rows and they'd regress to "missing".
	if _, err := svc.Run(ctx, SimPortfolioV2RunRequest{Market: SimPortfolioV2MarketAShare, FromDate: "2026-07-02", ToDate: "2026-07-02"}); err != nil {
		t.Fatalf("second Run: %v", err)
	}

	// Verify entry_open rows are STILL satisfied after rebuild.
	reqs, _ = repo.ListSimPortfolioV2PriceRequirementsForRepair(ctx, SimPortfolioV2MarketAShare, "2026-07-02", "", SimPortfolioV2PriceTypeEntryOpen, false)
	if len(reqs) == 0 {
		t.Fatalf("expected price requirements to exist after rebuild, got 0")
	}
	for _, req := range reqs {
		if req.Status != SimPortfolioV2PriceStatusSatisfied || req.Price != 9.8 {
			t.Fatalf("expected satisfied price=9.8 preserved after rebuild, got %+v", req)
		}
	}
}

// TestSimPortfolioV2DefinitionsExcludeStarBoardForAShareOnly is the
// regression test for the "ExcludedBoards missing on v2 A-share
// definitions" bug: spv2_ashare_a/spv2_ashare_b must exclude the STAR
// (科创板) board, while HKEX definitions must NOT declare any excluded
// board (港股不涉及科创板).
func TestSimPortfolioV2DefinitionsExcludeStarBoardForAShareOnly(t *testing.T) {
	defs := defaultSimPortfolioV2Definitions(time.Now().UTC())
	got := map[string]string{}
	for _, def := range defs {
		got[def.ID] = def.ExcludedBoards
	}
	// A-share portfolios must explicitly exclude the STAR (科创板) board.
	wantAShare := mustMarshal([]string{aShareBoardStar})
	for _, id := range []string{"spv2_ashare_a", "spv2_ashare_b"} {
		if got[id] != wantAShare {
			t.Fatalf("definition %s excluded_boards = %q, want %q", id, got[id], wantAShare)
		}
	}
	// HKEX portfolios have no STAR board concept and must NOT declare any
	// excluded board (kept as-is per business decision).
	excluded := decodeSimPortfolioExcludedBoards("")
	for _, id := range []string{"spv2_hkex_a", "spv2_hkex_b"} {
		if hkexExcluded := decodeSimPortfolioExcludedBoards(got[id]); len(hkexExcluded) != len(excluded) {
			t.Fatalf("definition %s excluded_boards = %q, want no excluded boards", id, got[id])
		}
		if _, ok := decodeSimPortfolioExcludedBoards(got[id])[aShareBoardStar]; ok {
			t.Fatalf("definition %s must not exclude STAR board", id)
		}
	}
}

// TestSimPortfolioV2SelectionExcludesStarBoardForAShare verifies the
// end-to-end selection path: when the A-share signal batch contains STAR
// (688/689) board candidates mixed with MAIN board candidates, the A-share
// portfolio A definition (top4) must skip the STAR candidates entirely.
func TestSimPortfolioV2SelectionExcludesStarBoardForAShare(t *testing.T) {
	repo, svc := setupSimPortfolioV2Test(t)
	ctx := context.Background()
	if err := repo.EnsureSimPortfolioV2Definitions(ctx); err != nil {
		t.Fatalf("EnsureSimPortfolioV2Definitions: %v", err)
	}
	now := time.Now().UTC()
	rows := []RankingSnapshot{
		{Code: "688001", Name: "star1", Exchange: "SSE", Rank: 1, Opportunity: 95, Risk: 5, ClosePrice: 10, PriceTradeDate: "2026-07-02", SnapshotDate: "2026-07-02", CreatedAt: now},
		{Code: "689001", Name: "star2", Exchange: "SSE", Rank: 2, Opportunity: 94, Risk: 6, ClosePrice: 10, PriceTradeDate: "2026-07-02", SnapshotDate: "2026-07-02", CreatedAt: now},
		{Code: "600002", Name: "main1", Exchange: "SSE", Rank: 3, Opportunity: 93, Risk: 7, ClosePrice: 10, PriceTradeDate: "2026-07-02", SnapshotDate: "2026-07-02", CreatedAt: now},
		{Code: "600003", Name: "main2", Exchange: "SSE", Rank: 4, Opportunity: 92, Risk: 8, ClosePrice: 10, PriceTradeDate: "2026-07-02", SnapshotDate: "2026-07-02", CreatedAt: now},
		{Code: "600004", Name: "main3", Exchange: "SSE", Rank: 5, Opportunity: 91, Risk: 9, ClosePrice: 10, PriceTradeDate: "2026-07-02", SnapshotDate: "2026-07-02", CreatedAt: now},
		{Code: "600005", Name: "main4", Exchange: "SSE", Rank: 6, Opportunity: 90, Risk: 10, ClosePrice: 10, PriceTradeDate: "2026-07-02", SnapshotDate: "2026-07-02", CreatedAt: now},
	}
	if err := repo.UpsertSnapshots(ctx, rows); err != nil {
		t.Fatalf("seed snapshots: %v", err)
	}
	if _, err := svc.buildSignalBatch(ctx, "", SimPortfolioV2MarketAShare, "2026-07-02"); err != nil {
		t.Fatalf("buildSignalBatch: %v", err)
	}
	defs, err := repo.ListActiveSimPortfolioV2Definitions(ctx)
	if err != nil {
		t.Fatalf("ListActiveSimPortfolioV2Definitions: %v", err)
	}
	var defA SimPortfolioV2Definition
	found := false
	for _, def := range defs {
		if def.ID == "spv2_ashare_a" {
			defA = def
			found = true
		}
	}
	if !found {
		t.Fatalf("spv2_ashare_a definition not found")
	}
	items, err := svc.selectV2Items(ctx, defA, "2026-07-02")
	if err != nil {
		t.Fatalf("selectV2Items: %v", err)
	}
	if len(items) != 4 {
		t.Fatalf("selected items = %d, want 4: %+v", len(items), items)
	}
	for _, item := range items {
		if strings.HasPrefix(item.Code, "688") || strings.HasPrefix(item.Code, "689") {
			t.Fatalf("expected STAR board code to be excluded, got %+v", items)
		}
	}
}

// TestSimPortfolioV2ResolveUsesDailyBarFallback verifies that
// resolveSinglePriceRequirement falls back to dailyBarFetcher when all other
// resolvers return 0.  This ensures the pipeline can resolve prices from the
// market API even when the local snapshot table has no data for the target
// trade date.
func TestSimPortfolioV2ResolveUsesDailyBarFallback(t *testing.T) {
	repo, svc := setupSimPortfolioV2Test(t)
	ctx := context.Background()
	seedV2RankingSnapshots(t, repo, SimPortfolioV2MarketAShare, "2026-07-02", 4)

	// All resolvers return 0 — only the dailyBarFetcher can provide prices.
	svc.SetOpenPriceResolver(func(ctx context.Context, code string, exchange string, tradeDate string) float64 { return 0 })
	svc.SetPriceLookupResolver(func(ctx context.Context, code string, exchange string, tradeDate string) PriceLookupResult {
		return PriceLookupResult{}
	})
	svc.SetPriceResolver(func(ctx context.Context, code string, exchange string, tradeDate string) float64 { return 0 })
	svc.SetDailyBarFetcher(func(ctx context.Context, code string, exchange string, lookbackDays int) ([]SimPortfolioV2DailyBar, error) {
		return []SimPortfolioV2DailyBar{
			{Date: "2026-07-03", Open: 9.8, Close: 10.8},
		}, nil
	})

	resp, err := svc.Run(ctx, SimPortfolioV2RunRequest{Market: SimPortfolioV2MarketAShare, FromDate: "2026-07-02", ToDate: "2026-07-02"})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if resp.GeneratedFacts < 1 {
		t.Fatalf("expected facts generated via daily_bar_fallback, got %+v", resp)
	}

	// Verify that the resolved prices came from daily_bar_fallback.
	reqs, _ := repo.ListSimPortfolioV2PriceRequirementsForRepair(ctx, SimPortfolioV2MarketAShare, "2026-07-02", "", "", false)
	foundFallback := false
	for _, req := range reqs {
		if req.Source == "daily_bar_fallback" && req.Status == SimPortfolioV2PriceStatusSatisfied {
			foundFallback = true
		}
	}
	if !foundFallback {
		t.Fatalf("expected at least one requirement resolved via daily_bar_fallback, got %+v", reqs)
	}
}
