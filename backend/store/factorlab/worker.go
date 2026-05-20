package factorlab

import (
	"bytes"
	"context"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

const (
	defaultComputeHour       = 21
	defaultComputeMinute     = 0
	defaultComputeTimeout    = 3 * time.Hour
	defaultStepTimeout       = 30 * time.Minute
	defaultProgressInterval  = 500
	defaultPythonBin         = "python3"
	defaultPhase0Script      = "quant/scripts/update_factor_lab_phase0_incremental.py"
	defaultPhase1Script      = "quant/scripts/compute_factor_lab_phase1.py"
	defaultPhase2Script      = "quant/scripts/compute_factor_lab_phase2.py"
	factorLabWorkerLogPrefix = "[factorlab-worker]"
)

type WorkerConfig struct {
	Enabled          bool
	DBPath           string
	BackupDir        string
	PythonBin        string
	Phase0ScriptPath string
	Phase1ScriptPath string
	Phase2ScriptPath string
	Hour             int
	Minute           int
	Timeout          time.Duration
	StepTimeout      time.Duration
	ProgressInterval int
}

type Worker struct {
	cfg       WorkerConfig
	lastRunAt time.Time
	lastError string
	mu        sync.Mutex
	running   bool
}

func NewWorker(cfg WorkerConfig) *Worker {
	return &Worker{cfg: normalizeWorkerConfig(cfg)}
}

func normalizeWorkerConfig(cfg WorkerConfig) WorkerConfig {
	if strings.TrimSpace(cfg.PythonBin) == "" {
		cfg.PythonBin = defaultPythonBin
	}
	if strings.TrimSpace(cfg.Phase0ScriptPath) == "" {
		cfg.Phase0ScriptPath = defaultPhase0Script
	}
	if strings.TrimSpace(cfg.Phase1ScriptPath) == "" {
		cfg.Phase1ScriptPath = defaultPhase1Script
	}
	if strings.TrimSpace(cfg.Phase2ScriptPath) == "" {
		cfg.Phase2ScriptPath = defaultPhase2Script
	}
	if cfg.Hour < 0 || cfg.Hour > 23 {
		cfg.Hour = defaultComputeHour
	}
	if cfg.Minute < 0 || cfg.Minute > 59 {
		cfg.Minute = defaultComputeMinute
	}
	if cfg.Timeout <= 0 {
		cfg.Timeout = defaultComputeTimeout
	}
	if cfg.StepTimeout <= 0 {
		cfg.StepTimeout = defaultStepTimeout
	}
	if cfg.ProgressInterval <= 0 {
		cfg.ProgressInterval = defaultProgressInterval
	}
	if strings.TrimSpace(cfg.BackupDir) == "" && strings.TrimSpace(cfg.DBPath) != "" {
		cfg.BackupDir = filepath.Join(filepath.Dir(cfg.DBPath), "backups", "factorlab")
	}
	return cfg
}

func (w *Worker) Start(ctx context.Context) {
	if !w.cfg.Enabled {
		log.Printf("%s disabled", factorLabWorkerLogPrefix)
		return
	}
	go func() {
		for {
			now := time.Now()
			next := nextDailyTriggerTime(now, w.cfg.Hour, w.cfg.Minute)
			wait := next.Sub(now)
			log.Printf("%s next trigger at %s (in %s)", factorLabWorkerLogPrefix, next.Format(time.RFC3339), wait.Round(time.Second))
			select {
			case <-ctx.Done():
				return
			case <-time.After(wait):
				if err := w.RunOnce(ctx); err != nil {
					log.Printf("%s pipeline failed: %v", factorLabWorkerLogPrefix, err)
				}
			}
		}
	}()
	log.Printf("%s started, scheduled daily at %02d:%02d CST", factorLabWorkerLogPrefix, w.cfg.Hour, w.cfg.Minute)
}

func (w *Worker) RunOnce(ctx context.Context) error {
	if !w.cfg.Enabled {
		return nil
	}
	if !w.tryStartRun() {
		return fmt.Errorf("factor lab pipeline is already running")
	}
	defer w.finishRun()
	if strings.TrimSpace(w.cfg.DBPath) == "" {
		w.lastError = "DB path is empty"
		return fmt.Errorf("factor lab pipeline db path is empty")
	}
	ctx, cancel := context.WithTimeout(ctx, w.cfg.Timeout)
	defer cancel()

	if err := w.quickCheckDatabase(ctx, "before"); err != nil {
		w.lastError = err.Error()
		return err
	}
	backupPath, err := w.backupDatabase(ctx)
	if err != nil {
		w.lastError = err.Error()
		return err
	}
	log.Printf("%s safe backup created: %s", factorLabWorkerLogPrefix, backupPath)

	if err := w.runScript(ctx, "phase0-incremental", w.cfg.Phase0ScriptPath, buildPhase0CommandArgs(w.cfg)); err != nil {
		return err
	}
	if err := w.runScript(ctx, "phase1", w.cfg.Phase1ScriptPath, buildPhase1CommandArgs(w.cfg)); err != nil {
		return err
	}
	if err := w.runScript(ctx, "phase2", w.cfg.Phase2ScriptPath, buildPhase2CommandArgs(w.cfg)); err != nil {
		return err
	}
	if err := w.quickCheckDatabase(ctx, "after"); err != nil {
		w.lastError = err.Error()
		return err
	}
	w.lastRunAt = time.Now()
	w.lastError = ""
	return nil
}

func (w *Worker) tryStartRun() bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.running {
		return false
	}
	w.running = true
	return true
}

func (w *Worker) finishRun() {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.running = false
}

func (w *Worker) runScript(ctx context.Context, label, scriptPath string, args []string) error {
	log.Printf("%s running %s: %s %s", factorLabWorkerLogPrefix, label, w.cfg.PythonBin, strings.Join(args, " "))
	cmd := exec.CommandContext(ctx, w.cfg.PythonBin, args...)
	var output bytes.Buffer
	cmd.Stdout = &output
	cmd.Stderr = &output
	cmd.Dir = inferCommandDir(scriptPath)
	err := cmd.Run()
	trimmedOutput := strings.TrimSpace(output.String())
	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			err = fmt.Errorf("%s compute timeout after %s", label, w.cfg.Timeout)
		}
		w.lastError = err.Error()
		log.Printf("%s ❌ %s failed: %v\n%s", factorLabWorkerLogPrefix, label, err, trimmedOutput)
		return err
	}
	log.Printf("%s ✅ %s completed\n%s", factorLabWorkerLogPrefix, label, trimmedOutput)
	return nil
}

func (w *Worker) quickCheckDatabase(ctx context.Context, stage string) error {
	output, err := runSQLiteCommand(ctx, w.cfg.DBPath, "PRAGMA quick_check;")
	if err != nil {
		return fmt.Errorf("sqlite quick_check %s failed: %w (%s)", stage, err, strings.TrimSpace(output))
	}
	if strings.TrimSpace(output) != "ok" {
		return fmt.Errorf("sqlite quick_check %s failed: %s", stage, strings.TrimSpace(output))
	}
	log.Printf("%s quick_check %s ok", factorLabWorkerLogPrefix, stage)
	return nil
}

func (w *Worker) backupDatabase(ctx context.Context) (string, error) {
	if strings.TrimSpace(w.cfg.BackupDir) == "" {
		return "", fmt.Errorf("backup dir is empty")
	}
	if err := os.MkdirAll(w.cfg.BackupDir, 0o755); err != nil {
		return "", fmt.Errorf("create backup dir: %w", err)
	}
	destPath := filepath.Join(w.cfg.BackupDir, fmt.Sprintf("factorlab_pipeline_%s.db", time.Now().Format("20060102_150405")))
	_ = os.Remove(destPath)
	output, err := runSQLiteCommand(ctx, w.cfg.DBPath, fmt.Sprintf(".backup %s", destPath))
	if err != nil {
		_ = os.Remove(destPath)
		return "", fmt.Errorf("sqlite backup failed: %w (%s)", err, strings.TrimSpace(output))
	}
	return destPath, nil
}

func runSQLiteCommand(ctx context.Context, dbPath string, statement string) (string, error) {
	cmd := exec.CommandContext(ctx, "sqlite3", dbPath, statement)
	var output bytes.Buffer
	cmd.Stdout = &output
	cmd.Stderr = &output
	err := cmd.Run()
	return output.String(), err
}

func buildPhase0CommandArgs(cfg WorkerConfig) []string {
	args := []string{
		cfg.Phase0ScriptPath,
		"--db", cfg.DBPath,
		"--write",
		"--progress-interval", fmt.Sprintf("%d", cfg.ProgressInterval),
		"--step-timeout-seconds", fmt.Sprintf("%d", int(cfg.StepTimeout.Seconds())),
	}
	return args
}

func buildPhase1CommandArgs(cfg WorkerConfig) []string {
	args := []string{
		cfg.Phase1ScriptPath,
		"--db", cfg.DBPath,
		"--write",
		"--progress-interval", fmt.Sprintf("%d", cfg.ProgressInterval),
	}
	return args
}

func buildPhase2CommandArgs(cfg WorkerConfig) []string {
	args := []string{
		cfg.Phase2ScriptPath,
		"--db", cfg.DBPath,
		"--write",
		"--progress-interval", fmt.Sprintf("%d", cfg.ProgressInterval),
	}
	return args
}

func inferCommandDir(scriptPath string) string {
	abs, err := filepath.Abs(scriptPath)
	if err != nil {
		return "."
	}
	root := filepath.Dir(filepath.Dir(filepath.Dir(abs)))
	return root
}

func nextDailyTriggerTime(now time.Time, hour, minute int) time.Time {
	loc, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		loc = time.FixedZone("CST", 8*3600)
	}
	nowCST := now.In(loc)
	today := time.Date(nowCST.Year(), nowCST.Month(), nowCST.Day(), hour, minute, 0, 0, loc)
	if nowCST.After(today) || nowCST.Equal(today) {
		return today.Add(24 * time.Hour)
	}
	return today
}

func (w *Worker) Status() (lastRunAt time.Time, lastError string) {
	return w.lastRunAt, w.lastError
}
