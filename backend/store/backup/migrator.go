package backup

import "gorm.io/gorm"

type Migrator struct{}

func NewMigrator() Migrator { return Migrator{} }

func (Migrator) Name() string { return "backup_logs" }

func (m Migrator) AutoMigrate(db *gorm.DB) error {
	return db.AutoMigrate(&BackupLogRecord{})
}
