package testutil

import (
	"testing"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// InMemoryDB returns a SQLite in-memory database connection.
// Uses glebarez/sqlite (pure Go, no CGO) — works in any CI environment.
func InMemoryDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:?_fk=1"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatalf("failed to open in-memory sqlite db: %v", err)
	}
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
