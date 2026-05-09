package factorlab

import (
	"reflect"
	"testing"
	"time"
)

func TestNormalizeWorkerConfigDefaults(t *testing.T) {
	cfg := normalizeWorkerConfig(WorkerConfig{Hour: -1, Minute: 99})
	if cfg.PythonBin != defaultPythonBin {
		t.Fatalf("expected default python bin, got %q", cfg.PythonBin)
	}
	if cfg.Phase1ScriptPath != defaultPhase1Script {
		t.Fatalf("expected default script, got %q", cfg.Phase1ScriptPath)
	}
	if cfg.Hour != defaultComputeHour || cfg.Minute != defaultComputeMinute {
		t.Fatalf("unexpected default schedule: %02d:%02d", cfg.Hour, cfg.Minute)
	}
	if cfg.Timeout != defaultComputeTimeout {
		t.Fatalf("unexpected timeout: %s", cfg.Timeout)
	}
	if cfg.ProgressInterval != defaultProgressInterval {
		t.Fatalf("unexpected progress interval: %d", cfg.ProgressInterval)
	}
}

func TestBuildPhase1CommandArgs(t *testing.T) {
	cfg := normalizeWorkerConfig(WorkerConfig{
		DBPath:           "data/pumpkin.db",
		Phase1ScriptPath: "quant/scripts/compute_factor_lab_phase1.py",
		ProgressInterval: 123,
	})
	got := buildPhase1CommandArgs(cfg)
	want := []string{
		"quant/scripts/compute_factor_lab_phase1.py",
		"--db", "data/pumpkin.db",
		"--write",
		"--progress-interval", "123",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected args\n got: %#v\nwant: %#v", got, want)
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
