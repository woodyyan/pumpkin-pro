package factorlab

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/woodyyan/pumpkin-pro/backend/store/companyprofile"
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
	testutil.AutoMigrateModels(t, db, &companyprofile.CompanyProfileRecord{}, &companyprofile.IndustryMappingRecord{})
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
	snapshots := []FactorSnapshot{
		{SnapshotDate: "2026-05-08", Code: "000001", Symbol: "000001.SZ", Name: "平安银行", Board: BoardMain, ClosePrice: 10, PE: ptrFloat(8), DividendYield: ptrFloat(0.03), Performance1Y: ptrFloat(12), FCFMargin: ptrFloat(18), ListingAgeDays: ptrInt(1000), AvailableTradingDays: 260, DataQualityFlags: `[]`, CreatedAt: now},
		{SnapshotDate: "2026-05-08", Code: "000002", Symbol: "000002.SZ", Name: "万科A", Board: BoardMain, ClosePrice: 8, PE: ptrFloat(20), Performance1Y: ptrFloat(2), ListingAgeDays: ptrInt(2000), AvailableTradingDays: 260, DataQualityFlags: `[]`, CreatedAt: now},
		{SnapshotDate: "2026-05-08", Code: "300001", Symbol: "300001.SZ", Name: "特锐德", Board: BoardChiNext, ClosePrice: 20, ListingAgeDays: ptrInt(100), AvailableTradingDays: 80, DataQualityFlags: `[]`, CreatedAt: now},
	}
	if err := repo.BulkUpsertSnapshots(context.Background(), snapshots); err != nil {
		t.Fatalf("seed snapshots: %v", err)
	}
	if err := repo.CreateTaskRun(context.Background(), FactorTaskRun{ID: "run-1", TaskType: "factor_score_compute", SnapshotDate: "2026-05-08", Status: TaskStatusSuccess, StartedAt: now}); err != nil {
		t.Fatalf("seed task run: %v", err)
	}
}

func seedFactorLabAdminMetadata(t *testing.T, repo *Repository) {
	t.Helper()
	now := time.Now().UTC()
	securities := []FactorSecurity{
		{Code: "000001", Symbol: "000001.SZ", Name: "平安银行", Exchange: "SZSE", Board: BoardMain, IsActive: true, UpdatedAt: now},
		{Code: "000002", Symbol: "000002.SZ", Name: "万科A", Exchange: "SZSE", Board: BoardMain, IsActive: true, UpdatedAt: now},
	}
	if err := repo.db.WithContext(context.Background()).Create(&securities).Error; err != nil {
		t.Fatalf("seed factor securities: %v", err)
	}
	profiles := []companyprofile.CompanyProfileRecord{
		{Symbol: "000001.SZ", Exchange: "SZSE", Code: "000001", Name: "平安银行", IndustryName: "银行", ListingStatus: companyprofile.ListingStatusListed, ProfileStatus: companyprofile.ProfileStatusComplete, QualityFlags: `[]`, CreatedAt: now, UpdatedAt: now},
		{Symbol: "000002.SZ", Exchange: "SZSE", Code: "000002", Name: "万科A", IndustryName: "房地产", ListingStatus: companyprofile.ListingStatusListed, ProfileStatus: companyprofile.ProfileStatusComplete, QualityFlags: `[]`, CreatedAt: now, UpdatedAt: now},
	}
	if err := repo.db.WithContext(context.Background()).Create(&profiles).Error; err != nil {
		t.Fatalf("seed company profiles: %v", err)
	}
	industryRows := []FactorSecurityIndustry{
		{Code: "000001", RawIndustryName: "银行", IndustryName: "银行", IndustrySource: "eastmoney:qt_clist_get", UpdatedAt: now},
		{Code: "000002", RawIndustryName: "房地产", IndustryName: "房地产", IndustrySource: "eastmoney:qt_clist_get", UpdatedAt: now},
	}
	if err := repo.db.WithContext(context.Background()).Create(&industryRows).Error; err != nil {
		t.Fatalf("seed security industries: %v", err)
	}
	industriesFinishedAt := now.Add(-110 * time.Minute)
	dividendsFinishedAt := now.Add(-23 * time.Hour)
	industriesSummary := `{"status":"partial","warnings":[{"mode":"industries","error":"akshare down","coverage_ratio":1.0}]}`
	if err := repo.CreateTaskRun(context.Background(), FactorTaskRun{
		ID:           "run-industries",
		TaskType:     TaskTypeBackfill,
		Status:       TaskStatusPartial,
		StartedAt:    now.Add(-2 * time.Hour),
		FinishedAt:   &industriesFinishedAt,
		ParamsJSON:   `{"mode":"industries","args":{"mode":"industries"}}`,
		SummaryJSON:  industriesSummary,
		ErrorMessage: "akshare down",
	}); err != nil {
		t.Fatalf("seed industries task run: %v", err)
	}
	if err := repo.CreateTaskRun(context.Background(), FactorTaskRun{
		ID:          "run-dividends",
		TaskType:    TaskTypeBackfill,
		Status:      TaskStatusSuccess,
		StartedAt:   now.Add(-24 * time.Hour),
		FinishedAt:  &dividendsFinishedAt,
		ParamsJSON:  `{"mode":"dividends","args":{"mode":"dividends"}}`,
		SummaryJSON: `{"status":"success","total":2,"success":2,"failed":0}`,
	}); err != nil {
		t.Fatalf("seed dividends task run: %v", err)
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

func TestFactorLabAdminStatusIncludesCoverageAndRecentRuns(t *testing.T) {
	svc, repo := setupFactorLabQueryService(t)
	seedFactorScores(t, repo)
	seedFactorLabAdminMetadata(t, repo)
	status, err := svc.AdminStatus(context.Background(), WorkerStatus{Enabled: true, Schedule: "21:00"})
	if err != nil {
		t.Fatalf("admin status: %v", err)
	}
	if status.DBHealth != "ok" {
		t.Fatalf("expected db health ok, got %q", status.DBHealth)
	}
	if status.Coverage.Universe != 3 || status.Coverage.RawMetrics["pe"] != 2 || status.Coverage.RawMetrics["fcf_margin"] != 1 || status.Coverage.Factors["value_score"] != 2 {
		t.Fatalf("unexpected coverage: %+v", status.Coverage)
	}
	if len(status.Coverage.Warnings) == 0 {
		t.Fatal("expected low coverage warnings")
	}
	if !strings.Contains(strings.Join(status.Coverage.Warnings, " "), "fcf_margin 覆盖率低于 80%") {
		t.Fatalf("expected fcf_margin warning, got %+v", status.Coverage.Warnings)
	}
	metadata := status.Coverage.Metadata
	if metadata == nil {
		t.Fatal("expected admin metadata")
	}
	industriesHealth, _ := metadata["industries_health"].(map[string]any)
	if industriesHealth["coverage_ratio"].(float64) != 1 {
		t.Fatalf("expected industries coverage ratio 1, got %+v", industriesHealth)
	}
	industriesMeta, _ := metadata["industries"].(map[string]any)
	if industriesMeta["status"] != TaskStatusPartial {
		t.Fatalf("expected industries latest status partial, got %+v", industriesMeta)
	}
	dividendsMeta, _ := metadata["dividends"].(map[string]any)
	if dividendsMeta["status"] != TaskStatusSuccess {
		t.Fatalf("expected dividends latest status success, got %+v", dividendsMeta)
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
		FactorWeights: map[string]float64{"value": 50, "low_volatility": 50},
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
	if _, err := svc.Screen(context.Background(), FactorScreenerRequest{FactorWeights: map[string]float64{"unknown": 100}}); err == nil {
		t.Fatal("expected invalid factor error")
	}
	if _, err := svc.Screen(context.Background(), FactorScreenerRequest{FactorWeights: map[string]float64{"value": 50}}); err == nil {
		t.Fatal("expected invalid sum error")
	}
	if _, err := svc.Screen(context.Background(), FactorScreenerRequest{FactorWeights: map[string]float64{"value": -1, "growth": 101}}); err == nil {
		t.Fatal("expected negative weight error")
	}
	if _, err := svc.Screen(context.Background(), FactorScreenerRequest{FactorWeights: map[string]float64{"value": 12.345, "growth": 87.655}}); err == nil {
		t.Fatal("expected precision error")
	}
	if _, err := svc.Screen(context.Background(), FactorScreenerRequest{FactorWeights: map[string]float64{"value": 12.5, "growth": 87.5}}); err != nil {
		t.Fatalf("expected decimal percentage accepted, got %v", err)
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
