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
		&RankingSnapshot{},
		&RankingPortfolioDefinition{},
		&RankingPortfolioSnapshot{},
		&RankingPortfolioSnapshotConstituent{},
		&RankingPortfolioMarketPrice{},
		&RankingPortfolioBenchmarkPrice{},
		&RankingPortfolioResult{},
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

func TestQuadrantService_BulkSave_NormalizesChineseNameWhitespace(t *testing.T) {
	repo, cleanup := setupQuadrantTest(t)
	defer cleanup()
	s := NewService(repo)

	input := BulkSaveInput{
		Items: []BulkSaveItem{
			{Code: "000858", Name: "五 粮 液", Exchange: "SZSE", Opportunity: 0.8, Risk: 0.2, Quadrant: "机会"},
			{Code: "000002", Name: "万  科Ａ", Exchange: "SZSE", Opportunity: 0.6, Risk: 0.4, Quadrant: "机会"},
			{Code: "000025", Name: "CHEVALIER INT'L", Exchange: "HKEX", Opportunity: 0.5, Risk: 0.5, Quadrant: "中性"},
		},
		ComputedAt: "2026-05-08T08:00:00Z",
	}

	_, err := s.BulkSave(context.Background(), input)
	if err != nil {
		t.Fatalf("BulkSave failed: %v", err)
	}

	records, err := repo.FindBySymbols(context.Background(), []string{"000858", "000002", "000025"})
	if err != nil {
		t.Fatalf("FindBySymbols failed: %v", err)
	}
	byCode := map[string]QuadrantScoreRecord{}
	for _, record := range records {
		byCode[record.Code] = record
	}
	if byCode["000858"].Name != "五粮液" {
		t.Fatalf("expected 五粮液, got %q", byCode["000858"].Name)
	}
	if byCode["000002"].Name != "万科Ａ" {
		t.Fatalf("expected 万科Ａ, got %q", byCode["000002"].Name)
	}
	if byCode["000025"].Name != "CHEVALIER INT'L" {
		t.Fatalf("expected English spacing preserved, got %q", byCode["000025"].Name)
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

	resp, err := s.GetAllWithWatchlist(ctx, nil, []string{"W1", "O1"})
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

// ── Full-path logging tests ──

func TestBulkSave_LogWrittenOnSuccess(t *testing.T) {
	repo, cleanup := setupQuadrantTest(t)
	defer cleanup()
	s := NewService(repo)
	ctx := context.Background()

	input := BulkSaveInput{
		Items: []BulkSaveItem{
			{Code: "000001", Name: "平安银行", Exchange: "SZSE", Opportunity: 65, Risk: 35, Quadrant: "机会"},
			{Code: "600036", Name: "招商银行", Exchange: "SSE", Opportunity: 55, Risk: 45, Quadrant: "中性"},
		},
		ComputedAt: "2026-04-16T10:00:00Z",
	}

	count, err := s.BulkSave(ctx, input)
	if err != nil {
		t.Fatalf("BulkSave failed: %v", err)
	}
	if count != 2 {
		t.Fatalf("expected 2 saved, got %d", count)
	}

	// Verify compute log was written
	logs, lErr := repo.ListComputeLogs(ctx, 10)
	if lErr != nil {
		t.Fatalf("ListComputeLogs failed: %v", lErr)
	}
	if len(logs) == 0 {
		t.Fatal("expected at least 1 compute log after successful BulkSave")
	}
	log := logs[0]
	if log.Status != "success" {
		t.Errorf("expected log status=success, got %s", log.Status)
	}
	if log.TotalCount != 2 {
		t.Errorf("expected log TotalCount=2, got %d", log.TotalCount)
	}
	if log.Exchange != "SZSE" { // first item's exchange
		t.Errorf("expected log Exchange=SZSE, got %s", log.Exchange)
	}
}

func TestBulkSave_LogWrittenOnEmptyItems(t *testing.T) {
	repo, cleanup := setupQuadrantTest(t)
	defer cleanup()
	s := NewService(repo)
	ctx := context.Background()

	input := BulkSaveInput{
		Items:      []BulkSaveItem{{Code: "", Name: "Empty"}},
		ComputedAt: "",
	}

	_, err := s.BulkSave(ctx, input)
	if err == nil {
		t.Fatal("expected error for empty items")
	}

	logs, _ := repo.ListComputeLogs(ctx, 10)
	if len(logs) == 0 {
		t.Fatal("expected failed log written even when items are empty")
	}
	log := logs[0]
	if log.Status != "failed" {
		t.Errorf("expected status=failed, got %s", log.Status)
	}
	if log.ErrorMsg == "" {
		t.Error("expected non-empty ErrorMsg for empty-items failure")
	}
}

func TestBulkSave_ReportFieldsParsed(t *testing.T) {
	repo, cleanup := setupQuadrantTest(t)
	defer cleanup()
	s := NewService(repo)
	ctx := context.Background()

	report := map[string]any{
		"mode":             "全量",
		"exchange":         "ASHARE",
		"duration_seconds": 123.5,
		"stock_count":      float64(3),
		"daily_bars":       map[string]any{"success": float64(500), "failed": float64(12)},
		"status":           "success",
		"quadrant_counts":  map[string]any{"机会": 100, "拥挤": 80},
	}

	input := BulkSaveInput{
		Items: []BulkSaveItem{
			{Code: "A1", Exchange: "SSE", Opportunity: 70, Risk: 30, Quadrant: "机会"},
			{Code: "A2", Exchange: "SSE", Opportunity: 75, Risk: 25, Quadrant: "机会"},
			{Code: "A3", Exchange: "SZSE", Opportunity: 40, Risk: 60, Quadrant: "防御"},
		},
		ComputedAt: "2026-04-16T11:00:00Z",
		Report:     report,
	}

	count, err := s.BulkSave(ctx, input)
	if err != nil {
		t.Fatalf("BulkSave failed: %v", err)
	}
	if count != 3 {
		t.Fatalf("expected 3 saved, got %d", count)
	}

	logs, _ := repo.ListComputeLogs(ctx, 10)
	if len(logs) == 0 {
		t.Fatal("no logs found")
	}
	log := logs[0]
	if log.Mode != "全量" {
		t.Errorf("expected Mode=全量, got %s", log.Mode)
	}
	if log.DurationSec != 123.5 {
		t.Errorf("expected DurationSec=123.5, got %.1f", log.DurationSec)
	}
	if log.SuccessCount != 3 {
		t.Errorf("expected SuccessCount=3 (from stock_count in report), got %d", log.SuccessCount)
	}
	if log.FailedCount != 12 {
		t.Errorf("expected FailedCount=12 (from daily_bars.failed), got %d", log.FailedCount)
	}
}

func TestBulkSave_HKEXProgressTerminalSet(t *testing.T) {
	repo, cleanup := setupQuadrantTest(t)
	defer cleanup()
	s := NewService(repo)

	// Set running state first so we can verify terminal transition
	UpdateProgress("HKEX", ComputeProgress{
		Exchange: "HKEX", Status: "running", Current: 50, Total: 100,
	})

	input := BulkSaveInput{
		Items: []BulkSaveItem{
			{Code: "00700", Name: "腾讯", Exchange: "HKEX", Opportunity: 72, Risk: 28, Quadrant: "机会"},
			{Code: "09988", Name: "阿里", Exchange: "HKEX", Opportunity: 60, Risk: 40, Quadrant: "中性"},
		},
		ComputedAt: "2026-04-16T12:00:00Z",
	}

	_, err := s.BulkSave(context.Background(), input)
	if err != nil {
		t.Fatalf("BulkSave failed: %v", err)
	}

	// Verify progress was set to terminal (success clamps Current=Total from prior state)
	p := GetProgress()["HKEX"]
	if p.Status != "success" {
		t.Errorf("expected HKEX progress=status=success after BulkSave, got %s", p.Status)
	}
	if p.Current != p.Total || p.Total != 100 {
		t.Errorf("expected HKEX progress clamped to prior Total (100/100), got (%d/%d)", p.Current, p.Total)
	}
}

func TestBulkSave_ProgressTerminalOnError(t *testing.T) {
	// This test verifies that when BulkSave fails at DB level,
	// the progress is still set to terminal "failed".
	repo, cleanup := setupQuadrantTest(t)
	defer cleanup()
	s := NewService(repo)

	// We can't easily force a DB error with sqlite :memory:, but we can test
	// the empty-items path which also calls SetProgressTerminal with "failed"
	input := BulkSaveInput{
		Items:      []BulkSaveItem{{Code: "", Name: "EmptyOnly"}},
		ComputedAt: "",
	}

	_, _ = s.BulkSave(context.Background(), input) // expected to return error

	p := GetProgress()["SZSE"] // default fallback for unknown exchange
	if p.Status == "running" {
		t.Error("progress should NOT remain running after BulkSave failure")
	}
}

func TestDetectExchange_Fallback(t *testing.T) {
	tests := []struct {
		name     string
		items    []BulkSaveItem
		expected string
	}{
		{"empty items", []BulkSaveItem{}, "SZSE"},
		{"no exchange field", []BulkSaveItem{{Code: "A"}}, "SZSE"},
		{"HKEX items", []BulkSaveItem{{Code: "00700", Exchange: "HKEX"}}, "HKEX"},
		{"mixed, first wins", []BulkSaveItem{
			{Code: "000001", Exchange: "SZSE"}, {Code: "00700", Exchange: "HKEX"},
		}, "SZSE"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := detectExchange(tt.items)
			if got != tt.expected {
				t.Errorf("detectExchange() = %s, want %s", got, tt.expected)
			}
		})
	}
}

func TestAllEmpty_VariousCases(t *testing.T) {
	if !allEmpty([]BulkSaveItem{{Code: ""}, {Code: "  "}}) {
		t.Error("expected allEmpty=true for whitespace-only codes")
	}
	if allEmpty([]BulkSaveItem{{Code: "X"}}) {
		t.Error("expected allEmpty=false for non-empty code")
	}
	if !allEmpty([]BulkSaveItem{}) {
		t.Error("expected allEmpty=true for empty slice")
	}
}
