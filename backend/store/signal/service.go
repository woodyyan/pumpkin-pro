package signal

import (
	"bytes"
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
)

const (
	defaultWebhookTimeoutMS   = 3000
	defaultCooldownSeconds    = 300
	defaultDispatchBatchSize  = 30
	defaultDispatcherInterval = 2 * time.Second
	defaultMaxAttempts        = 4
)

type ServiceConfig struct {
	SecretKey          string
	DispatcherInterval time.Duration
	MaxAttempts        int
}

type Service struct {
	repo               *Repository
	secretKey          [32]byte
	dispatcherInterval time.Duration
	maxAttempts        int
	retryBackoffs      []time.Duration
}

func NewService(repo *Repository, cfg ServiceConfig) *Service {
	secretRaw := strings.TrimSpace(cfg.SecretKey)
	if secretRaw == "" {
		secretRaw = "pumpkin-signal-default-key"
	}
	hash := sha256.Sum256([]byte(secretRaw))

	interval := cfg.DispatcherInterval
	if interval <= 0 {
		interval = defaultDispatcherInterval
	}
	maxAttempts := cfg.MaxAttempts
	if maxAttempts <= 0 {
		maxAttempts = defaultMaxAttempts
	}

	return &Service{
		repo:               repo,
		secretKey:          hash,
		dispatcherInterval: interval,
		maxAttempts:        maxAttempts,
		retryBackoffs:      []time.Duration{time.Minute, 5 * time.Minute, 15 * time.Minute},
	}
}

func (s *Service) StartDispatcher(ctx context.Context) {
	ticker := time.NewTicker(s.dispatcherInterval)
	go func() {
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				_, _ = s.DispatchDueDeliveries(context.Background(), defaultDispatchBatchSize)
			}
		}
	}()
}

func (s *Service) GetWebhookEndpoint(ctx context.Context, userID string) (*WebhookEndpoint, error) {
	if strings.TrimSpace(userID) == "" {
		return nil, ErrForbidden
	}
	record, err := s.repo.GetWebhookEndpoint(ctx, userID)
	if err != nil {
		if err == ErrNotFound {
			return nil, nil
		}
		return nil, err
	}
	return toWebhookEndpoint(record, strings.TrimSpace(record.SecretCipherText) != ""), nil
}

func (s *Service) UpsertWebhookEndpoint(ctx context.Context, userID string, input WebhookConfigInput) (*WebhookEndpoint, error) {
	if strings.TrimSpace(userID) == "" {
		return nil, ErrForbidden
	}

	existing, err := s.repo.GetWebhookEndpoint(ctx, userID)
	if err != nil && err != ErrNotFound {
		return nil, err
	}
	if err == ErrNotFound {
		existing = nil
	}

	rawURL := strings.TrimSpace(input.URL)
	if rawURL == "" && existing != nil {
		rawURL = existing.URL
	}
	if rawURL == "" {
		return nil, fmt.Errorf("%w: webhook url 不能为空", ErrInvalidInput)
	}
	normalizedURL, err := validateWebhookURL(rawURL)
	if err != nil {
		return nil, err
	}

	timeoutMS := input.TimeoutMS
	if timeoutMS <= 0 {
		if existing != nil && existing.TimeoutMS > 0 {
			timeoutMS = existing.TimeoutMS
		} else {
			timeoutMS = defaultWebhookTimeoutMS
		}
	}
	if timeoutMS < 1000 || timeoutMS > 10000 {
		return nil, fmt.Errorf("%w: timeout_ms 必须在 1000~10000 之间", ErrInvalidInput)
	}

	isEnabled := true
	if existing != nil {
		isEnabled = existing.IsEnabled
	}
	if input.IsEnabled != nil {
		isEnabled = *input.IsEnabled
	}

	secretCipherText := ""
	if existing != nil {
		secretCipherText = existing.SecretCipherText
	}
	newSecret := strings.TrimSpace(input.Secret)
	if newSecret != "" {
		secretCipherText, err = s.encryptSecret(newSecret)
		if err != nil {
			return nil, fmt.Errorf("encrypt webhook secret failed: %w", err)
		}
	}

	now := time.Now().UTC()
	record := WebhookEndpointRecord{
		ID:               uuid.NewString(),
		UserID:           strings.TrimSpace(userID),
		URL:              normalizedURL,
		SecretCipherText: secretCipherText,
		IsEnabled:        isEnabled,
		TimeoutMS:        timeoutMS,
		CreatedAt:        now,
		UpdatedAt:        now,
	}
	if existing != nil {
		record.ID = existing.ID
		record.CreatedAt = existing.CreatedAt
	}

	saved, err := s.repo.SaveWebhookEndpoint(ctx, record)
	if err != nil {
		return nil, err
	}
	return toWebhookEndpoint(saved, strings.TrimSpace(saved.SecretCipherText) != ""), nil
}

func (s *Service) ListSymbolConfigs(ctx context.Context, userID string) ([]*SymbolSignalConfig, error) {
	if strings.TrimSpace(userID) == "" {
		return nil, ErrForbidden
	}
	records, err := s.repo.ListSymbolConfigs(ctx, userID)
	if err != nil {
		return nil, err
	}
	items := make([]*SymbolSignalConfig, 0, len(records))
	for _, record := range records {
		item, err := toSymbolSignalConfig(record)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, nil
}

func (s *Service) UpsertSymbolConfig(ctx context.Context, userID, symbol string, input SymbolSignalConfigInput) (*SymbolSignalConfig, error) {
	if strings.TrimSpace(userID) == "" {
		return nil, ErrForbidden
	}
	normalizedSymbol, err := normalizeHKSymbol(symbol)
	if err != nil {
		return nil, err
	}

	existing, err := s.repo.GetSymbolConfig(ctx, userID, normalizedSymbol)
	if err != nil && err != ErrNotFound {
		return nil, err
	}
	if err == ErrNotFound {
		existing = nil
	}

	strategyID := strings.TrimSpace(input.StrategyID)
	if strategyID == "" && existing != nil {
		strategyID = existing.StrategyID
	}

	isEnabled := false
	if existing != nil {
		isEnabled = existing.IsEnabled
	}
	if input.IsEnabled != nil {
		isEnabled = *input.IsEnabled
	}
	if isEnabled && strategyID == "" {
		return nil, fmt.Errorf("%w: 启用信号时 strategy_id 不能为空", ErrInvalidInput)
	}

	cooldown := input.CooldownSeconds
	if cooldown <= 0 {
		if existing != nil && existing.CooldownSeconds > 0 {
			cooldown = existing.CooldownSeconds
		} else {
			cooldown = defaultCooldownSeconds
		}
	}
	if cooldown < 10 || cooldown > 3600 {
		return nil, fmt.Errorf("%w: cooldown_seconds 必须在 10~3600 之间", ErrInvalidInput)
	}

	thresholds := input.Thresholds
	if thresholds == nil {
		if existing != nil {
			restored := map[string]any{}
			if decodeErr := decodeJSONMap(existing.ThresholdsJSON, &restored); decodeErr == nil {
				thresholds = restored
			}
		}
	}
	thresholdsJSON, err := encodeJSONMap(thresholds)
	if err != nil {
		return nil, fmt.Errorf("%w: thresholds 字段格式错误", ErrInvalidInput)
	}

	now := time.Now().UTC()
	record := SymbolSignalConfigRecord{
		ID:              uuid.NewString(),
		UserID:          strings.TrimSpace(userID),
		Symbol:          normalizedSymbol,
		StrategyID:      strategyID,
		IsEnabled:       isEnabled,
		CooldownSeconds: cooldown,
		ThresholdsJSON:  thresholdsJSON,
		CreatedAt:       now,
		UpdatedAt:       now,
	}
	if existing != nil {
		record.ID = existing.ID
		record.CreatedAt = existing.CreatedAt
	}

	saved, err := s.repo.SaveSymbolConfig(ctx, record)
	if err != nil {
		return nil, err
	}
	return toSymbolSignalConfig(*saved)
}

func (s *Service) DeleteSymbolConfig(ctx context.Context, userID, symbol string) error {
	if strings.TrimSpace(userID) == "" {
		return ErrForbidden
	}
	normalizedSymbol, err := normalizeHKSymbol(symbol)
	if err != nil {
		return err
	}
	return s.repo.DeleteSymbolConfig(ctx, userID, normalizedSymbol)
}

func (s *Service) CountSymbolConfigRefsByStrategy(ctx context.Context, userID, strategyID string) (int64, error) {
	if strings.TrimSpace(userID) == "" {
		return 0, ErrForbidden
	}
	strategyID = strings.TrimSpace(strategyID)
	if strategyID == "" {
		return 0, fmt.Errorf("%w: strategy_id 不能为空", ErrInvalidInput)
	}
	return s.repo.CountSymbolConfigsByStrategy(ctx, strings.TrimSpace(userID), strategyID)
}

func (s *Service) EmitSignal(ctx context.Context, input EmitSignalInput) (*SignalEvent, error) {
	userID := strings.TrimSpace(input.UserID)
	if userID == "" {
		return nil, ErrForbidden
	}

	endpoint, err := s.repo.GetWebhookEndpoint(ctx, userID)
	if err != nil {
		if err == ErrNotFound {
			return nil, ErrWebhookMissing
		}
		return nil, err
	}
	if endpoint == nil {
		return nil, ErrWebhookMissing
	}
	if !endpoint.IsEnabled {
		return nil, ErrWebhookOff
	}

	normalizedSymbol, err := normalizeHKSymbol(input.Symbol)
	if err != nil {
		return nil, err
	}
	normalizedSide, err := normalizeSide(input.Side)
	if err != nil {
		return nil, err
	}

	eventTime := input.EventTime.UTC()
	if eventTime.IsZero() {
		eventTime = time.Now().UTC()
	}
	reasonJSON, err := encodeJSONMap(input.Reason)
	if err != nil {
		return nil, fmt.Errorf("%w: reason 字段格式错误", ErrInvalidInput)
	}

	eventID := "sig_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	fingerprint := buildFingerprint(userID, normalizedSymbol, strings.TrimSpace(input.StrategyID), normalizedSide, eventTime, input.IsTest, eventID)

	now := time.Now().UTC()
	eventRecord := SignalEventRecord{
		ID:          uuid.NewString(),
		EventID:     eventID,
		UserID:      userID,
		Symbol:      normalizedSymbol,
		StrategyID:  strings.TrimSpace(input.StrategyID),
		Side:        normalizedSide,
		SignalScore: input.SignalScore,
		ReasonJSON:  reasonJSON,
		Fingerprint: fingerprint,
		IsTest:      input.IsTest,
		EventTime:   eventTime,
		CreatedAt:   now,
	}
	deliveryRecord := WebhookDeliveryRecord{
		ID:          uuid.NewString(),
		EventID:     eventID,
		UserID:      userID,
		EndpointID:  endpoint.ID,
		AttemptNo:   1,
		Status:      "pending",
		NextRetryAt: ptrTime(now),
		CreatedAt:   now,
		UpdatedAt:   now,
	}

	if err := s.repo.CreateEventWithDelivery(ctx, eventRecord, deliveryRecord); err != nil {
		return nil, err
	}
	return toSignalEvent(eventRecord)
}

func (s *Service) SendTestSignal(ctx context.Context, userID string, input TestSignalInput) (*DispatchResult, error) {
	userID = strings.TrimSpace(userID)
	if userID == "" {
		return nil, ErrForbidden
	}

	normalizedSymbol, err := normalizeHKSymbol(input.Symbol)
	if err != nil {
		return nil, err
	}
	config, configErr := s.repo.GetSymbolConfig(ctx, userID, normalizedSymbol)
	if configErr != nil && configErr != ErrNotFound {
		return nil, configErr
	}
	strategyID := ""
	if configErr == nil && config != nil {
		strategyID = config.StrategyID
	}

	event, err := s.EmitSignal(ctx, EmitSignalInput{
		UserID:      userID,
		Symbol:      normalizedSymbol,
		StrategyID:  strategyID,
		Side:        input.Side,
		SignalScore: 0,
		Reason: map[string]any{
			"kind":    "webhook_test",
			"message": "这是一条测试信号，用于验证 webhook 可达性与签名。",
		},
		EventTime: time.Now().UTC(),
		IsTest:    true,
	})
	if err != nil {
		return nil, err
	}

	deliveryRecord, err := s.repo.GetLatestDeliveryByEventID(ctx, event.EventID)
	if err != nil {
		return nil, fmt.Errorf("%w: 测试事件已创建，但未找到投递记录", ErrWebhookDeliveryUndelivered)
	}

	deliveryRecord, err = s.dispatchDeliveryNow(ctx, deliveryRecord.ID)
	if err != nil {
		return nil, err
	}

	delivery := toWebhookDelivery(*deliveryRecord, event.Symbol)
	if !strings.EqualFold(strings.TrimSpace(deliveryRecord.Status), "delivered") {
		return nil, fmt.Errorf("%w: %s", ErrWebhookDeliveryUndelivered, formatWebhookDeliveryIssue(delivery))
	}
	return &DispatchResult{Event: event, Delivery: delivery}, nil
}

func (s *Service) dispatchDeliveryNow(ctx context.Context, deliveryID string) (*WebhookDeliveryRecord, error) {
	record, err := s.repo.GetDeliveryByID(ctx, deliveryID)
	if err != nil {
		return nil, err
	}
	if record == nil {
		return nil, ErrNotFound
	}

	status := strings.ToLower(strings.TrimSpace(record.Status))
	if status == "pending" || status == "retrying" {
		ok, claimErr := s.repo.ClaimDelivery(ctx, record.ID, time.Now().UTC())
		if claimErr != nil {
			return nil, claimErr
		}
		if ok {
			s.processDelivery(ctx, record.ID)
		}
	}

	return s.waitForDeliveryResult(ctx, record.ID, 12*time.Second)
}

func (s *Service) waitForDeliveryResult(ctx context.Context, deliveryID string, timeout time.Duration) (*WebhookDeliveryRecord, error) {
	deadline := time.Now().Add(timeout)
	for {
		record, err := s.repo.GetDeliveryByID(ctx, deliveryID)
		if err != nil {
			return nil, err
		}
		if record == nil {
			return nil, ErrNotFound
		}

		status := strings.ToLower(strings.TrimSpace(record.Status))
		if status != "pending" && status != "processing" {
			return record, nil
		}
		if time.Now().After(deadline) {
			return record, nil
		}

		select {
		case <-ctx.Done():
			return record, ctx.Err()
		case <-time.After(150 * time.Millisecond):
		}
	}
}

func formatWebhookDeliveryIssue(delivery *WebhookDelivery) string {
	if delivery == nil {
		return "Webhook 未送达，且未找到投递结果"
	}

	parts := []string{fmt.Sprintf("Webhook 未送达，当前状态：%s", formatWebhookDeliveryStatusText(delivery.Status))}
	if delivery.HTTPStatus > 0 {
		parts = append(parts, fmt.Sprintf("HTTP %d", delivery.HTTPStatus))
	}
	if delivery.LatencyMS > 0 {
		parts = append(parts, fmt.Sprintf("耗时 %dms", delivery.LatencyMS))
	}
	if text := strings.TrimSpace(delivery.ErrorMessage); text != "" {
		parts = append(parts, fmt.Sprintf("原因：%s", text))
	}
	return strings.Join(parts, "；")
}

func formatWebhookDeliveryStatusText(status string) string {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "pending":
		return "待发送"
	case "processing":
		return "发送中"
	case "retrying":
		return "重试中"
	case "delivered":
		return "已送达"
	case "failed":
		return "已失败"
	default:
		if strings.TrimSpace(status) == "" {
			return "未知"
		}
		return status
	}
}

func (s *Service) ListSignalEvents(ctx context.Context, userID, symbol string, limit int) ([]*SignalEvent, error) {
	userID = strings.TrimSpace(userID)
	if userID == "" {
		return nil, ErrForbidden
	}
	if strings.TrimSpace(symbol) != "" {
		normalized, err := normalizeHKSymbol(symbol)
		if err != nil {
			return nil, err
		}
		symbol = normalized
	}
	records, err := s.repo.ListSignalEvents(ctx, userID, symbol, limit)
	if err != nil {
		return nil, err
	}
	items := make([]*SignalEvent, 0, len(records))
	for _, record := range records {
		item, convErr := toSignalEvent(record)
		if convErr != nil {
			return nil, convErr
		}
		items = append(items, item)
	}
	return items, nil
}

func (s *Service) ListDeliveries(ctx context.Context, userID, symbol string, limit int) ([]*WebhookDelivery, error) {
	userID = strings.TrimSpace(userID)
	if userID == "" {
		return nil, ErrForbidden
	}
	if strings.TrimSpace(symbol) != "" {
		normalized, err := normalizeHKSymbol(symbol)
		if err != nil {
			return nil, err
		}
		symbol = normalized
	}
	records, err := s.repo.ListDeliveries(ctx, userID, symbol, limit)
	if err != nil {
		return nil, err
	}
	items := make([]*WebhookDelivery, 0, len(records))
	eventCache := map[string]*SignalEventRecord{}
	for _, record := range records {
		eventRecord, ok := eventCache[record.EventID]
		if !ok {
			event, eventErr := s.repo.GetSignalEventByEventID(ctx, record.EventID)
			if eventErr == nil {
				eventRecord = event
				eventCache[record.EventID] = event
			}
		}
		symbolValue := ""
		if eventRecord != nil {
			symbolValue = eventRecord.Symbol
		}
		items = append(items, toWebhookDelivery(record, symbolValue))
	}
	return items, nil
}

func (s *Service) GetLatestDelivery(ctx context.Context, userID string) (*WebhookDelivery, error) {
	userID = strings.TrimSpace(userID)
	if userID == "" {
		return nil, ErrForbidden
	}
	record, err := s.repo.GetLatestDelivery(ctx, userID)
	if err != nil {
		if err == ErrNotFound {
			return nil, nil
		}
		return nil, err
	}
	event, eventErr := s.repo.GetSignalEventByEventID(ctx, record.EventID)
	symbol := ""
	if eventErr == nil && event != nil {
		symbol = event.Symbol
	}
	return toWebhookDelivery(*record, symbol), nil
}

func (s *Service) DispatchDueDeliveries(ctx context.Context, limit int) (int, error) {
	now := time.Now().UTC()
	due, err := s.repo.ListDueDeliveries(ctx, now, limit)
	if err != nil {
		return 0, err
	}
	processed := 0
	for _, delivery := range due {
		ok, claimErr := s.repo.ClaimDelivery(ctx, delivery.ID, now)
		if claimErr != nil {
			continue
		}
		if !ok {
			continue
		}
		processed++
		s.processDelivery(ctx, delivery.ID)
	}
	return processed, nil
}

func (s *Service) processDelivery(ctx context.Context, deliveryID string) {
	attemptedAt := time.Now().UTC()
	record, err := s.repo.GetDeliveryByID(ctx, deliveryID)
	if err != nil || record == nil {
		return
	}

	event, err := s.repo.GetSignalEventByEventID(ctx, record.EventID)
	if err != nil {
		_ = s.repo.MarkDeliveryFailed(ctx, record.ID, 0, 0, "signal event not found", attemptedAt)
		return
	}
	endpoint, err := s.repo.GetWebhookEndpoint(ctx, record.UserID)
	if err != nil {
		_ = s.repo.MarkDeliveryFailed(ctx, record.ID, 0, 0, "webhook endpoint not found", attemptedAt)
		return
	}
	if !endpoint.IsEnabled {
		_ = s.repo.MarkDeliveryFailed(ctx, record.ID, 0, 0, "webhook endpoint is disabled", attemptedAt)
		return
	}

	secret := ""
	if strings.TrimSpace(endpoint.SecretCipherText) != "" {
		secret, err = s.decryptSecret(endpoint.SecretCipherText)
		if err != nil {
			_ = s.repo.MarkDeliveryFailed(ctx, record.ID, 0, 0, "webhook secret invalid", attemptedAt)
			return
		}
	}

	payload, err := s.buildWebhookPayload(*event)
	if err != nil {
		_ = s.repo.MarkDeliveryFailed(ctx, record.ID, 0, 0, "build payload failed", attemptedAt)
		return
	}

	bodyBytes, err := json.Marshal(payload)
	if err != nil {
		_ = s.repo.MarkDeliveryFailed(ctx, record.ID, 0, 0, "encode payload failed", attemptedAt)
		return
	}

	timestamp := strconv.FormatInt(attemptedAt.Unix(), 10)
	signature := ""
	if strings.TrimSpace(secret) != "" {
		signature = s.signPayload(timestamp, bodyBytes, secret)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint.URL, bytes.NewReader(bodyBytes))
	if err != nil {
		_ = s.scheduleRetryOrFail(ctx, record, attemptedAt, 0, 0, err.Error())
		return
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Pumpkin-Event-Id", event.EventID)
	req.Header.Set("X-Pumpkin-Timestamp", timestamp)
	if signature != "" {
		req.Header.Set("X-Pumpkin-Signature", signature)
	}

	timeout := endpoint.TimeoutMS
	if timeout <= 0 {
		timeout = defaultWebhookTimeoutMS
	}
	client := &http.Client{Timeout: time.Duration(timeout) * time.Millisecond}

	start := time.Now()
	resp, err := client.Do(req)
	latencyMS := time.Since(start).Milliseconds()
	if err != nil {
		_ = s.scheduleRetryOrFail(ctx, record, attemptedAt, 0, latencyMS, err.Error())
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		_ = s.repo.MarkDeliveryDelivered(ctx, record.ID, resp.StatusCode, latencyMS, attemptedAt)
		return
	}

	snippet := readResponseSnippet(resp.Body)
	errMsg := fmt.Sprintf("webhook returned %d", resp.StatusCode)
	if snippet != "" {
		errMsg = fmt.Sprintf("webhook returned %d: %s", resp.StatusCode, snippet)
	}
	_ = s.scheduleRetryOrFail(ctx, record, attemptedAt, resp.StatusCode, latencyMS, errMsg)
}

func (s *Service) scheduleRetryOrFail(ctx context.Context, record *WebhookDeliveryRecord, attemptedAt time.Time, httpStatus int, latencyMS int64, errMsg string) error {
	if record == nil {
		return nil
	}
	if record.AttemptNo >= s.maxAttempts {
		return s.repo.MarkDeliveryFailed(ctx, record.ID, httpStatus, latencyMS, errMsg, attemptedAt)
	}
	nextAttempt := record.AttemptNo + 1
	nextRetryAt := attemptedAt.Add(s.nextBackoff(record.AttemptNo))
	return s.repo.MarkDeliveryRetry(ctx, record.ID, nextAttempt, nextRetryAt, httpStatus, latencyMS, errMsg, attemptedAt)
}

func (s *Service) nextBackoff(currentAttempt int) time.Duration {
	if currentAttempt <= 0 {
		return time.Minute
	}
	index := currentAttempt - 1
	if index < 0 {
		index = 0
	}
	if index >= len(s.retryBackoffs) {
		return s.retryBackoffs[len(s.retryBackoffs)-1]
	}
	return s.retryBackoffs[index]
}

func (s *Service) buildWebhookPayload(event SignalEventRecord) (map[string]any, error) {
	reason := map[string]any{}
	if err := decodeJSONMap(event.ReasonJSON, &reason); err != nil {
		return nil, err
	}
	return map[string]any{
		"msgtype": "text",
		"text": map[string]any{
			"content": buildWebhookTextContent(event, reason),
		},
	}, nil
}

func buildWebhookTextContent(event SignalEventRecord, reason map[string]any) string {
	lines := []string{"股票交易信号来啦！"}
	if event.IsTest {
		lines = append(lines, "类型：测试信号")
	} else {
		lines = append(lines, "类型：正式信号")
	}

	symbol := strings.TrimSpace(event.Symbol)
	if symbol == "" {
		symbol = "00700.HK"
	}
	lines = append(lines,
		fmt.Sprintf("股票：%s", symbol),
		fmt.Sprintf("方向：%s", strings.ToUpper(strings.TrimSpace(event.Side))),
		fmt.Sprintf("时间：%s", event.EventTime.Local().Format("2006-01-02 15:04:05")),
	)

	if strategyID := strings.TrimSpace(event.StrategyID); strategyID != "" {
		lines = append(lines, fmt.Sprintf("策略：%s", strategyID))
	}
	if event.SignalScore != 0 {
		lines = append(lines, fmt.Sprintf("评分：%.2f", event.SignalScore))
	}
	if reasonText := summarizeWebhookReason(reason); reasonText != "" {
		lines = append(lines, fmt.Sprintf("原因：%s", reasonText))
	}
	return strings.Join(lines, "\n")
}

func summarizeWebhookReason(reason map[string]any) string {
	if len(reason) == 0 {
		return ""
	}
	if message := strings.TrimSpace(stringifyWebhookValue(reason["message"])); message != "" {
		return message
	}
	if kind := strings.TrimSpace(stringifyWebhookValue(reason["kind"])); kind != "" {
		return kind
	}
	raw, err := json.Marshal(reason)
	if err != nil {
		return ""
	}
	return string(raw)
}

func stringifyWebhookValue(value any) string {
	switch typed := value.(type) {
	case string:
		return typed
	case fmt.Stringer:
		return typed.String()
	case nil:
		return ""
	default:
		return fmt.Sprint(typed)
	}
}

func (s *Service) signPayload(timestamp string, body []byte, secret string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte(timestamp))
	_, _ = mac.Write([]byte("."))
	_, _ = mac.Write(body)
	sum := mac.Sum(nil)
	return "sha256=" + hex.EncodeToString(sum)
}

func (s *Service) encryptSecret(secret string) (string, error) {
	block, err := aes.NewCipher(s.secretKey[:])
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
	cipherText := gcm.Seal(nil, nonce, []byte(secret), nil)
	payload := append(nonce, cipherText...)
	return base64.RawURLEncoding.EncodeToString(payload), nil
}

func (s *Service) decryptSecret(cipherText string) (string, error) {
	raw, err := base64.RawURLEncoding.DecodeString(strings.TrimSpace(cipherText))
	if err != nil {
		return "", err
	}
	block, err := aes.NewCipher(s.secretKey[:])
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	nonceSize := gcm.NonceSize()
	if len(raw) <= nonceSize {
		return "", fmt.Errorf("invalid cipher text")
	}
	nonce := raw[:nonceSize]
	payload := raw[nonceSize:]
	plain, err := gcm.Open(nil, nonce, payload, nil)
	if err != nil {
		return "", err
	}
	return string(plain), nil
}

func validateWebhookURL(raw string) (string, error) {
	text := strings.TrimSpace(raw)
	if text == "" {
		return "", fmt.Errorf("%w: webhook url 不能为空", ErrInvalidInput)
	}
	parsed, err := url.ParseRequestURI(text)
	if err != nil {
		return "", fmt.Errorf("%w: webhook url 不合法", ErrInvalidInput)
	}
	if strings.ToLower(parsed.Scheme) != "https" {
		return "", fmt.Errorf("%w: 仅支持 https webhook", ErrInvalidInput)
	}
	if parsed.User != nil {
		return "", fmt.Errorf("%w: webhook url 不支持包含用户信息", ErrInvalidInput)
	}
	host := strings.TrimSpace(parsed.Hostname())
	if host == "" {
		return "", fmt.Errorf("%w: webhook host 不能为空", ErrInvalidInput)
	}
	if isPrivateHost(host) {
		return "", fmt.Errorf("%w: webhook host 不允许内网或回环地址", ErrInvalidInput)
	}
	return parsed.String(), nil
}

func isPrivateHost(host string) bool {
	lower := strings.ToLower(strings.TrimSpace(host))
	if lower == "localhost" || strings.HasSuffix(lower, ".local") {
		return true
	}
	ip := net.ParseIP(lower)
	if ip == nil {
		return false
	}
	return ip.IsPrivate() || ip.IsLoopback() || ip.IsLinkLocalMulticast() || ip.IsLinkLocalUnicast() || ip.IsUnspecified()
}

func normalizeHKSymbol(input string) (string, error) {
	raw := strings.ToUpper(strings.TrimSpace(input))
	raw = strings.TrimPrefix(raw, "HK")
	raw = strings.TrimSuffix(raw, ".HK")
	if raw == "" {
		return "", fmt.Errorf("%w: symbol 不能为空", ErrInvalidInput)
	}
	if len(raw) < 5 {
		raw = fmt.Sprintf("%05s", raw)
	}
	if len(raw) != 5 {
		return "", fmt.Errorf("%w: symbol 需为 5 位港股代码", ErrInvalidInput)
	}
	for _, ch := range raw {
		if ch < '0' || ch > '9' {
			return "", fmt.Errorf("%w: symbol 需为数字", ErrInvalidInput)
		}
	}
	return raw + ".HK", nil
}

func normalizeSide(side string) (string, error) {
	text := strings.ToUpper(strings.TrimSpace(side))
	if text == "" {
		text = "HOLD"
	}
	switch text {
	case "BUY", "SELL", "HOLD":
		return text, nil
	default:
		return "", fmt.Errorf("%w: side 仅支持 BUY/SELL/HOLD", ErrInvalidInput)
	}
}

func buildFingerprint(userID, symbol, strategyID, side string, eventTime time.Time, isTest bool, eventID string) string {
	bucket := eventTime.UTC().Truncate(time.Minute).Format("200601021504")
	seed := strings.Join([]string{userID, symbol, strategyID, side, bucket}, "|")
	if isTest {
		seed = seed + "|test|" + eventID
	}
	digest := sha256.Sum256([]byte(seed))
	return hex.EncodeToString(digest[:])
}

func ptrTime(value time.Time) *time.Time {
	v := value.UTC()
	return &v
}

func readResponseSnippet(reader io.Reader) string {
	if reader == nil {
		return ""
	}
	data, err := io.ReadAll(io.LimitReader(reader, 512))
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
}
