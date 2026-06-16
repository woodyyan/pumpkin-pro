package aipicker

import (
	"fmt"

	"gorm.io/gorm"
)

type Migrator struct{}

func NewMigrator() Migrator { return Migrator{} }

func (Migrator) Name() string { return "aipicker" }

// dailyResultUniqueIndex 是 SaveDailyResult upsert 依赖的业务联合唯一索引名，
// 必须与 model.go 中 DailyResult 的 uniqueIndex 标签一致。
const dailyResultUniqueIndex = "idx_aipicker_daily_market_date_trigger"

func (Migrator) AutoMigrate(db *gorm.DB) error {
	// 必须在 db.AutoMigrate 之前去重：当旧表已存在但缺唯一索引时，
	// GORM AutoMigrate 见到 struct 上的 uniqueIndex 标签会尝试自动建索引，
	// 若此时存量有重复行会直接失败。先把重复行清掉，AutoMigrate 才能顺利建索引。
	if err := dedupeDailyResults(db); err != nil {
		return err
	}
	if err := db.AutoMigrate(&DailyResult{}, &TechnicalSnapshot{}, &GenerateLogRecord{}); err != nil {
		return err
	}
	// 兜底：极少数情况下 AutoMigrate 不会补建索引（如某些驱动行为差异），
	// 这里再幂等地确保唯一索引存在。
	if err := ensureDailyResultUniqueIndex(db); err != nil {
		return err
	}
	return nil
}

// dedupeDailyResults 在 ai_picker_daily_results 表存在时，清理同一
// (market, trade_date, trigger) 的重复行，保留 id 最大的最新一条。
// 表不存在时（全新库）直接跳过。
func dedupeDailyResults(db *gorm.DB) error {
	table := DailyResult{}.TableName()
	if !db.Migrator().HasTable(table) {
		return nil
	}
	dedupeSQL := fmt.Sprintf(`
		DELETE FROM %q
		WHERE id NOT IN (
			SELECT MAX(id) FROM %q GROUP BY "market", "trade_date", "trigger"
		)
	`, table, table)
	if err := db.Exec(dedupeSQL).Error; err != nil {
		return fmt.Errorf("dedupe %s before unique index: %w", table, err)
	}
	return nil
}

// ensureDailyResultUniqueIndex 幂等地保证 ai_picker_daily_results 上存在
// (market, trade_date, trigger) 联合唯一索引。
func ensureDailyResultUniqueIndex(db *gorm.DB) error {
	if db.Migrator().HasIndex(&DailyResult{}, dailyResultUniqueIndex) {
		return nil
	}
	table := DailyResult{}.TableName()
	createSQL := fmt.Sprintf(
		`CREATE UNIQUE INDEX IF NOT EXISTS %q ON %q ("market", "trade_date", "trigger")`,
		dailyResultUniqueIndex, table,
	)
	if err := db.Exec(createSQL).Error; err != nil {
		return fmt.Errorf("create unique index on %s: %w", table, err)
	}
	return nil
}
