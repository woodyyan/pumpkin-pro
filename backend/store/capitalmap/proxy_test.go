package capitalmap

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
		if r.URL.Path != "/api/capital-map" {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"source":"quant","market":{"sampleCount":1}}`))
	}))
	defer server.Close()

	svc := NewProxyService(server.URL, server.Client())
	first, err := svc.GetPayload(context.Background())
	if err != nil {
		t.Fatalf("first GetPayload failed: %v", err)
	}
	second, err := svc.GetPayload(context.Background())
	if err != nil {
		t.Fatalf("second GetPayload failed: %v", err)
	}
	if first["cacheStatus"] != "fresh" || second["cacheStatus"] != "fresh" {
		t.Fatalf("expected fresh cache statuses, got %v / %v", first["cacheStatus"], second["cacheStatus"])
	}
	if calls != 1 {
		t.Fatalf("expected one quant call due to cache, got %d", calls)
	}
}

func TestProxyServiceReturnsStaleCacheWhenQuantFails(t *testing.T) {
	fail := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if fail {
			http.Error(w, "down", http.StatusServiceUnavailable)
			return
		}
		_, _ = w.Write([]byte(`{"source":"quant","market":{"sampleCount":1}}`))
	}))
	defer server.Close()

	svc := NewProxyService(server.URL, server.Client())
	svc.now = func() time.Time { return time.Unix(0, 0) }
	if _, err := svc.GetPayload(context.Background()); err != nil {
		t.Fatalf("warm fetch failed: %v", err)
	}

	fail = true
	svc.now = func() time.Time { return time.Unix(int64(QuantProxyCacheTTL/time.Second)+1, 0) }
	payload, err := svc.GetPayload(context.Background())
	if err != nil {
		t.Fatalf("expected stale payload, got error: %v", err)
	}
	if payload["cacheStatus"] != "stale" {
		t.Fatalf("expected stale cache status, got %v", payload["cacheStatus"])
	}
	if !strings.Contains(payload["lastError"].(string), "503") {
		t.Fatalf("expected lastError to contain HTTP status, got %v", payload["lastError"])
	}
}

func TestProxyServiceErrorsWhenNoCacheAndQuantFails(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "down", http.StatusServiceUnavailable)
	}))
	defer server.Close()

	svc := NewProxyService(server.URL, server.Client())
	_, err := svc.GetPayload(context.Background())
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "503") {
		t.Fatalf("expected HTTP status in error, got %v", err)
	}
}
