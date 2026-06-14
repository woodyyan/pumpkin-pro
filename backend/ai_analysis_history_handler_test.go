package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	analysis_history "github.com/woodyyan/pumpkin-pro/backend/store/analysis_history"
	"github.com/woodyyan/pumpkin-pro/backend/store/auth"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
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

func TestHandleAnalysisHistorySubroutesRejectsMismatchedSymbolDetail(t *testing.T) {
	repo := analysis_history.NewRepository(newAnalysisHistoryTestDB(t))
	ctx := context.Background()
	record := &analysis_history.AnalysisHistoryRecord{
		ID:              "hist_123",
		UserID:          "user-1",
		Symbol:          "00700.HK",
		SymbolName:      "腾讯控股",
		Signal:          "buy",
		ConfidenceScore: 88,
		ResultJSON:      `{"signal":"buy"}`,
		MetaJSON:        `{}`,
		CreatedAt:       time.Now().UTC(),
	}
	if err := repo.Create(ctx, record); err != nil {
		t.Fatalf("create record: %v", err)
	}

	server := &appServer{analysisHistoryRepo: repo}
	req := httptest.NewRequest(http.MethodGet, "/api/live/symbols/000001.SZ/analysis-history?id=hist_123", nil)
	req = req.WithContext(auth.WithCurrentUser(req.Context(), auth.CurrentUser{UserID: "user-1"}))
	resp := httptest.NewRecorder()

	server.handleAnalysisHistorySubroutes(resp, req, "000001.SZ")

	if resp.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", resp.Code)
	}
}


func newAnalysisHistoryTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&analysis_history.AnalysisHistoryRecord{}); err != nil {
		t.Fatalf("migrate analysis history: %v", err)
	}
	return db
}
