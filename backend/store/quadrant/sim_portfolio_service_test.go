package quadrant

import (
	"context"
	"math"
	"testing"
	"time"
)

func seedSimPortfolioDefinition(t *testing.T, repo *Repository) RankingPortfolioDefinition {
	t.Helper()
	ctx := context.Background()
	def := buildRankingPortfolioDefinitionRecord(defaultRankingPortfolioDefinitionSpecs()[0], time.Now().UTC())
	if err := repo.db.WithContext(ctx).Create(&def).Error; err != nil {
		t.Fatalf("seed definition: %v", err)
	}
	return def
}

func seedSimPortfolioSnapshots(t *testing.T, repo *Repository) {
	t.Helper()
	ctx := context.Background()
	now := time.Now().UTC()
	snapshots := []RankingSnapshot{
		{Code: "600001", Name: "A", Exchange: "SSE", Rank: 1, ClosePrice: 10.0, PriceTradeDate: "2026-06-01", SnapshotDate: "2026-06-01", CreatedAt: now},
		{Code: "600002", Name: "B", Exchange: "SSE", Rank: 2, ClosePrice: 10.0, PriceTradeDate: "2026-06-01", SnapshotDate: "2026-06-01", CreatedAt: now},
		{Code: "600003", Name: "C", Exchange: "SSE", Rank: 3, ClosePrice: 10.0, PriceTradeDate: "2026-06-01", SnapshotDate: "2026-06-01", CreatedAt: now},
		{Code: "600004", Name: "D", Exchange: "SSE", Rank: 4, ClosePrice: 10.0, PriceTradeDate: "2026-06-01", SnapshotDate: "2026-06-01", CreatedAt: now},
		{Code: "600001", Name: "A", Exchange: "SSE", Rank: 1, ClosePrice: 10.2, PriceTradeDate: "2026-06-02", SnapshotDate: "2026-06-02", CreatedAt: now},
		{Code: "600002", Name: "B", Exchange: "SSE", Rank: 2, ClosePrice: 9.8, PriceTradeDate: "2026-06-02", SnapshotDate: "2026-06-02", CreatedAt: now},
		{Code: "600005", Name: "E", Exchange: "SSE", Rank: 3, ClosePrice: 18.2, PriceTradeDate: "2026-06-02", SnapshotDate: "2026-06-02", CreatedAt: now},
		{Code: "600006", Name: "F", Exchange: "SSE", Rank: 4, ClosePrice: 12.4, PriceTradeDate: "2026-06-02", SnapshotDate: "2026-06-02", CreatedAt: now},
		{Code: "600003", Name: "C", Exchange: "SSE", Rank: 5, ClosePrice: 10.4, PriceTradeDate: "2026-06-02", SnapshotDate: "2026-06-02", CreatedAt: now},
		{Code: "600004", Name: "D", Exchange: "SSE", Rank: 6, ClosePrice: 9.9, PriceTradeDate: "2026-06-02", SnapshotDate: "2026-06-02", CreatedAt: now},
	}
	for _, snapshot := range snapshots {
		if err := repo.UpsertSnapshot(ctx, snapshot); err != nil {
			t.Fatalf("seed snapshot %s %s: %v", snapshot.SnapshotDate, snapshot.Code, err)
		}
	}
}

func seedSimPortfolioMarketPrices(t *testing.T, repo *Repository, definitionID string) {
	t.Helper()
	ctx := context.Background()
	now := time.Now().UTC()
	rows := []RankingPortfolioMarketPrice{
		{DefinitionID: definitionID, SnapshotVersion: "2026-06-01", SnapshotDate: "2026-06-01", Code: "600001", Exchange: "SSE", OpenPrice: 10, EntryTradeDate: "2026-06-02", CreatedAt: now, UpdatedAt: now},
		{DefinitionID: definitionID, SnapshotVersion: "2026-06-01", SnapshotDate: "2026-06-01", Code: "600002", Exchange: "SSE", OpenPrice: 10, EntryTradeDate: "2026-06-02", CreatedAt: now, UpdatedAt: now},
		{DefinitionID: definitionID, SnapshotVersion: "2026-06-01", SnapshotDate: "2026-06-01", Code: "600003", Exchange: "SSE", OpenPrice: 10, EntryTradeDate: "2026-06-02", CreatedAt: now, UpdatedAt: now},
		{DefinitionID: definitionID, SnapshotVersion: "2026-06-01", SnapshotDate: "2026-06-01", Code: "600004", Exchange: "SSE", OpenPrice: 10, EntryTradeDate: "2026-06-02", CreatedAt: now, UpdatedAt: now},
	}
	for _, row := range rows {
		if err := repo.db.WithContext(ctx).Create(&row).Error; err != nil {
			t.Fatalf("seed market price %s: %v", row.Code, err)
		}
	}
}

func TestRecomputeSimPortfoliosBuildsFactTables(t *testing.T) {
	repo, cleanup := setupQuadrantTest(t)
	defer cleanup()
	ctx := context.Background()
	def := seedSimPortfolioDefinition(t, repo)
	seedSimPortfolioSnapshots(t, repo)
	seedSimPortfolioMarketPrices(t, repo, def.ID)
	svc := NewService(repo)

	if err := svc.RecomputeSimPortfolios(ctx, def.ID, "2026-06-01", "2026-06-02", true); err != nil {
		t.Fatalf("RecomputeSimPortfolios: %v", err)
	}

	dailyRows, err := repo.ListAllSimPortfolioDaily(ctx, def.ID)
	if err != nil {
		t.Fatalf("ListAllSimPortfolioDaily: %v", err)
	}
	if len(dailyRows) != 2 {
		t.Fatalf("daily rows = %d, want 2", len(dailyRows))
	}
	if dailyRows[0].TradeDate != "2026-06-01" || dailyRows[0].Status != simPortfolioStatusSeeded {
		t.Fatalf("baseline row = %+v", dailyRows[0])
	}
	if dailyRows[1].TradeDate != "2026-06-02" || dailyRows[1].SignalDate != "2026-06-01" {
		t.Fatalf("daily row = %+v", dailyRows[1])
	}
	if math.Abs(dailyRows[1].TotalAssets-1007500) > 0.01 {
		t.Fatalf("total_assets = %v, want 1007500", dailyRows[1].TotalAssets)
	}
	positions, err := repo.ListSimPortfolioPositionsByTradeDate(ctx, def.ID, "2026-06-02")
	if err != nil {
		t.Fatalf("ListSimPortfolioPositionsByTradeDate: %v", err)
	}
	if len(positions) != 4 {
		t.Fatalf("positions = %d, want 4", len(positions))
	}
	trades, err := repo.ListSimPortfolioTradesRange(ctx, def.ID, "2026-06-02", "2026-06-02", "")
	if err != nil {
		t.Fatalf("ListSimPortfolioTradesRange: %v", err)
	}
	if len(trades) != 4 {
		t.Fatalf("trades = %d, want 4", len(trades))
	}
	for _, trade := range trades {
		if trade.Action != simPortfolioActionBuy {
			t.Fatalf("trade action = %s, want BUY", trade.Action)
		}
	}
	metrics, err := repo.GetLatestSimPortfolioMetrics(ctx, def.ID)
	if err != nil {
		t.Fatalf("GetLatestSimPortfolioMetrics: %v", err)
	}
	if metrics == nil || metrics.TradeDate != "2026-06-02" {
		t.Fatalf("metrics = %+v", metrics)
	}
}

func TestGetSimPortfolioOverviewShowsPendingOpenPrice(t *testing.T) {
	repo, cleanup := setupQuadrantTest(t)
	defer cleanup()
	ctx := context.Background()
	def := seedSimPortfolioDefinition(t, repo)
	seedSimPortfolioSnapshots(t, repo)
	seedSimPortfolioMarketPrices(t, repo, def.ID)
	svc := NewService(repo)

	if err := svc.RecomputeSimPortfolios(ctx, def.ID, "2026-06-01", "2026-06-02", true); err != nil {
		t.Fatalf("RecomputeSimPortfolios: %v", err)
	}
	resp, err := svc.GetSimPortfolioOverview(ctx)
	if err != nil {
		t.Fatalf("GetSimPortfolioOverview: %v", err)
	}
	if len(resp.Items) != 1 {
		t.Fatalf("overview items = %d, want 1", len(resp.Items))
	}
	if resp.Items[0].Status != "pending_open_price" {
		t.Fatalf("status = %s, want pending_open_price", resp.Items[0].Status)
	}
	if resp.Items[0].PendingSignalDate != "2026-06-02" {
		t.Fatalf("pending_signal_date = %s, want 2026-06-02", resp.Items[0].PendingSignalDate)
	}
}

func TestBackfillSimPortfolioOpenPrices_FillsMissingOpen(t *testing.T) {
	repo, cleanup := setupQuadrantTest(t)
	defer cleanup()
	ctx := context.Background()
	def := seedSimPortfolioDefinition(t, repo)
	seedSimPortfolioSnapshots(t, repo)
	now := time.Now().UTC()

	// Seed market price rows for 2026-06-02 with open_price=0 (pending).
	rows := []RankingPortfolioMarketPrice{
		{DefinitionID: def.ID, SnapshotVersion: "2026-06-02", SnapshotDate: "2026-06-02", Code: "600001", Exchange: "SSE", OpenPrice: 0, CreatedAt: now, UpdatedAt: now},
		{DefinitionID: def.ID, SnapshotVersion: "2026-06-02", SnapshotDate: "2026-06-02", Code: "600002", Exchange: "SSE", OpenPrice: 0, CreatedAt: now, UpdatedAt: now},
		{DefinitionID: def.ID, SnapshotVersion: "2026-06-02", SnapshotDate: "2026-06-02", Code: "600005", Exchange: "SSE", OpenPrice: 0, CreatedAt: now, UpdatedAt: now},
		{DefinitionID: def.ID, SnapshotVersion: "2026-06-02", SnapshotDate: "2026-06-02", Code: "600006", Exchange: "SSE", OpenPrice: 0, CreatedAt: now, UpdatedAt: now},
	}
	for _, row := range rows {
		if err := repo.db.WithContext(ctx).Create(&row).Error; err != nil {
			t.Fatalf("seed market price %s: %v", row.Code, err)
		}
	}

	svc := NewService(repo)
	svc.SetOpenPriceResolver(func(_ context.Context, code string, _ string, _ string) float64 {
		if code == "600005" {
			return 0 // simulate unavailable (suspended)
		}
		return 10.5
	})

	resp, err := svc.BackfillSimPortfolioOpenPrices(ctx, def.ID, "", true)
	if err != nil {
		t.Fatalf("BackfillSimPortfolioOpenPrices: %v", err)
	}
	if !resp.OK {
		t.Fatalf("backfill failed: %s", resp.Message)
	}
	if resp.Summary.FilledCount != 3 {
		t.Fatalf("filled = %d, want 3", resp.Summary.FilledCount)
	}
	if resp.Summary.StillPendingCount != 1 {
		t.Fatalf("still_pending = %d, want 1", resp.Summary.StillPendingCount)
	}

	// Verify the open prices were actually written.
	for _, code := range []string{"600001", "600002", "600006"} {
		openPrice, _, err := repo.GetRankingPortfolioSelectionOpenPrice(ctx, def.ID, "2026-06-02", code, "SSE")
		if err != nil {
			t.Fatalf("GetRankingPortfolioSelectionOpenPrice %s: %v", code, err)
		}
		if openPrice != 10.5 {
			t.Fatalf("open_price for %s = %v, want 10.5", code, openPrice)
		}
	}
}

func TestBackfillSimPortfolioOpenPrices_NoResolver(t *testing.T) {
	repo, cleanup := setupQuadrantTest(t)
	defer cleanup()
	ctx := context.Background()
	def := seedSimPortfolioDefinition(t, repo)
	seedSimPortfolioSnapshots(t, repo)

	svc := NewService(repo) // no openPriceResolver set
	resp, err := svc.BackfillSimPortfolioOpenPrices(ctx, def.ID, "", true)
	if err != nil {
		t.Fatalf("BackfillSimPortfolioOpenPrices: %v", err)
	}
	if resp.OK {
		t.Fatalf("expected OK=false when resolver not configured")
	}
}

func TestGetSimPortfolioAdminStatus_ShowsMissingOpenPriceCount(t *testing.T) {
	repo, cleanup := setupQuadrantTest(t)
	defer cleanup()
	ctx := context.Background()
	def := seedSimPortfolioDefinition(t, repo)
	seedSimPortfolioSnapshots(t, repo)
	seedSimPortfolioMarketPrices(t, repo, def.ID)
	svc := NewService(repo)

	// Recompute to build fact tables for 2026-06-01 -> 2026-06-02.
	if err := svc.RecomputeSimPortfolios(ctx, def.ID, "2026-06-01", "2026-06-02", true); err != nil {
		t.Fatalf("RecomputeSimPortfolios: %v", err)
	}

	// Now the latest signal is 2026-06-02 but market prices for 2026-06-02
	// haven't been created yet, so the status should show pending_open_price.
	resp, err := svc.GetSimPortfolioAdminStatus(ctx)
	if err != nil {
		t.Fatalf("GetSimPortfolioAdminStatus: %v", err)
	}
	if len(resp.Items) != 1 {
		t.Fatalf("items = %d, want 1", len(resp.Items))
	}
	item := resp.Items[0]
	if item.Status != "pending_open_price" {
		t.Fatalf("status = %s, want pending_open_price", item.Status)
	}
	if item.MissingOpenPriceCount <= 0 {
		t.Fatalf("missing_open_price_count = %d, expected > 0", item.MissingOpenPriceCount)
	}
}

func TestVerifySimPortfolios_DetectsMissingOpenPrice(t *testing.T) {
	repo, cleanup := setupQuadrantTest(t)
	defer cleanup()
	ctx := context.Background()
	def := seedSimPortfolioDefinition(t, repo)
	seedSimPortfolioSnapshots(t, repo)
	seedSimPortfolioMarketPrices(t, repo, def.ID)
	svc := NewService(repo)

	if err := svc.RecomputeSimPortfolios(ctx, def.ID, "2026-06-01", "2026-06-02", true); err != nil {
		t.Fatalf("RecomputeSimPortfolios: %v", err)
	}

	// Tamper: set one position's buy_price to 0 to simulate missing open price.
	if err := repo.db.WithContext(ctx).
		Model(&SimPortfolioPosition{}).
		Where("portfolio_id = ? AND trade_date = ? AND stock_code = ?", def.ID, "2026-06-02", "600001").
		Update("buy_price", 0).Error; err != nil {
		t.Fatalf("tamper buy_price: %v", err)
	}

	resp, err := svc.VerifySimPortfolios(ctx, def.ID)
	if err != nil {
		t.Fatalf("VerifySimPortfolios: %v", err)
	}
	foundMissingOpen := false
	for _, item := range resp.Items {
		if item.Status == "missing_open_price" {
			foundMissingOpen = true
			break
		}
	}
	if !foundMissingOpen {
		t.Fatalf("expected at least one missing_open_price status")
	}
}

func TestSyncSimPortfoliosReportsGeneratedRows(t *testing.T) {
	repo, cleanup := setupQuadrantTest(t)
	defer cleanup()
	ctx := context.Background()
	def := seedSimPortfolioDefinition(t, repo)
	seedSimPortfolioSnapshots(t, repo)
	seedSimPortfolioMarketPrices(t, repo, def.ID)
	svc := NewService(repo)

	// Simulate the D0/baseline-only state found in local admin: a seeded row
	// exists at the first signal date while a successor snapshot is already ready.
	if err := svc.ensureSimPortfolioBaseline(ctx, def, "2026-06-01"); err != nil {
		t.Fatalf("ensureSimPortfolioBaseline: %v", err)
	}

	// Sync sees successor snapshot 2026-06-02 and should generate one
	// completed valuation day instead of silently staying at baseline.
	resp, err := svc.SyncSimPortfolios(ctx)
	if err != nil {
		t.Fatalf("second SyncSimPortfolios: %v", err)
	}
	if !resp.OK {
		t.Fatalf("sync response not ok: %+v", resp)
	}
	if len(resp.Items) != 1 {
		t.Fatalf("items = %d, want 1", len(resp.Items))
	}
	item := resp.Items[0]
	if item.GeneratedDailyCount != 1 || item.LastGeneratedTradeDate != "2026-06-02" || item.Status != "synced" {
		t.Fatalf("sync item = %+v, want generated 2026-06-02", item)
	}
	positions, err := repo.ListSimPortfolioPositionsByTradeDate(ctx, def.ID, "2026-06-02")
	if err != nil {
		t.Fatalf("ListSimPortfolioPositionsByTradeDate: %v", err)
	}
	if len(positions) != 4 {
		t.Fatalf("positions = %d, want 4", len(positions))
	}
}

func TestGetSimPortfolioAdminStatus_ShowsBaselineOnlyAndActionHint(t *testing.T) {
	repo, cleanup := setupQuadrantTest(t)
	defer cleanup()
	ctx := context.Background()
	def := seedSimPortfolioDefinition(t, repo)
	seedSimPortfolioSnapshots(t, repo)
	seedSimPortfolioMarketPrices(t, repo, def.ID)
	svc := NewService(repo)

	if err := svc.ensureSimPortfolioBaseline(ctx, def, "2026-06-01"); err != nil {
		t.Fatalf("ensureSimPortfolioBaseline: %v", err)
	}

	resp, err := svc.GetSimPortfolioAdminStatus(ctx)
	if err != nil {
		t.Fatalf("GetSimPortfolioAdminStatus: %v", err)
	}
	if len(resp.Items) != 1 {
		t.Fatalf("items = %d, want 1", len(resp.Items))
	}
	item := resp.Items[0]
	if !item.BaselineOnly {
		t.Fatalf("baseline_only = false, item=%+v", item)
	}
	if item.Status != "baseline_only" {
		t.Fatalf("status = %s, want baseline_only", item.Status)
	}
	if !item.CanSync || item.NextSyncSignalDate != "2026-06-01" || item.NextSyncTradeDate != "2026-06-02" {
		t.Fatalf("sync hint mismatch: %+v", item)
	}
	if item.DailyRowCount != 1 || item.PositionRowCount != 0 || item.TradeRowCount != 0 || item.MetricsRowCount != 0 {
		t.Fatalf("fact counts mismatch: %+v", item)
	}
	if item.ActionHint == "" {
		t.Fatalf("expected action hint, got empty")
	}
}
