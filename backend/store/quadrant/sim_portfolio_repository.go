package quadrant

import (
	"context"
	"errors"
	"strings"

	"gorm.io/gorm"
)

func (r *Repository) ListActiveRankingPortfolioDefinitions(ctx context.Context) ([]RankingPortfolioDefinition, error) {
	var items []RankingPortfolioDefinition
	err := r.db.WithContext(ctx).
		Where("is_active = ?", true).
		Order("exchange ASC, portfolio_variant ASC, code ASC").
		Find(&items).Error
	if err != nil {
		return nil, err
	}
	return items, nil
}

func (r *Repository) ListRankingSnapshotsByDate(ctx context.Context, exchange string, snapshotDate string, limit int) ([]RankingSnapshot, error) {
	query := r.db.WithContext(ctx).Model(&RankingSnapshot{}).
		Where("snapshot_date = ?", strings.TrimSpace(snapshotDate)).
		Order("rank ASC, code ASC")
	switch strings.ToUpper(strings.TrimSpace(exchange)) {
	case "HKEX":
		query = query.Where("exchange = ?", "HKEX")
	default:
		query = query.Where("exchange IN ?", []string{"SSE", "SZSE"})
	}
	if limit > 0 {
		query = query.Limit(limit)
	}
	var rows []RankingSnapshot
	if err := query.Find(&rows).Error; err != nil {
		return nil, err
	}
	return rows, nil
}

func (r *Repository) GetConsecutiveDaysAsOf(ctx context.Context, code string, exchanges []string, asOfDate string) (int, error) {
	code = strings.TrimSpace(code)
	asOfDate = strings.TrimSpace(asOfDate)
	if code == "" || asOfDate == "" {
		return 0, nil
	}
	var snapDates []string
	query := r.db.WithContext(ctx).Model(&RankingSnapshot{}).
		Distinct("snapshot_date").
		Where("code = ? AND snapshot_date <= ?", code, asOfDate).
		Order("snapshot_date DESC")
	if len(exchanges) > 0 {
		query = query.Where("exchange IN ?", exchanges)
	}
	if err := query.Pluck("snapshot_date", &snapDates).Error; err != nil {
		return 0, err
	}
	return len(consecutiveSnapshotDatesDesc(snapDates)), nil
}

func (r *Repository) GetRankingPortfolioSelectionOpenPrice(ctx context.Context, definitionID string, snapshotDate string, code string, exchange string) (float64, string, error) {
	var row RankingPortfolioMarketPrice
	query := r.db.WithContext(ctx).Model(&RankingPortfolioMarketPrice{}).
		Select("open_price, entry_trade_date").
		Where("definition_id = ? AND snapshot_date = ? AND code = ?", strings.TrimSpace(definitionID), strings.TrimSpace(snapshotDate), strings.TrimSpace(code))
	normalizedExchange := strings.ToUpper(strings.TrimSpace(exchange))
	if normalizedExchange != "" {
		query = query.Where("exchange = ?", normalizedExchange)
	}
	if err := query.Order("id DESC").First(&row).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return 0, "", nil
		}
		return 0, "", err
	}
	return row.OpenPrice, strings.TrimSpace(row.EntryTradeDate), nil
}

func (r *Repository) GetClosePriceByTradeDate(ctx context.Context, code string, exchange string, tradeDate string) (float64, error) {
	code = strings.TrimSpace(code)
	tradeDate = strings.TrimSpace(tradeDate)
	if code == "" || tradeDate == "" {
		return 0, nil
	}
	var row RankingSnapshot
	query := r.db.WithContext(ctx).Model(&RankingSnapshot{}).
		Select("close_price").
		Where("code = ? AND close_price > ? AND ((price_trade_date = ? AND price_trade_date != '') OR snapshot_date = ?)", code, 0, tradeDate, tradeDate).
		Order("CASE WHEN price_trade_date = '" + tradeDate + "' THEN 0 ELSE 1 END, id DESC")
	normalizedExchange := strings.ToUpper(strings.TrimSpace(exchange))
	if normalizedExchange != "" {
		query = query.Where("exchange = ?", normalizedExchange)
	}
	if err := query.First(&row).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return 0, nil
		}
		return 0, err
	}
	return row.ClosePrice, nil
}

func (r *Repository) GetLatestSimPortfolioDaily(ctx context.Context, portfolioID string) (*SimPortfolioDaily, error) {
	var row SimPortfolioDaily
	if err := r.db.WithContext(ctx).
		Where("portfolio_id = ?", strings.TrimSpace(portfolioID)).
		Order("trade_date DESC, id DESC").
		First(&row).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &row, nil
}

func (r *Repository) GetLatestSimPortfolioMetrics(ctx context.Context, portfolioID string) (*SimPortfolioMetrics, error) {
	var row SimPortfolioMetrics
	if err := r.db.WithContext(ctx).
		Where("portfolio_id = ?", strings.TrimSpace(portfolioID)).
		Order("trade_date DESC, id DESC").
		First(&row).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &row, nil
}

func (r *Repository) GetSimPortfolioDailyByTradeDate(ctx context.Context, portfolioID string, tradeDate string) (*SimPortfolioDaily, error) {
	var row SimPortfolioDaily
	if err := r.db.WithContext(ctx).
		Where("portfolio_id = ? AND trade_date = ?", strings.TrimSpace(portfolioID), strings.TrimSpace(tradeDate)).
		First(&row).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &row, nil
}

func (r *Repository) ListSimPortfolioDailyRange(ctx context.Context, portfolioID string, fromDate string, toDate string) ([]SimPortfolioDaily, error) {
	query := r.db.WithContext(ctx).Model(&SimPortfolioDaily{}).
		Where("portfolio_id = ?", strings.TrimSpace(portfolioID)).
		Order("trade_date ASC, id ASC")
	if strings.TrimSpace(fromDate) != "" {
		query = query.Where("trade_date >= ?", strings.TrimSpace(fromDate))
	}
	if strings.TrimSpace(toDate) != "" {
		query = query.Where("trade_date <= ?", strings.TrimSpace(toDate))
	}
	var rows []SimPortfolioDaily
	if err := query.Find(&rows).Error; err != nil {
		return nil, err
	}
	return rows, nil
}

func (r *Repository) ListAllSimPortfolioDaily(ctx context.Context, portfolioID string) ([]SimPortfolioDaily, error) {
	var rows []SimPortfolioDaily
	if err := r.db.WithContext(ctx).
		Where("portfolio_id = ?", strings.TrimSpace(portfolioID)).
		Order("trade_date ASC, id ASC").
		Find(&rows).Error; err != nil {
		return nil, err
	}
	return rows, nil
}

func (r *Repository) CountSimPortfolioPositions(ctx context.Context, portfolioID string) (int, error) {
	var count int64
	if err := r.db.WithContext(ctx).Model(&SimPortfolioPosition{}).
		Where("portfolio_id = ?", strings.TrimSpace(portfolioID)).
		Count(&count).Error; err != nil {
		return 0, err
	}
	return int(count), nil
}

func (r *Repository) CountSimPortfolioTrades(ctx context.Context, portfolioID string) (int, error) {
	var count int64
	if err := r.db.WithContext(ctx).Model(&SimPortfolioTrade{}).
		Where("portfolio_id = ?", strings.TrimSpace(portfolioID)).
		Count(&count).Error; err != nil {
		return 0, err
	}
	return int(count), nil
}

func (r *Repository) CountSimPortfolioMetrics(ctx context.Context, portfolioID string) (int, error) {
	var count int64
	if err := r.db.WithContext(ctx).Model(&SimPortfolioMetrics{}).
		Where("portfolio_id = ?", strings.TrimSpace(portfolioID)).
		Count(&count).Error; err != nil {
		return 0, err
	}
	return int(count), nil
}

func (r *Repository) ListSimPortfolioPositionsByTradeDate(ctx context.Context, portfolioID string, tradeDate string) ([]SimPortfolioPosition, error) {
	var rows []SimPortfolioPosition
	if err := r.db.WithContext(ctx).
		Where("portfolio_id = ? AND trade_date = ?", strings.TrimSpace(portfolioID), strings.TrimSpace(tradeDate)).
		Order("rank ASC, stock_code ASC").
		Find(&rows).Error; err != nil {
		return nil, err
	}
	return rows, nil
}

func (r *Repository) ListLatestSimPortfolioTrades(ctx context.Context, portfolioID string, limit int) ([]SimPortfolioTrade, error) {
	if limit <= 0 {
		limit = 8
	}
	query := r.db.WithContext(ctx).Model(&SimPortfolioTrade{}).
		Where("portfolio_id = ?", strings.TrimSpace(portfolioID)).
		Order("trade_date DESC, CASE action WHEN 'SELL' THEN 0 WHEN 'BUY' THEN 1 ELSE 2 END, stock_code ASC").
		Limit(limit)
	var rows []SimPortfolioTrade
	if err := query.Find(&rows).Error; err != nil {
		return nil, err
	}
	return rows, nil
}

func (r *Repository) ListSimPortfolioTradesRange(ctx context.Context, portfolioID string, fromDate string, toDate string, action string) ([]SimPortfolioTrade, error) {
	query := r.db.WithContext(ctx).Model(&SimPortfolioTrade{}).
		Where("portfolio_id = ?", strings.TrimSpace(portfolioID)).
		Order("trade_date DESC, CASE action WHEN 'SELL' THEN 0 WHEN 'BUY' THEN 1 ELSE 2 END, stock_code ASC")
	if strings.TrimSpace(fromDate) != "" {
		query = query.Where("trade_date >= ?", strings.TrimSpace(fromDate))
	}
	if strings.TrimSpace(toDate) != "" {
		query = query.Where("trade_date <= ?", strings.TrimSpace(toDate))
	}
	if strings.TrimSpace(action) != "" {
		query = query.Where("action = ?", strings.ToUpper(strings.TrimSpace(action)))
	}
	var rows []SimPortfolioTrade
	if err := query.Find(&rows).Error; err != nil {
		return nil, err
	}
	return rows, nil
}

func (r *Repository) ListAllSimPortfolioTrades(ctx context.Context, portfolioID string) ([]SimPortfolioTrade, error) {
	var rows []SimPortfolioTrade
	if err := r.db.WithContext(ctx).
		Where("portfolio_id = ?", strings.TrimSpace(portfolioID)).
		Order("trade_date ASC, stock_code ASC").
		Find(&rows).Error; err != nil {
		return nil, err
	}
	return rows, nil
}
