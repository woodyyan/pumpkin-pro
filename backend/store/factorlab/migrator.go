package factorlab

import "gorm.io/gorm"

type Migrator struct{}

func NewMigrator() Migrator { return Migrator{} }

func (Migrator) Name() string { return "factorlab" }

func (Migrator) AutoMigrate(db *gorm.DB) error {
	return db.AutoMigrate(
		&FactorSecurity{},
		&FactorDailyBar{},
		&FactorIndexDailyBar{},
		&FactorMarketMetric{},
		&FactorFinancialMetric{},
		&FactorDividendRecord{},
		&FactorSnapshot{},
		&FactorTaskRun{},
		&FactorTaskItem{},
	)
}
