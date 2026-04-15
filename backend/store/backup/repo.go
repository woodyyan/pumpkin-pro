package backup

import (
	"time"

	"gorm.io/gorm"
)

// Repository handles DB operations for backup logs.
type Repository struct {
	db *gorm.DB
}

// NewRepository creates a new backup repository.
func NewRepository(db *gorm.DB) *Repository {
	return &Repository{db: db}
}

// Insert creates a new backup log record.
func (r *Repository) Insert(log *BackupLogRecord) error {
	return r.db.Create(log).Error
}

// ListRecent returns the most recent backup logs, ordered by triggered_at DESC.
func (r *Repository) ListRecent(limit int) ([]BackupLogRecord, error) {
	var logs []BackupLogRecord
	err := r.db.Order("triggered_at DESC").Limit(limit).Find(&logs).Error
	return logs, err
}

// GetLatest returns the most recent backup log (or nil if none exists).
func (r *Repository) GetLatest() (*BackupLogRecord, error) {
	var log BackupLogRecord
	err := r.db.Order("triggered_at DESC").First(&log).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, nil
		}
		return nil, err
	}
	return &log, nil
}

// GetLastSuccessfulTime returns the triggered_at of the latest successful backup.
// Returns zero time if no successful backup exists.
func (r *Repository) GetLastSuccessfulTime() (time.Time, error) {
	var log BackupLogRecord
	err := r.db.Where("status = ?", "success").Order("triggered_at DESC").First(&log).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return time.Time{}, nil
		}
		return time.Time{}, err
	}
	return log.TriggeredAt, nil
}

// CountSince counts backups since the given cutoff time.
func (r *Repository) CountSince(since time.Time) (int64, error) {
	var count int64
	err := r.db.Model(&BackupLogRecord{}).
		Where("triggered_at >= ?", since).
		Count(&count).Error
	return count, err
}

// DeleteOlderThan deletes backup log records older than the cutoff.
func (r *Repository) DeleteOlderThan(cutoff time.Time) (int64, error) {
	result := r.db.Where("triggered_at < ?", cutoff).Delete(&BackupLogRecord{})
	return result.RowsAffected, result.Error
}
