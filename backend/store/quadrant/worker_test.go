package quadrant

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

func resetQuadrantProgressForTest() {
	progressMu.Lock()
	defer progressMu.Unlock()
	progressMap["ASHARE"] = &ComputeProgress{Exchange: "ASHARE", Status: "idle", UpdatedAt: time.Now()}
	progressMap["HKEX"] = &ComputeProgress{Exchange: "HKEX", Status: "idle", UpdatedAt: time.Now()}
}

func TestNormalizeWorkerConfigDefaults(t *testing.T) {
	cfg := normalizeWorkerConfig(WorkerConfig{
		ComputeHour:   -1,
		ComputeMinute: 99,
	})
	if cfg.ComputeHour != defaultWorkerHour || cfg.ComputeMinute != defaultWorkerMinute {
		t.Fatalf("expected default schedule %02d:%02d, got %02d:%02d", defaultWorkerHour, defaultWorkerMinute, cfg.ComputeHour, cfg.ComputeMinute)
	}
	if cfg.NowFunc == nil {
		t.Fatal("expected NowFunc to be initialized")
	}
}

func TestNextTriggerTimeUsesBeijing20Clock(t *testing.T) {
	before := time.Date(2026, 6, 29, 11, 59, 0, 0, time.UTC) // 19:59 CST
	next := nextTriggerTime(before, 20, 0)
	if got := next.In(workerScheduleLocation).Format("2006-01-02 15:04"); got != "2026-06-29 20:00" {
		t.Fatalf("before trigger got %s; want 2026-06-29 20:00", got)
	}

	after := time.Date(2026, 6, 29, 12, 1, 0, 0, time.UTC) // 20:01 CST
	next = nextTriggerTime(after, 20, 0)
	if got := next.In(workerScheduleLocation).Format("2006-01-02 15:04"); got != "2026-06-30 20:00" {
		t.Fatalf("after trigger got %s; want 2026-06-30 20:00", got)
	}
}

func TestResolveScheduledRunSkipsMarketHoliday(t *testing.T) {
	service := NewService(nil)
	service.SetTradeDateResolver(func(ctx context.Context, exchange string, computedAt time.Time) string {
		return "2026-06-26"
	})
	worker := NewWorker(service, WorkerConfig{
		Enabled:         true,
		QuantServiceURL: "http://quant.local",
		BackendBaseURL:  "http://backend.local",
	}, nil)
	scheduledAt := time.Date(2026, 6, 29, 20, 0, 0, 0, workerScheduleLocation)
	decision := worker.resolveScheduledRun(context.Background(), "HKEX", scheduledAt)
	if decision.ShouldRun {
		t.Fatalf("expected HKEX to be skipped on holiday, got %+v", decision)
	}
	if decision.ResolvedTradeDate != "2026-06-26" {
		t.Fatalf("ResolvedTradeDate = %s; want 2026-06-26", decision.ResolvedTradeDate)
	}
}

func TestResolveScheduledRunKeepsRunWhenTradeDateUnavailable(t *testing.T) {
	service := NewService(nil)
	service.SetTradeDateResolver(func(ctx context.Context, exchange string, computedAt time.Time) string {
		return ""
	})
	worker := NewWorker(service, WorkerConfig{
		Enabled:         true,
		QuantServiceURL: "http://quant.local",
		BackendBaseURL:  "http://backend.local",
	}, nil)
	scheduledAt := time.Date(2026, 6, 29, 20, 0, 0, 0, workerScheduleLocation)
	decision := worker.resolveScheduledRun(context.Background(), "ASHARE", scheduledAt)
	if !decision.ShouldRun {
		t.Fatalf("expected A-share run to continue when trade date is unavailable, got %+v", decision)
	}
}

func TestRunScheduledCycleTriggersOnlyEligibleMarket(t *testing.T) {
	resetQuadrantProgressForTest()

	var hkCalls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/quadrant/compute-hk-all":
			hkCalls.Add(1)
			w.WriteHeader(http.StatusAccepted)
			_, _ = w.Write([]byte(`{"status":"accepted"}`))
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer server.Close()

	service := NewService(nil)
	service.SetTradeDateResolver(func(ctx context.Context, exchange string, computedAt time.Time) string {
		if exchange == "HKEX" {
			return "2026-06-29"
		}
		return "2026-06-27"
	})
	worker := NewWorker(service, WorkerConfig{
		Enabled:         true,
		ComputeHour:     20,
		ComputeMinute:   0,
		QuantServiceURL: server.URL,
		BackendBaseURL:  "http://backend.local",
	}, nil)

	scheduledAt := time.Date(2026, 6, 29, 20, 0, 0, 0, workerScheduleLocation)
	worker.runScheduledCycle(context.Background(), scheduledAt)

	if got := hkCalls.Load(); got != 1 {
		t.Fatalf("expected HK trigger once, got %d", got)
	}
	progress := GetProgress()
	if progress["ASHARE"].Status != "skipped" {
		t.Fatalf("expected A-share progress skipped, got %+v", progress["ASHARE"])
	}
	if progress["HKEX"].Status != "running" {
		t.Fatalf("expected HK progress running after trigger, got %+v", progress["HKEX"])
	}
}
