package factorlab

import (
	"reflect"
	"testing"
	"time"
)

func TestNormalizeWorkerConfigDefaults(t *testing.T) {
	cfg := normalizeWorkerConfig(WorkerConfig{DBPath: "data/pumpkin.db", Hour: -1, Minute: 99})
	if cfg.PythonBin != defaultPythonBin {
		t.Fatalf("expected default python bin, got %q", cfg.PythonBin)
	}
	if cfg.Phase0ScriptPath != defaultPhase0Script {
		t.Fatalf("expected default phase0 script, got %q", cfg.Phase0ScriptPath)
	}
	if cfg.Phase1ScriptPath != defaultPhase1Script {
		t.Fatalf("expected default phase1 script, got %q", cfg.Phase1ScriptPath)
	}
	if cfg.Phase2ScriptPath != defaultPhase2Script {
		t.Fatalf("expected default phase2 script, got %q", cfg.Phase2ScriptPath)
	}
	if cfg.Hour != defaultComputeHour || cfg.Minute != defaultComputeMinute {
		t.Fatalf("unexpected default schedule: %02d:%02d", cfg.Hour, cfg.Minute)
	}
	if cfg.Timeout != defaultComputeTimeout {
		t.Fatalf("unexpected timeout: %s", cfg.Timeout)
	}
	if cfg.StepTimeout != defaultStepTimeout {
		t.Fatalf("unexpected step timeout: %s", cfg.StepTimeout)
	}
	if cfg.BackupDir != "data/backups/factorlab" {
		t.Fatalf("unexpected backup dir: %s", cfg.BackupDir)
	}
	if cfg.ProgressInterval != defaultProgressInterval {
		t.Fatalf("unexpected progress interval: %d", cfg.ProgressInterval)
	}
}

func TestBuildPhaseCommandArgs(t *testing.T) {
	cfg := normalizeWorkerConfig(WorkerConfig{
		DBPath:           "data/pumpkin.db",
		Phase0ScriptPath: "quant/scripts/update_factor_lab_phase0_incremental.py",
		Phase1ScriptPath: "quant/scripts/compute_factor_lab_phase1.py",
		Phase2ScriptPath: "quant/scripts/compute_factor_lab_phase2.py",
		StepTimeout:      10 * time.Minute,
		ProgressInterval: 123,
	})
	phase0 := buildPhase0CommandArgs(cfg)
	wantPhase0 := []string{
		"quant/scripts/update_factor_lab_phase0_incremental.py",
		"--db", "data/pumpkin.db",
		"--write",
		"--progress-interval", "123",
		"--step-timeout-seconds", "600",
	}
	if !reflect.DeepEqual(phase0, wantPhase0) {
		t.Fatalf("unexpected phase0 args\n got: %#v\nwant: %#v", phase0, wantPhase0)
	}
	phase1 := buildPhase1CommandArgs(cfg)
	wantPhase1 := []string{
		"quant/scripts/compute_factor_lab_phase1.py",
		"--db", "data/pumpkin.db",
		"--write",
		"--progress-interval", "123",
	}
	if !reflect.DeepEqual(phase1, wantPhase1) {
		t.Fatalf("unexpected phase1 args\n got: %#v\nwant: %#v", phase1, wantPhase1)
	}
	phase2 := buildPhase2CommandArgs(cfg)
	wantPhase2 := []string{
		"quant/scripts/compute_factor_lab_phase2.py",
		"--db", "data/pumpkin.db",
		"--write",
		"--progress-interval", "123",
	}
	if !reflect.DeepEqual(phase2, wantPhase2) {
		t.Fatalf("unexpected phase2 args\n got: %#v\nwant: %#v", phase2, wantPhase2)
	}
}

func TestNextDailyTriggerTime(t *testing.T) {
	loc := time.FixedZone("CST", 8*3600)
	before := time.Date(2026, 5, 9, 19, 0, 0, 0, loc)
	next := nextDailyTriggerTime(before, 20, 30)
	if next.Day() != 9 || next.Hour() != 20 || next.Minute() != 30 {
		t.Fatalf("expected same day 20:30, got %s", next)
	}
	after := time.Date(2026, 5, 9, 21, 0, 0, 0, loc)
	next = nextDailyTriggerTime(after, 20, 30)
	if next.Day() != 10 || next.Hour() != 20 || next.Minute() != 30 {
		t.Fatalf("expected next day 20:30, got %s", next)
	}
}

func TestRunOnceDisabledSkips(t *testing.T) {
	worker := NewWorker(WorkerConfig{Enabled: false})
	if err := worker.RunOnce(nil); err != nil {
		t.Fatalf("disabled worker should skip without error: %v", err)
	}
}

func TestBeginRunPreventsConcurrentRuns(t *testing.T) {
	worker := NewWorker(WorkerConfig{Enabled: true})
	if run, ok := worker.beginRun("manual"); !ok || run.TriggerType != "manual" {
		t.Fatalf("first run should start, got run=%+v ok=%v", run, ok)
	}
	if _, ok := worker.beginRun("manual"); ok {
		t.Fatal("second run should be rejected")
	}
	worker.finishRun()
	if _, ok := worker.beginRun("manual"); !ok {
		t.Fatal("run should start after finish")
	}
}
