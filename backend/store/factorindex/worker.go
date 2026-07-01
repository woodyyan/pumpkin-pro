package factorindex

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"
)

const (
	defaultRebalanceHour   = 20
	defaultRebalanceMinute = 10
	defaultDailyHour       = 20
	defaultDailyMinute     = 30
	defaultRunTimeout      = 2 * time.Hour
	defaultHistoryLimit    = 10
	workerLogPrefix        = "[factor-index-worker]"
)

var defaultRetryDelays = []time.Duration{15 * time.Minute, 30 * time.Minute, 60 * time.Minute}

type WorkerConfig struct {
	Enabled         bool
	RebalanceHour   int
	RebalanceMinute int
	DailyHour       int
	DailyMinute     int
	RunTimeout      time.Duration
	NowFunc         func() time.Time
}

type WorkerRunStatus struct {
	ID              string           `json:"id"`
	TriggerType     string           `json:"trigger_type"`
	Request         ManualRunRequest `json:"request"`
	Status          string           `json:"status"`
	StartedAt       time.Time        `json:"started_at"`
	FinishedAt      *time.Time       `json:"finished_at,omitempty"`
	DurationSeconds float64          `json:"duration_seconds"`
	ErrorMessage    string           `json:"error_message,omitempty"`
}

type WorkerStatus struct {
	Enabled           bool              `json:"enabled"`
	Running           bool              `json:"running"`
	RebalanceSchedule string            `json:"rebalance_schedule"`
	DailySchedule     string            `json:"daily_schedule"`
	NextRebalanceAt   time.Time         `json:"next_rebalance_at"`
	NextDailyAt       time.Time         `json:"next_daily_at"`
	LastRunAt         *time.Time        `json:"last_run_at,omitempty"`
	LastSuccessAt     *time.Time        `json:"last_success_at,omitempty"`
	LastError         string            `json:"last_error,omitempty"`
	Current           *WorkerRunStatus  `json:"current,omitempty"`
	History           []WorkerRunStatus `json:"history"`
}

type Worker struct {
	service       *Service
	cfg           WorkerConfig
	mu            sync.Mutex
	running       bool
	current       *WorkerRunStatus
	history       []WorkerRunStatus
	lastRunAt     *time.Time
	lastSuccessAt *time.Time
	lastError     string
}

func NewWorker(service *Service, cfg WorkerConfig) *Worker {
	return &Worker{service: service, cfg: normalizeWorkerConfig(cfg)}
}

func normalizeWorkerConfig(cfg WorkerConfig) WorkerConfig {
	if cfg.RebalanceHour < 0 || cfg.RebalanceHour > 23 {
		cfg.RebalanceHour = defaultRebalanceHour
	}
	if cfg.RebalanceMinute < 0 || cfg.RebalanceMinute > 59 {
		cfg.RebalanceMinute = defaultRebalanceMinute
	}
	if cfg.DailyHour < 0 || cfg.DailyHour > 23 {
		cfg.DailyHour = defaultDailyHour
	}
	if cfg.DailyMinute < 0 || cfg.DailyMinute > 59 {
		cfg.DailyMinute = defaultDailyMinute
	}
	if cfg.RunTimeout <= 0 {
		cfg.RunTimeout = defaultRunTimeout
	}
	if cfg.NowFunc == nil {
		cfg.NowFunc = time.Now
	}
	return cfg
}

func (w *Worker) Start(ctx context.Context) {
	if w == nil || !w.cfg.Enabled || w.service == nil {
		log.Printf("%s disabled", workerLogPrefix)
		return
	}
	go func() {
		if err := w.runWithRetry(ctx, "startup-catchup", ManualRunRequest{Operation: OperationSyncAll}, w.service.SyncAll); err != nil {
			log.Printf("%s startup catch-up failed: %v", workerLogPrefix, err)
		}
	}()
	go w.scheduleLoop(ctx, "scheduled-rebalance", w.cfg.RebalanceHour, w.cfg.RebalanceMinute, ManualRunRequest{Operation: OperationSyncRebalances}, w.service.SyncRebalances)
	go w.scheduleLoop(ctx, "scheduled-daily", w.cfg.DailyHour, w.cfg.DailyMinute, ManualRunRequest{Operation: OperationSyncAll}, w.service.SyncAll)
	log.Printf("%s started, rebalance at %02d:%02d CST, daily NAV at %02d:%02d CST", workerLogPrefix, w.cfg.RebalanceHour, w.cfg.RebalanceMinute, w.cfg.DailyHour, w.cfg.DailyMinute)
}

func (w *Worker) StartManual(ctx context.Context, request ManualRunRequest) (*WorkerRunStatus, error) {
	if w == nil || w.service == nil {
		return nil, fmt.Errorf("factor index worker is not initialized")
	}
	if !w.cfg.Enabled {
		return nil, fmt.Errorf("factor index worker is disabled")
	}
	normalized, err := normalizeManualRunRequest(request)
	if err != nil {
		return nil, err
	}
	run, ok := w.beginRun("manual", normalized)
	if !ok {
		return nil, fmt.Errorf("factor index worker is already running")
	}
	go func(req ManualRunRequest) {
		runCtx := context.Background()
		if ctx != nil {
			runCtx = context.WithoutCancel(ctx)
		}
		if err := w.executeRun(runCtx, func(execCtx context.Context) error {
			return w.service.RunManualRequest(execCtx, req)
		}, false); err != nil {
			log.Printf("%s manual run failed: %v", workerLogPrefix, err)
		}
	}(normalized)
	return run, nil
}

func (w *Worker) Snapshot(now time.Time) WorkerStatus {
	w.mu.Lock()
	defer w.mu.Unlock()
	status := WorkerStatus{
		Enabled:           w.cfg.Enabled,
		Running:           w.running,
		RebalanceSchedule: scheduleString(w.cfg.RebalanceHour, w.cfg.RebalanceMinute),
		DailySchedule:     scheduleString(w.cfg.DailyHour, w.cfg.DailyMinute),
		History:           cloneHistory(w.history),
	}
	if !now.IsZero() {
		status.NextRebalanceAt = nextDailyTriggerTime(now, w.cfg.RebalanceHour, w.cfg.RebalanceMinute)
		status.NextDailyAt = nextDailyTriggerTime(now, w.cfg.DailyHour, w.cfg.DailyMinute)
	}
	if w.current != nil {
		status.Current = cloneWorkerRunStatus(w.current)
	}
	if w.lastRunAt != nil {
		value := *w.lastRunAt
		status.LastRunAt = &value
	}
	if w.lastSuccessAt != nil {
		value := *w.lastSuccessAt
		status.LastSuccessAt = &value
	}
	status.LastError = w.lastError
	return status
}

func (w *Worker) scheduleLoop(ctx context.Context, triggerType string, hour, minute int, request ManualRunRequest, run func(context.Context) error) {
	for {
		now := w.cfg.NowFunc()
		next := nextDailyTriggerTime(now, hour, minute)
		wait := next.Sub(now)
		log.Printf("%s %s next trigger at %s (in %s)", workerLogPrefix, triggerType, next.Format(time.RFC3339), wait.Round(time.Second))
		select {
		case <-ctx.Done():
			return
		case <-time.After(wait):
			if err := w.runWithRetry(ctx, triggerType, request, run); err != nil {
				log.Printf("%s %s failed: %v", workerLogPrefix, triggerType, err)
			}
		}
	}
}

func (w *Worker) runWithRetry(ctx context.Context, triggerType string, request ManualRunRequest, run func(context.Context) error) error {
	runStatus, ok := w.beginRun(triggerType, request)
	if !ok {
		return fmt.Errorf("factor index worker is already running")
	}
	return w.executeRunWithRetry(ctx, runStatus, run)
}

func (w *Worker) executeRunWithRetry(ctx context.Context, run *WorkerRunStatus, runFn func(context.Context) error) error {
	attempts := append([]time.Duration{0}, defaultRetryDelays...)
	var lastErr error
	for attempt, delay := range attempts {
		if delay > 0 {
			select {
			case <-ctx.Done():
				lastErr = ctx.Err()
				w.failRun(lastErr)
				return lastErr
			case <-time.After(delay):
			}
		}
		runCtx, cancel := context.WithTimeout(ctx, w.cfg.RunTimeout)
		err := runFn(runCtx)
		cancel()
		if err == nil {
			w.completeRun()
			return nil
		}
		lastErr = err
		log.Printf("%s %s attempt %d/%d failed: %v", workerLogPrefix, run.TriggerType, attempt+1, len(attempts), err)
	}
	w.failRun(lastErr)
	return lastErr
}

func (w *Worker) executeRun(ctx context.Context, runFn func(context.Context) error, retry bool) error {
	if retry {
		current := w.currentSnapshot()
		if current == nil {
			return fmt.Errorf("factor index worker lost current run state")
		}
		return w.executeRunWithRetry(ctx, current, runFn)
	}
	runCtx, cancel := context.WithTimeout(ctx, w.cfg.RunTimeout)
	defer cancel()
	if err := runFn(runCtx); err != nil {
		w.failRun(err)
		return err
	}
	w.completeRun()
	return nil
}

func (w *Worker) beginRun(triggerType string, request ManualRunRequest) (*WorkerRunStatus, bool) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.running {
		return nil, false
	}
	now := w.cfg.NowFunc().UTC()
	run := &WorkerRunStatus{
		ID:          fmt.Sprintf("factorindex-%s", now.Format("20060102-150405.000")),
		TriggerType: triggerType,
		Request:     request,
		Status:      "running",
		StartedAt:   now,
	}
	w.running = true
	w.current = run
	return cloneWorkerRunStatus(run), true
}

func (w *Worker) currentSnapshot() *WorkerRunStatus {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.current == nil {
		return nil
	}
	return cloneWorkerRunStatus(w.current)
}

func (w *Worker) completeRun() {
	w.finalizeRun("completed", nil)
}

func (w *Worker) failRun(err error) {
	w.finalizeRun("failed", err)
}

func (w *Worker) finalizeRun(status string, err error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.current == nil {
		w.running = false
		return
	}
	finishedAt := w.cfg.NowFunc().UTC()
	w.current.Status = status
	w.current.FinishedAt = &finishedAt
	w.current.DurationSeconds = finishedAt.Sub(w.current.StartedAt).Seconds()
	if err != nil {
		w.current.ErrorMessage = err.Error()
		w.lastError = err.Error()
	} else {
		w.current.ErrorMessage = ""
		w.lastError = ""
		value := finishedAt
		w.lastSuccessAt = &value
	}
	w.lastRunAt = &finishedAt
	completed := cloneWorkerRunStatus(w.current)
	w.history = append([]WorkerRunStatus{*completed}, w.history...)
	if len(w.history) > defaultHistoryLimit {
		w.history = w.history[:defaultHistoryLimit]
	}
	w.current = nil
	w.running = false
}

func cloneWorkerRunStatus(run *WorkerRunStatus) *WorkerRunStatus {
	if run == nil {
		return nil
	}
	copyRun := *run
	if run.FinishedAt != nil {
		finishedAt := *run.FinishedAt
		copyRun.FinishedAt = &finishedAt
	}
	return &copyRun
}

func cloneHistory(history []WorkerRunStatus) []WorkerRunStatus {
	items := make([]WorkerRunStatus, 0, len(history))
	for _, item := range history {
		copyItem := item
		if item.FinishedAt != nil {
			finishedAt := *item.FinishedAt
			copyItem.FinishedAt = &finishedAt
		}
		items = append(items, copyItem)
	}
	return items
}

func scheduleString(hour, minute int) string {
	return fmt.Sprintf("%02d:%02d", hour, minute)
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
