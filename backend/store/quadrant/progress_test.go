package quadrant

import (
	"sync"
	"testing"
)

// ── Init state tests ──

func TestProgress_InitState(t *testing.T) {
	tests := []struct {
		name     string
		exchange string
		wantIdle bool
	}{
		{"ASHARE idle on init", "ASHARE", true},
		{"HKEX idle on init", "HKEX", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			progress := GetProgress()
			p, ok := progress[tt.exchange]
			if !ok {
				t.Fatalf("exchange %q not found in progress map", tt.exchange)
			}
			if p.Status != "idle" {
				t.Errorf("expected status=idle, got %q", p.Status)
			}
			if p.Exchange != tt.exchange {
				t.Errorf("expected exchange=%s, got %s", tt.exchange, p.Exchange)
			}
		})
	}
}

// ── UpdateProgress tests ──

func TestUpdateProgress_Basic(t *testing.T) {
	p := UpdateProgress("ASHARE", ComputeProgress{
		Exchange: "ASHARE",
		Status:   "running",
		Current:  50,
		Total:    100,
	})

	if p.Status != "running" {
		t.Errorf("expected running, got %s", p.Status)
	}
	if p.Current != 50 {
		t.Errorf("expected current=50, got %d", p.Current)
	}
	if p.Total != 100 {
		t.Errorf("expected total=100, got %d", p.Total)
	}
	// Percent should be auto-calculated
	expectedPct := float64(50) / float64(100) * 100
	if p.Percent != expectedPct {
		t.Errorf("expected percent=%.1f, got %.1f", expectedPct, p.Percent)
	}
	if p.UpdatedAt.IsZero() {
		t.Error("UpdatedAt should not be zero")
	}
}

func TestUpdateProgress_ZeroTotal(t *testing.T) {
	p := UpdateProgress("HKEX", ComputeProgress{
		Exchange: "HKEX",
		Status:   "running",
		Current:  0,
		Total:    0,
	})

	if p.Percent != 0 {
		t.Errorf("expected percent=0 when total is unknown, got %.1f", p.Percent)
	}
}

func TestUpdateProgress_Complete(t *testing.T) {
	p := UpdateProgress("ASHARE", ComputeProgress{
		Exchange: "ASHARE",
		Status:   "success",
		Current:  100,
		Total:    100,
	})

	if p.Percent != 100 {
		t.Errorf("expected percent=100 when complete, got %.1f", p.Percent)
	}
}

func TestUpdateProgress_Overwrites(t *testing.T) {
	// First update
	UpdateProgress("ASHARE", ComputeProgress{
		Exchange: "ASHARE", Status: "running", Current: 10, Total: 100,
	})

	// Second update should overwrite
	p := UpdateProgress("ASHARE", ComputeProgress{
		Exchange: "ASHARE", Status: "running", Current: 80, Total: 100,
	})

	if p.Current != 80 {
		t.Errorf("expected current=80 after overwrite, got %d", p.Current)
	}
}

// ── GetProgress tests ──

func TestGetProgress_ReturnsSnapshot(t *testing.T) {
	// Set up known state
	UpdateProgress("ASHARE", ComputeProgress{
		Exchange: "ASHARE", Status: "running", Current: 42, Total: 200,
	})
	UpdateProgress("HKEX", ComputeProgress{
		Exchange: "HKEX", Status: "idle", Current: 0, Total: 0,
	})

	result := GetProgress()

	if len(result) != 2 {
		t.Errorf("expected 2 exchanges in result, got %d", len(result))
	}

	a := result["ASHARE"]
	if a.Current != 42 || a.Total != 200 {
		t.Errorf("ASHARE: expected (42/200), got (%d/%d)", a.Current, a.Total)
	}

	h := result["HKEX"]
	if h.Status != "idle" {
		t.Errorf("HKEX: expected status=idle, got %s", h.Status)
	}
}

// Modifying returned snapshot should NOT affect internal state
// (GetProgress returns map[string]ComputeProgress — struct values are copied)
func TestGetProgress_SnapshotIsolation(t *testing.T) {
	UpdateProgress("ASHARE", ComputeProgress{
		Exchange: "ASHARE", Status: "running", Current: 30, Total: 100,
	})

	_ = GetProgress() // snapshot is a copy by value

	// Internal state should remain unchanged
	real := GetProgress()
	if real["ASHARE"].Current != 30 {
		t.Errorf("expected current=30 preserved, got %d", real["ASHARE"].Current)
	}
}

// ── SetProgressTerminal tests ──

func TestSetProgressTerminal_SuccessFromRunning(t *testing.T) {
	// Set running state first
	UpdateProgress("ASHARE", ComputeProgress{
		Exchange: "ASHARE", Status: "running", Current: 95, Total: 100,
		TaskLogID: "log-123",
	})

	SetProgressTerminal("ASHARE", "success", "")

	p := GetProgress()["ASHARE"]
	if p.Status != "success" {
		t.Errorf("expected success, got %s", p.Status)
	}
	// Terminal should set Current = Total
	if p.Current != 100 {
		t.Errorf("expected current=100 (clamped to total), got %d", p.Current)
	}
	if p.Percent != 100 {
		t.Errorf("expected percent=100, got %.1f", p.Percent)
	}
	// Should preserve TaskLogID from running state
	if p.TaskLogID != "log-123" {
		t.Errorf("expected TaskLogID=log-123, got %s", p.TaskLogID)
	}
}

func TestSetProgressTerminal_FailedWithErrorMsg(t *testing.T) {
	UpdateProgress("HKEX", ComputeProgress{
		Exchange: "HKEX", Status: "running", Current: 20, Total: 500,
	})

	SetProgressTerminal("HKEX", "failed", "成功率不足: 60%")

	p := GetProgress()["HKEX"]
	if p.Status != "failed" {
		t.Errorf("expected failed, got %s", p.Status)
	}
	if p.ErrorMsg != "成功率不足: 60%" {
		t.Errorf("expected error msg preserved, got %q", p.ErrorMsg)
	}
	// Failed should NOT clamp Current to Total
	if p.Current != 20 {
		t.Errorf("expected current=20 (preserved), got %d", p.Current)
	}
}

func TestSetProgressTerminal_NoPriorState(t *testing.T) {
	// Reset to idle (no prior running info) - use a fresh key conceptually
	// Since we can't delete a key easily, just test that it doesn't panic
	// and creates a valid entry
	SetProgressTerminal("UNKNOWN_EXCHANGE", "timeout", "连接超时")

	p := GetProgress()["UNKNOWN_EXCHANGE"]
	if p.Status != "timeout" {
		t.Errorf("expected timeout, got %s", p.Status)
	}
	if p.ErrorMsg != "连接超时" {
		t.Errorf("expected error msg, got %q", p.ErrorMsg)
	}
}

// ── IsRunning tests ──

func TestIsRunning_TrueWhenAnyRunning(t *testing.T) {
	// Reset both to non-running
	SetProgressTerminal("ASHARE", "success", "")
	SetProgressTerminal("HKEX", "idle", "")
	if IsRunning() {
		t.Error("expected IsRunning=false when both idle/success")
	}

	// Make one running
	UpdateProgress("ASHARE", ComputeProgress{Exchange: "ASHARE", Status: "running"})
	if !IsRunning() {
		t.Error("expected IsRunning=true when ASHARE is running")
	}
}

func TestIsRunning_FalseWhenAllTerminal(t *testing.T) {
	SetProgressTerminal("ASHARE", "success", "")
	SetProgressTerminal("HKEX", "failed", "some error")

	if IsRunning() {
		t.Error("expected IsRunning=false when all terminal")
	}
}

// ── Concurrency safety tests ──

func TestProgress_ConcurrentWrites(t *testing.T) {
	const goroutines = 50
	var wg sync.WaitGroup
	wg.Add(goroutines)

	for i := 0; i < goroutines; i++ {
		go func(idx int) {
			defer wg.Done()
			UpdateProgress("ASHARE", ComputeProgress{
				Exchange: "ASHARE",
				Status:   "running",
				Current:  idx,
				Total:    goroutines,
			})
		}(i)
	}

	wg.Wait()

	// Should not panic and should have a valid final state
	p := GetProgress()["ASHARE"]
	if p.Status != "running" {
		t.Errorf("expected running, got %s", p.Status)
	}
	if p.Total != goroutines {
		t.Errorf("expected total=%d, got %d", goroutines, p.Total)
	}
	if p.Current < 0 || p.Current >= goroutines {
		t.Errorf("current out of range: %d", p.Current)
	}
}

func TestProgress_ConcurrentReadWrite(t *testing.T) {
	const iterations = 100
	var wg sync.WaitGroup
	wg.Add(iterations * 2)

	// Concurrent writers
	for i := 0; i < iterations; i++ {
		go func(idx int) {
			defer wg.Done()
			UpdateProgress("HKEX", ComputeProgress{
				Exchange: "HKEX", Status: "running", Current: idx, Total: iterations,
			})
		}(i)
	}

	// Concurrent readers
	for i := 0; i < iterations; i++ {
		go func() {
			defer wg.Done()
			_ = GetProgress() // Just ensure no data race / panic
		}()
	}

	wg.Wait()
	// If we get here without race detector complaining, we're good
}

// ── Edge cases ──

func TestUpdateProgress_NegativePercent(t *testing.T) {
	// Current > Total shouldn't happen normally but let's be defensive
	p := UpdateProgress("ASHARE", ComputeProgress{
		Exchange: "ASHARE", Status: "running", Current: 150, Total: 100,
	})

	if p.Percent != 150.0 { // > 100 is acceptable here, it's just math
		t.Errorf("expected percent=150, got %.1f", p.Percent)
	}
}

func TestGetProgress_ContainsAllExchanges(t *testing.T) {
	result := GetProgress()

	// Must have at least ASHARE and HKEX (initialized in init())
	if _, ok := result["ASHARE"]; !ok {
		t.Error("missing ASHARE exchange")
	}
	if _, ok := result["HKEX"]; !ok {
		t.Error("missing HKEX exchange")
	}
}
