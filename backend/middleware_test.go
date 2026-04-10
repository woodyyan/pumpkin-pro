package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestStatusResponseWriter_Defaults(t *testing.T) {
	rec := httptest.NewRecorder()
	rw := newStatusResponseWriter(rec)

	if rw.statusCode != http.StatusOK {
		t.Errorf("default status should be 200, got %d", rw.statusCode)
	}
}

func TestStatusResponseWriter_WriteHeader(t *testing.T) {
	tests := []struct {
		code int
	}{
		{200},
		{404},
		{500},
	}
	for _, tc := range tests {
		rec := httptest.NewRecorder()
		rw := newStatusResponseWriter(rec)
		rw.WriteHeader(tc.code)
		if rw.statusCode != tc.code {
			t.Errorf("WriteHeader(%d): expected statusCode %d, got %d",
				tc.code, tc.code, rw.statusCode)
		}
		if rec.Code != tc.code {
			t.Errorf("WriteHeader(%d): expected ResponseWriter Code %d, got %d",
				tc.code, tc.code, rec.Code)
		}
	}
}

func TestStatusResponseWriter_WriteHeader_UpdatesBeforeWrite(t *testing.T) {
	rec := httptest.NewRecorder()
	rw := newStatusResponseWriter(rec)
	// Before any Write(), each WriteHeader call updates statusCode
	rw.WriteHeader(201)
	if rw.statusCode != 201 {
		t.Errorf("expected status 201, got %d", rw.statusCode)
	}
	rw.WriteHeader(500)
	if rw.statusCode != 500 {
		t.Errorf("expected status 500 (updated before Write), got %d", rw.statusCode)
	}
}

func TestStatusResponseWriter_Write_SetsWrittenFlag(t *testing.T) {
	rec := httptest.NewRecorder()
	rw := newStatusResponseWriter(rec)

	if rw.written {
		t.Error("written flag should start as false")
	}

	n, err := rw.Write([]byte("hello"))
	if err != nil {
		t.Fatalf("Write failed: %v", err)
	}
	if n != 5 {
		t.Errorf("expected Write to return 5, got %d", n)
	}
	if !rw.written {
		t.Error("written flag should be true after Write")
	}
}

func TestClientIP_XForwardedFor(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("X-Forwarded-For", "1.2.3.4, 10.0.0.1")

	ip := clientIP(req)
	if ip != "1.2.3.4" {
		t.Errorf("clientIP with XFF: expected '1.2.3.4', got %q", ip)
	}
}

func TestClientIP_FallbackToRemoteAddr(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/test-path", nil)

	ip := clientIP(req)
	// RemoteAddr in httptest is typically "192.0.2.1:12345" or similar
	if !strings.Contains(ip, ".") || strings.Contains(ip, ":") && !strings.Contains(ip, "]") {
		t.Logf("clientIP fallback = %q (format may vary)", ip)
	}
	// Just verify it's not empty and not an error case
	if ip == "" {
		t.Error("clientIP should not be empty")
	}
}

func TestTruncateString(t *testing.T) {
	short := "hello"
	got := truncateString(short, 10)
	if got != "hello" {
		t.Errorf("truncateString(short) = %q, want 'hello'", got)
	}

	long := strings.Repeat("A", 100)
	got = truncateString(long, 50)
	if len([]rune(got)) != 50 {
		t.Errorf("truncateString(long,50): len=%d, want 50", len([]rune(got)))
	}

	exact := strings.Repeat("B", 20)
	got = truncateString(exact, 20)
	if got != exact {
		t.Errorf("truncateString(exact,20) should return unchanged")
	}
}

func TestDefaultContext(t *testing.T) {
	ctx, cancel := defaultContext(1 * time.Nanosecond)
	defer cancel()
	// Just verify it returns a valid context that has a Done channel
	if ctx == nil {
		t.Error("defaultContext returned nil context")
	}
	_ = ctx.Done() // channel should exist and be usable
}

// ── Integration-style test for loggingMiddleware ──

func TestLoggingMiddleware_IgnoresSuccess(t *testing.T) {
	received := false
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	})

	a := &appServer{
		adminService: nil,
	}
	mw := a.loggingMiddleware(handler)

	req := httptest.NewRequest(http.MethodGet, "/api/test", nil)
	rec := httptest.NewRecorder()
	mw.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rec.Code)
	}
	// Middleware should NOT try to persist 2xx responses
	// We can't easily verify the async goroutine didn't run, but at least no crash
	_ = received
}
