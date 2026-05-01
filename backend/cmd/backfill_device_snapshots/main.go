package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/woodyyan/pumpkin-pro/backend/config"
	"github.com/woodyyan/pumpkin-pro/backend/store"
	"github.com/woodyyan/pumpkin-pro/backend/store/analytics"
	"gorm.io/gorm"
)

func main() {
	var dbPath string
	var days int
	var dryRun bool
	flag.StringVar(&dbPath, "db", "", "Database file path (required)")
	flag.IntVar(&days, "days", 30, "Number of days to backfill")
	flag.BoolVar(&dryRun, "dry-run", false, "Show what would be done without writing")
	flag.Parse()

	if dbPath == "" {
		fmt.Fprintln(os.Stderr, "Usage: backfill_device_snapshots -db <path> [-days 30] [-dry-run]")
		os.Exit(1)
	}

	if _, err := os.Stat(dbPath); os.IsNotExist(err) {
		log.Fatalf("Database file not found: %s", dbPath)
	}

	cfg := config.DBConfig{Type: "sqlite", Path: dbPath}
	s, err := store.New(cfg)
	if err != nil {
		log.Fatalf("Failed to open database: %v", err)
	}

	repo := analytics.NewRepository(s.DB)
	backfill := &Backfill{db: s.DB, repo: repo, dryRun: dryRun}

	since := time.Now().UTC().AddDate(0, 0, -days)
	log.Printf("Backfilling device_snapshots from %s (last %d days)", since.Format("2006-01-02"), days)

	pageViewCount, err := backfill.backfillPageViews(since)
	if err != nil {
		log.Fatalf("Backfill page_views failed: %v", err)
	}

	authCount, err := backfill.backfillAuthLogs(since)
	if err != nil {
		log.Fatalf("Backfill auth_audit_logs failed: %v", err)
	}

	log.Printf("Backfill complete: %d page_views, %d auth_audit_logs", pageViewCount, authCount)
}

type Backfill struct {
	db     *gorm.DB
	repo   *analytics.Repository
	dryRun bool
}

func (b *Backfill) backfillPageViews(since time.Time) (int, error) {
	var records []struct {
		ID        string
		UserID    string
		VisitorID string
		UserAgent string
		CreatedAt time.Time
	}

	if err := b.db.Model(&analytics.PageViewRecord{}).
		Select("id, user_id, visitor_id, user_agent, created_at").
		Where("created_at >= ?", since).
		Scan(&records).Error; err != nil {
		return 0, err
	}

	var count int
	for _, r := range records {
		if b.dryRun {
			count++
			continue
		}
		info := analytics.ParseUserAgent(r.UserAgent)
		snap := &analytics.DeviceSnapshot{
			UserID:         r.UserID,
			VisitorID:      r.VisitorID,
			Source:         "page_view",
			SourceID:       r.ID,
			DeviceType:     info.DeviceType,
			OSFamily:       info.OSFamily,
			OSVersion:      info.OSVersion,
			BrowserFamily:  info.BrowserFamily,
			BrowserVersion: info.BrowserVersion,
			RawUserAgent:   r.UserAgent,
			CreatedAt:      r.CreatedAt,
		}
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		if err := b.repo.InsertDeviceSnapshot(ctx, snap); err != nil {
			log.Printf("[warn] skip duplicate or error: source=%s source_id=%s err=%v", snap.Source, snap.SourceID, err)
		}
		cancel()
		count++
	}
	return count, nil
}

func (b *Backfill) backfillAuthLogs(since time.Time) (int, error) {
	var records []struct {
		ID        string
		UserID    string
		UserAgent string
		CreatedAt time.Time
	}

	if err := b.db.Table("auth_audit_logs").
		Select("id, user_id, user_agent, created_at").
		Where("created_at >= ? AND action = ? AND success = ?", since, "login", true).
		Scan(&records).Error; err != nil {
		return 0, err
	}

	var count int
	for _, r := range records {
		if b.dryRun {
			count++
			continue
		}
		info := analytics.ParseUserAgent(r.UserAgent)
		snap := &analytics.DeviceSnapshot{
			UserID:         r.UserID,
			VisitorID:      "",
			Source:         "auth",
			SourceID:       r.ID,
			DeviceType:     info.DeviceType,
			OSFamily:       info.OSFamily,
			OSVersion:      info.OSVersion,
			BrowserFamily:  info.BrowserFamily,
			BrowserVersion: info.BrowserVersion,
			RawUserAgent:   r.UserAgent,
			CreatedAt:      r.CreatedAt,
		}
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		if err := b.repo.InsertDeviceSnapshot(ctx, snap); err != nil {
			log.Printf("[warn] skip duplicate or error: source=%s source_id=%s err=%v", snap.Source, snap.SourceID, err)
		}
		cancel()
		count++
	}
	return count, nil
}
