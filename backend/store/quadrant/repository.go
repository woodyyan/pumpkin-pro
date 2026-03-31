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
					"trend", "flow", "revision", "volatility", "drawdown", "crowding",
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
