package factorlab

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"syscall"
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
	if cfg.IndustriesSource != "auto" || cfg.DailyBarsSource != "tencent" || cfg.FinancialsSource != "auto" || cfg.DividendsSource != "auto" {
		t.Fatalf("unexpected sources: industries=%s daily=%s financials=%s dividends=%s", cfg.IndustriesSource, cfg.DailyBarsSource, cfg.FinancialsSource, cfg.DividendsSource)
	}
	if !reflect.DeepEqual(cfg.DailyModes, []string{"securities", "industries", "daily-bars", "index-bars", "financials"}) {
		t.Fatalf("unexpected daily modes: %#v", cfg.DailyModes)
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
		"--industries-source", "auto",
		"--daily-bars-source", "tencent",
		"--financials-source", "auto",
		"--dividends-source", "auto",
		"--step-timeout-seconds", "600",
	}
	if !reflect.DeepEqual(phase0, wantPhase0) {
		t.Fatalf("unexpected phase0 args\n got: %#v\nwant: %#v", phase0, wantPhase0)
	}
	phase0FCFM := buildPhase0CommandArgs(cfg, PipelineRunRequest{Phase: "phase0", Phase0Mode: "financials", Scope: "repair_missing_fcfm_inputs"})
	wantPhase0FCFM := []string{
		"quant/scripts/update_factor_lab_phase0_incremental.py",
		"--db", "data/pumpkin.db",
		"--write",
		"--modes", "financials",
		"--scope", "repair_missing_fcfm_inputs",
		"--progress-interval", "123",
		"--item-progress-interval", "2",
		"--industries-source", "auto",
		"--daily-bars-source", "tencent",
		"--financials-source", "auto",
		"--dividends-source", "auto",
		"--step-timeout-seconds", "600",
	}
	if !reflect.DeepEqual(phase0FCFM, wantPhase0FCFM) {
		t.Fatalf("unexpected fcfm repair phase0 args\n got: %#v\nwant: %#v", phase0FCFM, wantPhase0FCFM)
	}
	phase0Industries := buildPhase0CommandArgs(cfg, PipelineRunRequest{Phase: "phase0", Phase0Mode: "industries", Scope: "incremental"})
	if got := phase0Industries[5]; got != "industries" {
		t.Fatalf("expected industries phase0 mode, got %q", got)
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
		t.Fatal("expected invalid dividend repair request")
	}
	if err := validatePipelineRunRequest(PipelineRunRequest{Phase: "phase0", Phase0Mode: "dividends", Scope: "repair_missing_dividend_yield"}); err != nil {
		t.Fatalf("expected valid dividend repair request: %v", err)
	}
	if err := validatePipelineRunRequest(PipelineRunRequest{Phase: "phase1", Phase0Mode: "financials", Scope: "repair_missing_fcfm_inputs"}); err == nil {
		t.Fatal("expected invalid fcfm repair request")
	}
	if err := validatePipelineRunRequest(PipelineRunRequest{Phase: "phase0", Phase0Mode: "financials", Scope: "repair_missing_fcfm_inputs"}); err != nil {
		t.Fatalf("expected valid fcfm repair request: %v", err)
	}
	if err := validatePipelineRunRequest(PipelineRunRequest{Phase: "phase0", Phase0Mode: "industries", Scope: "incremental"}); err != nil {
		t.Fatalf("expected valid industries request: %v", err)
	}
}

func TestValidatePipelineRunRequestSupportsFullRefreshDividendsAndCSVPhase0Modes(t *testing.T) {
	if err := validatePipelineRunRequest(PipelineRunRequest{Phase: "phase0", Phase0Mode: "dividends", Scope: "full_refresh_dividends"}); err != nil {
		t.Fatalf("expected full_refresh_dividends request valid: %v", err)
	}
	if err := validatePipelineRunRequest(PipelineRunRequest{Phase: "phase0", Phase0Mode: "securities,industries", Scope: "incremental"}); err != nil {
		t.Fatalf("expected csv phase0 modes valid: %v", err)
	}
	if err := validatePipelineRunRequest(PipelineRunRequest{Phase: "phase0", Phase0Mode: "industries,unknown", Scope: "incremental"}); err == nil {
		t.Fatal("expected invalid csv phase0 modes request")
	}
}

func TestPipelineStatusHelpers(t *testing.T) {
	if got := phaseStatusFromSummary(&PhaseStatus{Summary: map[string]any{"status": "partial"}}); got != "partial" {
		t.Fatalf("expected partial phase status, got %q", got)
	}
	if got := phaseStatusFromSummary(&PhaseStatus{Summary: map[string]any{"status": "failed"}}); got != "failed" {
		t.Fatalf("expected failed phase status, got %q", got)
	}
	if got := pipelineStatusFromPhases([]PhaseStatus{{Status: "success"}, {Status: "partial"}}); got != "partial" {
		t.Fatalf("expected partial pipeline status, got %q", got)
	}
	if got := pipelineStatusFromPhases([]PhaseStatus{{Status: "success"}, {Status: "failed"}}); got != "failed" {
		t.Fatalf("expected failed pipeline status, got %q", got)
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

func newScriptTestWorker(t *testing.T, python string) *Worker {
	t.Helper()
	worker := NewWorker(WorkerConfig{Enabled: true, PythonBin: python, Timeout: 2 * time.Second})
	if _, ok := worker.beginRun("manual", PipelineRunRequest{Phase: "phase0"}); !ok {
		t.Fatal("测试阶段未能启动")
	}
	return worker
}

func scriptTestPython(t *testing.T) string {
	t.Helper()
	if python := os.Getenv("FACTORLAB_TEST_PYTHON"); python != "" {
		path, err := exec.LookPath(python)
		if err != nil {
			t.Fatalf("测试 Python 不可用: %v", err)
		}
		return path
	}
	python, err := exec.LookPath("python3")
	if err != nil {
		t.Fatalf("测试需要 Python 3，可设置 FACTORLAB_TEST_PYTHON: %v", err)
	}
	return python
}

func scriptTestPath(t *testing.T) string {
	t.Helper()
	// 使用 -c 执行测试代码，只需让推断出的工作目录指向临时目录。
	return filepath.Join(t.TempDir(), "quant", "scripts", "worker_test.py")
}

func TestScriptOutputWriterDrainsTruncatedLines(t *testing.T) {
	worker := newScriptTestWorker(t, defaultPythonBin)
	output := &scriptOutputWriter{worker: worker, label: "phase0_incremental", stream: "stdout"}
	chunks := []string{
		"\n \r\nfirst\r",
		"\n" + strings.Repeat("x", defaultLogLineLimit-2),
		strings.Repeat("y", 2*defaultLogLineLimit) + "\nsecond\nsumm",
		`ary={"total":2,"success":1,"failed":0,"skipped":1,"status":"partial"}`,
	}
	for _, chunk := range chunks {
		if n, err := output.Write([]byte(chunk)); err != nil || n != len(chunk) {
			t.Fatalf("未完整消费输出: n=%d err=%v", n, err)
		}
		if output.used > defaultLogLineLimit {
			t.Fatalf("单行缓存超出上限: %d", output.used)
		}
	}
	output.flush()
	output.flush()
	phase := worker.Snapshot(time.Now()).Current.Phases[0]
	if len(phase.LogTail) != 4 || !strings.HasSuffix(phase.LogTail[0], "first") || !strings.HasSuffix(phase.LogTail[2], "second") {
		t.Fatalf("分块、空行或 CRLF 处理错误: 日志数=%d", len(phase.LogTail))
	}
	if line := phase.LogTail[1][len("15:04:05 "):]; len(line) != defaultLogLineLimit || !strings.HasSuffix(line, " [truncated]") {
		t.Fatalf("超长行未按上限截断: 长度=%d", len(line))
	}
	if phase.TotalCount != 2 || phase.SuccessCount != 1 || phase.SkippedCount != 1 || !strings.HasPrefix(phase.LastMessage, "summary=") {
		t.Fatalf("尾部无换行 summary 丢失: %+v", phase.Summary)
	}
}

func TestRunScriptDrainsOutputAndFinalSummary(t *testing.T) {
	t.Setenv("FACTORLAB_MANAGED_PROCESS_GROUP", "0")
	t.Setenv("FACTORLAB_PARENT_PID", "-1")
	python := scriptTestPython(t)
	for _, stream := range []string{"stdout", "stderr"} {
		t.Run(stream, func(t *testing.T) {
			worker := newScriptTestWorker(t, python)
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			code := `import os, sys
assert os.environ["FACTORLAB_MANAGED_PROCESS_GROUP"] == "1"
assert int(os.environ["FACTORLAB_PARENT_PID"]) == int(sys.argv[2])
output = getattr(sys, sys.argv[1])
output.write("x" * (4 * 1024 * 1024) + "\n")
for index in range(150):
    output.write("progress %d\n" % index)
output.write('summary={"total":5,"success":3,"failed":1,"skipped":1,"status":"partial"}')
output.flush()
`
			if err := worker.runScript(ctx, "phase0_incremental", scriptTestPath(t), []string{"-c", code, stream, fmt.Sprint(os.Getpid())}); err != nil {
				t.Fatalf("脚本输出未能正常排空: %v", err)
			}
			phase := worker.Snapshot(time.Now()).Current.Phases[0]
			if phase.Status != "partial" || phase.TotalCount != 5 || phase.SuccessCount != 3 || phase.FailedCount != 1 || phase.SkippedCount != 1 {
				t.Fatalf("末尾 summary 未解析: 状态=%s summary=%+v", phase.Status, phase.Summary)
			}
			if len(phase.LogTail) != defaultLogTailLimit || !strings.HasSuffix(phase.LogTail[len(phase.LogTail)-1], phase.LastMessage) || !strings.HasPrefix(phase.LastMessage, "summary=") {
				t.Fatalf("日志 tail 末尾 summary 丢失: 日志数=%d 最后消息=%q", len(phase.LogTail), phase.LastMessage)
			}
			if phase.FinishedAt == nil || phase.DurationSeconds <= 0 || phase.ErrorMessage != "" {
				t.Fatalf("阶段结束状态错误: %+v", phase)
			}
		})
	}
}

func TestRunScriptDrainsUnterminatedLongOutput(t *testing.T) {
	worker := newScriptTestWorker(t, scriptTestPython(t))
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	code := `import sys
sys.stdout.write("x" * (4 * 1024 * 1024))
sys.stdout.flush()
sys.stderr.write("y" * (4 * 1024 * 1024))
sys.stderr.flush()
`
	if err := worker.runScript(ctx, "phase0_incremental", scriptTestPath(t), []string{"-c", code}); err != nil {
		t.Fatalf("无换行超长输出阻塞: %v", err)
	}
	phase := worker.Snapshot(time.Now()).Current.Phases[0]
	if phase.Status != "success" || len(phase.LogTail) != 2 {
		t.Fatalf("无换行输出未完整收尾: 状态=%s 日志数=%d", phase.Status, len(phase.LogTail))
	}
	for _, line := range phase.LogTail {
		if len(line) != len("15:04:05 ")+defaultLogLineLimit || !strings.HasSuffix(line, " [truncated]") {
			t.Fatalf("无换行输出未有界截断: 长度=%d", len(line))
		}
	}
}

func TestRunScriptStartFailure(t *testing.T) {
	worker := newScriptTestWorker(t, filepath.Join(t.TempDir(), "missing-python"))
	err := worker.runScript(context.Background(), "phase0_incremental", scriptTestPath(t), nil)
	if !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("启动错误未返回: %v", err)
	}
	phase := worker.Snapshot(time.Now()).Current.Phases[0]
	if phase.Status != "failed" || phase.StartedAt == nil || phase.FinishedAt == nil || phase.ErrorMessage != err.Error() {
		t.Fatalf("启动失败仍被记录为运行中: %+v", phase)
	}
}

const scriptTestProcessTree = `import os, subprocess, sys, time
child_code = r'''import json, os, subprocess, sys, time
grandchild = subprocess.Popen([sys.executable, "-c", "import time; time.sleep(60)"])
with open(sys.argv[1] + ".tmp", "w") as output:
    json.dump({"parent": os.getppid(), "child": os.getpid(), "grandchild": grandchild.pid,
               "group": os.getpgrp(), "grandchild_group": os.getpgid(grandchild.pid),
               "managed": os.environ.get("FACTORLAB_MANAGED_PROCESS_GROUP"),
               "backend": os.environ.get("FACTORLAB_PARENT_PID")}, output)
os.replace(sys.argv[1] + ".tmp", sys.argv[1])
time.sleep(60)
'''
options = {}
if sys.argv[2] == "normal":
    options = {"stdout": subprocess.DEVNULL, "stderr": subprocess.DEVNULL}
subprocess.Popen([sys.executable, "-c", child_code, sys.argv[1]], **options)
while not os.path.exists(sys.argv[1]):
    time.sleep(0.01)
print('summary={"total":1,"success":1,"status":"success"}', flush=True)
if sys.argv[2] == "running":
    time.sleep(60)
`

type scriptTestProcesses struct {
	Parent          int    `json:"parent"`
	Child           int    `json:"child"`
	Grandchild      int    `json:"grandchild"`
	Group           int    `json:"group"`
	GrandchildGroup int    `json:"grandchild_group"`
	Managed         string `json:"managed"`
	Backend         string `json:"backend"`
}

func waitScriptTestProcesses(t *testing.T, path string) scriptTestProcesses {
	t.Helper()
	timer := time.NewTimer(5 * time.Second)
	defer timer.Stop()
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	for {
		var processes scriptTestProcesses
		if data, err := os.ReadFile(path); err == nil && json.Unmarshal(data, &processes) == nil && processes.Grandchild > 0 {
			// 只清理本测试创建的独立进程组，防止断言失败留下睡眠进程。
			if processes.Group != processes.Parent || processes.Group == syscall.Getpgrp() {
				t.Fatalf("脚本未使用独立进程组: %+v", processes)
			}
			t.Cleanup(func() { _ = syscall.Kill(-processes.Group, syscall.SIGKILL) })
			if processes.GrandchildGroup != processes.Group || processes.Managed != "1" || processes.Backend != fmt.Sprint(os.Getpid()) {
				t.Fatalf("后代未继承进程组或托管环境: %+v", processes)
			}
			return processes
		}
		select {
		case <-timer.C:
			t.Fatal("Python 进程树未能就绪")
		case <-ticker.C:
		}
	}
}

func assertScriptTestProcessesDead(t *testing.T, processes scriptTestProcesses) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for _, pid := range []int{processes.Parent, processes.Child, processes.Grandchild} {
		for {
			err := syscall.Kill(pid, 0)
			if errors.Is(err, syscall.ESRCH) {
				break
			}
			if err != nil {
				t.Fatalf("检查进程 %d 失败: %v", pid, err)
			}
			// Linux 上孤儿可能成为僵尸：已死亡但尚未被 init 回收。
			if runtime.GOOS == "linux" {
				state, err := os.ReadFile(fmt.Sprintf("/proc/%d/stat", pid))
				if errors.Is(err, os.ErrNotExist) {
					break
				}
				if err != nil {
					t.Fatalf("检查进程 %d 状态失败: %v", pid, err)
				}
				if end := strings.LastIndex(string(state), ") "); end >= 0 && strings.HasPrefix(string(state)[end+2:], "Z") {
					break
				}
			}
			if time.Now().After(deadline) {
				t.Fatalf("脚本后代仍存活: pid=%d", pid)
			}
			time.Sleep(10 * time.Millisecond)
		}
	}
}

func TestRunScriptKillsProcessTree(t *testing.T) {
	if runtime.GOOS != "linux" && runtime.GOOS != "darwin" {
		t.Skip("进程树测试针对 Linux/macOS")
	}
	python := scriptTestPython(t)
	for _, mode := range []string{"timeout", "cancel", "normal", "inherited-pipes"} {
		t.Run(mode, func(t *testing.T) {
			worker := newScriptTestWorker(t, python)
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			if mode == "timeout" {
				cancel()
				ctx, cancel = context.WithTimeout(context.Background(), worker.cfg.Timeout)
			}
			defer cancel()
			pidPath := filepath.Join(t.TempDir(), "processes.json")
			pythonMode := mode
			if mode == "timeout" || mode == "cancel" {
				pythonMode = "running"
			}
			result := make(chan error, 1)
			path := scriptTestPath(t)
			started := time.Now()
			go func() {
				result <- worker.runScript(ctx, "phase0_incremental", path, []string{"-c", scriptTestProcessTree, pidPath, pythonMode})
			}()
			processes := waitScriptTestProcesses(t, pidPath)
			if mode == "cancel" {
				cancel()
			}
			var err error
			select {
			case err = <-result:
			case <-time.After(7 * time.Second):
				t.Fatal("脚本退出未受 WaitDelay 限制")
			}
			phase := worker.Snapshot(time.Now()).Current.Phases[0]
			switch mode {
			case "normal":
				if err != nil || phase.Status != "success" {
					t.Fatalf("正常退出失败: err=%v 状态=%s", err, phase.Status)
				}
			case "inherited-pipes":
				if !errors.Is(err, exec.ErrWaitDelay) || time.Since(started) > 5*time.Second {
					t.Fatalf("继承输出管道未有界关闭: err=%v 用时=%s", err, time.Since(started))
				}
				if phase.Summary == nil || !strings.HasPrefix(phase.LastMessage, "summary=") {
					t.Fatalf("管道收尾丢失 summary: %+v", phase.Summary)
				}
			case "timeout", "cancel":
				if err == nil || phase.Status != "failed" || phase.FinishedAt == nil || phase.ErrorMessage != err.Error() {
					t.Fatalf("取消/超时未记录失败: err=%v 状态=%s", err, phase.Status)
				}
				if mode == "timeout" && (ctx.Err() != context.DeadlineExceeded || !strings.Contains(err.Error(), "compute timeout after 2s")) {
					t.Fatalf("超时错误不正确: %v", err)
				}
			}
			assertScriptTestProcessesDead(t, processes)
		})
	}
}
