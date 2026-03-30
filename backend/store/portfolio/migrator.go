package portfolio

import "gorm.io/gorm"

type Migrator struct{}

func NewMigrator() Migrator {
	return Migrator{}
}

func (Migrator) Name() string {
	return "portfolio"
}

func (Migrator) AutoMigrate(db *gorm.DB) error {
	return db.AutoMigrate(&PortfolioRecord{})
}
