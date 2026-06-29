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
