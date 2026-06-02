package portfolio

import (
	"time"

	"gorm.io/gorm"
)

type Migrator struct{}

func NewMigrator() Migrator {
	return Migrator{}
}

func (Migrator) Name() string {
	return "portfolio"
}

func (Migrator) AutoMigrate(db *gorm.DB) error {
	if err := db.AutoMigrate(
		&PortfolioRecord{},
		&PortfolioEventRecord{},
		&PortfolioPositionDailySnapshotRecord{},
		&SecurityProfileRecord{},
		&InvestmentProfileRecord{},
	); err != nil {
		return err
	}
	if err := ensurePortfolioDailySnapshotColumns(db); err != nil {
		return err
	}
	return db.AutoMigrate(
		&PortfolioSnapshotJobRunRecord{},
		&PortfolioSnapshotJobRunItemRecord{},
	)
}

func ensurePortfolioDailySnapshotColumns(db *gorm.DB) error {
	if !db.Migrator().HasTable(&PortfolioDailySnapshotRecord{}) {
		return db.AutoMigrate(&PortfolioDailySnapshotRecord{})
	}
	return db.Transaction(func(tx *gorm.DB) error {
		if err := ensureDailySnapshotColumn(tx, "source_type", "TEXT NOT NULL DEFAULT 'scheduled'"); err != nil {
			return err
		}
		if err := ensureDailySnapshotColumn(tx, "data_version", "INTEGER NOT NULL DEFAULT 1"); err != nil {
			return err
		}
		if err := ensureDailySnapshotColumn(tx, "computed_at", "datetime NOT NULL DEFAULT '1970-01-01 00:00:00'"); err != nil {
			return err
		}
		if err := ensureDailySnapshotColumn(tx, "job_run_id", "TEXT NOT NULL DEFAULT ''"); err != nil {
			return err
		}
		return backfillPortfolioDailySnapshotMetadata(tx)
	})
}

func ensureDailySnapshotColumn(db *gorm.DB, columnName, columnDDL string) error {
	if db.Migrator().HasColumn(&PortfolioDailySnapshotRecord{}, columnName) {
		return nil
	}
	return db.Exec("ALTER TABLE user_portfolio_daily_snapshots ADD COLUMN "+columnName+" "+columnDDL).Error
}

func backfillPortfolioDailySnapshotMetadata(db *gorm.DB) error {
	fallbackTime := time.Unix(0, 0).UTC()
	updates := []struct {
		query string
		args  []any
	}{
		{query: "UPDATE user_portfolio_daily_snapshots SET source_type = ? WHERE source_type IS NULL OR TRIM(source_type) = ''", args: []any{PortfolioSnapshotSourceScheduled}},
		{query: "UPDATE user_portfolio_daily_snapshots SET data_version = 1 WHERE data_version IS NULL OR data_version = 0"},
		{query: "UPDATE user_portfolio_daily_snapshots SET computed_at = COALESCE(updated_at, created_at, ?) WHERE computed_at IS NULL OR computed_at = ?", args: []any{fallbackTime, fallbackTime}},
		{query: "UPDATE user_portfolio_daily_snapshots SET job_run_id = '' WHERE job_run_id IS NULL"},
	}
	for _, item := range updates {
		if err := db.Exec(item.query, item.args...).Error; err != nil {
			return err
		}
	}
	return nil
}
