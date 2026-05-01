package analytics

import "gorm.io/gorm"

type Migrator struct{}

func NewMigrator() Migrator {
	return Migrator{}
}

func (Migrator) Name() string {
	return "analytics"
}

func (Migrator) AutoMigrate(db *gorm.DB) error {
	if err := db.AutoMigrate(&PageViewRecord{}); err != nil {
		return err
	}
	return db.AutoMigrate(&DeviceSnapshot{})
}
