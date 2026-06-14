package aipicker

import (
	"context"
	"log"
	"time"
)

const workerLogPrefix = "[ai-picker-worker]"

type WorkerConfig struct {
	Enabled bool
	Hour    int
	Minute  int
}

type Worker struct {
	cfg     WorkerConfig
	service *Service
	aiCfgFn func(context.Context) (AIConfig, error)
}

func NewWorker(cfg WorkerConfig, service *Service, aiCfgFn func(context.Context) (AIConfig, error)) *Worker {
	if cfg.Hour < 0 || cfg.Hour > 23 {
		cfg.Hour = 8
	}
	if cfg.Minute < 0 || cfg.Minute > 59 {
		cfg.Minute = 40
	}
	return &Worker{cfg: cfg, service: service, aiCfgFn: aiCfgFn}
}

func (w *Worker) Start(ctx context.Context) {
	if w == nil || !w.cfg.Enabled || w.service == nil || w.aiCfgFn == nil {
		log.Printf("%s disabled", workerLogPrefix)
		return
	}
	go func() {
		for {
			now := time.Now()
			next := nextDailyTriggerTime(now, w.cfg.Hour, w.cfg.Minute)
			wait := next.Sub(now)
			log.Printf("%s next trigger at %s (in %s)", workerLogPrefix, next.Format(time.RFC3339), wait.Round(time.Second))
			select {
			case <-ctx.Done():
				return
			case <-time.After(wait):
				if err := w.RunOnce(ctx); err != nil {
					log.Printf("%s run failed: %v", workerLogPrefix, err)
				}
			}
		}
	}()
	log.Printf("%s started, scheduled daily at %02d:%02d CST", workerLogPrefix, w.cfg.Hour, w.cfg.Minute)
}

func (w *Worker) RunOnce(ctx context.Context) error {
	cfg, err := w.aiCfgFn(ctx)
	if err != nil {
		return err
	}
	_, err = w.service.GenerateAndStoreDaily(ctx, cfg)
	return err
}

func nextDailyTriggerTime(now time.Time, hour, minute int) time.Time {
	loc := time.FixedZone("CST", 8*60*60)
	localNow := now.In(loc)
	next := time.Date(localNow.Year(), localNow.Month(), localNow.Day(), hour, minute, 0, 0, loc)
	if !next.After(localNow) {
		next = next.Add(24 * time.Hour)
	}
	return next
}
