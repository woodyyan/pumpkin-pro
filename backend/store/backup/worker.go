package backup

import (
	"context"
	"log"
	"sync"
	"time"
)

const (
	defaultBackupWorkerHour   = 4  // 04:00 CST fallback trigger
	defaultBackupWorkerMinute = 0
)

// Worker manages dual-trigger backup scheduling:
//   Trigger A: callback from quadrant worker (onQuadrantComplete)
//   Trigger B: scheduled fallback at fixed time every day
type Worker struct {
	service *Service
	mu      sync.Mutex
}

// NewWorker creates a new backup worker.
func NewWorker(service *Service) *Worker {
	return &Worker{service: service}
}

// Start launches the background fallback scheduler (Trigger B).
// It runs independently of any external callbacks.
func (w *Worker) Start(ctx context.Context) {
	go func() {
		for {
			now := time.Now()
			next := nextBackupTriggerTime(now, defaultBackupWorkerHour, defaultBackupWorkerMinute)
			wait := next.Sub(now)

			log.Printf("[backup-worker] next fallback trigger at %s (in %s)",
				next.Format(time.RFC3339), wait.Round(time.Second))

			select {
			case <-ctx.Done():
				return
			case <-time.After(wait):
				log.Printf("[backup-worker] executing fallback backup (trigger=scheduled_fallback)")
				result, err := w.service.Run(ctx, "scheduled_fallback")
				if err != nil {
					log.Printf("[backup-worker] ❌ fallback failed: %v", err)
				} else {
					if result.Status == "skipped" {
						log.Printf("[backup-worker] ⏭️ skipped (cooldown active)")
					}
				}
			}
		}
	}()
	log.Printf("[backup-worker] started, fallback daily at %02d:%02d CST",
		defaultBackupWorkerHour, defaultBackupWorkerMinute)
}

// OnQuadrantComplete is called by the quadrant worker after a successful
// compute cycle. This is Trigger A — it fires an immediate backup.
//
// The service's internal cooldown logic ensures we don't double-backup if
// the fallback would fire shortly anyway.
func (w *Worker) OnQuadrantComplete(ctx context.Context) {
	log.Printf("[backup-worker] received quadrant-complete callback, triggering backup")
	result, err := w.service.Run(ctx, "quadrant_callback")
	if err != nil {
		log.Printf("[backup-worker] ❌ post-quadrant backup failed: %v", err)
		return
	}
	if result.Status == "skipped" {
		log.Printf("[backup-worker] ⏭️ post-quadrant backup skipped (cooldown)")
	}
}

// nextBackupTriggerTime computes the next occurrence of hour:minute in Asia/Shanghai.
func nextBackupTriggerTime(now time.Time, hour, minute int) time.Time {
	loc, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		loc = time.FixedZone("CST", 8*3600)
	}
	nowCST := now.In(loc)
	today := time.Date(nowCST.Year(), nowCST.Month(), nowCST.Day(), hour, minute, 0, 0, loc)
	if nowCST.After(today) {
		return today.Add(24 * time.Hour)
	}
	return today
}
