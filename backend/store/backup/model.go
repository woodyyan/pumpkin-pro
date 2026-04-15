package backup

import "time"

// BackupLogRecord stores each backup execution record for observability.
type BackupLogRecord struct {
	ID             int64      `gorm:"primaryKey;autoIncrement"`
	TriggeredAt    time.Time  `gorm:"not null;index"`
	TriggerType    string     `gorm:"size:32;not null;default:'manual';index"` // 'quadrant_callback' | 'scheduled_fallback' | 'manual'
	Status         string     `gorm:"size:16;not null;default:'success'"`          // 'success' | 'failed' | 'partial'
	PumpkinFile    string     `gorm:"size:128;default:''"`
	PumpkinSize    int64      `gorm:"default:0"`
	CacheAFile     string     `gorm:"size:128;default:''"`
	CacheASize     int64      `gorm:"default:0"`
	CacheHKFile    string     `gorm:"size:128;default:''"`
	CacheHKSize    int64      `gorm:"default:0"`
	COSUploaded    bool       `gorm:"default:false"`
	IntegrityCheck string     `gorm:"size:16;default:'skipped'"` // 'ok' | 'failed' | 'skipped'
	ErrorMessage   string     `gorm:"type:text;default:''"`
	DurationMS     int64      `gorm:"default:0"`
	CreatedAt      time.Time  `gorm:"not null"`
}

func (BackupLogRecord) TableName() string {
	return "backup_logs"
}

// ── JSON DTOs ──

// BackupStatusResponse is returned by GET /api/admin/backup-status.
type BackupStatusResponse struct {
	LastBackupAt   *string `json:"last_backup_at,omitempty"`
	LastTriggerType string  `json:"last_trigger_type"`
	Status          string  `json:"status"`           // 'success' | 'failed' | 'never'
	PumpkinSize     int64   `json:"pumpkin_size_bytes"`
	CacheASize      int64   `json:"cache_a_size_bytes"`
	CacheHKSize     int64   `json:"cache_hk_size_bytes"`
	COSUploaded     bool    `json:"cos_uploaded"`
	DurationMS      int64   `json:"duration_ms"`
	ErrorMsg        string  `json:"error_msg,omitempty"`
}

// BackupHistoryItem is one row in the history list.
type BackupHistoryItem struct {
	ID             int64  `json:"id"`
	TriggeredAt    string `json:"triggered_at"`
	TriggerType    string `json:"trigger_type"`
	Status         string `json:"status"`
	PumpkinSize    int64  `json:"pumpkin_size_bytes"`
	CacheASize     int64  `json:"cache_a_size_bytes"`
	CacheHKSize    int64  `json:"cache_hk_size_bytes"`
	COSUploaded    bool   `json:"cos_uploaded"`
	IntegrityCheck string `json:"integrity_check"`
	ErrorMsg       string `json:"error_msg,omitempty"`
	DurationMS     int64  `json:"duration_ms"`
}

// BackupStorageStats holds local + cloud storage usage info.
type BackupStorageStats struct {
	LocalTotalBytes   int64  `json:"local_total_bytes"`
	LocalFileCount    int    `json:"local_file_count"`
	LocalRetentionDays int   `json:"local_retention_days"`
	CloudTotalBytes   int64  `json:"cloud_total_bytes,omitempty"`
	CloudFileCount    int    `json:"cloud_file_count,omitempty"`
	CloudEnabled      bool   `json:"cloud_enabled"`
}
