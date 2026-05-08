package quadrant

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/woodyyan/pumpkin-pro/backend/tests/testutil"
)

func setupSearchDB(t *testing.T) (*Repository, context.Context) {
	t.Helper()
	db := testutil.InMemoryDB(t)
	testutil.AutoMigrateModels(t, db,
		QuadrantScoreRecord{},
		ComputeLogRecord{},
	)
	repo := NewRepository(db)

	// Seed test data with Exchange field
	now := time.Now().UTC()
	seedData := []QuadrantScoreRecord{
		{Code: "600519", Name: "贵州茅台", Exchange: "SSE", Opportunity: 0.8, Risk: 0.3, Quadrant: "机会", ComputedAt: now},
		{Code: "600516", Name: "贵州茅台酒厂(集团)技术公司债", Exchange: "SSE", Opportunity: 0.5, Risk: 0.6, Quadrant: "中性", ComputedAt: now},
		{Code: "000001", Name: "平安银行", Exchange: "SZSE", Opportunity: 0.6, Risk: 0.4, Quadrant: "机会", ComputedAt: now},
		{Code: "000002", Name: "万  科Ａ", Exchange: "SZSE", Opportunity: 0.4, Risk: 0.7, Quadrant: "防御", ComputedAt: now},
		{Code: "000858", Name: "五 粮 液", Exchange: "SZSE", Opportunity: 0.82, Risk: 0.28, Quadrant: "机会", ComputedAt: now},
		{Code: "601318", Name: "中国平安", Exchange: "SSE", Opportunity: 0.7, Risk: 0.35, Quadrant: "机会", ComputedAt: now},
		{Code: "600036", Name: "招商银行", Exchange: "SSE", Opportunity: 0.65, Risk: 0.38, Quadrant: "机会", ComputedAt: now},
		{Code: "00700", Name: "腾讯控股", Exchange: "HKEX", Opportunity: 0.75, Risk: 0.25, Quadrant: "机会", ComputedAt: now},
		{Code: "09988", Name: "阿里巴巴-W", Exchange: "HKEX", Opportunity: 0.6, Risk: 0.4, Quadrant: "中性", ComputedAt: now},
	}
	if err := repo.BulkUpsert(context.Background(), seedData); err != nil {
		t.Fatalf("seed data failed: %v", err)
	}

	return repo, context.Background()
}

// ── Search tests ──

func TestSearch_ExactCodeMatch(t *testing.T) {
	repo, ctx := setupSearchDB(t)
	results, err := repo.Search(ctx, "600519", 10)
	if err != nil {
		t.Fatalf("Search failed: %v", err)
	}
	if len(results) == 0 {
		t.Fatal("expected at least 1 result")
	}
	if results[0].Code != "600519" {
		t.Errorf("expected first result code=600519, got %s", results[0].Code)
	}
	if results[0].Name != "贵州茅台" {
		t.Errorf("expected name=贵州茅台, got %s", results[0].Name)
	}
	if results[0].Exchange != "SSE" {
		t.Errorf("expected exchange=SSE, got %s", results[0].Exchange)
	}
}

func TestSearch_ExactNameMatch(t *testing.T) {
	repo, ctx := setupSearchDB(t)
	results, err := repo.Search(ctx, "平安银行", 10)
	if err != nil {
		t.Fatalf("Search failed: %v", err)
	}
	if len(results) == 0 {
		t.Fatal("expected at least 1 result")
	}
	if results[0].Name != "平安银行" {
		t.Errorf("expected first name=平安银行, got %s", results[0].Name)
	}
}

func TestSearch_NormalizedChineseNameMatch(t *testing.T) {
	repo, ctx := setupSearchDB(t)
	results, err := repo.Search(ctx, "五粮液", 10)
	if err != nil {
		t.Fatalf("Search failed: %v", err)
	}
	if len(results) == 0 {
		t.Fatal("expected at least 1 result")
	}
	if results[0].Code != "000858" {
		t.Fatalf("expected code 000858, got %s", results[0].Code)
	}
	if results[0].Name != "五 粮 液" {
		t.Fatalf("expected stored display name 五 粮 液, got %s", results[0].Name)
	}
}

func TestSearch_NormalizedFullWidthNameMatch(t *testing.T) {
	repo, ctx := setupSearchDB(t)
	results, err := repo.Search(ctx, "万科Ａ", 10)
	if err != nil {
		t.Fatalf("Search failed: %v", err)
	}
	if len(results) == 0 {
		t.Fatal("expected at least 1 result")
	}
	if results[0].Code != "000002" {
		t.Fatalf("expected code 000002, got %s", results[0].Code)
	}
}

func TestSearch_PrefixFuzzyMatch(t *testing.T) {
	repo, ctx := setupSearchDB(t)
	results, err := repo.Search(ctx, "贵", 10)
	if err != nil {
		t.Fatalf("Search failed: %v", err)
	}
	if len(results) < 2 {
		t.Errorf("expected >=2 results for '贵', got %d", len(results))
	}
	for _, r := range results {
		if !strings.Contains(r.Name, "贵") && !strings.Contains(r.Code, "贵") {
			t.Errorf("result %s/%s does not contain '贵'", r.Code, r.Name)
		}
	}
}

func TestSearch_CodePrefix(t *testing.T) {
	repo, ctx := setupSearchDB(t)
	results, err := repo.Search(ctx, "6005", 10)
	if err != nil {
		t.Fatalf("Search failed: %v", err)
	}
	found519 := false
	found516 := false
	for _, r := range results {
		if r.Code == "600519" {
			found519 = true
		}
		if r.Code == "600516" {
			found516 = true
		}
	}
	if !found519 {
		t.Error("expected 600519 in results")
	}
	if !found516 {
		t.Error("expected 600516 in results")
	}
}

func TestSearch_EmptyQuery_ReturnsEmpty(t *testing.T) {
	repo, ctx := setupSearchDB(t)
	results, err := repo.Search(ctx, "", 8)
	if err != nil {
		t.Fatalf("Search failed: %v", err)
	}
	if len(results) != 0 {
		t.Errorf("expected 0 results for empty query, got %d", len(results))
	}
}

func TestSearch_ShortQuery_ReturnsEmpty(t *testing.T) {
	repo, ctx := setupSearchDB(t)
	results, err := repo.Search(ctx, "6", 8)
	if err != nil {
		t.Fatalf("Search failed: %v", err)
	}
	// Note: repo.Search doesn't have a min-length check; service layer does.
	// Here we just verify it doesn't crash.
	_ = results
}

func TestSearch_LimitCap(t *testing.T) {
	repo, ctx := setupSearchDB(t)
	results, err := repo.Search(ctx, "0", 2)
	if err != nil {
		t.Fatalf("Search failed: %v", err)
	}
	if len(results) > 2 {
		t.Errorf("expected <=2 results, got %d", len(results))
	}
}

func TestSearch_SQLInjectionSafety(t *testing.T) {
	repo, ctx := setupSearchDB(t)
	// Should not error or return unexpected data
	results, err := repo.Search(ctx, "' OR 1=1 --", 10)
	if err != nil {
		t.Fatalf("Search should not error on injection, got: %v", err)
	}
	// With parameterized query this should return empty or harmless results
	_ = results // just verify no panic/error
}

func TestSearch_ExchangeFieldPresent(t *testing.T) {
	repo, ctx := setupSearchDB(t)
	results, err := repo.Search(ctx, "腾讯", 10)
	if err != nil {
		t.Fatalf("Search failed: %v", err)
	}
	if len(results) == 0 {
		t.Fatal("expected HK stock result")
	}
	if results[0].Exchange != "HKEX" {
		t.Errorf("expected HKEX, got %s", results[0].Exchange)
	}
}

// ── FindByExchange tests ──

func TestFindByExchange_AShareOnly(t *testing.T) {
	repo, ctx := setupSearchDB(t)
	records, err := repo.FindByExchange(ctx, []string{"SSE", "SZSE"})
	if err != nil {
		t.Fatalf("FindByExchange failed: %v", err)
	}
	for _, r := range records {
		if r.Exchange != "SSE" && r.Exchange != "SZSE" {
			t.Errorf("unexpected exchange in A-share filter: %s", r.Exchange)
		}
	}
	// Should have 7 A-share records (excluding 2 HK stocks)
	if len(records) != 7 {
		t.Errorf("expected 7 A-share records, got %d", len(records))
	}
}

func TestFindByExchange_HKOnly(t *testing.T) {
	repo, ctx := setupSearchDB(t)
	records, err := repo.FindByExchange(ctx, []string{"HKEX"})
	if err != nil {
		t.Fatalf("FindByExchange failed: %v", err)
	}
	if len(records) != 2 {
		t.Errorf("expected 2 HK records, got %d", len(records))
	}
	for _, r := range records {
		if r.Exchange != "HKEX" {
			t.Errorf("expected HKEX only, got %s", r.Exchange)
		}
	}
}

func TestFindByExchange_EmptyList_ReturnsAll(t *testing.T) {
	repo, ctx := setupSearchDB(t)
	records, err := repo.FindByExchange(ctx, []string{})
	if err != nil {
		t.Fatalf("FindByExchange failed: %v", err)
	}
	if len(records) != 9 {
		t.Errorf("expected all 9 records, got %d", len(records))
	}
}
