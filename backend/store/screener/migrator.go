package screener

import "gorm.io/gorm"

type Migrator struct{}

func NewMigrator() Migrator {
	return Migrator{}
}

func (Migrator) Name() string {
	return "screener"
}

func (Migrator) AutoMigrate(db *gorm.DB) error {
	return db.AutoMigrate(
		&WatchlistRecord{},
		&WatchlistStockRecord{},
	)
}
