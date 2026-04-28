package admin

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

const (
	AIProviderKeyDefault  = "default"
	AIHealthUnconfigured  = "unconfigured"
	AIHealthUnknown       = "unknown"
	AIHealthAvailable     = "available"
	AIHealthInvalidAuth   = "invalid_auth"
	AIHealthInvalidModel  = "invalid_model"
	AIHealthTimeout       = "timeout"
	AIHealthNetworkError  = "network_error"
	AIHealthProviderError = "provider_error"
	AIHealthDisabled      = "disabled"
)

type AIProviderConfigRecord struct {
	ID                string `gorm:"primaryKey;size:36"`
	ProviderKey       string `gorm:"size:32;not null;uniqueIndex"`
	BaseURL           string `gorm:"size:255;not null;default:''"`
	ModelID           string `gorm:"size:128;not null;default:''"`
	APIKeyEncrypted   string `gorm:"type:text;not null;default:''"`
	APIKeyMask        string `gorm:"size:64;not null;default:''"`
	IsEnabled         bool   `gorm:"not null;default:true"`
	LastTestStatus    string `gorm:"size:32;not null;default:'unknown'"`
	LastTestMessage   string `gorm:"size:255;not null;default:''"`
	LastTestLatencyMS int    `gorm:"not null;default:0"`
	LastTestAt        *time.Time
	CreatedAt         time.Time `gorm:"not null"`
	UpdatedAt         time.Time `gorm:"not null"`
}

func (AIProviderConfigRecord) TableName() string {
	return "ai_provider_configs"
}

type ResolvedAIConfig struct {
	Source     string
	BaseURL    string
	ModelID    string
	APIKey     string
	Enabled    bool
	Configured bool
}

type AIProviderConfigView struct {
	Effective AIProviderEffectiveView `json:"effective"`
	Config    AIProviderSavedView     `json:"config"`
	Health    AIProviderHealthView    `json:"health"`
}

type AIProviderEffectiveView struct {
	Source     string `json:"source"`
	BaseURL    string `json:"base_url"`
	ModelID    string `json:"model_id"`
	Configured bool   `json:"configured"`
	Enabled    bool   `json:"enabled"`
}

type AIProviderSavedView struct {
	BaseURL    string `json:"base_url"`
	ModelID    string `json:"model_id"`
	APIKeyMask string `json:"api_key_mask"`
	HasAPIKey  bool   `json:"has_api_key"`
	IsEnabled  bool   `json:"is_enabled"`
}

type AIProviderHealthView struct {
	Status    string `json:"status"`
	Message   string `json:"message"`
	LatencyMS int    `json:"latency_ms"`
	CheckedAt string `json:"checked_at"`
}

type SaveAIProviderConfigInput struct {
	BaseURL   string `json:"base_url"`
	ModelID   string `json:"model_id"`
	APIKey    string `json:"api_key"`
	IsEnabled bool   `json:"is_enabled"`
}

type TestAIProviderConfigInput struct {
	BaseURL   string `json:"base_url"`
	ModelID   string `json:"model_id"`
	APIKey    string `json:"api_key"`
	IsEnabled bool   `json:"is_enabled"`
}

type AIProviderHealthSnapshot struct {
	Status    string `json:"status"`
	Message   string `json:"message"`
	LatencyMS int    `json:"latency_ms"`
	CheckedAt string `json:"checked_at"`
}

type aiProviderTester func(ctx context.Context, baseURL, modelID, apiKey string) AIProviderHealthSnapshot

func (r *Repository) GetAIProviderConfig(ctx context.Context) (*AIProviderConfigRecord, error) {
	var row AIProviderConfigRecord
	err := r.db.WithContext(ctx).Where("provider_key = ?", AIProviderKeyDefault).First(&row).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &row, nil
}

func (r *Repository) UpsertAIProviderConfig(ctx context.Context, record AIProviderConfigRecord) error {
	var existing AIProviderConfigRecord
	err := r.db.WithContext(ctx).Where("provider_key = ?", record.ProviderKey).First(&existing).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		now := time.Now().UTC()
		createdAt := record.CreatedAt
		if createdAt.IsZero() {
			createdAt = now
		}
		updatedAt := record.UpdatedAt
		if updatedAt.IsZero() {
			updatedAt = now
		}
		return r.db.WithContext(ctx).Model(&AIProviderConfigRecord{}).Create(map[string]any{
			"id":                   record.ID,
			"provider_key":         strings.TrimSpace(record.ProviderKey),
			"base_url":             strings.TrimSpace(record.BaseURL),
			"model_id":             strings.TrimSpace(record.ModelID),
			"api_key_encrypted":    strings.TrimSpace(record.APIKeyEncrypted),
			"api_key_mask":         strings.TrimSpace(record.APIKeyMask),
			"is_enabled":           record.IsEnabled,
			"last_test_status":     strings.TrimSpace(record.LastTestStatus),
			"last_test_message":    strings.TrimSpace(record.LastTestMessage),
			"last_test_latency_ms": record.LastTestLatencyMS,
			"last_test_at":         record.LastTestAt,
			"created_at":           createdAt,
			"updated_at":           updatedAt,
		}).Error
	}
	if err != nil {
		return err
	}
	updates := map[string]any{
		"base_url":             strings.TrimSpace(record.BaseURL),
		"model_id":             strings.TrimSpace(record.ModelID),
		"api_key_encrypted":    strings.TrimSpace(record.APIKeyEncrypted),
		"api_key_mask":         strings.TrimSpace(record.APIKeyMask),
		"is_enabled":           record.IsEnabled,
		"last_test_status":     strings.TrimSpace(record.LastTestStatus),
		"last_test_message":    strings.TrimSpace(record.LastTestMessage),
		"last_test_latency_ms": record.LastTestLatencyMS,
		"last_test_at":         record.LastTestAt,
		"updated_at":           time.Now().UTC(),
	}
	return r.db.WithContext(ctx).Model(&AIProviderConfigRecord{}).Where("id = ?", existing.ID).Updates(updates).Error
}

func (r *Repository) UpdateAIProviderHealth(ctx context.Context, status AIProviderHealthSnapshot) error {
	updates := map[string]any{
		"last_test_status":     strings.TrimSpace(status.Status),
		"last_test_message":    strings.TrimSpace(status.Message),
		"last_test_latency_ms": status.LatencyMS,
	}
	if strings.TrimSpace(status.CheckedAt) != "" {
		if parsed, err := time.Parse(time.RFC3339, status.CheckedAt); err == nil {
			updates["last_test_at"] = parsed.UTC()
		}
	}
	return r.db.WithContext(ctx).Model(&AIProviderConfigRecord{}).Where("provider_key = ?", AIProviderKeyDefault).Updates(updates).Error
}

func (s *Service) GetAIProviderConfigView(ctx context.Context) (*AIProviderConfigView, error) {
	record, err := s.repo.GetAIProviderConfig(ctx)
	if err != nil {
		return nil, err
	}
	resolved, err := s.ResolveRuntimeAIConfig(ctx)
	if err != nil {
		return nil, err
	}
	view := &AIProviderConfigView{
		Effective: AIProviderEffectiveView{
			Source:     resolved.Source,
			BaseURL:    resolved.BaseURL,
			ModelID:    resolved.ModelID,
			Configured: resolved.Configured,
			Enabled:    resolved.Enabled,
		},
		Health: AIProviderHealthView{
			Status:  AIHealthUnconfigured,
			Message: "未配置",
		},
	}
	if record != nil {
		view.Config = AIProviderSavedView{
			BaseURL:    strings.TrimSpace(record.BaseURL),
			ModelID:    strings.TrimSpace(record.ModelID),
			APIKeyMask: strings.TrimSpace(record.APIKeyMask),
			HasAPIKey:  strings.TrimSpace(record.APIKeyEncrypted) != "",
			IsEnabled:  record.IsEnabled,
		}
		view.Health = toAIProviderHealthView(record)
	} else {
		view.Config = AIProviderSavedView{
			BaseURL:   strings.TrimSpace(s.cfg.EnvAI.BaseURL),
			ModelID:   strings.TrimSpace(s.cfg.EnvAI.Model),
			HasAPIKey: strings.TrimSpace(s.cfg.EnvAI.APIKey) != "",
			IsEnabled: false,
		}
	}
	if resolved.Source == "env" {
		if strings.TrimSpace(s.cfg.EnvAI.APIKey) != "" && strings.TrimSpace(s.cfg.EnvAI.BaseURL) != "" && strings.TrimSpace(s.cfg.EnvAI.Model) != "" {
			view.Health = AIProviderHealthView{Status: AIHealthUnknown, Message: "当前使用环境变量"}
		}
	}
	if resolved.Source == "none" {
		if record != nil && !record.IsEnabled {
			view.Health = AIProviderHealthView{Status: AIHealthDisabled, Message: "后台配置已禁用"}
		} else {
			view.Health = AIProviderHealthView{Status: AIHealthUnconfigured, Message: "请先配置 base URL、API Key 和模型"}
		}
	}
	return view, nil
}

func (s *Service) SaveAIProviderConfig(ctx context.Context, input SaveAIProviderConfigInput) (*AIProviderConfigView, error) {
	baseURL := normalizeAIBaseURL(input.BaseURL)
	modelID := strings.TrimSpace(input.ModelID)
	apiKey := strings.TrimSpace(input.APIKey)
	if baseURL == "" || modelID == "" {
		return nil, ErrAIConfigInvalid
	}
	if _, err := url.ParseRequestURI(baseURL); err != nil {
		return nil, ErrAIConfigInvalid
	}
	record, err := s.repo.GetAIProviderConfig(ctx)
	if err != nil {
		return nil, err
	}
	if record == nil {
		record = &AIProviderConfigRecord{
			ID:             uuid.NewString(),
			ProviderKey:    AIProviderKeyDefault,
			LastTestStatus: AIHealthUnknown,
		}
	}
	if apiKey == "" && strings.TrimSpace(record.APIKeyEncrypted) == "" {
		return nil, ErrAIConfigInvalid
	}
	if apiKey != "" {
		cipherKey := strings.TrimSpace(s.cfg.EnvAI.CipherKey)
		if len(cipherKey) != 32 {
			return nil, ErrAIConfigCipherKeyUnset
		}
		encrypted, err := encryptAISecret(cipherKey, apiKey)
		if err != nil {
			return nil, ErrAIConfigCipherKeyUnset
		}
		record.APIKeyEncrypted = encrypted
		record.APIKeyMask = maskAIKey(apiKey)
	}
	record.BaseURL = baseURL
	record.ModelID = modelID
	record.IsEnabled = input.IsEnabled
	record.LastTestStatus = AIHealthUnknown
	record.LastTestMessage = "保存后请重新测试连接"
	record.LastTestLatencyMS = 0
	record.LastTestAt = nil
	if err := s.repo.UpsertAIProviderConfig(ctx, *record); err != nil {
		return nil, err
	}
	return s.GetAIProviderConfigView(ctx)
}

func (s *Service) ResolveRuntimeAIConfig(ctx context.Context) (*ResolvedAIConfig, error) {
	record, err := s.repo.GetAIProviderConfig(ctx)
	if err != nil {
		return nil, err
	}
	if record != nil && record.IsEnabled {
		apiKey, err := decryptAISecret(strings.TrimSpace(s.cfg.EnvAI.CipherKey), strings.TrimSpace(record.APIKeyEncrypted))
		if err == nil {
			baseURL := normalizeAIBaseURL(record.BaseURL)
			modelID := strings.TrimSpace(record.ModelID)
			if baseURL != "" && modelID != "" && strings.TrimSpace(apiKey) != "" {
				return &ResolvedAIConfig{Source: "admin", BaseURL: baseURL, ModelID: modelID, APIKey: apiKey, Enabled: true, Configured: true}, nil
			}
		}
	}
	baseURL := normalizeAIBaseURL(s.cfg.EnvAI.BaseURL)
	modelID := strings.TrimSpace(s.cfg.EnvAI.Model)
	apiKey := strings.TrimSpace(s.cfg.EnvAI.APIKey)
	if baseURL != "" && modelID != "" && apiKey != "" {
		return &ResolvedAIConfig{Source: "env", BaseURL: baseURL, ModelID: modelID, APIKey: apiKey, Enabled: true, Configured: true}, nil
	}
	return &ResolvedAIConfig{Source: "none", BaseURL: baseURL, ModelID: modelID, APIKey: apiKey, Enabled: false, Configured: false}, nil
}

func (s *Service) TestAIProviderConfig(ctx context.Context, input *TestAIProviderConfigInput) (*AIProviderHealthSnapshot, error) {
	resolved, persistResult, err := s.resolveTestConfig(ctx, input)
	if err != nil {
		return nil, err
	}
	if !resolved.Configured || !resolved.Enabled {
		snapshot := &AIProviderHealthSnapshot{Status: AIHealthUnconfigured, Message: "请先配置 base URL、API Key 和模型", CheckedAt: time.Now().UTC().Format(time.RFC3339)}
		return snapshot, nil
	}
	tester := s.aiTester
	if tester == nil {
		tester = defaultAIProviderTester
	}
	result := tester(ctx, resolved.BaseURL, resolved.ModelID, resolved.APIKey)
	if persistResult {
		_ = s.repo.UpdateAIProviderHealth(ctx, result)
	}
	return &result, nil
}

func (s *Service) resolveTestConfig(ctx context.Context, input *TestAIProviderConfigInput) (*ResolvedAIConfig, bool, error) {
	if input == nil {
		resolved, err := s.ResolveRuntimeAIConfig(ctx)
		return resolved, true, err
	}
	baseURL := normalizeAIBaseURL(input.BaseURL)
	modelID := strings.TrimSpace(input.ModelID)
	apiKey := strings.TrimSpace(input.APIKey)
	if baseURL == "" || modelID == "" {
		return &ResolvedAIConfig{Source: "none", Enabled: false, Configured: false}, false, nil
	}
	if apiKey == "" {
		record, err := s.repo.GetAIProviderConfig(ctx)
		if err != nil {
			return nil, false, err
		}
		if record != nil && normalizeAIBaseURL(record.BaseURL) == baseURL && strings.TrimSpace(record.ModelID) == modelID {
			decrypted, err := decryptAISecret(strings.TrimSpace(s.cfg.EnvAI.CipherKey), strings.TrimSpace(record.APIKeyEncrypted))
			if err != nil {
				return nil, false, err
			}
			apiKey = decrypted
		}
	}
	return &ResolvedAIConfig{Source: "draft", BaseURL: baseURL, ModelID: modelID, APIKey: apiKey, Enabled: input.IsEnabled, Configured: baseURL != "" && modelID != "" && strings.TrimSpace(apiKey) != ""}, false, nil
}

func toAIProviderHealthView(record *AIProviderConfigRecord) AIProviderHealthView {
	if record == nil {
		return AIProviderHealthView{Status: AIHealthUnconfigured, Message: "未配置"}
	}
	checkedAt := ""
	if record.LastTestAt != nil {
		checkedAt = record.LastTestAt.UTC().Format(time.RFC3339)
	}
	status := strings.TrimSpace(record.LastTestStatus)
	if status == "" {
		status = AIHealthUnknown
	}
	return AIProviderHealthView{
		Status:    status,
		Message:   strings.TrimSpace(record.LastTestMessage),
		LatencyMS: record.LastTestLatencyMS,
		CheckedAt: checkedAt,
	}
}

func defaultAIProviderTester(ctx context.Context, baseURL, modelID, apiKey string) AIProviderHealthSnapshot {
	started := time.Now()
	snapshot := AIProviderHealthSnapshot{Status: AIHealthProviderError, Message: "AI 服务异常", CheckedAt: started.UTC().Format(time.RFC3339)}
	requestBody := map[string]any{
		"model":       modelID,
		"messages":    []map[string]string{{"role": "user", "content": "ping"}},
		"temperature": 0,
		"max_tokens":  1,
	}
	encoded, err := json.Marshal(requestBody)
	if err != nil {
		snapshot.Message = "测试请求序列化失败"
		return snapshot
	}
	requestCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	endpoint := normalizeAIBaseURL(baseURL) + "/chat/completions"
	req, err := http.NewRequestWithContext(requestCtx, http.MethodPost, endpoint, strings.NewReader(string(encoded)))
	if err != nil {
		snapshot.Message = "测试请求创建失败"
		return snapshot
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+strings.TrimSpace(apiKey))
	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	snapshot.LatencyMS = int(time.Since(started).Milliseconds())
	if err != nil {
		snapshot.Status, snapshot.Message = classifyAIProviderError(err)
		return snapshot
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		snapshot.Status = AIHealthInvalidAuth
		snapshot.Message = "鉴权失败，请检查 API Key"
		return snapshot
	}
	if resp.StatusCode == http.StatusNotFound {
		snapshot.Status = AIHealthInvalidModel
		snapshot.Message = "模型或接口地址不可用"
		return snapshot
	}
	if resp.StatusCode >= 400 {
		message := strings.TrimSpace(extractAIErrorMessage(body))
		if message == "" {
			message = fmt.Sprintf("服务返回 HTTP %d", resp.StatusCode)
		}
		if strings.Contains(strings.ToLower(message), "model") && strings.Contains(strings.ToLower(message), "not") {
			snapshot.Status = AIHealthInvalidModel
			snapshot.Message = "模型不可用，请检查 model_id"
			return snapshot
		}
		snapshot.Status = AIHealthProviderError
		snapshot.Message = message
		return snapshot
	}
	var parsed struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		snapshot.Status = AIHealthProviderError
		snapshot.Message = "返回格式无法解析"
		return snapshot
	}
	if len(parsed.Choices) == 0 {
		snapshot.Status = AIHealthProviderError
		snapshot.Message = "模型未返回有效结果"
		return snapshot
	}
	snapshot.Status = AIHealthAvailable
	snapshot.Message = "连接正常"
	return snapshot
}

func normalizeAIBaseURL(raw string) string {
	return strings.TrimRight(strings.TrimSpace(raw), "/")
}

func maskAIKey(raw string) string {
	text := strings.TrimSpace(raw)
	if text == "" {
		return ""
	}
	runes := []rune(text)
	if len(runes) <= 4 {
		return "***"
	}
	prefix := ""
	if len(runes) >= 3 {
		prefix = string(runes[:3])
	}
	suffix := string(runes[len(runes)-4:])
	return prefix + "***" + suffix
}

func encryptAISecret(cipherKey, plaintext string) (string, error) {
	key := strings.TrimSpace(cipherKey)
	if len(key) != 32 {
		return "", fmt.Errorf("ai config cipher key must be 32 bytes")
	}
	block, err := aes.NewCipher([]byte(key))
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return "", err
	}
	ciphertext := gcm.Seal(nonce, nonce, []byte(plaintext), nil)
	return base64.StdEncoding.EncodeToString(ciphertext), nil
}

func decryptAISecret(cipherKey, encoded string) (string, error) {
	if strings.TrimSpace(encoded) == "" {
		return "", nil
	}
	key := strings.TrimSpace(cipherKey)
	if len(key) != 32 {
		return "", fmt.Errorf("ai config cipher key must be 32 bytes")
	}
	raw, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return "", err
	}
	block, err := aes.NewCipher([]byte(key))
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	if len(raw) < gcm.NonceSize() {
		return "", fmt.Errorf("invalid encrypted ai key")
	}
	nonce := raw[:gcm.NonceSize()]
	ciphertext := raw[gcm.NonceSize():]
	plain, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return "", err
	}
	return string(plain), nil
}

func classifyAIProviderError(err error) (string, string) {
	if err == nil {
		return AIHealthProviderError, "AI 服务异常"
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return AIHealthTimeout, "请求超时"
	}
	var netErr net.Error
	if errors.As(err, &netErr) {
		if netErr.Timeout() {
			return AIHealthTimeout, "请求超时"
		}
		return AIHealthNetworkError, "网络异常"
	}
	return AIHealthNetworkError, "网络异常"
}

func extractAIErrorMessage(body []byte) string {
	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		return strings.TrimSpace(string(body))
	}
	if errObj, ok := payload["error"].(map[string]any); ok {
		if message, ok := errObj["message"].(string); ok {
			return strings.TrimSpace(message)
		}
	}
	if detail, ok := payload["detail"].(string); ok {
		return strings.TrimSpace(detail)
	}
	return strings.TrimSpace(string(body))
}
