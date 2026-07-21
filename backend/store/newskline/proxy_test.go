package newskline

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestProxyServiceFetchesQuantAndCaches(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.URL.Path != "/api/news-kline/600519.SH" {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		if r.URL.Query().Get("days") != "500" || r.URL.Query().Get("pages") != "3" {
			t.Fatalf("unexpected query %s", r.URL.RawQuery)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"META":{"symbol":"600519.SH"},"KLINE":[],"EVENTS":[],"STATS":[],"CATS":{}}`))
	}))
	defer server.Close()

	svc := NewProxyService(server.URL, server.Client())
	first, err := svc.GetPayload(context.Background(), "600519.SH", 500, 3, false)
	if err != nil {
		t.Fatalf("first GetPayload failed: %v", err)
	}
	second, err := svc.GetPayload(context.Background(), "600519.SH", 500, 3, false)
	if err != nil {
		t.Fatalf("second GetPayload failed: %v", err)
	}
	if calls != 1 {
		t.Fatalf("expected one quant call due to cache, got %d", calls)
	}
	for _, payload := range []map[string]any{first, second} {
		meta := payload["META"].(map[string]any)
		if meta["cache_status"] != "fresh" {
			t.Fatalf("expected fresh cache status, got %v", meta["cache_status"])
		}
	}
}

func TestProxyServiceReturnsStaleCacheWhenQuantFails(t *testing.T) {
	fail := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if fail {
			http.Error(w, "down", http.StatusServiceUnavailable)
			return
		}
		_, _ = w.Write([]byte(`{"META":{"symbol":"00700.HK"},"KLINE":[],"EVENTS":[],"STATS":[],"CATS":{}}`))
	}))
	defer server.Close()

	svc := NewProxyService(server.URL, server.Client())
	svc.now = func() time.Time { return time.Unix(0, 0) }
	if _, err := svc.GetPayload(context.Background(), "00700.HK", 500, 3, false); err != nil {
		t.Fatalf("warm fetch failed: %v", err)
	}

	fail = true
	svc.now = func() time.Time { return time.Unix(int64(QuantProxyCacheTTL/time.Second)+1, 0) }
	payload, err := svc.GetPayload(context.Background(), "00700.HK", 500, 3, false)
	if err != nil {
		t.Fatalf("expected stale payload, got error: %v", err)
	}
	meta := payload["META"].(map[string]any)
	if meta["cache_status"] != "stale" {
		t.Fatalf("expected stale cache status, got %v", meta["cache_status"])
	}
	if !strings.Contains(meta["last_error"].(string), "503") {
		t.Fatalf("expected last_error to contain HTTP status, got %v", meta["last_error"])
	}
}

func TestProxyServiceErrorsWhenNoCacheAndQuantFails(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "down", http.StatusServiceUnavailable)
	}))
	defer server.Close()

	svc := NewProxyService(server.URL, server.Client())
	_, err := svc.GetPayload(context.Background(), "600519.SH", 500, 3, false)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "503") {
		t.Fatalf("expected HTTP status in error, got %v", err)
	}
}

func TestNormalizeBounds(t *testing.T) {
	if got := NormalizeDays("10"); got != MinDays {
		t.Fatalf("expected min days %d, got %d", MinDays, got)
	}
	if got := NormalizePages("10"); got != MaxPages {
		t.Fatalf("expected max pages %d, got %d", MaxPages, got)
	}
}
