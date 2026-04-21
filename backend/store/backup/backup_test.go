package backup

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")
	db, err := gorm.Open(sqlite.Open(dbPath), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := db.AutoMigrate(&BackupLogRecord{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return db
}

func setupSourceDB(t *testing.T) (*gorm.DB, string, string) {
	t.Helper()
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "pumpkin.db")
	db, err := gorm.Open(sqlite.Open(dbPath), &gorm.Config{})
	if err != nil {
		t.Fatalf("open source db: %v", err)
	}
	if err := db.Exec("PRAGMA journal_mode=WAL;").Error; err != nil {
		t.Fatalf("enable wal: %v", err)
	}
	if err := db.Exec("CREATE TABLE IF NOT EXISTS sample_records (id INTEGER PRIMARY KEY AUTOINCREMENT, name TEXT NOT NULL)").Error; err != nil {
		t.Fatalf("create sample table: %v", err)
	}
	if err := db.Exec("INSERT INTO sample_records(name) VALUES (?)", "seed").Error; err != nil {
		t.Fatalf("insert sample row: %v", err)
	}
	if err := db.AutoMigrate(&BackupLogRecord{}); err != nil {
		t.Fatalf("migrate backup logs: %v", err)
	}
	return db, dir, dbPath
}

func writeCacheFixtures(t *testing.T, dir string) {
	t.Helper()
	for _, name := range []string{"quadrant_cache.db", "quadrant_cache_hk.db"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("cache fixture"), 0644); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}
}

func TestRepository_InsertAndGetLatest(t *testing.T) {
	db := setupTestDB(t)
	repo := NewRepository(db)

	now := time.Now().Truncate(time.Second)
	record := BackupLogRecord{
		TriggeredAt: now,
		TriggerType: "manual",
		Status:      "success",
		PumpkinFile: "pumpkin_test.db",
		PumpkinSize: 3456,
		CreatedAt:   time.Now(),
	}

	if err := repo.Insert(&record); err != nil {
		t.Fatalf("insert: %v", err)
	}
	if record.ID == 0 {
		t.Fatal("expected non-zero ID")
	}

	latest, err := repo.GetLatest()
	if err != nil {
		t.Fatalf("get latest: %v", err)
	}
	if latest.ID != record.ID {
		t.Errorf("expected ID %d, got %d", record.ID, latest.ID)
	}
	if latest.TriggerType != "manual" {
		t.Errorf("expected trigger_type manual, got %s", latest.TriggerType)
	}
}

func TestRepository_GetLatestEmpty(t *testing.T) {
	db := setupTestDB(t)
	repo := NewRepository(db)

	latest, err := repo.GetLatest()
	if err != nil {
		t.Fatalf("get latest empty: %v", err)
	}
	if latest != nil {
		t.Fatalf("expected nil, got %+v", latest)
	}
}

func TestRepository_ListRecent(t *testing.T) {
	db := setupTestDB(t)
	repo := NewRepository(db)

	baseTime := time.Now().Truncate(time.Second).Add(-2 * time.Hour)
	for i := 0; i < 5; i++ {
		ts := baseTime.Add(time.Duration(i) * time.Minute)
		repo.Insert(&BackupLogRecord{
			TriggeredAt: ts,
			TriggerType: "scheduled_fallback",
			Status:      "success",
			CreatedAt:   time.Now(),
		})
	}

	items, err := repo.ListRecent(3)
	if err != nil {
		t.Fatalf("list recent: %v", err)
	}
	if len(items) != 3 {
		t.Errorf("expected 3 items, got %d", len(items))
	}
	if items[0].TriggeredAt.Before(items[1].TriggeredAt) {
		t.Error("items should be ordered by triggered_at DESC")
	}
}

func TestService_Run_CooldownSkips(t *testing.T) {
	db, dir, dbPath := setupSourceDB(t)
	writeCacheFixtures(t, dir)

	svc := NewService(NewRepository(db), db, ServiceConfig{
		DBPath:          dbPath,
		BackupDir:       filepath.Join(dir, "backups"),
		CacheADir:       dir,
		CacheHKDir:      dir,
		RetentionDays:   7,
		CooldownMinutes: 120,
	})

	r1, err := svc.Run(nil, "manual")
	if err != nil {
		t.Fatalf("first run: %v", err)
	}
	if r1.Status != "success" {
		t.Fatalf("expected first run success, got %s (%s)", r1.Status, r1.ErrorMessage)
	}
	if r1.IntegrityCheck != "ok" {
		t.Fatalf("expected integrity_check ok, got %s (%s)", r1.IntegrityCheck, r1.ErrorMessage)
	}
	if _, err := os.Stat(filepath.Join(dir, "backups", r1.PumpkinFile)); err != nil {
		t.Fatalf("expected backup file to exist: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "backups", r1.PumpkinFile+"-wal")); !os.IsNotExist(err) {
		t.Fatalf("expected standalone backup without wal sidecar, err=%v", err)
	}

	r2, err := svc.Run(nil, "manual")
	if err != nil {
		t.Fatalf("second run: %v", err)
	}
	if r2.Status != "skipped" {
		t.Errorf("expected skipped, got %s", r2.Status)
	}
}

func TestService_Run_CacheNotFound(t *testing.T) {
	db, dir, dbPath := setupSourceDB(t)

	svc := NewService(NewRepository(db), db, ServiceConfig{
		DBPath:          dbPath,
		BackupDir:       filepath.Join(dir, "backups"),
		CacheADir:       dir,
		CacheHKDir:      dir,
		RetentionDays:   7,
		CooldownMinutes: 0,
	})

	result, err := svc.Run(nil, "test_cache_missing")
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if result.Status != "partial" {
		t.Fatalf("expected partial, got %s", result.Status)
	}
	if result.IntegrityCheck != "ok" {
		t.Fatalf("expected integrity_check ok, got %s (%s)", result.IntegrityCheck, result.ErrorMessage)
	}
	if !strings.Contains(result.ErrorMessage, "cache_a") || !strings.Contains(result.ErrorMessage, "cache_hk") {
		t.Fatalf("expected cache errors in message, got %s", result.ErrorMessage)
	}
}

func TestModel_TableName(t *testing.T) {
	r := BackupLogRecord{}
	if n := r.TableName(); n != "backup_logs" {
		t.Errorf("expected backup_logs, got %s", n)
	}
}

func TestGetStatus_NeverBackedUp(t *testing.T) {
	db := setupTestDB(t)
	svc := NewService(NewRepository(db), db, ServiceConfig{
		BackupDir: t.TempDir(),
	})

	status, err := svc.GetStatus(nil)
	if err != nil {
		t.Fatalf("get status: %v", err)
	}
	if status.Status != "never" {
		t.Errorf("expected never, got %s", status.Status)
	}
	if status.LastBackupAt != nil {
		t.Error("expected nil LastBackupAt")
	}
}
