package quadrant

import (
	"context"
	"encoding/json"
	"fmt"
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
		Code:       code,
		Name:       "股票" + code,
		Exchange:   exchange,
		Opportunity: 30,
		Risk:       70,
		Quadrant:   quadrant,
		Trend:      25,
		Flow:       20,
		Revision:   18,
		Liquidity:  40,
		AvgAmount5d: 5000.0,
		ComputedAt: time.Date(2026, 4, 15, 2, 30, 0, 0, time.UTC),
	}
}

// padCode returns a zero-padded string of length digits.
func padCode(n int, digits int) string {
	return fmt.Sprintf("%0*dd", digits, n)
}

// ── T-R1: Model / Struct Tests ──

func TestRankingItem_JSONRoundTrip(t *testing.T) {
	item := RankingItem{
		Rank:        1,
		Code:        "600519",
		Name:        "贵州茅台",
		Exchange:    "SSE",
		Opportunity: 96.5,
		Risk:        22.3,
		Quadrant:    "机会",
		Trend:       94.2,
		Flow:        88.7,
		Revision:    85.1,
		Liquidity:   92.0,
		AvgAmount5d: 150000.0,
	}

	data, err := json.Marshal(item)
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}

	var got RankingItem
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}

	if got.Rank != item.Rank || got.Code != item.Code || got.Opportunity != item.Opportunity {
		t.Errorf("roundtrip mismatch: got %+v", got)
	}

	// Verify JSON keys are correct (not Go struct names)
	var raw map[string]any
	json.Unmarshal(data, &raw)
	expectedKeys := []string{"rank", "code", "name", "exchange", "opportunity", "risk", "quadrant", "trend", "flow", "revision"}
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
		makeRankingRecord("600000", "SSE", 95, 35), // higher risk → rank lower
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
		makeRankingRecord("600519", "SSE", 99, 5),   // high opportunity but wrong exchange!
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
				Volatility: 30, Drawdown: 15, Crowding: 40, Exchange: "SSE"},
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
		makeHighLiquidityRecord("600519", "SSE", 98),   // passes filter
		makeHighLiquidityRecord("000001", "SZSE", 95),   // passes filter
		makeLowLiquidityRecord("300123", "SZSE", 99),    // high opp but illiquid → excluded!
		makeLowLiquidityRecord("600888", "SSE", 96),     // excluded
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
