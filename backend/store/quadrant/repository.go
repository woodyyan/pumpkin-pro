package quadrant

import (
	"context"
	"errors"
	"strings"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// Repository handles all database operations for quadrant scores.
type Repository struct {
	db *gorm.DB
}

func NewRepository(db *gorm.DB) *Repository {
	return &Repository{db: db}
}

// BulkUpsert replaces all quadrant scores in a single transaction.
func (r *Repository) BulkUpsert(ctx context.Context, records []QuadrantScoreRecord) error {
	if len(records) == 0 {
		return nil
	}

	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// Process in batches of 500 to avoid SQLite variable limit
		batchSize := 500
		for i := 0; i < len(records); i += batchSize {
			end := i + batchSize
			if end > len(records) {
				end = len(records)
			}
			batch := records[i:end]

			if err := tx.Clauses(clause.OnConflict{
				Columns: []clause.Column{{Name: "code"}},
				DoUpdates: clause.AssignmentColumns([]string{
					"name", "opportunity", "risk", "quadrant",
					"trend", "flow", "revision", "liquidity",
					"volatility", "drawdown", "crowding", "avg_amount5d",
					"computed_at",
				}),
			}).Create(&batch).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

// FindAll returns all quadrant scores.
func (r *Repository) FindAll(ctx context.Context) ([]QuadrantScoreRecord, error) {
	var records []QuadrantScoreRecord
	if err := r.db.WithContext(ctx).Find(&records).Error; err != nil {
		return nil, err
	}
	return records, nil
}

// FindByExchange returns scores filtered by exchange codes (e.g. "SSE","SZSE" or "HKEX").
func (r *Repository) FindByExchange(ctx context.Context, exchanges []string) ([]QuadrantScoreRecord, error) {
	if len(exchanges) == 0 {
		return r.FindAll(ctx)
	}
	var records []QuadrantScoreRecord
	if err := r.db.WithContext(ctx).Where("exchange IN ?", exchanges).Find(&records).Error; err != nil {
		return nil, err
	}
	return records, nil
}

// HasNonZeroLiquidity returns true if any record in the given exchanges has
// avg_amount5d > 0, indicating that data was computed after the liquidity field
// was introduced. Used for backward-compatible filter activation.
func (r *Repository) HasNonZeroLiquidity(ctx context.Context, exchanges []string) (bool, error) {
	var count int64
	query := r.db.WithContext(ctx).Model(&QuadrantScoreRecord{}).Where("avg_amount5d > 0")
	if len(exchanges) > 0 {
		query = query.Where("exchange IN ?", exchanges)
	}
	if err := query.Count(&count).Error; err != nil {
		return false, err
	}
	return count > 0, nil
}

// SearchResult is a minimal item returned by stock search.
type SearchResult struct {
	Code     string `json:"code"`
	Name     string `json:"name"`
	Exchange string `json:"exchange"`
}

// Search searches stocks by code or name prefix/fuzzy match.
func (r *Repository) Search(ctx context.Context, query string, limit int) ([]SearchResult, error) {
	if query == "" || limit <= 0 {
		return []SearchResult{}, nil
	}
	pattern := "%" + query + "%"
	var records []QuadrantScoreRecord
	err := r.db.WithContext(ctx).
		Select("code, name, exchange").
		Where("name LIKE ? OR code LIKE ?", pattern, pattern).
		Order("LENGTH(code) ASC").
		Limit(limit).
		Find(&records).Error
	if err != nil {
		return nil, err
	}
	results := make([]SearchResult, len(records))
	for i, rec := range records {
		results[i] = SearchResult{Code: rec.Code, Name: rec.Name, Exchange: rec.Exchange}
	}
	return results, nil
}

// FindBySymbols returns quadrant scores for specific symbols (A-share codes, 6 digits).
func (r *Repository) FindBySymbols(ctx context.Context, codes []string) ([]QuadrantScoreRecord, error) {
	if len(codes) == 0 {
		return nil, nil
	}
	var records []QuadrantScoreRecord
	if err := r.db.WithContext(ctx).Where("code IN ?", codes).Find(&records).Error; err != nil {
		return nil, err
	}
	return records, nil
}

// Count returns total number of quadrant scores in the table.
func (r *Repository) Count(ctx context.Context) (int64, error) {
	var count int64
	if err := r.db.WithContext(ctx).Model(&QuadrantScoreRecord{}).Count(&count).Error; err != nil {
		return 0, err
	}
	return count, nil
}

// GetLatestComputedAt returns the most recent computed_at timestamp.
func (r *Repository) GetLatestComputedAt(ctx context.Context) (*time.Time, error) {
	var record QuadrantScoreRecord
	if err := r.db.WithContext(ctx).Order("computed_at DESC").First(&record).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, nil
		}
		return nil, err
	}
	t := record.ComputedAt.UTC()
	return &t, nil
}

// ── Compute logs ──

// InsertComputeLog inserts or updates a compute log entry (upsert by ID).
func (r *Repository) InsertComputeLog(ctx context.Context, log ComputeLogRecord) error {
	return r.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "id"}},
		DoUpdates: clause.AssignmentColumns([]string{
			"computed_at", "mode", "duration_sec", "stock_count",
			"report_json", "status", "error_msg", "exchange",
			"started_at", "finished_at", "total_count",
			"success_count", "failed_count",
		}),
	}).Create(&log).Error
}

// FindLatestRunningLog returns the most recent "running" compute log for the given exchange,
// or nil if none exists. Used by BulkSave to promote a running log to terminal state.
func (r *Repository) FindLatestRunningLog(ctx context.Context, exchange string) (*ComputeLogRecord, error) {
	var log ComputeLogRecord
	err := r.db.WithContext(ctx).
		Where("exchange = ? AND status = ?", exchange, "running").
		Order("computed_at DESC").
		First(&log).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &log, nil
}

func (r *Repository) ListComputeLogs(ctx context.Context, limit int) ([]ComputeLogRecord, error) {
	var logs []ComputeLogRecord
	if err := r.db.WithContext(ctx).Order("computed_at DESC").Limit(limit).Find(&logs).Error; err != nil {
		return nil, err
	}
	return logs, nil
}

// ListComputeLogDetails returns failure detail logs for a given task log ID.
func (r *Repository) ListComputeLogDetails(ctx context.Context, taskLogID string) ([]ComputeLogRecord, error) {
	var details []ComputeLogRecord
	if err := r.db.WithContext(ctx).
		Where("task_log_id = ?", taskLogID).
		Order("id ASC").
		Find(&details).Error; err != nil {
		return nil, err
	}
	return details, nil
}

func (r *Repository) GetLatestComputeLog(ctx context.Context) (*ComputeLogRecord, error) {
	var log ComputeLogRecord
	if err := r.db.WithContext(ctx).Order("computed_at DESC").First(&log).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, nil
		}
		return nil, err
	}
	return &log, nil
}

// FindOpportunityZone returns records in the "机会" (opportunity) zone,
// ordered by opportunity DESC, risk ASC, limited to `limit`.
// minAmount is a liquidity hard-filter: only stocks with avg_amount_5d >= threshold are returned.
func (r *Repository) FindOpportunityZone(ctx context.Context, exchanges []string, limit int, minAmount float64) ([]QuadrantScoreRecord, int64, error) {
	var records []QuadrantScoreRecord

	query := r.db.WithContext(ctx).
		Where("quadrant = ?", "机会")

	if len(exchanges) > 0 {
		query = query.Where("exchange IN ?", exchanges)
	}

	// Liquidity hard filter: exclude illiquid stocks from ranking
	if minAmount > 0 {
		query = query.Where("avg_amount5d >= ?", minAmount)
	}

	// Exclude ST/*ST stocks (退市风险) from ranking — 排行榜面向小白用户，不应推荐 ST 股票
	query = query.Where("name NOT LIKE ?", "%ST%")

	// Get total count in zone (pre-filter for meta display)
	var totalInZone int64
	countQuery := r.db.WithContext(ctx).
		Model(&QuadrantScoreRecord{}).
		Where("quadrant = ?", "机会")
	if len(exchanges) > 0 {
		countQuery = countQuery.Where("exchange IN ?", exchanges)
	}
	if minAmount > 0 {
		countQuery = countQuery.Where("avg_amount5d >= ?", minAmount)
	}
	// 同步排除 ST，确保 totalInZone 与实际返回一致
	countQuery = countQuery.Where("name NOT LIKE ?", "%ST%")
	if err := countQuery.Count(&totalInZone).Error; err != nil {
		return nil, 0, err
	}

	// Apply sort + limit
	if limit <= 0 || limit > 50 {
		limit = 20
	}
	err := query.
		Order("opportunity DESC, risk ASC").
		Limit(limit).
		Find(&records).Error
	if err != nil {
		return nil, 0, err
	}
	return records, totalInZone, nil
}

// ── Ranking Snapshot ──

// UpsertSnapshot inserts or updates a ranking snapshot record.
// Unique constraint: (snapshot_date, code)
func (r *Repository) UpsertSnapshot(ctx context.Context, snap RankingSnapshot) error {
	return r.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns: []clause.Column{
			{Name: "snapshot_date"},
			{Name: "code"},
		},
		DoUpdates: clause.AssignmentColumns([]string{
			"name", "rank", "opportunity", "risk", "close_price",
		}),
	}).Create(&snap).Error
}

// UpsertSnapshots batch-upserts ranking snapshots in one transaction.
func (r *Repository) UpsertSnapshots(ctx context.Context, snaps []RankingSnapshot) error {
	if len(snaps) == 0 {
		return nil
	}
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		batchSize := 200
		for i := 0; i < len(snaps); i += batchSize {
			end := i + batchSize
			if end > len(snaps) {
				end = len(snaps)
			}
			batch := snaps[i:end]
			if err := tx.Clauses(clause.OnConflict{
				Columns: []clause.Column{
					{Name: "snapshot_date"},
					{Name: "code"},
				},
				DoUpdates: clause.AssignmentColumns([]string{
					"name", "rank", "opportunity", "risk", "close_price",
				}),
			}).Create(&batch).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

// GetConsecutiveDays returns how many consecutive days (including today) a stock has appeared
// in ranking snapshots, counting backward from the most recent snapshot date.
// Returns 0 if no snapshots exist for this stock/exchange.
func (r *Repository) GetConsecutiveDays(ctx context.Context, code string, exchanges []string) (int, error) {
	var snapDates []string

	query := r.db.WithContext(ctx).Model(&RankingSnapshot{}).
		Select("DISTINCT snapshot_date").
		Where("code = ?", code).
		Order("snapshot_date DESC")

	if len(exchanges) > 0 {
		query = query.Where("exchange IN ?", exchanges)
	}

	if err := query.Find(&snapDates).Error; err != nil || len(snapDates) == 0 {
		return 0, err
	}

	// Count consecutive days starting from the most recent date
	consecutive := 0
	refDate, _ := time.Parse("2006-01-02", snapDates[0])
	for _, dStr := range snapDates {
		d, err := time.Parse("2006-01-02", dStr)
		if err != nil {
			continue
		}
		diff := refDate.Sub(d).Hours() / 24
		if diff <= float64(consecutive)+0.5 { // allow same-day tolerance
			consecutive++
		} else {
			break // gap found — stop counting
		}
	}
	return consecutive, nil
}

// GetFirstAppearedDate returns the earliest snapshot_date for a given stock.
// Used to compute cumulative return since first appearance.
// Returns a date string in "2006-01-02" format, or empty if not found.
func (r *Repository) GetFirstAppearedDate(ctx context.Context, code string, exchanges []string) (string, error) {
	var snap RankingSnapshot
	query := r.db.WithContext(ctx).Model(&RankingSnapshot{}).
		Select("snapshot_date").
		Where("code = ?", code).
		Order("snapshot_date ASC")
	if len(exchanges) > 0 {
		query = query.Where("exchange IN ?", exchanges)
	}
	err := query.First(&snap).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return "", nil
		}
		return "", err
	}
	return snap.SnapshotDate, nil
}

// GetClosePriceOnDate returns the close_price stored in a snapshot for a specific stock+date.
func (r *Repository) GetClosePriceOnDate(ctx context.Context, code string, dateStr string) (float64, error) {
	var snap RankingSnapshot
	err := r.db.WithContext(ctx).Model(&RankingSnapshot{}).
		Select("close_price").
		Where("code = ? AND snapshot_date = ?", code, dateStr).
		Where("close_price > ?", 0).
		First(&snap).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return 0, nil
		}
		return 0, err
	}
	return snap.ClosePrice, nil
}

// GetEarliestAvailableClosePrice returns the earliest positive close_price on or after a date.
// This lets ranking returns recover when the first appearance snapshot was saved before a usable
// closing snapshot was present for that market/day.
func (r *Repository) GetEarliestAvailableClosePrice(ctx context.Context, code string, exchanges []string, fromDate string) (float64, string, error) {
	var snap RankingSnapshot
	query := r.db.WithContext(ctx).Model(&RankingSnapshot{}).
		Select("snapshot_date", "close_price").
		Where("code = ? AND close_price > ?", code, 0).
		Order("snapshot_date ASC")
	if len(exchanges) > 0 {
		query = query.Where("exchange IN ?", exchanges)
	}
	if strings.TrimSpace(fromDate) != "" {
		query = query.Where("snapshot_date >= ?", fromDate)
	}
	if err := query.First(&snap).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return 0, "", nil
		}
		return 0, "", err
	}
	return snap.ClosePrice, snap.SnapshotDate, nil
}

// GetLatestAvailableClosePrice returns the most recent positive close_price for a stock.
// Used as the "current price" when the latest ranking date itself has no valid close price yet.
func (r *Repository) GetLatestAvailableClosePrice(ctx context.Context, code string, exchanges []string) (float64, string, error) {
	var snap RankingSnapshot
	query := r.db.WithContext(ctx).Model(&RankingSnapshot{}).
		Select("snapshot_date", "close_price").
		Where("code = ? AND close_price > ?", code, 0).
		Order("snapshot_date DESC")
	if len(exchanges) > 0 {
		query = query.Where("exchange IN ?", exchanges)
	}
	if err := query.First(&snap).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return 0, "", nil
		}
		return 0, "", err
	}
	return snap.ClosePrice, snap.SnapshotDate, nil
}
