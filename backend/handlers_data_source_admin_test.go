package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/woodyyan/pumpkin-pro/backend/config"
)

func TestHandleAdminDataSourceHealth(t *testing.T) {
	quant := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/api/data-sources/health" {
			t.Fatalf("unexpected quant request: %s %s", r.Method, r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"providers":{"eastmoney":{"success":2,"failed":1,"last_status":"success"}},"capabilities":{"fundamentals":{"success":1,"failed":0,"last_provider":"eastmoney","last_status":"success","last_market":"ASHARE"}},"total_events":3,"recent":[]}`))
	}))
	defer quant.Close()

	server := &appServer{cfg: config.Config{QuantServiceURL: quant.URL}}
	req := httptest.NewRequest(http.MethodGet, "/api/admin/data-source-health", nil)
	resp := httptest.NewRecorder()
	server.handleAdminDataSourceHealth(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", resp.Code, resp.Body.String())
	}
	var payload map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	health, ok := payload["data_source_health"].(map[string]any)
	if !ok || health["total_events"] != float64(3) {
		t.Fatalf("unexpected data_source_health: %+v", payload)
	}
	if payload["updated_at"] == nil {
		t.Fatalf("expected updated_at, got %+v", payload)
	}
	if _, exists := payload["refresh"]; exists {
		t.Fatalf("retired company profile refresh state should not be returned: %+v", payload)
	}
}

func TestHandleAdminDataSourceHealthRejectsUnsupportedMethod(t *testing.T) {
	server := &appServer{}
	req := httptest.NewRequest(http.MethodPost, "/api/admin/data-source-health", nil)
	resp := httptest.NewRecorder()
	server.handleAdminDataSourceHealth(resp, req)
	if resp.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", resp.Code)
	}
}

func TestHandleAdminDataSourceHealthRequiresQuantURL(t *testing.T) {
	server := &appServer{}
	req := httptest.NewRequest(http.MethodGet, "/api/admin/data-source-health", nil)
	resp := httptest.NewRecorder()
	server.handleAdminDataSourceHealth(resp, req)
	if resp.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", resp.Code)
	}
}

func TestHandleAdminDataSourceHealthRejectsInvalidQuantPayload(t *testing.T) {
	quant := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`not-json`))
	}))
	defer quant.Close()

	server := &appServer{cfg: config.Config{QuantServiceURL: quant.URL}}
	req := httptest.NewRequest(http.MethodGet, "/api/admin/data-source-health", nil)
	resp := httptest.NewRecorder()
	server.handleAdminDataSourceHealth(resp, req)
	if resp.Code != http.StatusBadGateway {
		t.Fatalf("expected 502, got %d", resp.Code)
	}
}
