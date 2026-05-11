package quadrant

import "gorm.io/gorm"

// Migrator handles database migrations for the quadrant module.
type Migrator struct{}

func NewMigrator() Migrator { return Migrator{} }

func (Migrator) Name() string { return "quadrant" }

func (Migrator) AutoMigrate(db *gorm.DB) error {
	return db.AutoMigrate(
		&QuadrantScoreRecord{},
		&ComputeLogRecord{},
		&RankingSnapshot{},
		&RankingPortfolioDefinition{},
		&RankingPortfolioSnapshot{},
		&RankingPortfolioSnapshotConstituent{},
		&RankingPortfolioMarketPrice{},
		&RankingPortfolioBenchmarkPrice{},
		&RankingPortfolioResult{},
	)
}
