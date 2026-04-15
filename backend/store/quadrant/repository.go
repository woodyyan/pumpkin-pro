package quadrant

import (
	"context"
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

func (r *Repository) InsertComputeLog(ctx context.Context, log ComputeLogRecord) error {
	return r.db.WithContext(ctx).Create(&log).Error
}

func (r *Repository) ListComputeLogs(ctx context.Context, limit int) ([]ComputeLogRecord, error) {
	var logs []ComputeLogRecord
	if err := r.db.WithContext(ctx).Order("computed_at DESC").Limit(limit).Find(&logs).Error; err != nil {
		return nil, err
	}
	return logs, nil
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

	// Get total count in zone (pre-filter for meta display)
	var totalInZone int64
	countQuery := r.db.WithContext(ctx).
		Model(&QuadrantScoreRecord{}).
		Where("quadrant = ?", "机会")
	if len(exchanges) > 0 {
		countQuery = countQuery.Where("exchange IN ?", exchanges)
	}
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
