package factorlab

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
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
	defaultLogTailLimit      = 120
	defaultPythonBin         = "python3"
	defaultPhase0Script      = "quant/scripts/update_factor_lab_phase0_incremental.py"
	defaultPhase1Script      = "quant/scripts/compute_factor_lab_phase1.py"
	defaultPhase2Script      = "quant/scripts/compute_factor_lab_phase2.py"
	factorLabWorkerLogPrefix = "[factorlab-worker]"
)

type WorkerConfig struct {
	Enabled              bool
	DBPath               string
	BackupDir            string
	PythonBin            string
	Phase0ScriptPath     string
	Phase1ScriptPath     string
	Phase2ScriptPath     string
	DailyBarsSource      string
	FinancialsSource     string
	DividendsSource      string
	Hour                 int
	Minute               int
	Timeout              time.Duration
	StepTimeout          time.Duration
	ProgressInterval     int
	ItemProgressInterval int
}

type PipelineRunRequest struct {
	Phase      string `json:"phase"`
	Phase0Mode string `json:"phase0_mode"`
	Scope      string `json:"scope"`
}

type PhaseStatus struct {
	Name            string         `json:"name"`
	Status          string         `json:"status"`
	StartedAt       *time.Time     `json:"started_at,omitempty"`
	FinishedAt      *time.Time     `json:"finished_at,omitempty"`
	DurationSeconds float64        `json:"duration_seconds"`
	TotalCount      int            `json:"total_count"`
	SuccessCount    int            `json:"success_count"`
	FailedCount     int            `json:"failed_count"`
	SkippedCount    int            `json:"skipped_count"`
	LastMessage     string         `json:"last_message,omitempty"`
	ErrorMessage    string         `json:"error_message,omitempty"`
	Summary         map[string]any `json:"summary,omitempty"`
	LogTail         []string       `json:"log_tail"`
}

type PipelineRunStatus struct {
	ID              string             `json:"id"`
	TriggerType     string             `json:"trigger_type"`
	Request         PipelineRunRequest `json:"request"`
	Status          string             `json:"status"`
	CurrentPhase    string             `json:"current_phase"`
	StartedAt       time.Time          `json:"started_at"`
	FinishedAt      *time.Time         `json:"finished_at,omitempty"`
	DurationSeconds float64            `json:"duration_seconds"`
	DBHealthBefore  string             `json:"db_health_before"`
	DBHealthAfter   string             `json:"db_health_after"`
	BackupPath      string             `json:"backup_path,omitempty"`
	ErrorMessage    string             `json:"error_message,omitempty"`
	Phases          []PhaseStatus      `json:"phases"`
}

type WorkerStatus struct {
	Enabled   bool                `json:"enabled"`
	Running   bool                `json:"running"`
	Schedule  string              `json:"schedule"`
	NextRunAt time.Time           `json:"next_run_at"`
	LastRunAt *time.Time          `json:"last_run_at,omitempty"`
	LastError string              `json:"last_error,omitempty"`
	Current   *PipelineRunStatus  `json:"current,omitempty"`
	History   []PipelineRunStatus `json:"history"`
}

type Worker struct {
	cfg       WorkerConfig
	lastRunAt time.Time
	lastError string
	mu        sync.Mutex
	running   bool
	current   *PipelineRunStatus
	history   []PipelineRunStatus
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
	if strings.TrimSpace(cfg.DailyBarsSource) == "" {
		cfg.DailyBarsSource = "tencent"
	}
	if strings.TrimSpace(cfg.FinancialsSource) == "" {
		cfg.FinancialsSource = "auto"
	}
	if strings.TrimSpace(cfg.DividendsSource) == "" {
		cfg.DividendsSource = "auto"
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
	if cfg.ItemProgressInterval <= 0 {
		cfg.ItemProgressInterval = 1
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
	return w.runPipeline(ctx, "scheduled", PipelineRunRequest{Phase: "all", Phase0Mode: "all", Scope: "incremental"})
}

func (w *Worker) StartManual(ctx context.Context, request PipelineRunRequest) (*PipelineRunStatus, error) {
	if !w.cfg.Enabled {
		return nil, fmt.Errorf("factor lab pipeline disabled")
	}
	request = normalizePipelineRunRequest(request)
	if err := validatePipelineRunRequest(request); err != nil {
		return nil, err
	}
	run, ok := w.beginRun("manual", request)
	if !ok {
		return nil, fmt.Errorf("factor lab pipeline is already running")
	}
	go w.executePipeline(context.Background(), run)
	return run, nil
}

func (w *Worker) runPipeline(ctx context.Context, triggerType string, request PipelineRunRequest) error {
	if !w.cfg.Enabled {
		return nil
	}
	request = normalizePipelineRunRequest(request)
	if err := validatePipelineRunRequest(request); err != nil {
		return err
	}
	run, ok := w.beginRun(triggerType, request)
	if !ok {
		return fmt.Errorf("factor lab pipeline is already running")
	}
	return w.executePipeline(ctx, run)
}

func normalizePipelineRunRequest(request PipelineRunRequest) PipelineRunRequest {
	request.Phase = strings.TrimSpace(request.Phase)
	request.Phase0Mode = strings.TrimSpace(request.Phase0Mode)
	request.Scope = strings.TrimSpace(request.Scope)
	if request.Phase == "" {
		request.Phase = "all"
	}
	if request.Phase0Mode == "" {
		request.Phase0Mode = "all"
	}
	if request.Scope == "" {
		request.Scope = "incremental"
	}
	return request
}

func validatePipelineRunRequest(request PipelineRunRequest) error {
	if !containsString([]string{"all", "phase0", "phase1", "phase2", "phase1_phase2"}, request.Phase) {
		return fmt.Errorf("unsupported factor lab phase: %s", request.Phase)
	}
	if !containsString([]string{"all", "securities", "industries", "daily-bars", "index-bars", "financials", "dividends"}, request.Phase0Mode) {
		return fmt.Errorf("unsupported phase0 mode: %s", request.Phase0Mode)
	}
	if !containsString([]string{"incremental", "repair_missing_dividend_yield", "repair_missing_fcfm_inputs"}, request.Scope) {
		return fmt.Errorf("unsupported factor lab scope: %s", request.Scope)
	}
	if request.Scope == "repair_missing_dividend_yield" && (request.Phase0Mode != "dividends" || !containsString([]string{"all", "phase0"}, request.Phase)) {
		return fmt.Errorf("repair_missing_dividend_yield requires phase=all|phase0 and phase0_mode=dividends")
	}
	if request.Scope == "repair_missing_fcfm_inputs" && (request.Phase0Mode != "financials" || !containsString([]string{"all", "phase0"}, request.Phase)) {
		return fmt.Errorf("repair_missing_fcfm_inputs requires phase=all|phase0 and phase0_mode=financials")
	}
	return nil
}

func containsString(items []string, target string) bool {
	for _, item := range items {
		if item == target {
			return true
		}
	}
	return false
}

func (w *Worker) beginRun(triggerType string, request PipelineRunRequest) (*PipelineRunStatus, bool) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.running {
		return nil, false
	}
	now := time.Now()
	run := &PipelineRunStatus{
		ID:           fmt.Sprintf("factorlab-%s", now.Format("20060102-150405.000")),
		TriggerType:  triggerType,
		Request:      request,
		Status:       "running",
		StartedAt:    now,
		CurrentPhase: "pending",
		Phases:       initialPhaseStatuses(request),
	}
	w.running = true
	w.current = run
	return clonePipelineRunStatus(run), true
}

func initialPhaseStatuses(request PipelineRunRequest) []PhaseStatus {
	phases := []PhaseStatus{}
	if request.Phase == "all" || request.Phase == "phase0" {
		phases = append(phases, PhaseStatus{Name: "phase0_incremental", Status: "pending"})
	}
	if request.Phase == "all" || request.Phase == "phase1" || request.Phase == "phase1_phase2" {
		phases = append(phases, PhaseStatus{Name: "phase1", Status: "pending"})
	}
	if request.Phase == "all" || request.Phase == "phase2" || request.Phase == "phase1_phase2" {
		phases = append(phases, PhaseStatus{Name: "phase2", Status: "pending"})
	}
	return phases
}

func (w *Worker) executePipeline(ctx context.Context, runSnapshot *PipelineRunStatus) error {
	defer w.finishRun()
	if strings.TrimSpace(w.cfg.DBPath) == "" {
		err := fmt.Errorf("factor lab pipeline db path is empty")
		w.failRun(err)
		return err
	}
	ctx, cancel := context.WithTimeout(ctx, w.cfg.Timeout)
	defer cancel()

	if err := w.quickCheckDatabase(ctx, "before"); err != nil {
		w.failRun(err)
		return err
	}
	w.updateRun(func(run *PipelineRunStatus) { run.DBHealthBefore = "ok" })
	backupPath, err := w.backupDatabase(ctx)
	if err != nil {
		w.failRun(err)
		return err
	}
	w.updateRun(func(run *PipelineRunStatus) { run.BackupPath = backupPath })
	log.Printf("%s safe backup created: %s", factorLabWorkerLogPrefix, backupPath)

	steps := w.buildPipelineSteps(w.currentRequest())
	for _, step := range steps {
		if err := w.runScript(ctx, step.name, step.scriptPath, step.args); err != nil {
			w.failRun(err)
			return err
		}
	}
	if err := w.quickCheckDatabase(ctx, "after"); err != nil {
		w.failRun(err)
		return err
	}
	w.updateRun(func(run *PipelineRunStatus) { run.DBHealthAfter = "ok" })
	w.completeRun()
	return nil
}

type pipelineStep struct {
	name       string
	scriptPath string
	args       []string
}

func (w *Worker) currentRequest() PipelineRunRequest {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.current == nil {
		return PipelineRunRequest{Phase: "all", Phase0Mode: "all", Scope: "incremental"}
	}
	return w.current.Request
}

func (w *Worker) buildPipelineSteps(request PipelineRunRequest) []pipelineStep {
	steps := []pipelineStep{}
	if request.Phase == "all" || request.Phase == "phase0" {
		steps = append(steps, pipelineStep{name: "phase0_incremental", scriptPath: w.cfg.Phase0ScriptPath, args: buildPhase0CommandArgs(w.cfg, request)})
	}
	if request.Phase == "all" || request.Phase == "phase1" || request.Phase == "phase1_phase2" {
		steps = append(steps, pipelineStep{name: "phase1", scriptPath: w.cfg.Phase1ScriptPath, args: buildPhase1CommandArgs(w.cfg)})
	}
	if request.Phase == "all" || request.Phase == "phase2" || request.Phase == "phase1_phase2" {
		steps = append(steps, pipelineStep{name: "phase2", scriptPath: w.cfg.Phase2ScriptPath, args: buildPhase2CommandArgs(w.cfg)})
	}
	return steps
}

func (w *Worker) finishRun() {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.running = false
}

func (w *Worker) runScript(ctx context.Context, label, scriptPath string, args []string) error {
	started := time.Now()
	w.updatePhase(label, func(phase *PhaseStatus) {
		phase.Status = "running"
		phase.StartedAt = &started
		phase.LogTail = []string{}
		phase.LastMessage = ""
		phase.ErrorMessage = ""
		phase.Summary = nil
	})
	w.updateRun(func(run *PipelineRunStatus) { run.CurrentPhase = label })
	log.Printf("%s running %s: %s %s", factorLabWorkerLogPrefix, label, w.cfg.PythonBin, strings.Join(args, " "))
	cmd := exec.CommandContext(ctx, w.cfg.PythonBin, args...)
	cmd.Dir = inferCommandDir(scriptPath)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return err
	}
	if err := cmd.Start(); err != nil {
		return err
	}
	var wg sync.WaitGroup
	wg.Add(2)
	go w.captureScriptOutput(label, "stdout", stdout, &wg)
	go w.captureScriptOutput(label, "stderr", stderr, &wg)
	err = cmd.Wait()
	wg.Wait()
	finished := time.Now()
	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			err = fmt.Errorf("%s compute timeout after %s", label, w.cfg.Timeout)
		}
		w.updatePhase(label, func(phase *PhaseStatus) {
			phase.Status = "failed"
			phase.FinishedAt = &finished
			phase.DurationSeconds = finished.Sub(started).Seconds()
			phase.ErrorMessage = err.Error()
		})
		log.Printf("%s ❌ %s failed: %v", factorLabWorkerLogPrefix, label, err)
		return err
	}
	w.updatePhase(label, func(phase *PhaseStatus) {
		phase.Status = "success"
		phase.FinishedAt = &finished
		phase.DurationSeconds = finished.Sub(started).Seconds()
	})
	log.Printf("%s ✅ %s completed", factorLabWorkerLogPrefix, label)
	return nil
}

func (w *Worker) captureScriptOutput(label, stream string, reader io.Reader, wg *sync.WaitGroup) {
	defer wg.Done()
	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, 1024), 1024*1024)
	for scanner.Scan() {
		line := strings.TrimRight(scanner.Text(), "\r\n")
		if strings.TrimSpace(line) == "" {
			continue
		}
		w.appendPhaseLogLine(label, stream, line)
	}
	if err := scanner.Err(); err != nil {
		w.appendPhaseLogLine(label, stream, "log scan error: "+err.Error())
	}
}

func (w *Worker) appendPhaseLogLine(label, stream, line string) {
	log.Printf("%s %s %s | %s", factorLabWorkerLogPrefix, label, stream, line)
	w.updatePhase(label, func(phase *PhaseStatus) {
		entry := fmt.Sprintf("%s %s", time.Now().Format("15:04:05"), line)
		phase.LogTail = append(phase.LogTail, entry)
		if len(phase.LogTail) > defaultLogTailLimit {
			phase.LogTail = phase.LogTail[len(phase.LogTail)-defaultLogTailLimit:]
		}
		phase.LastMessage = line
		applyPhaseLogSummary(phase, line)
	})
}

func applyPhaseLogSummary(phase *PhaseStatus, line string) {
	if idx := strings.Index(line, "summary="); idx >= 0 {
		raw := line[idx+len("summary="):]
		if end := strings.LastIndex(raw, " status="); end >= 0 {
			raw = raw[:end]
		}
		var summary map[string]any
		if err := json.Unmarshal([]byte(strings.TrimSpace(raw)), &summary); err == nil {
			phase.Summary = summary
			phase.TotalCount = intFromSummary(summary, "total")
			phase.SuccessCount = intFromSummary(summary, "success")
			phase.FailedCount = intFromSummary(summary, "failed")
			phase.SkippedCount = intFromSummary(summary, "skipped")
		}
	}
	if strings.Contains(line, "failed:") || strings.Contains(line, "失败") {
		phase.ErrorMessage = line
	}
}

func intFromSummary(summary map[string]any, key string) int {
	value, ok := summary[key]
	if !ok || value == nil {
		return 0
	}
	switch typed := value.(type) {
	case float64:
		return int(typed)
	case int:
		return typed
	case json.Number:
		parsed, _ := typed.Int64()
		return int(parsed)
	default:
		return 0
	}
}

func (w *Worker) updateRun(mutator func(*PipelineRunStatus)) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.current == nil {
		return
	}
	mutator(w.current)
}

func (w *Worker) updatePhase(name string, mutator func(*PhaseStatus)) {
	w.updateRun(func(run *PipelineRunStatus) {
		for idx := range run.Phases {
			if run.Phases[idx].Name == name {
				mutator(&run.Phases[idx])
				return
			}
		}
	})
}

func (w *Worker) failRun(err error) {
	finished := time.Now()
	w.mu.Lock()
	defer w.mu.Unlock()
	w.lastError = err.Error()
	if w.current != nil {
		w.current.Status = "failed"
		w.current.ErrorMessage = err.Error()
		w.current.FinishedAt = &finished
		w.current.DurationSeconds = finished.Sub(w.current.StartedAt).Seconds()
		w.appendHistoryLocked(*w.current)
	}
}

func (w *Worker) completeRun() {
	finished := time.Now()
	w.mu.Lock()
	defer w.mu.Unlock()
	w.lastRunAt = finished
	w.lastError = ""
	if w.current != nil {
		w.current.Status = "success"
		w.current.CurrentPhase = ""
		w.current.FinishedAt = &finished
		w.current.DurationSeconds = finished.Sub(w.current.StartedAt).Seconds()
		w.appendHistoryLocked(*w.current)
	}
}

func (w *Worker) appendHistoryLocked(run PipelineRunStatus) {
	w.history = append([]PipelineRunStatus{*clonePipelineRunStatus(&run)}, w.history...)
	if len(w.history) > 10 {
		w.history = w.history[:10]
	}
}

func clonePipelineRunStatus(run *PipelineRunStatus) *PipelineRunStatus {
	if run == nil {
		return nil
	}
	copyRun := *run
	copyRun.Phases = make([]PhaseStatus, 0, len(run.Phases))
	for _, phase := range run.Phases {
		copyRun.Phases = append(copyRun.Phases, clonePhaseStatus(phase))
	}
	return &copyRun
}

func clonePhaseStatus(phase PhaseStatus) PhaseStatus {
	copyPhase := phase
	copyPhase.LogTail = append([]string(nil), phase.LogTail...)
	if phase.Summary != nil {
		copyPhase.Summary = make(map[string]any, len(phase.Summary))
		for key, value := range phase.Summary {
			copyPhase.Summary[key] = value
		}
	}
	return copyPhase
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

func buildPhase0CommandArgs(cfg WorkerConfig, request PipelineRunRequest) []string {
	args := []string{
		cfg.Phase0ScriptPath,
		"--db", cfg.DBPath,
		"--write",
		"--modes", request.Phase0Mode,
		"--scope", request.Scope,
		"--progress-interval", fmt.Sprintf("%d", cfg.ProgressInterval),
		"--item-progress-interval", fmt.Sprintf("%d", cfg.ItemProgressInterval),
		"--daily-bars-source", cfg.DailyBarsSource,
		"--financials-source", cfg.FinancialsSource,
		"--dividends-source", cfg.DividendsSource,
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

func (w *Worker) Snapshot(now time.Time) WorkerStatus {
	w.mu.Lock()
	defer w.mu.Unlock()
	var lastRunAt *time.Time
	if !w.lastRunAt.IsZero() {
		value := w.lastRunAt
		lastRunAt = &value
	}
	history := make([]PipelineRunStatus, 0, len(w.history))
	for _, item := range w.history {
		history = append(history, *clonePipelineRunStatus(&item))
	}
	return WorkerStatus{
		Enabled:   w.cfg.Enabled,
		Running:   w.running,
		Schedule:  fmt.Sprintf("%02d:%02d", w.cfg.Hour, w.cfg.Minute),
		NextRunAt: nextDailyTriggerTime(now, w.cfg.Hour, w.cfg.Minute),
		LastRunAt: lastRunAt,
		LastError: w.lastError,
		Current:   clonePipelineRunStatus(w.current),
		History:   history,
	}
}

func (w *Worker) Status() (lastRunAt time.Time, lastError string) {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.lastRunAt, w.lastError
}
