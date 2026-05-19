package main

import (
	"context"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/woodyyan/pumpkin-pro/backend/store/quadrant"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func setupRebuildTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatalf("open sqlite failed: %v", err)
	}
	if err := quadrant.NewMigrator().AutoMigrate(db); err != nil {
		t.Fatalf("migrate sqlite failed: %v", err)
	}
	return db
}

func seedBenchmarkPrice(t *testing.T, db *gorm.DB, definition quadrant.RankingPortfolioDefinition, snapshotDate string, closePrice float64) {
	t.Helper()
	now := time.Now().UTC()
	row := quadrant.RankingPortfolioBenchmarkPrice{
		DefinitionID:    definition.ID,
		SnapshotVersion: snapshotDate,
		SnapshotDate:    snapshotDate,
		BenchmarkCode:   definition.BenchmarkCode,
		BenchmarkName:   definition.BenchmarkName,
		ClosePrice:      closePrice,
		PriceTradeDate:  snapshotDate,
		CreatedAt:       now,
		UpdatedAt:       now,
	}
	if err := db.Create(&row).Error; err != nil {
		t.Fatalf("seed benchmark price failed: %v", err)
	}
}

func TestBuildPlanForDate_HKEXUsesHongKongSnapshots(t *testing.T) {
	db := setupRebuildTestDB(t)
	ctx := context.Background()
	now := time.Now().UTC()
	snapshotDate := "2026-05-17"
	definition := quadrant.RankingPortfolioDefinition{
		ID:               definitionIDHKA,
		Code:             definitionCodeHKA,
		Name:             "模拟组合A",
		Exchange:         "HKEX",
		PortfolioVariant: "A",
		BenchmarkCode:    "HSI",
		BenchmarkName:    "恒生指数",
		MaxHoldings:      4,
		SelectionRule:    selectionRuleTop4,
		ExcludedBoards:   "[]",
	}

	snapshots := []quadrant.RankingSnapshot{
		{Code: "600001", Name: "A股样本1", Exchange: "SSE", Rank: 1, Opportunity: 99, Risk: 10, ClosePrice: 10, PriceTradeDate: snapshotDate, SnapshotDate: snapshotDate, CreatedAt: now},
		{Code: "000001", Name: "A股样本2", Exchange: "SZSE", Rank: 2, Opportunity: 98, Risk: 11, ClosePrice: 11, PriceTradeDate: snapshotDate, SnapshotDate: snapshotDate, CreatedAt: now},
		{Code: "00175", Name: "吉利汽车", Exchange: "HKEX", Rank: 1, Opportunity: 97, Risk: 12, ClosePrice: 12, PriceTradeDate: snapshotDate, SnapshotDate: snapshotDate, CreatedAt: now},
		{Code: "01109", Name: "华润置地", Exchange: "HKEX", Rank: 2, Opportunity: 96, Risk: 13, ClosePrice: 13, PriceTradeDate: snapshotDate, SnapshotDate: snapshotDate, CreatedAt: now},
		{Code: "09618", Name: "京东集团-SW", Exchange: "HKEX", Rank: 3, Opportunity: 95, Risk: 14, ClosePrice: 14, PriceTradeDate: snapshotDate, SnapshotDate: snapshotDate, CreatedAt: now},
		{Code: "02618", Name: "京东物流", Exchange: "HKEX", Rank: 4, Opportunity: 94, Risk: 15, ClosePrice: 15, PriceTradeDate: snapshotDate, SnapshotDate: snapshotDate, CreatedAt: now},
		{Code: "01070", Name: "TCL电子", Exchange: "HKEX", Rank: 5, Opportunity: 93, Risk: 16, ClosePrice: 16, PriceTradeDate: snapshotDate, SnapshotDate: snapshotDate, CreatedAt: now},
	}
	if err := db.WithContext(ctx).Create(&snapshots).Error; err != nil {
		t.Fatalf("seed snapshots failed: %v", err)
	}
	seedBenchmarkPrice(t, db, definition, snapshotDate, 20000)

	plan, constituents, err := buildPlanForDate(ctx, db, definition, snapshotDate, nil, nil, nil)
	if err != nil {
		t.Fatalf("buildPlanForDate failed: %v", err)
	}
	if plan.Progress.SourceCount != 5 {
		t.Fatalf("expected 5 HK source rows, got %d", plan.Progress.SourceCount)
	}
	if len(constituents) != 4 {
		t.Fatalf("expected 4 HK constituents, got %d", len(constituents))
	}
	for _, item := range constituents {
		if item.Exchange != "HKEX" {
			t.Fatalf("expected HKEX constituent, got %+v", item)
		}
	}
	gotCodes := []string{constituents[0].Code, constituents[1].Code, constituents[2].Code, constituents[3].Code}
	wantCodes := []string{"00175", "01109", "09618", "02618"}
	for i := range wantCodes {
		if gotCodes[i] != wantCodes[i] {
			t.Fatalf("unexpected HK constituent order: got %v want %v", gotCodes, wantCodes)
		}
	}
}

func TestLoadDefinition_FallsBackForAllPortfolioVariants(t *testing.T) {
	db := setupRebuildTestDB(t)
	ctx := context.Background()

	cases := []struct {
		id              string
		exchange        string
		variant         string
		benchmarkCode   string
		selectionRule   string
		selectionWindow int
	}{
		{defaultDefinitionID, "ASHARE", "A", "SHCI", selectionRuleTop4, 0},
		{definitionIDAShareB, "ASHARE", "B", "SHCI", selectionRuleTop10ByStreak, 10},
		{definitionIDHKA, "HKEX", "A", "HSI", selectionRuleTop4, 0},
		{definitionIDHKB, "HKEX", "B", "HSI", selectionRuleTop10ByStreak, 10},
	}

	for _, tc := range cases {
		definition, err := loadDefinition(ctx, db, tc.id)
		if err != nil {
			t.Fatalf("loadDefinition(%s) failed: %v", tc.id, err)
		}
		if definition.Exchange != tc.exchange || definition.PortfolioVariant != tc.variant {
			t.Fatalf("unexpected fallback market identity for %s: %+v", tc.id, definition)
		}
		if definition.BenchmarkCode != tc.benchmarkCode || definition.SelectionRule != tc.selectionRule || definition.SelectionWindow != tc.selectionWindow {
			t.Fatalf("unexpected fallback rules for %s: %+v", tc.id, definition)
		}
	}
}
