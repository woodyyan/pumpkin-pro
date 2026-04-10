package quadrant

import (
	"context"
	"testing"
	"time"

	"github.com/woodyyan/pumpkin-pro/backend/tests/testutil"
)

func setupQuadrantTest(t *testing.T) (*Repository, func()) {
	t.Helper()
	db := testutil.InMemoryDB(t)
	testutil.AutoMigrateModels(t, db,
		&QuadrantScoreRecord{},
		&ComputeLogRecord{},
	)
	return NewRepository(db), func() {}
}

func makeQuadrantScoreRecord(code string) QuadrantScoreRecord {
	return QuadrantScoreRecord{
		Code:        code,
		Name:        "股票" + code,
		Opportunity: 0.65,
		Risk:        0.35,
		Quadrant:    "机会",
		Trend:       0.7,
		Flow:        0.6,
		Revision:    0.5,
		Volatility:  0.3,
		Drawdown:    0.15,
		Crowding:    0.4,
		ComputedAt:  time.Now().UTC(),
	}
}

// ── Repository Tests ──

func TestQuadrantRepo_BulkUpsert(t *testing.T) {
	repo, cleanup := setupQuadrantTest(t)
	defer cleanup()
	ctx := context.Background()

	records := []QuadrantScoreRecord{
		makeQuadrantScoreRecord("000001"),
		makeQuadrantScoreRecord("600036"),
		makeQuadrantScoreRecord("00700"),
	}

	err := repo.BulkUpsert(ctx, records)
	if err != nil {
		t.Fatalf("BulkUpsert failed: %v", err)
	}

	count, err := repo.Count(ctx)
	if err != nil {
		t.Fatalf("Count failed: %v", err)
	}
	if count != 3 {
		t.Errorf("expected count 3 after bulk upsert, got %d", count)
	}
}

func TestQuadrantRepo_BulkUpsert_Empty(t *testing.T) {
	repo, cleanup := setupQuadrantTest(t)
	defer cleanup()

	err := repo.BulkUpsert(context.Background(), []QuadrantScoreRecord{})
	if err != nil {
		t.Errorf("expected no error for empty batch, got %v", err)
	}
}

func TestQuadrantRepo_FindAll(t *testing.T) {
	repo, cleanup := setupQuadrantTest(t)
	defer cleanup()
	ctx := context.Background()

	for _, code := range []string{"000001", "600036", "601318"} {
		r := makeQuadrantScoreRecord(code)
		r.Quadrant = mapCodeToQuadrant(code)
		_ = repo.BulkUpsert(ctx, []QuadrantScoreRecord{r})
	}

	all, err := repo.FindAll(ctx)
	if err != nil {
		t.Fatalf("FindAll failed: %v", err)
	}
	if len(all) != 3 {
		t.Errorf("expected 3 records, got %d", len(all))
	}
}

func TestQuadrantRepo_FindBySymbols(t *testing.T) {
	repo, cleanup := setupQuadrantTest(t)
	defer cleanup()
	ctx := context.Background()

	codes := []string{"A", "B", "C", "D"}
	for _, c := range codes {
		r := makeQuadrantScoreRecord(c)
		_ = repo.BulkUpsert(ctx, []QuadrantScoreRecord{r})
	}

	found, err := repo.FindBySymbols(ctx, []string{"A", "C"})
	if err != nil {
		t.Fatalf("FindBySymbols failed: %v", err)
	}
	if len(found) != 2 {
		t.Errorf("expected 2 records for symbols [A,C], got %d", len(found))
	}

	empty, _ := repo.FindBySymbols(ctx, []string{"NONEXIST"})
	if len(empty) != 0 {
		t.Errorf("expected 0 for nonexistent symbols, got %d", len(empty))
	}

	nilResult, _ := repo.FindBySymbols(ctx, []string{})
	if len(nilResult) != 0 {
		t.Errorf("expected 0 for empty codes, got %d", len(nilResult))
	}
}

func TestQuadrantRepo_Count(t *testing.T) {
	repo, cleanup := setupQuadrantTest(t)
	defer cleanup()

	count, err := repo.Count(context.Background())
	if err != nil || count != 0 {
		t.Errorf("expected count 0 on empty DB, got (%d,%v)", count, err)
	}
}

func TestQuadrantRepo_GetLatestComputedAt(t *testing.T) {
	repo, cleanup := setupQuadrantTest(t)
	defer cleanup()
	ctx := context.Background()

	// Empty → nil
	got, err := repo.GetLatestComputedAt(ctx)
	if err != nil {
		t.Fatalf("GetLatestComputedAt failed: %v", err)
	}
	if got != nil {
		t.Error("expected nil when table is empty")
	}

	now := time.Date(2026, 4, 10, 12, 0, 0, 0, time.UTC)
	record := makeQuadrantScoreRecord("TIME_TEST")
	record.ComputedAt = now
	_ = repo.BulkUpsert(ctx, []QuadrantScoreRecord{record})

	got2, _ := repo.GetLatestComputedAt(ctx)
	if got2 == nil {
		t.Fatal("expected non-nil after insert")
	}
	if !got2.Equal(now) {
		t.Errorf("expected computed_at=%v, got %v", now, *got2)
	}
}

func TestQuadrantRepo_ComputeLogs(t *testing.T) {
	repo, cleanup := setupQuadrantTest(t)
	defer cleanup()
	ctx := context.Background()

	log := ComputeLogRecord{
		ID:          "qcl-test-01",
		ComputedAt:  time.Now().UTC(),
		Mode:        "full",
		DurationSec: float64(45.2),
		StockCount:  4800,
		ReportJSON:  `{"status":"success"}`,
		Status:      "success",
	}

	err := repo.InsertComputeLog(ctx, log)
	if err != nil {
		t.Fatalf("InsertComputeLog failed: %v", err)
	}

	logs, err := repo.ListComputeLogs(ctx, 10)
	if err != nil {
		t.Fatalf("ListComputeLogs failed: %v", err)
	}
	if len(logs) != 1 {
		t.Errorf("expected 1 log entry, got %d", len(logs))
	}
	if logs[0].Mode != "full" {
		t.Errorf("expected mode 'full', got %s", logs[0].Mode)
	}

	latest, err := repo.GetLatestComputeLog(ctx)
	if err != nil {
		t.Fatalf("GetLatestComputeLog failed: %v", err)
	}
	if latest.ID != "qcl-test-01" {
		t.Errorf("expected latest log ID qcl-test-01, got %s", latest.ID)
	}
}

// ── Service Tests ──

func TestQuadrantService_BulkSave(t *testing.T) {
	repo, cleanup := setupQuadrantTest(t)
	defer cleanup()
	s := NewService(repo)

	input := BulkSaveInput{
		Items: []BulkSaveItem{
			{Code: "S1", Name: "Stock 1", Opportunity: 0.8, Risk: 0.2, Quadrant: "机会"},
			{Code: "S2", Name: "Stock 2", Opportunity: 0.4, Risk: 0.7, Quadrant: "防御"},
			{Code: "", Name: "Empty Code", Opportunity: 0.5, Risk: 0.5, Quadrant: "机会"},
		},
		ComputedAt: "2026-04-10T08:00:00Z",
	}

	savedCount, err := s.BulkSave(context.Background(), input)
	if err != nil {
		t.Fatalf("BulkSave failed: %v", err)
	}
	if savedCount != 2 { // Empty-code item should be skipped
		t.Errorf("expected 2 saved (empty code skipped), got %d", savedCount)
	}
}

func TestQuadrantService_BulkSave_EmptyItems(t *testing.T) {
	repo, cleanup := setupQuadrantTest(t)
	defer cleanup()
	s := NewService(repo)

	input := BulkSaveInput{
		Items:      []BulkSaveItem{{Code: "", Name: "Only Empty"}},
		ComputedAt: "",
	}
	_, err := s.BulkSave(context.Background(), input)
	if err == nil {
		t.Error("expected error when all items have empty codes")
	}
}

func TestQuadrantService_GetAllWithWatchlist(t *testing.T) {
	repo, cleanup := setupQuadrantTest(t)
	defer cleanup()
	ctx := context.Background()
	s := NewService(repo)

	// Insert some data
	allCodes := []string{"W1", "W2", "W3", "O1", "O2"}
	for _, c := range allCodes {
		r := makeQuadrantScoreRecord(c)
		r.Quadrant = mapCodeToQuadrant(c)
		_ = repo.BulkUpsert(ctx, []QuadrantScoreRecord{r})
	}

	resp, err := s.GetAllWithWatchlist(ctx, []string{"W1", "O1"})
	if err != nil {
		t.Fatalf("GetAllWithWatchlist failed: %v", err)
	}

	if resp.Meta.TotalCount != 5 {
		t.Errorf("expected TotalCount 5, got %d", resp.Meta.TotalCount)
	}
	if len(resp.AllStocks) != 5 {
		t.Errorf("expected 5 all_stocks, got %d", len(resp.AllStocks))
	}
	if len(resp.WatchlistDetails) != 2 {
		t.Errorf("expected 2 watchlist details, got %d", len(resp.WatchlistDetails))
	}
}

func TestQuadrantService_GetStatus(t *testing.T) {
	repo, cleanup := setupQuadrantTest(t)
	defer cleanup()
	ctx := context.Background()
	s := NewService(repo)

	status, err := s.GetStatus(ctx)
	if err != nil {
		t.Fatalf("GetStatus failed: %v", err)
	}
	if status.StockCount != 0 {
		t.Errorf("expected StockCount 0 on empty DB, got %d", status.StockCount)
	}
	if status.LastComputedAt != "" {
		t.Errorf("expected empty LastComputedAt, got %s", status.LastComputedAt)
	}
}

func TestQuadrantService_ListComputeLogs_LimitClamp(t *testing.T) {
	repo, cleanup := setupQuadrantTest(t)
	defer cleanup()
	s := NewService(repo)

	logs, err := s.ListComputeLogs(context.Background(), -5)
	if err != nil {
		t.Fatalf("ListComputeLogs with negative limit failed: %v", err)
	}
	if logs == nil { // just ensure it doesn't panic; default limit should apply
		t.Error("ListComputeLogs should return non-nil even when empty")
	}
	_ = logs
}

// ── Model conversion tests ──

func TestQuadrantScoreRecord_ToCompact(t *testing.T) {
	r := QuadrantScoreRecord{
		Code: "TEST", Name: "TestStock", Opportunity: 0.72, Risk: 0.28, Quadrant: "机会",
	}
	c := r.ToCompact()
	if c.Code != "TEST" || c.Name != "TestStock" {
		t.Error("ToCompact field mismatch")
	}
	if c.Opportunity != 0.72 || c.Risk != 0.28 {
		t.Error("ToCompact numeric mismatch")
	}
}

func TestQuadrantScoreRecord_ToDetail(t *testing.T) {
	r := QuadrantScoreRecord{
		Code: "DETAIL", Name: "DetailStock", Opportunity: 0.6, Risk: 0.4, Quadrant: "防御",
		Trend: 0.55, Flow: 0.48, Revision: 0.33,
		Volatility: 0.22, Drawdown: 0.11, Crowding: 0.39,
	}
	d := r.ToDetail()
	if d.Code != "DETAIL" || d.Quadrant != "防御" {
		t.Error("ToDetail field mismatch")
	}
	if d.Trend != 0.55 || d.Crowding != 0.39 {
		t.Error("ToDetail sub-field mismatch")
	}
}

func TestQuadrantSummary_Accumulation(t *testing.T) {
	s := QuadrantSummary{}
	records := []QuadrantScoreRecord{
		{Quadrant: "机会"}, {Quadrant: "拥挤"},
		{Quadrant: "泡沫"}, {Quadrant: "防御"},
		{Quadrant: "未知"},
	}
	for _, r := range records {
		switch r.Quadrant {
		case "机会":
			s.OpportunityZone++
		case "拥挤":
			s.CrowdedZone++
		case "泡沫":
			s.BubbleZone++
		case "防御":
			s.DefensiveZone++
		default:
			s.NeutralZone++
		}
	}
	if s.OpportunityZone != 1 || s.CrowdedZone != 1 ||
		s.BubbleZone != 1 || s.DefensiveZone != 1 || s.NeutralZone != 1 {
		t.Errorf("quadrant summary mismatch: %+v", s)
	}
}

// ── Helpers ──

func mapCodeToQuadrant(code string) string {
	// Deterministic mapping based on code for test consistency
	switch code[0] {
	case 'W':
		return "机会"
	case 'O':
		return "拥挤"
	case 'A', 'B', 'C', 'D':
		return "泡沫"
	default:
		return "防御"
	}
}
