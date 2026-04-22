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

// setupRiskTest 创建内存数据库并初始化测试数据
func setupRiskTest(t *testing.T) (*RiskRepository, *gorm.DB, *gorm.DB) {
	t.Helper()

	// 创建主数据库（模拟 portfolio 表）
	mainDB := testutil.InMemoryDB(t)
	testutil.AutoMigrateModels(t, mainDB, &PortfolioRecord{})

	// 创建A股缓存数据库
	cacheDB := testutil.InMemoryDB(t)
	testutil.AutoMigrateModels(t, cacheDB, &quadrant.QuadrantScoreRecord{})

	// 创建港股缓存数据库
	hkCacheDB := testutil.InMemoryDB(t)
	testutil.AutoMigrateModels(t, hkCacheDB, &quadrant.QuadrantScoreRecord{})

	// 初始化仓库
	portfolioRepo := NewRepository(mainDB)
	quadrantRepo := quadrant.NewRepository(cacheDB) // 只需一个数据库，用于接口兼容
	riskRepo := NewRiskRepository(portfolioRepo, quadrantRepo, cacheDB, hkCacheDB)

	return riskRepo, cacheDB, hkCacheDB
}

// insertPortfolioRecords 插入测试持仓记录
func insertPortfolioRecords(t *testing.T, db *gorm.DB, records []PortfolioRecord) {
	t.Helper()
	for _, rec := range records {
		if err := db.Create(&rec).Error; err != nil {
			t.Fatalf("插入持仓记录失败: %v", err)
		}
	}
}

// insertQuadrantRecords 插入四象限分数记录
func insertQuadrantRecords(t *testing.T, db *gorm.DB, records []quadrant.QuadrantScoreRecord) {
	t.Helper()
	for _, rec := range records {
		if err := db.Create(&rec).Error; err != nil {
			t.Fatalf("插入四象限记录失败: %v", err)
		}
	}
}

func TestCalcConcentrationRisk(t *testing.T) {
	riskRepo, _, _ := setupRiskTest(t)

	t.Run("空持仓", func(t *testing.T) {
		positions := []positionWithWeight{}
		totalValue := 0.0
		metrics := riskRepo.calcConcentrationRisk(positions, totalValue)
		assert.NotNil(t, metrics)
		assert.Equal(t, 0.0, metrics.SingleStockMaxWeight)
		assert.Equal(t, 0.0, metrics.Top3Weight)
		assert.Equal(t, 0.0, metrics.Top5Weight)
		assert.Equal(t, 0.0, metrics.HerfindahlIndex)
		assert.Empty(t, metrics.Warnings)
	})

	t.Run("单只股票持仓", func(t *testing.T) {
		positions := []positionWithWeight{
			{Symbol: "000001", MarketValue: 100000, Weight: 1.0},
		}
		totalValue := 100000.0
		metrics := riskRepo.calcConcentrationRisk(positions, totalValue)
		assert.Equal(t, 1.0, metrics.SingleStockMaxWeight)
		assert.Equal(t, 1.0, metrics.Top3Weight)
		assert.Equal(t, 1.0, metrics.Top5Weight)
		assert.Equal(t, 1.0, metrics.HerfindahlIndex) // 1^2 = 1
		assert.Contains(t, metrics.Warnings, "单股集中度100.0%超过20%")
		assert.Contains(t, metrics.Warnings, "前三大持仓占比100.0%超过60%")
		assert.Contains(t, metrics.Warnings, "赫芬达尔指数1.000表明集中度较高")
	})

	t.Run("等权重三只股票", func(t *testing.T) {
		positions := []positionWithWeight{
			{Symbol: "000001", MarketValue: 33333, Weight: 0.33333},
			{Symbol: "000002", MarketValue: 33333, Weight: 0.33333},
			{Symbol: "000003", MarketValue: 33333, Weight: 0.33333},
		}
		totalValue := 99999.0
		metrics := riskRepo.calcConcentrationRisk(positions, totalValue)
		assert.InDelta(t, 0.33333, metrics.SingleStockMaxWeight, 0.0001)
		assert.InDelta(t, 1.0, metrics.Top3Weight, 0.0001)
		assert.InDelta(t, 1.0, metrics.Top5Weight, 0.0001)
		assert.InDelta(t, 0.33333, metrics.HerfindahlIndex, 0.0001) // 3 * (0.33333^2) ≈ 0.33333
		assert.Empty(t, metrics.Warnings) // 单股权重 < 20%，前三大权重 = 100%但未超过60%，HHI < 0.25
	})

	t.Run("集中持仓", func(t *testing.T) {
		positions := []positionWithWeight{
			{Symbol: "000001", MarketValue: 50000, Weight: 0.5},
			{Symbol: "000002", MarketValue: 30000, Weight: 0.3},
			{Symbol: "000003", MarketValue: 20000, Weight: 0.2},
		}
		totalValue := 100000.0
		metrics := riskRepo.calcConcentrationRisk(positions, totalValue)
		assert.Equal(t, 0.5, metrics.SingleStockMaxWeight)
		assert.Equal(t, 1.0, metrics.Top3Weight) // 0.5+0.3+0.2 = 1.0
		assert.Equal(t, 1.0, metrics.Top5Weight)
		assert.InDelta(t, 0.38, metrics.HerfindahlIndex, 0.01) // 0.5^2 + 0.3^2 + 0.2^2 = 0.25+0.09+0.04 = 0.38
		assert.Contains(t, metrics.Warnings, "单股集中度50.0%超过20%")
		assert.Contains(t, metrics.Warnings, "前三大持仓占比100.0%超过60%")
		assert.Contains(t, metrics.Warnings, "赫芬达尔指数0.380表明集中度较高")
	})
}

func TestCalcLiquidityRisk(t *testing.T) {
	ctx := context.Background()
	riskRepo, cacheDB, hkCacheDB := setupRiskTest(t)

	// 插入测试四象限数据
	computedAt := time.Now().Truncate(time.Hour * 24) // 今天零点
	records := []quadrant.QuadrantScoreRecord{
		{
			Code:        "000001",
			Name:        "平安银行",
			Exchange:    "SZSE",
			AvgAmount5d: 5000.0, // 高流动性
			Liquidity:   0.85,
			ComputedAt:  computedAt,
		},
		{
			Code:        "000002",
			Name:        "万科A",
			Exchange:    "SZSE",
			AvgAmount5d: 80.0, // 低流动性
			Liquidity:   0.15,
			ComputedAt:  computedAt,
		},
		{
			Code:        "00700",
			Name:        "腾讯控股",
			Exchange:    "HKEX",
			AvgAmount5d: 8000.0, // 高流动性
			Liquidity:   0.90,
			ComputedAt:  computedAt,
		},
		{
			Code:        "00883",
			Name:        "中国海洋石油",
			Exchange:    "HKEX",
			AvgAmount5d: 50.0, // 低流动性
			Liquidity:   0.10,
			ComputedAt:  computedAt,
		},
	}
	// A股记录插入 cacheDB，港股记录插入 hkCacheDB
	var aShareRecords []quadrant.QuadrantScoreRecord
	var hkRecords []quadrant.QuadrantScoreRecord
	for _, rec := range records {
		if rec.Exchange == "HKEX" {
			hkRecords = append(hkRecords, rec)
		} else {
			aShareRecords = append(aShareRecords, rec)
		}
	}
	insertQuadrantRecords(t, cacheDB, aShareRecords)
	insertQuadrantRecords(t, hkCacheDB, hkRecords)

	t.Run("无持仓", func(t *testing.T) {
		positions := []positionWithWeight{}
		totalValue := 0.0
		metrics, err := riskRepo.calcLiquidityRisk(ctx, positions, totalValue, "all")
		assert.NoError(t, err)
		assert.NotNil(t, metrics)
		assert.Equal(t, 0.0, metrics.AvgDailyTurnover)
		assert.Equal(t, 0.0, metrics.AvgTurnoverRate)
		assert.Equal(t, 0, metrics.IlliquidCount)
		assert.Equal(t, 0.0, metrics.LowLiquidityWeight)
		assert.Equal(t, 0.0, metrics.LiquidityScore)
		assert.Empty(t, metrics.Warnings)
	})

	t.Run("全部高流动性股票", func(t *testing.T) {
		positions := []positionWithWeight{
			{Symbol: "000001", MarketValue: 60000, Weight: 0.6},
			{Symbol: "00700", MarketValue: 40000, Weight: 0.4},
		}
		totalValue := 100000.0
		metrics, err := riskRepo.calcLiquidityRisk(ctx, positions, totalValue, "all")
		assert.NoError(t, err)
		assert.InDelta(t, 5000*0.6 + 8000*0.4, metrics.AvgDailyTurnover, 0.1) // 加权平均
		assert.Equal(t, 0.0, metrics.AvgTurnoverRate) // 暂不可用
		assert.Equal(t, 0, metrics.IlliquidCount)
		assert.Equal(t, 0.0, metrics.LowLiquidityWeight)
		assert.InDelta(t, (0.85*0.6 + 0.90*0.4) * 100, metrics.LiquidityScore, 0.1)
		assert.Empty(t, metrics.Warnings)
	})

	t.Run("包含低流动性股票", func(t *testing.T) {
		positions := []positionWithWeight{
			{Symbol: "000001", MarketValue: 40000, Weight: 0.4},
			{Symbol: "000002", MarketValue: 30000, Weight: 0.3},
			{Symbol: "00700", MarketValue: 20000, Weight: 0.2},
			{Symbol: "00883", MarketValue: 10000, Weight: 0.1},
		}
		totalValue := 100000.0
		metrics, err := riskRepo.calcLiquidityRisk(ctx, positions, totalValue, "all")
		assert.NoError(t, err)
		// 低流动性股票：000002 (80万) 和 00883 (50万) 都低于100万
		assert.Equal(t, 2, metrics.IlliquidCount)
		// 低流动性权重：0.3 + 0.1 = 0.4
		assert.InDelta(t, 0.4, metrics.LowLiquidityWeight, 0.001)
		assert.Contains(t, metrics.Warnings, "有2只股票流动性较低（成交额<100万元），权重合计40.0%")
	})

	t.Run("A股范围过滤", func(t *testing.T) {
		positions := []positionWithWeight{
			{Symbol: "000001", MarketValue: 50000, Weight: 0.5},
			{Symbol: "00700", MarketValue: 50000, Weight: 0.5},
		}
		totalValue := 100000.0
		metrics, err := riskRepo.calcLiquidityRisk(ctx, positions, totalValue, "ashares")
		assert.NoError(t, err)
		// 只计算A股000001，港股00700被过滤
		assert.InDelta(t, 5000.0, metrics.AvgDailyTurnover, 0.1)
		assert.InDelta(t, 0.85*100, metrics.LiquidityScore, 0.1)
		assert.Equal(t, 0, metrics.IlliquidCount) // 000001是高流动性
	})

	t.Run("港股范围过滤", func(t *testing.T) {
		positions := []positionWithWeight{
			{Symbol: "000001", MarketValue: 50000, Weight: 0.5},
			{Symbol: "00700", MarketValue: 50000, Weight: 0.5},
		}
		totalValue := 100000.0
		metrics, err := riskRepo.calcLiquidityRisk(ctx, positions, totalValue, "hkshares")
		assert.NoError(t, err)
		// 只计算港股00700
		assert.InDelta(t, 8000.0, metrics.AvgDailyTurnover, 0.1)
		assert.InDelta(t, 0.90*100, metrics.LiquidityScore, 0.1)
		assert.Equal(t, 0, metrics.IlliquidCount)
	})

	t.Run("缺少流动性数据", func(t *testing.T) {
		positions := []positionWithWeight{
			{Symbol: "999999", MarketValue: 100000, Weight: 1.0}, // 不存在于quadrant_scores
		}
		totalValue := 100000.0
		metrics, err := riskRepo.calcLiquidityRisk(ctx, positions, totalValue, "all")
		assert.NoError(t, err)
		assert.Equal(t, 0.0, metrics.AvgDailyTurnover)
		assert.Equal(t, 0.0, metrics.LiquidityScore)
		assert.Contains(t, metrics.Warnings, "1只股票缺少流动性数据")
	})
}

func TestCalcOverallRiskScore(t *testing.T) {
	riskRepo, _, _ := setupRiskTest(t)

	t.Run("基础分", func(t *testing.T) {
		metrics := &RiskMetrics{}
		score := riskRepo.calcOverallRiskScore(metrics)
		assert.Equal(t, 5.0, score)
	})

	t.Run("单股集中度贡献", func(t *testing.T) {
		metrics := &RiskMetrics{
			ConcentrationRisk: &ConcentrationRiskMetrics{
				SingleStockMaxWeight: 0.35, // >0.3
			},
		}
		score := riskRepo.calcOverallRiskScore(metrics)
		assert.Equal(t, 7.0, score) // 5 + 2

		metrics.ConcentrationRisk.SingleStockMaxWeight = 0.25 // >0.2, ≤0.3
		score = riskRepo.calcOverallRiskScore(metrics)
		assert.Equal(t, 6.0, score) // 5 + 1

		metrics.ConcentrationRisk.SingleStockMaxWeight = 0.15 // ≤0.2
		score = riskRepo.calcOverallRiskScore(metrics)
		assert.Equal(t, 5.0, score)
	})

	t.Run("前三大集中度贡献", func(t *testing.T) {
		metrics := &RiskMetrics{
			ConcentrationRisk: &ConcentrationRiskMetrics{
				Top3Weight: 0.75, // >0.7
			},
		}
		score := riskRepo.calcOverallRiskScore(metrics)
		assert.Equal(t, 7.0, score) // 5 + 2

		metrics.ConcentrationRisk.Top3Weight = 0.6 // >0.5, ≤0.7
		score = riskRepo.calcOverallRiskScore(metrics)
		assert.Equal(t, 6.0, score) // 5 + 1

		metrics.ConcentrationRisk.Top3Weight = 0.4 // ≤0.5
		score = riskRepo.calcOverallRiskScore(metrics)
		assert.Equal(t, 5.0, score)
	})

	t.Run("HHI贡献", func(t *testing.T) {
		metrics := &RiskMetrics{
			ConcentrationRisk: &ConcentrationRiskMetrics{
				HerfindahlIndex: 0.35, // >0.3
			},
		}
		score := riskRepo.calcOverallRiskScore(metrics)
		assert.Equal(t, 6.0, score) // 5 + 1

		metrics.ConcentrationRisk.HerfindahlIndex = 0.2 // ≤0.3
		score = riskRepo.calcOverallRiskScore(metrics)
		assert.Equal(t, 5.0, score)
	})

	t.Run("综合贡献", func(t *testing.T) {
		metrics := &RiskMetrics{
			ConcentrationRisk: &ConcentrationRiskMetrics{
				SingleStockMaxWeight: 0.35, // +2
				Top3Weight:           0.75, // +2
				HerfindahlIndex:      0.35, // +1
			},
		}
		score := riskRepo.calcOverallRiskScore(metrics)
		assert.Equal(t, 10.0, score) // 5+2+2+1 = 10，钳位在10
	})

	t.Run("钳位下限", func(t *testing.T) {
		// 没有集中度风险，基础分5.0，不会低于1.0
		metrics := &RiskMetrics{}
		score := riskRepo.calcOverallRiskScore(metrics)
		assert.GreaterOrEqual(t, score, 1.0)
	})
}