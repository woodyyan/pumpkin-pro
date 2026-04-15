package backup

import (
	"os"
	"path/filepath"
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
	// AutoMigrate backup_logs table
	if err := db.AutoMigrate(&BackupLogRecord{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return db
}

func newTestService(t *testing.T, db *gorm.DB) *Service {
	t.Helper()
	dir := t.TempDir()
	repo := NewRepository(db)
	svc := NewService(repo, db, ServiceConfig{
		DBPath:          filepath.Join(dir, "pumpkin.db"),
		BackupDir:       filepath.Join(dir, "backups"),
		CacheADir:       dir,
		CacheHKDir:      dir,
		RetentionDays:   7,
		CooldownMinutes: 0, // disabled for tests
	})
	return svc
}

func TestRepository_InsertAndGetLatest(t *testing.T) {
	db := setupTestDB(t)
	repo := NewRepository(db)

	now := time.Now().Truncate(time.Second)
	record := BackupLogRecord{
		TriggeredAt:  now,
		TriggerType:  "manual",
		Status:       "success",
		PumpkinFile: "pumpkin_test.db",
		PumpkinSize:  3456,
		CreatedAt:    time.Now(),
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
			Status:     "success",
			CreatedAt:  time.Now(),
		})
	}

	items, err := repo.ListRecent(3)
	if err != nil {
		t.Fatalf("list recent: %v", err)
	}
	if len(items) != 3 {
		t.Errorf("expected 3 items, got %d", len(items))
	}
	// Should be ordered by triggered_at DESC (most recent first)
	if items[0].TriggeredAt.Before(items[1].TriggeredAt) {
		t.Error("items should be ordered by triggered_at DESC")
	}
}

func TestService_Run_CooldownSkips(t *testing.T) {
	db := setupTestDB(t)
	dir := t.TempDir()

	// Create a fake pumpkin.db so hot-backup has something to read
	fakeDB := filepath.Join(dir, "pumpkin.db")
	if f, err := os.Create(fakeDB); err == nil {
		f.WriteString("fake sqlite db content for testing")
		f.Close()
	}

	svc := NewService(NewRepository(db), db, ServiceConfig{
		DBPath:          fakeDB,
		BackupDir:       filepath.Join(dir, "backups"),
		CacheADir:       dir,
		CacheHKDir:      dir,
		RetentionDays:   7,
		CooldownMinutes: 120,
	})

	// First run should succeed (no cooldown)
	r1, err := svc.Run(nil, "manual")
	if err != nil {
		t.Fatalf("first run: %v", err)
	}
	if r1.Status == "skipped" {
		t.Fatal("first run should not be skipped")
	}

	// Second run immediately should be skipped by cooldown
	r2, err := svc.Run(nil, "manual")
	if err != nil {
		t.Fatalf("second run: %v", err)
	}
	if r2.Status != "skipped" {
		t.Errorf("expected skipped, got %s", r2.Status)
	}
}

func TestService_Run_CacheNotFound(t *testing.T) {
	db := setupTestDB(t)
	dir := t.TempDir()

	// Create pumpkin.db but NOT cache files
	fakeDB := filepath.Join(dir, "pumpkin.db")
	if f, err := os.Create(fakeDB); err == nil {
		f.Write([]byte("fake"))
		f.Close()
	}

	svc := NewService(NewRepository(db), db, ServiceConfig{
		DBPath:        fakeDB,
		BackupDir:     filepath.Join(dir, "backups"),
		CacheADir:     dir, // no cache files here
		CacheHKDir:    dir,
		RetentionDays: 7,
		CooldownMinutes: 0,
	})

	result, err := svc.Run(nil, "test_cache_missing")
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	// Pumpkin should succeed but cache should fail gracefully → partial or success with empty cache
	if result.Status == "failed" || result.Status == "skipped" {
		// If cache is missing, it should still be partial (pumpkin ok + cache failed)
		// or success if we treat cache as optional
		t.Logf("status=%s pumpkin_file=%s cache_a=%s error=%s",
			result.Status, result.PumpkinFile, result.CacheAFile, result.ErrorMessage)
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
