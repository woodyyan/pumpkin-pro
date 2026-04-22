package portfolio

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"math"
	"sort"
	"time"

	"github.com/woodyyan/pumpkin-pro/backend/store/quadrant"
	"gorm.io/gorm"
)

// RiskMetrics 组合风险指标
type RiskMetrics struct {
	Scope       string    `json:"scope"`
	ComputedAt  time.Time `json:"computed_at"`
	ConcentrationRisk *ConcentrationRiskMetrics `json:"concentration_risk,omitempty"`
	VolatilityRisk    *VolatilityRiskMetrics    `json:"volatility_risk,omitempty"`
	LiquidityRisk     *LiquidityRiskMetrics     `json:"liquidity_risk,omitempty"`
	TailRisk          *TailRiskMetrics          `json:"tail_risk,omitempty"`
	CorrelationRisk   *CorrelationRiskMetrics   `json:"correlation_risk,omitempty"`
	OverallRiskScore  float64                   `json:"overall_risk_score"` // 1-10分，越高风险越大
}

// ConcentrationRiskMetrics 集中度风险指标
type ConcentrationRiskMetrics struct {
	SingleStockMaxWeight float64   `json:"single_stock_max_weight"` // 单股最大权重
	Top3Weight           float64   `json:"top3_weight"`             // 前三大持仓权重合计
	Top5Weight           float64   `json:"top5_weight"`             // 前五大持仓权重合计
	HerfindahlIndex      float64   `json:"herfindahl_index"`        // 赫芬达尔指数（HHI）
	Warnings             []string  `json:"warnings"`                // 预警信息
}

// VolatilityRiskMetrics 波动率风险指标
type VolatilityRiskMetrics struct {
	AnnualizedVolatility float64   `json:"annualized_volatility"` // 年化波动率（标准差）
	MaxDrawdown          float64   `json:"max_drawdown"`          // 历史最大回撤（负值）
	DownsideProbability  float64   `json:"downside_probability"`  // 下跌概率（过去252交易日中收益为负的比例）
	DailyVar95           float64   `json:"daily_var_95"`          // 单日95% VaR（负值）
	WeeklyVar95          float64   `json:"weekly_var_95"`         // 单周95% VaR（负值）
	Warnings             []string  `json:"warnings"`
}

// LiquidityRiskMetrics 流动性风险指标
type LiquidityRiskMetrics struct {
	AvgDailyTurnover      float64   `json:"avg_daily_turnover"`      // 组合平均日成交额（万元）
	AvgTurnoverRate       float64   `json:"avg_turnover_rate"`       // 组合平均换手率（%）
	IlliquidCount         int       `json:"illiquid_count"`          // 低流动性股票数量（成交额<100万元）
	LowLiquidityWeight    float64   `json:"low_liquidity_weight"`    // 低流动性股票权重
	LiquidityScore        float64   `json:"liquidity_score"`         // 流动性评分（0-100，越高流动性越好）
	Warnings              []string  `json:"warnings"`
}

// TailRiskMetrics 尾部风险指标
type TailRiskMetrics struct {
	Var95OneDay           float64   `json:"var_95_one_day"`            // 95%置信度单日VaR（负值）
	Var95OneWeek          float64   `json:"var_95_one_week"`           // 95%置信度单周VaR（负值）
	ExpectedShortfall95   float64   `json:"expected_shortfall_95"`     // 95%置信度期望损失（ES）
	WorstCaseLoss         float64   `json:"worst_case_loss"`           // 历史最差单日损失（负值）
	Warnings              []string  `json:"warnings"`
}

// CorrelationRiskMetrics 相关性风险指标
type CorrelationRiskMetrics struct {
	AvgCorrelation        float64                     `json:"avg_correlation"`         // 平均相关系数
	HighCorrelationPairs  []HighCorrelationPair       `json:"high_correlation_pairs"`  // 高相关性股票对（>0.7）
	CorrelationMatrix     map[string]map[string]float64 `json:"correlation_matrix,omitempty"` // 相关系数矩阵（可选，数据量大时省略）
	DiversificationScore  float64                     `json:"diversification_score"`   // 分散化评分（0-100，越高分散越好）
	Warnings              []string                    `json:"warnings"`
}

// HighCorrelationPair 高相关性股票对
type HighCorrelationPair struct {
	Symbol1      string  `json:"symbol1"`
	Symbol2      string  `json:"symbol2"`
	Correlation  float64 `json:"correlation"`
}

// RiskRepository 风险数据仓库
type RiskRepository struct {
	portfolioRepo *Repository
	quadrantRepo  *quadrant.Repository
	cacheDB       *gorm.DB // quadrant_cache.db 连接
	hkCacheDB     *gorm.DB // quadrant_cache_hk.db 连接
}

// NewRiskRepository 创建风险数据仓库
func NewRiskRepository(portfolioRepo *Repository, quadrantRepo *quadrant.Repository, cacheDB *gorm.DB, hkCacheDB *gorm.DB) *RiskRepository {
	return &RiskRepository{
		portfolioRepo: portfolioRepo,
		quadrantRepo:  quadrantRepo,
		cacheDB:       cacheDB,
		hkCacheDB:     hkCacheDB,
	}
}

// GetRiskMetrics 计算用户组合的风险指标
func (r *RiskRepository) GetRiskMetrics(ctx context.Context, userID string, scope string) (*RiskMetrics, error) {
	// 1. 获取用户持仓
	records, err := r.portfolioRepo.ListByUser(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("获取持仓失败: %w", err)
	}
	if len(records) == 0 {
		return &RiskMetrics{
			Scope:      scope,
			ComputedAt: time.Now(),
		}, nil
	}

	// 2. 获取持仓的实时市值和权重
	positions, totalValue, err := r.enrichPositionsWithMarketValue(ctx, records, scope)
	if err != nil {
		return nil, fmt.Errorf("获取持仓市值失败: %w", err)
	}

	// 3. 计算各项风险指标
	metrics := &RiskMetrics{
		Scope:      scope,
		ComputedAt: time.Now(),
	}

	// 集中度风险
	metrics.ConcentrationRisk = r.calcConcentrationRisk(positions, totalValue)

	// 波动率风险（需要历史价格）
	metrics.VolatilityRisk, err = r.calcVolatilityRisk(ctx, positions, totalValue, scope)
	if err != nil {
		log.Printf("计算波动率风险失败: %v", err)
		metrics.VolatilityRisk = &VolatilityRiskMetrics{}
	}

	// 流动性风险
	metrics.LiquidityRisk, err = r.calcLiquidityRisk(ctx, positions, totalValue, scope)
	if err != nil {
		log.Printf("计算流动性风险失败: %v", err)
		metrics.LiquidityRisk = &LiquidityRiskMetrics{}
	}

	// 尾部风险
	metrics.TailRisk, err = r.calcTailRisk(ctx, positions, totalValue, scope)
	if err != nil {
		log.Printf("计算尾部风险失败: %v", err)
		metrics.TailRisk = &TailRiskMetrics{}
	}

	// 相关性风险
	metrics.CorrelationRisk, err = r.calcCorrelationRisk(ctx, positions, totalValue, scope)
	if err != nil {
		log.Printf("计算相关性风险失败: %v", err)
		metrics.CorrelationRisk = &CorrelationRiskMetrics{}
	}

	// 综合风险评分
	metrics.OverallRiskScore = r.calcOverallRiskScore(metrics)

	return metrics, nil
}

// enrichPositionsWithMarketValue 获取持仓的实时市值并计算权重
func (r *RiskRepository) enrichPositionsWithMarketValue(ctx context.Context, records []PortfolioRecord, scope string) ([]positionWithWeight, float64, error) {
	// 简化实现：使用持仓记录中的总成本作为市值代理
	// TODO: 实际应从行情接口获取最新价格
	positions := make([]positionWithWeight, 0, len(records))
	var totalValue float64
	for _, rec := range records {
		// 使用总成本作为市值（近似）
		marketValue := rec.TotalCostAmount
		positions = append(positions, positionWithWeight{
			Symbol:      rec.Symbol,
			MarketValue: marketValue,
		})
		totalValue += marketValue
	}

	// 计算权重
	for i := range positions {
		if totalValue > 0 {
			positions[i].Weight = positions[i].MarketValue / totalValue
		}
	}

	// 按权重降序排序
	sort.Slice(positions, func(i, j int) bool {
		return positions[i].Weight > positions[j].Weight
	})

	return positions, totalValue, nil
}

// positionWithWeight 带权重的持仓
type positionWithWeight struct {
	Symbol      string
	MarketValue float64
	Weight      float64
}

// calcConcentrationRisk 计算集中度风险
func (r *RiskRepository) calcConcentrationRisk(positions []positionWithWeight, totalValue float64) *ConcentrationRiskMetrics {
	metrics := &ConcentrationRiskMetrics{Warnings: []string{}}

	if len(positions) == 0 || totalValue == 0 {
		return metrics
	}

	// 单股最大权重
	metrics.SingleStockMaxWeight = positions[0].Weight

	// 前三大、前五大权重
	for i, pos := range positions {
		if i < 3 {
			metrics.Top3Weight += pos.Weight
		}
		if i < 5 {
			metrics.Top5Weight += pos.Weight
		}
	}

	// 赫芬达尔指数（HHI）: sum(weight_i^2)
	var hhi float64
	for _, pos := range positions {
		hhi += pos.Weight * pos.Weight
	}
	metrics.HerfindahlIndex = hhi

	// 预警规则
	if metrics.SingleStockMaxWeight > 0.2 {
		metrics.Warnings = append(metrics.Warnings, fmt.Sprintf("单股集中度%.1f%%超过20%%", metrics.SingleStockMaxWeight*100))
	}
	if metrics.Top3Weight > 0.6 {
		metrics.Warnings = append(metrics.Warnings, fmt.Sprintf("前三大持仓占比%.1f%%超过60%%", metrics.Top3Weight*100))
	}
	if metrics.HerfindahlIndex > 0.25 {
		metrics.Warnings = append(metrics.Warnings, fmt.Sprintf("赫芬达尔指数%.3f表明集中度较高", metrics.HerfindahlIndex))
	}

	return metrics
}

// calcVolatilityRisk 计算波动率风险
func (r *RiskRepository) calcVolatilityRisk(ctx context.Context, positions []positionWithWeight, totalValue float64, scope string) (*VolatilityRiskMetrics, error) {
	// 简化实现：暂时返回空指标，待历史价格数据接入后实现
	// TODO: 从daily_bars获取历史价格，计算组合收益率序列
	return &VolatilityRiskMetrics{
		Warnings: []string{"波动率计算需要历史价格数据，暂未实现"},
	}, nil
}

// calcLiquidityRisk 计算流动性风险
func (r *RiskRepository) calcLiquidityRisk(ctx context.Context, positions []positionWithWeight, totalValue float64, scope string) (*LiquidityRiskMetrics, error) {
	metrics := &LiquidityRiskMetrics{Warnings: []string{}}

	if len(positions) == 0 {
		return metrics, nil
	}

	// 根据scope过滤持仓
	filteredPositions := make([]positionWithWeight, 0, len(positions))
	for _, pos := range positions {
		isAShare := !isHKCode(pos.Symbol) // A股代码不是5位全数字
		isHKShare := isHKCode(pos.Symbol)

		switch scope {
		case "ashares":
			if isAShare {
				filteredPositions = append(filteredPositions, pos)
			}
		case "hkshares":
			if isHKShare {
				filteredPositions = append(filteredPositions, pos)
			}
		default: // "all" or empty
			filteredPositions = append(filteredPositions, pos)
		}
	}

	if len(filteredPositions) == 0 {
		return metrics, nil
	}

	// 从quadrant_scores表获取流动性数据
	type LiquidityRecord struct {
		AvgAmount5d float64
		Liquidity   float64 // 百分位 (0-1)
	}
	liquidityMap := make(map[string]LiquidityRecord)

	// 分批查询A股和港股
	var aShareCodes []string
	var hkCodes []string
	for _, pos := range filteredPositions {
		if isHKCode(pos.Symbol) {
			hkCodes = append(hkCodes, pos.Symbol)
		} else {
			aShareCodes = append(aShareCodes, pos.Symbol)
		}
	}

	// 查询A股数据
	if len(aShareCodes) > 0 && r.cacheDB != nil {
		var aShareRecords []quadrant.QuadrantScoreRecord
		err := r.cacheDB.WithContext(ctx).
			Where("code IN (?) AND computed_at = (SELECT MAX(computed_at) FROM quadrant_scores)", aShareCodes).
			Find(&aShareRecords).Error
		if err != nil {
			log.Printf("查询A股流动性数据失败: %v", err)
		} else {
			for _, rec := range aShareRecords {
				liquidityMap[rec.Code] = LiquidityRecord{
					AvgAmount5d: rec.AvgAmount5d,
					Liquidity:   rec.Liquidity,
				}
			}
		}
	}

	// 查询港股数据
	if len(hkCodes) > 0 && r.hkCacheDB != nil {
		var hkRecords []quadrant.QuadrantScoreRecord
		err := r.hkCacheDB.WithContext(ctx).
			Where("code IN (?) AND computed_at = (SELECT MAX(computed_at) FROM quadrant_scores)", hkCodes).
			Find(&hkRecords).Error
		if err != nil {
			log.Printf("查询港股流动性数据失败: %v", err)
		} else {
			for _, rec := range hkRecords {
				liquidityMap[rec.Code] = LiquidityRecord{
					AvgAmount5d: rec.AvgAmount5d,
					Liquidity:   rec.Liquidity,
				}
			}
		}
	}

	// 计算指标
	var weightedTurnoverSum float64
	var weightedLiquiditySum float64
	var totalWeight float64
	illiquidCount := 0
	lowLiquidityWeight := 0.0

	for _, pos := range filteredPositions {
		rec, ok := liquidityMap[pos.Symbol]
		if !ok {
			// 没有流动性数据，跳过
			continue
		}

		weightedTurnoverSum += rec.AvgAmount5d * pos.Weight
		weightedLiquiditySum += rec.Liquidity * pos.Weight
		totalWeight += pos.Weight

		// 低流动性股票：近5日均成交额 < 100万元
		if rec.AvgAmount5d < 100.0 {
			illiquidCount++
			lowLiquidityWeight += pos.Weight
		}
	}

	if totalWeight > 0 {
		metrics.AvgDailyTurnover = weightedTurnoverSum / totalWeight
		metrics.LiquidityScore = weightedLiquiditySum / totalWeight * 100.0 // 转换为0-100分
	}
	// 换手率数据暂不可用，设置为0并添加警告
	metrics.AvgTurnoverRate = 0.0
	metrics.IlliquidCount = illiquidCount
	metrics.LowLiquidityWeight = lowLiquidityWeight

	// 预警
	if illiquidCount > 0 {
		metrics.Warnings = append(metrics.Warnings, fmt.Sprintf("有%d只股票流动性较低（成交额<100万元），权重合计%.1f%%", illiquidCount, lowLiquidityWeight*100))
	}
	if len(liquidityMap) < len(filteredPositions) {
		missing := len(filteredPositions) - len(liquidityMap)
		metrics.Warnings = append(metrics.Warnings, fmt.Sprintf("%d只股票缺少流动性数据", missing))
	}

	return metrics, nil
}

// calcTailRisk 计算尾部风险
func (r *RiskRepository) calcTailRisk(ctx context.Context, positions []positionWithWeight, totalValue float64, scope string) (*TailRiskMetrics, error) {
	// 简化实现：暂时返回空指标
	// TODO: 基于历史收益率计算VaR和ES
	return &TailRiskMetrics{
		Warnings: []string{"尾部风险计算需要历史收益率数据，暂未实现"},
	}, nil
}

// calcCorrelationRisk 计算相关性风险
func (r *RiskRepository) calcCorrelationRisk(ctx context.Context, positions []positionWithWeight, totalValue float64, scope string) (*CorrelationRiskMetrics, error) {
	// 简化实现：暂时返回空指标
	// TODO: 计算股票间相关系数矩阵
	return &CorrelationRiskMetrics{
		Warnings: []string{"相关性计算需要历史收益率数据，暂未实现"},
	}, nil
}

// calcOverallRiskScore 计算综合风险评分（1-10分）
func (r *RiskRepository) calcOverallRiskScore(metrics *RiskMetrics) float64 {
	// 简化实现：基于集中度风险计算初步评分
	score := 5.0 // 基础分

	if metrics.ConcentrationRisk != nil {
		// 单股集中度贡献
		if metrics.ConcentrationRisk.SingleStockMaxWeight > 0.3 {
			score += 2.0
		} else if metrics.ConcentrationRisk.SingleStockMaxWeight > 0.2 {
			score += 1.0
		}

		// 前三大集中度贡献
		if metrics.ConcentrationRisk.Top3Weight > 0.7 {
			score += 2.0
		} else if metrics.ConcentrationRisk.Top3Weight > 0.5 {
			score += 1.0
		}

		// HHI贡献
		if metrics.ConcentrationRisk.HerfindahlIndex > 0.3 {
			score += 1.0
		}
	}

	// 限制在1-10分
	if score < 1.0 {
		score = 1.0
	}
	if score > 10.0 {
		score = 10.0
	}

	return math.Round(score*10) / 10
}

// isHKCode 判断是否为港股代码（5位全数字）
func isHKCode(symbol string) bool {
	if len(symbol) != 5 {
		return false
	}
	for _, ch := range symbol {
		if ch < '0' || ch > '9' {
			return false
		}
	}
	return true
}

// GetRiskMetricsJSON 返回JSON格式的风险指标（用于API响应）
func (r *RiskRepository) GetRiskMetricsJSON(ctx context.Context, userID string, scope string) ([]byte, error) {
	metrics, err := r.GetRiskMetrics(ctx, userID, scope)
	if err != nil {
		return nil, err
	}

	// 移除相关性矩阵等大数据字段，避免响应过大
	if metrics.CorrelationRisk != nil {
		metrics.CorrelationRisk.CorrelationMatrix = nil
	}

	return json.Marshal(metrics)
}