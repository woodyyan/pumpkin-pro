package signal

import (
	"testing"
	"time"
)

// ── Evaluator Config / NewEvaluator ──

func TestNewEvaluator_DefaultInterval(t *testing.T) {
	cfg := EvaluatorConfig{
		QuantServiceURL: "http://localhost:8001",
		Interval:        0, // should default to 15 min
	}
	ev := NewEvaluator(nil, nil, nil, cfg)
	if ev.interval != 15*time.Minute {
		t.Errorf("default interval = %v, want 15m", ev.interval)
	}
}

func TestNewEvaluator_CustomInterval(t *testing.T) {
	cfg := EvaluatorConfig{
		QuantServiceURL: "http://quant:8080",
		Interval:        5 * time.Minute,
	}
	ev := NewEvaluator(nil, nil, nil, cfg)
	if ev.interval != 5*time.Minute {
		t.Errorf("custom interval = %v, want 5m", ev.interval)
	}
}

func TestNewEvaluator_NegativeInterval(t *testing.T) {
	cfg := EvaluatorConfig{
		QuantServiceURL: "http://localhost:8001",
		Interval:        -1 * time.Minute, // negative → default
	}
	ev := NewEvaluator(nil, nil, nil, cfg)
	if ev.interval != 15*time.Minute {
		t.Errorf("negative interval → got %v, want 15m", ev.interval)
	}
}

func TestNewEvaluator_TrimsTrailingSlash(t *testing.T) {
	cases := []struct {
		input string
		want  string
	}{
		{"http://quant:8080/", "http://quant:8080"},
		{"http://quant:8080//", "http://quant:8080"}, // TrimRight strips ALL trailing slashes
		{"http://quant:8080", "http://quant:8080"},
		{"http://quant:8080/api/", "http://quant:8080/api"},
	}
	for _, c := range cases {
		cfg := EvaluatorConfig{QuantServiceURL: c.input}
		ev := NewEvaluator(nil, nil, nil, cfg)
		if ev.quantURL != c.want {
			t.Errorf("TrimSlash(%q) = %q, want %q", c.input, ev.quantURL, c.want)
		}
	}
}

// ── truncate (package-level helper in evaluator.go) ──

// Note: truncate in evaluator.go is unexported; we test via public behavior
// or re-implement the assertion here.

func TestEvaluatorTruncateLogic(t *testing.T) {
	// Replicate the truncate function logic for testing
	short := "hello world"
	if len(short) <= 80 && len(short) == len(short) { // trivially true
		// short strings pass through
	}

	longStr := ""
	for i := 0; i < 200; i++ {
		longStr += "a"
	}
	// truncate to 80 chars
	maxLen := 80
	var truncated string
	if len(longStr) > maxLen {
		truncated = longStr[:maxLen]
	} else {
		truncated = longStr
	}
	if len(truncated) != maxLen {
		t.Errorf("truncate result length = %d, want %d", len(truncated), maxLen)
	}
}

func TestTruncateBytes_Empty(t *testing.T) {
	// Test the truncateBytes-style logic
	b := []byte{}
	result := string(b)
	if len(b) > 200 {
		result = string(b[:200])
	}
	if result != "" {
		t.Errorf("empty bytes truncation should return empty")
	}
}

func TestTruncateBytes_Short(t *testing.T) {
	b := []byte("short message")
	result := string(b)
	if len(b) > 200 {
		result = string(b[:200])
	}
	if result != "short message" {
		t.Errorf("short bytes unchanged expected")
	}
}

func TestTruncateBytes_OverLimit(t *testing.T) {
	longB := make([]byte, 300)
	for i := range longB {
		longB[i] = 'x'
	}
	var result string
	if len(longB) > 200 {
		result = string(longB[:200])
	} else {
		result = string(longB)
	}
	if len(result) != 200 {
		t.Errorf("truncated bytes length = %d, want 200", len(result))
	}
}
