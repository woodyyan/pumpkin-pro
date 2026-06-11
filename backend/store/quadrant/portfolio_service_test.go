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
	// 调仓参考价使用 T+1 开盘价；保存（T 收盘）时开盘价尚未回填，故为 0（pending）。
	if item := itemsByCode["600002"]; item.Action != "sell" || item.FromWeight != 0.25 || item.ToWeight != 0 || item.ReferencePrice != 0 {
		t.Fatalf("unexpected sell rebalance item: %+v", item)
	}
	if item := itemsByCode["300001"]; item.Action != "sell" || item.FromWeight != 0.25 || item.ToWeight != 0 || item.ReferencePrice != 0 {
		t.Fatalf("unexpected sell rebalance item: %+v", item)
	}
	if item := itemsByCode["600003"]; item.Action != "buy" || item.FromWeight != 0 || item.ToWeight != 0.25 || item.ReferencePrice != 0 {
		t.Fatalf("unexpected buy rebalance item: %+v", item)
	}
	if item := itemsByCode["300002"]; item.Action != "buy" || item.FromWeight != 0 || item.ToWeight != 0.25 || item.ReferencePrice != 0 {
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
	if metrics.LatestDailyReturnPct == nil || *metrics.LatestDailyReturnPct != -1.9417 {
		t.Fatalf("latest_daily_return_pct (昨日, T-1) = %v, want -1.9417", metrics.LatestDailyReturnPct)
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

	// 买入价来自连续在仓周期建仓日(T+1=2026-05-07)的开盘价快照。
	marketPrices := []RankingPortfolioMarketPrice{
		{DefinitionID: defaultRankingPortfolioDefinitionID, SnapshotVersion: "2026-05-06", SnapshotDate: "2026-05-06", Code: "600001", Exchange: "SSE", OpenPrice: 10, EntryTradeDate: "2026-05-07", ClosePrice: 10, PriceTradeDate: "2026-05-06", CreatedAt: now, UpdatedAt: now},
	}
	if err := repo.db.WithContext(ctx).Create(&marketPrices).Error; err != nil {
		t.Fatalf("seed market prices failed: %v", err)
	}
	// 最新价来自每半小时刷新的实时价缓存。
	if err := repo.UpsertRankingPortfolioRealtimePrice(ctx, RankingPortfolioRealtimePrice{Code: "600001", Exchange: "SSE", LastPrice: 12, QuoteTime: now}); err != nil {
		t.Fatalf("seed realtime price failed: %v", err)
	}

	items := []RankingPortfolioConstituentItem{{Code: "600001", Exchange: "SSE"}}
	definition := RankingPortfolioDefinition{ID: defaultRankingPortfolioDefinitionID}
	if err := svc.enrichRankingPortfolioCurrentConstituents(ctx, definition, items, "2026-05-08"); err != nil {
		t.Fatalf("enrich constituents failed: %v", err)
	}
	if items[0].EntryTradeDate != "2026-05-07" {
		t.Fatalf("entry_trade_date = %s, want 2026-05-07 (T+1 建仓日)", items[0].EntryTradeDate)
	}
	if items[0].EntryPrice != 10 {
		t.Fatalf("entry_price (开盘买入价) = %v, want 10", items[0].EntryPrice)
	}
	if items[0].EntryPricePending {
		t.Fatalf("entry_price_pending should be false when open price exists")
	}
	if items[0].LatestPrice != 12 {
		t.Fatalf("latest_price (实时价) = %v, want 12", items[0].LatestPrice)
	}
	if items[0].LatestReturnPct == nil || *items[0].LatestReturnPct != 20 {
		t.Fatalf("latest_return_pct = %v, want 20", items[0].LatestReturnPct)
	}
}

func TestEnrichRankingPortfolioCurrentConstituents_PendingBeforeOpen(t *testing.T) {
	repo, cleanup := setupQuadrantTest(t)
	defer cleanup()
	svc := NewService(repo)
	ctx := context.Background()
	now := time.Now().UTC()

	snapshots := []RankingPortfolioSnapshot{
		{DefinitionID: defaultRankingPortfolioDefinitionID, SnapshotVersion: "2026-05-08", SnapshotDate: "2026-05-08", SourceTradeDate: "2026-05-08", CreatedAt: now, UpdatedAt: now},
	}
	if err := repo.db.WithContext(ctx).Create(&snapshots).Error; err != nil {
		t.Fatalf("seed snapshots failed: %v", err)
	}
	rows := []RankingPortfolioSnapshotConstituent{
		{DefinitionID: defaultRankingPortfolioDefinitionID, SnapshotVersion: "2026-05-08", SnapshotDate: "2026-05-08", Code: "600001", Exchange: "SSE", Rank: 1, Weight: 0.25, CreatedAt: now, UpdatedAt: now},
	}
	if err := repo.db.WithContext(ctx).Create(&rows).Error; err != nil {
		t.Fatalf("seed constituents failed: %v", err)
	}
	// 收盘价存在，但开盘价尚未回填（T+1 9:25 前）。
	if err := repo.db.WithContext(ctx).Create(&RankingSnapshot{Code: "600001", Name: "股票600001", Exchange: "SSE", ClosePrice: 12, PriceTradeDate: "2026-05-08", SnapshotDate: "2026-05-08", CreatedAt: now}).Error; err != nil {
		t.Fatalf("seed ranking snapshot failed: %v", err)
	}

	items := []RankingPortfolioConstituentItem{{Code: "600001", Exchange: "SSE"}}
	definition := RankingPortfolioDefinition{ID: defaultRankingPortfolioDefinitionID}
	if err := svc.enrichRankingPortfolioCurrentConstituents(ctx, definition, items, "2026-05-08"); err != nil {
		t.Fatalf("enrich constituents failed: %v", err)
	}
	if !items[0].EntryPricePending {
		t.Fatalf("expected entry_price_pending=true before open price filled, got %+v", items[0])
	}
	if items[0].EntryPrice != 0 {
		t.Fatalf("entry_price must stay 0 (no close-price fallback), got %v", items[0].EntryPrice)
	}
	if items[0].LatestReturnPct != nil {
		t.Fatalf("latest_return_pct must be nil while pending, got %v", *items[0].LatestReturnPct)
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

// ── BackfillMissingEntryOpenPrices Tests ──

func TestBackfillMissingEntryOpenPrices_FillsRows(t *testing.T) {
	repo, cleanup := setupQuadrantTest(t)
	defer cleanup()
	ctx := context.Background()
	svc := NewService(repo)

	// Seed a definition.
	def := defaultRankingPortfolioDefinitionRecords(time.Now().UTC())[0]
	if err := repo.db.WithContext(ctx).Create(&def).Error; err != nil {
		t.Fatalf("seed definition: %v", err)
	}

	// Seed a snapshot on D0 and the next day (T+1).
	d0 := "2026-06-10"
	d1 := "2026-06-11"
	snaps := []RankingPortfolioSnapshot{
		{DefinitionID: def.ID, SnapshotVersion: d0, SnapshotDate: d0, CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC()},
		{DefinitionID: def.ID, SnapshotVersion: d1, SnapshotDate: d1, CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC()},
	}
	if err := repo.db.WithContext(ctx).Create(&snaps).Error; err != nil {
		t.Fatalf("seed snapshots: %v", err)
	}

	// Seed a market price row for D0 with open_price=0 (pending).
	mp := RankingPortfolioMarketPrice{
		DefinitionID:    def.ID,
		SnapshotVersion: d0,
		SnapshotDate:    d0,
		Code:            "600001",
		Exchange:        "SSE",
		ClosePrice:      10.0,
		OpenPrice:       0,
		CreatedAt:       time.Now().UTC(),
		UpdatedAt:       time.Now().UTC(),
	}
	if err := repo.db.WithContext(ctx).Create(&mp).Error; err != nil {
		t.Fatalf("seed market price: %v", err)
	}

	// Inject an open price resolver that returns a price for the entry date (D1).
	svc.SetOpenPriceResolver(func(_ context.Context, code, exchange, tradeDate string) float64 {
		if code == "600001" && exchange == "SSE" && tradeDate == d1 {
			return 10.5
		}
		return 0
	})

	result, err := svc.BackfillMissingEntryOpenPrices(ctx, d0)
	if err != nil {
		t.Fatalf("backfill failed: %v", err)
	}
	if result.FilledCount != 1 {
		t.Fatalf("FilledCount = %d, want 1", result.FilledCount)
	}
	if result.StillPendingCount != 0 {
		t.Fatalf("StillPendingCount = %d, want 0", result.StillPendingCount)
	}

	// Verify the DB row was updated.
	var updated RankingPortfolioMarketPrice
	if err := repo.db.WithContext(ctx).
		Where("definition_id = ? AND snapshot_version = ? AND code = ?", def.ID, d0, "600001").
		First(&updated).Error; err != nil {
		t.Fatalf("reload market price: %v", err)
	}
	if updated.OpenPrice != 10.5 {
		t.Fatalf("open_price = %v, want 10.5", updated.OpenPrice)
	}
	if updated.EntryTradeDate != d1 {
		t.Fatalf("entry_trade_date = %s, want %s", updated.EntryTradeDate, d1)
	}
}

func TestBackfillMissingEntryOpenPrices_SkipsBeforeCutover(t *testing.T) {
	repo, cleanup := setupQuadrantTest(t)
	defer cleanup()
	ctx := context.Background()
	svc := NewService(repo)

	def := defaultRankingPortfolioDefinitionRecords(time.Now().UTC())[0]
	if err := repo.db.WithContext(ctx).Create(&def).Error; err != nil {
		t.Fatalf("seed definition: %v", err)
	}

	// Snapshot before cutover.
	old := "2026-05-01"
	oldSnap := RankingPortfolioSnapshot{
		DefinitionID: def.ID, SnapshotVersion: old, SnapshotDate: old,
		CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC(),
	}
	if err := repo.db.WithContext(ctx).Create(&oldSnap).Error; err != nil {
		t.Fatalf("seed snapshot: %v", err)
	}
	oldMP := RankingPortfolioMarketPrice{
		DefinitionID: def.ID, SnapshotVersion: old, SnapshotDate: old,
		Code: "600001", Exchange: "SSE", ClosePrice: 9.0, OpenPrice: 0,
		CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC(),
	}
	if err := repo.db.WithContext(ctx).Create(&oldMP).Error; err != nil {
		t.Fatalf("seed market price: %v", err)
	}

	called := false
	svc.SetOpenPriceResolver(func(_ context.Context, code, exchange, tradeDate string) float64 {
		called = true
		return 9.5
	})

	// cutoverDate is after the snapshot date → should be skipped entirely.
	result, err := svc.BackfillMissingEntryOpenPrices(ctx, "2026-06-10")
	if err != nil {
		t.Fatalf("backfill failed: %v", err)
	}
	if result.FilledCount != 0 {
		t.Fatalf("FilledCount = %d, want 0 (pre-cutover rows must be skipped)", result.FilledCount)
	}
	if called {
		t.Fatal("open price resolver must not be called for pre-cutover rows")
	}
}

// 修复A 回归：当 snapshot 是最新一条（无后继），但 snapshot_date < 今天（北京时间）时，
// resolveEntryTradeDateForSnapshot 应以今日作为 T+1，而不是返回 ""。
// 这覆盖了 D0 当天及港股只有一条 snapshot 的场景。
func TestBackfillMissingEntryOpenPrices_FillsWhenLatestSnapshotHasNoSuccessor(t *testing.T) {
	repo, cleanup := setupQuadrantTest(t)
	defer cleanup()
	ctx := context.Background()
	svc := NewService(repo)

	def := defaultRankingPortfolioDefinitionRecords(time.Now().UTC())[0]
	if err := repo.db.WithContext(ctx).Create(&def).Error; err != nil {
		t.Fatalf("seed definition: %v", err)
	}

	// Use yesterday as snapshot_date so todayBJ > snapshotDate is always true.
	yesterday := time.Now().In(beijingLocation()).AddDate(0, 0, -1).Format("2006-01-02")
	todayBJ := time.Now().In(beijingLocation()).Format("2006-01-02")

	snap := RankingPortfolioSnapshot{
		DefinitionID: def.ID, SnapshotVersion: yesterday, SnapshotDate: yesterday,
		CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC(),
	}
	if err := repo.db.WithContext(ctx).Create(&snap).Error; err != nil {
		t.Fatalf("seed snapshot: %v", err)
	}
	mp := RankingPortfolioMarketPrice{
		DefinitionID: def.ID, SnapshotVersion: yesterday, SnapshotDate: yesterday,
		Code: "600001", Exchange: "SSE", ClosePrice: 10.0, OpenPrice: 0,
		CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC(),
	}
	if err := repo.db.WithContext(ctx).Create(&mp).Error; err != nil {
		t.Fatalf("seed market price: %v", err)
	}

	// Resolver returns a price only when called with today as entry date.
	svc.SetOpenPriceResolver(func(_ context.Context, _, _, tradeDate string) float64 {
		if tradeDate == todayBJ {
			return 11.2
		}
		return 0
	})

	result, err := svc.BackfillMissingEntryOpenPrices(ctx, yesterday)
	if err != nil {
		t.Fatalf("backfill failed: %v", err)
	}
	if result.FilledCount != 1 {
		t.Fatalf("FilledCount = %d, want 1 (latest snapshot with no successor should use today as T+1)", result.FilledCount)
	}
	if result.StillPendingCount != 0 {
		t.Fatalf("StillPendingCount = %d, want 0", result.StillPendingCount)
	}

	var updated RankingPortfolioMarketPrice
	if err := repo.db.WithContext(ctx).
		Where("definition_id = ? AND snapshot_version = ? AND code = ?", def.ID, yesterday, "600001").
		First(&updated).Error; err != nil {
		t.Fatalf("reload market price: %v", err)
	}
	if updated.OpenPrice != 11.2 {
		t.Fatalf("open_price = %v, want 11.2", updated.OpenPrice)
	}
	if updated.EntryTradeDate != todayBJ {
		t.Fatalf("entry_trade_date = %s, want %s (today BJ)", updated.EntryTradeDate, todayBJ)
	}
}

// 修复A 回归：当 snapshot_date == 今天（T+1 尚未到来）时，兜底不应触发，
// 应仍然返回 pending（T+1 还没开始，无法确定 entry date）。
func TestBackfillMissingEntryOpenPrices_StillPendingWhenSnapshotDateIsToday(t *testing.T) {
	repo, cleanup := setupQuadrantTest(t)
	defer cleanup()
	ctx := context.Background()
	svc := NewService(repo)

	def := defaultRankingPortfolioDefinitionRecords(time.Now().UTC())[0]
	if err := repo.db.WithContext(ctx).Create(&def).Error; err != nil {
		t.Fatalf("seed definition: %v", err)
	}

	// snapshot_date == today: T+1 hasn't started yet, no fallback should fire.
	todayBJ := time.Now().In(beijingLocation()).Format("2006-01-02")
	snap := RankingPortfolioSnapshot{
		DefinitionID: def.ID, SnapshotVersion: todayBJ, SnapshotDate: todayBJ,
		CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC(),
	}
	if err := repo.db.WithContext(ctx).Create(&snap).Error; err != nil {
		t.Fatalf("seed snapshot: %v", err)
	}
	mp := RankingPortfolioMarketPrice{
		DefinitionID: def.ID, SnapshotVersion: todayBJ, SnapshotDate: todayBJ,
		Code: "600001", Exchange: "SSE", ClosePrice: 10.0, OpenPrice: 0,
		CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC(),
	}
	if err := repo.db.WithContext(ctx).Create(&mp).Error; err != nil {
		t.Fatalf("seed market price: %v", err)
	}

	svc.SetOpenPriceResolver(func(_ context.Context, _, _, _ string) float64 { return 10.5 })

	result, err := svc.BackfillMissingEntryOpenPrices(ctx, todayBJ)
	if err != nil {
		t.Fatalf("backfill failed: %v", err)
	}
	if result.FilledCount != 0 {
		t.Fatalf("FilledCount = %d, want 0 (snapshot_date==today, T+1 not yet determinable)", result.FilledCount)
	}
	if result.StillPendingCount != 1 {
		t.Fatalf("StillPendingCount = %d, want 1", result.StillPendingCount)
	}
}

func TestBackfillMissingEntryOpenPrices_PendingWhenNoSuccessorSnapshot(t *testing.T) {
	repo, cleanup := setupQuadrantTest(t)
	defer cleanup()
	ctx := context.Background()
	svc := NewService(repo)

	def := defaultRankingPortfolioDefinitionRecords(time.Now().UTC())[0]
	if err := repo.db.WithContext(ctx).Create(&def).Error; err != nil {
		t.Fatalf("seed definition: %v", err)
	}

	// snapshot_date == today: no successor and snapshotDate == todayBJ,
	// so the fallback must NOT fire (T+1 cannot be determined yet).
	todayBJ := time.Now().In(beijingLocation()).Format("2006-01-02")
	snap := RankingPortfolioSnapshot{
		DefinitionID: def.ID, SnapshotVersion: todayBJ, SnapshotDate: todayBJ,
		CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC(),
	}
	if err := repo.db.WithContext(ctx).Create(&snap).Error; err != nil {
		t.Fatalf("seed snapshot: %v", err)
	}
	mp := RankingPortfolioMarketPrice{
		DefinitionID: def.ID, SnapshotVersion: todayBJ, SnapshotDate: todayBJ,
		Code: "600001", Exchange: "SSE", ClosePrice: 10.0, OpenPrice: 0,
		CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC(),
	}
	if err := repo.db.WithContext(ctx).Create(&mp).Error; err != nil {
		t.Fatalf("seed market price: %v", err)
	}

	svc.SetOpenPriceResolver(func(_ context.Context, _, _, _ string) float64 { return 10.5 })

	result, err := svc.BackfillMissingEntryOpenPrices(ctx, todayBJ)
	if err != nil {
		t.Fatalf("backfill failed: %v", err)
	}
	if result.FilledCount != 0 {
		t.Fatalf("FilledCount = %d, want 0 (no successor snapshot yet)", result.FilledCount)
	}
	if result.StillPendingCount != 1 {
		t.Fatalf("StillPendingCount = %d, want 1", result.StillPendingCount)
	}
}

func TestRebuildLaggingRankingPortfolioResultsFromDate_D0Guard(t *testing.T) {
	repo, cleanup := setupQuadrantTest(t)
	defer cleanup()
	ctx := context.Background()
	svc := NewService(repo)

	// Seed definition.
	defs := defaultRankingPortfolioDefinitionRecords(time.Now().UTC())
	for i := range defs {
		if err := repo.db.WithContext(ctx).Create(&defs[i]).Error; err != nil {
			t.Fatalf("seed definition: %v", err)
		}
	}

	// Seed two ranking snapshots: one before cutover (old) and one after (new).
	old := "2026-05-01"
	new_ := "2026-06-10"
	exchange := "SZSE"
	for _, date := range []string{old, new_} {
		snap := RankingSnapshot{
			Code:           "000001",
			Name:           "平安银行",
			Exchange:       exchange,
			Rank:           1,
			Opportunity:    0.9,
			Risk:           0.1,
			ClosePrice:     10.0,
			PriceTradeDate: date,
			SnapshotDate:   date,
			CreatedAt:      time.Now().UTC(),
		}
		if err := repo.db.WithContext(ctx).Create(&snap).Error; err != nil {
			t.Fatalf("seed ranking snapshot: %v", err)
		}
	}

	// Run rebuild with D0 = new_ (should only attempt new_, not old).
	// Since there are no constituents/market-prices, results are empty — but the
	// logic must not error and must not attempt to build a result for old.
	if err := svc.RebuildLaggingRankingPortfolioResultsFromDate(ctx, "test-d0-guard", false, new_); err != nil {
		t.Fatalf("rebuild with D0 guard failed: %v", err)
	}

	// Verify that no result was created for the pre-cutover date.
	var count int64
	if err := repo.db.WithContext(ctx).
		Model(&RankingPortfolioResult{}).
		Where("snapshot_date < ?", new_).
		Count(&count).Error; err != nil {
		t.Fatalf("count pre-cutover results: %v", err)
	}
	if count != 0 {
		t.Fatalf("expected 0 pre-cutover results, got %d", count)
	}
}
