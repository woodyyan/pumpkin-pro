package factorlab

import (
	"context"
	"testing"
	"time"

	"github.com/woodyyan/pumpkin-pro/backend/tests/testutil"
)

func ptrFloat(v float64) *float64 { return &v }
func ptrInt(v int) *int           { return &v }

func setupFactorLabQueryService(t *testing.T) (*Service, *Repository) {
	t.Helper()
	db := testutil.InMemoryDB(t)
	if err := NewMigrator().AutoMigrate(db); err != nil {
		t.Fatalf("migrate factorlab: %v", err)
	}
	repo := NewRepository(db)
	return NewService(repo), repo
}

func seedFactorSnapshots(t *testing.T, repo *Repository) {
	t.Helper()
	now := time.Now().UTC()
	records := []FactorSnapshot{
		{SnapshotDate: "2026-05-08", Code: "000001", Symbol: "000001.SZ", Name: "平安银行", Board: BoardMain, ClosePrice: 10, MarketCap: ptrFloat(100e8), PE: ptrFloat(8), PB: ptrFloat(0.8), PS: ptrFloat(1.2), DividendYield: ptrFloat(0.035), EarningGrowth: ptrFloat(20), RevenueGrowth: ptrFloat(10), ROE: ptrFloat(12), OperatingCFMargin: ptrFloat(18), AssetToEquity: ptrFloat(8), Momentum1M: ptrFloat(5), Volatility1M: ptrFloat(15), Beta1Y: ptrFloat(0.9), ListingAgeDays: ptrInt(1000), AvailableTradingDays: 260, DataQualityFlags: `[]`, CreatedAt: now},
		{SnapshotDate: "2026-05-08", Code: "000002", Symbol: "000002.SZ", Name: "万科A", Board: BoardMain, ClosePrice: 8, MarketCap: ptrFloat(80e8), PE: ptrFloat(20), PB: ptrFloat(1.5), PS: ptrFloat(2.5), DividendYield: ptrFloat(0.01), EarningGrowth: ptrFloat(-5), RevenueGrowth: ptrFloat(2), ROE: ptrFloat(5), AssetToEquity: ptrFloat(12), Momentum1M: ptrFloat(-3), Volatility1M: ptrFloat(25), Beta1Y: ptrFloat(1.2), ListingAgeDays: ptrInt(2000), AvailableTradingDays: 260, DataQualityFlags: `["no_operating_cf_margin"]`, CreatedAt: now},
		{SnapshotDate: "2026-05-08", Code: "300001", Symbol: "300001.SZ", Name: "特锐德", Board: BoardChiNext, ClosePrice: 20, MarketCap: ptrFloat(30e8), PE: nil, PB: ptrFloat(3), IsNewStock: true, ListingAgeDays: ptrInt(100), AvailableTradingDays: 80, DataQualityFlags: `["no_pe"]`, CreatedAt: now},
	}
	if err := repo.BulkUpsertSnapshots(context.Background(), records); err != nil {
		t.Fatalf("seed snapshots: %v", err)
	}
	if err := repo.CreateTaskRun(context.Background(), FactorTaskRun{ID: "run-1", TaskType: TaskTypeDailyCompute, SnapshotDate: "2026-05-08", Status: TaskStatusSuccess, StartedAt: now}); err != nil {
		t.Fatalf("seed task run: %v", err)
	}
}

func TestFactorLabMetaNoSnapshot(t *testing.T) {
	svc, _ := setupFactorLabQueryService(t)
	meta, err := svc.Meta(context.Background())
	if err != nil {
		t.Fatalf("meta: %v", err)
	}
	if meta.HasSnapshot {
		t.Fatal("expected no snapshot")
	}
	if len(meta.Metrics) == 0 {
		t.Fatal("expected metric definitions")
	}
}

func TestFactorLabMetaWithCoverage(t *testing.T) {
	svc, repo := setupFactorLabQueryService(t)
	seedFactorSnapshots(t, repo)
	meta, err := svc.Meta(context.Background())
	if err != nil {
		t.Fatalf("meta: %v", err)
	}
	if !meta.HasSnapshot || meta.SnapshotDate != "2026-05-08" {
		t.Fatalf("unexpected meta snapshot: %+v", meta)
	}
	if meta.Universe.Total != 3 || meta.Universe.NewStockCount != 1 {
		t.Fatalf("unexpected universe: %+v", meta.Universe)
	}
	if meta.Coverage["pe"] != 2 || meta.Coverage["operating_cf_margin"] != 1 {
		t.Fatalf("unexpected coverage: %+v", meta.Coverage)
	}
	if meta.LastRun.Status != TaskStatusSuccess {
		t.Fatalf("expected last run status, got %+v", meta.LastRun)
	}
}

func TestFactorLabScreenerFiltersAndDividendScale(t *testing.T) {
	svc, repo := setupFactorLabQueryService(t)
	seedFactorSnapshots(t, repo)
	minDividend := 3.0
	maxPE := 10.0
	resp, err := svc.Screen(context.Background(), FactorScreenerRequest{
		Filters: map[string]FactorFilterRange{
			"dividend_yield": {Min: &minDividend},
			"pe":             {Max: &maxPE},
		},
		SortBy:    "pe",
		SortOrder: "asc",
		Page:      1,
		PageSize:  10,
	})
	if err != nil {
		t.Fatalf("screen: %v", err)
	}
	if resp.Total != 1 || len(resp.Items) != 1 || resp.Items[0].Code != "000001" {
		t.Fatalf("unexpected response: %+v", resp)
	}
}

func TestFactorLabScreenerMissingValuesExcluded(t *testing.T) {
	svc, repo := setupFactorLabQueryService(t)
	seedFactorSnapshots(t, repo)
	minPE := 0.0
	resp, err := svc.Screen(context.Background(), FactorScreenerRequest{Filters: map[string]FactorFilterRange{"pe": {Min: &minPE}}, PageSize: 10})
	if err != nil {
		t.Fatalf("screen: %v", err)
	}
	if resp.Total != 2 {
		t.Fatalf("expected records with non-null pe only, got %d", resp.Total)
	}
}

func TestFactorLabScreenerRejectsInvalidFilterAndRange(t *testing.T) {
	svc, repo := setupFactorLabQueryService(t)
	seedFactorSnapshots(t, repo)
	min := 10.0
	max := 1.0
	if _, err := svc.Screen(context.Background(), FactorScreenerRequest{Filters: map[string]FactorFilterRange{"unknown": {Min: &min}}}); err == nil {
		t.Fatal("expected invalid filter error")
	}
	if _, err := svc.Screen(context.Background(), FactorScreenerRequest{Filters: map[string]FactorFilterRange{"pe": {Min: &min, Max: &max}}}); err == nil {
		t.Fatal("expected invalid range error")
	}
}

func TestFactorLabScreenerPaginationAndSort(t *testing.T) {
	svc, repo := setupFactorLabQueryService(t)
	seedFactorSnapshots(t, repo)
	resp, err := svc.Screen(context.Background(), FactorScreenerRequest{SortBy: "market_cap", SortOrder: "desc", Page: 1, PageSize: 1})
	if err != nil {
		t.Fatalf("screen: %v", err)
	}
	if resp.Total != 3 || len(resp.Items) != 1 || resp.Items[0].Code != "000001" {
		t.Fatalf("unexpected pagination response: %+v", resp)
	}
}
