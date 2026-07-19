package portfolio

import (
	"context"
	"testing"
	"time"
)

func TestPortfolioWorkerRunOnceExecutesScopedSnapshot(t *testing.T) {
	svc, ctx := setupPortfolioService(t)
	seedPortfolioProfile(t, svc, "600519.SH", "SSE", "贵州茅台")
	svc.historyReader = &stubHistoryReader{bars: map[string][]DailyBarRecord{
		"600519": buildStubBars("600519", map[string]float64{
			"2026-04-20": 10,
			"2026-04-21": 11,
		}),
	}}
	if _, _, err := svc.CreateEvent(ctx, "worker-user", "600519.SH", CreatePortfolioEventInput{
		EventType: EventTypeBuy,
		TradeDate: "2026-04-20",
		Quantity:  100,
		Price:     10,
	}); err != nil {
		t.Fatalf("CreateEvent failed: %v", err)
	}
	worker := NewWorker(svc, WorkerConfig{Enabled: true, RunTimeout: time.Minute, TriggerSource: PortfolioSnapshotJobTriggerScheduler})
	jobRun, err := worker.RunOnce(context.Background(), PortfolioScopeAShare, "2026-04-21", time.Date(2026, 4, 21, 16, 0, 0, 0, time.UTC), PortfolioSnapshotJobTriggerScheduler)
	if err != nil {
		t.Fatalf("RunOnce failed: %v", err)
	}
	if jobRun.Scope != PortfolioScopeAShare || jobRun.TargetDate != "2026-04-21" {
		t.Fatalf("unexpected job run: %+v", jobRun)
	}
	if jobRun.SnapshotCountWritten != 1 || jobRun.UserCountFailed != 0 {
		t.Fatalf("unexpected job run summary: %+v", jobRun)
	}
	status := worker.Snapshot(time.Date(2026, 4, 21, 10, 0, 0, 0, time.UTC))
	if !status.Enabled || len(status.LastRuns) != 1 {
		t.Fatalf("unexpected worker status: %+v", status)
	}
	if status.LastRuns[0].JobRunID != jobRun.ID || status.LastRuns[0].Status != jobRun.Status {
		t.Fatalf("expected snapshot to expose last run, got %+v", status.LastRuns[0])
	}
}

func TestPortfolioWorkerStartupCatchUpRunsMissedTradingDay(t *testing.T) {
	svc, ctx := setupPortfolioService(t)
	seedPortfolioProfile(t, svc, "600519.SH", "SSE", "贵州茅台")
	svc.historyReader = &stubHistoryReader{bars: map[string][]DailyBarRecord{
		"600519": buildStubBars("600519", map[string]float64{"2026-07-17": 11}),
	}}
	if _, _, err := svc.CreateEvent(ctx, "catch-up-user", "600519.SH", CreatePortfolioEventInput{
		EventType: EventTypeBuy, TradeDate: "2026-07-16", Quantity: 100, Price: 10,
	}); err != nil {
		t.Fatalf("CreateEvent failed: %v", err)
	}
	now := time.Date(2026, 7, 17, 18, 0, 0, 0, shanghaiLocation())
	worker := NewWorker(svc, WorkerConfig{Enabled: true, AShareHour: 16, NowFunc: func() time.Time { return now }, RunTimeout: time.Minute})
	worker.runStartupCatchUp(context.Background(), PortfolioScopeAShare, 16, 0)

	hasRun, err := svc.repo.HasCompletedSnapshotJobRun(ctx, PortfolioScopeAShare, "2026-07-17")
	if err != nil {
		t.Fatalf("HasCompletedSnapshotJobRun failed: %v", err)
	}
	if !hasRun {
		t.Fatal("expected startup catch-up to create completed job run")
	}
	hasSnapshot, err := svc.repo.HasDailySnapshot(ctx, "catch-up-user", PortfolioScopeAShare, "2026-07-17")
	if err != nil || !hasSnapshot {
		t.Fatalf("expected startup catch-up snapshot, hasSnapshot=%v err=%v", hasSnapshot, err)
	}
}

func TestPortfolioWorkerStartupCatchUpSkipsBeforeScheduleAndExistingRun(t *testing.T) {
	svc, ctx := setupPortfolioService(t)
	beforeSchedule := time.Date(2026, 7, 17, 15, 59, 0, 0, shanghaiLocation())
	worker := NewWorker(svc, WorkerConfig{Enabled: true, NowFunc: func() time.Time { return beforeSchedule }})
	worker.runStartupCatchUp(context.Background(), PortfolioScopeAShare, 16, 0)
	hasRun, err := svc.repo.HasCompletedSnapshotJobRun(ctx, PortfolioScopeAShare, "2026-07-17")
	if err != nil || hasRun {
		t.Fatalf("expected no catch-up before schedule, hasRun=%v err=%v", hasRun, err)
	}

	now := time.Now().UTC()
	if err := svc.repo.CreateSnapshotJobRun(ctx, &PortfolioSnapshotJobRunRecord{
		ID: "existing-run", JobType: PortfolioSnapshotJobTypeDailyMarket, Scope: PortfolioScopeAShare, TargetDate: "2026-07-17",
		ScheduledTime: now, StartedAt: now, Status: PortfolioSnapshotJobStatusSuccess, TriggerSource: PortfolioSnapshotJobTriggerScheduler,
		CreatedAt: now, UpdatedAt: now,
	}); err != nil {
		t.Fatalf("CreateSnapshotJobRun failed: %v", err)
	}
	afterSchedule := time.Date(2026, 7, 17, 18, 0, 0, 0, shanghaiLocation())
	worker.cfg.NowFunc = func() time.Time { return afterSchedule }
	worker.runStartupCatchUp(context.Background(), PortfolioScopeAShare, 16, 0)
	var count int64
	if err := svc.repo.db.Model(&PortfolioSnapshotJobRunRecord{}).Where("scope = ? AND target_date = ?", PortfolioScopeAShare, "2026-07-17").Count(&count).Error; err != nil {
		t.Fatalf("count job runs failed: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected existing run to prevent duplicate catch-up, got %d runs", count)
	}
}

func TestPortfolioWorkerRunOnceRejectsConcurrentSameScope(t *testing.T) {
	worker := NewWorker(nil, WorkerConfig{Enabled: true})
	worker.running[PortfolioScopeAShare] = true
	if _, err := worker.RunOnce(context.Background(), PortfolioScopeAShare, "2026-04-21", time.Time{}, PortfolioSnapshotJobTriggerManual); err == nil {
		t.Fatal("expected concurrent run to be rejected")
	}
}
