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
	"github.com/google/uuid"
	"gorm.io/gorm"
)

const (
	BackupStatusSuccess = "success"
	BackupStatusPartial = "partial"
	BackupStatusFailed  = "failed"
	BackupStatusSkipped = "skipped"
	BackupStatusNever   = "never"

	BackupCOSStatusDisabled  = "disabled"
	BackupCOSStatusNever     = "never"
	BackupCOSStatusPending   = "pending"
	BackupCOSStatusUploading = "uploading"
	BackupCOSStatusSuccess   = "success"
	BackupCOSStatusPartial   = "partial"
	BackupCOSStatusFailed    = "failed"
	BackupCOSStatusSkipped   = "skipped"

	BackupJobStatusIdle    = "idle"
	BackupJobStatusQueued  = "queued"
	BackupJobStatusRunning = "running"
)

// Service orchestrates database backups (local + optional COS).
type Service struct {
	repo *Repository
	db   *gorm.DB

	// Paths (absolute or relative to CWD)
	dbPath          string
	backupDir       string
	cacheADir       string
	cacheHKDir      string
	retentionDays   int
	cooldownMinutes int

	// COS configuration (empty = disabled)
	cosBucket    string
	cosRegion    string
	cosPrefix    string
	cosSecretID  string
	cosSecretKey string

	cloudClient CloudStorageClient
	now         func() time.Time

	mu           sync.Mutex
	lastBackupAt time.Time
	job          *BackupJobState
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

// BackupResult is returned from a single Run() invocation.
type BackupResult struct {
	TriggerType     string
	Status          string
	PumpkinFile     string
	PumpkinSize     int64
	CacheAFile      string
	CacheASize      int64
	CacheHKFile     string
	CacheHKSize     int64
	COSUploaded     bool
	COSStatus       string
	COSErrorMessage string
	IntegrityCheck  string
	ErrorMessage    string
	DurationMS      int64
}

type BackupJobState struct {
	ID          string
	Status      string
	Phase       string
	TriggerType string
	Message     string
	StartedAt   time.Time
	FinishedAt  time.Time
}

type backupArtifact struct {
	localName    string
	localPath    string
	contentType  string
	displayLabel string
}

// NewService creates a new backup service.
func NewService(repo *Repository, db *gorm.DB, cfg ServiceConfig) *Service {
	if cfg.RetentionDays <= 0 {
		cfg.RetentionDays = 7
	}
	if cfg.CooldownMinutes <= 0 {
		cfg.CooldownMinutes = 120
	}
	svc := &Service{
		repo:            repo,
		db:              db,
		dbPath:          cfg.DBPath,
		backupDir:       cfg.BackupDir,
		cacheADir:       cfg.CacheADir,
		cacheHKDir:      cfg.CacheHKDir,
		retentionDays:   cfg.RetentionDays,
		cooldownMinutes: cfg.CooldownMinutes,
		cosBucket:       strings.TrimSpace(cfg.COSBucket),
		cosRegion:       strings.TrimSpace(cfg.COSRegion),
		cosPrefix:       normalizeCOSPrefix(cfg.COSPrefix),
		cosSecretID:     strings.TrimSpace(cfg.COSSecretID),
		cosSecretKey:    strings.TrimSpace(cfg.COSSecretKey),
		now:             time.Now,
	}
	if svc.cosEnabled() {
		svc.cloudClient = NewCOSCloudStorageClient(svc.cosBucket, svc.cosRegion, svc.cosSecretID, svc.cosSecretKey)
	}
	return svc
}

// Run executes a full backup cycle synchronously.
func (s *Service) Run(ctx context.Context, triggerType string) (*BackupResult, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	job, skip := s.beginJob(triggerType, BackupJobStatusRunning, "preparing", "开始执行备份")
	if skip != nil {
		return skip, nil
	}
	return s.executeRun(ctx, job.ID, triggerType), nil
}

// TriggerAsync schedules a manual backup and returns immediately.
func (s *Service) TriggerAsync(ctx context.Context, triggerType string) (*BackupTriggerResponse, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	job, resp := s.beginAsyncJob(triggerType)
	if job == nil {
		return resp, nil
	}
	go func(jobID string, trigger string) {
		s.setJobProgress(jobID, BackupJobStatusRunning, "preparing", "开始执行备份")
		s.executeRun(context.Background(), jobID, trigger)
	}(job.ID, triggerType)
	return resp, nil
}

func (s *Service) beginAsyncJob(triggerType string) (*BackupJobState, *BackupTriggerResponse) {
	now := s.now()

	s.mu.Lock()
	defer s.mu.Unlock()

	if err := s.hydrateLastBackupAtLocked(); err != nil {
		return nil, &BackupTriggerResponse{
			Accepted:         false,
			Reason:           "state_error",
			Message:          fmt.Sprintf("读取备份状态失败: %v", err),
			CurrentJobStatus: s.currentJobStatusLocked(),
		}
	}
	if s.hasActiveJobLocked() {
		job := s.cloneJobLocked()
		return nil, &BackupTriggerResponse{
			Accepted:            false,
			Reason:              "running",
			Message:             "已有备份任务在执行，请等待当前任务完成",
			JobID:               job.ID,
			CurrentJobStatus:    job.Status,
			CurrentJobPhase:     job.Phase,
			CurrentJobStartedAt: formatTimePtr(job.StartedAt),
		}
	}
	if next := s.nextAllowedAtLocked(now); next != nil {
		return nil, &BackupTriggerResponse{
			Accepted:         false,
			Reason:           "cooldown",
			Message:          fmt.Sprintf("冷却中，请在 %s 后重试", next.Local().Format("2006-01-02 15:04:05")),
			CurrentJobStatus: BackupJobStatusIdle,
			NextAllowedAt:    formatTimePtr(*next),
		}
	}

	job := &BackupJobState{
		ID:          newBackupJobID(triggerType),
		Status:      BackupJobStatusQueued,
		Phase:       "queued",
		TriggerType: triggerType,
		Message:     "备份任务已入队，等待执行",
		StartedAt:   now,
	}
	s.job = job

	return s.cloneJobLocked(), &BackupTriggerResponse{
		Accepted:            true,
		Reason:              "accepted",
		Message:             "备份任务已受理，后台开始执行",
		JobID:               job.ID,
		CurrentJobStatus:    job.Status,
		CurrentJobPhase:     job.Phase,
		CurrentJobStartedAt: formatTimePtr(job.StartedAt),
	}
}

func (s *Service) beginJob(triggerType, status, phase, message string) (*BackupJobState, *BackupResult) {
	now := s.now()

	s.mu.Lock()
	defer s.mu.Unlock()

	if err := s.hydrateLastBackupAtLocked(); err != nil {
		return nil, &BackupResult{Status: BackupStatusFailed, ErrorMessage: fmt.Sprintf("load latest backup state: %v", err)}
	}
	if s.hasActiveJobLocked() {
		return nil, &BackupResult{Status: BackupStatusSkipped, ErrorMessage: "backup already running"}
	}
	if next := s.nextAllowedAtLocked(now); next != nil {
		return nil, &BackupResult{Status: BackupStatusSkipped, ErrorMessage: fmt.Sprintf("cooldown active until %s", next.Local().Format("2006-01-02 15:04:05"))}
	}

	job := &BackupJobState{
		ID:          newBackupJobID(triggerType),
		Status:      status,
		Phase:       phase,
		TriggerType: triggerType,
		Message:     message,
		StartedAt:   now,
	}
	s.job = job
	return s.cloneJobLocked(), nil
}

func (s *Service) executeRun(ctx context.Context, jobID, triggerType string) *BackupResult {
	start := s.now()
	result := &BackupResult{
		Status:      BackupStatusSuccess,
		TriggerType: triggerType,
		COSStatus:   s.defaultCOSStatus(),
	}

	defer func() {
		result.DurationMS = s.now().Sub(start).Milliseconds()
		s.recordAndFinish(triggerType, result, start)
		s.finishJob(jobID, result)
		log.Printf("[backup] %s | trigger=%s | pumpkin=%dKB | cache_a=%dKB | cache_hk=%dKB | cos=%s | %.1fs",
			result.Status, triggerType,
			result.PumpkinSize/1024, result.CacheASize/1024, result.CacheHKSize/1024,
			result.COSStatus, float64(result.DurationMS)/1000)
	}()

	s.setJobProgress(jobID, BackupJobStatusRunning, "prepare_dir", "准备本地备份目录")
	if err := os.MkdirAll(s.backupDir, 0755); err != nil {
		result.Status = BackupStatusFailed
		result.ErrorMessage = fmt.Sprintf("create backup dir: %v", err)
		return result
	}

	timestamp := s.now().Format("20060102_150405")

	s.setJobProgress(jobID, BackupJobStatusRunning, "backup_pumpkin", "备份主库 pumpkin.db")
	pumpkinFile, pumpkinSize, err := s.hotBackupPumpkin(timestamp)
	if err != nil {
		log.Printf("[backup] pumpkin hot-backup failed: %v", err)
		result.Status = BackupStatusPartial
		result.ErrorMessage += fmt.Sprintf("pumpkin: %v; ", err)
	} else {
		result.PumpkinFile = pumpkinFile
		result.PumpkinSize = pumpkinSize
	}

	s.setJobProgress(jobID, BackupJobStatusRunning, "backup_cache_a", "压缩 A 股缓存数据库")
	cacheAFile, cacheASize, err := s.compressCacheDB("quadrant_cache.db", s.cacheADir, "cache_a", timestamp)
	if err != nil {
		log.Printf("[backup] cache_a copy failed: %v", err)
		if result.Status == BackupStatusSuccess {
			result.Status = BackupStatusPartial
		}
		result.ErrorMessage += fmt.Sprintf("cache_a: %v; ", err)
	} else {
		result.CacheAFile = cacheAFile
		result.CacheASize = cacheASize
	}

	s.setJobProgress(jobID, BackupJobStatusRunning, "backup_cache_hk", "压缩港股缓存数据库")
	cacheHKFile, cacheHKSize, err := s.compressCacheDB("quadrant_cache_hk.db", s.cacheHKDir, "cache_hk", timestamp)
	if err != nil {
		log.Printf("[backup] cache_hk copy failed: %v", err)
		if result.Status == BackupStatusSuccess {
			result.Status = BackupStatusPartial
		}
		result.ErrorMessage += fmt.Sprintf("cache_hk: %v; ", err)
	} else {
		result.CacheHKFile = cacheHKFile
		result.CacheHKSize = cacheHKSize
	}

	if result.PumpkinFile != "" {
		s.setJobProgress(jobID, BackupJobStatusRunning, "integrity_check", "校验主库备份完整性")
		ic, icErr := s.integrityCheck(result.PumpkinFile)
		result.IntegrityCheck = ic
		if ic == BackupStatusFailed {
			result.Status = BackupStatusPartial
			if icErr != nil {
				result.ErrorMessage += fmt.Sprintf("integrity_check failed: %v; ", icErr)
			} else {
				result.ErrorMessage += "integrity_check failed; "
			}
		}
	} else {
		result.IntegrityCheck = BackupCOSStatusSkipped
	}

	if !result.hasLocalArtifacts() {
		result.Status = BackupStatusFailed
	}

	s.setJobProgress(jobID, BackupJobStatusRunning, "cleanup_local", "清理过期本地备份")
	s.cleanupOldBackups()

	if s.cosEnabled() {
		s.uploadToCOS(ctx, jobID, result)
	} else {
		result.COSStatus = BackupCOSStatusDisabled
		result.COSErrorMessage = ""
		log.Printf("[backup] COS not configured, skipping cloud upload")
	}

	return result
}

// GetStatus returns summary of the latest backup for the admin panel.
func (s *Service) GetStatus(ctx context.Context) (*BackupStatusResponse, error) {
	latest, err := s.repo.GetLatest()
	if err != nil {
		return nil, err
	}

	resp := &BackupStatusResponse{
		Status:           BackupStatusNever,
		COSStatus:        s.defaultCOSStatus(),
		CurrentJobStatus: BackupJobStatusIdle,
	}
	if latest != nil {
		resp.LastBackupAt = formatTimePtr(latest.TriggeredAt)
		resp.LastTriggerType = latest.TriggerType
		resp.Status = latest.Status
		resp.PumpkinSize = latest.PumpkinSize
		resp.CacheASize = latest.CacheASize
		resp.CacheHKSize = latest.CacheHKSize
		resp.COSUploaded = latest.COSUploaded
		resp.COSStatus = latest.COSStatus
		if resp.COSStatus == "" {
			resp.COSStatus = s.defaultCOSStatus()
		}
		resp.COSErrorMsg = latest.COSErrorMessage
		resp.DurationMS = latest.DurationMS
		resp.ErrorMsg = latest.ErrorMessage
	}

	s.mu.Lock()
	_ = s.hydrateLastBackupAtLocked()
	job := s.cloneJobLocked()
	next := s.nextAllowedAtLocked(s.now())
	s.mu.Unlock()

	if job != nil {
		resp.CurrentJobID = job.ID
		resp.CurrentJobStatus = job.Status
		resp.CurrentJobPhase = job.Phase
		resp.CurrentJobTriggerType = job.TriggerType
		resp.CurrentJobMessage = job.Message
		resp.CurrentJobStartedAt = formatTimePtr(job.StartedAt)
		resp.CurrentJobFinishedAt = formatTimePtr(job.FinishedAt)
	}
	if next != nil {
		resp.NextAllowedAt = formatTimePtr(*next)
	}

	return resp, nil
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
			COSStatus:      firstNonEmpty(r.COSStatus, s.defaultCOSStatus()),
			COSErrorMsg:    r.COSErrorMessage,
			IntegrityCheck: r.IntegrityCheck,
			ErrorMsg:       r.ErrorMessage,
			DurationMS:     r.DurationMS,
		}
	}
	return items, nil
}

// GetStorageStats returns local + cloud storage usage information.
func (s *Service) GetStorageStats(ctx context.Context) (*BackupStorageStats, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	stats := &BackupStorageStats{
		LocalRetentionDays: s.retentionDays,
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

	if !s.cosEnabled() {
		return stats, nil
	}
	if s.cloudClient == nil {
		stats.CloudErrorMsg = "cloud storage client not initialized"
		return stats, nil
	}
	objects, err := s.cloudClient.List(ctx, s.cosPrefix)
	if err != nil {
		stats.CloudErrorMsg = err.Error()
		return stats, nil
	}
	for _, object := range objects {
		stats.CloudFileCount++
		stats.CloudTotalBytes += object.Size
	}
	return stats, nil
}

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
	if _, err := os.Stat(sourcePath); os.IsNotExist(err) {
		return "", 0, fmt.Errorf("source not found: %s", sourcePath)
	}

	srcF, err := os.Open(sourcePath)
	if err != nil {
		return "", 0, fmt.Errorf("open: %w", err)
	}
	defer srcF.Close()

	destName := fmt.Sprintf("%s_%s.db.gz", prefix, timestamp)
	destPath := filepath.Join(s.backupDir, destName)

	dstF, err := os.Create(destPath)
	if err != nil {
		return "", 0, fmt.Errorf("create: %w", err)
	}
	defer dstF.Close()

	gzW := gzip.NewWriter(dstF)
	if _, err := io.Copy(gzW, srcF); err != nil {
		_ = gzW.Close()
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
		return BackupStatusFailed, err
	}
	var result string
	row := db.Raw("PRAGMA integrity_check").Row()
	if err := row.Scan(&result); err != nil {
		return BackupStatusFailed, err
	}
	sqlDB, _ := db.DB()
	_ = sqlDB.Close()
	if result == "ok" {
		return "ok", nil
	}
	return BackupStatusFailed, fmt.Errorf(result)
}

func (s *Service) cleanupOldBackups() {
	cutoff := s.now().AddDate(0, 0, -s.retentionDays)
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
	return s.cosBucket != "" && s.cosRegion != "" && s.cosSecretID != "" && s.cosSecretKey != ""
}

func (s *Service) uploadToCOS(ctx context.Context, jobID string, result *BackupResult) {
	artifacts := s.collectArtifacts(result)
	if len(artifacts) == 0 {
		result.COSStatus = BackupCOSStatusSkipped
		result.COSErrorMessage = "no local artifacts available for cloud upload"
		return
	}
	if s.cloudClient == nil {
		result.COSStatus = BackupCOSStatusFailed
		result.COSErrorMessage = "cloud storage client not initialized"
		return
	}

	s.setJobProgress(jobID, BackupJobStatusRunning, "uploading_cloud", fmt.Sprintf("上传 %d 个备份文件到 COS", len(artifacts)))
	result.COSStatus = BackupCOSStatusUploading

	successCount := 0
	var failures []string
	for _, artifact := range artifacts {
		if err := s.cloudClient.Upload(ctx, s.objectKey(artifact.localName), artifact.localPath, artifact.contentType); err != nil {
			failures = append(failures, fmt.Sprintf("%s: %v", artifact.displayLabel, err))
			continue
		}
		successCount++
	}

	switch {
	case successCount == len(artifacts):
		result.COSUploaded = true
		result.COSStatus = BackupCOSStatusSuccess
	case successCount > 0:
		result.COSUploaded = true
		result.COSStatus = BackupCOSStatusPartial
	default:
		result.COSUploaded = false
		result.COSStatus = BackupCOSStatusFailed
	}
	result.COSErrorMessage = strings.Join(failures, "; ")
}

func (s *Service) collectArtifacts(result *BackupResult) []backupArtifact {
	artifacts := make([]backupArtifact, 0, 3)
	if result.PumpkinFile != "" {
		artifacts = append(artifacts, backupArtifact{
			localName:    result.PumpkinFile,
			localPath:    filepath.Join(s.backupDir, result.PumpkinFile),
			contentType:  "application/x-sqlite3",
			displayLabel: "pumpkin",
		})
	}
	if result.CacheAFile != "" {
		artifacts = append(artifacts, backupArtifact{
			localName:    result.CacheAFile,
			localPath:    filepath.Join(s.backupDir, result.CacheAFile),
			contentType:  "application/gzip",
			displayLabel: "cache_a",
		})
	}
	if result.CacheHKFile != "" {
		artifacts = append(artifacts, backupArtifact{
			localName:    result.CacheHKFile,
			localPath:    filepath.Join(s.backupDir, result.CacheHKFile),
			contentType:  "application/gzip",
			displayLabel: "cache_hk",
		})
	}
	return artifacts
}

func (s *Service) objectKey(localName string) string {
	if s.cosPrefix == "" {
		return localName
	}
	return s.cosPrefix + localName
}

func (s *Service) recordAndFinish(triggerType string, result *BackupResult, start time.Time) {
	record := BackupLogRecord{
		TriggeredAt:     start,
		TriggerType:     triggerType,
		Status:          result.Status,
		PumpkinFile:     result.PumpkinFile,
		PumpkinSize:     result.PumpkinSize,
		CacheAFile:      result.CacheAFile,
		CacheASize:      result.CacheASize,
		CacheHKFile:     result.CacheHKFile,
		CacheHKSize:     result.CacheHKSize,
		COSUploaded:     result.COSUploaded,
		COSStatus:       result.COSStatus,
		COSErrorMessage: result.COSErrorMessage,
		IntegrityCheck:  result.IntegrityCheck,
		ErrorMessage:    result.ErrorMessage,
		DurationMS:      result.DurationMS,
		CreatedAt:       s.now(),
	}
	if s.repo != nil {
		if err := s.repo.Insert(&record); err != nil {
			log.Printf("[backup] failed to record log: %v", err)
		}
	}

	s.mu.Lock()
	s.lastBackupAt = start
	s.mu.Unlock()
}

func (s *Service) finishJob(jobID string, result *BackupResult) {
	message := "备份完成"
	if result.Status == BackupStatusPartial {
		message = "备份部分成功"
	}
	if result.Status == BackupStatusFailed {
		message = firstNonEmpty(result.ErrorMessage, "备份失败")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.job == nil || s.job.ID != jobID {
		return
	}
	s.job.Status = result.Status
	s.job.Phase = "completed"
	s.job.Message = message
	s.job.FinishedAt = s.now()
}

func (s *Service) setJobProgress(jobID, status, phase, message string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.job == nil || s.job.ID != jobID {
		return
	}
	s.job.Status = status
	s.job.Phase = phase
	s.job.Message = message
}

func (s *Service) hasActiveJobLocked() bool {
	if s.job == nil {
		return false
	}
	return s.job.Status == BackupJobStatusQueued || s.job.Status == BackupJobStatusRunning
}

func (s *Service) currentJobStatusLocked() string {
	if s.job == nil {
		return BackupJobStatusIdle
	}
	return s.job.Status
}

func (s *Service) cloneJobLocked() *BackupJobState {
	if s.job == nil {
		return nil
	}
	copy := *s.job
	return &copy
}

func (s *Service) hydrateLastBackupAtLocked() error {
	if !s.lastBackupAt.IsZero() || s.repo == nil {
		return nil
	}
	latest, err := s.repo.GetLatest()
	if err != nil {
		return err
	}
	if latest != nil {
		s.lastBackupAt = latest.TriggeredAt
	}
	return nil
}

func (s *Service) nextAllowedAtLocked(now time.Time) *time.Time {
	if s.lastBackupAt.IsZero() {
		return nil
	}
	next := s.lastBackupAt.Add(time.Duration(s.cooldownMinutes) * time.Minute)
	if now.Before(next) {
		return &next
	}
	return nil
}

func (s *Service) defaultCOSStatus() string {
	if s.cosEnabled() {
		return BackupCOSStatusNever
	}
	return BackupCOSStatusDisabled
}

func (r *BackupResult) hasLocalArtifacts() bool {
	return r.PumpkinFile != "" || r.CacheAFile != "" || r.CacheHKFile != ""
}

func normalizeCOSPrefix(prefix string) string {
	prefix = strings.TrimSpace(prefix)
	prefix = strings.Trim(prefix, "/")
	if prefix == "" {
		return ""
	}
	return prefix + "/"
}

func newBackupJobID(triggerType string) string {
	return fmt.Sprintf("%s-%s", triggerType, uuid.NewString())
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func formatTimePtr(ts time.Time) *string {
	if ts.IsZero() {
		return nil
	}
	formatted := ts.Local().Format("2006-01-02 15:04:05")
	return &formatted
}
