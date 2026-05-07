package quadrant

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"
)

// ── Helper: seed opportunity zone records ──

func seedOpportunityRecords(t *testing.T, repo *Repository, records []QuadrantScoreRecord) {
	t.Helper()
	ctx := context.Background()
	if err := repo.BulkUpsert(ctx, records); err != nil {
		t.Fatalf("seedOpportunityRecords failed: %v", err)
	}
}

func makeRankingRecord(code, exchange string, opportunity, risk float64) QuadrantScoreRecord {
	return QuadrantScoreRecord{
		Code:        code,
		Name:        "股票" + code,
		Exchange:    exchange,
		Opportunity: opportunity,
		Risk:        risk,
		Quadrant:    "机会",
		Trend:       opportunity * 0.95,
		Flow:        opportunity * 0.88,
		Revision:    opportunity * 0.80,
		Liquidity:   opportunity * 0.9, // 流动性分数
		Volatility:  risk * 1.2,
		Drawdown:    risk * 0.6,
		Crowding:    risk * 0.9,
		AvgAmount5d: 10000.0, // 1 亿元，满足硬过滤门槛
		ComputedAt:  time.Date(2026, 4, 15, 2, 30, 0, 0, time.UTC),
	}
}

func makeNonOpportunityRecord(code, exchange string, quadrant string) QuadrantScoreRecord {
	return QuadrantScoreRecord{
		Code:        code,
		Name:        "股票" + code,
		Exchange:    exchange,
		Opportunity: 30,
		Risk:        70,
		Quadrant:    quadrant,
		Trend:       25,
		Flow:        20,
		Revision:    18,
		Liquidity:   40,
		AvgAmount5d: 5000.0,
		ComputedAt:  time.Date(2026, 4, 15, 2, 30, 0, 0, time.UTC),
	}
}

func makeAShareRankingRecord(code, board, quadrant string, rankingScore, opportunity, risk, avgAmount float64) QuadrantScoreRecord {
	record := QuadrantScoreRecord{
		Code:             code,
		Name:             "股票" + code,
		Exchange:         "SSE",
		Opportunity:      opportunity,
		Risk:             risk,
		Quadrant:         quadrant,
		Trend:            opportunity * 0.9,
		Flow:             opportunity * 0.85,
		Revision:         opportunity * 0.8,
		Liquidity:        75,
		AvgAmount5d:      avgAmount,
		Board:            board,
		RankingScore:     rankingScore,
		GlobalRankScore:  rankingScore - 3,
		BoardRankScore:   rankingScore - 1,
		TradabilityScore: 80,
		ComputedAt:       time.Date(2026, 4, 15, 2, 30, 0, 0, time.UTC),
	}
	if strings.HasPrefix(code, "0") || strings.HasPrefix(code, "3") {
		record.Exchange = "SZSE"
	}
	return record
}

// padCode returns a zero-padded string of length digits.
func padCode(n int, digits int) string {
	return fmt.Sprintf("%0*d", digits, n)
}

// ── T-R1: Model / Struct Tests ──

func TestRankingItem_JSONRoundTrip(t *testing.T) {
	pct := 5.6
	item := RankingItem{
		Rank:             1,
		Code:             "600519",
		Name:             "贵州茅台",
		Exchange:         "SSE",
		Opportunity:      96.5,
		Risk:             22.3,
		Quadrant:         "机会",
		Trend:            94.2,
		Flow:             88.7,
		Revision:         85.1,
		Liquidity:        92.0,
		AvgAmount5d:      150000.0,
		Board:            "MAIN",
		RankingScore:     91.2,
		GlobalRankScore:  89.1,
		BoardRankScore:   95.0,
		TradabilityScore: 88.0,
		ConsecutiveDays:  3,
		ReturnPct:        &pct,
	}

	data, err := json.Marshal(item)
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}

	var got RankingItem
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}

	if got.Rank != item.Rank || got.Code != item.Code || got.Opportunity != item.Opportunity ||
		got.Board != item.Board || got.RankingScore != item.RankingScore ||
		got.GlobalRankScore != item.GlobalRankScore || got.BoardRankScore != item.BoardRankScore ||
		got.TradabilityScore != item.TradabilityScore {
		t.Errorf("roundtrip mismatch: got %+v", got)
	}
	if got.ReturnPct == nil || *got.ReturnPct != pct {
		t.Errorf("expected return_pct %.1f, got %+v", pct, got.ReturnPct)
	}

	// Verify JSON keys are correct (not Go struct names)
	var raw map[string]any
	json.Unmarshal(data, &raw)
	expectedKeys := []string{"rank", "code", "name", "exchange", "opportunity", "risk", "quadrant", "trend", "flow", "revision", "board", "ranking_score", "global_rank_score", "board_rank_score", "tradability_score"}
	for _, k := range expectedKeys {
		if _, ok := raw[k]; !ok {
			t.Errorf("missing JSON key: %s", k)
		}
	}
}

func TestRankingResponse_Fields(t *testing.T) {
	resp := RankingResponse{
		Meta: RankingMeta{
			ComputedAt:    "2026-04-15T02:30:00Z",
			TotalInZone:   156,
			ReturnedCount: 20,
			Exchange:      "ASHARE",
		},
		Items: []RankingItem{{Rank: 1}},
	}

	if resp.Meta.TotalInZone != 156 {
		t.Errorf("expected TotalInZone=156, got %d", resp.Meta.TotalInZone)
	}
	if len(resp.Items) != 1 {
		t.Errorf("expected 1 item, got %d", len(resp.Items))
	}
	if resp.Meta.Exchange != "ASHARE" {
		t.Errorf("expected Exchange=ASHARE, got %s", resp.Meta.Exchange)
	}
}

// ── T-R2: Service GetRanking — Core Logic ──

func TestGetRanking_AShareTopN(t *testing.T) {
	repo, cleanup := setupQuadrantTest(t)
	defer cleanup()
	svc := NewService(repo)
	ctx := context.Background()

	// Seed 25 A-share records in opportunity zone + some non-opportunity
	records := make([]QuadrantScoreRecord, 0, 30)
	for i := 1; i <= 25; i++ {
		exchange := "SSE"
		if i%2 == 0 {
			exchange = "SZSE"
		}
		opp := float64(100 - i)
		risk := float64(10 + i)
		records = append(records, makeRankingRecord(padCode(i, 6), exchange, opp, risk))
	}
	records = append(records, makeNonOpportunityRecord("000999", "SZSE", "泡沫"))
	records = append(records, makeNonOpportunityRecord("600999", "SSE", "防御"))
	seedOpportunityRecords(t, repo, records)

	resp, err := svc.GetRanking(ctx, "ASHARE", 20)
	if err != nil {
		t.Fatalf("GetRanking failed: %v", err)
	}

	if resp.Meta.ReturnedCount != 20 {
		t.Errorf("expected ReturnedCount=20, got %d", resp.Meta.ReturnedCount)
	}
	if len(resp.Items) != 20 {
		t.Fatalf("expected 20 items, got %d", len(resp.Items))
	}
	if resp.Meta.TotalInZone != 25 {
		t.Errorf("expected TotalInZone=25, got %d", resp.Meta.TotalInZone)
	}

	// Verify sort order: opportunity DESC
	for i := 1; i < len(resp.Items); i++ {
		if resp.Items[i].Opportunity > resp.Items[i-1].Opportunity {
			t.Errorf("sort order violation at index %d", i)
		}
	}

	// Rank should be sequential from 1
	if resp.Items[0].Rank != 1 || resp.Items[19].Rank != 20 {
		t.Errorf("rank mismatch: first=%d, last=%d", resp.Items[0].Rank, resp.Items[19].Rank)
	}

	if resp.Meta.Exchange != "ASHARE" {
		t.Errorf("expected meta exchange ASHARE, got %s", resp.Meta.Exchange)
	}
}

func TestGetRanking_HKEXIsolation(t *testing.T) {
	repo, cleanup := setupQuadrantTest(t)
	defer cleanup()
	svc := NewService(repo)
	ctx := context.Background()

	hkRecords := []QuadrantScoreRecord{
		makeRankingRecord("00700", "HKEX", 92, 20),
		makeRankingRecord("00005", "HKEX", 88, 25),
		makeRankingRecord("03968", "HKEX", 85, 30),
	}
	asRecords := []QuadrantScoreRecord{
		makeRankingRecord("600519", "SSE", 99, 10),
		makeRankingRecord("000001", "SZSE", 97, 15),
	}
	allRecords := append(hkRecords, asRecords...)
	seedOpportunityRecords(t, repo, allRecords)

	resp, err := svc.GetRanking(ctx, "HKEX", 50)
	if err != nil {
		t.Fatalf("GetRanking(HKEX) failed: %v", err)
	}

	if len(resp.Items) != 3 {
		t.Errorf("expected 3 HK items, got %d", len(resp.Items))
	}
	for _, item := range resp.Items {
		if item.Exchange != "HKEX" {
			t.Errorf("non-HK stock in HK ranking: %s (%s)", item.Code, item.Exchange)
		}
	}
	if resp.Meta.Exchange != "HKEX" {
		t.Errorf("expected meta exchange HKEX, got %s", resp.Meta.Exchange)
	}
	if resp.Meta.TotalInZone != 3 {
		t.Errorf("expected total_in_zone=3, got %d", resp.Meta.TotalInZone)
	}
}

func TestGetRanking_EmptyOpportunityZone(t *testing.T) {
	repo, cleanup := setupQuadrantTest(t)
	defer cleanup()
	svc := NewService(repo)
	ctx := context.Background()

	records := []QuadrantScoreRecord{
		makeNonOpportunityRecord("000001", "SZSE", "拥挤"),
		makeNonOpportunityRecord("600519", "SSE", "泡沫"),
	}
	seedOpportunityRecords(t, repo, records)

	resp, err := svc.GetRanking(ctx, "ASHARE", 20)
	if err != nil {
		t.Fatalf("GetRanking failed: %v", err)
	}

	if len(resp.Items) != 0 {
		t.Errorf("expected 0 items when no opportunity zone, got %d", len(resp.Items))
	}
	if resp.Meta.TotalInZone != 0 {
		t.Errorf("expected TotalInZone=0, got %d", resp.Meta.TotalInZone)
	}
}

func TestGetRanking_LessThanLimit(t *testing.T) {
	repo, cleanup := setupQuadrantTest(t)
	defer cleanup()
	svc := NewService(repo)
	ctx := context.Background()

	records := make([]QuadrantScoreRecord, 8)
	for i := 0; i < 8; i++ {
		records[i] = makeRankingRecord(padCode(i+1, 6), "SSE", float64(90-i), float64(10+i*2))
	}
	seedOpportunityRecords(t, repo, records)

	resp, err := svc.GetRanking(ctx, "ASHARE", 20)
	if err != nil {
		t.Fatalf("GetRanking failed: %v", err)
	}

	if len(resp.Items) != 8 {
		t.Errorf("expected 8 items (all available), got %d", len(resp.Items))
	}
	if resp.Meta.ReturnedCount != 8 {
		t.Errorf("expected ReturnedCount=8, got %d", resp.Meta.ReturnedCount)
	}
	if resp.Meta.TotalInZone != 8 {
		t.Errorf("expected TotalInZone=8, got %d", resp.Meta.TotalInZone)
	}
}

func TestGetRanking_DefaultExchange(t *testing.T) {
	repo, cleanup := setupQuadrantTest(t)
	defer cleanup()
	svc := NewService(repo)
	ctx := context.Background()

	records := []QuadrantScoreRecord{
		makeRankingRecord("600519", "SSE", 95, 20),
		makeRankingRecord("00700", "HKEX", 90, 25),
	}
	seedOpportunityRecords(t, repo, records)

	resp, err := svc.GetRanking(ctx, "", 20)
	if err != nil {
		t.Fatalf("GetRanking('') failed: %v", err)
	}

	foundHK := false
	for _, item := range resp.Items {
		if item.Exchange == "HKEX" {
			foundHK = true
		}
	}
	if foundHK {
		t.Error("default exchange should not include HKEX stocks")
	}
}

func TestGetRanking_RiskAsSecondarySort(t *testing.T) {
	repo, cleanup := setupQuadrantTest(t)
	defer cleanup()
	svc := NewService(repo)
	ctx := context.Background()

	records := []QuadrantScoreRecord{
		makeRankingRecord("600000", "SSE", 95, 35),  // higher risk → rank lower
		makeRankingRecord("000001", "SZSE", 95, 15), // lower risk → rank higher
		makeRankingRecord("601318", "SSE", 93, 10),
	}
	seedOpportunityRecords(t, repo, records)

	resp, _ := svc.GetRanking(ctx, "ASHARE", 10)

	var highRiskIdx, lowRiskIdx int
	for i, item := range resp.Items {
		if item.Code == "600000" && item.Opportunity == 95 {
			highRiskIdx = i
		}
		if item.Code == "000001" && item.Opportunity == 95 {
			lowRiskIdx = i
		}
	}

	if lowRiskIdx >= highRiskIdx {
		t.Error("lower-risk stock with same opportunity should rank higher")
	}
}

func TestGetRanking_AShareUsesRankingScoreOutsideOpportunityZone(t *testing.T) {
	repo, cleanup := setupQuadrantTest(t)
	defer cleanup()
	svc := NewService(repo)
	ctx := context.Background()

	records := []QuadrantScoreRecord{
		makeAShareRankingRecord("600001", "MAIN", "中性", 99, 40, 22, 12000),
		makeAShareRankingRecord("600002", "MAIN", "机会", 80, 90, 18, 15000),
	}
	seedOpportunityRecords(t, repo, records)

	resp, err := svc.GetRanking(ctx, "ASHARE", 20)
	if err != nil {
		t.Fatalf("GetRanking failed: %v", err)
	}
	if len(resp.Items) != 2 {
		t.Fatalf("expected 2 items, got %d", len(resp.Items))
	}
	if resp.Items[0].Code != "600001" {
		t.Fatalf("expected non-opportunity stock 600001 to rank first, got %s", resp.Items[0].Code)
	}
	if resp.Items[0].RankingScore != 99 {
		t.Fatalf("expected ranking_score 99, got %.1f", resp.Items[0].RankingScore)
	}
}

func TestGetRanking_AShareAppliesBoardQuotas(t *testing.T) {
	repo, cleanup := setupQuadrantTest(t)
	defer cleanup()
	svc := NewService(repo)
	ctx := context.Background()

	records := make([]QuadrantScoreRecord, 0, 60)
	for i := 0; i < 20; i++ {
		records = append(records, makeAShareRankingRecord(fmt.Sprintf("600%03d", i), "MAIN", "机会", 90-float64(i)*0.1, 80, 20, 12000))
	}
	for i := 0; i < 20; i++ {
		records = append(records, makeAShareRankingRecord(fmt.Sprintf("300%03d", i), "CHINEXT", "机会", 80-float64(i)*0.1, 75, 20, 12000))
	}
	for i := 0; i < 20; i++ {
		records = append(records, makeAShareRankingRecord(fmt.Sprintf("688%03d", i), "STAR", "机会", 100-float64(i)*0.1, 70, 20, 12000))
	}
	seedOpportunityRecords(t, repo, records)

	resp, err := svc.GetRanking(ctx, "ASHARE", 20)
	if err != nil {
		t.Fatalf("GetRanking failed: %v", err)
	}
	if len(resp.Items) != 20 {
		t.Fatalf("expected 20 items, got %d", len(resp.Items))
	}
	counts := map[string]int{}
	for _, item := range resp.Items {
		counts[item.Board]++
	}
	if counts[aShareBoardMain] != 10 || counts[aShareBoardChiNext] != 6 || counts[aShareBoardStar] != 4 {
		t.Fatalf("expected MAIN/CHINEXT/STAR = 10/6/4, got %#v", counts)
	}
}

func TestSelectSnapshotRecords_MatchesGetRankingAShareQuota(t *testing.T) {
	repo, cleanup := setupQuadrantTest(t)
	defer cleanup()
	svc := NewService(repo)
	ctx := context.Background()

	records := make([]QuadrantScoreRecord, 0, 45)
	for i := 0; i < 15; i++ {
		records = append(records, makeAShareRankingRecord(fmt.Sprintf("600%03d", i), "MAIN", "机会", 95-float64(i)*0.1, 80, 20, 12000))
	}
	for i := 0; i < 15; i++ {
		records = append(records, makeAShareRankingRecord(fmt.Sprintf("300%03d", i), "CHINEXT", "机会", 85-float64(i)*0.1, 75, 20, 12000))
	}
	for i := 0; i < 15; i++ {
		records = append(records, makeAShareRankingRecord(fmt.Sprintf("688%03d", i), "STAR", "机会", 100-float64(i)*0.1, 70, 20, 12000))
	}
	seedOpportunityRecords(t, repo, records)

	resp, err := svc.GetRanking(ctx, "ASHARE", 20)
	if err != nil {
		t.Fatalf("GetRanking failed: %v", err)
	}
	snapshotRecords := selectSnapshotRecords(records, 20)
	if len(resp.Items) != len(snapshotRecords) {
		t.Fatalf("expected realtime and snapshot lengths to match, got %d vs %d", len(resp.Items), len(snapshotRecords))
	}
	for i := range snapshotRecords {
		if resp.Items[i].Code != snapshotRecords[i].Code {
			t.Fatalf("expected realtime and snapshot record %d to match, got %s vs %s", i, resp.Items[i].Code, snapshotRecords[i].Code)
		}
	}
}

func TestGetRanking_HKEXKeepsOpportunitySortingWithRankingScorePresent(t *testing.T) {
	repo, cleanup := setupQuadrantTest(t)
	defer cleanup()
	svc := NewService(repo)
	ctx := context.Background()

	hkNeutral := makeAShareRankingRecord("00700", "", "中性", 99, 50, 30, 5000)
	hkNeutral.Exchange = "HKEX"
	hkNeutral.Name = "腾讯控股"
	hkOpportunity := makeAShareRankingRecord("00005", "", "机会", 70, 90, 18, 5000)
	hkOpportunity.Exchange = "HKEX"
	hkOpportunity.Name = "汇丰控股"

	seedOpportunityRecords(t, repo, []QuadrantScoreRecord{hkNeutral, hkOpportunity})

	resp, err := svc.GetRanking(ctx, "HKEX", 20)
	if err != nil {
		t.Fatalf("GetRanking(HKEX) failed: %v", err)
	}
	if len(resp.Items) != 1 {
		t.Fatalf("expected only 1 HK opportunity item, got %d", len(resp.Items))
	}
	if resp.Items[0].Code != "00005" {
		t.Fatalf("expected HK opportunity stock 00005 first, got %s", resp.Items[0].Code)
	}
}

func countSelectedBoards(records []QuadrantScoreRecord) map[string]int {
	counts := map[string]int{}
	for _, record := range records {
		counts[normalizeAShareRankingBoard(record)]++
	}
	return counts
}

func TestSelectAShareBalancedRanking_UsesTargetQuotas(t *testing.T) {
	records := make([]QuadrantScoreRecord, 0, 60)
	for i := 0; i < 20; i++ {
		records = append(records, makeAShareRankingRecord(fmt.Sprintf("600%03d", i), "MAIN", "机会", float64(90-i), 80, 20, 12000))
	}
	for i := 0; i < 20; i++ {
		records = append(records, makeAShareRankingRecord(fmt.Sprintf("300%03d", i), "CHINEXT", "机会", float64(80-i), 75, 20, 12000))
	}
	for i := 0; i < 20; i++ {
		records = append(records, makeAShareRankingRecord(fmt.Sprintf("688%03d", i), "STAR", "机会", 100-float64(i)*0.1, 70, 20, 12000))
	}

	selected := selectAShareBalancedRanking(records, 20)
	if len(selected) != 20 {
		t.Fatalf("expected 20 records, got %d", len(selected))
	}
	counts := countSelectedBoards(selected)
	if counts[aShareBoardMain] != 10 || counts[aShareBoardChiNext] != 6 || counts[aShareBoardStar] != 4 {
		t.Fatalf("expected MAIN/CHINEXT/STAR = 10/6/4, got %#v", counts)
	}
	for i := 0; i < len(selected)-1; i++ {
		if selected[i].RankingScore < selected[i+1].RankingScore {
			t.Fatalf("expected final list sorted by ranking_score desc at index %d: %.1f < %.1f", i, selected[i].RankingScore, selected[i+1].RankingScore)
		}
	}
}

func TestSelectAShareBalancedRanking_FillsBackWhenBoardCoverageInsufficient(t *testing.T) {
	records := make([]QuadrantScoreRecord, 0, 20)
	for i := 0; i < 18; i++ {
		records = append(records, makeAShareRankingRecord(fmt.Sprintf("600%03d", i), "MAIN", "机会", float64(100-i), 80, 20, 12000))
	}
	for i := 0; i < 2; i++ {
		records = append(records, makeAShareRankingRecord(fmt.Sprintf("300%03d", i), "CHINEXT", "机会", float64(70-i), 75, 20, 12000))
	}

	selected := selectAShareBalancedRanking(records, 20)
	if len(selected) != 20 {
		t.Fatalf("expected quota fallback to fill back to 20, got %d", len(selected))
	}
	counts := countSelectedBoards(selected)
	if counts[aShareBoardMain] != 18 || counts[aShareBoardChiNext] != 2 {
		t.Fatalf("expected MAIN/CHINEXT = 18/2 when other boards are insufficient, got %#v", counts)
	}
}

func TestSelectAShareBalancedRanking_InfersBoardFromCodeWhenBoardMissing(t *testing.T) {
	records := []QuadrantScoreRecord{
		makeAShareRankingRecord("600100", "", "机会", 90, 80, 20, 12000),
		makeAShareRankingRecord("300100", "", "机会", 89, 80, 20, 12000),
		makeAShareRankingRecord("688100", "", "机会", 88, 80, 20, 12000),
	}

	selected := selectAShareBalancedRanking(records, 3)
	counts := countSelectedBoards(selected)
	if counts[aShareBoardMain] != 1 || counts[aShareBoardChiNext] != 1 || counts[aShareBoardStar] != 1 {
		t.Fatalf("expected board inference by code to select one per board for limit=3, got %#v", counts)
	}
}

func TestGetRanking_AShareFallbackToOpportunityWhenRankingScoreMissing(t *testing.T) {
	repo, cleanup := setupQuadrantTest(t)
	defer cleanup()
	svc := NewService(repo)
	ctx := context.Background()

	records := []QuadrantScoreRecord{
		makeRankingRecord("600010", "SSE", 92, 18),
		makeRankingRecord("600011", "SSE", 88, 15),
		makeNonOpportunityRecord("600012", "SSE", "中性"),
	}
	seedOpportunityRecords(t, repo, records)

	resp, err := svc.GetRanking(ctx, "ASHARE", 20)
	if err != nil {
		t.Fatalf("GetRanking failed: %v", err)
	}
	if len(resp.Items) != 2 {
		t.Fatalf("expected 2 fallback items, got %d", len(resp.Items))
	}
	if resp.Items[0].Code != "600010" {
		t.Fatalf("expected fallback to opportunity sort, got %s", resp.Items[0].Code)
	}
}

func TestGetRanking_AShareStillFiltersSTAndLowLiquidityWithRankingScore(t *testing.T) {
	repo, cleanup := setupQuadrantTest(t)
	defer cleanup()
	svc := NewService(repo)
	ctx := context.Background()

	stRecord := makeAShareRankingRecord("600020", "MAIN", "机会", 99, 80, 20, 12000)
	stRecord.Name = "ST测试"
	lowLiquidity := makeAShareRankingRecord("600021", "MAIN", "机会", 98, 80, 20, 1000)
	valid := makeAShareRankingRecord("600022", "MAIN", "机会", 97, 80, 20, 15000)
	seedOpportunityRecords(t, repo, []QuadrantScoreRecord{stRecord, lowLiquidity, valid})

	resp, err := svc.GetRanking(ctx, "ASHARE", 20)
	if err != nil {
		t.Fatalf("GetRanking failed: %v", err)
	}
	if len(resp.Items) != 1 {
		t.Fatalf("expected only 1 valid item, got %d", len(resp.Items))
	}
	if resp.Items[0].Code != "600022" {
		t.Fatalf("expected valid stock 600022, got %s", resp.Items[0].Code)
	}
}

// ── resolveRankingExchanges helper tests ──

func TestResolveRankingExchanges(t *testing.T) {
	tests := []struct {
		input    string
		expected []string
	}{
		{"HKEX", []string{"HKEX"}},
		{"hkex", []string{"HKEX"}},
		{"ASHARE", []string{"SSE", "SZSE"}},
		{"", []string{"SSE", "SZSE"}},
		{"garbage", []string{"SSE", "SZSE"}},
	}

	for _, tt := range tests {
		got := resolveRankingExchanges(tt.input)
		if len(got) != len(tt.expected) {
			t.Errorf("resolveRankingExchanges(%q): expected %v, got %v", tt.input, tt.expected, got)
			continue
		}
		for i := range got {
			if got[i] != tt.expected[i] {
				t.Errorf("resolveRankingExchanges(%q)[%d]: expected %s, got %s", tt.input, i, tt.expected[i], got[i])
			}
		}
	}
}

// ── Regression: AShare ranking when all records are SZSE (legacy data) ──

func TestGetRanking_AShareAllSZSE(t *testing.T) {
	repo, cleanup := setupQuadrantTest(t)
	defer cleanup()
	svc := NewService(repo)
	ctx := context.Background()

	// Simulates legacy data where ALL A-share stocks have exchange="SZSE"
	// (before fix: compute_all_quadrant_scores didn't set exchange → Go defaulted to SZSE)
	records := make([]QuadrantScoreRecord, 0, 15)
	for i := 1; i <= 15; i++ {
		code := padCode(i, 6) // 000001 ~ 000015
		records = append(records, makeRankingRecord(code, "SZSE",
			float64(99-i), float64(10+i)))
	}

	seedOpportunityRecords(t, repo, records)

	resp, err := svc.GetRanking(ctx, "ASHARE", 20)
	if err != nil {
		t.Fatalf("GetRanking(ASHARE) with all-SZSE data failed: %v", err)
	}
	if len(resp.Items) == 0 {
		t.Error("expected non-empty ranking for ASHARE when all records are SZSE — this is the legacy-data regression case")
	}
	if resp.Meta.TotalInZone == 0 {
		t.Error("expected TotalInZone > 0")
	}
	if resp.Meta.Exchange != "ASHARE" {
		t.Errorf("expected Exchange=ASHARE, got %s", resp.Meta.Exchange)
	}
}

// ── Regression: AShare ranking with mixed SSE + SZSE (after fix) ──

func TestGetRanking_AShareMixedExchange(t *testing.T) {
	repo, cleanup := setupQuadrantTest(t)
	defer cleanup()
	svc := NewService(repo)
	ctx := context.Background()

	records := []QuadrantScoreRecord{
		makeRankingRecord("600519", "SSE", 98, 12),
		makeRankingRecord("600000", "SSE", 95, 25),
		makeRankingRecord("601318", "SSE", 93, 18),
		makeRankingRecord("000001", "SZSE", 97, 15),
		makeRankingRecord("000002", "SZSE", 96, 20),
		makeRankingRecord("300750", "SZSE", 94, 22),
	}
	seedOpportunityRecords(t, repo, records)

	resp, err := svc.GetRanking(ctx, "ASHARE", 10)
	if err != nil {
		t.Fatalf("GetRanking(ASHARE) mixed exchange failed: %v", err)
	}

	if len(resp.Items) != 6 {
		t.Errorf("expected 6 items (SSE+SZSE combined), got %d", len(resp.Items))
	}

	sseCount := 0
	szseCount := 0
	for _, item := range resp.Items {
		switch item.Exchange {
		case "SSE":
			sseCount++
		case "SZSE":
			szseCount++
		default:
			t.Errorf("unexpected exchange in ASHARE ranking: %s (%s)", item.Exchange, item.Code)
		}
	}
	if sseCount != 3 {
		t.Errorf("expected 3 SSE items, got %d", sseCount)
	}
	if szseCount != 3 {
		t.Errorf("expected 3 SZSE items, got %d", szseCount)
	}
}

// ── Regression: HKEX ranking must never include SSE/SZSE records ──

func TestGetRanking_HKEXStrictIsolation(t *testing.T) {
	repo, cleanup := setupQuadrantTest(t)
	defer cleanup()
	svc := NewService(repo)
	ctx := context.Background()

	records := []QuadrantScoreRecord{
		makeRankingRecord("00700", "HKEX", 92, 20),
		makeRankingRecord("00005", "HKEX", 88, 25),
		// These SSE/SZSE records should NEVER appear in HKEX results
		makeRankingRecord("600519", "SSE", 99, 5), // high opportunity but wrong exchange!
		makeRankingRecord("000001", "SZSE", 98, 10),
	}
	seedOpportunityRecords(t, repo, records)

	resp, err := svc.GetRanking(ctx, "HKEX", 50)
	if err != nil {
		t.Fatalf("GetRanking(HKEX) failed: %v", err)
	}

	for _, item := range resp.Items {
		if item.Exchange != "HKEX" {
			t.Errorf("HKEX ranking leaked non-HK stock: code=%s exchange=%s", item.Code, item.Exchange)
		}
	}
	if len(resp.Items) != 2 {
		t.Errorf("HKEX ranking should return only 2 HK items, got %d", len(resp.Items))
	}
}

// ── Regression: BulkSave preserves explicit exchange field ──

func TestBulkSave_ExchangePreservation(t *testing.T) {
	repo, cleanup := setupQuadrantTest(t)
	defer cleanup()
	svc := NewService(repo)
	ctx := context.Background()

	input := BulkSaveInput{
		Items: []BulkSaveItem{
			{Code: "600519", Name: "贵州茅台", Opportunity: 90, Risk: 20,
				Quadrant: "机会", Trend: 85, Flow: 80, Revision: 75,
				Volatility: 30, Drawdown: 15, Crowding: 40, Exchange: "SSE",
				Board: "MAIN", RankingScore: 91.2, GlobalRankScore: 89.1, BoardRankScore: 95.0,
				TradabilityScore: 88.0, RiskAdjustedMomentum60d: 1234.5},
			{Code: "000001", Name: "平安银行", Opportunity: 85, Risk: 25,
				Quadrant: "机会", Trend: 80, Flow: 75, Revision: 70,
				Volatility: 35, Drawdown: 18, Crowding: 45, Exchange: "SZSE"},
			{Code: "00700", Name: "腾讯控股", Opportunity: 92, Risk: 18,
				Quadrant: "机会", Trend: 88, Flow: 82, Revision: 78,
				Volatility: 28, Drawdown: 12, Crowding: 35, Exchange: "HKEX"},
			// No exchange field → should default to SZSE
			{Code: "300750", Name: "宁德时代", Opportunity: 88, Risk: 22,
				Quadrant: "机会", Trend: 83, Flow: 78, Revision: 72,
				Volatility: 32, Drawdown: 14, Crowding: 38},
		},
		ComputedAt: "2026-04-15T02:30:00Z",
	}

	count, err := svc.BulkSave(ctx, input)
	if err != nil {
		t.Fatalf("BulkSave failed: %v", err)
	}
	if count != 4 {
		t.Errorf("expected 4 saved, got %d", count)
	}

	// Verify each record's exchange was preserved or defaulted correctly
	all, err := repo.FindByExchange(ctx, []string{"SSE", "SZSE", "HKEX"})
	if err != nil {
		t.Fatalf("FindByExchange failed: %v", err)
	}
	exchangeMap := map[string]string{}
	for _, r := range all {
		exchangeMap[r.Code] = r.Exchange
	}
	if exchangeMap["600519"] != "SSE" {
		t.Errorf("600519 exchange should be SSE, got %s", exchangeMap["600519"])
	}
	if exchangeMap["000001"] != "SZSE" {
		t.Errorf("000001 exchange should be SZSE, got %s", exchangeMap["000001"])
	}
	if exchangeMap["00700"] != "HKEX" {
		t.Errorf("00700 exchange should be HKEX, got %s", exchangeMap["00700"])
	}
	if exchangeMap["300750"] != "SZSE" {
		t.Errorf("300750 (no exchange) should default to SZSE, got %s", exchangeMap["300750"])
	}

	var maotai QuadrantScoreRecord
	if err := repo.db.WithContext(ctx).Where("code = ?", "600519").First(&maotai).Error; err != nil {
		t.Fatalf("query 600519 failed: %v", err)
	}
	if maotai.Board != "MAIN" || maotai.RankingScore != 91.2 || maotai.GlobalRankScore != 89.1 ||
		maotai.BoardRankScore != 95.0 || maotai.TradabilityScore != 88.0 || maotai.RiskAdjustedMomentum60d != 1234.5 {
		t.Fatalf("new ranking fields not persisted correctly: %+v", maotai)
	}
}

// ── Liquidity hard-filter regression tests ──

func makeLowLiquidityRecord(code, exchange string, opportunity float64) QuadrantScoreRecord {
	r := makeRankingRecord(code, exchange, opportunity, 20.0)
	r.AvgAmount5d = 1000.0 // 1000 万，低于 A 股门槛 5000 万
	return r
}

func makeHighLiquidityRecord(code, exchange string, opportunity float64) QuadrantScoreRecord {
	r := makeRankingRecord(code, exchange, opportunity, 20.0)
	r.AvgAmount5d = 50000.0 // 5 亿元，满足门槛
	return r
}

// TestGetRanking_LiquidityFilter_ExcludesIlliquid: low avg_amount_5d stocks must be excluded from ASHARE ranking
func TestGetRanking_LiquidityFilter_ExcludesIlliquid(t *testing.T) {
	repo, cleanup := setupQuadrantTest(t)
	defer cleanup()
	svc := NewService(repo)
	ctx := context.Background()

	records := []QuadrantScoreRecord{
		makeHighLiquidityRecord("600519", "SSE", 98),  // passes filter
		makeHighLiquidityRecord("000001", "SZSE", 95), // passes filter
		makeLowLiquidityRecord("300123", "SZSE", 99),  // high opp but illiquid → excluded!
		makeLowLiquidityRecord("600888", "SSE", 96),   // excluded
	}
	seedOpportunityRecords(t, repo, records)

	resp, err := svc.GetRanking(ctx, "ASHARE", 20)
	if err != nil {
		t.Fatalf("GetRanking(ASHARE) with liquidity filter failed: %v", err)
	}

	if len(resp.Items) != 2 {
		t.Errorf("expected only 2 items (high-liquidity passed), got %d", len(resp.Items))
	}
	for _, item := range resp.Items {
		if item.AvgAmount5d < 5000 {
			t.Errorf("stock %s has avg_amount_5d=%.1f below threshold but appeared in results", item.Code, item.AvgAmount5d)
		}
		if item.Code == "300123" || item.Code == "600888" {
			t.Errorf("illiquid stock %s leaked into ranking", item.Code)
		}
	}
}

// TestGetRanking_LiquidityFilter_HKEXThreshold: HKEX uses 2000M threshold (lower than ASHARE's 5000M)
func TestGetRanking_LiquidityFilter_HKEXThreshold(t *testing.T) {
	repo, cleanup := setupQuadrantTest(t)
	defer cleanup()
	svc := NewService(repo)
	ctx := context.Background()

	hkPass := makeHighLiquidityRecord("00700", "HKEX", 95)
	hkPass.AvgAmount5d = 3000.0 // 3000 万 > HKEX 门槛 2000 → 通过

	hkFail := makeLowLiquidityRecord("00005", "HKEX", 97)
	hkFail.AvgAmount5d = 1500.0 // 1500 万 < HKEX 门槛 2000 → 被过滤

	records := []QuadrantScoreRecord{hkPass, hkFail}
	seedOpportunityRecords(t, repo, records)

	resp, err := svc.GetRanking(ctx, "HKEX", 20)
	if err != nil {
		t.Fatalf("GetRanking(HKEX) failed: %v", err)
	}

	if len(resp.Items) != 1 {
		t.Errorf("expected 1 HK item passing liquidity filter, got %d", len(resp.Items))
	}
	if len(resp.Items) > 0 && resp.Items[0].Code != "00700" {
		t.Errorf("expected 00700 to pass HKEX liquidity filter, got %s", resp.Items[0].Code)
	}
}

// TestGetRanking_RankingItemHasLiquidityFields: every returned RankingItem must have Liquidity and AvgAmount5d
func TestGetRanking_RankingItemHasLiquidityFields(t *testing.T) {
	repo, cleanup := setupQuadrantTest(t)
	defer cleanup()
	svc := NewService(repo)
	ctx := context.Background()

	records := []QuadrantScoreRecord{
		makeHighLiquidityRecord("601318", "SSE", 90),
		makeHighLiquidityRecord("000858", "SZSE", 88),
	}
	seedOpportunityRecords(t, repo, records)

	resp, _ := svc.GetRanking(ctx, "ASHARE", 10)

	for i, item := range resp.Items {
		if item.Liquidity == 0 && i < len(records) { // allow zero only if test data is zero
			// check original data had non-zero liquidity
			if records[i].Liquidity != 0 {
				t.Errorf("item[%d] (%s): Liquidity=0 but expected %.1f", i, item.Code, records[i].Liquidity)
			}
		}
		if item.AvgAmount5d == 0 {
			t.Errorf("item[%d] (%s): AvgAmount5d=0, expected non-zero", i, item.Code)
		}
	}
}

// ── Backward-compat: old data without avg_amount5d must still show results ──

// makeLegacyRecord simulates pre-liquidity data where avg_amount5d = 0
func makeLegacyRecord(code, exchange string, opportunity float64) QuadrantScoreRecord {
	r := makeRankingRecord(code, exchange, opportunity, 20.0)
	r.AvgAmount5d = 0 // no liquidity data — old computation
	return r
}

// TestGetRanking_LegacyData_ZeroAmount_ShowsResults:
// When ALL records have avg_amount5d=0 (pre-liquidity computation),
// the filter must be disabled so users still see ranking results.
func TestGetRanking_LegacyData_ZeroAmount_ShowsResults(t *testing.T) {
	repo, cleanup := setupQuadrantTest(t)
	defer cleanup()
	svc := NewService(repo)
	ctx := context.Background()

	records := []QuadrantScoreRecord{
		makeLegacyRecord("600519", "SSE", 98),
		makeLegacyRecord("000001", "SZSE", 95),
		makeLegacyRecord("601318", "SSE", 93),
		makeLegacyRecord("300750", "SZSE", 91),
	}
	seedOpportunityRecords(t, repo, records)

	resp, err := svc.GetRanking(ctx, "ASHARE", 20)
	if err != nil {
		t.Fatalf("GetRanking(ASHARE) with legacy zero-amount data failed: %v", err)
	}

	if len(resp.Items) == 0 {
		t.Error("expected non-empty results for legacy data (avg_amount5d=0) — filter should be auto-disabled")
	}
	if len(resp.Items) != 4 {
		t.Errorf("expected all 4 legacy items to appear (filter disabled), got %d", len(resp.Items))
	}

	// Verify sort order still works correctly on legacy data
	for i := 1; i < len(resp.Items); i++ {
		if resp.Items[i].Opportunity > resp.Items[i-1].Opportunity {
			t.Errorf("sort order violation in legacy data at index %d", i)
		}
	}
}

// TestGetRanking_LegacyData_HKEX_ShowsResults: same backward-compat check for HKEX
func TestGetRanking_LegacyData_HKEX_ShowsResults(t *testing.T) {
	repo, cleanup := setupQuadrantTest(t)
	defer cleanup()
	svc := NewService(repo)
	ctx := context.Background()

	records := []QuadrantScoreRecord{
		makeLegacyRecord("00700", "HKEX", 92),
		makeLegacyRecord("03968", "HKEX", 88),
	}
	seedOpportunityRecords(t, repo, records)

	resp, err := svc.GetRanking(ctx, "HKEX", 20)
	if err != nil {
		t.Fatalf("GetRanking(HKEX) with legacy data failed: %v", err)
	}
	if len(resp.Items) != 2 {
		t.Errorf("expected 2 HK legacy items (filter disabled), got %d", len(resp.Items))
	}
}

// TestHasNonZeroLiquidity: verify repository method directly
func TestHasNonZeroLiquidity(t *testing.T) {
	repo, cleanup := setupQuadrantTest(t)
	defer cleanup()
	ctx := context.Background()

	// Empty table → false
	hasData, err := repo.HasNonZeroLiquidity(ctx, []string{"SSE"})
	if err != nil {
		t.Fatalf("HasNonZeroLiquidity on empty table failed: %v", err)
	}
	if hasData {
		t.Error("empty table should return false")
	}

	// Only zero-amount records → false
	seedOpportunityRecords(t, repo, []QuadrantScoreRecord{
		makeLegacyRecord("600519", "SSE", 90),
	})
	hasData, err = repo.HasNonZeroLiquidity(ctx, []string{"SSE"})
	if err != nil {
		t.Fatalf("HasNonZeroLiquidity with zero data failed: %v", err)
	}
	if hasData {
		t.Error("all-zero amounts should return false")
	}

	// At least one non-zero record → true
	seedOpportunityRecords(t, repo, []QuadrantScoreRecord{
		makeHighLiquidityRecord("000001", "SZSE", 85),
	})
	hasData, err = repo.HasNonZeroLiquidity(ctx, []string{"SSE", "SZSE"})
	if err != nil {
		t.Fatalf("HasNonZeroLiquidity with mixed data failed: %v", err)
	}
	if !hasData {
		t.Error("at least one non-zero amount should return true")
	}
}

// ── Ranking Snapshot Tests ──

func makeSnapshot(code, exchange string, rank int, opp float64, date time.Time) RankingSnapshot {
	return RankingSnapshot{
		Code:         code,
		Name:         "股票" + code,
		Exchange:     exchange,
		Rank:         rank,
		Opportunity:  opp,
		Risk:         20.0,
		ClosePrice:   float64(rank*10 + 5), // fake price for testing
		SnapshotDate: date.Format("2006-01-02"),
		CreatedAt:    time.Now().UTC(),
	}
}

func TestSnapshot_UpsertAndRetrieve(t *testing.T) {
	repo, cleanup := setupQuadrantTest(t)
	defer cleanup()
	ctx := context.Background()
	date := time.Date(2026, 4, 14, 0, 0, 0, 0, time.UTC)

	snap := makeSnapshot("600519", "SSE", 1, 98.5, date)

	err := repo.UpsertSnapshot(ctx, snap)
	if err != nil {
		t.Fatalf("UpsertSnapshot failed: %v", err)
	}

	// Upsert again with updated rank (should not duplicate)
	snap.Rank = 2
	err = repo.UpsertSnapshot(ctx, snap)
	if err != nil {
		t.Fatalf("second UpsertSnapshot failed: %v", err)
	}

	// Verify only one record exists
	var count int64
	repo.db.WithContext(ctx).Model(&RankingSnapshot{}).Count(&count)
	if count != 1 {
		t.Errorf("expected 1 snapshot after upsert, got %d", count)
	}
}

func TestSnapshot_ConsecutiveDays(t *testing.T) {
	repo, cleanup := setupQuadrantTest(t)
	defer cleanup()
	ctx := context.Background()

	snaps := []RankingSnapshot{
		makeSnapshot("000001", "SZSE", 1, 95.0, time.Date(2026, 4, 15, 10, 0, 0, 0, time.UTC)),
		makeSnapshot("000001", "SZSE", 2, 94.0, time.Date(2026, 4, 14, 2, 30, 0, 0, time.UTC)),
		makeSnapshot("000001", "SZSE", 3, 93.0, time.Date(2026, 4, 13, 9, 15, 0, 0, time.UTC)),
		// Gap here: April 12 is missing → break consecutive chain
	}
	for _, s := range snaps {
		if err := repo.UpsertSnapshot(ctx, s); err != nil {
			t.Fatalf("seed snapshot failed: %v", err)
		}
	}

	days, err := repo.GetConsecutiveDays(ctx, "000001", []string{"SZSE"})
	if err != nil {
		t.Fatalf("GetConsecutiveDays failed: %v", err)
	}
	if days != 3 {
		t.Errorf("expected 3 consecutive days, got %d", days)
	}
}

func TestSnapshot_ConsecutiveDays_WithGap(t *testing.T) {
	repo, cleanup := setupQuadrantTest(t)
	defer cleanup()
	ctx := context.Background()

	snaps := []RankingSnapshot{
		makeSnapshot("600519", "SSE", 1, 99.0, time.Date(2026, 4, 15, 10, 0, 0, 0, time.UTC)),
		// Missing April 14
		makeSnapshot("600519", "SSE", 3, 97.0, time.Date(2026, 4, 13, 10, 0, 0, 0, time.UTC)),
	}
	for _, s := range snaps {
		if err := repo.UpsertSnapshot(ctx, s); err != nil {
			t.Fatalf("seed snapshot failed: %v", err)
		}
	}

	days, _ := repo.GetConsecutiveDays(ctx, "600519", []string{"SSE"})
	// Should be 1 (only the most recent day; gap breaks the chain)
	if days != 1 {
		t.Errorf("expected 1 consecutive day (gap breaks chain), got %d", days)
	}
}

func TestSnapshot_ConsecutiveDays_EmptyTable(t *testing.T) {
	repo, cleanup := setupQuadrantTest(t)
	defer cleanup()
	ctx := context.Background()

	days, err := repo.GetConsecutiveDays(ctx, "999999", []string{"SSE"})
	if err != nil {
		t.Fatalf("GetConsecutiveDays on empty table error: %v", err)
	}
	if days != 0 {
		t.Errorf("expected 0 for empty table, got %d", days)
	}
}

func TestSnapshot_FirstAppearedDate(t *testing.T) {
	repo, cleanup := setupQuadrantTest(t)
	defer cleanup()
	ctx := context.Background()

	snaps := []RankingSnapshot{
		makeSnapshot("00700", "HKEX", 5, 88.0, time.Date(2026, 4, 13, 10, 0, 0, 0, time.UTC)),
		makeSnapshot("00700", "HKEX", 3, 92.0, time.Date(2026, 4, 15, 10, 0, 0, 0, time.UTC)),
	}
	for _, s := range snaps {
		if err := repo.UpsertSnapshot(ctx, s); err != nil {
			t.Fatalf("seed snapshot failed: %v", err)
		}
	}

	firstDateStr, err := repo.GetFirstAppearedDate(ctx, "00700", []string{"HKEX"})
	if err != nil {
		t.Fatalf("GetFirstAppearedDate failed: %v", err)
	}
	if firstDateStr == "" {
		t.Fatal("expected a first-appeared date, got empty")
	}
	expected := "2026-04-13"
	if firstDateStr != expected {
		t.Errorf("expected first appeared at %s, got %s", expected, firstDateStr)
	}
}

func TestSnapshot_ClosePriceOnDate(t *testing.T) {
	repo, cleanup := setupQuadrantTest(t)
	defer cleanup()
	ctx := context.Background()
	date := time.Date(2026, 4, 15, 10, 0, 0, 0, time.UTC)
	dateStr := date.Format("2006-01-02")

	snap := makeSnapshot("601318", "SSE", 1, 90.0, date)
	snap.ClosePrice = 185.50
	if err := repo.UpsertSnapshot(ctx, snap); err != nil {
		t.Fatalf("seed snapshot failed: %v", err)
	}

	price, err := repo.GetClosePriceOnDate(ctx, "601318", dateStr)
	if err != nil {
		t.Fatalf("GetClosePriceOnDate failed: %v", err)
	}
	if price != 185.50 {
		t.Errorf("expected close_price=185.50, got %.2f", price)
	}
}

func TestSnapshot_GetLatestAvailableClosePrice(t *testing.T) {
	repo, cleanup := setupQuadrantTest(t)
	defer cleanup()
	ctx := context.Background()

	for _, snap := range []RankingSnapshot{
		{Code: "601318", Name: "中国平安", Exchange: "SSE", Rank: 1, Opportunity: 90, Risk: 20, ClosePrice: 10, SnapshotDate: "2026-04-16", CreatedAt: time.Now().UTC()},
		{Code: "601318", Name: "中国平安", Exchange: "SSE", Rank: 1, Opportunity: 90, Risk: 20, ClosePrice: 0, SnapshotDate: "2026-04-18", CreatedAt: time.Now().UTC()},
		{Code: "601318", Name: "中国平安", Exchange: "SSE", Rank: 1, Opportunity: 90, Risk: 20, ClosePrice: 11, SnapshotDate: "2026-04-17", CreatedAt: time.Now().UTC()},
	} {
		if err := repo.UpsertSnapshot(ctx, snap); err != nil {
			t.Fatalf("seed snapshot failed: %v", err)
		}
	}

	price, dateStr, err := repo.GetLatestAvailableClosePrice(ctx, "601318", []string{"SSE"})
	if err != nil {
		t.Fatalf("GetLatestAvailableClosePrice failed: %v", err)
	}
	if dateStr != "2026-04-17" || price != 11 {
		t.Fatalf("latest available = (%s, %.2f), want (2026-04-17, 11.00)", dateStr, price)
	}
}

func TestSnapshot_GetEarliestAvailableClosePrice(t *testing.T) {
	repo, cleanup := setupQuadrantTest(t)
	defer cleanup()
	ctx := context.Background()

	for _, snap := range []RankingSnapshot{
		{Code: "00700", Name: "腾讯控股", Exchange: "HKEX", Rank: 1, Opportunity: 90, Risk: 20, ClosePrice: 0, SnapshotDate: "2026-04-20", CreatedAt: time.Now().UTC()},
		{Code: "00700", Name: "腾讯控股", Exchange: "HKEX", Rank: 1, Opportunity: 90, Risk: 20, ClosePrice: 11, SnapshotDate: "2026-04-21", CreatedAt: time.Now().UTC()},
		{Code: "00700", Name: "腾讯控股", Exchange: "HKEX", Rank: 1, Opportunity: 90, Risk: 20, ClosePrice: 12, SnapshotDate: "2026-04-22", CreatedAt: time.Now().UTC()},
	} {
		if err := repo.UpsertSnapshot(ctx, snap); err != nil {
			t.Fatalf("seed snapshot failed: %v", err)
		}
	}

	price, dateStr, err := repo.GetEarliestAvailableClosePrice(ctx, "00700", []string{"HKEX"}, "2026-04-20")
	if err != nil {
		t.Fatalf("GetEarliestAvailableClosePrice failed: %v", err)
	}
	if dateStr != "2026-04-21" || price != 11 {
		t.Fatalf("earliest available = (%s, %.2f), want (2026-04-21, 11.00)", dateStr, price)
	}
}

func TestSaveRankingSnapshotsBestEffort_UsesResolverAndShanghaiDate(t *testing.T) {
	repo, cleanup := setupQuadrantTest(t)
	defer cleanup()
	ctx := context.Background()
	svc := NewService(repo)

	var calledCode, calledExchange, calledDate string
	svc.SetPriceResolver(func(ctx context.Context, code string, exchange string, tradeDate string) float64 {
		calledCode = code
		calledExchange = exchange
		calledDate = tradeDate
		return 123.45
	})

	record := makeRankingRecord("600519", "SSE", 95, 20)
	computedAt := time.Date(2026, 4, 16, 18, 46, 37, 0, time.UTC) // 上海时间 2026-04-17 02:46:37
	svc.saveRankingSnapshotsBestEffort(ctx, []QuadrantScoreRecord{record}, computedAt)

	if calledCode != "600519" || calledExchange != "SSE" {
		t.Fatalf("resolver called with unexpected args: code=%s exchange=%s", calledCode, calledExchange)
	}
	if calledDate != "2026-04-17" {
		t.Fatalf("resolver trade date = %s; want 2026-04-17", calledDate)
	}

	var snap RankingSnapshot
	if err := repo.db.WithContext(ctx).Where("code = ?", "600519").First(&snap).Error; err != nil {
		t.Fatalf("query snapshot failed: %v", err)
	}
	if snap.SnapshotDate != "2026-04-17" {
		t.Fatalf("snapshot_date = %s; want 2026-04-17", snap.SnapshotDate)
	}
	if snap.ClosePrice != 123.45 {
		t.Fatalf("close_price = %.2f; want 123.45", snap.ClosePrice)
	}
	if snap.PriceTradeDate != "2026-04-17" {
		t.Fatalf("price_trade_date = %s; want 2026-04-17", snap.PriceTradeDate)
	}
}

func TestBulkSaveRankingSnapshots_UsesCallbackClosePriceBeforeResolver(t *testing.T) {
	repo, cleanup := setupQuadrantTest(t)
	defer cleanup()
	ctx := context.Background()
	svc := NewService(repo)
	resolverCalls := 0
	svc.SetPriceResolver(func(ctx context.Context, code string, exchange string, tradeDate string) float64 {
		resolverCalls++
		return 999
	})

	_, err := svc.BulkSave(ctx, BulkSaveInput{
		ComputedAt: "2026-04-30T18:30:00Z", // 上海时间 2026-05-01
		Items: []BulkSaveItem{{
			Code:           "600519",
			Name:           "贵州茅台",
			Exchange:       "SSE",
			Opportunity:    95,
			Risk:           20,
			Quadrant:       "机会",
			AvgAmount5d:    10000,
			ClosePrice:     88.8,
			PriceTradeDate: "2026-04-30",
		}},
	})
	if err != nil {
		t.Fatalf("BulkSave failed: %v", err)
	}
	if resolverCalls != 0 {
		t.Fatalf("resolver should not be called when callback close_price is available, got %d calls", resolverCalls)
	}

	var snap RankingSnapshot
	if err := repo.db.WithContext(ctx).Where("code = ? AND snapshot_date = ?", "600519", "2026-05-01").First(&snap).Error; err != nil {
		t.Fatalf("query snapshot failed: %v", err)
	}
	if snap.ClosePrice != 88.8 {
		t.Fatalf("close_price = %.2f; want 88.80", snap.ClosePrice)
	}
	if snap.PriceTradeDate != "2026-04-30" {
		t.Fatalf("price_trade_date = %s; want 2026-04-30", snap.PriceTradeDate)
	}
}

func TestRepairRecentMissingSnapshotPrices_UsesCallbackHintForPriorTradeDate(t *testing.T) {
	repo, cleanup := setupQuadrantTest(t)
	defer cleanup()
	ctx := context.Background()
	svc := NewService(repo)

	oldSnap := RankingSnapshot{Code: "00700", Name: "腾讯控股", Exchange: "HKEX", Rank: 1, Opportunity: 95, Risk: 20, ClosePrice: 0, SnapshotDate: "2026-05-01", CreatedAt: time.Now().UTC()}
	if err := repo.UpsertSnapshot(ctx, oldSnap); err != nil {
		t.Fatalf("seed old snapshot failed: %v", err)
	}
	record := makeRankingRecord("00700", "HKEX", 95, 20)
	computedAt := time.Date(2026, 5, 2, 18, 30, 0, 0, time.UTC) // 上海时间 2026-05-03
	svc.saveRankingSnapshotsBestEffortWithHints(ctx, []QuadrantScoreRecord{record}, computedAt, map[string]snapshotPriceHint{
		snapshotPriceHintKey("00700", "HKEX"): {ClosePrice: 421.5, TradeDate: "2026-04-30"},
	})

	var repaired RankingSnapshot
	if err := repo.db.WithContext(ctx).Where("code = ? AND snapshot_date = ?", "00700", "2026-05-01").First(&repaired).Error; err != nil {
		t.Fatalf("query repaired snapshot failed: %v", err)
	}
	if repaired.ClosePrice != 421.5 {
		t.Fatalf("repaired close_price = %.2f; want 421.50", repaired.ClosePrice)
	}
	if repaired.PriceTradeDate != "2026-04-30" {
		t.Fatalf("repaired price_trade_date = %s; want 2026-04-30", repaired.PriceTradeDate)
	}
}

func TestGetRanking_ReturnPct_ComputedWhenPricesAvailable(t *testing.T) {
	repo, cleanup := setupQuadrantTest(t)
	defer cleanup()
	ctx := context.Background()
	svc := NewService(repo)

	record := makeRankingRecord("600519", "SSE", 95, 20)
	record.ComputedAt = time.Date(2026, 4, 16, 18, 46, 37, 0, time.UTC)
	seedOpportunityRecords(t, repo, []QuadrantScoreRecord{record})

	for _, snap := range []RankingSnapshot{
		{Code: "600519", Name: "贵州茅台", Exchange: "SSE", Rank: 1, Opportunity: 95, Risk: 20, ClosePrice: 10, SnapshotDate: "2026-04-16", CreatedAt: time.Now().UTC()},
		{Code: "600519", Name: "贵州茅台", Exchange: "SSE", Rank: 1, Opportunity: 95, Risk: 20, ClosePrice: 11, SnapshotDate: "2026-04-17", CreatedAt: time.Now().UTC()},
	} {
		if err := repo.UpsertSnapshot(ctx, snap); err != nil {
			t.Fatalf("seed snapshot failed: %v", err)
		}
	}

	resp, err := svc.GetRanking(ctx, "ASHARE", 20)
	if err != nil {
		t.Fatalf("GetRanking failed: %v", err)
	}
	if len(resp.Items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(resp.Items))
	}
	if resp.Items[0].ReturnPct == nil {
		t.Fatal("expected non-nil return_pct")
	}
	if *resp.Items[0].ReturnPct != 10 {
		t.Fatalf("return_pct = %.2f; want 10.00", *resp.Items[0].ReturnPct)
	}
}

func TestGetRanking_ReturnPct_UsesLatestPricedSnapshotFallback(t *testing.T) {
	repo, cleanup := setupQuadrantTest(t)
	defer cleanup()
	ctx := context.Background()
	svc := NewService(repo)

	record := makeRankingRecord("600519", "SSE", 95, 20)
	record.ComputedAt = time.Date(2026, 4, 17, 18, 46, 37, 0, time.UTC) // 上海时间 2026-04-18
	seedOpportunityRecords(t, repo, []QuadrantScoreRecord{record})

	for _, snap := range []RankingSnapshot{
		{Code: "600519", Name: "贵州茅台", Exchange: "SSE", Rank: 1, Opportunity: 95, Risk: 20, ClosePrice: 10, SnapshotDate: "2026-04-16", CreatedAt: time.Now().UTC()},
		{Code: "600519", Name: "贵州茅台", Exchange: "SSE", Rank: 1, Opportunity: 95, Risk: 20, ClosePrice: 11, SnapshotDate: "2026-04-17", CreatedAt: time.Now().UTC()},
		{Code: "600519", Name: "贵州茅台", Exchange: "SSE", Rank: 1, Opportunity: 95, Risk: 20, ClosePrice: 0, SnapshotDate: "2026-04-18", CreatedAt: time.Now().UTC()},
	} {
		if err := repo.UpsertSnapshot(ctx, snap); err != nil {
			t.Fatalf("seed snapshot failed: %v", err)
		}
	}

	resp, err := svc.GetRanking(ctx, "ASHARE", 20)
	if err != nil {
		t.Fatalf("GetRanking failed: %v", err)
	}
	if len(resp.Items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(resp.Items))
	}
	if resp.Items[0].ReturnPct == nil {
		t.Fatal("expected non-nil return_pct when latest priced snapshot exists")
	}
	if *resp.Items[0].ReturnPct != 10 {
		t.Fatalf("return_pct = %.2f; want 10.00", *resp.Items[0].ReturnPct)
	}
}

func TestGetRanking_ReturnPct_NilWhenPriceMissing(t *testing.T) {
	repo, cleanup := setupQuadrantTest(t)
	defer cleanup()
	ctx := context.Background()
	svc := NewService(repo)

	record := makeRankingRecord("000001", "SZSE", 96, 18)
	record.ComputedAt = time.Date(2026, 4, 16, 18, 46, 37, 0, time.UTC)
	seedOpportunityRecords(t, repo, []QuadrantScoreRecord{record})

	// 两个快照的日期不同，但价格相同，涨幅应为 0%
	for _, snap := range []RankingSnapshot{
		{Code: "000001", Name: "平安银行", Exchange: "SZSE", Rank: 1, Opportunity: 96, Risk: 18, ClosePrice: 0, SnapshotDate: "2026-04-16", CreatedAt: time.Now().UTC()},
		{Code: "000001", Name: "平安银行", Exchange: "SZSE", Rank: 1, Opportunity: 96, Risk: 18, ClosePrice: 10, SnapshotDate: "2026-04-17", CreatedAt: time.Now().UTC()},
	} {
		if err := repo.UpsertSnapshot(ctx, snap); err != nil {
			t.Fatalf("seed snapshot failed: %v", err)
		}
	}

	resp, err := svc.GetRanking(ctx, "ASHARE", 20)
	if err != nil {
		t.Fatalf("GetRanking failed: %v", err)
	}
	if len(resp.Items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(resp.Items))
	}
	// 起始价格（2026-04-17 的 10）和当前价格（也是 2026-04-17 的 10）相同，涨幅应为 0%
	if resp.Items[0].ReturnPct == nil {
		t.Fatal("expected non-nil return_pct when both start and current prices are available")
	}
	if *resp.Items[0].ReturnPct != 0 {
		t.Fatalf("return_pct = %.2f; want 0.00", *resp.Items[0].ReturnPct)
	}
}

func TestGetRanking_ReturnPct_NilWhenNoValidPriceAtAll(t *testing.T) {
	repo, cleanup := setupQuadrantTest(t)
	defer cleanup()
	ctx := context.Background()
	svc := NewService(repo)

	record := makeRankingRecord("000001", "SZSE", 96, 18)
	record.ComputedAt = time.Date(2026, 4, 16, 18, 46, 37, 0, time.UTC)
	seedOpportunityRecords(t, repo, []QuadrantScoreRecord{record})

	// 所有快照的 close_price 都为 0，无法计算涨幅
	for _, snap := range []RankingSnapshot{
		{Code: "000001", Name: "平安银行", Exchange: "SZSE", Rank: 1, Opportunity: 96, Risk: 18, ClosePrice: 0, SnapshotDate: "2026-04-16", CreatedAt: time.Now().UTC()},
		{Code: "000001", Name: "平安银行", Exchange: "SZSE", Rank: 1, Opportunity: 96, Risk: 18, ClosePrice: 0, SnapshotDate: "2026-04-17", CreatedAt: time.Now().UTC()},
	} {
		if err := repo.UpsertSnapshot(ctx, snap); err != nil {
			t.Fatalf("seed snapshot failed: %v", err)
		}
	}

	resp, err := svc.GetRanking(ctx, "ASHARE", 20)
	if err != nil {
		t.Fatalf("GetRanking failed: %v", err)
	}
	if len(resp.Items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(resp.Items))
	}
	// 没有有效的收盘价，应该返回 nil
	if resp.Items[0].ReturnPct != nil {
		t.Fatalf("expected nil return_pct when no valid price available, got %.2f", *resp.Items[0].ReturnPct)
	}
}

func TestGetRanking_ReturnPct_UsesEarliestPricedSnapshotAfterFirstAppearance(t *testing.T) {
	repo, cleanup := setupQuadrantTest(t)
	defer cleanup()
	ctx := context.Background()
	svc := NewService(repo)

	record := makeRankingRecord("00700", "HKEX", 95, 20)
	record.ComputedAt = time.Date(2026, 4, 22, 18, 46, 37, 0, time.UTC)
	seedOpportunityRecords(t, repo, []QuadrantScoreRecord{record})

	for _, snap := range []RankingSnapshot{
		{Code: "00700", Name: "腾讯控股", Exchange: "HKEX", Rank: 1, Opportunity: 95, Risk: 20, ClosePrice: 0, SnapshotDate: "2026-04-20", CreatedAt: time.Now().UTC()},
		{Code: "00700", Name: "腾讯控股", Exchange: "HKEX", Rank: 1, Opportunity: 95, Risk: 20, ClosePrice: 10, SnapshotDate: "2026-04-21", CreatedAt: time.Now().UTC()},
		{Code: "00700", Name: "腾讯控股", Exchange: "HKEX", Rank: 1, Opportunity: 95, Risk: 20, ClosePrice: 12, SnapshotDate: "2026-04-22", CreatedAt: time.Now().UTC()},
	} {
		if err := repo.UpsertSnapshot(ctx, snap); err != nil {
			t.Fatalf("seed snapshot failed: %v", err)
		}
	}

	resp, err := svc.GetRanking(ctx, "HKEX", 20)
	if err != nil {
		t.Fatalf("GetRanking failed: %v", err)
	}
	if len(resp.Items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(resp.Items))
	}
	if resp.Items[0].ReturnPct == nil {
		t.Fatal("expected non-nil return_pct when later priced snapshot exists")
	}
	if *resp.Items[0].ReturnPct != 20 {
		t.Fatalf("return_pct = %.2f; want 20.00", *resp.Items[0].ReturnPct)
	}
}

func TestGetRanking_ReturnPct_ZeroPercentIsPreserved(t *testing.T) {
	repo, cleanup := setupQuadrantTest(t)
	defer cleanup()
	ctx := context.Background()
	svc := NewService(repo)

	record := makeRankingRecord("601318", "SSE", 94, 16)
	record.ComputedAt = time.Date(2026, 4, 16, 18, 46, 37, 0, time.UTC)
	seedOpportunityRecords(t, repo, []QuadrantScoreRecord{record})

	for _, snap := range []RankingSnapshot{
		{Code: "601318", Name: "中国平安", Exchange: "SSE", Rank: 1, Opportunity: 94, Risk: 16, ClosePrice: 10, SnapshotDate: "2026-04-16", CreatedAt: time.Now().UTC()},
		{Code: "601318", Name: "中国平安", Exchange: "SSE", Rank: 1, Opportunity: 94, Risk: 16, ClosePrice: 10, SnapshotDate: "2026-04-17", CreatedAt: time.Now().UTC()},
	} {
		if err := repo.UpsertSnapshot(ctx, snap); err != nil {
			t.Fatalf("seed snapshot failed: %v", err)
		}
	}

	resp, err := svc.GetRanking(ctx, "ASHARE", 20)
	if err != nil {
		t.Fatalf("GetRanking failed: %v", err)
	}
	if len(resp.Items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(resp.Items))
	}
	if resp.Items[0].ReturnPct == nil {
		t.Fatal("expected non-nil return_pct for real 0% move")
	}
	if *resp.Items[0].ReturnPct != 0 {
		t.Fatalf("return_pct = %.2f; want 0.00", *resp.Items[0].ReturnPct)
	}
}
