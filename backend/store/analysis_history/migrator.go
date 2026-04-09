package analysis_history

import "gorm.io/gorm"

type Migrator struct{}

func NewMigrator() Migrator {
	return Migrator{}
}

func (Migrator) Name() string { return "analysis_history" }

func (Migrator) AutoMigrate(db *gorm.DB) error {
	return db.AutoMigrate(&AnalysisHistoryRecord{})
}
