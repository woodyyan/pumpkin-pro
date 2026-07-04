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
		&RankingPortfolioRealtimePrice{},
		&RankingPortfolioResult{},
		&RankingPortfolioJobStatus{},
		&SimPortfolioDaily{},
		&SimPortfolioPosition{},
		&SimPortfolioTrade{},
		&SimPortfolioMetrics{},
		&SimPortfolioTrackingConfig{},
		&SimPortfolioTrackingJob{},
		&MarketCalendar{},
		&SimPortfolioV2Definition{},
		&SimPortfolioV2PipelineRun{},
		&SimPortfolioV2PipelineDayStatus{},
		&SimPortfolioV2SignalBatch{},
		&SimPortfolioV2SignalItem{},
		&SimPortfolioV2SelectionBatch{},
		&SimPortfolioV2SelectionItem{},
		&SimPortfolioV2PriceRequirement{},
		&SimPortfolioV2Daily{},
		&SimPortfolioV2Position{},
		&SimPortfolioV2Trade{},
		&SimPortfolioV2Metrics{},
		&SimPortfolioV2Watermark{},
	)
}
