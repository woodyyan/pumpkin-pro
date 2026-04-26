package portfolio

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/woodyyan/pumpkin-pro/backend/store/quadrant"
	"github.com/woodyyan/pumpkin-pro/backend/tests/testutil"
	"gorm.io/gorm"
)

func setupRiskTest(t *testing.T) (*RiskRepository, *gorm.DB, *gorm.DB, *gorm.DB) {
	t.Helper()

	mainDB := testutil.InMemoryDB(t)
	testutil.AutoMigrateModels(t, mainDB, &PortfolioRecord{})

	cacheDB := testutil.InMemoryDB(t)
	testutil.AutoMigrateModels(t, cacheDB, &quadrant.QuadrantScoreRecord{}, &DailyBarRecord{})

	hkCacheDB := testutil.InMemoryDB(t)
	testutil.AutoMigrateModels(t, hkCacheDB, &quadrant.QuadrantScoreRecord{}, &DailyBarRecord{})

	portfolioRepo := NewRepository(mainDB)
	quadrantRepo := quadrant.NewRepository(cacheDB)
	riskRepo := NewRiskRepository(portfolioRepo, quadrantRepo, cacheDB, hkCacheDB)

	return riskRepo, mainDB, cacheDB, hkCacheDB
}

func insertPortfolioRecords(t *testing.T, db *gorm.DB, records []PortfolioRecord) {
	t.Helper()
	for _, rec := range records {
		if err := db.Create(&rec).Error; err != nil {
			t.Fatalf("插入持仓记录失败: %v", err)
		}
	}
}

func insertQuadrantRecords(t *testing.T, db *gorm.DB, records []quadrant.QuadrantScoreRecord) {
	t.Helper()
	for _, rec := range records {
		if err := db.Create(&rec).Error; err != nil {
			t.Fatalf("插入四象限记录失败: %v", err)
		}
	}
}

func insertDailyBarRecords(t *testing.T, db *gorm.DB, records []DailyBarRecord) {
	t.Helper()
	for _, rec := range records {
		if err := db.Create(&rec).Error; err != nil {
			t.Fatalf("插入日线记录失败: %v", err)
		}
	}
}

func TestCalcConcentrationRisk(t *testing.T) {
	riskRepo, _, _, _ := setupRiskTest(t)

	t.Run("空持仓", func(t *testing.T) {
		positions := []positionWithWeight{}
		metrics := riskRepo.calcConcentrationRisk(positions, 0)
		assert.NotNil(t, metrics)
		assert.Equal(t, 0.0, metrics.SingleStockMaxWeight)
		assert.Equal(t, 0.0, metrics.Top3Weight)
		assert.Equal(t, 0.0, metrics.Top5Weight)
		assert.Equal(t, 0.0, metrics.HerfindahlIndex)
		assert.Empty(t, metrics.Warnings)
	})

	t.Run("集中持仓预警", func(t *testing.T) {
		positions := []positionWithWeight{
			{Symbol: "000001.SZ", MarketValue: 50000, Weight: 0.5},
			{Symbol: "000002.SZ", MarketValue: 30000, Weight: 0.3},
			{Symbol: "000003.SZ", MarketValue: 20000, Weight: 0.2},
		}
		metrics := riskRepo.calcConcentrationRisk(positions, 100000)
		assert.Equal(t, 0.5, metrics.SingleStockMaxWeight)
		assert.Equal(t, 1.0, metrics.Top3Weight)
		assert.Equal(t, 1.0, metrics.Top5Weight)
		assert.InDelta(t, 0.38, metrics.HerfindahlIndex, 0.001)
		assert.Contains(t, metrics.Warnings, "单股集中度50.0%超过20%")
		assert.Contains(t, metrics.Warnings, "前三大持仓占比100.0%超过60%")
		assert.Contains(t, metrics.Warnings, "赫芬达尔指数0.380表明集中度较高")
	})
}

func TestEnrichPositionsWithMarketValue(t *testing.T) {
	ctx := context.Background()
	riskRepo, _, cacheDB, hkCacheDB := setupRiskTest(t)

	insertDailyBarRecords(t, cacheDB, []DailyBarRecord{{Code: "000001", Date: "2026-04-24", Close: 12.5}})
	insertDailyBarRecords(t, hkCacheDB, []DailyBarRecord{{Code: "00700", Date: "2026-04-24", Close: 380.0}})

	records := []PortfolioRecord{
		{UserID: "u", Symbol: "000001.SZ", Shares: 100, TotalCostAmount: 900},
		{UserID: "u", Symbol: "00700.HK", Shares: 10, TotalCostAmount: 3500},
		{UserID: "u", Symbol: "000002.SZ", Shares: 50, TotalCostAmount: 700},
	}

	positions, totalValue, err := riskRepo.enrichPositionsWithMarketValue(ctx, records)
	assert.NoError(t, err)
	assert.Len(t, positions, 3)
	assert.InDelta(t, 5750, totalValue, 0.001)
	assert.Equal(t, "00700.HK", positions[0].Symbol)
	assert.InDelta(t, 3800, positions[0].MarketValue, 0.001)
	assert.InDelta(t, 700, positions[2].MarketValue, 0.001)
}

func TestCalcLiquidityRisk(t *testing.T) {
	ctx := context.Background()
	riskRepo, _, cacheDB, hkCacheDB := setupRiskTest(t)
	computedAt := time.Now().Truncate(24 * time.Hour)

	insertQuadrantRecords(t, cacheDB, []quadrant.QuadrantScoreRecord{
		{Code: "000001", Exchange: "SZSE", AvgAmount5d: 5000, Liquidity: 0.85, ComputedAt: computedAt},
		{Code: "000002", Exchange: "SZSE", AvgAmount5d: 80, Liquidity: 0.15, ComputedAt: computedAt},
	})
	insertQuadrantRecords(t, hkCacheDB, []quadrant.QuadrantScoreRecord{
		{Code: "00700", Exchange: "HKEX", AvgAmount5d: 8000, Liquidity: 0.90, ComputedAt: computedAt},
		{Code: "00883", Exchange: "HKEX", AvgAmount5d: 50, Liquidity: 0.10, ComputedAt: computedAt},
	})

	positions := []positionWithWeight{
		{Symbol: "000001.SZ", HistoryCode: "000001", Scope: PortfolioScopeAShare, Weight: 0.4},
		{Symbol: "000002.SZ", HistoryCode: "000002", Scope: PortfolioScopeAShare, Weight: 0.3},
		{Symbol: "00700.HK", HistoryCode: "00700", Scope: PortfolioScopeHK, Weight: 0.2},
		{Symbol: "00883.HK", HistoryCode: "00883", Scope: PortfolioScopeHK, Weight: 0.1},
	}

	t.Run("全部市场", func(t *testing.T) {
		metrics, err := riskRepo.calcLiquidityRisk(ctx, positions, 100000, PortfolioScopeAll)
		assert.NoError(t, err)
		assert.InDelta(t, 3629, metrics.AvgDailyTurnover, 0.1)
		assert.InDelta(t, 57.5, metrics.LiquidityScore, 0.1)
		assert.Equal(t, 2, metrics.IlliquidCount)
		assert.InDelta(t, 0.4, metrics.LowLiquidityWeight, 0.001)
		assert.Contains(t, metrics.Warnings, "换手率数据暂不可用，已返回0")
		assert.Contains(t, metrics.Warnings, "有2只股票流动性较低（成交额<100万元），权重合计40.0%")
	})

	t.Run("A股范围过滤", func(t *testing.T) {
		metrics, err := riskRepo.calcLiquidityRisk(ctx, positions, 100000, "ashares")
		assert.NoError(t, err)
		assert.InDelta(t, (5000*0.4+80*0.3)/0.7, metrics.AvgDailyTurnover, 0.1)
		assert.InDelta(t, ((0.85*0.4+0.15*0.3)/0.7)*100, metrics.LiquidityScore, 0.1)
		assert.Equal(t, 1, metrics.IlliquidCount)
	})

	t.Run("港股范围过滤", func(t *testing.T) {
		metrics, err := riskRepo.calcLiquidityRisk(ctx, positions, 100000, "hkshares")
		assert.NoError(t, err)
		assert.InDelta(t, (8000*0.2+50*0.1)/0.3, metrics.AvgDailyTurnover, 0.1)
		assert.InDelta(t, ((0.90*0.2+0.10*0.1)/0.3)*100, metrics.LiquidityScore, 0.1)
		assert.Equal(t, 1, metrics.IlliquidCount)
	})
}

func TestPortfolioHistoricalAnalyticsAndRiskCalcs(t *testing.T) {
	ctx := context.Background()
	riskRepo, _, cacheDB, hkCacheDB := setupRiskTest(t)

	insertDailyBarRecords(t, cacheDB, []DailyBarRecord{
		{Code: "000001", Date: "2026-04-21", Close: 100},
		{Code: "000001", Date: "2026-04-22", Close: 102},
		{Code: "000001", Date: "2026-04-23", Close: 101},
		{Code: "000001", Date: "2026-04-24", Close: 104},
		{Code: "000002", Date: "2026-04-21", Close: 50},
		{Code: "000002", Date: "2026-04-22", Close: 49},
		{Code: "000002", Date: "2026-04-23", Close: 50},
		{Code: "000002", Date: "2026-04-24", Close: 52},
	})
	insertDailyBarRecords(t, hkCacheDB, []DailyBarRecord{{Code: "00700", Date: "2026-04-21", Close: 300}, {Code: "00700", Date: "2026-04-22", Close: 303}, {Code: "00700", Date: "2026-04-23", Close: 306}, {Code: "00700", Date: "2026-04-24", Close: 309}})

	positions := []positionWithWeight{
		{Symbol: "000001.SZ", HistoryCode: "000001", Scope: PortfolioScopeAShare, Weight: 0.5},
		{Symbol: "000002.SZ", HistoryCode: "000002", Scope: PortfolioScopeAShare, Weight: 0.3},
		{Symbol: "00700.HK", HistoryCode: "00700", Scope: PortfolioScopeHK, Weight: 0.2},
	}

	analytics, err := riskRepo.buildPortfolioHistoricalAnalytics(ctx, positions, 30)
	assert.NoError(t, err)
	assert.Len(t, analytics.DailyReturns, 3)
	assert.NotEmpty(t, analytics.Warnings)

	vol := riskRepo.calcVolatilityRisk(analytics)
	assert.NotNil(t, vol)
	assert.NotZero(t, vol.AnnualizedVolatility)
	assert.LessOrEqual(t, vol.MaxDrawdown, 0.0)

	tail := riskRepo.calcTailRisk(analytics)
	assert.NotNil(t, tail)
	assert.NotZero(t, tail.Var95OneDay)
	assert.NotZero(t, tail.ExpectedShortfall95)

	corr := riskRepo.calcCorrelationRisk(positions, analytics)
	assert.NotNil(t, corr)
	assert.GreaterOrEqual(t, corr.DiversificationScore, 0.0)
	assert.LessOrEqual(t, corr.DiversificationScore, 100.0)
}

func TestGetRiskMetrics(t *testing.T) {
	ctx := context.Background()
	riskRepo, mainDB, cacheDB, hkCacheDB := setupRiskTest(t)
	computedAt := time.Now().Truncate(24 * time.Hour)

	insertPortfolioRecords(t, mainDB, []PortfolioRecord{
		{ID: "1", UserID: "u1", Symbol: "000001.SZ", Shares: 100, TotalCostAmount: 1000},
		{ID: "2", UserID: "u1", Symbol: "00700.HK", Shares: 10, TotalCostAmount: 3000},
	})
	insertDailyBarRecords(t, cacheDB, []DailyBarRecord{{Code: "000001", Date: "2026-04-23", Close: 10}, {Code: "000001", Date: "2026-04-24", Close: 11}})
	insertDailyBarRecords(t, hkCacheDB, []DailyBarRecord{{Code: "00700", Date: "2026-04-23", Close: 300}, {Code: "00700", Date: "2026-04-24", Close: 310}})
	insertQuadrantRecords(t, cacheDB, []quadrant.QuadrantScoreRecord{{Code: "000001", Exchange: "SZSE", AvgAmount5d: 5000, Liquidity: 0.8, ComputedAt: computedAt}})
	insertQuadrantRecords(t, hkCacheDB, []quadrant.QuadrantScoreRecord{{Code: "00700", Exchange: "HKEX", AvgAmount5d: 8000, Liquidity: 0.9, ComputedAt: computedAt}})

	t.Run("空持仓", func(t *testing.T) {
		metrics, err := riskRepo.GetRiskMetrics(ctx, "missing-user", PortfolioScopeAll)
		assert.NoError(t, err)
		assert.Equal(t, PortfolioScopeAll, metrics.Scope)
		assert.Nil(t, metrics.ConcentrationRisk)
	})

	t.Run("按scope过滤", func(t *testing.T) {
		metrics, err := riskRepo.GetRiskMetrics(ctx, "u1", PortfolioScopeAShare)
		assert.NoError(t, err)
		assert.Equal(t, PortfolioScopeAShare, metrics.Scope)
		assert.NotNil(t, metrics.ConcentrationRisk)
		assert.InDelta(t, 1.0, metrics.ConcentrationRisk.SingleStockMaxWeight, 0.001)
		assert.NotNil(t, metrics.LiquidityRisk)
	})
}

func TestCalcOverallRiskScore(t *testing.T) {
	riskRepo, _, _, _ := setupRiskTest(t)

	assert.Equal(t, 5.0, riskRepo.calcOverallRiskScore(&RiskMetrics{}))
	assert.Equal(t, 7.0, riskRepo.calcOverallRiskScore(&RiskMetrics{ConcentrationRisk: &ConcentrationRiskMetrics{SingleStockMaxWeight: 0.35}}))
	assert.Equal(t, 6.0, riskRepo.calcOverallRiskScore(&RiskMetrics{ConcentrationRisk: &ConcentrationRiskMetrics{Top3Weight: 0.6}}))
	assert.Equal(t, 6.0, riskRepo.calcOverallRiskScore(&RiskMetrics{ConcentrationRisk: &ConcentrationRiskMetrics{HerfindahlIndex: 0.35}}))
	assert.Equal(t, 10.0, riskRepo.calcOverallRiskScore(&RiskMetrics{ConcentrationRisk: &ConcentrationRiskMetrics{SingleStockMaxWeight: 0.35, Top3Weight: 0.75, HerfindahlIndex: 0.35}}))
}
