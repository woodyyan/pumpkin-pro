package factorindex

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/woodyyan/pumpkin-pro/backend/store/factorlab"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type Repository struct {
	db *gorm.DB
}

func NewRepository(db *gorm.DB) *Repository {
	return &Repository{db: db}
}

func (r *Repository) EnsureDefaultDefinitions(ctx context.Context) error {
	now := time.Now().UTC()
	records := make([]Definition, 0, len(defaultDefinitions))
	for _, item := range defaultDefinitions {
		records = append(records, Definition{
			ID:        item.ID,
			FactorKey: item.FactorKey,
			Name:      item.Name,
			Exchange:  ExchangeAShare,
			BaseNAV:   defaultBaseNAV,
			TopN:      defaultTopN,
			Weight:    defaultWeight,
			IsActive:  true,
			CreatedAt: now,
			UpdatedAt: now,
		})
	}
	return r.db.WithContext(ctx).Clauses(clause.OnConflict{UpdateAll: true}).Create(&records).Error
}

func (r *Repository) ListActiveDefinitions(ctx context.Context, exchange string) ([]Definition, error) {
	query := r.db.WithContext(ctx).Model(&Definition{}).Where("is_active = 1")
	if strings.TrimSpace(exchange) != "" {
		query = query.Where("exchange = ?", strings.ToUpper(strings.TrimSpace(exchange)))
	}
	var rows []Definition
	if err := query.Order("created_at ASC").Find(&rows).Error; err != nil {
		return nil, err
	}
	return rows, nil
}

// PoolSource 是卧龙股池的可复现来源。它只引用已物化的最新日度指数行，
// 因此 constituent 的调仓批次、数据日期和状态始终保持一致。
type PoolSource struct {
	Definition Definition
	Daily      *Daily
	Rebalance  *Rebalance
	Items      []Constituent
}

// ListCurrentPoolSources 返回当前每个活跃单因子指数的 TopN 成分。
// "当前"定义为该指数最新已物化日度记录关联的 rebalance_id，而不是独立查询
// 最新调仓记录，避免日度指数、月度成分来自不同批次而发生口径漂移。
func (r *Repository) ListCurrentPoolSources(ctx context.Context, exchange string, topN int) ([]PoolSource, error) {
	if topN <= 0 {
		topN = 10
	}
	definitions, err := r.ListActiveDefinitions(ctx, exchange)
	if err != nil {
		return nil, err
	}
	definitionByKey := make(map[string]Definition, len(definitions))
	for _, definition := range definitions {
		definitionByKey[definition.FactorKey] = definition
	}

	ordered := make([]Definition, 0, len(definitions))
	for _, item := range defaultDefinitions {
		if definition, ok := definitionByKey[item.FactorKey]; ok {
			ordered = append(ordered, definition)
			delete(definitionByKey, item.FactorKey)
		}
	}
	extraDefinitions := make([]Definition, 0, len(definitionByKey))
	for _, definition := range definitionByKey {
		extraDefinitions = append(extraDefinitions, definition)
	}
	sort.Slice(extraDefinitions, func(left, right int) bool {
		return extraDefinitions[left].FactorKey < extraDefinitions[right].FactorKey
	})
	ordered = append(ordered, extraDefinitions...)

	result := make([]PoolSource, 0, len(ordered))
	for _, definition := range ordered {
		daily, err := r.LatestDaily(ctx, definition.ID)
		if err != nil {
			return nil, err
		}
		source := PoolSource{Definition: definition, Daily: daily, Items: []Constituent{}}
		if daily == nil || strings.TrimSpace(daily.RebalanceID) == "" {
			result = append(result, source)
			continue
		}
		rebalance, err := r.GetRebalanceByID(ctx, daily.RebalanceID)
		if err != nil {
			return nil, err
		}
		items, err := r.ListTopConstituentsByRebalance(ctx, daily.RebalanceID, topN)
		if err != nil {
			return nil, err
		}
		source.Rebalance = rebalance
		source.Items = items
		result = append(result, source)
	}
	return result, nil
}

func (r *Repository) ListSnapshotDates(ctx context.Context) ([]string, error) {
	var dates []string
	err := r.db.WithContext(ctx).Model(&factorlab.FactorScore{}).
		Distinct("snapshot_date").
		Order("snapshot_date ASC").
		Pluck("snapshot_date", &dates).Error
	return dates, err
}

func (r *Repository) LatestSnapshotDate(ctx context.Context) (string, error) {
	var value string
	err := r.db.WithContext(ctx).Model(&factorlab.FactorScore{}).
		Select("COALESCE(MAX(snapshot_date), '')").
		Scan(&value).Error
	return strings.TrimSpace(value), err
}

func (r *Repository) ListTradeDates(ctx context.Context) ([]string, error) {
	var dates []string
	err := r.db.WithContext(ctx).Model(&factorlab.FactorDailyBar{}).
		Distinct("trade_date").
		Order("trade_date ASC").
		Pluck("trade_date", &dates).Error
	return dates, err
}

func (r *Repository) LatestTradeDate(ctx context.Context) (string, error) {
	var value string
	err := r.db.WithContext(ctx).Model(&factorlab.FactorDailyBar{}).
		Select("COALESCE(MAX(trade_date), '')").
		Scan(&value).Error
	return strings.TrimSpace(value), err
}

// ListTradeDateBarCounts 返回每个交易日有日线记录的去重股票数，用于
// NAV 计算前的覆盖率门槛判断：单日覆盖数显著低于基线时说明当日数据尚未补齐。
func (r *Repository) ListTradeDateBarCounts(ctx context.Context) (map[string]int, error) {
	type row struct {
		TradeDate string
		Cnt       int
	}
	var rows []row
	err := r.db.WithContext(ctx).Model(&factorlab.FactorDailyBar{}).
		Select("trade_date, COUNT(DISTINCT code) AS cnt").
		Group("trade_date").
		Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	result := make(map[string]int, len(rows))
	for _, item := range rows {
		result[strings.TrimSpace(item.TradeDate)] = item.Cnt
	}
	return result, nil
}

// ListSuspendedCodes 返回在 tradeDate 应视为「停牌或已退市」的股票集合。
// 判定口径：每只股票取 tradeDate 当日或之前最近一条 factor_market_metrics 记录，
// 若该记录 is_suspended=1，且股票在 tradeDate 无日线（由调用方保证此前置），
// 则视为停牌/退市——当日无数据是正常市场现象，按 0 收益处理在经济上正确，
// 不计入数据缺失。取「最近一条」而非「当日」，是为了覆盖退市股：数据源在其
// 退市后会停止推送快照，但最近一条快照的停牌标记仍然成立。
func (r *Repository) ListSuspendedCodes(ctx context.Context, codes []string, tradeDate string) (map[string]bool, error) {
	cleanCodes := make([]string, 0, len(codes))
	seen := map[string]struct{}{}
	for _, code := range codes {
		value := strings.TrimSpace(code)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		cleanCodes = append(cleanCodes, value)
	}
	result := make(map[string]bool, len(cleanCodes))
	if len(cleanCodes) == 0 {
		return result, nil
	}
	var suspended []string
	err := r.db.WithContext(ctx).Model(&factorlab.FactorMarketMetric{}).
		Select("factor_market_metrics.code").
		Joins("JOIN (SELECT code, MAX(trade_date) AS max_date FROM factor_market_metrics WHERE trade_date <= ? AND code IN ? GROUP BY code) latest ON latest.code = factor_market_metrics.code AND latest.max_date = factor_market_metrics.trade_date", strings.TrimSpace(tradeDate), cleanCodes).
		Where("factor_market_metrics.is_suspended = 1").
		Pluck("factor_market_metrics.code", &suspended).Error
	if err != nil {
		return nil, err
	}
	for _, code := range suspended {
		result[strings.TrimSpace(code)] = true
	}
	return result, nil
}

func (r *Repository) RebalanceExists(ctx context.Context, indexID, signalDate string) (bool, error) {
	var count int64
	err := r.db.WithContext(ctx).Model(&Rebalance{}).
		Where("index_id = ? AND signal_date = ?", strings.TrimSpace(indexID), strings.TrimSpace(signalDate)).
		Count(&count).Error
	return count > 0, err
}

type scoreSelectionRow struct {
	Code       string
	Name       string
	Exchange   string
	Industry   string
	ClosePrice float64
	Score      float64
}

func (r *Repository) ListTopScores(ctx context.Context, snapshotDate, scoreField string, limit int) ([]scoreSelectionRow, error) {
	column := strings.TrimSpace(scoreField)
	if column == "" {
		return nil, fmt.Errorf("score field is required")
	}
	if limit <= 0 {
		limit = defaultTopN
	}
	// Exclude stocks that are explicitly inactive (delisted) or flagged as ST.
	// COALESCE defaults handle the case where no matching row exists in
	// factor_securities (treat as active, non-ST when the record is absent).
	// is_active and is_st are stored as SQLite numeric booleans (1/0); we use
	// explicit integer comparisons rather than relying on the boolean default
	// to avoid NULL-ambiguity for is_active=false rows written by GORM.
	query := fmt.Sprintf(`
		SELECT fs.code, fs.name, COALESCE(sec.exchange, '') AS exchange, fs.industry, fs.close_price, fs.%s AS score
		FROM factor_scores fs
		LEFT JOIN factor_securities sec ON sec.code = fs.code
		WHERE fs.snapshot_date = ?
		  AND fs.%s IS NOT NULL
		  AND fs.close_price > 0
		  AND COALESCE(sec.is_active, 1) <> 0
		  AND COALESCE(sec.is_st, 0) <> 1
		  AND COALESCE(sec.exchange, '') IN ('SSE', 'SZSE')
		ORDER BY fs.%s DESC, fs.code ASC
		LIMIT ?
	`, column, column, column)
	var rows []scoreSelectionRow
	if err := r.db.WithContext(ctx).Raw(query, strings.TrimSpace(snapshotDate), limit).Scan(&rows).Error; err != nil {
		return nil, err
	}
	return rows, nil
}

func (r *Repository) SaveRebalance(ctx context.Context, rebalance Rebalance, constituents []Constituent) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Clauses(clause.OnConflict{UpdateAll: true}).Create(&rebalance).Error; err != nil {
			return err
		}
		if err := tx.Where("rebalance_id = ?", rebalance.ID).Delete(&Constituent{}).Error; err != nil {
			return err
		}
		if len(constituents) > 0 {
			if err := tx.Create(&constituents).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

func (r *Repository) LatestRebalanceBeforeTradeDate(ctx context.Context, indexID, tradeDate string) (*Rebalance, error) {
	var row Rebalance
	err := r.db.WithContext(ctx).Model(&Rebalance{}).
		Where("index_id = ? AND signal_date < ? AND constituent_count > 0 AND status IN ?", strings.TrimSpace(indexID), strings.TrimSpace(tradeDate), []string{StatusCompleted, StatusPartial}).
		Order("signal_date DESC").
		First(&row).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, nil
		}
		return nil, err
	}
	return &row, nil
}

func (r *Repository) UpdateRebalanceActivation(ctx context.Context, rebalanceID, effectiveStartDate string) error {
	updates := map[string]any{
		"effective_start_date": strings.TrimSpace(effectiveStartDate),
		"updated_at":           time.Now().UTC(),
	}
	return r.db.WithContext(ctx).Model(&Rebalance{}).Where("id = ?", strings.TrimSpace(rebalanceID)).Updates(updates).Error
}

func (r *Repository) ClosePreviousRebalances(ctx context.Context, indexID, currentRebalanceID, effectiveEndDate string) error {
	return r.db.WithContext(ctx).Model(&Rebalance{}).
		Where("index_id = ? AND id <> ? AND effective_start_date <> '' AND (effective_end_date = '' OR effective_end_date > ?)", strings.TrimSpace(indexID), strings.TrimSpace(currentRebalanceID), strings.TrimSpace(effectiveEndDate)).
		Updates(map[string]any{"effective_end_date": strings.TrimSpace(effectiveEndDate), "updated_at": time.Now().UTC()}).Error
}

func (r *Repository) ListConstituentsByRebalance(ctx context.Context, rebalanceID string) ([]Constituent, error) {
	var rows []Constituent
	err := r.db.WithContext(ctx).Model(&Constituent{}).
		Where("rebalance_id = ?", strings.TrimSpace(rebalanceID)).
		Order("rank ASC, stock_code ASC").
		Find(&rows).Error
	return rows, err
}

func (r *Repository) ListTopConstituentsByRebalance(ctx context.Context, rebalanceID string, limit int) ([]Constituent, error) {
	if limit <= 0 {
		limit = 10
	}
	var rows []Constituent
	err := r.db.WithContext(ctx).Model(&Constituent{}).
		Where("rebalance_id = ? AND rank > 0", strings.TrimSpace(rebalanceID)).
		Order("rank ASC, stock_code ASC").
		Limit(limit).
		Find(&rows).Error
	return rows, err
}

func (r *Repository) GetRebalanceByID(ctx context.Context, rebalanceID string) (*Rebalance, error) {
	var row Rebalance
	err := r.db.WithContext(ctx).Where("id = ?", strings.TrimSpace(rebalanceID)).First(&row).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, nil
		}
		return nil, err
	}
	return &row, nil
}

func (r *Repository) GetDailyByTradeDate(ctx context.Context, indexID, tradeDate string) (*Daily, error) {
	var row Daily
	err := r.db.WithContext(ctx).Model(&Daily{}).
		Where("index_id = ? AND trade_date = ?", strings.TrimSpace(indexID), strings.TrimSpace(tradeDate)).
		First(&row).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, nil
		}
		return nil, err
	}
	return &row, nil
}

func (r *Repository) LatestDaily(ctx context.Context, indexID string) (*Daily, error) {
	var row Daily
	err := r.db.WithContext(ctx).Model(&Daily{}).
		Where("index_id = ?", strings.TrimSpace(indexID)).
		Order("trade_date DESC").
		First(&row).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, nil
		}
		return nil, err
	}
	return &row, nil
}

func (r *Repository) LatestDailyBeforeTradeDate(ctx context.Context, indexID, tradeDate string) (*Daily, error) {
	var row Daily
	err := r.db.WithContext(ctx).Model(&Daily{}).
		Where("index_id = ? AND trade_date < ?", strings.TrimSpace(indexID), strings.TrimSpace(tradeDate)).
		Order("trade_date DESC").
		First(&row).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, nil
		}
		return nil, err
	}
	return &row, nil
}

func (r *Repository) ListRecentDailyRows(ctx context.Context, indexID, tradeDate string, limit int) ([]Daily, error) {
	if limit <= 0 {
		limit = 20
	}
	var rows []Daily
	err := r.db.WithContext(ctx).Model(&Daily{}).
		Where("index_id = ? AND trade_date <= ?", strings.TrimSpace(indexID), strings.TrimSpace(tradeDate)).
		Order("trade_date DESC").
		Limit(limit).
		Find(&rows).Error
	if err != nil {
		return nil, err
	}
	for left, right := 0, len(rows)-1; left < right; left, right = left+1, right-1 {
		rows[left], rows[right] = rows[right], rows[left]
	}
	return rows, nil
}

func (r *Repository) SaveDaily(ctx context.Context, row Daily) error {
	return r.db.WithContext(ctx).Clauses(clause.OnConflict{UpdateAll: true}).Create(&row).Error
}

type priceWindowRow struct {
	Code      string
	TradeDate string
	Close     float64
}

func (r *Repository) ListPriceWindows(ctx context.Context, codes []string, tradeDate string) (map[string][]priceWindowRow, error) {
	cleanCodes := make([]string, 0, len(codes))
	seen := map[string]struct{}{}
	for _, code := range codes {
		value := strings.TrimSpace(code)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		cleanCodes = append(cleanCodes, value)
	}
	if len(cleanCodes) == 0 {
		return map[string][]priceWindowRow{}, nil
	}
	query := `
		SELECT code, trade_date, close
		FROM (
			SELECT code, trade_date, close,
			       ROW_NUMBER() OVER (PARTITION BY code ORDER BY trade_date DESC) AS rn
			FROM factor_daily_bars
			WHERE trade_date <= ? AND code IN ?
		)
		WHERE rn <= 2
		ORDER BY code ASC, trade_date DESC
	`
	var rows []priceWindowRow
	if err := r.db.WithContext(ctx).Raw(query, strings.TrimSpace(tradeDate), cleanCodes).Scan(&rows).Error; err != nil {
		return nil, err
	}
	result := make(map[string][]priceWindowRow, len(cleanCodes))
	for _, row := range rows {
		result[row.Code] = append(result[row.Code], row)
	}
	return result, nil
}
