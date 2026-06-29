package quadrant

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"
)

const (
	defaultWorkerHour     = 20 // 北京时间收盘后 20:00 触发
	defaultWorkerMinute   = 0
	workerMaxAttempts     = 3
	workerCallbackTimeout = 30 * time.Minute // Quant 计算超时
	workerHTTPTimeout     = 10 * time.Second // 触发 HTTP 请求超时
)

var (
	workerBackoffs         = []time.Duration{5 * time.Minute, 10 * time.Minute}
	workerScheduleLocation = loadWorkerScheduleLocation()
)

func loadWorkerScheduleLocation() *time.Location {
	loc, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		return time.FixedZone("CST", 8*3600)
	}
	return loc
}

// Worker triggers scheduled quadrant computation via Quant service.
type Worker struct {
	service        *Service
	cfg            WorkerConfig
	quantURL       string
	callbackURL    string // Go 自身的 bulk-save URL
	signalService  webhookNotifier
	mu             sync.Mutex
	lastComputedAt time.Time
	lastError      string
	attemptsToday  int
}

// webhookNotifier is a minimal interface to send system notifications.
// The signal service can optionally implement this.
type webhookNotifier interface {
	SendSystemNotification(ctx context.Context, message string) error
}

// WorkerConfig holds configuration for the quadrant worker.
type WorkerConfig struct {
	Enabled            bool
	ComputeHour        int
	ComputeMinute      int
	RunOnNonTradingDay bool
	QuantServiceURL    string
	BackendBaseURL     string // e.g. "http://localhost:8080"
	NowFunc            func() time.Time
}

type scheduledRunDecision struct {
	Exchange           string
	ScheduledTradeDate string
	ResolvedTradeDate  string
	ShouldRun          bool
	Reason             string
}

// NewWorker creates a new quadrant worker.
func NewWorker(service *Service, cfg WorkerConfig, notifier webhookNotifier) *Worker {
	cfg = normalizeWorkerConfig(cfg)
	quantURL := strings.TrimRight(cfg.QuantServiceURL, "/")
	callbackURL := strings.TrimRight(cfg.BackendBaseURL, "/") + "/api/quadrant/bulk-save"

	return &Worker{
		service:       service,
		cfg:           cfg,
		quantURL:      quantURL,
		callbackURL:   callbackURL,
		signalService: notifier,
	}
}

func normalizeWorkerConfig(cfg WorkerConfig) WorkerConfig {
	if cfg.ComputeHour < 0 || cfg.ComputeHour > 23 {
		cfg.ComputeHour = defaultWorkerHour
	}
	if cfg.ComputeMinute < 0 || cfg.ComputeMinute > 59 {
		cfg.ComputeMinute = defaultWorkerMinute
	}
	if cfg.NowFunc == nil {
		cfg.NowFunc = time.Now
	}
	return cfg
}

// Start launches the background daily worker.
func (w *Worker) Start(ctx context.Context) {
	if w == nil || !w.cfg.Enabled || w.service == nil {
		log.Printf("[quadrant-worker] disabled")
		return
	}

	go func() {
		for {
			now := w.now()
			next := nextTriggerTime(now, w.cfg.ComputeHour, w.cfg.ComputeMinute)
			wait := next.Sub(now)

			log.Printf("[quadrant-worker] next trigger at %s (in %s)", next.Format(time.RFC3339), wait.Round(time.Second))

			select {
			case <-ctx.Done():
				return
			case <-time.After(wait):
				w.runScheduledCycle(ctx, next)
			}
		}
	}()
	log.Printf("[quadrant-worker] started, scheduled daily at %02d:%02d CST", w.cfg.ComputeHour, w.cfg.ComputeMinute)
}

func nextTriggerTime(now time.Time, hour, minute int) time.Time {
	nowCST := now.In(workerScheduleLocation)
	today := time.Date(nowCST.Year(), nowCST.Month(), nowCST.Day(), hour, minute, 0, 0, workerScheduleLocation)
	if nowCST.After(today) {
		return today.Add(24 * time.Hour)
	}
	return today
}

func scheduledTradeDate(t time.Time) string {
	return t.In(workerScheduleLocation).Format("2006-01-02")
}

func normalizeScheduledExchange(exchange string) string {
	if strings.EqualFold(strings.TrimSpace(exchange), "HKEX") {
		return "HKEX"
	}
	return "ASHARE"
}

func (w *Worker) now() time.Time {
	if w != nil && w.cfg.NowFunc != nil {
		return w.cfg.NowFunc()
	}
	return time.Now()
}

func (w *Worker) runScheduledCycle(ctx context.Context, scheduledAt time.Time) {
	ashareDecision := w.resolveScheduledRun(ctx, "ASHARE", scheduledAt)
	hkDecision := w.resolveScheduledRun(ctx, "HKEX", scheduledAt)

	if !ashareDecision.ShouldRun {
		w.markScheduledSkip(ashareDecision)
	} else {
		w.runWithRetry(ctx)
	}

	if !hkDecision.ShouldRun {
		w.markScheduledSkip(hkDecision)
	} else {
		w.triggerHKCompute(ctx)
	}
}

func (w *Worker) resolveScheduledRun(ctx context.Context, exchange string, scheduledAt time.Time) scheduledRunDecision {
	decision := scheduledRunDecision{
		Exchange:           normalizeScheduledExchange(exchange),
		ScheduledTradeDate: scheduledTradeDate(scheduledAt),
		ShouldRun:          true,
	}
	if w == nil || w.service == nil || w.service.tradeDateResolver == nil || w.cfg.RunOnNonTradingDay {
		decision.Reason = "scheduled run enabled"
		return decision
	}

	resolved := strings.TrimSpace(w.service.tradeDateResolver(ctx, decision.Exchange, scheduledAt))
	decision.ResolvedTradeDate = resolved
	if resolved == "" {
		decision.Reason = "trade date unavailable, continue scheduled run to avoid missing a trading day"
		return decision
	}
	if resolved == decision.ScheduledTradeDate {
		decision.Reason = "latest trade date matches scheduled date"
		return decision
	}
	decision.ShouldRun = false
	decision.Reason = fmt.Sprintf("latest trade date %s does not match scheduled date %s", resolved, decision.ScheduledTradeDate)
	return decision
}

func (w *Worker) markScheduledSkip(decision scheduledRunDecision) {
	message := fmt.Sprintf("%s 今日非交易日，20:00 自动跳过", scheduledExchangeLabel(decision.Exchange))
	if strings.TrimSpace(decision.ResolvedTradeDate) != "" {
		message = fmt.Sprintf("%s 今日非交易日，20:00 自动跳过（最近交易日 %s）", scheduledExchangeLabel(decision.Exchange), decision.ResolvedTradeDate)
	}
	log.Printf("[quadrant-worker] [%s] scheduled run skipped: %s", decision.Exchange, decision.Reason)
	UpdateProgress(decision.Exchange, ComputeProgress{
		Exchange: decision.Exchange,
		Status:   "skipped",
		Message:  message,
	})
	if decision.Exchange == "ASHARE" {
		w.mu.Lock()
		w.attemptsToday = 0
		w.lastError = ""
		w.mu.Unlock()
	}
}

func scheduledExchangeLabel(exchange string) string {
	if strings.EqualFold(strings.TrimSpace(exchange), "HKEX") {
		return "港股"
	}
	return "A股"
}

func (w *Worker) runWithRetry(ctx context.Context) {
	w.mu.Lock()
	w.attemptsToday = 0
	w.lastError = ""
	w.mu.Unlock()

	for attempt := 1; attempt <= workerMaxAttempts; attempt++ {
		w.mu.Lock()
		w.attemptsToday = attempt
		w.mu.Unlock()

		log.Printf("[quadrant-worker] attempt %d/%d: triggering Quant compute-all (A-share)", attempt, workerMaxAttempts)

		err := w.triggerCompute(ctx)
		if err == nil {
			if waitErr := w.waitForCompletion(ctx); waitErr == nil {
				w.mu.Lock()
				w.lastError = ""
				w.mu.Unlock()
				log.Printf("[quadrant-worker] ✅ A-share compute cycle completed successfully on attempt %d", attempt)
				return
			} else {
				log.Printf("[quadrant-worker] ⚠️ A-share callback wait failed on attempt %d: %v", attempt, waitErr)
				err = waitErr
			}
		} else {
			log.Printf("[quadrant-worker] ⚠️ A-share trigger failed on attempt %d: %v", attempt, err)
		}

		w.mu.Lock()
		w.lastError = err.Error()
		w.mu.Unlock()

		SetProgressTerminal("ASHARE", "failed", err.Error())

		if attempt < workerMaxAttempts {
			backoff := workerBackoffs[0]
			if attempt-1 < len(workerBackoffs) {
				backoff = workerBackoffs[attempt-1]
			}
			log.Printf("[quadrant-worker] retrying in %s...", backoff)
			select {
			case <-ctx.Done():
				return
			case <-time.After(backoff):
			}
		}
	}

	errMsg := fmt.Sprintf("四象限数据计算失败：已重试 %d 次均失败。最后错误：%s", workerMaxAttempts, w.lastError)
	log.Printf("[quadrant-worker] ❌ %s", errMsg)
	SetProgressTerminal("ASHARE", "failed", errMsg)
	if w.signalService != nil {
		notifyMsg := fmt.Sprintf(
			"⚠️ 四象限数据计算失败\n时间：%s\n重试：已重试 %d 次均失败\n原因：%s\n影响：四象限图数据可能已过期",
			w.now().Format("2006-01-02 15:04:05"),
			workerMaxAttempts,
			w.lastError,
		)
		if notifyErr := w.signalService.SendSystemNotification(context.Background(), notifyMsg); notifyErr != nil {
			log.Printf("[quadrant-worker] system notification failed: %v", notifyErr)
		}
	}
}

// TriggerComputeAShare publicly exposes A-share compute triggering for admin manual use.
func (w *Worker) TriggerComputeAShare() {
	ctx := context.Background()
	err := w.triggerCompute(ctx)
	if err != nil {
		log.Printf("[quadrant-worker] [manual] A-share trigger failed: %v", err)
		SetProgressTerminal("ASHARE", "failed", err.Error())
	}
}

// TriggerComputeHK publicly exposes HK compute triggering for admin manual use.
func (w *Worker) TriggerComputeHK() {
	w.triggerHKCompute(context.Background())
}

func (w *Worker) triggerCompute(ctx context.Context) error {
	exchange := "ASHARE"
	now := w.now()
	UpdateProgress(exchange, ComputeProgress{
		Exchange:  exchange,
		Status:    "running",
		Current:   0,
		Total:     5000,
		TaskLogID: fmt.Sprintf("qcl-%d", now.UnixMilli()),
		UpdatedAt: now,
	})

	url := w.quantURL + "/api/quadrant/compute-all"
	payload := map[string]string{"callback_url": w.callbackURL}
	body, _ := json.Marshal(payload)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: workerHTTPTimeout}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("quant request failed: %w", err)
	}
	defer resp.Body.Close()
	_, _ = io.ReadAll(resp.Body)

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("quant returned HTTP %d", resp.StatusCode)
	}

	return nil
}

// triggerHKCompute triggers the HK quadrant computation as a best-effort fire-and-forget operation.
func (w *Worker) triggerHKCompute(ctx context.Context) {
	exchange := "HKEX"
	now := w.now()
	UpdateProgress(exchange, ComputeProgress{
		Exchange:  exchange,
		Status:    "running",
		Current:   0,
		Total:     2600,
		TaskLogID: fmt.Sprintf("qcl-%d", now.UnixMilli()),
		UpdatedAt: now,
	})

	url := w.quantURL + "/api/quadrant/compute-hk-all"
	payload := map[string]string{"callback_url": w.callbackURL}
	body, _ := json.Marshal(payload)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		log.Printf("[quadrant-worker] [hk] create request failed: %v", err)
		return
	}
	req.Header.Set("Content-Type", "application/json")
	client := &http.Client{Timeout: workerHTTPTimeout}
	resp, err := client.Do(req)
	if err != nil {
		log.Printf("[quadrant-worker] [hk] quant request failed: %v", err)
		return
	}
	defer resp.Body.Close()
	_, _ = io.ReadAll(resp.Body)

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		log.Printf("[quadrant-worker] [hk] quant returned HTTP %d", resp.StatusCode)
		return
	}

	log.Printf("[quadrant-worker] [hk] ✅ HK quadrant compute triggered successfully")
}

func (w *Worker) waitForCompletion(ctx context.Context) error {
	deadline := w.now().Add(workerCallbackTimeout)

	w.mu.Lock()
	beforeAt := w.lastComputedAt
	w.mu.Unlock()

	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			latestAt, err := w.service.repo.GetLatestComputedAt(context.Background())
			if err != nil {
				continue
			}
			if latestAt != nil && latestAt.After(beforeAt) {
				w.mu.Lock()
				w.lastComputedAt = *latestAt
				w.mu.Unlock()
				return nil
			}
			if w.now().After(deadline) {
				SetProgressTerminal("ASHARE", "timeout", "回调超时（30min），数据可能已在后台完成")
				return fmt.Errorf("callback timeout after %s", workerCallbackTimeout)
			}
		}
	}
}

// GetWorkerStatus returns the internal worker state (for status API).
func (w *Worker) GetWorkerStatus() (lastComputedAt time.Time, lastError string, attemptsToday int) {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.lastComputedAt, w.lastError, w.attemptsToday
}
