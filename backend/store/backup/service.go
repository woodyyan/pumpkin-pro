package backup

import (
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

// Service orchestrates database backups (local + optional COS).
type Service struct {
	repo *Repository
	db   *gorm.DB // main GORM DB instance for pumpkin.db

	// Paths (absolute or relative to CWD)
	dbPath          string // path to pumpkin.db (for hot backup)
	backupDir       string // local backup output directory
	cacheADir       string // directory containing quadrant_cache.db
	cacheHKDir      string // directory containing quadrant_cache_hk.db
	retentionDays   int    // local retention in days
	cooldownMinutes int    // minimum minutes between backups

	// COS configuration (empty = disabled)
	cosBucket    string
	cosRegion    string
	cosPrefix    string
	cosSecretID  string
	cosSecretKey string

	mu           sync.Mutex
	lastBackupAt time.Time
}

// ServiceConfig holds all configurable parameters.
type ServiceConfig struct {
	DBPath          string
	BackupDir       string
	CacheADir       string
	CacheHKDir      string
	RetentionDays   int
	CooldownMinutes int

	// COS (all empty = skip cloud)
	COSBucket    string
	COSRegion    string
	COSPrefix    string
	COSSecretID  string
	COSSecretKey string
}

// NewService creates a new backup service.
func NewService(repo *Repository, db *gorm.DB, cfg ServiceConfig) *Service {
	if cfg.RetentionDays <= 0 {
		cfg.RetentionDays = 7
	}
	if cfg.CooldownMinutes <= 0 {
		cfg.CooldownMinutes = 120 // 2h default
	}
	return &Service{
		repo:            repo,
		db:              db,
		dbPath:          cfg.DBPath,
		backupDir:       cfg.BackupDir,
		cacheADir:       cfg.CacheADir,
		cacheHKDir:      cfg.CacheHKDir,
		retentionDays:   cfg.RetentionDays,
		cooldownMinutes: cfg.CooldownMinutes,
		cosBucket:       cfg.COSBucket,
		cosRegion:       cfg.COSRegion,
		cosPrefix:       cfg.COSPrefix,
		cosSecretID:     cfg.COSSecretID,
		cosSecretKey:    cfg.COSSecretKey,
	}
}

// BackupResult is returned from a single Run() invocation.
type BackupResult struct {
	TriggerType    string // populated by recordAndFinish, used for display
	Status         string
	PumpkinFile    string
	PumpkinSize    int64
	CacheAFile     string
	CacheASize     int64
	CacheHKFile    string
	CacheHKSize    int64
	COSUploaded    bool
	IntegrityCheck string
	ErrorMessage   string
	DurationMS     int64
}

// Run executes a full backup cycle. It is safe to call concurrently —
// if a cooldown is active it returns early with status "skipped".
func (s *Service) Run(ctx context.Context, triggerType string) (*BackupResult, error) {
	start := time.Now()

	// Cooldown check
	s.mu.Lock()
	if !s.lastBackupAt.IsZero() && time.Since(s.lastBackupAt) < time.Duration(s.cooldownMinutes)*time.Minute {
		s.mu.Unlock()
		log.Printf("[backup] skipped: cooldown active (last %s, < %dm)",
			s.lastBackupAt.Format(time.RFC3339), s.cooldownMinutes)
		return &BackupResult{Status: "skipped"}, nil
	}
	s.mu.Unlock()

	result := &BackupResult{
		Status:      "success",
		TriggerType: triggerType,
	}

	// Ensure backup directory exists
	if err := os.MkdirAll(s.backupDir, 0755); err != nil {
		result.Status = "failed"
		result.ErrorMessage = fmt.Sprintf("create backup dir: %v", err)
		s.recordAndFinish(ctx, triggerType, result, start)
		return result, nil
	}

	timestamp := time.Now().Format("20060102_150405")

	// 1. Hot-backup pumpkin.db using SQLite online backup API
	pumpkinFile, pumpkinSize, err := s.hotBackupPumpkin(timestamp)
	if err != nil {
		log.Printf("[backup] pumpkin hot-backup failed: %v", err)
		result.Status = "partial"
		result.ErrorMessage += fmt.Sprintf("pumpkin: %v; ", err)
	} else {
		result.PumpkinFile = pumpkinFile
		result.PumpkinSize = pumpkinSize
	}

	// 2. Cold-copy + gzip cache databases
	cacheAFile, cacheASize, err := s.compressCacheDB("quadrant_cache.db", s.cacheADir, "cache_a", timestamp)
	if err != nil {
		log.Printf("[backup] cache_a copy failed: %v", err)
		if result.Status == "success" {
			result.Status = "partial"
		}
		result.ErrorMessage += fmt.Sprintf("cache_a: %v; ", err)
	} else {
		result.CacheAFile = cacheAFile
		result.CacheASize = cacheASize
	}

	cacheHKFile, cacheHKSize, err := s.compressCacheDB("quadrant_cache_hk.db", s.cacheHKDir, "cache_hk", timestamp)
	if err != nil {
		log.Printf("[backup] cache_hk copy failed: %v", err)
		if result.Status == "success" {
			result.Status = "partial"
		}
		result.ErrorMessage += fmt.Sprintf("cache_hk: %v; ", err)
	} else {
		result.CacheHKFile = cacheHKFile
		result.CacheHKSize = cacheHKSize
	}

	// 3. Integrity check on pumpkin backup (if succeeded)
	if result.PumpkinFile != "" {
		ic, icErr := s.integrityCheck(result.PumpkinFile)
		result.IntegrityCheck = ic
		if ic == "failed" {
			result.Status = "partial"
			if icErr != nil {
				result.ErrorMessage += fmt.Sprintf("integrity_check failed: %v; ", icErr)
			} else {
				result.ErrorMessage += "integrity_check failed; "
			}
		}
	} else if result.PumpkinFile == "" {
		result.IntegrityCheck = "skipped"
	}

	// 4. Cleanup old local backups
	s.cleanupOldBackups()

	// 5. Upload to COS (if configured)
	if s.cosEnabled() {
		uploaded := s.uploadToCOS(ctx, result)
		result.COSUploaded = uploaded
	} else {
		log.Printf("[backup] COS not configured, skipping cloud upload")
	}

	result.DurationMS = time.Since(start).Milliseconds()
	s.recordAndFinish(ctx, triggerType, result, start)

	log.Printf("[backup] ✅ %s | trigger=%s | pumpkin=%dKB | cache_a=%dKB | cache_hk=%dKB | cos=%v | %.1fs",
		result.Status, triggerType,
		result.PumpkinSize/1024, result.CacheASize/1024, result.CacheHKSize/1024,
		result.COSUploaded, float64(result.DurationMS)/1000)

	return result, nil
}

// ── Internal methods ──

func (s *Service) hotBackupPumpkin(timestamp string) (string, int64, error) {
	if s.dbPath == "" {
		return "", 0, fmt.Errorf("db_path not configured")
	}

	destName := fmt.Sprintf("pumpkin_%s.db", timestamp)
	destPath := filepath.Join(s.backupDir, destName)

	if err := os.Remove(destPath); err != nil && !os.IsNotExist(err) {
		return "", 0, fmt.Errorf("cleanup existing dest: %w", err)
	}
	_ = os.Remove(destPath + "-wal")
	_ = os.Remove(destPath + "-shm")

	sourceDB := s.db
	if sourceDB == nil {
		var err error
		sourceDB, err = gorm.Open(sqlite.Open(s.dbPath+"?_pragma=busy_timeout(5000)&_pragma=foreign_keys(ON)"), &gorm.Config{})
		if err != nil {
			return "", 0, fmt.Errorf("open source db: %w", err)
		}
		if sqlDB, sqlErr := sourceDB.DB(); sqlErr == nil {
			defer sqlDB.Close()
		}
	}

	if err := sourceDB.Exec("VACUUM INTO ?", destPath).Error; err != nil {
		_ = os.Remove(destPath)
		return "", 0, fmt.Errorf("vacuum into: %w", err)
	}

	info, statErr := os.Stat(destPath)
	size := int64(0)
	if statErr == nil {
		size = info.Size()
	}

	return destName, size, nil
}

func (s *Service) compressCacheDB(dbFilename, sourceDir, prefix, timestamp string) (string, int64, error) {
	sourcePath := filepath.Join(sourceDir, dbFilename)

	// Check source exists
	if _, err := os.Stat(sourcePath); os.IsNotExist(err) {
		return "", 0, fmt.Errorf("source not found: %s", sourcePath)
	}

	// Read source
	srcF, err := os.Open(sourcePath)
	if err != nil {
		return "", 0, fmt.Errorf("open: %w", err)
	}
	defer srcF.Close()

	// Write gzipped copy
	destName := fmt.Sprintf("%s_%s.db.gz", prefix, timestamp)
	destPath := filepath.Join(s.backupDir, destName)

	dstF, err := os.Create(destPath)
	if err != nil {
		return "", 0, fmt.Errorf("create: %w", err)
	}
	defer dstF.Close()

	gzW := gzip.NewWriter(dstF)
	if _, err := io.Copy(gzW, srcF); err != nil {
		gzW.Close()
		return "", 0, fmt.Errorf("compress: %w", err)
	}
	if err := gzW.Close(); err != nil {
		return "", 0, fmt.Errorf("close gzip: %w", err)
	}

	info, statErr := os.Stat(destPath)
	size := int64(0)
	if statErr == nil {
		size = info.Size()
	}

	return destName, size, nil
}

func (s *Service) integrityCheck(filePath string) (string, error) {
	fullPath := filepath.Join(s.backupDir, filePath)
	db, err := gorm.Open(sqlite.Open(fullPath+"?mode=ro"), &gorm.Config{})
	if err != nil {
		return "failed", err
	}
	var result string
	row := db.Raw("PRAGMA integrity_check").Row()
	if err := row.Scan(&result); err != nil {
		return "failed", err
	}
	sqlDB, _ := db.DB()
	sqlDB.Close()
	if result == "ok" {
		return "ok", nil
	}
	return "failed", fmt.Errorf(result)
}

func (s *Service) cleanupOldBackups() {
	cutoff := time.Now().AddDate(0, 0, -s.retentionDays)

	entries, err := os.ReadDir(s.backupDir)
	if err != nil {
		log.Printf("[backup] cleanup: cannot read dir: %v", err)
		return
	}

	removed := 0
	for _, entry := range entries {
		if entry.IsDir() || strings.HasPrefix(entry.Name(), ".") {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			continue
		}
		if info.ModTime().Before(cutoff) {
			fullPath := filepath.Join(s.backupDir, entry.Name())
			if err := os.Remove(fullPath); err != nil {
				log.Printf("[backup] cleanup: failed to remove %s: %v", entry.Name(), err)
			} else {
				removed++
			}
		}
	}
	if removed > 0 {
		log.Printf("[backup] cleaned up %d old backup files (>%d days)", removed, s.retentionDays)
	}
}

func (s *Service) cosEnabled() bool {
	return s.cosBucket != "" && s.cosSecretID != "" && s.cosSecretKey != ""
}

func (s *Service) uploadToCOS(ctx context.Context, result *BackupResult) bool {
	// COS upload will be implemented once cos SDK dependency is added.
	// For now, log and return false — will be wired up after `go get`.
	files := []struct{ localName string }{}
	if result.PumpkinFile != "" {
		files = append(files, struct{ localName string }{result.PumpkinFile})
	}
	if result.CacheAFile != "" {
		files = append(files, struct{ localName string }{result.CacheAFile})
	}
	if result.CacheHKFile != "" {
		files = append(files, struct{ localName string }{result.CacheHKFile})
	}

	log.Printf("[backup-cos] %d files pending upload (SDK integration next step)", len(files))
	return false // TODO: wire up COS SDK
}

func (s *Service) recordAndFinish(ctx context.Context, triggerType string, result *BackupResult, start time.Time) {
	// Record to DB
	record := BackupLogRecord{
		TriggeredAt:    start,
		TriggerType:    triggerType,
		Status:         result.Status,
		PumpkinFile:    result.PumpkinFile,
		PumpkinSize:    result.PumpkinSize,
		CacheAFile:     result.CacheAFile,
		CacheASize:     result.CacheASize,
		CacheHKFile:    result.CacheHKFile,
		CacheHKSize:    result.CacheHKSize,
		COSUploaded:    result.COSUploaded,
		IntegrityCheck: result.IntegrityCheck,
		ErrorMessage:   result.ErrorMessage,
		DurationMS:     result.DurationMS,
		CreatedAt:      time.Now(),
	}
	if err := s.repo.Insert(&record); err != nil {
		log.Printf("[backup] failed to record log: %v", err)
	}

	// Update last backup time
	s.mu.Lock()
	s.lastBackupAt = start
	s.mu.Unlock()
}

// ── Admin API helpers ──

// GetStatus returns summary of the latest backup for the admin panel.
func (s *Service) GetStatus(ctx context.Context) (*BackupStatusResponse, error) {
	latest, err := s.repo.GetLatest()
	if err != nil {
		return nil, err
	}

	resp := BackupStatusResponse{
		Status: "never",
	}
	if latest != nil {
		ts := latest.TriggeredAt.Local().Format("2006-01-02 15:04:05")
		resp.LastBackupAt = &ts
		resp.LastTriggerType = latest.TriggerType
		resp.Status = latest.Status
		resp.PumpkinSize = latest.PumpkinSize
		resp.CacheASize = latest.CacheASize
		resp.CacheHKSize = latest.CacheHKSize
		resp.COSUploaded = latest.COSUploaded
		resp.DurationMS = latest.DurationMS
		resp.ErrorMsg = latest.ErrorMessage
	}
	return &resp, nil
}

// GetHistory returns recent backup logs for the admin panel.
func (s *Service) GetHistory(limit int) ([]BackupHistoryItem, error) {
	records, err := s.repo.ListRecent(limit)
	if err != nil {
		return nil, err
	}

	items := make([]BackupHistoryItem, len(records))
	for i, r := range records {
		items[i] = BackupHistoryItem{
			ID:             r.ID,
			TriggeredAt:    r.TriggeredAt.Local().Format("2006-01-02 15:04:05"),
			TriggerType:    r.TriggerType,
			Status:         r.Status,
			PumpkinSize:    r.PumpkinSize,
			CacheASize:     r.CacheASize,
			CacheHKSize:    r.CacheHKSize,
			COSUploaded:    r.COSUploaded,
			IntegrityCheck: r.IntegrityCheck,
			ErrorMsg:       r.ErrorMessage,
			DurationMS:     r.DurationMS,
		}
	}
	return items, nil
}

// GetStorageStats returns local + cloud storage usage information.
func (s *Service) GetStorageStats(ctx context.Context) (*BackupStorageStats, error) {
	stats := &BackupStorageStats{
		LocalRetentionDays: s.retentionDays,
		LocalFileCount:     0,
		LocalTotalBytes:    0,
		CloudEnabled:       s.cosEnabled(),
	}

	entries, err := os.ReadDir(s.backupDir)
	if err == nil {
		for _, e := range entries {
			if e.IsDir() || strings.HasPrefix(e.Name(), ".") {
				continue
			}
			info, ferr := e.Info()
			if ferr == nil {
				stats.LocalTotalBytes += info.Size()
				stats.LocalFileCount++
			}
		}
	}

	return stats, nil
}

func ctxBackground() context.Context { return context.Background() }
