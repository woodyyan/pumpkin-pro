package aipicker

import (
	"context"
	"fmt"
	"sync/atomic"
	"testing"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

var memDBSeq int64

// openMemDB 打开一个完全独立的内存数据库。
// 使用唯一 DSN + cache=private，确保每个测试用例互不共享底层内存库，
// 避免 SQLite 在同进程内对 "file::memory:" 复用同一数据库导致的跨用例脏数据。
func openMemDB(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := fmt.Sprintf("file:memdb_%d?mode=memory&cache=shared", atomic.AddInt64(&memDBSeq, 1))
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	// 限制单连接，确保内存库在整个测试期间存活且查询可见同一份数据。
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("get sql db: %v", err)
	}
	sqlDB.SetMaxOpenConns(1)
	return db
}

// TestMigratorCreatesUniqueIndexAndUpsertWorks 验证：全新库经 Migrator 迁移后，
// (market, trade_date, trigger) 上存在唯一索引，且 SaveDailyResult 的 upsert 能正常覆盖。
func TestMigratorCreatesUniqueIndexAndUpsertWorks(t *testing.T) {
	db := openMemDB(t)
	if err := (Migrator{}).AutoMigrate(db); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if !db.Migrator().HasIndex(&DailyResult{}, dailyResultUniqueIndex) {
		t.Fatalf("expected unique index %s to exist", dailyResultUniqueIndex)
	}

	repo := NewRepository(db)
	ctx := context.Background()
	// 同一业务键写两次：第二次应覆盖（upsert），而不是报 ON CONFLICT 错误或追加新行。
	if err := repo.SaveDailyResult(ctx, DailyResult{Market: MarketAShare, TradeDate: "2026-06-16", Trigger: TriggerManual, SnapshotDate: "2026-06-15", SelectionBasis: SelectionBasisFactorLab, Model: "m1", PayloadJSON: `{"v":1}`}); err != nil {
		t.Fatalf("first save: %v", err)
	}
	if err := repo.SaveDailyResult(ctx, DailyResult{Market: MarketAShare, TradeDate: "2026-06-16", Trigger: TriggerManual, SnapshotDate: "2026-06-15", SelectionBasis: SelectionBasisFactorLab, Model: "m2", PayloadJSON: `{"v":2}`}); err != nil {
		t.Fatalf("upsert save: %v", err)
	}

	var count int64
	if err := db.Model(&DailyResult{}).Where("market = ? AND trade_date = ? AND trigger = ?", MarketAShare, "2026-06-16", TriggerManual).Count(&count).Error; err != nil {
		t.Fatalf("count: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected exactly 1 row after upsert, got %d", count)
	}
	got, err := repo.GetLatestDailyResult(ctx, MarketAShare)
	if err != nil {
		t.Fatalf("get latest: %v", err)
	}
	if got.PayloadJSON != `{"v":2}` {
		t.Fatalf("expected payload to be overwritten to v2, got %s", got.PayloadJSON)
	}
}

// TestMigratorHealsLegacyTableWithDuplicateRows 模拟旧脏数据场景：
// 旧表没有唯一索引，且同一 (market, trade_date, trigger) 存在多条重复行；
// 迁移应去重（保留 id 最大的最新一条）并补建唯一索引，随后 upsert 不再报错。
func TestMigratorHealsLegacyTableWithDuplicateRows(t *testing.T) {
	db := openMemDB(t)

	// 1) 用 raw SQL 建一张「没有唯一索引」的旧表，模拟历史 schema。
	if err := db.Exec(`
		CREATE TABLE ai_picker_daily_results (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			market TEXT NOT NULL DEFAULT '',
			trade_date TEXT NOT NULL DEFAULT '',
			trigger TEXT NOT NULL DEFAULT 'daily_auto',
			snapshot_date TEXT NOT NULL DEFAULT '',
			selection_basis TEXT NOT NULL DEFAULT 'factor_lab',
			model TEXT NOT NULL DEFAULT '',
			payload_json TEXT NOT NULL DEFAULT '',
			created_at DATETIME,
			updated_at DATETIME
		)
	`).Error; err != nil {
		t.Fatalf("create legacy table: %v", err)
	}
	// 2) 注入重复脏数据：同一业务键 3 条。
	for i, payload := range []string{`{"v":1}`, `{"v":2}`, `{"v":3}`} {
		if err := db.Exec(
			`INSERT INTO ai_picker_daily_results (market, trade_date, trigger, snapshot_date, selection_basis, model, payload_json) VALUES (?,?,?,?,?,?,?)`,
			MarketAShare, "2026-06-16", TriggerManual, "2026-06-15", SelectionBasisFactorLab, "legacy", payload,
		).Error; err != nil {
			t.Fatalf("insert dup %d: %v", i, err)
		}
	}
	// 另一组无重复的正常数据，迁移后应保留。
	if err := db.Exec(
		`INSERT INTO ai_picker_daily_results (market, trade_date, trigger, snapshot_date, selection_basis, model, payload_json) VALUES (?,?,?,?,?,?,?)`,
		MarketAShare, "2026-06-15", TriggerDailyAuto, "2026-06-14", SelectionBasisFactorLab, "legacy", `{"v":10}`,
	).Error; err != nil {
		t.Fatalf("insert auto: %v", err)
	}

	// 3) 执行迁移自愈。
	if err := (Migrator{}).AutoMigrate(db); err != nil {
		t.Fatalf("migrate legacy: %v", err)
	}
	if !db.Migrator().HasIndex(&DailyResult{}, dailyResultUniqueIndex) {
		t.Fatalf("expected unique index after healing")
	}

	// 4) 重复组应只剩 1 条，且是 id 最大（payload v3）。
	var dupCount int64
	if err := db.Model(&DailyResult{}).Where("market = ? AND trade_date = ? AND trigger = ?", MarketAShare, "2026-06-16", TriggerManual).Count(&dupCount).Error; err != nil {
		t.Fatalf("count dup: %v", err)
	}
	if dupCount != 1 {
		t.Fatalf("expected 1 row after dedupe, got %d", dupCount)
	}
	repo := NewRepository(db)
	ctx := context.Background()
	var kept DailyResult
	if err := db.Where("market = ? AND trade_date = ? AND trigger = ?", MarketAShare, "2026-06-16", TriggerManual).First(&kept).Error; err != nil {
		t.Fatalf("load kept: %v", err)
	}
	if kept.PayloadJSON != `{"v":3}` {
		t.Fatalf("expected to keep newest row v3, got %s", kept.PayloadJSON)
	}

	// 5) 正常数据应保留。
	var total int64
	db.Model(&DailyResult{}).Count(&total)
	if total != 2 {
		t.Fatalf("expected 2 total rows (1 deduped + 1 auto), got %d", total)
	}

	// 6) 现在 upsert 应正常工作，不再报 ON CONFLICT 错误。
	if err := repo.SaveDailyResult(ctx, DailyResult{Market: MarketAShare, TradeDate: "2026-06-16", Trigger: TriggerManual, SnapshotDate: "2026-06-15", SelectionBasis: SelectionBasisFactorLab, Model: "new", PayloadJSON: `{"v":99}`}); err != nil {
		t.Fatalf("upsert after heal: %v", err)
	}
	if err := db.Where("market = ? AND trade_date = ? AND trigger = ?", MarketAShare, "2026-06-16", TriggerManual).First(&kept).Error; err != nil {
		t.Fatalf("reload kept: %v", err)
	}
	if kept.PayloadJSON != `{"v":99}` {
		t.Fatalf("expected upsert to overwrite to v99, got %s", kept.PayloadJSON)
	}
}

// TestMigratorIsIdempotent 验证迁移可重复执行而不报错（HasIndex 短路）。
func TestMigratorIsIdempotent(t *testing.T) {
	db := openMemDB(t)
	for i := 0; i < 3; i++ {
		if err := (Migrator{}).AutoMigrate(db); err != nil {
			t.Fatalf("migrate run %d: %v", i, err)
		}
	}
	if !db.Migrator().HasIndex(&DailyResult{}, dailyResultUniqueIndex) {
		t.Fatalf("expected unique index to exist after repeated migrations")
	}
}
