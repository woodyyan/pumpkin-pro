package aireport

import "gorm.io/gorm"

type Migrator struct{}

func NewMigrator() Migrator {
	return Migrator{}
}

func (Migrator) Name() string {
	return "aireport"
}

func (Migrator) AutoMigrate(db *gorm.DB) error {
	return db.AutoMigrate(&ResearchReportRecord{}, &ServiceConfigRecord{})
}
