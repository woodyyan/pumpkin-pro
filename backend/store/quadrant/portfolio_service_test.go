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

	priceMap := map[string]float64{
		snapshotPriceHintKey("600001", "SSE") + "@2026-04-14":  10,
		snapshotPriceHintKey("000001", "SZSE") + "@2026-04-14": 20,
		snapshotPriceHintKey("300001", "SZSE") + "@2026-04-14": 30,
		snapshotPriceHintKey("600002", "SSE") + "@2026-04-14":  40,
		snapshotPriceHintKey("600003", "SSE") + "@2026-04-14":  45,
		snapshotPriceHintKey("300002", "SZSE") + "@2026-04-14": 55,
		snapshotPriceHintKey("600001", "SSE") + "@2026-05-06":  10,
		snapshotPriceHintKey("000001", "SZSE") + "@2026-05-06": 20,
		snapshotPriceHintKey("300001", "SZSE") + "@2026-05-06": 30,
		snapshotPriceHintKey("600002", "SSE") + "@2026-05-06":  40,
		snapshotPriceHintKey("600003", "SSE") + "@2026-05-06":  45,
		snapshotPriceHintKey("300002", "SZSE") + "@2026-05-06": 55,
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

	if err := svc.saveRankingPortfolio(ctx, recordsDay1, time.Date(2026, 5, 6, 15, 0, 0, 0, rankingSnapshotLocation), nil, ""); err != nil {
		t.Fatalf("save day1 portfolio failed: %v", err)
	}
	if err := svc.saveRankingPortfolio(ctx, recordsDay2, time.Date(2026, 5, 7, 15, 0, 0, 0, rankingSnapshotLocation), nil, ""); err != nil {
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
	if aResp.Series[1].PortfolioReturnPct >= 0 {
		t.Fatalf("expected negative portfolio return after trade cost, got %+v", aResp.Series[1])
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
	if item := itemsByCode["600002"]; item.Action != "sell" || item.FromWeight != 0.25 || item.ToWeight != 0 || item.ReferencePrice != 40 || item.ReferenceCostPrice != 39.992 {
		t.Fatalf("unexpected sell rebalance item: %+v", item)
	}
	if item := itemsByCode["300001"]; item.Action != "sell" || item.FromWeight != 0.25 || item.ToWeight != 0 || item.ReferencePrice != 30 || item.ReferenceCostPrice != 29.994 {
		t.Fatalf("unexpected sell rebalance item: %+v", item)
	}
	if item := itemsByCode["600003"]; item.Action != "buy" || item.FromWeight != 0 || item.ToWeight != 0.25 || item.ReferencePrice != 45 || item.ReferenceCostPrice != 45.009 {
		t.Fatalf("unexpected buy rebalance item: %+v", item)
	}
	if item := itemsByCode["300002"]; item.Action != "buy" || item.FromWeight != 0 || item.ToWeight != 0.25 || item.ReferencePrice != 55 || item.ReferenceCostPrice != 55.011 {
		t.Fatalf("unexpected buy rebalance item: %+v", item)
	}
}

func TestBuildRankingPortfolioSummaryMetrics(t *testing.T) {
	series := []RankingPortfolioSeriesPoint{
		{Date: "2026-05-29", SourceTradeDate: "2026-05-29", Nav: 1, PortfolioReturnPct: 0, DailyPortfolioReturnPct: 0, DrawdownPct: 0},
		{Date: "2026-06-02", SourceTradeDate: "2026-06-02", Nav: 1.03, PortfolioReturnPct: 3, DailyPortfolioReturnPct: 3, DrawdownPct: 0},
		{Date: "2026-06-03", SourceTradeDate: "2026-06-03", Nav: 1.01, PortfolioReturnPct: 1, DailyPortfolioReturnPct: -1.9417, DrawdownPct: -1.9417},
		{Date: "2026-06-04", SourceTradeDate: "2026-06-04", Nav: 1.06, PortfolioReturnPct: 6, DailyPortfolioReturnPct: 4.9505, DrawdownPct: 0},
	}

	metrics := buildRankingPortfolioSummaryMetrics(series)
	if metrics.InceptionTradeDate != "2026-05-29" {
		t.Fatalf("inception_trade_date = %s, want 2026-05-29", metrics.InceptionTradeDate)
	}
	if metrics.InceptionDays != 7 {
		t.Fatalf("inception_days = %d, want 7", metrics.InceptionDays)
	}
	if metrics.LatestDailyReturnPct == nil || *metrics.LatestDailyReturnPct != 4.9505 {
		t.Fatalf("latest_daily_return_pct = %v, want 4.9505", metrics.LatestDailyReturnPct)
	}
	if metrics.CurrentMonthReturnPct == nil || *metrics.CurrentMonthReturnPct != 6 {
		t.Fatalf("current_month_return_pct = %v, want 6", metrics.CurrentMonthReturnPct)
	}
	if metrics.MaxDrawdownPct == nil || *metrics.MaxDrawdownPct != -1.9417 {
		t.Fatalf("max_drawdown_pct = %v, want -1.9417", metrics.MaxDrawdownPct)
	}
	if metrics.DailyWinRatePct == nil || *metrics.DailyWinRatePct != 66.6667 {
		t.Fatalf("daily_win_rate_pct = %v, want 66.6667", metrics.DailyWinRatePct)
	}
	if metrics.VolatilityPct == nil || *metrics.VolatilityPct <= 0 {
		t.Fatalf("volatility_pct should be positive, got %v", metrics.VolatilityPct)
	}
}

func TestEnrichRankingPortfolioCurrentConstituents_UsesConsecutiveEntryTradeDate(t *testing.T) {
	repo, cleanup := setupQuadrantTest(t)
	defer cleanup()
	svc := NewService(repo)
	ctx := context.Background()
	now := time.Now().UTC()

	snapshots := []RankingPortfolioSnapshot{
		{DefinitionID: defaultRankingPortfolioDefinitionID, SnapshotVersion: "2026-05-06", SnapshotDate: "2026-05-06", SourceTradeDate: "2026-05-06", CreatedAt: now, UpdatedAt: now},
		{DefinitionID: defaultRankingPortfolioDefinitionID, SnapshotVersion: "2026-05-07", SnapshotDate: "2026-05-07", SourceTradeDate: "2026-05-07", CreatedAt: now, UpdatedAt: now},
		{DefinitionID: defaultRankingPortfolioDefinitionID, SnapshotVersion: "2026-05-08", SnapshotDate: "2026-05-08", SourceTradeDate: "2026-05-08", CreatedAt: now, UpdatedAt: now},
	}
	if err := repo.db.WithContext(ctx).Create(&snapshots).Error; err != nil {
		t.Fatalf("seed snapshots failed: %v", err)
	}

	rows := []RankingPortfolioSnapshotConstituent{
		{DefinitionID: defaultRankingPortfolioDefinitionID, SnapshotVersion: "2026-05-06", SnapshotDate: "2026-05-06", Code: "600001", Exchange: "SSE", Rank: 1, Weight: 0.25, CreatedAt: now, UpdatedAt: now},
		{DefinitionID: defaultRankingPortfolioDefinitionID, SnapshotVersion: "2026-05-07", SnapshotDate: "2026-05-07", Code: "600001", Exchange: "SSE", Rank: 1, Weight: 0.25, CreatedAt: now, UpdatedAt: now},
		{DefinitionID: defaultRankingPortfolioDefinitionID, SnapshotVersion: "2026-05-08", SnapshotDate: "2026-05-08", Code: "600001", Exchange: "SSE", Rank: 1, Weight: 0.25, CreatedAt: now, UpdatedAt: now},
	}
	if err := repo.db.WithContext(ctx).Create(&rows).Error; err != nil {
		t.Fatalf("seed constituents failed: %v", err)
	}

	rankingRows := []RankingSnapshot{
		{Code: "600001", Name: "股票600001", Exchange: "SSE", ClosePrice: 10, PriceTradeDate: "2026-05-06", SnapshotDate: "2026-05-06", CreatedAt: now},
		{Code: "600001", Name: "股票600001", Exchange: "SSE", ClosePrice: 12, PriceTradeDate: "2026-05-08", SnapshotDate: "2026-05-08", CreatedAt: now},
	}
	if err := repo.db.WithContext(ctx).Create(&rankingRows).Error; err != nil {
		t.Fatalf("seed ranking snapshots failed: %v", err)
	}

	items := []RankingPortfolioConstituentItem{{Code: "600001", Exchange: "SSE"}}
	definition := RankingPortfolioDefinition{ID: defaultRankingPortfolioDefinitionID}
	if err := svc.enrichRankingPortfolioCurrentConstituents(ctx, definition, items, "2026-05-08"); err != nil {
		t.Fatalf("enrich constituents failed: %v", err)
	}
	if items[0].EntryTradeDate != "2026-05-06" {
		t.Fatalf("entry_trade_date = %s, want 2026-05-06", items[0].EntryTradeDate)
	}
	if items[0].EntryPrice != 10 {
		t.Fatalf("entry_price = %v, want 10", items[0].EntryPrice)
	}
	if items[0].LatestTradeDate != "2026-05-08" {
		t.Fatalf("latest_trade_date = %s, want 2026-05-08", items[0].LatestTradeDate)
	}
	if items[0].LatestClosePrice != 12 {
		t.Fatalf("latest_close_price = %v, want 12", items[0].LatestClosePrice)
	}
	if items[0].LatestReturnPct == nil || *items[0].LatestReturnPct != 20 {
		t.Fatalf("latest_return_pct = %v, want 20", items[0].LatestReturnPct)
	}
}

func TestSaveRankingPortfolio_RebuildsSameSnapshotVersion(t *testing.T) {
	repo, cleanup := setupQuadrantTest(t)
	defer cleanup()
	svc := NewService(repo)
	ctx := context.Background()

	priceMap := map[string]float64{
		snapshotPriceHintKey("600001", "SSE") + "@2026-04-14":  10,
		snapshotPriceHintKey("000001", "SZSE") + "@2026-04-14": 20,
		snapshotPriceHintKey("300001", "SZSE") + "@2026-04-14": 30,
		snapshotPriceHintKey("600002", "SSE") + "@2026-04-14":  40,
		snapshotPriceHintKey("600003", "SSE") + "@2026-04-14":  45,
		snapshotPriceHintKey("300002", "SZSE") + "@2026-04-14": 55,
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
	if err := svc.saveRankingPortfolio(ctx, recordsDay1, time.Date(2026, 5, 6, 15, 0, 0, 0, rankingSnapshotLocation), nil, ""); err != nil {
		t.Fatalf("save day1 portfolio failed: %v", err)
	}
	if err := svc.saveRankingPortfolio(ctx, recordsDay2, time.Date(2026, 5, 7, 15, 0, 0, 0, rankingSnapshotLocation), nil, ""); err != nil {
		t.Fatalf("save day2 portfolio failed: %v", err)
	}

	if err := repo.db.WithContext(ctx).Where("definition_id = ? AND snapshot_date = ?", defaultRankingPortfolioDefinitionID, "2026-05-07").Delete(&RankingPortfolioResult{}).Error; err != nil {
		t.Fatalf("delete lagging result failed: %v", err)
	}
	if err := repo.db.WithContext(ctx).Create(&RankingSnapshot{Code: "600003", Name: "股票600003", Exchange: "SSE", Rank: 1, Opportunity: 97, Risk: 86, ClosePrice: 45, PriceTradeDate: "2026-04-14", SnapshotDate: "2026-05-07", CreatedAt: time.Now().UTC()}).Error; err != nil {
		t.Fatalf("seed ranking snapshot failed: %v", err)
	}

	if err := svc.RebuildLaggingRankingPortfolioResults(ctx, "manual-repair", false); err != nil {
		t.Fatalf("rebuild lagging results failed: %v", err)
	}

	result, err := repo.GetLatestRankingPortfolioResultByDefinition(ctx, defaultRankingPortfolioDefinitionID)
	if err != nil {
		t.Fatalf("load rebuilt result failed: %v", err)
	}
	if result == nil || result.SnapshotDate != "2026-05-07" {
		t.Fatalf("expected rebuilt latest result on 2026-05-07, got %+v", result)
	}
	if result.SourceTradeDate != "2026-04-14" {
		t.Fatalf("rebuilt source_trade_date = %s, want 2026-04-14", result.SourceTradeDate)
	}
	status, err := repo.ListLatestRankingPortfolioJobStatuses(ctx)
	if err != nil {
		t.Fatalf("list statuses failed: %v", err)
	}
	var found bool
	for _, item := range status {
		if item.TaskLogID == "manual-repair" && item.DefinitionID == defaultRankingPortfolioDefinitionID {
			found = true
			if item.Status != "success" || item.SnapshotDate != "2026-05-07" {
				t.Fatalf("unexpected repair status: %+v", item)
			}
		}
	}
	if !found {
		t.Fatal("expected manual repair status for default definition")
	}
}

func TestRebuildLaggingRankingPortfolioResults_RebuildsMissingPortfolioSnapshotFromRankingSnapshot(t *testing.T) {
	repo, cleanup := setupQuadrantTest(t)
	defer cleanup()
	svc := NewService(repo)
	ctx := context.Background()

	priceMap := map[string]float64{
		snapshotPriceHintKey("600001", "SSE") + "@2026-04-14":  10,
		snapshotPriceHintKey("000001", "SZSE") + "@2026-04-14": 20,
		snapshotPriceHintKey("300001", "SZSE") + "@2026-04-14": 30,
		snapshotPriceHintKey("600002", "SSE") + "@2026-04-14":  40,
	}
	svc.SetPriceResolver(func(ctx context.Context, code string, exchange string, tradeDate string) float64 {
		return priceMap[snapshotPriceHintKey(code, exchange)+"@"+tradeDate]
	})

	now := time.Date(2026, 5, 8, 15, 0, 0, 0, rankingSnapshotLocation).UTC()
	rows := []RankingSnapshot{
		{Code: "600001", Name: "股票600001", Exchange: "SSE", Rank: 1, Opportunity: 95, Risk: 84, ClosePrice: 10, PriceTradeDate: "2026-04-14", SnapshotDate: "2026-05-08", CreatedAt: now},
		{Code: "000001", Name: "股票000001", Exchange: "SZSE", Rank: 2, Opportunity: 94, Risk: 83, ClosePrice: 20, PriceTradeDate: "2026-04-14", SnapshotDate: "2026-05-08", CreatedAt: now},
		{Code: "300001", Name: "股票300001", Exchange: "SZSE", Rank: 3, Opportunity: 93, Risk: 82, ClosePrice: 30, PriceTradeDate: "2026-04-14", SnapshotDate: "2026-05-08", CreatedAt: now},
		{Code: "600002", Name: "股票600002", Exchange: "SSE", Rank: 4, Opportunity: 92, Risk: 81, ClosePrice: 40, PriceTradeDate: "2026-04-14", SnapshotDate: "2026-05-08", CreatedAt: now},
	}
	if err := repo.db.WithContext(ctx).Create(&rows).Error; err != nil {
		t.Fatalf("seed ranking snapshots failed: %v", err)
	}
	if err := svc.RebuildLaggingRankingPortfolioResults(ctx, "manual-repair", false); err != nil {
		t.Fatalf("rebuild missing portfolio snapshot failed: %v", err)
	}
	result, err := repo.GetLatestRankingPortfolioResultByDefinition(ctx, defaultRankingPortfolioDefinitionID)
	if err != nil {
		t.Fatalf("load rebuilt result failed: %v", err)
	}
	if result == nil || result.SnapshotDate != "2026-05-08" {
		t.Fatalf("expected rebuilt result at 2026-05-08, got %+v", result)
	}
	if result.SourceTradeDate != "2026-04-14" {
		t.Fatalf("source_trade_date = %s, want 2026-04-14", result.SourceTradeDate)
	}
	var snapshot RankingPortfolioSnapshot
	if err := repo.db.WithContext(ctx).Where("definition_id = ? AND snapshot_date = ?", defaultRankingPortfolioDefinitionID, "2026-05-08").First(&snapshot).Error; err != nil {
		t.Fatalf("expected rebuilt portfolio snapshot: %v", err)
	}
	var constituents []RankingPortfolioSnapshotConstituent
	if err := repo.db.WithContext(ctx).Where("definition_id = ? AND snapshot_version = ?", defaultRankingPortfolioDefinitionID, snapshot.SnapshotVersion).Find(&constituents).Error; err != nil {
		t.Fatalf("load rebuilt constituents failed: %v", err)
	}
	if len(constituents) != 4 {
		t.Fatalf("expected 4 rebuilt constituents, got %d", len(constituents))
	}
}

func TestRebuildLaggingRankingPortfolioResults_BackfillsMissingMarketCloseFromHistoricalSnapshot(t *testing.T) {
	repo, cleanup := setupQuadrantTest(t)
	defer cleanup()
	svc := NewService(repo)
	ctx := context.Background()

	svc.SetPriceResolver(func(ctx context.Context, code string, exchange string, tradeDate string) float64 {
		return 0
	})

	seedAt := time.Date(2026, 5, 28, 15, 0, 0, 0, rankingSnapshotLocation).UTC()
	rows := []RankingSnapshot{
		{Code: "000725", Name: "京东方A", Exchange: "SZSE", Rank: 1, Opportunity: 95, Risk: 84, ClosePrice: 10.5, PriceTradeDate: "2026-05-27", SnapshotDate: "2026-05-27", CreatedAt: seedAt},
		{Code: "600001", Name: "股票600001", Exchange: "SSE", Rank: 2, Opportunity: 94, Risk: 83, ClosePrice: 20, PriceTradeDate: "2026-05-28", SnapshotDate: "2026-05-28", CreatedAt: seedAt},
		{Code: "000001", Name: "股票000001", Exchange: "SZSE", Rank: 3, Opportunity: 93, Risk: 82, ClosePrice: 30, PriceTradeDate: "2026-05-28", SnapshotDate: "2026-05-28", CreatedAt: seedAt},
		{Code: "300001", Name: "股票300001", Exchange: "SZSE", Rank: 4, Opportunity: 92, Risk: 81, ClosePrice: 40, PriceTradeDate: "2026-05-28", SnapshotDate: "2026-05-28", CreatedAt: seedAt},
		{Code: "000725", Name: "京东方A", Exchange: "SZSE", Rank: 5, Opportunity: 91, Risk: 80, ClosePrice: 0, PriceTradeDate: "2026-05-28", SnapshotDate: "2026-05-28", CreatedAt: seedAt},
	}
	if err := repo.db.WithContext(ctx).Create(&rows).Error; err != nil {
		t.Fatalf("seed ranking snapshots failed: %v", err)
	}

	if err := svc.RebuildLaggingRankingPortfolioResults(ctx, "manual-repair", false); err != nil {
		t.Fatalf("rebuild with historical market close failed: %v", err)
	}

	var marketPrice RankingPortfolioMarketPrice
	if err := repo.db.WithContext(ctx).
		Where("definition_id = ? AND snapshot_date = ? AND code = ? AND exchange = ?", defaultRankingPortfolioDefinitionID, "2026-05-28", "000725", "SZSE").
		First(&marketPrice).Error; err != nil {
		t.Fatalf("load rebuilt market price failed: %v", err)
	}
	if marketPrice.ClosePrice != 10.5 {
		t.Fatalf("close_price = %v, want 10.5", marketPrice.ClosePrice)
	}
	if marketPrice.PriceTradeDate != "2026-05-27" {
		t.Fatalf("price_trade_date = %s, want 2026-05-27", marketPrice.PriceTradeDate)
	}
}
