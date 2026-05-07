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
