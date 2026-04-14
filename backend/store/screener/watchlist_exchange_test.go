package screener

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/google/uuid"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// ── Test Helpers ───────────────────────────────────────────

func setupTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to open test db: %v", err)
	}
	if err := db.AutoMigrate(&WatchlistRecord{}, &WatchlistStockRecord{}); err != nil {
		t.Fatalf("failed to migrate: %v", err)
	}
	return db
}

func newTestRepo(t *testing.T) (*Repository, func()) {
	t.Helper()
	db := setupTestDB(t)
	repo := NewRepository(db)
	return repo, func() {
		sqlDB, _ := db.DB()
		sqlDB.Close()
	}
}

func newTestService(t *testing.T) (*Service, func()) {
	t.Helper()
	repo, cleanup := newTestRepo(t)
	return NewService(repo), cleanup
}

// ── T3: Model Tests ────────────────────────────────────────

func TestModel_WatchlistRecord_HasExchangeField(t *testing.T) {
	rec := WatchlistRecord{
		ID:        uuid.New().String(),
		UserID:    "user-1",
		Name:      "测试自选",
		Exchange:  "HKEX",
	}

	if rec.Exchange != "HKEX" {
		t.Errorf("Exchange field = %q, want HKEX", rec.Exchange)
	}
}

func TestModel_WatchlistRecord_DefaultExchange(t *testing.T) {
	rec := WatchlistRecord{
		ID:       uuid.New().String(),
		UserID:   "user-1",
		Name:     "默认A股",
		Exchange: "", // 模拟未设置
	}

	// 空值在 GORM 层面会被设为默认值 ASHARE，这里仅验证字段存在
	if len(rec.Exchange) == 0 {
		// 空字符串是合法的（service 层会补全为 ASHARE）
		t.Log("Empty exchange is acceptable; service layer normalizes to ASHARE")
	}
}

func TestModel_ToListItem_IncludesExchange(t *testing.T) {
	rec := WatchlistRecord{
		ID:        uuid.New().String(),
		UserID:    "user-1",
		Name:      "港股组合",
		Exchange:  "HKEX",
	}
	item := rec.toListItem(3)

	if item.Exchange != "HKEX" {
		t.Errorf("toListItem Exchange = %q, want HKEX", item.Exchange)
	}
	if item.StockCount != 3 {
		t.Errorf("toListItem StockCount = %d, want 3", item.StockCount)
	}
}

func TestModel_ToDetail_IncludesExchange(t *testing.T) {
	rec := WatchlistRecord{
		ID:        uuid.New().String(),
		UserID:    "user-1",
		Name:      "港股详情",
		Exchange:  "HKEX",
	}
	stocks := []WatchlistStockRecord{
		{Code: "00700", Name: "腾讯"},
		{Code: "09988", Name: "阿里巴巴"},
	}
	detail := rec.toDetail(stocks)

	if detail.Exchange != "HKEX" {
		t.Errorf("toDetail Exchange = %q, want HKEX", detail.Exchange)
	}
	if len(detail.Stocks) != 2 {
		t.Errorf("toDetail Stocks count = %d, want 2", len(detail.Stocks))
	}
}

func TestModel_CreateWatchlistInput_HasExchangeField(t *testing.T) {
	input := CreateWatchlistInput{
		Name:     "我的港股",
		Exchange: "HKEX",
		Stocks: []WatchlistStock{
			{Code: "00700", Name: "腾讯"},
		},
	}

	if input.Exchange != "HKEX" {
		t.Errorf("CreateWatchlistInput.Exchange = %q, want HKEX", input.Exchange)
	}
}

func TestModel_JSONRoundTrip(t *testing.T) {
	wl := Watchlist{
		ID:         "test-id",
		Name:       "测试",
		Exchange:   "HKEX",
		StockCount: 5,
		CreatedAt:  "2026-01-01T00:00:00Z",
		UpdatedAt:  "2026-01-02T00:00:00Z",
	}

	data, err := json.Marshal(wl)
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}

	var decoded map[string]any
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	if decoded["exchange"] != "HKEX" {
		t.Errorf("JSON round-trip exchange = %v, want HKEX", decoded["exchange"])
	}
}

// ── T4: Repository Tests ───────────────────────────────────

func TestRepo_CreateAndRetrieveWithExchange(t *testing.T) {
	repo, cleanup := newTestRepo(t)
	defer cleanup()

	ctx := context.Background()
	wlID := uuid.New().String()
	userID := "u1"

	wl := WatchlistRecord{
		ID:        wlID,
		UserID:    userID,
		Name:      "港股自选",
		Exchange:  "HKEX",
	}

	stocks := []WatchlistStockRecord{
		{ID: uuid.New().String(), WatchlistID: wlID, Code: "00700", Name: "腾讯"},
	}

	err := repo.Create(ctx, wl, stocks)
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	// Retrieve by ID
	gotWl, gotStocks, err := repo.GetByID(ctx, userID, wlID)
	if err != nil {
		t.Fatalf("GetByID failed: %v", err)
	}

	if gotWl.Exchange != "HKEX" {
		t.Errorf("got Exchange = %q, want HKEX", gotWl.Exchange)
	}
	if len(gotStocks) != 1 || gotStocks[0].Code != "00700" {
		t.Errorf("unexpected stocks: %+v", gotStocks)
	}
}

func TestRepo_ListWithExchange(t *testing.T) {
	repo, cleanup := newTestRepo(t)
	defer cleanup()

	ctx := context.Background()
	userID := "u-list"

	// Create A股 watchlist
	repo.Create(ctx, WatchlistRecord{ID: uuid.New().String(), UserID: userID, Name: "A股", Exchange: "ASHARE"}, nil)
	// Create 港股 watchlist
	hkID := uuid.New().String()
	repo.Create(ctx, WatchlistRecord{ID: hkID, UserID: userID, Name: "港股", Exchange: "HKEX"}, nil)

	items, err := repo.List(ctx, userID)
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}

	if len(items) != 2 {
		t.Fatalf("List returned %d items, want 2", len(items))
	}

	var hasASHARE, hasHKEX bool
	for _, it := range items {
		if it.Exchange == "ASHARE" {
			hasASHARE = true
		}
		if it.Exchange == "HKEX" {
			hasHKEX = true
		}
	}
	if !hasASHARE {
		t.Error("Expected at least one ASHARE watchlist in list")
	}
	if !hasHKEX {
		t.Error("Expected at least one HKEX watchlist in list")
	}
}

func TestRepo_DeleteWithExchange(t *testing.T) {
	repo, cleanup := newTestRepo(t)
	defer cleanup()

	ctx := context.Background()
	userID := "u-del"
	wlID := uuid.New().String()

	repo.Create(ctx, WatchlistRecord{ID: wlID, UserID: userID, Name: "删除测试", Exchange: "HKEX"}, nil)

	err := repo.Delete(ctx, userID, wlID)
	if err != nil {
		t.Fatalf("Delete failed: %v", err)
	}

	// Verify deleted
	_, _, err = repo.GetByID(ctx, userID, wlID)
	if err != ErrNotFound {
		t.Errorf("after delete, expected ErrNotFound, got: %v", err)
	}
}

func TestRepo_CountByUser(t *testing.T) {
	repo, cleanup := newTestRepo(t)
	defer cleanup()

	ctx := context.Background()
	userID := "u-count"

	count, _ := repo.CountByUser(ctx, userID)
	if count != 0 {
		t.Errorf("initial count = %d, want 0", count)
	}

	repo.Create(ctx, WatchlistRecord{ID: uuid.New().String(), UserID: userID, Name: "w1", Exchange: "ASHARE"}, nil)
	repo.Create(ctx, WatchlistRecord{ID: uuid.New().String(), UserID: userID, Name: "w2", Exchange: "HKEX"}, nil)

	count, _ = repo.CountByUser(ctx, userID)
	if count != 2 {
		t.Errorf("count after 2 creates = %d, want 2", count)
	}
}

// ── T5: Service Tests ──────────────────────────────────────

func TestService_Create_AShareDefault(t *testing.T) {
	svc, cleanup := newTestService(t)
	defer cleanup()

	ctx := context.Background()
	input := CreateWatchlistInput{
		Name: "默认A股自选",
		Stocks: []WatchlistStock{
			{Code: "000001", Name: "平安银行"},
		},
		// Exchange not set → should default to ASHARE
	}

	detail, err := svc.Create(ctx, "user-1", input)
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	if detail.Exchange != "ASHARE" {
		t.Errorf("default exchange = %q, want ASHARE", detail.Exchange)
	}
}

func TestService_Create_HKEXExplicit(t *testing.T) {
	svc, cleanup := newTestService(t)
	defer cleanup()

	ctx := context.Background()
	input := CreateWatchlistInput{
		Name:     "港股组合",
		Exchange: "HKEX",
		Stocks: []WatchlistStock{
			{Code: "00700", Name: "腾讯"},
			{Code: "09988", Name: "阿里"},
		},
	}

	detail, err := svc.Create(ctx, "user-hk", input)
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	if detail.Exchange != "HKEX" {
		t.Errorf("HKEX exchange = %q, want HKEX", detail.Exchange)
	}
	if len(detail.Stocks) != 2 {
		t.Errorf("stock count = %d, want 2", len(detail.Stocks))
	}
}

func TestService_Create_CaseInsensitiveExchange(t *testing.T) {
	svc, cleanup := newTestService(t)
	defer cleanup()

	ctx := context.Background()
	input := CreateWatchlistInput{
		Name:     "小写hkex",
		Exchange: "hkex", // lowercase
		Stocks: []WatchlistStock{
			{Code: "00700", Name: "腾讯"},
		},
	}

	detail, err := svc.Create(ctx, "user-case", input)
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	// Should be normalized to uppercase
	if detail.Exchange != "HKEX" {
		t.Errorf("normalized exchange = %q, want HKEX", detail.Exchange)
	}
}

func TestService_Create_InvalidExchange(t *testing.T) {
	svc, cleanup := newTestService(t)
	defer cleanup()

	ctx := context.Background()
	input := CreateWatchlistInput{
		Name:     "非法市场",
		Exchange: "NYSE",
		Stocks: []WatchlistStock{
			{Code: "AAPL", Name: "Apple"},
		},
	}

	_, err := svc.Create(ctx, "user-bad", input)
	if err == nil {
		t.Fatal("expected error for invalid exchange NYSE")
	}
	if !strings.Contains(err.Error(), "无效的市场类型") {
		t.Errorf("error should mention invalid market type, got: %v", err)
	}
}

func TestService_Create_EmptyName(t *testing.T) {
	svc, cleanup := newTestService(t)
	defer cleanup()

	input := CreateWatchlistInput{
		Name:     "",
		Exchange: "HKEX",
		Stocks: []WatchlistStock{{Code: "00700", Name: "腾讯"}},
	}

	_, err := svc.Create(context.Background(), "u1", input)
	if err == nil {
		t.Fatal("expected error for empty name")
	}
}

func TestService_Create_EmptyStocks(t *testing.T) {
	svc, cleanup := newTestService(t)
	defer cleanup()

	input := CreateWatchlistInput{
		Name:     "空股票列表",
		Exchange: "HKEX",
		Stocks:   []WatchlistStock{},
	}

	_, err := svc.Create(context.Background(), "u1", input)
	if err == nil {
		t.Fatal("expected error for empty stocks")
	}
}

func TestService_GetByID_ReturnsExchange(t *testing.T) {
	svc, cleanup := newTestService(t)
	defer cleanup()

	ctx := context.Background()
	input := CreateWatchlistInput{
		Name:     "查询测试",
		Exchange: "HKEX",
		Stocks: []WatchlistStock{{Code: "00700", Name: "腾讯"}},
	}

	created, err := svc.Create(ctx, "user-get", input)
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	got, err := svc.GetByID(ctx, "user-get", created.ID)
	if err != nil {
		t.Fatalf("GetByID failed: %v", err)
	}

	if got.Exchange != "HKEX" {
		t.Errorf("GetByID exchange = %q, want HKEX", got.Exchange)
	}
}

func TestService_List_MixedExchanges(t *testing.T) {
	svc, cleanup := newTestService(t)
	defer cleanup()

	ctx := context.Background()
	userID := "user-mix"

	svc.Create(ctx, userID, CreateWatchlistInput{Name: "A1", Exchange: "ASHARE",
		Stocks: []WatchlistStock{{Code: "000001", Name: "平安"}}})
	svc.Create(ctx, userID, CreateWatchlistInput{Name: "H1", Exchange: "HKEX",
		Stocks: []WatchlistStock{{Code: "00700", Name: "腾讯"}}})
	svc.Create(ctx, userID, CreateWatchlistInput{Name: "A2", Exchange: "ASHARE",
		Stocks: []WatchlistStock{{Code: "600036", Name: "招商"}}})

	items, err := svc.List(ctx, userID)
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}

	if len(items) != 3 {
		t.Fatalf("expected 3 watchlists, got %d", len(items))
	}

	ashareCount, hkexCount := 0, 0
	for _, it := range items {
		switch it.Exchange {
		case "ASHARE":
			ashareCount++
		case "HKEX":
			hkexCount++
		default:
			t.Errorf("unexpected exchange: %q", it.Exchange)
		}
	}
	if ashareCount != 2 {
		t.Errorf("expected 2 ASHARE, got %d", ashareCount)
	}
	if hkexCount != 1 {
		t.Errorf("expected 1 HKEX, got %d", hkexCount)
	}
}

func TestService_Delete_WatchlistWithExchange(t *testing.T) {
	svc, cleanup := newTestService(t)
	defer cleanup()

	ctx := context.Background()
	userID := "user-del-s"

	created, err := svc.Create(ctx, userID, CreateWatchlistInput{
		Name: "待删除", Exchange: "HKEX", Stocks: []WatchlistStock{{Code: "00700", Name: "腾讯"}},
	})
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	err = svc.Delete(ctx, userID, created.ID)
	if err != nil {
		t.Fatalf("Delete failed: %v", err)
	}

	items, err := svc.List(ctx, userID)
	if err != nil {
		t.Fatalf("List after delete failed: %v", err)
	}
	if len(items) != 0 {
		t.Errorf("expected 0 items after delete, got %d", len(items))
	}
}
