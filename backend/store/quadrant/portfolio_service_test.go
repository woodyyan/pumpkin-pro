package quadrant

import (
	"context"
	"testing"
	"time"
)

func TestBuildRankingPortfolioEffectiveTime_SkipsWeekend(t *testing.T) {
	computedAt := time.Date(2026, 5, 8, 15, 0, 0, 0, rankingSnapshotLocation)
	effectiveAt := buildRankingPortfolioEffectiveTime(computedAt)
	want := time.Date(2026, 5, 11, 9, 30, 0, 0, rankingSnapshotLocation).UTC()
	if !effectiveAt.Equal(want) {
		t.Fatalf("effectiveAt = %s, want %s", effectiveAt.Format(time.RFC3339), want.Format(time.RFC3339))
	}
}

func TestSelectRankingPortfolioConstituents_ExcludesStarAndUsesTopFour(t *testing.T) {
	records := []QuadrantScoreRecord{
		makeAShareRankingRecord("688001", "STAR", "机会", 99, 88, 10, 12000),
		makeAShareRankingRecord("688002", "STAR", "机会", 98, 87, 10, 12000),
		makeAShareRankingRecord("688003", "STAR", "机会", 97, 86, 10, 12000),
		makeAShareRankingRecord("688004", "STAR", "机会", 96, 85, 10, 12000),
		makeAShareRankingRecord("600001", "MAIN", "机会", 95, 84, 10, 12000),
		makeAShareRankingRecord("000001", "MAIN", "机会", 94, 83, 10, 12000),
		makeAShareRankingRecord("300001", "CHINEXT", "机会", 93, 82, 10, 12000),
		makeAShareRankingRecord("600002", "MAIN", "机会", 92, 81, 10, 12000),
		makeAShareRankingRecord("000002", "MAIN", "机会", 91, 80, 10, 12000),
	}

	items := selectRankingPortfolioConstituents(records, 4)
	if len(items) != 4 {
		t.Fatalf("expected 4 constituents, got %d", len(items))
	}
	for _, item := range items {
		if item.Board == aShareBoardStar {
			t.Fatalf("STAR board should be excluded: %+v", item)
		}
		if item.Weight != 0.25 {
			t.Fatalf("expected equal weight 0.25, got %.4f", item.Weight)
		}
	}
	if items[0].Code != "600001" || items[3].Code != "600002" {
		t.Fatalf("unexpected selected constituents: %+v", items)
	}
}

func TestCalculateRankingPortfolioTradeRatio(t *testing.T) {
	previous := []RankingPortfolioConstituentItem{
		{Code: "600001", Exchange: "SSE", Weight: 0.25},
		{Code: "000001", Exchange: "SZSE", Weight: 0.25},
		{Code: "300001", Exchange: "SZSE", Weight: 0.25},
		{Code: "600002", Exchange: "SSE", Weight: 0.25},
	}
	current := []RankingPortfolioConstituentItem{
		{Code: "600001", Exchange: "SSE", Weight: 0.25},
		{Code: "000001", Exchange: "SZSE", Weight: 0.25},
		{Code: "300002", Exchange: "SZSE", Weight: 0.25},
		{Code: "600003", Exchange: "SSE", Weight: 0.25},
	}

	ratio := calculateRankingPortfolioTradeRatio(previous, current)
	if ratio != 1 {
		t.Fatalf("expected full traded ratio 1.0, got %.4f", ratio)
	}
}

func TestBuildRankingPortfolioConstituentItems_SelectsTop10ByStreak(t *testing.T) {
	definition := RankingPortfolioDefinition{
		Exchange:         "ASHARE",
		PortfolioVariant: rankingPortfolioVariantB,
		SelectionRule:    rankingPortfolioSelectionRuleTop10ByStreak,
		SelectionWindow:  10,
		MaxHoldings:      4,
		ExcludedBoards:   mustMarshal([]string{aShareBoardStar}),
	}
	rankingItems := []RankingItem{
		{Rank: 1, Code: "688001", Exchange: "SSE", Board: aShareBoardStar, ConsecutiveDays: 15},
		{Rank: 2, Code: "600001", Exchange: "SSE", Board: aShareBoardMain, ConsecutiveDays: 6},
		{Rank: 3, Code: "000001", Exchange: "SZSE", Board: aShareBoardMain, ConsecutiveDays: 9},
		{Rank: 4, Code: "300001", Exchange: "SZSE", Board: aShareBoardChiNext, ConsecutiveDays: 5},
		{Rank: 5, Code: "600002", Exchange: "SSE", Board: aShareBoardMain, ConsecutiveDays: 9},
		{Rank: 6, Code: "000002", Exchange: "SZSE", Board: aShareBoardMain, ConsecutiveDays: 4},
		{Rank: 7, Code: "600003", Exchange: "SSE", Board: aShareBoardMain, ConsecutiveDays: 8},
		{Rank: 8, Code: "000003", Exchange: "SZSE", Board: aShareBoardMain, ConsecutiveDays: 3},
	}

	items := buildRankingPortfolioConstituentItems(definition, rankingItems)
	if len(items) != 4 {
		t.Fatalf("expected 4 constituents, got %d", len(items))
	}
	gotCodes := []string{items[0].Code, items[1].Code, items[2].Code, items[3].Code}
	wantCodes := []string{"000001", "600002", "600003", "600001"}
	for i := range wantCodes {
		if gotCodes[i] != wantCodes[i] {
			t.Fatalf("unexpected selected order: got %v want %v", gotCodes, wantCodes)
		}
	}
	if items[0].SourceRank != 3 || items[1].SourceRank != 5 {
		t.Fatalf("expected source ranks from ranking list, got %+v", items)
	}
}

func TestSaveAndGetRankingPortfolio(t *testing.T) {
	repo, cleanup := setupQuadrantTest(t)
	defer cleanup()
	svc := NewService(repo)
	ctx := context.Background()

	benchmarkPrices := map[string]float64{
		"2026-05-06": 3000,
		"2026-05-07": 3030,
	}
	svc.SetBenchmarkPriceResolver(func(ctx context.Context, benchmark string, tradeDate string) (float64, string) {
		return benchmarkPrices[tradeDate], tradeDate
	})

	priceMap := map[string]float64{
		snapshotPriceHintKey("600001", "SSE") + "@2026-05-06":  10,
		snapshotPriceHintKey("000001", "SZSE") + "@2026-05-06": 20,
		snapshotPriceHintKey("300001", "SZSE") + "@2026-05-06": 30,
		snapshotPriceHintKey("600002", "SSE") + "@2026-05-06":  40,
		snapshotPriceHintKey("600001", "SSE") + "@2026-05-07":  11,
		snapshotPriceHintKey("000001", "SZSE") + "@2026-05-07": 21,
		snapshotPriceHintKey("300001", "SZSE") + "@2026-05-07": 33,
		snapshotPriceHintKey("600002", "SSE") + "@2026-05-07":  44,
		snapshotPriceHintKey("600003", "SSE") + "@2026-05-07":  50,
		snapshotPriceHintKey("300002", "SZSE") + "@2026-05-07": 60,
	}
	svc.SetPriceResolver(func(ctx context.Context, code string, exchange string, tradeDate string) float64 {
		return priceMap[snapshotPriceHintKey(code, exchange)+"@"+tradeDate]
	})

	recordsDay1 := []QuadrantScoreRecord{
		makeAShareRankingRecord("688001", "STAR", "机会", 99, 88, 10, 12000),
		makeAShareRankingRecord("600001", "MAIN", "机会", 95, 84, 10, 12000),
		makeAShareRankingRecord("000001", "MAIN", "机会", 94, 83, 10, 12000),
		makeAShareRankingRecord("300001", "CHINEXT", "机会", 93, 82, 10, 12000),
		makeAShareRankingRecord("600002", "MAIN", "机会", 92, 81, 10, 12000),
	}
	recordsDay2 := []QuadrantScoreRecord{
		makeAShareRankingRecord("688001", "STAR", "机会", 99, 88, 10, 12000),
		makeAShareRankingRecord("600003", "MAIN", "机会", 97, 86, 10, 12000),
		makeAShareRankingRecord("300002", "CHINEXT", "机会", 96, 85, 10, 12000),
		makeAShareRankingRecord("600001", "MAIN", "机会", 95, 84, 10, 12000),
		makeAShareRankingRecord("000001", "MAIN", "机会", 94, 83, 10, 12000),
	}

	if err := svc.saveRankingPortfolio(ctx, recordsDay1, time.Date(2026, 5, 6, 15, 0, 0, 0, rankingSnapshotLocation), nil); err != nil {
		t.Fatalf("save day1 portfolio failed: %v", err)
	}
	if err := svc.saveRankingPortfolio(ctx, recordsDay2, time.Date(2026, 5, 7, 15, 0, 0, 0, rankingSnapshotLocation), nil); err != nil {
		t.Fatalf("save day2 portfolio failed: %v", err)
	}

	resp, err := svc.GetRankingPortfolio(ctx)
	if err != nil {
		t.Fatalf("GetRankingPortfolio failed: %v", err)
	}
	if len(resp.Items) != 4 {
		t.Fatalf("expected 4 portfolio responses, got %d", len(resp.Items))
	}

	itemsByID := map[string]RankingPortfolioResponse{}
	for _, item := range resp.Items {
		itemsByID[item.Meta.DefinitionID] = item
	}

	aResp, ok := itemsByID[defaultRankingPortfolioDefinitionID]
	if !ok {
		t.Fatalf("missing default A-share portfolio response: %+v", resp.Items)
	}
	if aResp.Meta.SnapshotVersion != "2026-05-07" {
		t.Fatalf("expected latest snapshot version 2026-05-07, got %s", aResp.Meta.SnapshotVersion)
	}
	if aResp.Meta.SourceTradeDate != "2026-04-14" {
		t.Fatalf("source_trade_date = %s, want 2026-04-14", aResp.Meta.SourceTradeDate)
	}
	if len(aResp.Series) != 2 {
		t.Fatalf("expected 2 series points, got %d", len(aResp.Series))
	}
	if aResp.Series[1].PortfolioReturnPct <= 0 {
		t.Fatalf("expected positive portfolio return, got %+v", aResp.Series[1])
	}
	if aResp.Series[1].ExcessReturnPct <= 0 {
		t.Fatalf("expected positive excess return, got %+v", aResp.Series[1])
	}
	if len(aResp.Constituents) != 4 {
		t.Fatalf("expected 4 latest constituents, got %d", len(aResp.Constituents))
	}
	if aResp.Meta.BatchID == "" {
		t.Fatalf("expected batch id, got %+v", aResp.Meta)
	}
	if aResp.Meta.MethodNote != "" {
		t.Fatalf("expected method note to be omitted from response, got %q", aResp.Meta.MethodNote)
	}
	bResp, ok := itemsByID["wolong_ai_top10_ex_star_by_streak_v1"]
	if !ok {
		t.Fatalf("missing A-share B portfolio response: %+v", resp.Items)
	}
	if bResp.Meta.PortfolioVariant != rankingPortfolioVariantB || bResp.Meta.SelectionWindow != 10 {
		t.Fatalf("unexpected B meta: %+v", bResp.Meta)
	}
	if len(bResp.Constituents) != 4 {
		t.Fatalf("expected 4 B constituents, got %d", len(bResp.Constituents))
	}
	if bResp.Constituents[0].Code != "600003" || bResp.Constituents[0].SourceRank != 2 {
		t.Fatalf("unexpected B leading constituent: %+v", bResp.Constituents)
	}

	wantEffectiveTime := time.Date(2026, 5, 8, 9, 30, 0, 0, rankingSnapshotLocation).UTC().Format(time.RFC3339)
	if aResp.Meta.HoldingsEffectiveTime != wantEffectiveTime {
		t.Fatalf("holdings_effective_time = %s, want %s", aResp.Meta.HoldingsEffectiveTime, wantEffectiveTime)
	}
	if aResp.LatestRebalance == nil {
		t.Fatal("expected latest rebalance payload")
	}
	if aResp.LatestRebalance.EffectiveTime != wantEffectiveTime {
		t.Fatalf("latest rebalance effective_time = %s, want %s", aResp.LatestRebalance.EffectiveTime, wantEffectiveTime)
	}
	if aResp.LatestRebalance.ChangeCount != 4 || len(aResp.LatestRebalance.Items) != 4 {
		t.Fatalf("expected 4 rebalance items, got %+v", aResp.LatestRebalance)
	}
	itemsByCode := map[string]RankingPortfolioRebalanceItem{}
	for _, item := range aResp.LatestRebalance.Items {
		itemsByCode[item.Code] = item
	}
	if item := itemsByCode["600002"]; item.Action != "sell" || item.FromWeight != 0.25 || item.ToWeight != 0 || item.ReferencePrice != 44 || item.ReferenceCostPrice != 43.9912 {
		t.Fatalf("unexpected sell rebalance item: %+v", item)
	}
	if item := itemsByCode["300001"]; item.Action != "sell" || item.FromWeight != 0.25 || item.ToWeight != 0 || item.ReferencePrice != 33 || item.ReferenceCostPrice != 32.9934 {
		t.Fatalf("unexpected sell rebalance item: %+v", item)
	}
	if item := itemsByCode["600003"]; item.Action != "buy" || item.FromWeight != 0 || item.ToWeight != 0.25 || item.ReferencePrice != 50 || item.ReferenceCostPrice != 50.01 {
		t.Fatalf("unexpected buy rebalance item: %+v", item)
	}
	if item := itemsByCode["300002"]; item.Action != "buy" || item.FromWeight != 0 || item.ToWeight != 0.25 || item.ReferencePrice != 60 || item.ReferenceCostPrice != 60.012 {
		t.Fatalf("unexpected buy rebalance item: %+v", item)
	}
}

func TestSaveRankingPortfolio_RebuildsSameSnapshotVersion(t *testing.T) {
	repo, cleanup := setupQuadrantTest(t)
	defer cleanup()
	svc := NewService(repo)
	ctx := context.Background()

	benchmarkPrices := map[string]float64{"2026-05-06": 3000}
	svc.SetBenchmarkPriceResolver(func(ctx context.Context, benchmark string, tradeDate string) (float64, string) {
		return benchmarkPrices[tradeDate], tradeDate
	})

	priceMap := map[string]float64{
		snapshotPriceHintKey("600001", "SSE") + "@2026-05-06":  10,
		snapshotPriceHintKey("000001", "SZSE") + "@2026-05-06": 20,
		snapshotPriceHintKey("300001", "SZSE") + "@2026-05-06": 30,
		snapshotPriceHintKey("600002", "SSE") + "@2026-05-06":  40,
	}
	svc.SetPriceResolver(func(ctx context.Context, code string, exchange string, tradeDate string) float64 {
		return priceMap[snapshotPriceHintKey(code, exchange)+"@"+tradeDate]
	})

	computedAt := time.Date(2026, 5, 6, 15, 0, 0, 0, rankingSnapshotLocation)
	badRecords := []QuadrantScoreRecord{
		makeAShareRankingRecord("688001", "STAR", "机会", 99, 88, 10, 12000),
		makeAShareRankingRecord("688002", "STAR", "机会", 98, 87, 10, 12000),
	}
	goodRecords := []QuadrantScoreRecord{
		makeAShareRankingRecord("688001", "STAR", "机会", 99, 88, 10, 12000),
		makeAShareRankingRecord("600001", "MAIN", "机会", 95, 84, 10, 12000),
		makeAShareRankingRecord("000001", "MAIN", "机会", 94, 83, 10, 12000),
		makeAShareRankingRecord("300001", "CHINEXT", "机会", 93, 82, 10, 12000),
		makeAShareRankingRecord("600002", "MAIN", "机会", 92, 81, 10, 12000),
	}

	if err := svc.saveRankingPortfolio(ctx, badRecords, computedAt, nil); err != nil {
		t.Fatalf("first save failed: %v", err)
	}
	if err := svc.saveRankingPortfolio(ctx, goodRecords, computedAt, nil); err != nil {
		t.Fatalf("second save failed: %v", err)
	}

	var snapshots []RankingPortfolioSnapshot
	if err := repo.db.WithContext(ctx).
		Where("definition_id = ?", defaultRankingPortfolioDefinitionID).
		Order("id ASC").
		Find(&snapshots).Error; err != nil {
		t.Fatalf("load snapshots failed: %v", err)
	}
	if len(snapshots) != 1 {
		t.Fatalf("expected 1 rebuilt snapshot, got %d", len(snapshots))
	}
	if snapshots[0].ConstituentsCount != 4 || snapshots[0].HasShortfall {
		t.Fatalf("expected rebuilt snapshot with 4 constituents, got %+v", snapshots[0])
	}

	var constituents []RankingPortfolioSnapshotConstituent
	if err := repo.db.WithContext(ctx).
		Where("definition_id = ? AND snapshot_version = ?", defaultRankingPortfolioDefinitionID, "2026-05-06").
		Order("rank ASC").
		Find(&constituents).Error; err != nil {
		t.Fatalf("load constituents failed: %v", err)
	}
	if len(constituents) != 4 {
		t.Fatalf("expected 4 rebuilt constituents, got %d", len(constituents))
	}

	var result RankingPortfolioResult
	if err := repo.db.WithContext(ctx).
		Where("definition_id = ? AND snapshot_version = ?", defaultRankingPortfolioDefinitionID, "2026-05-06").
		First(&result).Error; err != nil {
		t.Fatalf("load result failed: %v", err)
	}
	if result.CurrentConstituentCount != 4 || result.HasShortfall {
		t.Fatalf("expected rebuilt result to expose 4 constituents, got %+v", result)
	}
}

func TestGetRankingPortfolio_UsesLatestRankingForCurrentConstituents(t *testing.T) {
	repo, cleanup := setupQuadrantTest(t)
	defer cleanup()
	svc := NewService(repo)
	ctx := context.Background()

	svc.SetBenchmarkPriceResolver(func(ctx context.Context, benchmark string, tradeDate string) (float64, string) {
		return 3000, tradeDate
	})
	svc.SetPriceResolver(func(ctx context.Context, code string, exchange string, tradeDate string) float64 {
		return 10
	})

	storedRecords := []QuadrantScoreRecord{
		makeAShareRankingRecord("000988", "MAIN", "机会", 97, 90, 15, 12000),
		makeAShareRankingRecord("601991", "MAIN", "机会", 96, 89, 16, 12000),
		makeAShareRankingRecord("001309", "MAIN", "机会", 95, 88, 17, 12000),
		makeAShareRankingRecord("002384", "MAIN", "机会", 94, 87, 18, 12000),
	}
	storedComputedAt := time.Date(2026, 5, 20, 15, 0, 0, 0, rankingSnapshotLocation)
	for i := range storedRecords {
		storedRecords[i].SourceTradeDate = "2026-05-19"
	}
	if err := svc.saveRankingPortfolio(ctx, storedRecords, storedComputedAt, nil); err != nil {
		t.Fatalf("save stored portfolio failed: %v", err)
	}

	latestComputedAt := time.Date(2026, 5, 22, 2, 0, 0, 0, rankingSnapshotLocation).UTC()
	latestRecords := []QuadrantScoreRecord{
		makeAShareRankingRecord("688802", "STAR", "机会", 99, 98, 10, 15000),
		makeAShareRankingRecord("000988", "MAIN", "机会", 98, 97, 20, 15000),
		makeAShareRankingRecord("600522", "MAIN", "机会", 97, 96, 21, 15000),
		makeAShareRankingRecord("600584", "MAIN", "机会", 96, 95, 22, 15000),
		makeAShareRankingRecord("600487", "MAIN", "机会", 95, 94, 23, 15000),
	}
	for i := range latestRecords {
		latestRecords[i].ComputedAt = latestComputedAt
		latestRecords[i].SourceTradeDate = "2026-05-21"
	}
	seedOpportunityRecords(t, repo, latestRecords)

	resp, err := svc.GetRankingPortfolio(ctx)
	if err != nil {
		t.Fatalf("GetRankingPortfolio failed: %v", err)
	}

	var aResp *RankingPortfolioResponse
	for i := range resp.Items {
		if resp.Items[i].Meta.DefinitionID == defaultRankingPortfolioDefinitionID {
			aResp = &resp.Items[i]
			break
		}
	}
	if aResp == nil {
		t.Fatalf("missing A-share portfolio response: %+v", resp.Items)
	}

	if aResp.Meta.SnapshotVersion != "2026-05-20" {
		t.Fatalf("snapshot_version = %s, want 2026-05-20", aResp.Meta.SnapshotVersion)
	}
	if aResp.Meta.SourceTradeDate != "2026-05-19" {
		t.Fatalf("source_trade_date = %s, want 2026-05-19", aResp.Meta.SourceTradeDate)
	}
	if len(aResp.Constituents) != 4 {
		t.Fatalf("expected 4 current constituents, got %d", len(aResp.Constituents))
	}
	gotCodes := []string{aResp.Constituents[0].Code, aResp.Constituents[1].Code, aResp.Constituents[2].Code, aResp.Constituents[3].Code}
	wantCodes := []string{"000988", "600522", "600584", "600487"}
	for i := range wantCodes {
		if gotCodes[i] != wantCodes[i] {
			t.Fatalf("current constituents = %v, want %v", gotCodes, wantCodes)
		}
	}
	if aResp.Meta.CurrentConstituentCount != 4 {
		t.Fatalf("current_constituent_count = %d, want 4", aResp.Meta.CurrentConstituentCount)
	}
	if aResp.Meta.CurrentConstituentSourceDate != "2026-05-21" {
		t.Fatalf("current_constituent_source_date = %s, want 2026-05-21", aResp.Meta.CurrentConstituentSourceDate)
	}
	wantEffectiveTime := time.Date(2026, 5, 22, 9, 30, 0, 0, rankingSnapshotLocation).UTC().Format(time.RFC3339)
	if aResp.Meta.CurrentConstituentEffectiveTime != wantEffectiveTime {
		t.Fatalf("current_constituent_effective_time = %s, want %s", aResp.Meta.CurrentConstituentEffectiveTime, wantEffectiveTime)
	}
	if aResp.LatestRebalance != nil {
		t.Fatalf("expected latest rebalance to be hidden when current ranking is newer, got %+v", aResp.LatestRebalance)
	}
}
