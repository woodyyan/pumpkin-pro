package factorlab

import (
	"context"
	"testing"
	"time"

	"github.com/woodyyan/pumpkin-pro/backend/tests/testutil"
)

func ptrFloat(v float64) *float64 { return &v }

func setupFactorLabQueryService(t *testing.T) (*Service, *Repository) {
	t.Helper()
	db := testutil.InMemoryDB(t)
	if err := NewMigrator().AutoMigrate(db); err != nil {
		t.Fatalf("migrate factorlab: %v", err)
	}
	repo := NewRepository(db)
	return NewService(repo), repo
}

func seedFactorScores(t *testing.T, repo *Repository) {
	t.Helper()
	now := time.Now().UTC()
	records := []FactorScore{
		{SnapshotDate: "2026-05-08", Code: "000001", Symbol: "000001.SZ", Name: "平安银行", Industry: "银行", ClosePrice: 10, ValueScore: ptrFloat(90), DividendYieldScore: ptrFloat(80), GrowthScore: ptrFloat(40), QualityScore: ptrFloat(70), MomentumScore: ptrFloat(30), SizeScore: ptrFloat(20), LowVolatilityScore: ptrFloat(85), CreatedAt: now},
		{SnapshotDate: "2026-05-08", Code: "000002", Symbol: "000002.SZ", Name: "万科A", Industry: "房地产", ClosePrice: 8, ValueScore: ptrFloat(70), DividendYieldScore: ptrFloat(60), GrowthScore: ptrFloat(20), QualityScore: ptrFloat(40), MomentumScore: ptrFloat(50), SizeScore: ptrFloat(30), LowVolatilityScore: ptrFloat(65), CreatedAt: now},
		{SnapshotDate: "2026-05-08", Code: "300001", Symbol: "300001.SZ", Name: "特锐德", Industry: "电力设备", IsNewStock: true, ClosePrice: 20, ValueScore: nil, DividendYieldScore: ptrFloat(10), GrowthScore: ptrFloat(95), QualityScore: ptrFloat(60), MomentumScore: ptrFloat(90), SizeScore: ptrFloat(100), LowVolatilityScore: nil, CreatedAt: now},
	}
	if err := repo.db.WithContext(context.Background()).Create(&records).Error; err != nil {
		t.Fatalf("seed factor scores: %v", err)
	}
	if err := repo.CreateTaskRun(context.Background(), FactorTaskRun{ID: "run-1", TaskType: "factor_score_compute", SnapshotDate: "2026-05-08", Status: TaskStatusSuccess, StartedAt: now}); err != nil {
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
	if len(meta.Factors) != 7 {
		t.Fatalf("expected factor definitions, got %d", len(meta.Factors))
	}
}

func TestFactorLabMetaWithCoverage(t *testing.T) {
	svc, repo := setupFactorLabQueryService(t)
	seedFactorScores(t, repo)
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
	if meta.Coverage["value_score"] != 2 || meta.Coverage["low_volatility_score"] != 2 {
		t.Fatalf("unexpected coverage: %+v", meta.Coverage)
	}
	if meta.LastRun.Status != TaskStatusSuccess {
		t.Fatalf("expected last run status, got %+v", meta.LastRun)
	}
}

func TestFactorLabScreenerDefaultEqualWeightCompositeSort(t *testing.T) {
	svc, repo := setupFactorLabQueryService(t)
	seedFactorScores(t, repo)
	resp, err := svc.Screen(context.Background(), FactorScreenerRequest{Page: 1, PageSize: 10})
	if err != nil {
		t.Fatalf("screen: %v", err)
	}
	if resp.Total != 3 || len(resp.Items) != 3 {
		t.Fatalf("unexpected response size: %+v", resp)
	}
	if resp.Items[0].Code != "300001" {
		t.Fatalf("expected default equal-weight composite sort, got first=%s", resp.Items[0].Code)
	}
	if resp.Items[0].CompositeScore == nil {
		t.Fatal("expected composite score")
	}
}

func TestFactorLabScreenerCustomWeightsNormalizeMissingScores(t *testing.T) {
	svc, repo := setupFactorLabQueryService(t)
	seedFactorScores(t, repo)
	resp, err := svc.Screen(context.Background(), FactorScreenerRequest{
		FactorWeights: map[string]float64{"value": 0.5, "low_volatility": 0.5},
		SortBy:        "composite_score",
		SortOrder:     "desc",
		Page:          1,
		PageSize:      10,
	})
	if err != nil {
		t.Fatalf("screen: %v", err)
	}
	if resp.Items[0].Code != "000001" {
		t.Fatalf("expected 000001 first, got %+v", resp.Items)
	}
	var item300001 FactorScreenerItem
	for _, item := range resp.Items {
		if item.Code == "300001" {
			item300001 = item
		}
	}
	if item300001.CompositeScore != nil {
		t.Fatalf("expected nil composite when selected factors are missing, got %v", *item300001.CompositeScore)
	}
}

func TestFactorLabScreenerRejectsInvalidWeights(t *testing.T) {
	svc, repo := setupFactorLabQueryService(t)
	seedFactorScores(t, repo)
	if _, err := svc.Screen(context.Background(), FactorScreenerRequest{FactorWeights: map[string]float64{"unknown": 1}}); err == nil {
		t.Fatal("expected invalid factor error")
	}
	if _, err := svc.Screen(context.Background(), FactorScreenerRequest{FactorWeights: map[string]float64{"value": 0.5}}); err == nil {
		t.Fatal("expected invalid sum error")
	}
	if _, err := svc.Screen(context.Background(), FactorScreenerRequest{FactorWeights: map[string]float64{"value": -1, "growth": 2}}); err == nil {
		t.Fatal("expected negative weight error")
	}
}

func TestFactorLabScreenerPaginationAndFactorSort(t *testing.T) {
	svc, repo := setupFactorLabQueryService(t)
	seedFactorScores(t, repo)
	resp, err := svc.Screen(context.Background(), FactorScreenerRequest{SortBy: "value_score", SortOrder: "desc", Page: 1, PageSize: 1})
	if err != nil {
		t.Fatalf("screen: %v", err)
	}
	if resp.Total != 3 || len(resp.Items) != 1 || resp.Items[0].Code != "000001" {
		t.Fatalf("unexpected pagination response: %+v", resp)
	}
}
