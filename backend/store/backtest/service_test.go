package backtest

import (
	"context"
	"testing"
	"time"

	"github.com/woodyyan/pumpkin-pro/backend/tests/testutil"
)

func setupBacktestTest(t *testing.T) (*Repository, func()) {
	t.Helper()
	db := testutil.InMemoryDB(t)
	testutil.AutoMigrateModels(t, db, &BacktestRunRecord{})
	return NewRepository(db), func() {}
}

func makeBacktestRecord(userID string) BacktestRunRecord {
	return BacktestRunRecord{
		ID:           "bt-" + userID + "-" + time.Now().Format("150405"),
		UserID:       userID,
		Title:        "测试回测",
		DataSource:   "sample",
		Ticker:       "000001.SZ",
		TickerName:   "平安银行",
		StrategyID:   "strat-001",
		StrategyName: "均线交叉策略",
		StartDate:    "2026-01-01",
		EndDate:      "2026-03-31",
		Capital:      100000,
		FeePct:       0.001,
		Status:       "success",
		DurationMS:   2500,
		SummaryJSON:  `{"total_return_pct":15.2}`,
		ResultJSON:   `{"metrics":{"sharpe_ratio":1.2}}`,
		CreatedAt:    time.Now().UTC(),
	}
}

// ── Repository Tests ──

func TestBacktestRepo_CreateAndGet(t *testing.T) {
	repo, cleanup := setupBacktestTest(t)
	defer cleanup()
	ctx := context.Background()

	record := makeBacktestRecord("user-bt1")
	err := repo.Create(ctx, &record)
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	got, err := repo.GetByID(ctx, "user-bt1", record.ID)
	if err != nil {
		t.Fatalf("GetByID failed: %v", err)
	}
	if got.Title != "测试回测" {
		t.Errorf("expected title '测试回测', got %s", got.Title)
	}
	if got.Capital != 100000 {
		t.Errorf("expected capital 100000, got %f", got.Capital)
	}
}

func TestBacktestRepo_GetByID_NotFound(t *testing.T) {
	repo, cleanup := setupBacktestTest(t)
	defer cleanup()

	_, err := repo.GetByID(context.Background(), "u1", "nonexistent")
	if !IsNotFoundError(err) {
		t.Errorf("expected ErrNotFound (or gorm.ErrRecordNotFound), got %v", err)
	}
}

func TestBacktestRepo_ListByUser_Pagination(t *testing.T) {
	repo, cleanup := setupBacktestTest(t)
	defer cleanup()
	ctx := context.Background()

	// Create 5 records
	for i := 0; i < 5; i++ {
		r := makeBacktestRecord("list-user")
		r.ID = t.Name() + "-run-" + itobt(i)
		_ = repo.Create(ctx, &r)
	}

	records, total, err := repo.ListByUser(ctx, "list-user", 3, 0)
	if err != nil {
		t.Fatalf("ListByUser failed: %v", err)
	}
	if total != 5 {
		t.Errorf("expected total 5, got %d", total)
	}
	if len(records) != 3 {
		t.Errorf("expected 3 records with limit=3, got %d", len(records))
	}
}

func TestBacktestRepo_Delete(t *testing.T) {
	repo, cleanup := setupBacktestTest(t)
	defer cleanup()
	ctx := context.Background()

	record := makeBacktestRecord("del-user")
	_ = repo.Create(ctx, &record)

	err := repo.Delete(ctx, "del-user", record.ID)
	if err != nil {
		t.Fatalf("Delete failed: %v", err)
	}

	_, err = repo.GetByID(ctx, "del-user", record.ID)
	if !IsNotFoundError(err) {
		t.Errorf("expected not found after delete, got %v", err)
	}
}

func TestBacktestRepo_Delete_NotFound(t *testing.T) {
	repo, cleanup := setupBacktestTest(t)
	defer cleanup()

	err := repo.Delete(context.Background(), "u1", "nonexistent")
	if !IsNotFoundError(err) {
		t.Errorf("expected not found error, got %v", err)
	}
}

func TestBacktestRepo_CountByUser(t *testing.T) {
	repo, cleanup := setupBacktestTest(t)
	defer cleanup()
	ctx := context.Background()

	count, err := repo.CountByUser(ctx, "count-user")
	if err != nil {
		t.Fatalf("CountByUser failed: %v", err)
	}
	if count != 0 {
		t.Errorf("expected count 0 initially, got %d", count)
	}

	record := makeBacktestRecord("count-user")
	_ = repo.Create(ctx, &record)

	count, _ = repo.CountByUser(ctx, "count-user")
	if count != 1 {
		t.Errorf("expected count 1 after insert, got %d", count)
	}
}

func TestBacktestRepo_DeleteOldest(t *testing.T) {
	repo, cleanup := setupBacktestTest(t)
	defer cleanup()
	ctx := context.Background()

	// Create 4 records
	var ids [4]string
	for i := 0; i < 4; i++ {
		r := makeBacktestRecord("oldest-user")
		r.ID = "old-" + itobt(i)
		ids[i] = r.ID
		_ = repo.Create(ctx, &r)
	}

	// Keep only 2 newest
	err := repo.DeleteOldest(ctx, "oldest-user", 2)
	if err != nil {
		t.Fatalf("DeleteOldest failed: %v", err)
	}

	// Verify only 2 remain
	count, _ := repo.CountByUser(ctx, "oldest-user")
	if count != 2 {
		t.Errorf("expected 2 remaining after DeleteOldest(keep=2), got %d", count)
	}
}

// ── Service Tests ──

func TestBacktestService_SaveAndList(t *testing.T) {
	repo, cleanup := setupBacktestTest(t)
	defer cleanup()
	s := NewService(repo)

	request := map[string]any{
		"data_source": "sample",
		"ticker":      "600036.SH",
		"start_date":  "2025-06-01",
		"end_date":    "2026-03-31",
		"capital":     float64(50000),
		"fee_pct":     float64(0.002),
		"strategy_id": "strat-rsi",
		"strategy_name": "RSI Range Strategy",
	}
	result := map[string]any{
		"metrics": map[string]any{
			"total_return_pct":    float64(22.5),
			"annual_return_pct":   float64(18.3),
			"max_drawdown_pct":    float64(-12.1),
			"sharpe_ratio":        float64(1.45),
			"total_trades":        float64(28),
			"final_capital":       float64(61250),
			"win_rate_pct":        float64(68.5),
		},
		"data_summary": map[string]any{
			"ticker_name":   "招商银行",
			"ticker_display": "招商银行 (600036)",
		},
	}

	err := s.saveRun(context.Background(), "svc-user", request, result, 1200, "success")
	if err != nil {
		t.Fatalf("saveRun failed: %v", err)
	}

	items, total, err := s.List(context.Background(), "svc-user", 10, 0)
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}
	if total != 1 {
		t.Errorf("expected 1 item in list, got %d", total)
	}
	if len(items) != 1 {
		t.Fatalf("expected 1 item returned, got %d", len(items))
	}
	item := items[0]
	if item.Title == "" {
		t.Error("title should not be empty after save")
	}
	if item.Status != "success" {
		t.Errorf("expected status 'success', got %s", item.Status)
	}
}

func TestBacktestService_GetByID(t *testing.T) {
	repo, cleanup := setupBacktestTest(t)
	defer cleanup()
	s := NewService(repo)

	request := map[string]any{"data_source": "csv", "capital": float64(100000)}
	result := map[string]any{"metrics": map[string]any{}}
	_ = s.saveRun(context.Background(), "detail-user", request, result, 500, "success")

	items, _, _ := s.List(context.Background(), "detail-user", 1, 0)
	if len(items) == 0 {
		t.Fatal("no items to get detail for")
	}

	detail, err := s.GetByID(context.Background(), "detail-user", items[0].ID)
	if err != nil {
		t.Fatalf("GetByID failed: %v", err)
	}
	if detail.ID != items[0].ID {
		t.Errorf("detail ID mismatch: %s vs %s", detail.ID, items[0].ID)
	}
}

func TestBacktestService_GetByID_NotFound(t *testing.T) {
	repo, cleanup := setupBacktestTest(t)
	defer cleanup()
	s := NewService(repo)

	_, err := s.GetByID(context.Background(), "nope", "nonexistent")
	if err != ErrNotFound {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

func TestBacktestService_Delete(t *testing.T) {
	repo, cleanup := setupBacktestTest(t)
	defer cleanup()
	s := NewService(repo)

	request := map[string]any{"capital": float64(1000)}
	_ = s.saveRun(context.Background(), "del-bt-user", request, map[string]any{}, 100, "success")

	items, _, _ := s.List(context.Background(), "del-bt-user", 1, 0)
	if len(items) == 0 {
		t.Fatal("no items to delete")
	}

	err := s.Delete(context.Background(), "del-bt-user", items[0].ID)
	if err != nil {
		t.Fatalf("Delete failed: %v", err)
	}

	// Should be gone
	items2, _, _ := s.List(context.Background(), "del-bt-user", 10, 0)
	if len(items2) != 0 {
		t.Errorf("expected 0 items after delete, got %d", len(items2))
	}
}

func TestBacktestService_List_LimitClamp(t *testing.T) {
	repo, cleanup := setupBacktestTest(t)
	defer cleanup()
	s := NewService(repo)

	items, total, err := s.List(context.Background(), "limit-user", -1, -5)
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}
	if total != 0 { // no data yet
		t.Error("expected 0 total when no data exists")
	}
	_ = items // just verify it doesn't panic
	_ = total
}

// ── Helper function tests ──

func TestBuildTitle(t *testing.T) {
	tests := []struct {
		display, name, strategy, dataSource string
		contains                              []string
	}{
		{"平安银行", "PING AN BANK", "MACD 策略", "sample", []string{"平安银行", "PING AN BANK", "MACD"}},
		{"600036.SH", "", "", "csv", []string{"600036.SH"}},  // tickerDisplay takes precedence over dataSource
		{"", "", "", "sample", []string{"示例行情"}},
		{"", "", "", "", []string{"回测"}},
		{"腾讯", "", "布林带策略", "akshare", []string{"腾讯", "布林带策略"}},
	}
	for _, tc := range tests {
		got := buildTitle(tc.display, tc.name, tc.strategy, tc.dataSource)
		for _, substr := range tc.contains {
			if !stringsContain(got, substr) {
				t.Errorf("buildTitle(%q,%q,%q,%q)=%q should contain %q",
					tc.display, tc.name, tc.strategy, tc.dataSource, got, substr)
			}
		}
	}
}

func TestAsString(t *testing.T) {
	tests := []struct {
		input any
		want  string
	}{
		{nil, ""},
		{"hello", "hello"},
		{"  spaced  ", "spaced"},
		{123, "123"},
		{float64(3.14), "3.14"},
		{true, "true"},
	}
	for _, tc := range tests {
		got := asString(tc.input)
		if got != tc.want {
			t.Errorf("asString(%v) = %q, want %q", tc.input, got, tc.want)
		}
	}
}

func TestAsFloat64(t *testing.T) {
	tests := []struct {
		input any
		want  float64
	}{
		{nil, 0},
		{float64(99.9), 99.9},
		{int(42), 42.0},
		{0, 0},
		{"not-a-number", 0}, // non-string numbers return 0
	}
	for _, tc := range tests {
		got := asFloat64(tc.input)
		if got != tc.want {
			t.Errorf("asFloat64(%v) = %f, want %f", tc.input, got, tc.want)
		}
	}
}

func TestAsMapNested(t *testing.T) {
	// nil parent
	m := asMapNested(nil, "key")
	if m == nil || len(m) != 0 {
		t.Error("asMapNested(nil) should return non-nil empty map")
	}

	// key doesn't exist
	parent := map[string]any{"a": 1}
	m = asMapNested(parent, "missing")
	if m == nil || len(m) != 0 {
		t.Error("asMapNested with missing key should return non-nil empty map")
	}

	// key exists but value is not a map
	parent["bad"] = "string-value"
	m = asMapNested(parent, "bad")
	if m == nil || len(m) != 0 {
		t.Error("asMapNested with non-map value should return non-nil empty map")
	}

	// normal case
	parent["good"] = map[string]any{"nested_key": "nested_val"}
	m = asMapNested(parent, "good")
	if m["nested_key"] != "nested_val" {
		t.Errorf("asMapNested should find nested value, got %v", m)
	}
}

// ── Model conversion tests ──

func TestBacktestRunRecord_ToListItem(t *testing.T) {
	now := time.Now().UTC()
	r := BacktestRunRecord{
		ID:          "list-item-01",
		Title:       "ListItem Title",
		DataSource:  "akshare",
		Ticker:      "000001.SZ",
		TickerName:  "平安银行",
		StrategyName: "MA Cross",
		StartDate:   "2026-01-01",
		EndDate:     "2026-03-31",
		Capital:     200000,
		Status:      "running",
		DurationMS:  5500,
		SummaryJSON: `{"sharpe_ratio":1.8,"total_return_pct":25.5}`,
		CreatedAt:   now,
	}

	item := r.toListItem()
	if item.ID != r.ID {
		t.Errorf("ID mismatch: %s vs %s", item.ID, r.ID)
	}
	if item.MetricsSummary == nil {
		t.Error("MetricsSummary should be parsed from SummaryJSON")
	}
	if item.MetricsSummary["sharpe_ratio"] != float64(1.8) {
		t.Errorf("sharpe_ratio mismatch: %v", item.MetricsSummary["sharpe_ratio"])
	}
}

func TestBacktestRunRecord_ToDetail(t *testing.T) {
	now := time.Now().UTC()
	r := BacktestRunRecord{
		ID:           "detail-01",
		Title:        "Detail Title",
		ResultJSON:   `{"metrics":{"max_drawdown_pct":-8},"trades":[...]}`,
		FeePct:       0.001,
		CreatedAt:    now,
	}

	detail := r.toDetail()
	if detail.Result == nil {
		t.Error("Result should be parsed from ResultJSON")
	}
	if detail.FeePct != 0.001 {
		t.Errorf("FeePct mismatch: got %f", detail.FeePct)
	}
}

// ── Helpers ──

func itobt(i int) string {
	switch i {
	case 0: return "0"
	case 1: return "1"
	case 2: return "2"
	case 3: return "3"
	case 4: return "4"
	default: return "N"
	}
}

func IsNotFoundError(err error) bool {
	if err == ErrNotFound {
		return true
	}
	// Also accept gorm's record-not-found error
	return err != nil && (err.Error() == "record not found" ||
		containsString(err.Error(), "record not found"))
}

func stringsContain(s, substr string) bool {
	return containsString(s, substr)
}

func containsString(s, sub string) bool {
	return len(s) >= len(sub) && searchString(s, sub)
}

func searchString(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
