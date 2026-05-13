package quadrant

import (
	"context"
	"testing"
	"time"
)

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
		makeAShareRankingRecord("600001", "MAIN", "机会", 95, 84, 10, 12000),
		makeAShareRankingRecord("000001", "MAIN", "机会", 94, 83, 10, 12000),
		makeAShareRankingRecord("300001", "CHINEXT", "机会", 93, 82, 10, 12000),
		makeAShareRankingRecord("600002", "MAIN", "机会", 92, 81, 10, 12000),
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
	if resp.Meta.SnapshotVersion != "2026-05-07" {
		t.Fatalf("expected latest snapshot version 2026-05-07, got %s", resp.Meta.SnapshotVersion)
	}
	if len(resp.Series) != 2 {
		t.Fatalf("expected 2 series points, got %d", len(resp.Series))
	}
	if resp.Series[1].PortfolioReturnPct <= 0 {
		t.Fatalf("expected positive portfolio return, got %+v", resp.Series[1])
	}
	if resp.Series[1].ExcessReturnPct <= 0 {
		t.Fatalf("expected positive excess return, got %+v", resp.Series[1])
	}
	if len(resp.Constituents) != 4 {
		t.Fatalf("expected 4 latest constituents, got %d", len(resp.Constituents))
	}
	if resp.Meta.BatchID == "" || resp.Meta.MethodNote == "" {
		t.Fatalf("expected batch id and method note, got %+v", resp.Meta)
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
