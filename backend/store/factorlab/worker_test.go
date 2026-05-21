package factorlab

import (
	"reflect"
	"strings"
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
	if cfg.DailyBarsSource != "tencent" || cfg.FinancialsSource != "auto" || cfg.DividendsSource != "auto" {
		t.Fatalf("unexpected sources: daily=%s financials=%s dividends=%s", cfg.DailyBarsSource, cfg.FinancialsSource, cfg.DividendsSource)
	}
	if cfg.ItemProgressInterval != 1 {
		t.Fatalf("unexpected item progress interval: %d", cfg.ItemProgressInterval)
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
		DBPath:               "data/pumpkin.db",
		Phase0ScriptPath:     "quant/scripts/update_factor_lab_phase0_incremental.py",
		Phase1ScriptPath:     "quant/scripts/compute_factor_lab_phase1.py",
		Phase2ScriptPath:     "quant/scripts/compute_factor_lab_phase2.py",
		StepTimeout:          10 * time.Minute,
		ProgressInterval:     123,
		ItemProgressInterval: 2,
	})
	phase0 := buildPhase0CommandArgs(cfg, PipelineRunRequest{Phase: "phase0", Phase0Mode: "dividends", Scope: "repair_missing_dividend_yield"})
	wantPhase0 := []string{
		"quant/scripts/update_factor_lab_phase0_incremental.py",
		"--db", "data/pumpkin.db",
		"--write",
		"--modes", "dividends",
		"--scope", "repair_missing_dividend_yield",
		"--progress-interval", "123",
		"--item-progress-interval", "2",
		"--daily-bars-source", "tencent",
		"--financials-source", "auto",
		"--dividends-source", "auto",
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

func TestValidatePipelineRunRequestRejectsInvalidRepairScope(t *testing.T) {
	if err := validatePipelineRunRequest(PipelineRunRequest{Phase: "phase1", Phase0Mode: "dividends", Scope: "repair_missing_dividend_yield"}); err == nil {
		t.Fatal("expected invalid repair request")
	}
	if err := validatePipelineRunRequest(PipelineRunRequest{Phase: "phase0", Phase0Mode: "dividends", Scope: "repair_missing_dividend_yield"}); err != nil {
		t.Fatalf("expected valid dividend repair request: %v", err)
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
	request := PipelineRunRequest{Phase: "phase0", Phase0Mode: "dividends", Scope: "repair_missing_dividend_yield"}
	if run, ok := worker.beginRun("manual", request); !ok || run.TriggerType != "manual" || run.Request.Scope != "repair_missing_dividend_yield" {
		t.Fatalf("first run should start, got run=%+v ok=%v", run, ok)
	}
	if _, ok := worker.beginRun("manual", request); ok {
		t.Fatal("second run should be rejected")
	}
	worker.finishRun()
	if _, ok := worker.beginRun("manual", request); !ok {
		t.Fatal("run should start after finish")
	}
}

func TestAppendPhaseLogLineCapturesTailAndSummary(t *testing.T) {
	worker := NewWorker(WorkerConfig{Enabled: true})
	if _, ok := worker.beginRun("manual", PipelineRunRequest{Phase: "all", Phase0Mode: "all", Scope: "incremental"}); !ok {
		t.Fatal("run should start")
	}
	worker.appendPhaseLogLine("phase0_incremental", "stdout", "[12:00:00] daily-bars: 写入进度: 100/4338")
	worker.appendPhaseLogLine("phase0_incremental", "stdout", `summary={"total":5,"success":3,"failed":1,"skipped":1} status=partial`)
	snapshot := worker.Snapshot(time.Now())
	phase := snapshot.Current.Phases[0]
	if phase.TotalCount != 5 || phase.SuccessCount != 3 || phase.FailedCount != 1 || phase.SkippedCount != 1 {
		t.Fatalf("unexpected counts: %+v", phase)
	}
	if len(phase.LogTail) != 2 || !strings.Contains(phase.LogTail[1], "summary=") {
		t.Fatalf("unexpected log tail: %+v", phase.LogTail)
	}
}

func TestAppendPhaseLogLineLimitsTail(t *testing.T) {
	worker := NewWorker(WorkerConfig{Enabled: true})
	if _, ok := worker.beginRun("manual", PipelineRunRequest{Phase: "all", Phase0Mode: "all", Scope: "incremental"}); !ok {
		t.Fatal("run should start")
	}
	for idx := 0; idx < defaultLogTailLimit+5; idx++ {
		worker.appendPhaseLogLine("phase0_incremental", "stdout", strings.Repeat("x", 1)+string(rune('a'+idx%26)))
	}
	snapshot := worker.Snapshot(time.Now())
	if len(snapshot.Current.Phases[0].LogTail) != defaultLogTailLimit {
		t.Fatalf("expected tail limit %d, got %d", defaultLogTailLimit, len(snapshot.Current.Phases[0].LogTail))
	}
}
