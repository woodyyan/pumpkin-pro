package main

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHandleGlobalAIAnalysisHistoryRequiresLogin(t *testing.T) {
	server := &appServer{}

	req := httptest.NewRequest(http.MethodGet, "/api/ai-analysis/history?page=1&page_size=10", nil)
	resp := httptest.NewRecorder()

	server.withRequiredAuth(server.handleGlobalAIAnalysisHistory)(resp, req)

	if resp.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", resp.Code)
	}
}
