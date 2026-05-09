package factorlab

import (
	"bytes"
	"context"
	"fmt"
	"log"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

const (
	defaultComputeHour       = 20
	defaultComputeMinute     = 30
	defaultComputeTimeout    = 60 * time.Minute
	defaultProgressInterval  = 500
	defaultPythonBin         = "python3"
	defaultPhase1Script      = "quant/scripts/compute_factor_lab_phase1.py"
	factorLabWorkerLogPrefix = "[factorlab-worker]"
)

type WorkerConfig struct {
	Enabled          bool
	DBPath           string
	PythonBin        string
	Phase1ScriptPath string
	Hour             int
	Minute           int
	Timeout          time.Duration
	ProgressInterval int
}

type Worker struct {
	cfg       WorkerConfig
	lastRunAt time.Time
	lastError string
}

func NewWorker(cfg WorkerConfig) *Worker {
	return &Worker{cfg: normalizeWorkerConfig(cfg)}
}

func normalizeWorkerConfig(cfg WorkerConfig) WorkerConfig {
	if strings.TrimSpace(cfg.PythonBin) == "" {
		cfg.PythonBin = defaultPythonBin
	}
	if strings.TrimSpace(cfg.Phase1ScriptPath) == "" {
		cfg.Phase1ScriptPath = defaultPhase1Script
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
	if cfg.ProgressInterval <= 0 {
		cfg.ProgressInterval = defaultProgressInterval
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
				w.RunOnce(ctx)
			}
		}
	}()
	log.Printf("%s started, scheduled daily at %02d:%02d CST", factorLabWorkerLogPrefix, w.cfg.Hour, w.cfg.Minute)
}

func (w *Worker) RunOnce(ctx context.Context) error {
	if !w.cfg.Enabled {
		return nil
	}
	if strings.TrimSpace(w.cfg.DBPath) == "" {
		w.lastError = "DB path is empty"
		return fmt.Errorf("factor lab daily compute db path is empty")
	}
	ctx, cancel := context.WithTimeout(ctx, w.cfg.Timeout)
	defer cancel()

	args := buildPhase1CommandArgs(w.cfg)
	log.Printf("%s running: %s %s", factorLabWorkerLogPrefix, w.cfg.PythonBin, strings.Join(args, " "))
	cmd := exec.CommandContext(ctx, w.cfg.PythonBin, args...)
	var output bytes.Buffer
	cmd.Stdout = &output
	cmd.Stderr = &output
	cmd.Dir = inferCommandDir(w.cfg.Phase1ScriptPath)
	err := cmd.Run()
	trimmedOutput := strings.TrimSpace(output.String())
	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			err = fmt.Errorf("phase1 compute timeout after %s", w.cfg.Timeout)
		}
		w.lastError = err.Error()
		log.Printf("%s ❌ failed: %v\n%s", factorLabWorkerLogPrefix, err, trimmedOutput)
		return err
	}
	w.lastRunAt = time.Now()
	w.lastError = ""
	log.Printf("%s ✅ completed\n%s", factorLabWorkerLogPrefix, trimmedOutput)
	return nil
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
