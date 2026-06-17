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
