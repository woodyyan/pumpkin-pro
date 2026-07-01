package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/woodyyan/pumpkin-pro/backend/store/factorindex"
	"github.com/woodyyan/pumpkin-pro/backend/store/factorlab"
	"github.com/woodyyan/pumpkin-pro/backend/tests/testutil"
)

func setupFactorIndexAdminServer(t *testing.T) *appServer {
	t.Helper()
	db := testutil.InMemoryDB(t)
	if err := factorlab.NewMigrator().AutoMigrate(db); err != nil {
		t.Fatalf("migrate factorlab: %v", err)
	}
	if err := factorindex.NewMigrator().AutoMigrate(db); err != nil {
		t.Fatalf("migrate factorindex: %v", err)
	}
	repo := factorindex.NewRepository(db)
	service := factorindex.NewService(repo)
	worker := factorindex.NewWorker(service, factorindex.WorkerConfig{Enabled: true, RunTimeout: time.Minute})
	return &appServer{factorIndexService: service, factorIndexWorker: worker}
}

func TestHandleAdminFactorIndexStatus(t *testing.T) {
	server := setupFactorIndexAdminServer(t)
	req := httptest.NewRequest(http.MethodGet, "/api/admin/factor-index/status", nil)
	resp := httptest.NewRecorder()

	server.handleAdminFactorIndexStatus(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.Code)
	}
	var body map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	items, ok := body["items"].([]any)
	if !ok || len(items) != 7 {
		t.Fatalf("expected 7 factor index items, got %+v", body["items"])
	}
}

func TestHandleAdminFactorIndexRecompute(t *testing.T) {
	server := setupFactorIndexAdminServer(t)
	body := bytes.NewBufferString(`{"operation":"sync_all"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/admin/factor-index/recompute", body)
	resp := httptest.NewRecorder()

	server.handleAdminFactorIndexRecompute(resp, req)

	if resp.Code != http.StatusAccepted {
		t.Fatalf("expected 202, got %d", resp.Code)
	}
	var payload map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if payload["ok"] != true {
		t.Fatalf("expected ok=true, got %+v", payload)
	}
}

func TestHandleAdminFactorIndexRecomputeRejectsInvalidJSON(t *testing.T) {
	server := setupFactorIndexAdminServer(t)
	req := httptest.NewRequest(http.MethodPost, "/api/admin/factor-index/recompute", bytes.NewBufferString("{"))
	resp := httptest.NewRecorder()

	server.handleAdminFactorIndexRecompute(resp, req)

	if resp.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", resp.Code)
	}
}
