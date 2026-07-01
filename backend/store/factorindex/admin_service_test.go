package factorindex

import (
	"context"
	"testing"
	"time"
)

func TestFactorIndexAdminStatusIncludesWorkerSnapshot(t *testing.T) {
	svc, repo := setupFactorIndexService(t)
	seedFactorIndexDataset(t, repo, false)
	if err := svc.SyncAll(context.Background()); err != nil {
		t.Fatalf("sync all: %v", err)
	}
	status, err := svc.AdminStatus(context.Background(), WorkerStatus{
		Enabled:           true,
		RebalanceSchedule: "20:10",
		DailySchedule:     "20:30",
	})
	if err != nil {
		t.Fatalf("admin status: %v", err)
	}
	if status.LatestSnapshotDate != "2026-06-01" {
		t.Fatalf("expected latest snapshot date 2026-06-01, got %s", status.LatestSnapshotDate)
	}
	if status.LatestTradeDate != "2026-06-03" {
		t.Fatalf("expected latest trade date 2026-06-03, got %s", status.LatestTradeDate)
	}
	if len(status.Items) != 7 {
		t.Fatalf("expected 7 admin items, got %d", len(status.Items))
	}
	if status.Worker.DailySchedule != "20:30" {
		t.Fatalf("expected worker daily schedule 20:30, got %s", status.Worker.DailySchedule)
	}
	first := status.Items[0]
	if first.FactorKey != "value" {
		t.Fatalf("expected first factor key value, got %s", first.FactorKey)
	}
	if first.LatestTradeDate != "2026-06-03" {
		t.Fatalf("expected value latest trade date 2026-06-03, got %s", first.LatestTradeDate)
	}
	if first.NAV == nil || *first.NAV <= defaultBaseNAV {
		t.Fatalf("expected nav above base, got %+v", first.NAV)
	}
}

func TestFactorIndexManualRunRequestRebuildsMissingDailyRange(t *testing.T) {
	svc, repo := setupFactorIndexService(t)
	seedFactorIndexDataset(t, repo, false)
	if err := svc.SyncAll(context.Background()); err != nil {
		t.Fatalf("sync all: %v", err)
	}
	valueID := defaultDefinitions[0].ID
	if err := repo.DeleteDailyRows(context.Background(), []string{valueID}, "2026-06-03", "2026-06-03"); err != nil {
		t.Fatalf("delete daily row: %v", err)
	}
	deleted, err := repo.GetDailyByTradeDate(context.Background(), valueID, "2026-06-03")
	if err != nil {
		t.Fatalf("load deleted daily row: %v", err)
	}
	if deleted != nil {
		t.Fatal("expected daily row to be deleted before manual run")
	}
	if err := svc.RunManualRequest(context.Background(), ManualRunRequest{
		Operation: OperationSyncDaily,
		FactorKey: "value",
		FromDate:  "2026-06-03",
		ToDate:    "2026-06-03",
	}); err != nil {
		t.Fatalf("manual run request: %v", err)
	}
	restored, err := repo.GetDailyByTradeDate(context.Background(), valueID, "2026-06-03")
	if err != nil {
		t.Fatalf("load restored daily row: %v", err)
	}
	if restored == nil {
		t.Fatal("expected daily row to be rebuilt")
	}
	if restored.Status != StatusCompleted {
		t.Fatalf("expected rebuilt row completed, got %s", restored.Status)
	}
}

func TestFactorIndexManualRunRequestRejectsScopedResetForFullRebuild(t *testing.T) {
	svc, _ := setupFactorIndexService(t)
	err := svc.RunManualRequest(context.Background(), ManualRunRequest{
		Operation: OperationSyncAll,
		FromDate:  "2026-06-01",
		Reset:     true,
	})
	if err == nil {
		t.Fatal("expected validation error for scoped reset full rebuild")
	}
}

func TestFactorIndexWorkerTracksManualRunHistory(t *testing.T) {
	svc, repo := setupFactorIndexService(t)
	seedFactorIndexDataset(t, repo, false)
	worker := NewWorker(svc, WorkerConfig{Enabled: true, RunTimeout: time.Minute})
	run, err := worker.StartManual(context.Background(), ManualRunRequest{Operation: OperationSyncAll})
	if err != nil {
		t.Fatalf("start manual run: %v", err)
	}
	if run.Status != "running" {
		t.Fatalf("expected running status, got %s", run.Status)
	}
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		snapshot := worker.Snapshot(time.Now())
		if !snapshot.Running {
			if len(snapshot.History) == 0 {
				t.Fatal("expected worker history after run completion")
			}
			if snapshot.History[0].Status != "completed" {
				t.Fatalf("expected completed history item, got %s", snapshot.History[0].Status)
			}
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("manual worker run did not finish in time")
}
