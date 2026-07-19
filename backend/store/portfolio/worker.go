package portfolio

import (
	"context"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/woodyyan/pumpkin-pro/backend/store/quadrant"
)

const (
	defaultAShareSnapshotHour   = 16
	defaultAShareSnapshotMinute = 0
	defaultHKSnapshotHour       = 17
	defaultHKSnapshotMinute     = 0
	defaultSnapshotRunTimeout   = 2 * time.Hour
	portfolioWorkerLogPrefix    = "[portfolio-worker]"
)

type WorkerConfig struct {
	Enabled       bool
	AShareHour    int
	AShareMinute  int
	HKHour        int
	HKMinute      int
	RunTimeout    time.Duration
	NowFunc       func() time.Time
	TriggerSource string
}

type WorkerRun struct {
	Scope         string     `json:"scope"`
	TargetDate    string     `json:"target_date"`
	Status        string     `json:"status"`
	StartedAt     time.Time  `json:"started_at"`
	FinishedAt    *time.Time `json:"finished_at,omitempty"`
	JobRunID      string     `json:"job_run_id,omitempty"`
	ErrorMessage  string     `json:"error_message,omitempty"`
	TriggerSource string     `json:"trigger_source"`
}

type WorkerStatus struct {
	Enabled   bool        `json:"enabled"`
	Running   bool        `json:"running"`
	Schedules []string    `json:"schedules"`
	NextRunAt []time.Time `json:"next_run_at"`
	LastRuns  []WorkerRun `json:"last_runs"`
}

type Worker struct {
	service  *Service
	cfg      WorkerConfig
	mu       sync.Mutex
	running  map[string]bool
	lastRuns map[string]WorkerRun
}

func NewWorker(service *Service, cfg WorkerConfig) *Worker {
	return &Worker{
		service:  service,
		cfg:      normalizeWorkerConfig(cfg),
		running:  map[string]bool{},
		lastRuns: map[string]WorkerRun{},
	}
}

func normalizeWorkerConfig(cfg WorkerConfig) WorkerConfig {
	if cfg.AShareHour < 0 || cfg.AShareHour > 23 {
		cfg.AShareHour = defaultAShareSnapshotHour
	}
	if cfg.AShareMinute < 0 || cfg.AShareMinute > 59 {
		cfg.AShareMinute = defaultAShareSnapshotMinute
	}
	if cfg.HKHour < 0 || cfg.HKHour > 23 {
		cfg.HKHour = defaultHKSnapshotHour
	}
	if cfg.HKMinute < 0 || cfg.HKMinute > 59 {
		cfg.HKMinute = defaultHKSnapshotMinute
	}
	if cfg.RunTimeout <= 0 {
		cfg.RunTimeout = defaultSnapshotRunTimeout
	}
	if cfg.NowFunc == nil {
		cfg.NowFunc = time.Now
	}
	if strings.TrimSpace(cfg.TriggerSource) == "" {
		cfg.TriggerSource = PortfolioSnapshotJobTriggerScheduler
	}
	return cfg
}

func (w *Worker) Start(ctx context.Context) {
	if !w.cfg.Enabled || w.service == nil {
		log.Printf("%s disabled", portfolioWorkerLogPrefix)
		return
	}
	go w.runStartupCatchUp(ctx, PortfolioScopeAShare, w.cfg.AShareHour, w.cfg.AShareMinute)
	go w.runStartupCatchUp(ctx, PortfolioScopeHK, w.cfg.HKHour, w.cfg.HKMinute)
	go w.scheduleLoop(ctx, PortfolioScopeAShare, w.cfg.AShareHour, w.cfg.AShareMinute)
	go w.scheduleLoop(ctx, PortfolioScopeHK, w.cfg.HKHour, w.cfg.HKMinute)
	log.Printf("%s started, A-share at %02d:%02d CST, HK at %02d:%02d CST", portfolioWorkerLogPrefix, w.cfg.AShareHour, w.cfg.AShareMinute, w.cfg.HKHour, w.cfg.HKMinute)
}

func (w *Worker) runStartupCatchUp(ctx context.Context, scope string, hour, minute int) {
	now := w.cfg.NowFunc().In(shanghaiLocation())
	targetDate := now.Format("2006-01-02")
	scheduledTime := time.Date(now.Year(), now.Month(), now.Day(), hour, minute, 0, 0, now.Location())
	if now.Before(scheduledTime) {
		return
	}
	calendar := quadrant.NewSimPortfolioV2CalendarService()
	if !calendar.IsTradingDay(scope, targetDate) {
		return
	}
	hasRun, err := w.service.repo.HasCompletedSnapshotJobRun(ctx, scope, targetDate)
	if err != nil {
		log.Printf("%s %s startup catch-up check failed for %s: %v", portfolioWorkerLogPrefix, scope, targetDate, err)
		return
	}
	if hasRun {
		return
	}
	if _, err := w.RunOnce(ctx, scope, targetDate, scheduledTime, PortfolioSnapshotJobTriggerScheduler); err != nil {
		log.Printf("%s %s startup catch-up failed for %s: %v", portfolioWorkerLogPrefix, scope, targetDate, err)
		return
	}
	log.Printf("%s %s startup catch-up completed for %s", portfolioWorkerLogPrefix, scope, targetDate)
}

func (w *Worker) scheduleLoop(ctx context.Context, scope string, hour, minute int) {
	for {
		now := w.cfg.NowFunc()
		next := nextDailyTriggerTime(now, hour, minute)
		wait := next.Sub(now)
		log.Printf("%s %s next trigger at %s (in %s)", portfolioWorkerLogPrefix, scope, next.Format(time.RFC3339), wait.Round(time.Second))
		select {
		case <-ctx.Done():
			return
		case <-time.After(wait):
			if _, err := w.RunOnce(ctx, scope, "", time.Time{}, w.cfg.TriggerSource); err != nil {
				log.Printf("%s %s run failed: %v", portfolioWorkerLogPrefix, scope, err)
			}
		}
	}
}

func (w *Worker) RunOnce(ctx context.Context, scope, targetDate string, scheduledTime time.Time, triggerSource string) (*PortfolioSnapshotJobRunRecord, error) {
	if w.service == nil {
		return nil, fmt.Errorf("portfolio snapshot service is nil")
	}
	scope = strings.ToUpper(strings.TrimSpace(scope))
	if scope != PortfolioScopeAShare && scope != PortfolioScopeHK {
		return nil, fmt.Errorf("invalid snapshot scope: %s", scope)
	}
	if !w.beginRun(scope, targetDate, triggerSource) {
		return nil, fmt.Errorf("portfolio snapshot worker for %s is already running", scope)
	}
	defer w.finishRun(scope)
	if strings.TrimSpace(triggerSource) == "" {
		triggerSource = w.cfg.TriggerSource
	}
	runCtx := ctx
	cancel := func() {}
	if w.cfg.RunTimeout > 0 {
		runCtx, cancel = context.WithTimeout(ctx, w.cfg.RunTimeout)
	}
	defer cancel()
	jobRun, err := w.service.RunDailyMarketSnapshot(runCtx, scope, targetDate, scheduledTime, triggerSource)
	w.completeRun(scope, targetDate, triggerSource, jobRun, err)
	if err != nil {
		return nil, err
	}
	return jobRun, nil
}

func (w *Worker) beginRun(scope, targetDate, triggerSource string) bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.running[scope] {
		return false
	}
	w.running[scope] = true
	startedAt := w.cfg.NowFunc().UTC()
	w.lastRuns[scope] = WorkerRun{
		Scope:         scope,
		TargetDate:    strings.TrimSpace(targetDate),
		Status:        PortfolioSnapshotJobStatusRunning,
		StartedAt:     startedAt,
		TriggerSource: strings.TrimSpace(triggerSource),
	}
	return true
}

func (w *Worker) finishRun(scope string) {
	w.mu.Lock()
	defer w.mu.Unlock()
	delete(w.running, scope)
}

func (w *Worker) completeRun(scope, targetDate, triggerSource string, jobRun *PortfolioSnapshotJobRunRecord, err error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	run := w.lastRuns[scope]
	finishedAt := w.cfg.NowFunc().UTC()
	run.FinishedAt = &finishedAt
	if strings.TrimSpace(targetDate) != "" {
		run.TargetDate = strings.TrimSpace(targetDate)
	}
	if strings.TrimSpace(triggerSource) != "" {
		run.TriggerSource = strings.TrimSpace(triggerSource)
	}
	if jobRun != nil {
		run.JobRunID = jobRun.ID
		run.TargetDate = jobRun.TargetDate
		run.Status = jobRun.Status
		if strings.TrimSpace(jobRun.TriggerSource) != "" {
			run.TriggerSource = jobRun.TriggerSource
		}
		if jobRun.FinishedAt != nil {
			run.FinishedAt = jobRun.FinishedAt
		}
		if strings.TrimSpace(jobRun.Message) != "" && err == nil && jobRun.Status != PortfolioSnapshotJobStatusSuccess {
			run.ErrorMessage = jobRun.Message
		}
	}
	if err != nil {
		run.Status = PortfolioSnapshotJobStatusFailed
		run.ErrorMessage = err.Error()
	}
	w.lastRuns[scope] = run
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
	status := WorkerStatus{
		Enabled: w.cfg.Enabled,
		Running: w.running[PortfolioScopeAShare] || w.running[PortfolioScopeHK],
		Schedules: []string{
			fmt.Sprintf("%s %02d:%02d", PortfolioScopeAShare, w.cfg.AShareHour, w.cfg.AShareMinute),
			fmt.Sprintf("%s %02d:%02d", PortfolioScopeHK, w.cfg.HKHour, w.cfg.HKMinute),
		},
		NextRunAt: []time.Time{
			nextDailyTriggerTime(now, w.cfg.AShareHour, w.cfg.AShareMinute),
			nextDailyTriggerTime(now, w.cfg.HKHour, w.cfg.HKMinute),
		},
	}
	for _, scope := range []string{PortfolioScopeAShare, PortfolioScopeHK} {
		if run, ok := w.lastRuns[scope]; ok {
			status.LastRuns = append(status.LastRuns, run)
		}
	}
	return status
}
