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
	defaultWorkerHour     = 2  // 凌晨 2 点触发
	defaultWorkerMinute   = 0
	workerMaxAttempts     = 3
	workerCallbackTimeout = 30 * time.Minute // Quant 计算超时
	workerHTTPTimeout     = 10 * time.Second  // 触发 HTTP 请求超时
)

var workerBackoffs = []time.Duration{5 * time.Minute, 10 * time.Minute}

// Worker triggers daily quadrant computation via Quant service.
type Worker struct {
	service        *Service
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
	QuantServiceURL string
	BackendBaseURL  string // e.g. "http://localhost:8080"
}

// NewWorker creates a new quadrant worker.
func NewWorker(service *Service, cfg WorkerConfig, notifier webhookNotifier) *Worker {
	quantURL := strings.TrimRight(cfg.QuantServiceURL, "/")
	callbackURL := strings.TrimRight(cfg.BackendBaseURL, "/") + "/api/quadrant/bulk-save"

	return &Worker{
		service:     service,
		quantURL:    quantURL,
		callbackURL: callbackURL,
		signalService: notifier,
	}
}

// Start launches the background daily worker.
func (w *Worker) Start(ctx context.Context) {
	go func() {
		for {
			now := time.Now()
			next := nextTriggerTime(now, defaultWorkerHour, defaultWorkerMinute)
			wait := next.Sub(now)

			log.Printf("[quadrant-worker] next trigger at %s (in %s)", next.Format(time.RFC3339), wait.Round(time.Second))

			select {
			case <-ctx.Done():
				return
			case <-time.After(wait):
				w.runWithRetry(ctx)
			}
		}
	}()
	log.Printf("[quadrant-worker] started, scheduled daily at %02d:%02d", defaultWorkerHour, defaultWorkerMinute)
}

func nextTriggerTime(now time.Time, hour, minute int) time.Time {
	// Always use Asia/Shanghai timezone for scheduling
	loc, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		loc = time.FixedZone("CST", 8*3600)
	}
	nowCST := now.In(loc)
	today := time.Date(nowCST.Year(), nowCST.Month(), nowCST.Day(), hour, minute, 0, 0, loc)
	if nowCST.After(today) {
		// Already past today's trigger time, schedule for tomorrow
		return today.Add(24 * time.Hour)
	}
	return today
}

func (w *Worker) runWithRetry(ctx context.Context) {
	w.mu.Lock()
	w.attemptsToday = 0
	w.mu.Unlock()

	// Phase 1: Trigger A-share quadrant computation (primary)
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

				// Phase 2: Trigger HK quadrant computation (best-effort, non-blocking)
				w.triggerHKCompute(ctx)

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

	// All attempts failed — send notification
	errMsg := fmt.Sprintf("四象限数据计算失败：已重试 %d 次均失败。最后错误：%s", workerMaxAttempts, w.lastError)
	log.Printf("[quadrant-worker] ❌ %s", errMsg)
	if w.signalService != nil {
		notifyMsg := fmt.Sprintf(
			"⚠️ 四象限数据计算失败\n时间：%s\n重试：已重试 %d 次均失败\n原因：%s\n影响：四象限图数据可能已过期",
			time.Now().Format("2006-01-02 15:04:05"),
			workerMaxAttempts,
			w.lastError,
		)
		if notifyErr := w.signalService.SendSystemNotification(context.Background(), notifyMsg); notifyErr != nil {
			log.Printf("[quadrant-worker] system notification failed: %v", notifyErr)
		}
	}
}

func (w *Worker) triggerCompute(ctx context.Context) error {
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
	io.ReadAll(resp.Body)

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("quant returned HTTP %d", resp.StatusCode)
	}

	return nil
}

// triggerHKCompute triggers the HK quadrant computation as a best-effort fire-and-forget operation.
// Failure here does NOT affect the overall A-share compute cycle success status.
func (w *Worker) triggerHKCompute(ctx context.Context) {
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
	io.ReadAll(resp.Body)

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		log.Printf("[quadrant-worker] [hk] quant returned HTTP %d", resp.StatusCode)
		return
	}

	log.Printf("[quadrant-worker] [hk] ✅ HK quadrant compute triggered successfully")
}

func (w *Worker) waitForCompletion(ctx context.Context) error {
	// Wait up to workerCallbackTimeout for the bulk-save callback to update computed_at
	deadline := time.Now().Add(workerCallbackTimeout)

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
			if time.Now().After(deadline) {
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
