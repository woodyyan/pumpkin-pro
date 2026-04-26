package portfolio

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"math"
	"sort"
	"strings"
	"time"

	"github.com/woodyyan/pumpkin-pro/backend/store/live"
	"github.com/woodyyan/pumpkin-pro/backend/store/quadrant"
	"gorm.io/gorm"
)

const (
	riskHistoryDays                 = 252
	riskMinSampleDays               = 60
	riskCoverageThreshold           = 0.70
	correlationHistoryDays          = 120
	correlationMinSampleDays        = 30
	correlationTopPositionLimit     = 10
	highCorrelationThreshold        = 0.70
	highCorrelationPairsReturnLimit = 10
)

// RiskMetrics 组合风险指标
type RiskMetrics struct {
	Scope             string                    `json:"scope"`
	ComputedAt        time.Time                 `json:"computed_at"`
	ConcentrationRisk *ConcentrationRiskMetrics `json:"concentration_risk,omitempty"`
	VolatilityRisk    *VolatilityRiskMetrics    `json:"volatility_risk,omitempty"`
	LiquidityRisk     *LiquidityRiskMetrics     `json:"liquidity_risk,omitempty"`
	TailRisk          *TailRiskMetrics          `json:"tail_risk,omitempty"`
	CorrelationRisk   *CorrelationRiskMetrics   `json:"correlation_risk,omitempty"`
	OverallRiskScore  float64                   `json:"overall_risk_score"`
}

// ConcentrationRiskMetrics 集中度风险指标
type ConcentrationRiskMetrics struct {
	SingleStockMaxWeight float64  `json:"single_stock_max_weight"`
	Top3Weight           float64  `json:"top3_weight"`
	Top5Weight           float64  `json:"top5_weight"`
	HerfindahlIndex      float64  `json:"herfindahl_index"`
	Warnings             []string `json:"warnings"`
}

// VolatilityRiskMetrics 波动率风险指标
type VolatilityRiskMetrics struct {
	AnnualizedVolatility float64  `json:"annualized_volatility"`
	MaxDrawdown          float64  `json:"max_drawdown"`
	DownsideProbability  float64  `json:"downside_probability"`
	DailyVar95           float64  `json:"daily_var_95"`
	WeeklyVar95          float64  `json:"weekly_var_95"`
	Warnings             []string `json:"warnings"`
}

// LiquidityRiskMetrics 流动性风险指标
type LiquidityRiskMetrics struct {
	AvgDailyTurnover   float64  `json:"avg_daily_turnover"`
	AvgTurnoverRate    float64  `json:"avg_turnover_rate"`
	IlliquidCount      int      `json:"illiquid_count"`
	LowLiquidityWeight float64  `json:"low_liquidity_weight"`
	LiquidityScore     float64  `json:"liquidity_score"`
	Warnings           []string `json:"warnings"`
}

// TailRiskMetrics 尾部风险指标
type TailRiskMetrics struct {
	Var95OneDay         float64  `json:"var_95_one_day"`
	Var95OneWeek        float64  `json:"var_95_one_week"`
	ExpectedShortfall95 float64  `json:"expected_shortfall_95"`
	WorstCaseLoss       float64  `json:"worst_case_loss"`
	Warnings            []string `json:"warnings"`
}

// CorrelationRiskMetrics 相关性风险指标
type CorrelationRiskMetrics struct {
	AvgCorrelation       float64                       `json:"avg_correlation"`
	HighCorrelationPairs []HighCorrelationPair         `json:"high_correlation_pairs"`
	CorrelationMatrix    map[string]map[string]float64 `json:"correlation_matrix,omitempty"`
	DiversificationScore float64                       `json:"diversification_score"`
	Warnings             []string                      `json:"warnings"`
}

// HighCorrelationPair 高相关性股票对
type HighCorrelationPair struct {
	Symbol1     string  `json:"symbol1"`
	Symbol2     string  `json:"symbol2"`
	Correlation float64 `json:"correlation"`
}

// RiskRepository 风险数据仓库
type RiskRepository struct {
	portfolioRepo *Repository
	quadrantRepo  *quadrant.Repository
	cacheDB       *gorm.DB
	hkCacheDB     *gorm.DB
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

type positionWithWeight struct {
	Symbol          string
	HistoryCode     string
	Exchange        string
	Scope           string
	Shares          float64
	TotalCostAmount float64
	MarketValue     float64
	Weight          float64
}

type portfolioHistoricalAnalytics struct {
	DailyReturns    []float64
	WeeklyReturns   []float64
	ReturnsBySymbol map[string]map[string]float64
	Warnings        []string
}

// GetRiskMetrics 计算用户组合的风险指标
func (r *RiskRepository) GetRiskMetrics(ctx context.Context, userID string, scope string) (*RiskMetrics, error) {
	normalizedScope, err := normalizePortfolioScope(scope)
	if err != nil {
		return nil, err
	}

	records, err := r.portfolioRepo.ListByUser(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("获取持仓失败: %w", err)
	}
	filteredRecords := filterPortfolioRecordsByScope(records, normalizedScope)
	if len(filteredRecords) == 0 {
		return &RiskMetrics{Scope: normalizedScope, ComputedAt: time.Now().UTC()}, nil
	}

	positions, totalValue, err := r.enrichPositionsWithMarketValue(ctx, filteredRecords)
	if err != nil {
		return nil, fmt.Errorf("获取持仓市值失败: %w", err)
	}

	metrics := &RiskMetrics{Scope: normalizedScope, ComputedAt: time.Now().UTC()}
	metrics.ConcentrationRisk = r.calcConcentrationRisk(positions, totalValue)

	metrics.LiquidityRisk, err = r.calcLiquidityRisk(ctx, positions, totalValue, normalizedScope)
	if err != nil {
		log.Printf("计算流动性风险失败: %v", err)
		metrics.LiquidityRisk = &LiquidityRiskMetrics{Warnings: []string{"流动性指标计算失败"}}
	}

	historical, err := r.buildPortfolioHistoricalAnalytics(ctx, positions, riskHistoryDays)
	if err != nil {
		log.Printf("构建组合历史收益率失败: %v", err)
		historical = &portfolioHistoricalAnalytics{Warnings: []string{"历史行情读取失败"}}
	}
	metrics.VolatilityRisk = r.calcVolatilityRisk(historical)
	metrics.TailRisk = r.calcTailRisk(historical)
	metrics.CorrelationRisk = r.calcCorrelationRisk(positions, historical)
	metrics.OverallRiskScore = r.calcOverallRiskScore(metrics)
	return metrics, nil
}

func filterPortfolioRecordsByScope(records []PortfolioRecord, scope string) []PortfolioRecord {
	filtered := make([]PortfolioRecord, 0, len(records))
	for _, record := range records {
		normalizedSymbol, exchange := normalizeRiskSymbol(record.Symbol)
		if normalizedSymbol == "" {
			continue
		}
		record.Symbol = normalizedSymbol
		recordScope := exchangeToScope(exchange)
		if scope != PortfolioScopeAll && recordScope != scope {
			continue
		}
		filtered = append(filtered, record)
	}
	return filtered
}

func normalizeRiskSymbol(symbol string) (string, string) {
	normalized := normalizePortfolioSymbol(symbol)
	exchange := live.ExchangeFromSymbol(normalized)
	if exchange != "" {
		return normalized, exchange
	}
	parsed, parsedExchange, err := live.NormalizeSymbol(normalized)
	if err != nil {
		return "", ""
	}
	return parsed, parsedExchange
}

func normalizeRiskScopeAlias(scope string) string {
	switch strings.ToUpper(strings.TrimSpace(scope)) {
	case "", "ALL":
		return PortfolioScopeAll
	case "ASHARE", "ASHARES", "SSE", "SZSE":
		return PortfolioScopeAShare
	case "HKEX", "HKSHARES":
		return PortfolioScopeHK
	default:
		return strings.ToUpper(strings.TrimSpace(scope))
	}
}

func positionHistoryCode(position positionWithWeight) string {
	if strings.TrimSpace(position.HistoryCode) != "" {
		return position.HistoryCode
	}
	return historyCodeFromSymbol(position.Symbol)
}

func inferRiskPositionScope(position positionWithWeight) string {
	if strings.TrimSpace(position.Scope) != "" {
		return normalizeRiskScopeAlias(position.Scope)
	}
	if scope := exchangeToScope(position.Exchange); scope != "" {
		return scope
	}
	code := positionHistoryCode(position)
	if isHKCode(code) {
		return PortfolioScopeHK
	}
	if code != "" {
		return PortfolioScopeAShare
	}
	return ""
}

func filterRiskPositionsByScope(positions []positionWithWeight, scope string) []positionWithWeight {
	normalizedScope := normalizeRiskScopeAlias(scope)
	if normalizedScope == "" || normalizedScope == PortfolioScopeAll {
		return append([]positionWithWeight(nil), positions...)
	}
	filtered := make([]positionWithWeight, 0, len(positions))
	for _, position := range positions {
		positionScope := inferRiskPositionScope(position)
		if positionScope == normalizedScope {
			filtered = append(filtered, position)
		}
	}
	return filtered
}

func (r *RiskRepository) riskDBRepository() *RiskDBRepository {
	return &RiskDBRepository{cacheDB: r.cacheDB, hkCacheDB: r.hkCacheDB}
}

// enrichPositionsWithMarketValue 获取持仓的最新市值并计算权重
func (r *RiskRepository) enrichPositionsWithMarketValue(ctx context.Context, records []PortfolioRecord) ([]positionWithWeight, float64, error) {
	positions := make([]positionWithWeight, 0, len(records))
	for _, record := range records {
		normalizedSymbol, exchange := normalizeRiskSymbol(record.Symbol)
		if normalizedSymbol == "" || record.Shares <= 0 {
			continue
		}
		positions = append(positions, positionWithWeight{
			Symbol:          normalizedSymbol,
			HistoryCode:     historyCodeFromSymbol(normalizedSymbol),
			Exchange:        exchange,
			Scope:           inferRiskPositionScope(positionWithWeight{Symbol: normalizedSymbol, Exchange: exchange}),
			Shares:          record.Shares,
			TotalCostAmount: record.TotalCostAmount,
		})
	}

	latestCloseBySymbol, err := r.loadLatestCloseBySymbol(ctx, positions)
	if err != nil {
		return nil, 0, err
	}

	totalValue := 0.0
	for i := range positions {
		marketValue := positions[i].TotalCostAmount
		if latestClose, ok := latestCloseBySymbol[positions[i].Symbol]; ok && latestClose > 0 {
			marketValue = positions[i].Shares * latestClose
		}
		if marketValue < 0 {
			marketValue = 0
		}
		positions[i].MarketValue = marketValue
		totalValue += marketValue
	}
	if totalValue > 0 {
		for i := range positions {
			positions[i].Weight = positions[i].MarketValue / totalValue
		}
	}
	sort.Slice(positions, func(i, j int) bool {
		if positions[i].Weight == positions[j].Weight {
			return positions[i].Symbol < positions[j].Symbol
		}
		return positions[i].Weight > positions[j].Weight
	})
	return positions, totalValue, nil
}

func (r *RiskRepository) loadLatestCloseBySymbol(ctx context.Context, positions []positionWithWeight) (map[string]float64, error) {
	result := make(map[string]float64, len(positions))
	if len(positions) == 0 {
		return result, nil
	}
	codes := make([]string, 0, len(positions))
	seen := make(map[string]struct{}, len(positions))
	for _, position := range positions {
		code := positionHistoryCode(position)
		if code == "" {
			continue
		}
		if _, ok := seen[code]; ok {
			continue
		}
		seen[code] = struct{}{}
		codes = append(codes, code)
	}
	barsByCode, err := r.riskDBRepository().GetDailyBarsForPeriod(ctx, codes, 15)
	if err != nil {
		return nil, err
	}
	latestCloseByCode := make(map[string]float64, len(barsByCode))
	for code, bars := range barsByCode {
		if len(bars) == 0 {
			continue
		}
		last := bars[len(bars)-1]
		if last.Close > 0 {
			latestCloseByCode[code] = last.Close
		}
	}
	for _, position := range positions {
		if latestClose, ok := latestCloseByCode[positionHistoryCode(position)]; ok {
			result[position.Symbol] = latestClose
		}
	}
	return result, nil
}

// calcConcentrationRisk 计算集中度风险
func (r *RiskRepository) calcConcentrationRisk(positions []positionWithWeight, totalValue float64) *ConcentrationRiskMetrics {
	metrics := &ConcentrationRiskMetrics{Warnings: []string{}}
	if len(positions) == 0 || totalValue == 0 {
		return metrics
	}
	metrics.SingleStockMaxWeight = positions[0].Weight
	for i, position := range positions {
		if i < 3 {
			metrics.Top3Weight += position.Weight
		}
		if i < 5 {
			metrics.Top5Weight += position.Weight
		}
		metrics.HerfindahlIndex += position.Weight * position.Weight
	}
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

func (r *RiskRepository) buildPortfolioHistoricalAnalytics(ctx context.Context, positions []positionWithWeight, days int) (*portfolioHistoricalAnalytics, error) {
	analytics := &portfolioHistoricalAnalytics{
		ReturnsBySymbol: make(map[string]map[string]float64),
		Warnings:        []string{},
	}
	if len(positions) == 0 {
		return analytics, nil
	}

	codes := make([]string, 0, len(positions))
	seen := make(map[string]struct{}, len(positions))
	for _, position := range positions {
		code := positionHistoryCode(position)
		if code == "" {
			continue
		}
		if _, ok := seen[code]; ok {
			continue
		}
		seen[code] = struct{}{}
		codes = append(codes, code)
	}
	barsByCode, err := r.riskDBRepository().GetDailyBarsForPeriod(ctx, codes, days)
	if err != nil {
		return nil, err
	}

	dateSet := make(map[string]struct{})
	missingCount := 0
	missingWeight := 0.0
	for _, position := range positions {
		returnsByDate := buildSymbolDailyReturnMap(barsByCode[positionHistoryCode(position)])
		if len(returnsByDate) == 0 {
			missingCount++
			missingWeight += position.Weight
			continue
		}
		analytics.ReturnsBySymbol[position.Symbol] = returnsByDate
		for date := range returnsByDate {
			dateSet[date] = struct{}{}
		}
	}
	if missingCount > 0 {
		analytics.Warnings = append(analytics.Warnings, fmt.Sprintf("%d只股票缺少历史日线，缺失权重约%.1f%%", missingCount, missingWeight*100))
	}
	if len(dateSet) == 0 {
		analytics.Warnings = append(analytics.Warnings, "缺少可用历史收益率数据")
		return analytics, nil
	}

	dates := make([]string, 0, len(dateSet))
	for date := range dateSet {
		dates = append(dates, date)
	}
	sort.Strings(dates)

	for _, date := range dates {
		weightedReturn := 0.0
		coveredWeight := 0.0
		for _, position := range positions {
			ret, ok := analytics.ReturnsBySymbol[position.Symbol][date]
			if !ok {
				continue
			}
			weightedReturn += position.Weight * ret
			coveredWeight += position.Weight
		}
		if coveredWeight < riskCoverageThreshold {
			continue
		}
		analytics.DailyReturns = append(analytics.DailyReturns, weightedReturn/coveredWeight)
	}
	if len(analytics.DailyReturns) < riskMinSampleDays {
		analytics.Warnings = append(analytics.Warnings, fmt.Sprintf("有效历史样本仅%d天，少于%d天", len(analytics.DailyReturns), riskMinSampleDays))
	}
	analytics.WeeklyReturns = buildRollingCompoundReturns(analytics.DailyReturns, 5)
	return analytics, nil
}

func buildSymbolDailyReturnMap(bars []DailyBarRecord) map[string]float64 {
	result := make(map[string]float64)
	if len(bars) < 2 {
		return result
	}
	for i := 1; i < len(bars); i++ {
		prevClose := bars[i-1].Close
		closePrice := bars[i].Close
		if prevClose <= 0 || closePrice <= 0 {
			continue
		}
		result[bars[i].Date] = closePrice/prevClose - 1
	}
	return result
}

func buildRollingCompoundReturns(returns []float64, window int) []float64 {
	if window <= 0 || len(returns) < window {
		return nil
	}
	result := make([]float64, 0, len(returns)-window+1)
	for i := window - 1; i < len(returns); i++ {
		value := 1.0
		for j := i - window + 1; j <= i; j++ {
			value *= 1 + returns[j]
		}
		result = append(result, value-1)
	}
	return result
}

// calcVolatilityRisk 计算波动率风险
func (r *RiskRepository) calcVolatilityRisk(analytics *portfolioHistoricalAnalytics) *VolatilityRiskMetrics {
	metrics := &VolatilityRiskMetrics{Warnings: append([]string{}, analyticsWarnings(analytics)...)}
	if analytics == nil || len(analytics.DailyReturns) == 0 {
		if len(metrics.Warnings) == 0 {
			metrics.Warnings = append(metrics.Warnings, "缺少历史收益率数据")
		}
		return metrics
	}
	metrics.AnnualizedVolatility = standardDeviation(analytics.DailyReturns) * math.Sqrt(252)
	metrics.MaxDrawdown = computeMaxDrawdown(analytics.DailyReturns)
	metrics.DownsideProbability = negativeReturnRatio(analytics.DailyReturns)
	metrics.DailyVar95 = empiricalQuantile(analytics.DailyReturns, 0.05)
	metrics.WeeklyVar95 = empiricalQuantile(analytics.WeeklyReturns, 0.05)
	return metrics
}

// calcLiquidityRisk 计算流动性风险
func (r *RiskRepository) calcLiquidityRisk(ctx context.Context, positions []positionWithWeight, totalValue float64, scope string) (*LiquidityRiskMetrics, error) {
	metrics := &LiquidityRiskMetrics{Warnings: []string{}}
	filteredPositions := filterRiskPositionsByScope(positions, scope)
	if len(filteredPositions) == 0 {
		return metrics, nil
	}

	type liquidityRecord struct {
		AvgAmount5d float64
		Liquidity   float64
	}
	liquidityMap := make(map[string]liquidityRecord)
	aShareCodes := make([]string, 0, len(filteredPositions))
	hkCodes := make([]string, 0, len(filteredPositions))
	for _, position := range filteredPositions {
		code := positionHistoryCode(position)
		if code == "" {
			continue
		}
		if inferRiskPositionScope(position) == PortfolioScopeHK {
			hkCodes = append(hkCodes, code)
		} else {
			aShareCodes = append(aShareCodes, code)
		}
	}

	if len(aShareCodes) > 0 && r.cacheDB != nil {
		var records []quadrant.QuadrantScoreRecord
		err := r.cacheDB.WithContext(ctx).
			Where("code IN (?) AND computed_at = (SELECT MAX(computed_at) FROM quadrant_scores)", aShareCodes).
			Find(&records).Error
		if err != nil {
			log.Printf("查询A股流动性数据失败: %v", err)
		} else {
			for _, record := range records {
				liquidityMap[record.Code] = liquidityRecord{AvgAmount5d: record.AvgAmount5d, Liquidity: record.Liquidity}
			}
		}
	}
	if len(hkCodes) > 0 {
		if r.hkCacheDB == nil {
			metrics.Warnings = append(metrics.Warnings, "港股流动性缓存不可用")
		} else {
			var records []quadrant.QuadrantScoreRecord
			err := r.hkCacheDB.WithContext(ctx).
				Where("code IN (?) AND computed_at = (SELECT MAX(computed_at) FROM quadrant_scores)", hkCodes).
				Find(&records).Error
			if err != nil {
				log.Printf("查询港股流动性数据失败: %v", err)
			} else {
				for _, record := range records {
					liquidityMap[record.Code] = liquidityRecord{AvgAmount5d: record.AvgAmount5d, Liquidity: record.Liquidity}
				}
			}
		}
	}

	weightedTurnoverSum := 0.0
	weightedLiquiditySum := 0.0
	totalCoveredWeight := 0.0
	for _, position := range filteredPositions {
		liquidity, ok := liquidityMap[positionHistoryCode(position)]
		if !ok {
			continue
		}
		weightedTurnoverSum += liquidity.AvgAmount5d * position.Weight
		weightedLiquiditySum += liquidity.Liquidity * position.Weight
		totalCoveredWeight += position.Weight
		if liquidity.AvgAmount5d < 100 {
			metrics.IlliquidCount++
			metrics.LowLiquidityWeight += position.Weight
		}
	}
	if totalCoveredWeight > 0 {
		metrics.AvgDailyTurnover = weightedTurnoverSum / totalCoveredWeight
		metrics.LiquidityScore = weightedLiquiditySum / totalCoveredWeight * 100
	}
	metrics.AvgTurnoverRate = 0
	metrics.Warnings = append(metrics.Warnings, "换手率数据暂不可用，已返回0")
	if metrics.IlliquidCount > 0 {
		metrics.Warnings = append(metrics.Warnings, fmt.Sprintf("有%d只股票流动性较低（成交额<100万元），权重合计%.1f%%", metrics.IlliquidCount, metrics.LowLiquidityWeight*100))
	}
	if len(liquidityMap) < len(filteredPositions) {
		metrics.Warnings = append(metrics.Warnings, fmt.Sprintf("%d只股票缺少流动性数据", len(filteredPositions)-len(liquidityMap)))
	}
	return metrics, nil
}

// calcTailRisk 计算尾部风险
func (r *RiskRepository) calcTailRisk(analytics *portfolioHistoricalAnalytics) *TailRiskMetrics {
	metrics := &TailRiskMetrics{Warnings: append([]string{}, analyticsWarnings(analytics)...)}
	if analytics == nil || len(analytics.DailyReturns) == 0 {
		if len(metrics.Warnings) == 0 {
			metrics.Warnings = append(metrics.Warnings, "缺少历史收益率数据")
		}
		return metrics
	}
	metrics.Var95OneDay = empiricalQuantile(analytics.DailyReturns, 0.05)
	metrics.Var95OneWeek = empiricalQuantile(analytics.WeeklyReturns, 0.05)
	metrics.ExpectedShortfall95 = expectedShortfall(analytics.DailyReturns, metrics.Var95OneDay)
	metrics.WorstCaseLoss = minFloat64Slice(analytics.DailyReturns)
	return metrics
}

// calcCorrelationRisk 计算相关性风险
func (r *RiskRepository) calcCorrelationRisk(positions []positionWithWeight, analytics *portfolioHistoricalAnalytics) *CorrelationRiskMetrics {
	metrics := &CorrelationRiskMetrics{Warnings: append([]string{}, analyticsWarnings(analytics)...)}
	if analytics == nil || len(analytics.ReturnsBySymbol) == 0 {
		if len(metrics.Warnings) == 0 {
			metrics.Warnings = append(metrics.Warnings, "缺少历史收益率数据")
		}
		metrics.DiversificationScore = 50
		return metrics
	}

	candidates := make([]positionWithWeight, 0, len(positions))
	for _, position := range positions {
		if len(analytics.ReturnsBySymbol[position.Symbol]) == 0 {
			continue
		}
		candidates = append(candidates, position)
	}
	if len(candidates) < 2 {
		metrics.Warnings = append(metrics.Warnings, "有效持仓少于2只，无法计算相关性")
		metrics.DiversificationScore = 50
		return metrics
	}
	if len(candidates) > correlationTopPositionLimit {
		metrics.Warnings = append(metrics.Warnings, fmt.Sprintf("相关性仅基于前%d大持仓计算", correlationTopPositionLimit))
		candidates = candidates[:correlationTopPositionLimit]
	}

	correlations := make([]float64, 0)
	highPairs := make([]HighCorrelationPair, 0)
	for i := 0; i < len(candidates); i++ {
		for j := i + 1; j < len(candidates); j++ {
			xs, ys := alignSymbolReturns(analytics.ReturnsBySymbol[candidates[i].Symbol], analytics.ReturnsBySymbol[candidates[j].Symbol], correlationHistoryDays)
			if len(xs) < correlationMinSampleDays {
				continue
			}
			corr := pearsonCorrelation(xs, ys)
			correlations = append(correlations, corr)
			if corr > highCorrelationThreshold {
				highPairs = append(highPairs, HighCorrelationPair{Symbol1: candidates[i].Symbol, Symbol2: candidates[j].Symbol, Correlation: corr})
			}
		}
	}
	if len(correlations) == 0 {
		metrics.Warnings = append(metrics.Warnings, "可用的重叠历史样本不足，无法形成相关性统计")
		metrics.DiversificationScore = 50
		return metrics
	}

	metrics.AvgCorrelation = average(correlations)
	metrics.DiversificationScore = 100 * (1 - clamp(metrics.AvgCorrelation, 0, 1))
	sort.Slice(highPairs, func(i, j int) bool {
		if highPairs[i].Correlation == highPairs[j].Correlation {
			if highPairs[i].Symbol1 == highPairs[j].Symbol1 {
				return highPairs[i].Symbol2 < highPairs[j].Symbol2
			}
			return highPairs[i].Symbol1 < highPairs[j].Symbol1
		}
		return highPairs[i].Correlation > highPairs[j].Correlation
	})
	if len(highPairs) > highCorrelationPairsReturnLimit {
		highPairs = highPairs[:highCorrelationPairsReturnLimit]
	}
	metrics.HighCorrelationPairs = highPairs
	if len(highPairs) > 0 {
		metrics.Warnings = append(metrics.Warnings, fmt.Sprintf("存在%d对高相关持仓（相关系数>%.1f）", len(highPairs), highCorrelationThreshold))
	}
	return metrics
}

// calcOverallRiskScore 计算综合风险评分（1-10分）
func (r *RiskRepository) calcOverallRiskScore(metrics *RiskMetrics) float64 {
	score := 5.0
	if metrics != nil && metrics.ConcentrationRisk != nil {
		if metrics.ConcentrationRisk.SingleStockMaxWeight > 0.3 {
			score += 2.0
		} else if metrics.ConcentrationRisk.SingleStockMaxWeight > 0.2 {
			score += 1.0
		}
		if metrics.ConcentrationRisk.Top3Weight > 0.7 {
			score += 2.0
		} else if metrics.ConcentrationRisk.Top3Weight > 0.5 {
			score += 1.0
		}
		if metrics.ConcentrationRisk.HerfindahlIndex > 0.3 {
			score += 1.0
		}
	}
	if score < 1.0 {
		score = 1.0
	}
	if score > 10.0 {
		score = 10.0
	}
	return math.Round(score*10) / 10
}

func concentrationRiskScore(metrics *ConcentrationRiskMetrics) float64 {
	if metrics == nil {
		return 5
	}
	return average([]float64{
		clamp(metrics.SingleStockMaxWeight/0.35*10, 0, 10),
		clamp(metrics.Top3Weight/0.80*10, 0, 10),
		clamp(metrics.HerfindahlIndex/0.35*10, 0, 10),
	})
}

func volatilityRiskScore(metrics *VolatilityRiskMetrics) float64 {
	if metrics == nil {
		return 5
	}
	return average([]float64{
		clamp(metrics.AnnualizedVolatility/0.40*10, 0, 10),
		clamp(math.Abs(metrics.MaxDrawdown)/0.35*10, 0, 10),
		clamp(metrics.DownsideProbability/0.60*10, 0, 10),
		clamp(math.Abs(metrics.DailyVar95)/0.05*10, 0, 10),
	})
}

func liquidityRiskScore(metrics *LiquidityRiskMetrics) float64 {
	if metrics == nil {
		return 5
	}
	return average([]float64{
		clamp((100-metrics.LiquidityScore)/10, 0, 10),
		clamp(metrics.LowLiquidityWeight*10, 0, 10),
		clamp(float64(metrics.IlliquidCount)/5*10, 0, 10),
	})
}

func tailRiskScore(metrics *TailRiskMetrics) float64 {
	if metrics == nil {
		return 5
	}
	return average([]float64{
		clamp(math.Abs(metrics.Var95OneDay)/0.05*10, 0, 10),
		clamp(math.Abs(metrics.Var95OneWeek)/0.12*10, 0, 10),
		clamp(math.Abs(metrics.ExpectedShortfall95)/0.07*10, 0, 10),
		clamp(math.Abs(metrics.WorstCaseLoss)/0.10*10, 0, 10),
	})
}

func correlationRiskScore(metrics *CorrelationRiskMetrics) float64 {
	if metrics == nil {
		return 5
	}
	return average([]float64{
		clamp(metrics.AvgCorrelation*10, 0, 10),
		clamp((100-metrics.DiversificationScore)/10, 0, 10),
		clamp(float64(len(metrics.HighCorrelationPairs))/5*10, 0, 10),
	})
}

func analyticsWarnings(analytics *portfolioHistoricalAnalytics) []string {
	if analytics == nil || len(analytics.Warnings) == 0 {
		return nil
	}
	return append([]string{}, analytics.Warnings...)
}

func computeMaxDrawdown(returns []float64) float64 {
	if len(returns) == 0 {
		return 0
	}
	equity := 1.0
	peak := 1.0
	maxDrawdown := 0.0
	for _, ret := range returns {
		equity *= 1 + ret
		if equity > peak {
			peak = equity
		}
		drawdown := equity/peak - 1
		if drawdown < maxDrawdown {
			maxDrawdown = drawdown
		}
	}
	return maxDrawdown
}

func negativeReturnRatio(returns []float64) float64 {
	if len(returns) == 0 {
		return 0
	}
	count := 0
	for _, ret := range returns {
		if ret < 0 {
			count++
		}
	}
	return float64(count) / float64(len(returns))
}

func empiricalQuantile(values []float64, p float64) float64 {
	if len(values) == 0 {
		return 0
	}
	cloned := append([]float64(nil), values...)
	sort.Float64s(cloned)
	if p <= 0 {
		return cloned[0]
	}
	if p >= 1 {
		return cloned[len(cloned)-1]
	}
	index := int(math.Floor(p * float64(len(cloned)-1)))
	if index < 0 {
		index = 0
	}
	if index >= len(cloned) {
		index = len(cloned) - 1
	}
	return cloned[index]
}

func expectedShortfall(values []float64, threshold float64) float64 {
	if len(values) == 0 {
		return 0
	}
	sum := 0.0
	count := 0
	for _, value := range values {
		if value <= threshold {
			sum += value
			count++
		}
	}
	if count == 0 {
		return threshold
	}
	return sum / float64(count)
}

func minFloat64Slice(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}
	minValue := values[0]
	for _, value := range values[1:] {
		if value < minValue {
			minValue = value
		}
	}
	return minValue
}

func standardDeviation(values []float64) float64 {
	if len(values) < 2 {
		return 0
	}
	mean := average(values)
	variance := 0.0
	for _, value := range values {
		delta := value - mean
		variance += delta * delta
	}
	variance /= float64(len(values) - 1)
	if variance < 0 {
		return 0
	}
	return math.Sqrt(variance)
}

func alignSymbolReturns(left map[string]float64, right map[string]float64, limit int) ([]float64, []float64) {
	commonDates := make([]string, 0)
	for date := range left {
		if _, ok := right[date]; ok {
			commonDates = append(commonDates, date)
		}
	}
	sort.Strings(commonDates)
	if limit > 0 && len(commonDates) > limit {
		commonDates = commonDates[len(commonDates)-limit:]
	}
	xs := make([]float64, 0, len(commonDates))
	ys := make([]float64, 0, len(commonDates))
	for _, date := range commonDates {
		xs = append(xs, left[date])
		ys = append(ys, right[date])
	}
	return xs, ys
}

func pearsonCorrelation(xs []float64, ys []float64) float64 {
	if len(xs) != len(ys) || len(xs) < 2 {
		return 0
	}
	meanX := average(xs)
	meanY := average(ys)
	numerator := 0.0
	varianceX := 0.0
	varianceY := 0.0
	for i := range xs {
		dx := xs[i] - meanX
		dy := ys[i] - meanY
		numerator += dx * dy
		varianceX += dx * dx
		varianceY += dy * dy
	}
	if varianceX == 0 || varianceY == 0 {
		return 0
	}
	return numerator / math.Sqrt(varianceX*varianceY)
}

func average(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}
	sum := 0.0
	for _, value := range values {
		sum += value
	}
	return sum / float64(len(values))
}

func clamp(value, minValue, maxValue float64) float64 {
	if value < minValue {
		return minValue
	}
	if value > maxValue {
		return maxValue
	}
	return value
}

// GetRiskMetricsJSON 返回JSON格式的风险指标（用于API响应）
func (r *RiskRepository) GetRiskMetricsJSON(ctx context.Context, userID string, scope string) ([]byte, error) {
	metrics, err := r.GetRiskMetrics(ctx, userID, scope)
	if err != nil {
		return nil, err
	}
	if metrics.CorrelationRisk != nil {
		metrics.CorrelationRisk.CorrelationMatrix = nil
	}
	return json.Marshal(metrics)
}

func isHKCode(symbol string) bool {
	return isNumeric(strings.TrimSpace(symbol)) && len(strings.TrimSpace(symbol)) == 5
}
