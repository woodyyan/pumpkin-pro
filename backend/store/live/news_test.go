package live

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/woodyyan/pumpkin-pro/backend/tests/testutil"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (fn roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return fn(req)
}

func TestNewsService_GetSymbolNews_FiltersAndDigests(t *testing.T) {
	payload := buildNewsFixturePayload()

	db := testutil.InMemoryDB(t)
	testutil.AutoMigrateModels(t, db, &StockNewsRecord{}, &StockNewsCacheRecord{})
	repo := NewRepository(db)
	svc := NewNewsService(repo, "https://quant.example")
	svc.client = &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			body, err := json.Marshal(payload)
			if err != nil {
				return nil, err
			}
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     make(http.Header),
				Body:       io.NopCloser(strings.NewReader(string(body))),
				Request:    req,
			}, nil
		}),
	}

	result, err := svc.GetSymbolNews(context.Background(), "600519.SH", StockNewsListOptions{Type: "announcement", Limit: 10})
	if err != nil {
		t.Fatalf("GetSymbolNews returned error: %v", err)
	}
	if len(result.Items) != 1 {
		t.Fatalf("expected 1 filtered item, got %d", len(result.Items))
	}
	if result.Items[0].Type != "announcement" {
		t.Fatalf("expected announcement item, got %s", result.Items[0].Type)
	}

	fullResult, err := svc.GetSymbolNews(context.Background(), "600519.SH", StockNewsListOptions{Type: "all", Limit: 10})
	if err != nil {
		t.Fatalf("GetSymbolNews (all) returned error: %v", err)
	}
	if fullResult.Summary.FilingCount != 1 {
		t.Fatalf("expected summary filing count to stay 1, got %d", fullResult.Summary.FilingCount)
	}

	digest, err := svc.BuildAIDigest(context.Background(), "600519.SH", 2)
	if err != nil {
		t.Fatalf("BuildAIDigest returned error: %v", err)
	}
	if len(digest.Items) != 2 {
		t.Fatalf("expected digest item count capped to 2, got %d", len(digest.Items))
	}
	if digest.Summary["filing_count"].(int) != 1 {
		t.Fatalf("expected filing_count=1 in digest summary, got %+v", digest.Summary)
	}
}

func TestNewsService_FallsBackToPersistedDataWhenQuantTimeouts(t *testing.T) {
	db := testutil.InMemoryDB(t)
	testutil.AutoMigrateModels(t, db, &StockNewsRecord{}, &StockNewsCacheRecord{})
	repo := NewRepository(db)
	ctx := context.Background()
	payload := fixturePayloadStruct()
	rawItems := buildNewsFixtureRawItems()

	svc := NewNewsService(repo, "https://quant.example")
	svc.persistPayload(ctx, "600519.SH", payload, rawItems)
	svc.client = &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			return nil, context.DeadlineExceeded
		}),
	}

	result, err := svc.GetSymbolNews(ctx, "600519.SH", StockNewsListOptions{Type: "all", Limit: 10})
	if err != nil {
		t.Fatalf("expected persisted fallback without error, got %v", err)
	}
	if len(result.Items) != 3 {
		t.Fatalf("expected persisted items, got %d", len(result.Items))
	}
	if result.Meta["storage"] != "database" {
		t.Fatalf("expected storage=database, got %+v", result.Meta)
	}
}

func TestNewsService_UsesPersistedDataBeforeExpiredCache(t *testing.T) {
	db := testutil.InMemoryDB(t)
	testutil.AutoMigrateModels(t, db, &StockNewsRecord{}, &StockNewsCacheRecord{})
	repo := NewRepository(db)
	ctx := context.Background()
	payload := fixturePayloadStruct()
	rawItems := buildNewsFixtureRawItems()

	if err := repo.UpsertStockNewsCache(ctx, "600519.SH", "list", `{"stale":true}`); err != nil {
		t.Fatalf("seed list cache failed: %v", err)
	}
	if err := repo.UpsertStockNewsCache(ctx, "600519.SH", "summary", `{"stale":true}`); err != nil {
		t.Fatalf("seed summary cache failed: %v", err)
	}
	if err := repo.ReplaceStockNewsItems(ctx, "600519.SH", payload.Items, rawItems); err != nil {
		t.Fatalf("seed persisted items failed: %v", err)
	}
	backdated := time.Now().UTC().Add(-4 * time.Hour)
	if err := db.Model(&StockNewsCacheRecord{}).Where("symbol = ?", "600519.SH").Update("fetched_at", backdated).Error; err != nil {
		t.Fatalf("backdate cache failed: %v", err)
	}

	release := make(chan struct{})
	svc := NewNewsService(repo, "https://quant.example")
	svc.client = &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			<-release
			return nil, fmt.Errorf("background refresh failed as expected")
		}),
	}

	resultCh := make(chan *StockNewsPayload, 1)
	errCh := make(chan error, 1)
	go func() {
		result, err := svc.GetSymbolNewsSummary(ctx, "600519.SH")
		if err != nil {
			errCh <- err
			return
		}
		resultCh <- result
	}()

	select {
	case err := <-errCh:
		t.Fatalf("expected db-backed summary, got %v", err)
	case result := <-resultCh:
		if result.Summary.FilingCount != 1 {
			t.Fatalf("expected summary from persisted rows, got %+v", result.Summary)
		}
	case <-time.After(150 * time.Millisecond):
		t.Fatalf("expected persisted summary to return without waiting for quant refresh")
	}

	close(release)
}

func TestNewsService_ReturnsDegradedPayloadWhenNoCacheAndQuantTimeouts(t *testing.T) {
	db := testutil.InMemoryDB(t)
	testutil.AutoMigrateModels(t, db, &StockNewsRecord{}, &StockNewsCacheRecord{})
	repo := NewRepository(db)
	svc := NewNewsService(repo, "https://quant.example")
	svc.client = &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			return nil, context.DeadlineExceeded
		}),
	}

	result, err := svc.GetSymbolNews(context.Background(), "601138.SH", StockNewsListOptions{Type: "all", Limit: 10})
	if err != nil {
		t.Fatalf("expected degraded payload instead of error, got %v", err)
	}
	if len(result.Items) != 0 {
		t.Fatalf("expected empty degraded items, got %d", len(result.Items))
	}
	if !isDegradedNewsPayload(result) {
		t.Fatalf("expected degraded payload meta, got %+v", result.Meta)
	}
}

func TestRepository_StockNewsCacheTTL(t *testing.T) {
	db := testutil.InMemoryDB(t)
	testutil.AutoMigrateModels(t, db, &StockNewsCacheRecord{})
	repo := NewRepository(db)
	ctx := context.Background()

	if err := repo.UpsertStockNewsCache(ctx, "600519.SH", "summary", `{"ok":true}`); err != nil {
		t.Fatalf("UpsertStockNewsCache failed: %v", err)
	}

	if _, hit, err := repo.GetStockNewsCache(ctx, "600519.SH", "summary", time.Hour); err != nil || !hit {
		t.Fatalf("expected cache hit, err=%v hit=%v", err, hit)
	}

	if err := db.Model(&StockNewsCacheRecord{}).Where("symbol = ? AND scope = ?", "600519.SH", "summary").Update("fetched_at", time.Now().UTC().Add(-2*time.Hour)).Error; err != nil {
		t.Fatalf("failed to backdate cache row: %v", err)
	}

	if _, hit, err := repo.GetStockNewsCache(ctx, "600519.SH", "summary", time.Minute); err != nil {
		t.Fatalf("GetStockNewsCache after expiry returned error: %v", err)
	} else if hit {
		t.Fatalf("expected cache miss after TTL expiry")
	}
}

func buildNewsFixturePayload() map[string]any {
	return map[string]any{
		"symbol":     "600519.SH",
		"exchange":   "SSE",
		"updated_at": "2026-04-27T10:32:00Z",
		"summary": map[string]any{
			"last_24h_count":     3,
			"announcement_count": 1,
			"filing_count":       1,
			"latest_headline":    "2026Q1 财报发布",
			"highlight_tags":     []string{"财报", "业绩"},
		},
		"items": buildNewsFixtureRawItems(),
		"meta":  map[string]any{},
	}
}

func buildNewsFixtureRawItems() []map[string]any {
	return []map[string]any{
		{
			"id":               "filing-1",
			"type":             "filing",
			"source_type":      "official",
			"source_name":      "财务披露",
			"title":            "2026Q1 财报发布",
			"summary":          "净利润增长 32%",
			"published_at":     "2026-04-27T09:28:00Z",
			"url":              "",
			"report_period":    "2026Q1",
			"report_type":      "一季报",
			"importance_score": 98,
			"is_ai_relevant":   true,
		},
		{
			"id":               "news-1",
			"type":             "news",
			"source_type":      "media",
			"source_name":      "财联社",
			"title":            "新品进入放量周期",
			"summary":          "渠道反馈改善",
			"published_at":     "2026-04-27T08:41:00Z",
			"url":              "https://example.com/news-1",
			"importance_score": 65,
			"is_ai_relevant":   true,
		},
		{
			"id":               "announcement-1",
			"type":             "announcement",
			"source_type":      "official",
			"source_name":      "上交所",
			"title":            "回购公告",
			"summary":          "回购金额上限提升",
			"published_at":     "2026-04-26T18:00:00Z",
			"url":              "https://example.com/notice-1",
			"importance_score": 88,
			"is_ai_relevant":   true,
		},
	}
}

func fixturePayloadStruct() *StockNewsPayload {
	items := []StockNewsItem{
		{
			ID:              "filing-1",
			Type:            "filing",
			SourceType:      "official",
			SourceName:      "财务披露",
			Title:           "2026Q1 财报发布",
			Summary:         "净利润增长 32%",
			PublishedAt:     "2026-04-27T09:28:00Z",
			ReportPeriod:    "2026Q1",
			ReportType:      "一季报",
			ImportanceScore: 98,
			IsAIRelevant:    true,
		},
		{
			ID:              "news-1",
			Type:            "news",
			SourceType:      "media",
			SourceName:      "财联社",
			Title:           "新品进入放量周期",
			Summary:         "渠道反馈改善",
			PublishedAt:     "2026-04-27T08:41:00Z",
			URL:             "https://example.com/news-1",
			ImportanceScore: 65,
			IsAIRelevant:    true,
		},
		{
			ID:              "announcement-1",
			Type:            "announcement",
			SourceType:      "official",
			SourceName:      "上交所",
			Title:           "回购公告",
			Summary:         "回购金额上限提升",
			PublishedAt:     "2026-04-26T18:00:00Z",
			URL:             "https://example.com/notice-1",
			ImportanceScore: 88,
			IsAIRelevant:    true,
		},
	}
	return &StockNewsPayload{
		Symbol:    "600519.SH",
		Exchange:  "SSE",
		UpdatedAt: "2026-04-27T10:32:00Z",
		Summary:   buildSummaryFromItems(items, StockNewsSummary{}),
		Items:     items,
		Meta:      map[string]any{},
	}
}
