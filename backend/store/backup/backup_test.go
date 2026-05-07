package backup

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
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

type fakeCloudStorage struct {
	mu          sync.Mutex
	uploads     []string
	uploadErrs  map[string]error
	listItems   []CloudObjectInfo
	listErr     error
	blockUpload chan struct{}
}

func (f *fakeCloudStorage) Upload(ctx context.Context, objectKey, localPath, contentType string) error {
	if f.blockUpload != nil {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-f.blockUpload:
		}
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.uploads = append(f.uploads, fmt.Sprintf("%s|%s|%s", objectKey, filepath.Base(localPath), contentType))
	if f.uploadErrs != nil {
		if err := f.uploadErrs[objectKey]; err != nil {
			return err
		}
	}
	return nil
}

func (f *fakeCloudStorage) List(ctx context.Context, prefix string) ([]CloudObjectInfo, error) {
	if f.listErr != nil {
		return nil, f.listErr
	}
	return append([]CloudObjectInfo(nil), f.listItems...), nil
}

func TestRepository_InsertAndGetLatest(t *testing.T) {
	db := setupTestDB(t)
	repo := NewRepository(db)

	now := time.Now().Truncate(time.Second)
	record := BackupLogRecord{
		TriggeredAt: now,
		TriggerType: "manual",
		Status:      BackupStatusSuccess,
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
			Status:      BackupStatusSuccess,
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
	if r1.Status != BackupStatusSuccess {
		t.Fatalf("expected first run success, got %s (%s)", r1.Status, r1.ErrorMessage)
	}
	if r1.COSStatus != BackupCOSStatusDisabled {
		t.Fatalf("expected COS disabled, got %s", r1.COSStatus)
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
	if r2.Status != BackupStatusSkipped {
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
	if result.Status != BackupStatusPartial {
		t.Fatalf("expected partial, got %s", result.Status)
	}
	if result.IntegrityCheck != "ok" {
		t.Fatalf("expected integrity_check ok, got %s (%s)", result.IntegrityCheck, result.ErrorMessage)
	}
	if !strings.Contains(result.ErrorMessage, "cache_a") || !strings.Contains(result.ErrorMessage, "cache_hk") {
		t.Fatalf("expected cache errors in message, got %s", result.ErrorMessage)
	}
}

func TestService_Run_UploadsToCloud(t *testing.T) {
	db, dir, dbPath := setupSourceDB(t)
	writeCacheFixtures(t, dir)

	svc := NewService(NewRepository(db), db, ServiceConfig{
		DBPath:          dbPath,
		BackupDir:       filepath.Join(dir, "backups"),
		CacheADir:       dir,
		CacheHKDir:      dir,
		RetentionDays:   7,
		CooldownMinutes: 120,
		COSBucket:       "bucket-1",
		COSRegion:       "ap-guangzhou",
		COSPrefix:       "pumpkin-pro-backups",
		COSSecretID:     "secret-id",
		COSSecretKey:    "secret-key",
	})
	cloud := &fakeCloudStorage{}
	svc.cloudClient = cloud

	result, err := svc.Run(nil, "manual")
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if result.COSStatus != BackupCOSStatusSuccess {
		t.Fatalf("expected COS success, got %s (%s)", result.COSStatus, result.COSErrorMessage)
	}
	if !result.COSUploaded {
		t.Fatal("expected COSUploaded true")
	}
	if len(cloud.uploads) != 3 {
		t.Fatalf("expected 3 uploads, got %d", len(cloud.uploads))
	}
	if !strings.Contains(cloud.uploads[0], "pumpkin-pro-backups/") {
		t.Fatalf("expected prefix in upload key, got %q", cloud.uploads[0])
	}
}

func TestService_Run_PartialCloudFailure(t *testing.T) {
	db, dir, dbPath := setupSourceDB(t)
	writeCacheFixtures(t, dir)

	svc := NewService(NewRepository(db), db, ServiceConfig{
		DBPath:          dbPath,
		BackupDir:       filepath.Join(dir, "backups"),
		CacheADir:       dir,
		CacheHKDir:      dir,
		RetentionDays:   7,
		CooldownMinutes: 120,
		COSBucket:       "bucket-1",
		COSRegion:       "ap-guangzhou",
		COSPrefix:       "pumpkin-pro-backups",
		COSSecretID:     "secret-id",
		COSSecretKey:    "secret-key",
	})
	cloud := &fakeCloudStorage{}
	svc.cloudClient = cloud

	result2 := &BackupResult{
		PumpkinFile: "pumpkin_1.db",
		CacheAFile:  "cache_a_1.db.gz",
		CacheHKFile: "cache_hk_1.db.gz",
	}
	if err := os.MkdirAll(svc.backupDir, 0755); err != nil {
		t.Fatalf("mkdir backup dir: %v", err)
	}
	for _, file := range []string{result2.PumpkinFile, result2.CacheAFile, result2.CacheHKFile} {
		if err := os.WriteFile(filepath.Join(svc.backupDir, file), []byte("fixture"), 0644); err != nil {
			t.Fatalf("write fixture %s: %v", file, err)
		}
	}
	cloud.uploadErrs = map[string]error{
		svc.objectKey(result2.CacheHKFile): errors.New("cos write failed"),
	}
	jobID := newBackupJobID("manual")
	svc.job = &BackupJobState{ID: jobID, Status: BackupJobStatusRunning}
	svc.uploadToCOS(context.Background(), jobID, result2)
	if result2.COSStatus != BackupCOSStatusPartial {
		t.Fatalf("expected partial COS status, got %s", result2.COSStatus)
	}
	if !result2.COSUploaded {
		t.Fatal("expected COSUploaded true for partial success")
	}
	if !strings.Contains(result2.COSErrorMessage, "cache_hk") {
		t.Fatalf("expected cache_hk error message, got %s", result2.COSErrorMessage)
	}
}

func TestService_TriggerAsyncAndStatus(t *testing.T) {
	db, dir, dbPath := setupSourceDB(t)
	writeCacheFixtures(t, dir)

	svc := NewService(NewRepository(db), db, ServiceConfig{
		DBPath:          dbPath,
		BackupDir:       filepath.Join(dir, "backups"),
		CacheADir:       dir,
		CacheHKDir:      dir,
		RetentionDays:   7,
		CooldownMinutes: 120,
		COSBucket:       "bucket-1",
		COSRegion:       "ap-guangzhou",
		COSPrefix:       "pumpkin-pro-backups",
		COSSecretID:     "secret-id",
		COSSecretKey:    "secret-key",
	})
	blockUpload := make(chan struct{})
	cloud := &fakeCloudStorage{blockUpload: blockUpload}
	svc.cloudClient = cloud

	resp, err := svc.TriggerAsync(context.Background(), "manual")
	if err != nil {
		t.Fatalf("trigger async: %v", err)
	}
	if !resp.Accepted {
		t.Fatalf("expected accepted response, got %+v", resp)
	}

	released := false
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		status, err := svc.GetStatus(context.Background())
		if err != nil {
			t.Fatalf("get status: %v", err)
		}
		if status.CurrentJobStatus == BackupJobStatusRunning || status.CurrentJobStatus == BackupJobStatusQueued {
			if status.CurrentJobID == "" {
				t.Fatal("expected current job id while running")
			}
			if !released {
				close(blockUpload)
				released = true
			}
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if !released {
		close(blockUpload)
	}

	deadline = time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		status, err := svc.GetStatus(context.Background())
		if err != nil {
			t.Fatalf("get status: %v", err)
		}
		if status.CurrentJobFinishedAt != nil {
			if status.COSStatus != BackupCOSStatusSuccess {
				t.Fatalf("expected COS success after job finished, got %s", status.COSStatus)
			}
			if status.CurrentJobStatus != BackupStatusSuccess {
				t.Fatalf("expected current job status success, got %s", status.CurrentJobStatus)
			}
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("timed out waiting for async backup completion")
}

func TestService_TriggerAsyncReturnsConflictWhenRunning(t *testing.T) {
	db := setupTestDB(t)
	svc := NewService(NewRepository(db), db, ServiceConfig{BackupDir: t.TempDir()})
	svc.job = &BackupJobState{ID: "job-1", Status: BackupJobStatusRunning, Phase: "uploading"}

	resp, err := svc.TriggerAsync(context.Background(), "manual")
	if err != nil {
		t.Fatalf("trigger async: %v", err)
	}
	if resp.Accepted {
		t.Fatalf("expected conflict response, got %+v", resp)
	}
	if resp.Reason != "running" {
		t.Fatalf("expected running reason, got %s", resp.Reason)
	}
}

func TestService_GetStorageStatsCloudError(t *testing.T) {
	db := setupTestDB(t)
	svc := NewService(NewRepository(db), db, ServiceConfig{
		BackupDir:    t.TempDir(),
		COSBucket:    "bucket-1",
		COSRegion:    "ap-guangzhou",
		COSSecretID:  "secret-id",
		COSSecretKey: "secret-key",
	})
	svc.cloudClient = &fakeCloudStorage{listErr: errors.New("list failed")}

	stats, err := svc.GetStorageStats(context.Background())
	if err != nil {
		t.Fatalf("get storage stats: %v", err)
	}
	if stats.CloudErrorMsg != "list failed" {
		t.Fatalf("expected cloud error message, got %q", stats.CloudErrorMsg)
	}
}

func TestService_RecordAndFinishUpdatesCooldownWithoutRepo(t *testing.T) {
	svc := NewService(nil, nil, ServiceConfig{BackupDir: t.TempDir()})
	start := time.Now().Add(-time.Minute).Truncate(time.Second)
	svc.recordAndFinish("manual", &BackupResult{Status: BackupStatusSuccess}, start)
	if !svc.lastBackupAt.Equal(start) {
		t.Fatalf("expected lastBackupAt updated to %s, got %s", start, svc.lastBackupAt)
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
	if status.Status != BackupStatusNever {
		t.Errorf("expected never, got %s", status.Status)
	}
	if status.COSStatus != BackupCOSStatusDisabled {
		t.Fatalf("expected disabled COS status, got %s", status.COSStatus)
	}
	if status.LastBackupAt != nil {
		t.Error("expected nil LastBackupAt")
	}
}
