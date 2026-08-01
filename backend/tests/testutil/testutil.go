package testutil

import (
	"fmt"
	"sync/atomic"
	"testing"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

var memDBSeq int64

// InMemoryDB returns an isolated SQLite in-memory database for a test.
// Uses glebarez/sqlite (pure Go, no CGO) — works in any CI environment.
//
// Two SQLite in-memory pitfalls are handled here:
//  1. A plain ":memory:" DSN gives every pooled connection its own private,
//     empty database. Any concurrent access — e.g. the async best-effort
//     goroutine spawned by BulkSave, or service code that issues a nested
//     query through the root handle while inside a transaction — can then be
//     handed a connection whose database has no tables, surfacing as flaky
//     "no such table" errors (CI runs with -race, which widens the race
//     window).
//  2. A shared "file::memory:" DSN is reused process-wide, which leaks data
//     across tests in the same package.
//
// A uniquely named shared-cache DSN gives each call an isolated in-memory
// database whose pooled connections all see the same data, mirroring the
// multi-connection pool used against the file-based production database.
// (Pinning the pool to a single connection is NOT an option: transaction
// callbacks that query through the root handle would deadlock waiting for
// the pinned connection.)
func InMemoryDB(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := fmt.Sprintf("file:memdb_%d?mode=memory&cache=shared&_fk=1", atomic.AddInt64(&memDBSeq, 1))
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatalf("failed to open in-memory sqlite db: %v", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("failed to get underlying sql.DB: %v", err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })
	return db
}

// AutoMigrateModels runs AutoMigrate for one or more model structs.
// Call this after InMemoryDB to set up tables.
func AutoMigrateModels(t *testing.T, db *gorm.DB, models ...any) {
	t.Helper()
	if err := db.AutoMigrate(models...); err != nil {
		t.Fatalf("auto-migrate failed: %v", err)
	}
}
