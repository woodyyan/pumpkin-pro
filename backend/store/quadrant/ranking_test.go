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
		Volatility:  risk * 1.2,
		Drawdown:    risk * 0.6,
		Crowding:    risk * 0.9,
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
