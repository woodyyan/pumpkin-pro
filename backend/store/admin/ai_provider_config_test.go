package admin

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/woodyyan/pumpkin-pro/backend/config"
	"github.com/woodyyan/pumpkin-pro/backend/tests/testutil"
)

const testAICipherKey = "0123456789abcdef0123456789abcdef"

func newTestAIService(t *testing.T, envAI config.AIConfig) *Service {
	t.Helper()
	db := testutil.InMemoryDB(t)
	if err := NewMigrator().AutoMigrate(db); err != nil {
		t.Fatalf("auto migrate admin models: %v", err)
	}
	repo := NewRepository(db)
	return NewService(repo, ServiceConfig{EnvAI: envAI})
}

func TestSaveAIProviderConfigPreservesExistingKey(t *testing.T) {
	svc := newTestAIService(t, config.AIConfig{CipherKey: testAICipherKey})
	ctx := context.Background()

	first, err := svc.SaveAIProviderConfig(ctx, SaveAIProviderConfigInput{
		BaseURL:   "https://provider.example/v1/",
		ModelID:   "model-a",
		APIKey:    "secret-key-1",
		IsEnabled: true,
	})
	if err != nil {
		t.Fatalf("save first config: %v", err)
	}
	if !first.Config.HasAPIKey {
		t.Fatalf("expected saved API key metadata")
	}
	initialMask := first.Config.APIKeyMask
	if initialMask == "" {
		t.Fatalf("expected masked API key")
	}

	second, err := svc.SaveAIProviderConfig(ctx, SaveAIProviderConfigInput{
		BaseURL:   "https://provider.example/v1",
		ModelID:   "model-b",
		APIKey:    "",
		IsEnabled: true,
	})
	if err != nil {
		t.Fatalf("save second config: %v", err)
	}
	if second.Config.APIKeyMask != initialMask {
		t.Fatalf("expected API key mask to be preserved, got %q want %q", second.Config.APIKeyMask, initialMask)
	}

	resolved, err := svc.ResolveRuntimeAIConfig(ctx)
	if err != nil {
		t.Fatalf("resolve runtime config: %v", err)
	}
	if resolved.Source != "admin" {
		t.Fatalf("expected admin config source, got %q", resolved.Source)
	}
	if resolved.ModelID != "model-b" {
		t.Fatalf("expected updated model id, got %q", resolved.ModelID)
	}
	if resolved.APIKey != "secret-key-1" {
		t.Fatalf("expected preserved API key, got %q", resolved.APIKey)
	}
}

func TestResolveRuntimeAIConfigFallsBackToEnv(t *testing.T) {
	svc := newTestAIService(t, config.AIConfig{
		APIKey:    "env-key",
		BaseURL:   "https://env.example/v1",
		Model:     "env-model",
		CipherKey: testAICipherKey,
	})
	ctx := context.Background()

	if _, err := svc.SaveAIProviderConfig(ctx, SaveAIProviderConfigInput{
		BaseURL:   "https://admin.example/v1",
		ModelID:   "admin-model",
		APIKey:    "admin-key",
		IsEnabled: false,
	}); err != nil {
		t.Fatalf("save disabled config: %v", err)
	}

	resolved, err := svc.ResolveRuntimeAIConfig(ctx)
	if err != nil {
		t.Fatalf("resolve runtime config: %v", err)
	}
	if resolved.Source != "env" {
		t.Fatalf("expected env source, got %q", resolved.Source)
	}
	if resolved.APIKey != "env-key" || resolved.BaseURL != "https://env.example/v1" || resolved.ModelID != "env-model" {
		t.Fatalf("unexpected env fallback config: %+v", resolved)
	}
}

func TestResolveRuntimeAIConfigIgnoresBrokenEncryptedConfig(t *testing.T) {
	svc := newTestAIService(t, config.AIConfig{
		APIKey:    "env-key",
		BaseURL:   "https://env.example/v1",
		Model:     "env-model",
		CipherKey: testAICipherKey,
	})
	ctx := context.Background()

	record := AIProviderConfigRecord{
		ID:              "broken-config",
		ProviderKey:     AIProviderKeyDefault,
		BaseURL:         "https://admin.example/v1",
		ModelID:         "admin-model",
		APIKeyEncrypted: "not-base64",
		APIKeyMask:      "adm***1234",
		IsEnabled:       true,
		CreatedAt:       time.Now().UTC(),
		UpdatedAt:       time.Now().UTC(),
	}
	if err := svc.repo.UpsertAIProviderConfig(ctx, record); err != nil {
		t.Fatalf("insert broken config: %v", err)
	}

	resolved, err := svc.ResolveRuntimeAIConfig(ctx)
	if err != nil {
		t.Fatalf("resolve runtime config: %v", err)
	}
	if resolved.Source != "env" {
		t.Fatalf("expected env fallback when admin secret is broken, got %q", resolved.Source)
	}
}

func TestTestAIProviderConfigPersistsSavedHealth(t *testing.T) {
	svc := newTestAIService(t, config.AIConfig{CipherKey: testAICipherKey})
	ctx := context.Background()
	if _, err := svc.SaveAIProviderConfig(ctx, SaveAIProviderConfigInput{
		BaseURL:   "https://provider.example/v1",
		ModelID:   "model-a",
		APIKey:    "secret-key-1",
		IsEnabled: true,
	}); err != nil {
		t.Fatalf("save config: %v", err)
	}

	svc.aiTester = func(ctx context.Context, baseURL, modelID, apiKey string) AIProviderHealthSnapshot {
		return AIProviderHealthSnapshot{
			Status:    AIHealthAvailable,
			Message:   "连接正常",
			LatencyMS: 321,
			CheckedAt: "2026-04-28T10:36:00Z",
		}
	}

	result, err := svc.TestAIProviderConfig(ctx, nil)
	if err != nil {
		t.Fatalf("test saved config: %v", err)
	}
	if result.Status != AIHealthAvailable {
		t.Fatalf("expected available status, got %q", result.Status)
	}

	view, err := svc.GetAIProviderConfigView(ctx)
	if err != nil {
		t.Fatalf("reload view: %v", err)
	}
	if view.Health.Status != AIHealthAvailable || view.Health.LatencyMS != 321 {
		t.Fatalf("expected persisted health snapshot, got %+v", view.Health)
	}
	if view.Health.CheckedAt != "2026-04-28T10:36:00Z" {
		t.Fatalf("unexpected checked_at: %q", view.Health.CheckedAt)
	}
}

func TestTestAIProviderConfigUsesDraftWithoutPersisting(t *testing.T) {
	svc := newTestAIService(t, config.AIConfig{CipherKey: testAICipherKey})
	ctx := context.Background()
	if _, err := svc.SaveAIProviderConfig(ctx, SaveAIProviderConfigInput{
		BaseURL:   "https://provider.example/v1",
		ModelID:   "model-a",
		APIKey:    "secret-key-1",
		IsEnabled: true,
	}); err != nil {
		t.Fatalf("save config: %v", err)
	}

	calls := 0
	svc.aiTester = func(ctx context.Context, baseURL, modelID, apiKey string) AIProviderHealthSnapshot {
		calls++
		if baseURL != "https://draft.example/v1" || modelID != "draft-model" || apiKey != "draft-key" {
			t.Fatalf("draft tester received unexpected config: %s %s %s", baseURL, modelID, apiKey)
		}
		return AIProviderHealthSnapshot{
			Status:    AIHealthInvalidModel,
			Message:   "模型不可用",
			LatencyMS: 654,
			CheckedAt: "2026-04-28T11:00:00Z",
		}
	}

	result, err := svc.TestAIProviderConfig(ctx, &TestAIProviderConfigInput{
		BaseURL:   "https://draft.example/v1/",
		ModelID:   "draft-model",
		APIKey:    "draft-key",
		IsEnabled: true,
	})
	if err != nil {
		t.Fatalf("test draft config: %v", err)
	}
	if calls != 1 {
		t.Fatalf("expected tester to be called once, got %d", calls)
	}
	if result.Status != AIHealthInvalidModel {
		t.Fatalf("expected invalid model result, got %q", result.Status)
	}

	view, err := svc.GetAIProviderConfigView(ctx)
	if err != nil {
		t.Fatalf("reload view: %v", err)
	}
	if view.Health.Status != AIHealthUnknown {
		t.Fatalf("expected saved health to remain unknown, got %q", view.Health.Status)
	}
}

func TestSaveAIProviderConfigRequiresCipherKeyWhenSavingNewAPIKey(t *testing.T) {
	svc := newTestAIService(t, config.AIConfig{})
	ctx := context.Background()

	_, err := svc.SaveAIProviderConfig(ctx, SaveAIProviderConfigInput{
		BaseURL:   "https://provider.example/v1",
		ModelID:   "model-a",
		APIKey:    "secret-key-1",
		IsEnabled: true,
	})
	if !errors.Is(err, ErrAIConfigCipherKeyUnset) {
		t.Fatalf("expected ErrAIConfigCipherKeyUnset, got %v", err)
	}
}
