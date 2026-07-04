package quadrant

import (
	"context"
	"errors"
	"strings"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

func defaultSimPortfolioV2Definitions(now time.Time) []SimPortfolioV2Definition {
	return []SimPortfolioV2Definition{
		{ID: "spv2_ashare_a", Code: "sim-v2-ashare-a", Name: "模拟组合A", Market: SimPortfolioV2MarketAShare, PortfolioVariant: "A", MaxHoldings: 4, SelectionRule: rankingPortfolioSelectionRuleTop4, WeightingMethod: "equal", InitialAssets: simPortfolioInitialAssets, IsActive: true, CreatedAt: now, UpdatedAt: now},
		{ID: "spv2_ashare_b", Code: "sim-v2-ashare-b", Name: "模拟组合B", Market: SimPortfolioV2MarketAShare, PortfolioVariant: "B", MaxHoldings: 4, SelectionRule: rankingPortfolioSelectionRuleTop10ByStreak, SelectionWindow: 10, WeightingMethod: "equal", InitialAssets: simPortfolioInitialAssets, IsActive: true, CreatedAt: now, UpdatedAt: now},
		{ID: "spv2_hkex_a", Code: "sim-v2-hkex-a", Name: "模拟组合A", Market: SimPortfolioV2MarketHKEX, PortfolioVariant: "A", MaxHoldings: 4, SelectionRule: rankingPortfolioSelectionRuleTop4, WeightingMethod: "equal", InitialAssets: simPortfolioInitialAssets, IsActive: true, CreatedAt: now, UpdatedAt: now},
		{ID: "spv2_hkex_b", Code: "sim-v2-hkex-b", Name: "模拟组合B", Market: SimPortfolioV2MarketHKEX, PortfolioVariant: "B", MaxHoldings: 4, SelectionRule: rankingPortfolioSelectionRuleTop10ByStreak, SelectionWindow: 10, WeightingMethod: "equal", InitialAssets: simPortfolioInitialAssets, IsActive: true, CreatedAt: now, UpdatedAt: now},
	}
}

func (r *Repository) EnsureSimPortfolioV2Definitions(ctx context.Context) error {
	now := time.Now().UTC()
	defs := defaultSimPortfolioV2Definitions(now)
	return r.db.WithContext(ctx).Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "id"}}, DoUpdates: clause.AssignmentColumns([]string{"code", "name", "market", "portfolio_variant", "max_holdings", "selection_rule", "selection_window", "excluded_boards", "weighting_method", "initial_assets", "is_active", "updated_at"})}).Create(&defs).Error
}

func (r *Repository) ListActiveSimPortfolioV2Definitions(ctx context.Context) ([]SimPortfolioV2Definition, error) {
	var defs []SimPortfolioV2Definition
	err := r.db.WithContext(ctx).Where("is_active = ?", true).Order("market ASC, portfolio_variant ASC").Find(&defs).Error
	return defs, err
}

func (r *Repository) UpsertMarketCalendar(ctx context.Context, row MarketCalendar) error {
	return r.db.WithContext(ctx).Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "market"}, {Name: "trade_date"}}, DoUpdates: clause.AssignmentColumns([]string{"is_trading_day", "prev_trade_date", "next_trade_date", "holiday_name", "is_half_day", "source", "updated_at"})}).Create(&row).Error
}

func (r *Repository) UpsertSimPortfolioV2DayStatus(ctx context.Context, row SimPortfolioV2PipelineDayStatus) error {
	return r.db.WithContext(ctx).Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "market"}, {Name: "trade_date"}, {Name: "stage"}}, DoUpdates: clause.AssignmentColumns([]string{"status", "expected_count", "actual_count", "missing_count", "failed_count", "message", "action_hint", "run_id", "updated_at"})}).Create(&row).Error
}

func (r *Repository) CreateSimPortfolioV2Run(ctx context.Context, run *SimPortfolioV2PipelineRun) error {
	return r.db.WithContext(ctx).Create(run).Error
}

func (r *Repository) UpdateSimPortfolioV2Run(ctx context.Context, run *SimPortfolioV2PipelineRun) error {
	return r.db.WithContext(ctx).Save(run).Error
}

func (r *Repository) ListSimPortfolioV2Runs(ctx context.Context, limit int) ([]SimPortfolioV2PipelineRun, error) {
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	var rows []SimPortfolioV2PipelineRun
	err := r.db.WithContext(ctx).Order("started_at DESC").Limit(limit).Find(&rows).Error
	return rows, err
}

func (r *Repository) ListSimPortfolioV2DayStatuses(ctx context.Context, market, fromDate, toDate string) ([]SimPortfolioV2PipelineDayStatus, error) {
	q := r.db.WithContext(ctx).Model(&SimPortfolioV2PipelineDayStatus{}).Order("trade_date DESC, market ASC, stage ASC")
	if strings.TrimSpace(market) != "" && !strings.EqualFold(market, "ALL") {
		q = q.Where("market = ?", normalizeSimPortfolioV2Market(market))
	}
	if strings.TrimSpace(fromDate) != "" {
		q = q.Where("trade_date >= ?", strings.TrimSpace(fromDate))
	}
	if strings.TrimSpace(toDate) != "" {
		q = q.Where("trade_date <= ?", strings.TrimSpace(toDate))
	}
	var rows []SimPortfolioV2PipelineDayStatus
	err := q.Find(&rows).Error
	return rows, err
}

func (r *Repository) GetSimPortfolioV2SignalBatch(ctx context.Context, market, date string) (*SimPortfolioV2SignalBatch, error) {
	var row SimPortfolioV2SignalBatch
	err := r.db.WithContext(ctx).Where("market = ? AND source_trade_date = ?", normalizeSimPortfolioV2Market(market), strings.TrimSpace(date)).First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	return &row, err
}

func (r *Repository) ListSimPortfolioV2SignalItems(ctx context.Context, batchID string) ([]SimPortfolioV2SignalItem, error) {
	var rows []SimPortfolioV2SignalItem
	err := r.db.WithContext(ctx).Where("batch_id = ?", strings.TrimSpace(batchID)).Order("rank ASC, code ASC").Find(&rows).Error
	return rows, err
}

func (r *Repository) ReplaceSimPortfolioV2SignalBatch(ctx context.Context, batch SimPortfolioV2SignalBatch, items []SimPortfolioV2SignalItem) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("market = ? AND source_trade_date = ?", batch.Market, batch.SourceTradeDate).Delete(&SimPortfolioV2SignalItem{}).Error; err != nil {
			return err
		}
		if err := tx.Where("market = ? AND source_trade_date = ?", batch.Market, batch.SourceTradeDate).Delete(&SimPortfolioV2SignalBatch{}).Error; err != nil {
			return err
		}
		if err := tx.Create(&batch).Error; err != nil {
			return err
		}
		if len(items) > 0 {
			return tx.Create(&items).Error
		}
		return nil
	})
}

func (r *Repository) ReplaceSimPortfolioV2Selection(ctx context.Context, batch SimPortfolioV2SelectionBatch, items []SimPortfolioV2SelectionItem) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("portfolio_id = ? AND signal_date = ?", batch.PortfolioID, batch.SignalDate).Delete(&SimPortfolioV2SelectionItem{}).Error; err != nil {
			return err
		}
		if err := tx.Where("portfolio_id = ? AND signal_date = ?", batch.PortfolioID, batch.SignalDate).Delete(&SimPortfolioV2SelectionBatch{}).Error; err != nil {
			return err
		}
		if err := tx.Create(&batch).Error; err != nil {
			return err
		}
		if len(items) > 0 {
			return tx.Create(&items).Error
		}
		return nil
	})
}

func (r *Repository) ListSimPortfolioV2SelectionItems(ctx context.Context, portfolioID, signalDate string) ([]SimPortfolioV2SelectionItem, error) {
	var rows []SimPortfolioV2SelectionItem
	err := r.db.WithContext(ctx).Where("portfolio_id = ? AND signal_date = ?", strings.TrimSpace(portfolioID), strings.TrimSpace(signalDate)).Order("rank ASC, code ASC").Find(&rows).Error
	return rows, err
}

func (r *Repository) ReplaceSimPortfolioV2PriceRequirements(ctx context.Context, portfolioID, signalDate string, rows []SimPortfolioV2PriceRequirement) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("portfolio_id = ? AND signal_date = ?", portfolioID, signalDate).Delete(&SimPortfolioV2PriceRequirement{}).Error; err != nil {
			return err
		}
		if len(rows) > 0 {
			return tx.Create(&rows).Error
		}
		return nil
	})
}

func (r *Repository) ListSimPortfolioV2PriceRequirements(ctx context.Context, portfolioID, signalDate string) ([]SimPortfolioV2PriceRequirement, error) {
	var rows []SimPortfolioV2PriceRequirement
	err := r.db.WithContext(ctx).Where("portfolio_id = ? AND signal_date = ?", portfolioID, signalDate).Order("price_type ASC, code ASC").Find(&rows).Error
	return rows, err
}

func (r *Repository) ListSimPortfolioV2PriceRequirementsForRepair(ctx context.Context, market, signalDate, portfolioID, priceType string, onlyMissing bool) ([]SimPortfolioV2PriceRequirement, error) {
	q := r.db.WithContext(ctx).Model(&SimPortfolioV2PriceRequirement{}).
		Where("market = ? AND signal_date = ?", normalizeSimPortfolioV2Market(market), strings.TrimSpace(signalDate)).
		Order("portfolio_id ASC, price_type ASC, code ASC")
	if strings.TrimSpace(portfolioID) != "" {
		q = q.Where("portfolio_id = ?", strings.TrimSpace(portfolioID))
	}
	if strings.TrimSpace(priceType) != "" {
		q = q.Where("price_type = ?", strings.TrimSpace(priceType))
	}
	if onlyMissing {
		q = q.Where("status <> ? OR price <= 0", SimPortfolioV2PriceStatusSatisfied)
	}
	var rows []SimPortfolioV2PriceRequirement
	err := q.Find(&rows).Error
	return rows, err
}

func (r *Repository) UpdateSimPortfolioV2PriceRequirement(ctx context.Context, row SimPortfolioV2PriceRequirement) error {
	return r.db.WithContext(ctx).Save(&row).Error
}

func (r *Repository) CreateSimPortfolioV2PriceRepairAudit(ctx context.Context, row *SimPortfolioV2PriceRepairAudit) error {
	return r.db.WithContext(ctx).Create(row).Error
}

func (r *Repository) UpdateSimPortfolioV2PriceRepairAudit(ctx context.Context, row *SimPortfolioV2PriceRepairAudit) error {
	return r.db.WithContext(ctx).Save(row).Error
}

func (r *Repository) UpsertSimPortfolioV2PriceOverride(ctx context.Context, row SimPortfolioV2PriceOverride) error {
	return r.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "market"}, {Name: "code"}, {Name: "exchange"}, {Name: "trade_date"}, {Name: "price_type"}},
		DoUpdates: clause.AssignmentColumns([]string{"price", "reason", "evidence", "operator", "audit_id", "updated_at"}),
	}).Create(&row).Error
}

func (r *Repository) GetSimPortfolioV2PriceOverride(ctx context.Context, market, code, exchange, tradeDate, priceType string) (*SimPortfolioV2PriceOverride, error) {
	var row SimPortfolioV2PriceOverride
	err := r.db.WithContext(ctx).Where(
		"market = ? AND code = ? AND exchange = ? AND trade_date = ? AND price_type = ?",
		normalizeSimPortfolioV2Market(market), strings.TrimSpace(code), strings.ToUpper(strings.TrimSpace(exchange)), strings.TrimSpace(tradeDate), strings.TrimSpace(priceType),
	).First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	return &row, err
}

func (r *Repository) GetLatestSimPortfolioV2Daily(ctx context.Context, portfolioID string) (*SimPortfolioV2Daily, error) {
	var row SimPortfolioV2Daily
	err := r.db.WithContext(ctx).Where("portfolio_id = ? AND status = ?", strings.TrimSpace(portfolioID), "verified").Order("trade_date DESC, id DESC").First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	return &row, err
}

func (r *Repository) ListSimPortfolioV2DailyRange(ctx context.Context, portfolioID, fromDate, toDate string) ([]SimPortfolioV2Daily, error) {
	q := r.db.WithContext(ctx).Where("portfolio_id = ? AND status = ?", strings.TrimSpace(portfolioID), "verified").Order("trade_date ASC, id ASC")
	if strings.TrimSpace(fromDate) != "" {
		q = q.Where("trade_date >= ?", strings.TrimSpace(fromDate))
	}
	if strings.TrimSpace(toDate) != "" {
		q = q.Where("trade_date <= ?", strings.TrimSpace(toDate))
	}
	var rows []SimPortfolioV2Daily
	err := q.Find(&rows).Error
	return rows, err
}

func (r *Repository) ReplaceSimPortfolioV2FactDate(ctx context.Context, daily SimPortfolioV2Daily, positions []SimPortfolioV2Position, trades []SimPortfolioV2Trade, metrics SimPortfolioV2Metrics) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		pid, date := daily.PortfolioID, daily.TradeDate
		if err := tx.Where("portfolio_id = ? AND trade_date = ?", pid, date).Delete(&SimPortfolioV2Position{}).Error; err != nil {
			return err
		}
		if err := tx.Where("portfolio_id = ? AND trade_date = ?", pid, date).Delete(&SimPortfolioV2Trade{}).Error; err != nil {
			return err
		}
		if err := tx.Where("portfolio_id = ? AND trade_date = ?", pid, date).Delete(&SimPortfolioV2Metrics{}).Error; err != nil {
			return err
		}
		if err := tx.Where("portfolio_id = ? AND trade_date = ?", pid, date).Delete(&SimPortfolioV2Daily{}).Error; err != nil {
			return err
		}
		if err := tx.Create(&daily).Error; err != nil {
			return err
		}
		if len(positions) > 0 {
			if err := tx.Create(&positions).Error; err != nil {
				return err
			}
		}
		if len(trades) > 0 {
			if err := tx.Create(&trades).Error; err != nil {
				return err
			}
		}
		return tx.Create(&metrics).Error
	})
}

func (r *Repository) ListSimPortfolioV2PositionsByTradeDate(ctx context.Context, portfolioID, tradeDate string) ([]SimPortfolioV2Position, error) {
	var rows []SimPortfolioV2Position
	err := r.db.WithContext(ctx).Where("portfolio_id = ? AND trade_date = ?", strings.TrimSpace(portfolioID), strings.TrimSpace(tradeDate)).Order("rank ASC, code ASC").Find(&rows).Error
	return rows, err
}

func (r *Repository) ListSimPortfolioV2Trades(ctx context.Context, portfolioID, fromDate, toDate, action string) ([]SimPortfolioV2Trade, error) {
	q := r.db.WithContext(ctx).Where("portfolio_id = ?", strings.TrimSpace(portfolioID)).Order("trade_date DESC, id DESC")
	if strings.TrimSpace(fromDate) != "" {
		q = q.Where("trade_date >= ?", strings.TrimSpace(fromDate))
	}
	if strings.TrimSpace(toDate) != "" {
		q = q.Where("trade_date <= ?", strings.TrimSpace(toDate))
	}
	if strings.TrimSpace(action) != "" {
		q = q.Where("action = ?", strings.TrimSpace(action))
	}
	var rows []SimPortfolioV2Trade
	err := q.Find(&rows).Error
	return rows, err
}

func (r *Repository) GetLatestSimPortfolioV2Metrics(ctx context.Context, portfolioID string) (*SimPortfolioV2Metrics, error) {
	var row SimPortfolioV2Metrics
	err := r.db.WithContext(ctx).Where("portfolio_id = ?", strings.TrimSpace(portfolioID)).Order("trade_date DESC, id DESC").First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	return &row, err
}

func (r *Repository) UpsertSimPortfolioV2MarketConfig(ctx context.Context, row SimPortfolioV2MarketConfig) error {
	return r.db.WithContext(ctx).Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "market"}}, DoUpdates: clause.AssignmentColumns([]string{"start_signal_date", "published_job_id", "latest_published_trade_date", "status", "updated_by", "updated_at"})}).Create(&row).Error
}

func (r *Repository) GetSimPortfolioV2MarketConfig(ctx context.Context, market string) (*SimPortfolioV2MarketConfig, error) {
	var row SimPortfolioV2MarketConfig
	err := r.db.WithContext(ctx).Where("market = ?", normalizeSimPortfolioV2Market(market)).First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	return &row, err
}

func (r *Repository) ListSimPortfolioV2SelectionBatches(ctx context.Context, market, fromDate, toDate string) ([]SimPortfolioV2SelectionBatch, error) {
	q := r.db.WithContext(ctx).Model(&SimPortfolioV2SelectionBatch{}).Where("market = ?", normalizeSimPortfolioV2Market(market))
	if strings.TrimSpace(fromDate) != "" {
		q = q.Where("signal_date >= ?", strings.TrimSpace(fromDate))
	}
	if strings.TrimSpace(toDate) != "" {
		q = q.Where("signal_date <= ?", strings.TrimSpace(toDate))
	}
	var rows []SimPortfolioV2SelectionBatch
	err := q.Order("signal_date ASC, portfolio_id ASC").Find(&rows).Error
	return rows, err
}

func (r *Repository) ListSimPortfolioV2PriceRequirementsByMarketDate(ctx context.Context, market, signalDate string) ([]SimPortfolioV2PriceRequirement, error) {
	var rows []SimPortfolioV2PriceRequirement
	err := r.db.WithContext(ctx).Where("market = ? AND signal_date = ?", normalizeSimPortfolioV2Market(market), strings.TrimSpace(signalDate)).Order("portfolio_id ASC, price_type ASC, code ASC").Find(&rows).Error
	return rows, err
}

func (r *Repository) CountSimPortfolioV2DailyByMarketSignalDate(ctx context.Context, market, signalDate string) (int, error) {
	var count int64
	err := r.db.WithContext(ctx).Model(&SimPortfolioV2Daily{}).Where("market = ? AND signal_date = ? AND status = ?", normalizeSimPortfolioV2Market(market), strings.TrimSpace(signalDate), "verified").Count(&count).Error
	return int(count), err
}

func (r *Repository) DeleteSimPortfolioV2FactsForMarketFromSignalDate(ctx context.Context, market, startSignalDate string) error {
	market = normalizeSimPortfolioV2Market(market)
	startSignalDate = strings.TrimSpace(startSignalDate)
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("market = ? AND signal_date >= ?", market, startSignalDate).Delete(&SimPortfolioV2Position{}).Error; err != nil {
			return err
		}
		if err := tx.Where("market = ? AND signal_date >= ?", market, startSignalDate).Delete(&SimPortfolioV2Trade{}).Error; err != nil {
			return err
		}
		if err := tx.Where("market = ? AND signal_date >= ?", market, startSignalDate).Delete(&SimPortfolioV2Metrics{}).Error; err != nil {
			return err
		}
		if err := tx.Where("market = ? AND signal_date >= ?", market, startSignalDate).Delete(&SimPortfolioV2Daily{}).Error; err != nil {
			return err
		}
		return nil
	})
}
