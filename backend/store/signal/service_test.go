package signal

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/woodyyan/pumpkin-pro/backend/store/quadrant"
	"github.com/woodyyan/pumpkin-pro/backend/tests/testutil"
)

// ── Pure function / helper tests (no DB needed) ──

func TestNormalizeSide(t *testing.T) {
	tests := []struct {
		input   string
		want    string
		wantErr bool
	}{
		{"BUY", "BUY", false},
		{"buy", "BUY", false},
		{"  sell  ", "SELL", false},
		{"HOLD", "HOLD", false},
		{"", "HOLD", false}, // default
		{"INVALID", "", true},
	}
	for _, tc := range tests {
		got, err := normalizeSide(tc.input)
		if tc.wantErr && err == nil {
			t.Errorf("normalizeSide(%q): expected error", tc.input)
		}
		if !tc.wantErr && err != nil {
			t.Errorf("normalizeSide(%q): unexpected error %v", tc.input, err)
		}
		if !tc.wantErr && got != tc.want {
			t.Errorf("normalizeSide(%q) = %s, want %s", tc.input, got, tc.want)
		}
	}
}

func TestValidateWebhookURL(t *testing.T) {
	tests := []struct {
		url    string
		valid  bool
		reason string
	}{
		{"https://hooks.example.com/webhook", true, ""},
		{"https://api.example.com/path?q=1", true, ""},
		{"http://insecure.example.com", false, "https only"},
		{"ftp://example.com/hook", false, "invalid scheme"},
		{"", false, "empty"},
		{"  ", false, "whitespace only"},
		{"https://user:pass@example.com/hook", false, "userinfo"},
		{"https://localhost:8443/hook", false, "private host"},
		{"https://127.0.0.1/hook", false, "loopback"},
		{"https://10.0.0.1/hook", false, "private ip"},
	}
	for _, tc := range tests {
		got, err := validateWebhookURL(tc.url)
		if tc.valid {
			if err != nil {
				t.Errorf("validateWebhookURL(%q): expected success, got error %v", tc.url, err)
			}
			if !strings.HasPrefix(got, "https://") {
				t.Errorf("validateWebhookURL(%q): expected https prefix, got %q", tc.url, got)
			}
		} else {
			if err == nil {
				t.Errorf("validateWebhookURL(%q): expected error for reason=%s", tc.url, tc.reason)
			}
		}
	}
}

func TestIsPrivateHost(t *testing.T) {
	tests := []struct {
		host    string
		private bool
	}{
		{"localhost", true},
		{"localhost.local", true},
		{"myhost.local", true},
		{"127.0.0.1", true},
		{"::1", true},
		{"10.0.0.5", true},
		{"192.168.1.1", true},
		{"172.16.0.1", true},
		{"169.254.0.1", true}, // link-local
		{"0.0.0.0", true},      // unspecified
		{"8.8.8.8", false},
		{"hooks.example.com", false},
		{"api.tencentcloud.com", false},
	}
	for _, tc := range tests {
		got := isPrivateHost(tc.host)
		if got != tc.private {
			t.Errorf("isPrivateHost(%q) = %v, want %v", tc.host, got, tc.private)
		}
	}
}

func TestTrimError(t *testing.T) {
	short := trimError("short error")
	if short != "short error" {
		t.Errorf("trimError(short) = %q", short)
	}
	long := strings.Repeat("x", 2000)
	trimmed := trimError(long)
	if len(trimmed) > 1000 {
		t.Errorf("trimError(long): len=%d > 1000", len(trimmed))
	}
}

func TestFormatWebhookDeliveryStatusText(t *testing.T) {
	tests := []struct {
		status string
		want   string
	}{
		{"pending", "待发送"},
		{"processing", "发送中"},
		{"retrying", "重试中"},
		{"delivered", "已送达"},
		{"failed", "已失败"},
		{"", "未知"},
		{"UNKNOWN_STATUS", "UNKNOWN_STATUS"},
	}
	for _, tc := range tests {
		got := formatWebhookDeliveryStatusText(tc.status)
		if got != tc.want {
			t.Errorf("formatWebhookDeliveryStatusText(%q) = %q, want %q", tc.status, got, tc.want)
		}
	}
}

func TestPtrTime(t *testing.T) {
	now := time.Now().UTC()
	p := ptrTime(now)
	if p == nil {
		t.Fatal("ptrTime returned nil")
	}
	if !p.Equal(now) {
		t.Errorf("ptrTime mismatch: %v vs %v", p, now)
	}
}

// ── Service construction tests ──

func TestNewService_Defaults(t *testing.T) {
	s := NewService(nil, ServiceConfig{})
	if s.dispatcherInterval != defaultDispatcherInterval {
		t.Errorf("expected default dispatcher interval %v, got %v", defaultDispatcherInterval, s.dispatcherInterval)
	}
	if s.maxAttempts != defaultMaxAttempts {
		t.Errorf("expected default max attempts %d, got %d", defaultMaxAttempts, s.maxAttempts)
	}
	if len(s.retryBackoffs) == 0 {
		t.Error("retryBackoffs should not be empty")
	}
}

func TestNewService_CustomConfig(t *testing.T) {
	s := NewService(nil, ServiceConfig{
		SecretKey:          "test-key",
		DispatcherInterval: 5 * time.Second,
		MaxAttempts:        2,
	})
	if s.dispatcherInterval != 5*time.Second {
		t.Errorf("expected custom interval, got %v", s.dispatcherInterval)
	}
	if s.maxAttempts != 2 {
		t.Errorf("expected custom maxAttempts=2, got %d", s.maxAttempts)
	}
}

// ── Crypto round-trip test (AES-GCM) ──

func TestEncryptDecryptSecret_RoundTrip(t *testing.T) {
	s := NewService(nil, ServiceConfig{SecretKey: "test-secret-key-for-roundtrip"})

	plaintext := "my-webhook-secret-12345"

	cipherText, err := s.encryptSecret(plaintext)
	if err != nil {
		t.Fatalf("encryptSecret failed: %v", err)
	}
	if cipherText == "" {
		t.Fatal("encryptSecret returned empty")
	}

	decrypted, err := s.decryptSecret(cipherText)
	if err != nil {
		t.Fatalf("decryptSecret failed: %v", err)
	}
	if decrypted != plaintext {
		t.Errorf("round-trip mismatch: original=%q, decrypted=%q", plaintext, decrypted)
	}
}

func TestDecryptSecret_InvalidCipherText(t *testing.T) {
	s := NewService(nil, ServiceConfig{SecretKey: "another-key"})

	_, err := s.decryptSecret("not-valid-base64!!")
	if err == nil {
		t.Error("expected error for invalid base64")
	}

	_, err = s.decryptSecret("")
	if err == nil {
		t.Error("expected error for empty cipher text")
	}
}

func TestSignPayload_Deterministic(t *testing.T) {
	s := NewService(nil, ServiceConfig{SecretKey: "signing-key"})

	sig1 := s.signPayload("1234567890", []byte(`{"test":true}`), "secret")
	sig2 := s.signPayload("1234567890", []byte(`{"test":true}`), "secret")

	if sig1 != sig2 {
		t.Errorf("signPayload should be deterministic: sig1=%q sig2=%q", sig1, sig2)
	}
	if !strings.HasPrefix(sig1, "sha256=") {
		t.Errorf("signature should start with 'sha256=', got %q", sig1)
	}
}

func TestSignPayload_ChangesWithInput(t *testing.T) {
	s := NewService(nil, ServiceConfig{SecretKey: "key"})

	sigA := s.signPayload("100", []byte("body-a"), "secret")
	sigB := s.signPayload("200", []byte("body-b"), "other-secret")

	if sigA == sigB {
		t.Error("different inputs should produce different signatures")
	}
}

func TestBuildWebhookTextContent(t *testing.T) {
	event := SignalEventRecord{
		Symbol:    "600036.SH",
		Side:      "SELL",
		StrategyID: "strat-macd",
		SignalScore: 0.92,
		IsTest:     false,
		EventTime:  time.Date(2026, 4, 10, 14, 30, 0, 0, cstLocation()),
	}

	content := buildWebhookTextContent(event, map[string]any{"message": "MACD 死叉"})
	if !strings.Contains(content, "正式信号") {
		t.Error("should contain '正式信号' for non-test event")
	}
	if !strings.Contains(content, "600036.SH") {
		t.Error("should contain symbol")
	}
	if !strings.Contains(content, "SELL") {
		t.Error("should contain side SELL")
	}
	if !strings.Contains(content, "MACD 死叉") {
		t.Error("should contain reason message")
	}
}

func TestBuildWebhookTextContent_TestEvent(t *testing.T) {
	event := SignalEventRecord{
		Symbol:   "00700.HK",
		Side:     "BUY",
		IsTest:   true,
		EventTime: time.Now(),
	}
	content := buildWebhookTextContent(event, map[string]any{"kind": "webhook_test"})

	if !strings.Contains(content, "测试信号") {
		t.Error("should contain '测试信号' for test event")
	}
}

func TestSummarizeWebhookReason(t *testing.T) {
	tests := []struct {
		name     string
		reason   map[string]any
		expected string
	}{
		{"with message", map[string]any{"message": "custom msg"}, "custom msg"},
		{"with kind only", map[string]any{"kind": "macd_signal"}, "macd_signal"},
		{"empty", map[string]any{}, ""},
		{"nil", nil, ""},
		{"complex", map[string]any{"score": 0.85, "indicators": []string{"RSI", "MACD"}}, `{"indicators":["RSI","MACD"],"score":0.85}`},
	}
	for _, tc := range tests {
		got := summarizeWebhookReason(tc.reason)
		if got != tc.expected {
			t.Errorf("summarizeWebhookReason(%s): got %q, want %q", tc.name, got, tc.expected)
		}
	}
}

// ── Service-level tests (with DB) ──

func setupSignalServiceTest(t *testing.T) (*Service, *Repository, func()) {
	t.Helper()
	db := testutil.InMemoryDB(t)
	testutil.AutoMigrateModels(t, db,
		&SymbolSignalConfigRecord{},
		&quadrant.QuadrantScoreRecord{},
	)
	repo := NewRepository(db)
	svc := NewService(repo, ServiceConfig{})
	return svc, repo, func() {}
}

func TestSignalService_ListSymbolConfigRefs(t *testing.T) {
	svc, repo, cleanup := setupSignalServiceTest(t)
	defer cleanup()
	ctx := context.Background()

	for _, sym := range []string{"000001.SZ", "600000.SH"} {
		r := SymbolSignalConfigRecord{
			ID:                  "sc-" + sym,
			UserID:              "svc-ref-user",
			Symbol:              sym,
			StrategyID:          "svc-ref-strategy",
			IsEnabled:           true,
			CooldownSeconds:     3600,
			EvalIntervalSeconds: 3600,
			ThresholdsJSON:      "{}",
		}
		_, _ = repo.SaveSymbolConfig(ctx, r)
	}

	refs, err := svc.ListSymbolConfigRefs(ctx, "svc-ref-user", "svc-ref-strategy")
	if err != nil {
		t.Fatalf("ListSymbolConfigRefs failed: %v", err)
	}
	if len(refs) != 2 {
		t.Fatalf("expected 2 refs, got %d", len(refs))
	}
	got := map[string]bool{}
	for _, r := range refs {
		got[r.Symbol] = true
	}
	if !got["000001.SZ"] || !got["600000.SH"] {
		t.Errorf("expected symbols 000001.SZ and 600000.SH, got %v", got)
	}
}

func TestSignalService_ListSymbolConfigRefs_NoRefs(t *testing.T) {
	svc, _, cleanup := setupSignalServiceTest(t)
	defer cleanup()
	ctx := context.Background()

	refs, err := svc.ListSymbolConfigRefs(ctx, "svc-no-refs-user", "no-ref-strategy")
	if err != nil {
		t.Fatalf("ListSymbolConfigRefs failed: %v", err)
	}
	if len(refs) != 0 {
		t.Errorf("expected 0 refs, got %d", len(refs))
	}
}
