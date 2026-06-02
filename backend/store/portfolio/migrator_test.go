package portfolio

import (
	"testing"
	"time"

	"github.com/woodyyan/pumpkin-pro/backend/tests/testutil"
)

type legacyDailySnapshotRecord struct {
	ID                  string    `gorm:"primaryKey;size:36"`
	UserID              string    `gorm:"size:36;not null"`
	Scope               string    `gorm:"size:16;not null"`
	SnapshotDate        string    `gorm:"size:10;not null"`
	CurrencyCode        string    `gorm:"size:8;not null;default:''"`
	MarketValueAmount   float64   `gorm:"not null;default:0"`
	TotalCostAmount     float64   `gorm:"not null;default:0"`
	UnrealizedPnlAmount float64   `gorm:"not null;default:0"`
	RealizedPnlAmount   float64   `gorm:"not null;default:0"`
	TotalPnlAmount      float64   `gorm:"not null;default:0"`
	TodayPnlAmount      float64   `gorm:"not null;default:0"`
	PositionCount       int       `gorm:"not null;default:0"`
	CreatedAt           time.Time `gorm:"not null"`
	UpdatedAt           time.Time `gorm:"not null"`
}

func (legacyDailySnapshotRecord) TableName() string { return "user_portfolio_daily_snapshots" }

func TestMigratorAutoMigrateExistingDailySnapshotTable(t *testing.T) {
	db := testutil.InMemoryDB(t)
	if err := db.AutoMigrate(&legacyDailySnapshotRecord{}); err != nil {
		t.Fatalf("migrate legacy table failed: %v", err)
	}
	now := time.Date(2026, 6, 2, 12, 0, 0, 0, time.UTC)
	if err := db.Table("user_portfolio_daily_snapshots").Create(map[string]any{
		"id":                    "legacy-1",
		"user_id":               "user-1",
		"scope":                 PortfolioScopeAShare,
		"snapshot_date":         "2026-06-01",
		"currency_code":         "CNY",
		"market_value_amount":   100,
		"total_cost_amount":     90,
		"unrealized_pnl_amount": 10,
		"realized_pnl_amount":   0,
		"total_pnl_amount":      10,
		"today_pnl_amount":      2,
		"position_count":        1,
		"created_at":            now,
		"updated_at":            now,
	}).Error; err != nil {
		t.Fatalf("seed legacy row failed: %v", err)
	}

	if err := NewMigrator().AutoMigrate(db); err != nil {
		t.Fatalf("portfolio migrator failed on legacy table: %v", err)
	}
	for _, column := range []string{"source_type", "data_version", "computed_at", "job_run_id"} {
		if !db.Migrator().HasColumn(&PortfolioDailySnapshotRecord{}, column) {
			t.Fatalf("expected column %s to exist", column)
		}
	}
	var row struct {
		SourceType string
		DataVersion int
		ComputedAt time.Time
		JobRunID string
	}
	if err := db.Table("user_portfolio_daily_snapshots").Select("source_type, data_version, computed_at, job_run_id").Where("id = ?", "legacy-1").Scan(&row).Error; err != nil {
		t.Fatalf("query migrated row failed: %v", err)
	}
	if row.SourceType != PortfolioSnapshotSourceScheduled {
		t.Fatalf("expected source_type backfilled, got %q", row.SourceType)
	}
	if row.DataVersion != 1 {
		t.Fatalf("expected data_version=1, got %d", row.DataVersion)
	}
	if row.ComputedAt.IsZero() {
		t.Fatal("expected computed_at backfilled")
	}
	if row.JobRunID != "" {
		t.Fatalf("expected empty job_run_id, got %q", row.JobRunID)
	}
}
