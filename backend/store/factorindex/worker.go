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

type Worker struct {
	service *Service
	cfg     WorkerConfig
	mu      sync.Mutex
	running bool
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
		if err := w.runWithRetry(ctx, "startup-catchup", w.service.SyncAll); err != nil {
			log.Printf("%s startup catch-up failed: %v", workerLogPrefix, err)
		}
	}()
	go w.scheduleLoop(ctx, "rebalance", w.cfg.RebalanceHour, w.cfg.RebalanceMinute, w.service.SyncRebalances)
	go w.scheduleLoop(ctx, "daily", w.cfg.DailyHour, w.cfg.DailyMinute, w.service.SyncAll)
	log.Printf("%s started, rebalance at %02d:%02d CST, daily NAV at %02d:%02d CST", workerLogPrefix, w.cfg.RebalanceHour, w.cfg.RebalanceMinute, w.cfg.DailyHour, w.cfg.DailyMinute)
}

func (w *Worker) scheduleLoop(ctx context.Context, name string, hour, minute int, run func(context.Context) error) {
	for {
		now := w.cfg.NowFunc()
		next := nextDailyTriggerTime(now, hour, minute)
		wait := next.Sub(now)
		log.Printf("%s %s next trigger at %s (in %s)", workerLogPrefix, name, next.Format(time.RFC3339), wait.Round(time.Second))
		select {
		case <-ctx.Done():
			return
		case <-time.After(wait):
			if err := w.runWithRetry(ctx, name, run); err != nil {
				log.Printf("%s %s failed: %v", workerLogPrefix, name, err)
			}
		}
	}
}

func (w *Worker) runWithRetry(ctx context.Context, name string, run func(context.Context) error) error {
	if !w.beginRun() {
		return fmt.Errorf("worker is already running")
	}
	defer w.finishRun()
	attempts := append([]time.Duration{0}, defaultRetryDelays...)
	var lastErr error
	for attempt, delay := range attempts {
		if delay > 0 {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(delay):
			}
		}
		runCtx, cancel := context.WithTimeout(ctx, w.cfg.RunTimeout)
		err := run(runCtx)
		cancel()
		if err == nil {
			return nil
		}
		lastErr = err
		log.Printf("%s %s attempt %d/%d failed: %v", workerLogPrefix, name, attempt+1, len(attempts), err)
	}
	return lastErr
}

func (w *Worker) beginRun() bool {
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
