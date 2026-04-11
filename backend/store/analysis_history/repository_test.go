package analysis_history

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupTestDB(t *testing.T) (*Repository, func()) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&AnalysisHistoryRecord{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return NewRepository(db), func() {
		db.Exec("DROP TABLE stock_analysis_history")
		sql, _ := db.DB()
		sql.Close()
	}
}

func sampleAPIResponse() []byte {
	b, _ := json.Marshal(map[string]any{
		"analysis": map[string]any{
			"signal": "buy", "confidence_score": 78,
			"logic_summary": "MACD cross", "data_timestamp": "2026/04/11 12:00:00",
		},
		"meta": map[string]any{
			"model": "gpt-4", "generated_at": "2026-04-11T12:00:00Z",
			"symbol_meta": map[string]any{"symbol": "000001.SZ", "name": "平安银行"},
		},
	})
	return b
}

// ═════ 1. SaveFromAPIResponse ═════

func TestSaveFromAPIResponse_Normal(t *testing.T) {
	repo, cleanup := setupTestDB(t)
	defer cleanup()
	ctx := context.Background()

	if err := repo.SaveFromAPIResponse(ctx, "u1", "000001.SZ", "平安银行", sampleAPIResponse()); err != nil {
		t.Fatalf("save: %v", err)
	}
	rec, err := repo.GetLatestBySymbol(ctx, "u1", "000001.SZ")
	if err != nil || rec == nil {
		t.Fatal("expected saved record")
	}
	if rec.Symbol != "000001.SZ" || rec.SymbolName != "平安银行" || rec.Signal != "buy" || rec.ConfidenceScore != 78 {
		t.Errorf("fields wrong: sym=%q name=%q sig=%q conf=%d", rec.Symbol, rec.SymbolName, rec.Signal, rec.ConfidenceScore)
	}
	if rec.ID == "" || rec.ResultJSON == "" {
		t.Error("ID or ResultJSON empty")
	}
}

func TestSaveFromAPIResponse_EmptyAnalysis(t *testing.T) {
	repo, cleanup := setupTestDB(t)
	defer cleanup()
	if err := repo.SaveFromAPIResponse(context.Background(), "u", "s", "", []byte(`{"meta":{},"analysis":null}`)); err == nil {
		t.Error("expected error for null analysis")
	}
}

func TestSaveFromAPIResponse_InvalidJSON(t *testing.T) {
	repo, cleanup := setupTestDB(t)
	defer cleanup()
	if err := repo.SaveFromAPIMock(context.Background(), "s", "", []byte("not json")); err == nil {
		t.Error("expected error for bad JSON")
	}
}

func TestSaveFromAPIResponse_MissingFields(t *testing.T) {
	repo, cleanup := setupTestDB(t)
	defer cleanup()
	partial := `{"analysis":{"logic_summary":"no signal"},"meta":{}}`
	if err := repo.SaveFromAPIMock(context.Background(), "TEST.HK", "腾讯", []byte(partial)); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	rec, err := repo.GetLatestBySymbol(context.Background(), "mock-user", "TEST.HK")
	if err != nil || rec == nil {
		t.Fatalf("expected record: %v", err)
	}
	if rec.Signal != "" || rec.ConfidenceScore != 0 {
		t.Errorf("expected zero values, got %q / %d", rec.Signal, rec.ConfidenceScore)
	}
}

func TestSaveFromAPIResponse_OldFormatNoSymbolMeta(t *testing.T) {
	repo, cleanup := setupTestDB(t)
	defer cleanup()
	oldResp := `{"analysis":{"signal":"hold","confidence_score":50},"meta":{"model":"old-model"}}`
	if err := repo.SaveFromAPIMock(context.Background(), "600519.SH", "贵州茅台", []byte(oldResp)); err != nil {
		t.Fatalf("unexpected: %v", err)
	}
	rec, err := repo.GetLatestBySymbol(context.Background(), "mock-user", "600519.SH")
	if err != nil || rec == nil { t.Fatalf("expected record: %v", err) }
	if rec.Symbol != "600519.SH" { t.Errorf("symbol = %q", rec.Symbol) }
}

func TestSaveFromAPIResponse_RoundTrip(t *testing.T) {
	repo, cleanup := setupTestDB(t)
	defer cleanup()
	ctx := context.Background()

	if err := repo.SaveFromAPResponse(ctx, "u-rt", "000002.SZ", "万科A", sampleAPIResponse()); err != nil {
		t.Fatalf("save: %v", err)
	}
	records, err := repo.ListBySymbol(ctx, "u-rt", "000002.SZ", 10)
	if err != nil || len(records) == 0 { t.Fatalf("list: %v", err) }

	detail, _ := records[0].ToDetail()
	if detail.Result["signal"] != "buy" { t.Errorf("signal = %v", detail.Result["signal"]) }
	if detail.Meta["model"] == nil { t.Error("missing meta.model") }
}

// ═════ 2. Create — eviction ═════

func TestCreate_WithinLimit(t *testing.T) {
	repo, cleanup := setupTestDB(t)
	defer cleanup()
	ctx := context.Background()
	for i := 0; i < MaxRecordsPerUser-1; i++ {
		r := &AnalysisHistoryRecord{ID: genID(), UserID: "u-evict", Symbol: "S.SZ", ResultJSON: "{}", MetaJSON: "{}",
			CreatedAt: time.Now().UTC().Add(time.Duration(i) * time.Minute)}
		if err := repo.Create(ctx, r); err != nil { t.Fatalf("#%d: %v", i, err) }
	}
	var c int64
	repo.db.WithContext(ctx).Model(&AnalysisHistoryRecord{}).Where("user_id = ?", "u-evict").Count(&c)
	if int(c) != MaxRecordsPerUser-1 { t.Errorf("count = %d", c) }
}

func TestCreate_AutoEviction(t *testing.T) {
	repo, cleanup := setupTestDB(t)
	defer cleanup()
	ctx := context.Background()
	firstID := ""
	for i := 0; i < MaxRecordsPerUser; i++ {
		r := &AnalysisHistoryRecord{ID: genID(), UserID: "u-e2", Symbol: "E.SZ", ResultJSON: "{}", MetaJSON: "{}",
			CreatedAt: time.Now().UTC().Add(time.Duration(i) * time.Minute)}
		if i == 0 { firstID = r.ID }
		_ = repo.Create(ctx, r)
	}
	_ = repo.Create(ctx, &AnalysisHistoryRecord{ID: genID(), UserID: "u-e2", Symbol: "E.SZ", ResultJSON: "{}", MetaJSON: "{}",
		CreatedAt: time.Now().UTC().Add(time.Hour)})
	var oldRec AnalysisHistoryRecord
	if repo.db.WithContext(ctx).Where("id = ?", firstID).First(&oldRec).Error == nil {
		t.Error("oldest should be evicted")
	}
}

// ═════ 3. CRUD ═════

func TestListBySymbol(t *testing.T) {
	repo, cleanup := setupTestDB(t)
	defer cleanup()
	ctx := context.Background()
	for i := 0; i < 3; i++ {
		_ = repo.Create(ctx, &AnalysisHistoryRecord{ID: genID(), UserID: "u-list", Symbol: "LIST.SZ",
			ResultJSON: "{}", MetaJSON: "{}", CreatedAt: time.Now().Add(time.Duration(i) * time.Minute)})
	}
	recs, err := repo.ListBySymbol(ctx, "u-list", "LIST.SZ", 10)
	if err != nil || len(recs) != 3 { t.Fatalf("list: %v (len=%d)", err, len(recs)) }
	if recs[0].CreatedAt.Before(recs[1].CreatedAt) { t.Error("not descending") }
}

func TestGetLatestBySymbol(t *testing.T) {
	repo, cleanup := setupTestDB(t)
	defer cleanup()
	ctx := context.Background()
	old := &AnalysisHistoryRecord{ID: genID(), UserID: "u-gl", Symbol: "GL.SZ", ResultJSON: "{}", MetaJSON: "{}", CreatedAt: time.Now().Add(-time.Hour)}
	newer := &AnalysisHistoryRecord{ID: genID(), UserID: "u-gl", Symbol: "GL.SZ", ResultJSON: "{}", MetaJSON: "{}", CreatedAt: time.Now()}
	_ = repo.Create(ctx, old)
	_ = repo.Create(ctx, newer)
	l, err := repo.GetLatestBySymbol(ctx, "u-gl", "GL.SZ")
	if err != nil || l.ID != newer.ID { t.Error("should get newest") }
}

func TestGetByID_UserIsolation(t *testing.T) {
	repo, cleanup := setupTestDB(t)
	defer cleanup()
	ctx := context.Background()
	r := &AnalysisHistoryRecord{ID: genID(), UserID: "owner", Symbol: "X.SZ", ResultJSON: "{}", MetaJSON: "{}"}
	_ = repo.Create(ctx, r)
	if _, err := repo.GetByID(ctx, "stranger", r.ID); err == nil { t.Error("should not find other user") }
}

func TestDelete(t *testing.T) {
	repo, cleanup := setupTestDB(t)
	defer cleanup()
	ctx := context.Background()
	r := &AnalysisHistoryRecord{ID: genID(), UserID: "u-del", Symbol: "DEL.SZ", ResultJSON: "{}", MetaJSON: "{}"}
	_ = repo.Create(ctx, r)
	if err := repo.Delete(ctx, "u-del", r.ID); err != nil { t.Fatalf("del: %v", err) }
	if _, err := repo.GetByID(ctx, "u-del", r.ID); err == nil { t.Error("deleted record exists") }
}

// ═════ 4. ToListItem / ToDetail ═════

func TestToListItem(t *testing.T) {
	now := time.Date(2026, 4, 11, 12, 0, 0, 0, time.UTC)
	item := AnalysisHistoryRecord{ID: "id-123", Symbol: "T.HK", Signal: "sell", ConfidenceScore: 65, CreatedAt: now}.ToListItem()
	if item.ID != "id-123" || item.CreatedAt != now.Format(time.RFC3339) { t.Error("mismatch") }
}

func TestToDetail_WithResult(t *testing.T) {
	d, err := AnalysisHistoryRecord{ID: "d1", ResultJSON: `{"signal":"buy","score":80}`, MetaJSON: `{"model":"gpt-4"}`, CreatedAt: time.Now()}.ToDetail()
	if err != nil || d.Result["signal"] != "buy" || d.Meta["model"] != "gpt-4" { t.Error("mismatch") }
}

func TestToDetail_EmptyResult(t *testing.T) {
	d, err := AnalysisHistoryRecord{ID: "d2", ResultJSON: "", MetaJSON: ""}.ToDetail()
	if err != nil || d.Result == nil || d.Meta == nil { t.Error("empty maps expected") }
}

// ── helpers ──

func genID() string { return generateUUID() }

// SaveFromAPIMock wraps with mock-user ID
func (r *Repository) SaveFromAPIMock(_ context.Context, symbol, symbolName string, respBytes []byte) error {
	return r.SaveFromAPResponse(context.Background(), "mock-user", symbol, symbolName, respBytes)
}

func (r *Repository) SaveFromAPResponse(_ context.Context, userID, symbol, symbolName string, respBytes []byte) error {
	return r.SaveFromAPIResponse(context.Background(), userID, symbol, symbolName, respBytes)
}
