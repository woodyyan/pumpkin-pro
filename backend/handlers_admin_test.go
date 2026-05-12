package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/woodyyan/pumpkin-pro/backend/store/admin"
	"github.com/woodyyan/pumpkin-pro/backend/store/backup"
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

func TestHandleAdminBackupTriggerAccepted(t *testing.T) {
	db := testutil.InMemoryDB(t)
	testutil.AutoMigrateModels(t, db, &backup.BackupLogRecord{})
	service := backup.NewService(backup.NewRepository(db), db, backup.ServiceConfig{BackupDir: t.TempDir()})
	server := &appServer{backupService: service}

	req := httptest.NewRequest(http.MethodPost, "/api/admin/backup-trigger", nil)
	resp := httptest.NewRecorder()
	server.handleAdminBackupTrigger(resp, req)

	if resp.Code != http.StatusAccepted {
		t.Fatalf("expected 202, got %d", resp.Code)
	}
	var body backup.BackupTriggerResponse
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if !body.Accepted {
		t.Fatalf("expected accepted response, got %+v", body)
	}
	if body.CurrentJobStatus != backup.BackupJobStatusQueued {
		t.Fatalf("expected queued job status, got %s", body.CurrentJobStatus)
	}
}

func TestHandleAdminBackupTriggerConflict(t *testing.T) {
	db := testutil.InMemoryDB(t)
	testutil.AutoMigrateModels(t, db, &backup.BackupLogRecord{})
	repo := backup.NewRepository(db)
	service := backup.NewService(repo, db, backup.ServiceConfig{BackupDir: t.TempDir(), CooldownMinutes: 120})
	if err := repo.Insert(&backup.BackupLogRecord{
		TriggeredAt: time.Now(),
		TriggerType: "manual",
		Status:      backup.BackupStatusSuccess,
		CreatedAt:   time.Now(),
	}); err != nil {
		t.Fatalf("insert backup log: %v", err)
	}
	server := &appServer{backupService: service}

	req := httptest.NewRequest(http.MethodPost, "/api/admin/backup-trigger", nil)
	resp := httptest.NewRecorder()
	server.handleAdminBackupTrigger(resp, req)

	if resp.Code != http.StatusConflict {
		t.Fatalf("expected 409, got %d", resp.Code)
	}
	var body backup.BackupTriggerResponse
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body.Accepted {
		t.Fatalf("expected rejected response, got %+v", body)
	}
	if body.Reason != "cooldown" {
		t.Fatalf("expected cooldown reason, got %s", body.Reason)
	}
}

func TestHandleAdminLoginSetsSessionCookie(t *testing.T) {
	db := testutil.InMemoryDB(t)
	if err := admin.NewMigrator().AutoMigrate(db); err != nil {
		t.Fatalf("auto migrate admin models: %v", err)
	}
	svc := admin.NewService(admin.NewRepository(db), admin.ServiceConfig{JWTSecret: "test-secret", AccessTTL: time.Hour})
	if err := svc.SeedAdmin(t.Context(), "admin@example.com", "password123"); err != nil {
		t.Fatalf("seed admin: %v", err)
	}
	server := &appServer{adminService: svc}

	req := httptest.NewRequest(http.MethodPost, "/api/admin/login", strings.NewReader(`{"email":"admin@example.com","password":"password123"}`))
	req.Header.Set("Content-Type", "application/json")
	resp := httptest.NewRecorder()

	server.handleAdminLogin(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.Code)
	}
	result := resp.Result()
	defer result.Body.Close()
	var found bool
	for _, cookie := range result.Cookies() {
		if cookie.Name == adminSessionCookieName {
			found = true
			if !cookie.HttpOnly {
				t.Fatalf("expected admin cookie to be HttpOnly")
			}
			if cookie.Value == "" {
				t.Fatalf("expected admin cookie value")
			}
		}
	}
	if !found {
		t.Fatalf("expected admin session cookie")
	}
	var body map[string]any
	if err := json.NewDecoder(result.Body).Decode(&body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if _, ok := body["access_token"]; ok {
		t.Fatalf("login response should not expose access_token")
	}
}

func TestWithSuperAdminAuthAcceptsCookie(t *testing.T) {
	db := testutil.InMemoryDB(t)
	if err := admin.NewMigrator().AutoMigrate(db); err != nil {
		t.Fatalf("auto migrate admin models: %v", err)
	}
	svc := admin.NewService(admin.NewRepository(db), admin.ServiceConfig{JWTSecret: "test-secret", AccessTTL: time.Hour})
	if err := svc.SeedAdmin(t.Context(), "admin@example.com", "password123"); err != nil {
		t.Fatalf("seed admin: %v", err)
	}
	login, err := svc.Login(t.Context(), admin.AdminLoginInput{Email: "admin@example.com", Password: "password123"})
	if err != nil {
		t.Fatalf("login: %v", err)
	}
	server := &appServer{adminService: svc}

	called := false
	handler := server.withSuperAdminAuth(func(w http.ResponseWriter, r *http.Request) {
		called = true
		writeJSON(w, http.StatusOK, map[string]any{"ok": true})
	})
	req := httptest.NewRequest(http.MethodGet, "/api/admin/stats", nil)
	req.AddCookie(&http.Cookie{Name: adminSessionCookieName, Value: login.AccessToken})
	resp := httptest.NewRecorder()

	handler(resp, req)

	if !called {
		t.Fatalf("expected next handler to be called")
	}
	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.Code)
	}
}
