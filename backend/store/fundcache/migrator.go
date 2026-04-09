package fundcache

import "gorm.io/gorm"

type Migrator struct{}

func NewMigrator() Migrator {
	return Migrator{}
}

func (Migrator) Name() string {
	return "fundcache"
}

func (Migrator) AutoMigrate(db *gorm.DB) error {
	return db.AutoMigrate(&FundamentalsCacheRow{})
}
