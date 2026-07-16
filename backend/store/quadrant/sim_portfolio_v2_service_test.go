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

func TestSimPortfolioV2SignalBatchRebuildsAfterInvalidation(t *testing.T) {
	repo, svc := setupSimPortfolioV2Test(t)
	ctx := context.Background()
	const date = "2026-07-02"
	now := time.Now().UTC()
	stale := SimPortfolioV2SignalBatch{
		ID: "sig-ashare-2026-07-02", Market: SimPortfolioV2MarketAShare, SourceTradeDate: date,
		ComputedAt: now, Status: SimPortfolioV2StatusBlocked, Message: "缺少模拟组合信号快照。",
		CreatedAt: now, UpdatedAt: now,
	}
	if err := repo.ReplaceSimPortfolioV2SignalBatch(ctx, stale, nil); err != nil {
		t.Fatalf("seed stale signal batch: %v", err)
	}
	seedV2RankingSnapshots(t, repo, SimPortfolioV2MarketAShare, date, 4)

	if err := repo.DeleteSimPortfolioV2SignalBatch(ctx, SimPortfolioV2MarketAShare, date); err != nil {
		t.Fatalf("DeleteSimPortfolioV2SignalBatch: %v", err)
	}
	if batch, err := repo.GetSimPortfolioV2SignalBatch(ctx, SimPortfolioV2MarketAShare, date); err != nil || batch != nil {
		t.Fatalf("expected invalidated batch to be absent, got batch=%+v err=%v", batch, err)
	}

	detail, err := svc.GetAdminCalendarDay(ctx, SimPortfolioV2MarketAShare, date)
	if err != nil {
		t.Fatalf("GetAdminCalendarDay: %v", err)
	}
	if detail.Signal.Status != SimPortfolioV2StatusOK || detail.Signal.CandidateCount != 4 || detail.Signal.MissingPriceCount != 0 {
		t.Fatalf("expected rebuilt ready signal batch, got %+v", detail.Signal)
	}
	if len(detail.RepairSuggestions) != 0 {
		t.Fatalf("expected no signal repair suggestion, got %+v", detail.RepairSuggestions)
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

// TestSimPortfolioV2BackfillSignalClosePriceUpdatesOnlyMissing verifies the
// precise close-price backfill: when candidates already exist (e.g. 50 of them)
// but only a few are missing their close price, the targeted backfill fills in
// ONLY the missing ones (leaving the already-satisfied snapshots untouched) and
// flips the day's signal batch back to "ok" — without a full quadrant recompute.
func TestSimPortfolioV2BackfillSignalClosePriceUpdatesOnlyMissing(t *testing.T) {
	repo, svc := setupSimPortfolioV2Test(t)
	ctx := context.Background()
	date := "2026-07-06"
	now := time.Now().UTC()
	// Seed 50 candidates; the first 4 are missing their close price, the rest
	// already have a valid close price aligned to the trading day.
	rows := []RankingSnapshot{}
	for i := 0; i < 50; i++ {
		close := 10 + float64(i)
		priceDate := date
		if i < 4 {
			close = 0
			priceDate = ""
		}
		rows = append(rows, RankingSnapshot{Code: fmt.Sprintf("600%03d", i), Name: fmt.Sprintf("600%03d", i), Exchange: "SSE", Rank: i + 1, Opportunity: 90 - float64(i), Risk: 10 + float64(i), ClosePrice: close, PriceTradeDate: priceDate, SnapshotDate: date, CreatedAt: now})
	}
	if err := repo.UpsertSnapshots(ctx, rows); err != nil {
		t.Fatalf("seed snapshots: %v", err)
	}

	// Sanity: the calendar day should report 4 missing prices and be blocked.
	detail, err := svc.GetAdminCalendarDay(ctx, SimPortfolioV2MarketAShare, date)
	if err != nil {
		t.Fatalf("GetAdminCalendarDay: %v", err)
	}
	if detail.Signal.MissingPriceCount != 4 || detail.Signal.CandidateCount != 50 {
		t.Fatalf("pre-backfill signal = %+v, want candidate=50 missing=4", detail.Signal)
	}

	closeResolverCalls := 0
	svc.SetPriceResolver(func(ctx context.Context, code string, exchange string, tradeDate string) float64 {
		closeResolverCalls++
		return 12.5
	})

	resp, err := svc.BackfillSignalClosePrice(ctx, SimPortfolioV2SignalClosePriceBackfillRequest{Market: SimPortfolioV2MarketAShare, Date: date})
	if err != nil {
		t.Fatalf("BackfillSignalClosePrice: %v", err)
	}
	// Only the 4 missing candidates should be checked/updated, NOT all 50.
	if resp.Checked != 4 || resp.Updated != 4 || resp.StillMissing != 0 {
		t.Fatalf("unexpected backfill resp: %+v", resp)
	}
	if !resp.OK || resp.Status != SimPortfolioV2StatusOK {
		t.Fatalf("expected ok status, got %+v", resp)
	}
	if closeResolverCalls != 4 {
		t.Fatalf("price resolver should be called exactly 4 times (only missing), got %d", closeResolverCalls)
	}

	// After backfill, the day should no longer be blocked / missing any price.
	detail, err = svc.GetAdminCalendarDay(ctx, SimPortfolioV2MarketAShare, date)
	if err != nil {
		t.Fatalf("GetAdminCalendarDay after backfill: %v", err)
	}
	if detail.Signal.MissingPriceCount != 0 {
		t.Fatalf("expected missing_price_count=0 after backfill, got %d", detail.Signal.MissingPriceCount)
	}
	// The already-satisfied snapshots (index 4..49) must keep their original prices.
	after, _ := repo.ListRankingSnapshotsByDate(ctx, SimPortfolioV2MarketAShare, date, 50)
	for _, row := range after {
		if row.ClosePrice <= 0 {
			t.Fatalf("snapshot %s still missing close price after backfill", row.Code)
		}
	}
}

// TestSimPortfolioV2BackfillSignalClosePriceReportsStillMissing verifies the
// degraded path: when a stock's close price cannot be resolved from any source,
// the day stays blocked and the response reports the still-missing count so the
// admin UI can fall back to full recompute / manual override.
func TestSimPortfolioV2BackfillSignalClosePriceReportsStillMissing(t *testing.T) {
	repo, svc := setupSimPortfolioV2Test(t)
	ctx := context.Background()
	date := "2026-07-06"
	now := time.Now().UTC()
	rows := []RankingSnapshot{}
	for i := 0; i < 3; i++ {
		rows = append(rows, RankingSnapshot{Code: fmt.Sprintf("600%03d", i), Name: fmt.Sprintf("600%03d", i), Exchange: "SSE", Rank: i + 1, Opportunity: 90 - float64(i), Risk: 10 + float64(i), ClosePrice: 0, PriceTradeDate: "", SnapshotDate: date, CreatedAt: now})
	}
	if err := repo.UpsertSnapshots(ctx, rows); err != nil {
		t.Fatalf("seed snapshots: %v", err)
	}
	// No resolver / fetcher configured -> nothing can be backfilled.
	resp, err := svc.BackfillSignalClosePrice(ctx, SimPortfolioV2SignalClosePriceBackfillRequest{Market: SimPortfolioV2MarketAShare, Date: date})
	if err != nil {
		t.Fatalf("BackfillSignalClosePrice: %v", err)
	}
	if resp.Checked != 3 || resp.Updated != 0 || resp.StillMissing != 3 {
		t.Fatalf("unexpected resp: %+v", resp)
	}
	if resp.OK || resp.Status != SimPortfolioV2StatusBlocked {
		t.Fatalf("expected blocked status when nothing resolvable, got %+v", resp)
	}
	if len(resp.MissingItems) != 3 {
		t.Fatalf("expected 3 missing items, got %+v", resp.MissingItems)
	}
}

// TestSimPortfolioV2BackfillSignalClosePriceRejectsEmptyCandidates verifies that
// when there are no candidate snapshots at all, the backfill refuses and points
// the admin to a full quadrant rebuild instead.
func TestSimPortfolioV2BackfillSignalClosePriceRejectsEmptyCandidates(t *testing.T) {
	_, svc := setupSimPortfolioV2Test(t)
	ctx := context.Background()
	resp, err := svc.BackfillSignalClosePrice(ctx, SimPortfolioV2SignalClosePriceBackfillRequest{Market: SimPortfolioV2MarketAShare, Date: "2026-07-06"})
	if err != nil {
		t.Fatalf("BackfillSignalClosePrice: %v", err)
	}
	if resp.OK || resp.Candidates != 0 || resp.RequiresRerun {
		t.Fatalf("expected refusal for empty candidates, got %+v", resp)
	}
	if !strings.Contains(resp.Message, "重建该日四象限") {
		t.Fatalf("expected message to suggest quadrant rebuild, got %q", resp.Message)
	}
}

// TestSimPortfolioV2GenerateFactsRebalanceDiff verifies that generateFacts
// produces SELL / HOLD / BUY trades by diffing the previous day's positions
// against the current day's selection.  This is a regression test for the bug
// where all trades were unconditionally written as BUY.
func TestSimPortfolioV2GenerateFactsRebalanceDiff(t *testing.T) {
	repo, svc := setupSimPortfolioV2Test(t)
	ctx := context.Background()
	now := time.Now().UTC()

	def := SimPortfolioV2Definition{
		ID:          "test-port-A",
		Code:        "TPA",
		Name:        "Test Portfolio A",
		Market:      SimPortfolioV2MarketAShare,
		MaxHoldings: 4,
		InitialAssets: 1_000_000,
		IsActive:    true,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	if err := repo.db.Create(&def).Error; err != nil {
		t.Fatalf("create definition: %v", err)
	}

	// --- Day 1: seed a completed daily + positions for stocks A/B/C/D ---
	day1Date := "2026-07-06"
	day1Daily := SimPortfolioV2Daily{
		PortfolioID: def.ID, Market: def.Market, TradeDate: day1Date,
		NAV: 1.0, TotalAssets: 1_000_000, PreviousAssets: 1_000_000,
		DailyReturn: 0, TotalReturn: 0, PositionCount: 4,
		Rebalance: true, Status: "verified", ComputedAt: now, CreatedAt: now, UpdatedAt: now,
	}
	if err := repo.db.Create(&day1Daily).Error; err != nil {
		t.Fatalf("create day1 daily: %v", err)
	}
	day1Stocks := []string{"600001", "600002", "600003", "600004"}
	for i, code := range day1Stocks {
		pos := SimPortfolioV2Position{
			PortfolioID: def.ID, Market: def.Market, TradeDate: day1Date,
			Code: code, Exchange: "SSE", Name: "S" + code, Rank: i + 1,
			Weight: 0.25, TargetValue: 250_000, Shares: 25000, BuyPrice: 10, ClosePrice: 10,
			MarketValue: 250_000, Profit: 0, ProfitRate: 0, CreatedAt: now, UpdatedAt: now,
		}
		if err := repo.db.Create(&pos).Error; err != nil {
			t.Fatalf("create day1 position %s: %v", code, err)
		}
	}

	// --- Day 2: selection = C/D (hold) + E/F (buy); A/B dropped (sell) ---
	signalDate2 := "2026-07-07"
	tradeDate2 := "2026-07-08"

	selItems := []SimPortfolioV2SelectionItem{
		{SelectionBatchID: "sel-2", PortfolioID: def.ID, SignalDate: signalDate2, Code: "600003", Exchange: "SSE", Name: "S600003", Rank: 1, Weight: 0.25, CreatedAt: now},
		{SelectionBatchID: "sel-2", PortfolioID: def.ID, SignalDate: signalDate2, Code: "600004", Exchange: "SSE", Name: "S600004", Rank: 2, Weight: 0.25, CreatedAt: now},
		{SelectionBatchID: "sel-2", PortfolioID: def.ID, SignalDate: signalDate2, Code: "600005", Exchange: "SSE", Name: "S600005", Rank: 3, Weight: 0.25, CreatedAt: now},
		{SelectionBatchID: "sel-2", PortfolioID: def.ID, SignalDate: signalDate2, Code: "600006", Exchange: "SSE", Name: "S600006", Rank: 4, Weight: 0.25, CreatedAt: now},
	}
	batch := SimPortfolioV2SelectionBatch{ID: "sel-2", PortfolioID: def.ID, Market: def.Market, SignalDate: signalDate2, EntryTradeDate: tradeDate2, Status: SimPortfolioV2StatusOK, SelectedCount: 4, CreatedAt: now, UpdatedAt: now}
	if err := repo.ReplaceSimPortfolioV2Selection(ctx, batch, selItems); err != nil {
		t.Fatalf("seed selection: %v", err)
	}

	// Price requirements for day-2 selected stocks (entry_open + valuation_close).
	priceReqs := []SimPortfolioV2PriceRequirement{}
	for _, item := range selItems {
		for _, pt := range []string{SimPortfolioV2PriceTypeEntryOpen, SimPortfolioV2PriceTypeValuationClose} {
			priceReqs = append(priceReqs, SimPortfolioV2PriceRequirement{
				PortfolioID: def.ID, Market: def.Market, SignalDate: signalDate2,
				TradeDate: tradeDate2, Code: item.Code, Exchange: item.Exchange,
				PriceType: pt, Required: true, Status: SimPortfolioV2PriceStatusSatisfied,
				Price: 10.0, PriceTradeDate: tradeDate2, Source: "test",
				ResolvedAt: now, CreatedAt: now, UpdatedAt: now,
			})
		}
	}
	if err := repo.ReplaceSimPortfolioV2PriceRequirements(ctx, def.ID, signalDate2, priceReqs); err != nil {
		t.Fatalf("seed price reqs: %v", err)
	}

	// Set openPriceResolver so SELL trades can get a trade price for dropped stocks.
	svc.SetOpenPriceResolver(func(ctx context.Context, code, exchange, td string) float64 {
		return 10.5
	})

	if err := svc.generateFacts(ctx, def, signalDate2, tradeDate2); err != nil {
		t.Fatalf("generateFacts: %v", err)
	}

	// Verify trades.
	tradeRows, err := repo.ListSimPortfolioV2Trades(ctx, def.ID, "", "", "")
	if err != nil {
		t.Fatalf("list trades: %v", err)
	}
	if len(tradeRows) != 6 {
		t.Fatalf("expected 6 trades (2 sell + 2 hold + 2 buy), got %d", len(tradeRows))
	}

	byCode := map[string]SimPortfolioV2Trade{}
	for _, tr := range tradeRows {
		byCode[tr.Code] = tr
	}

	// SELL: 600001, 600002 — dropped from portfolio.
	for _, code := range []string{"600001", "600002"} {
		tr := byCode[code]
		if tr.Action != simPortfolioActionSell {
			t.Errorf("%s: action=%s, want SELL", code, tr.Action)
		}
		if tr.OldWeight != 0.25 || tr.OldShares != 25000 {
			t.Errorf("%s: oldWeight=%v oldShares=%v, want 0.25 / 25000", code, tr.OldWeight, tr.OldShares)
		}
		if tr.NewWeight != 0 || tr.NewShares != 0 {
			t.Errorf("%s: newWeight=%v newShares=%v, want 0 / 0", code, tr.NewWeight, tr.NewShares)
		}
		if tr.ShareDelta != -25000 {
			t.Errorf("%s: shareDelta=%v, want -25000", code, tr.ShareDelta)
		}
		if tr.Reason != simPortfolioReasonDropTop4 {
			t.Errorf("%s: reason=%s, want %s", code, tr.Reason, simPortfolioReasonDropTop4)
		}
	}

	// HOLD: 600003, 600004 — stayed in portfolio.
	for _, code := range []string{"600003", "600004"} {
		tr := byCode[code]
		if tr.Action != simPortfolioActionHold {
			t.Errorf("%s: action=%s, want HOLD", code, tr.Action)
		}
		if tr.OldWeight != 0.25 || tr.OldShares != 25000 {
			t.Errorf("%s: oldWeight=%v oldShares=%v, want 0.25 / 25000", code, tr.OldWeight, tr.OldShares)
		}
		if tr.NewWeight != 0.25 || tr.NewShares == 0 {
			t.Errorf("%s: newWeight=%v newShares=%v, want 0.25 / >0", code, tr.NewWeight, tr.NewShares)
		}
		if tr.Reason != simPortfolioReasonStayTop4 {
			t.Errorf("%s: reason=%s, want %s", code, tr.Reason, simPortfolioReasonStayTop4)
		}
	}

	// BUY: 600005, 600006 — newly entered.
	for _, code := range []string{"600005", "600006"} {
		tr := byCode[code]
		if tr.Action != simPortfolioActionBuy {
			t.Errorf("%s: action=%s, want BUY", code, tr.Action)
		}
		if tr.OldWeight != 0 || tr.OldShares != 0 {
			t.Errorf("%s: oldWeight=%v oldShares=%v, want 0 / 0", code, tr.OldWeight, tr.OldShares)
		}
		if tr.NewWeight != 0.25 || tr.NewShares == 0 {
			t.Errorf("%s: newWeight=%v newShares=%v, want 0.25 / >0", code, tr.NewWeight, tr.NewShares)
		}
		if tr.Reason != simPortfolioReasonEnterTop4 {
			t.Errorf("%s: reason=%s, want %s", code, tr.Reason, simPortfolioReasonEnterTop4)
		}
	}
}

// TestSimPortfolioV2GenerateFactsFirstDayAllBuy verifies that on the first
// trading day (no previous daily), all trades are BUY — the expected baseline.
func TestSimPortfolioV2GenerateFactsFirstDayAllBuy(t *testing.T) {
	repo, svc := setupSimPortfolioV2Test(t)
	ctx := context.Background()
	now := time.Now().UTC()

	def := SimPortfolioV2Definition{
		ID: "test-port-B", Code: "TPB", Name: "Test B", Market: SimPortfolioV2MarketAShare,
		MaxHoldings: 4, InitialAssets: 1_000_000, IsActive: true, CreatedAt: now, UpdatedAt: now,
	}
	if err := repo.db.Create(&def).Error; err != nil {
		t.Fatalf("create definition: %v", err)
	}

	signalDate := "2026-07-06"
	tradeDate := "2026-07-07"
	selItems := []SimPortfolioV2SelectionItem{}
	for i := 0; i < 4; i++ {
		code := fmt.Sprintf("600%03d", i+1)
		selItems = append(selItems, SimPortfolioV2SelectionItem{
			SelectionBatchID: "sel-b", PortfolioID: def.ID, SignalDate: signalDate,
			Code: code, Exchange: "SSE", Name: code, Rank: i + 1, Weight: 0.25, CreatedAt: now,
		})
	}
	batch := SimPortfolioV2SelectionBatch{ID: "sel-b", PortfolioID: def.ID, Market: def.Market, SignalDate: signalDate, EntryTradeDate: tradeDate, Status: SimPortfolioV2StatusOK, SelectedCount: 4, CreatedAt: now, UpdatedAt: now}
	if err := repo.ReplaceSimPortfolioV2Selection(ctx, batch, selItems); err != nil {
		t.Fatalf("seed selection: %v", err)
	}

	priceReqs := []SimPortfolioV2PriceRequirement{}
	for _, item := range selItems {
		for _, pt := range []string{SimPortfolioV2PriceTypeEntryOpen, SimPortfolioV2PriceTypeValuationClose} {
			priceReqs = append(priceReqs, SimPortfolioV2PriceRequirement{
				PortfolioID: def.ID, Market: def.Market, SignalDate: signalDate,
				TradeDate: tradeDate, Code: item.Code, Exchange: item.Exchange,
				PriceType: pt, Required: true, Status: SimPortfolioV2PriceStatusSatisfied,
				Price: 10.0, PriceTradeDate: tradeDate, Source: "test",
				ResolvedAt: now, CreatedAt: now, UpdatedAt: now,
			})
		}
	}
	if err := repo.ReplaceSimPortfolioV2PriceRequirements(ctx, def.ID, signalDate, priceReqs); err != nil {
		t.Fatalf("seed price reqs: %v", err)
	}

	// No previous daily exists → first day.
	if err := svc.generateFacts(ctx, def, signalDate, tradeDate); err != nil {
		t.Fatalf("generateFacts: %v", err)
	}

	tradeRows, err := repo.ListSimPortfolioV2Trades(ctx, def.ID, "", "", "")
	if err != nil {
		t.Fatalf("list trades: %v", err)
	}
	if len(tradeRows) != 4 {
		t.Fatalf("expected 4 BUY trades on first day, got %d", len(tradeRows))
	}
	for _, tr := range tradeRows {
		if tr.Action != simPortfolioActionBuy {
			t.Errorf("first-day trade %s: action=%s, want BUY", tr.Code, tr.Action)
		}
		if tr.OldWeight != 0 || tr.OldShares != 0 {
			t.Errorf("first-day trade %s: oldWeight=%v oldShares=%v, want 0/0", tr.Code, tr.OldWeight, tr.OldShares)
		}
	}
}
