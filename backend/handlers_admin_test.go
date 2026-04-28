package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/woodyyan/pumpkin-pro/backend/store/admin"
	"github.com/woodyyan/pumpkin-pro/backend/tests/testutil"
)

func TestHandleAdminAIConfigReturnsCipherKeyError(t *testing.T) {
	db := testutil.InMemoryDB(t)
	if err := admin.NewMigrator().AutoMigrate(db); err != nil {
		t.Fatalf("auto migrate admin models: %v", err)
	}
	server := &appServer{
		adminService: admin.NewService(admin.NewRepository(db), admin.ServiceConfig{}),
	}

	req := httptest.NewRequest(http.MethodPut, "/api/admin/ai-config", strings.NewReader(`{"base_url":"https://provider.example/v1","model_id":"model-a","api_key":"secret-key-1","is_enabled":true}`))
	req.Header.Set("Content-Type", "application/json")
	resp := httptest.NewRecorder()

	server.handleAdminAIConfig(resp, req)

	if resp.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", resp.Code)
	}
	var body map[string]string
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body["detail"] != "服务器未配置 AI 配置加密密钥，暂时无法保存后台 AI 配置" {
		t.Fatalf("unexpected detail: %q", body["detail"])
	}
}
