package live

import "gorm.io/gorm"

type Migrator struct{}

func NewMigrator() Migrator {
	return Migrator{}
}

func (Migrator) Name() string {
	return "live"
}

func (Migrator) AutoMigrate(db *gorm.DB) error {
	return db.AutoMigrate(&WatchlistRecord{})
}
