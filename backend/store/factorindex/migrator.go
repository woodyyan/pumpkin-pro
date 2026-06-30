package factorindex

import "gorm.io/gorm"

type Migrator struct{}

func NewMigrator() Migrator { return Migrator{} }

func (Migrator) Name() string { return "factorindex" }

func (Migrator) AutoMigrate(db *gorm.DB) error {
	return db.AutoMigrate(
		&Definition{},
		&Rebalance{},
		&Constituent{},
		&Daily{},
	)
}
