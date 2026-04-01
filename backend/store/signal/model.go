package signal

import (
	"encoding/json"
	"fmt"
	"time"
)

type WebhookEndpointRecord struct {
	ID               string    `gorm:"primaryKey;size:36"`
	UserID           string    `gorm:"size:36;not null;uniqueIndex"`
	URL              string    `gorm:"size:1024;not null"`
	SecretCipherText string    `gorm:"type:text;not null;default:''"`
	IsEnabled        bool      `gorm:"not null;default:true;index"`
	TimeoutMS        int       `gorm:"not null;default:3000"`
	CreatedAt        time.Time `gorm:"not null"`
	UpdatedAt        time.Time `gorm:"not null"`
}

func (WebhookEndpointRecord) TableName() string {
	return "webhook_endpoints"
}

type SymbolSignalConfigRecord struct {
	ID                  string     `gorm:"primaryKey;size:36"`
	UserID              string     `gorm:"size:36;not null;index;uniqueIndex:idx_signal_config_user_symbol,priority:1"`
	Symbol              string     `gorm:"size:16;not null;index;uniqueIndex:idx_signal_config_user_symbol,priority:2"`
	StrategyID          string     `gorm:"size:128;not null;default:'';index"`
	IsEnabled           bool       `gorm:"not null;default:false;index"`
	CooldownSeconds     int        `gorm:"not null;default:300"`
	EvalIntervalSeconds int        `gorm:"not null;default:3600"`
	ThresholdsJSON      string     `gorm:"type:text;not null;default:'{}'"`
	LastEvaluatedAt     *time.Time `gorm:"index"`
	CreatedAt           time.Time  `gorm:"not null"`
	UpdatedAt           time.Time  `gorm:"not null"`
}

func (SymbolSignalConfigRecord) TableName() string {
	return "symbol_signal_configs"
}

type SignalEventRecord struct {
	ID          string    `gorm:"primaryKey;size:36"`
	EventID     string    `gorm:"size:64;not null;uniqueIndex"`
	UserID      string    `gorm:"size:36;not null;index"`
	Symbol      string    `gorm:"size:16;not null;index"`
	StrategyID  string    `gorm:"size:128;not null;default:'';index"`
	Side        string    `gorm:"size:16;not null;index"`
	SignalScore float64   `gorm:"not null;default:0"`
	ReasonJSON  string    `gorm:"type:text;not null;default:'{}'"`
	Fingerprint string    `gorm:"size:128;not null;uniqueIndex"`
	IsTest      bool      `gorm:"not null;default:false;index"`
	EventTime   time.Time `gorm:"not null;index"`
	CreatedAt   time.Time `gorm:"not null;index"`
}

func (SignalEventRecord) TableName() string {
	return "signal_events"
}

type WebhookDeliveryRecord struct {
	ID            string     `gorm:"primaryKey;size:36"`
	EventID       string     `gorm:"size:64;not null;index"`
	UserID        string     `gorm:"size:36;not null;index"`
	EndpointID    string     `gorm:"size:36;not null;index"`
	AttemptNo     int        `gorm:"not null;default:1"`
	Status        string     `gorm:"size:32;not null;index"`
	HTTPStatus    int        `gorm:"not null;default:0"`
	LatencyMS     int64      `gorm:"not null;default:0"`
	ErrorMessage  string     `gorm:"type:text;not null;default:''"`
	NextRetryAt   *time.Time `gorm:"index"`
	LastAttemptAt *time.Time `gorm:"index"`
	DeliveredAt   *time.Time `gorm:"index"`
	CreatedAt     time.Time  `gorm:"not null;index"`
	UpdatedAt     time.Time  `gorm:"not null;index"`
}

func (WebhookDeliveryRecord) TableName() string {
	return "webhook_deliveries"
}

type WebhookEndpoint struct {
	URL       string `json:"url"`
	HasSecret bool   `json:"has_secret"`
	IsEnabled bool   `json:"is_enabled"`
	TimeoutMS int    `json:"timeout_ms"`
	UpdatedAt string `json:"updated_at"`
}

type SymbolSignalConfig struct {
	Symbol              string         `json:"symbol"`
	StrategyID          string         `json:"strategy_id"`
	IsEnabled           bool           `json:"is_enabled"`
	CooldownSeconds     int            `json:"cooldown_seconds"`
	EvalIntervalSeconds int            `json:"eval_interval_seconds"`
	Thresholds          map[string]any `json:"thresholds"`
	UpdatedAt           string         `json:"updated_at"`
}

type SignalEvent struct {
	EventID     string         `json:"event_id"`
	Symbol      string         `json:"symbol"`
	StrategyID  string         `json:"strategy_id"`
	Side        string         `json:"side"`
	SignalScore float64        `json:"signal_score"`
	IsTest      bool           `json:"is_test"`
	EventTime   string         `json:"event_time"`
	Reason      map[string]any `json:"reason"`
}

type WebhookDelivery struct {
	EventID      string `json:"event_id"`
	Symbol       string `json:"symbol"`
	AttemptNo    int    `json:"attempt_no"`
	Status       string `json:"status"`
	HTTPStatus   int    `json:"http_status"`
	LatencyMS    int64  `json:"latency_ms"`
	ErrorMessage string `json:"error_message"`
	NextRetryAt  string `json:"next_retry_at,omitempty"`
	DeliveredAt  string `json:"delivered_at,omitempty"`
	CreatedAt    string `json:"created_at"`
	UpdatedAt    string `json:"updated_at"`
}

type WebhookConfigInput struct {
	URL       string `json:"url"`
	Secret    string `json:"secret"`
	IsEnabled *bool  `json:"is_enabled"`
	TimeoutMS int    `json:"timeout_ms"`
}

type SymbolSignalConfigInput struct {
	StrategyID          string         `json:"strategy_id"`
	IsEnabled           *bool          `json:"is_enabled"`
	CooldownSeconds     int            `json:"cooldown_seconds"`
	EvalIntervalSeconds int            `json:"eval_interval_seconds"`
	Thresholds          map[string]any `json:"thresholds"`
}

type TestSignalInput struct {
	Symbol string `json:"symbol"`
	Side   string `json:"side"`
}

type EmitSignalInput struct {
	UserID      string
	Symbol      string
	StrategyID  string
	Side        string
	SignalScore float64
	Reason      map[string]any
	EventTime   time.Time
	IsTest      bool
}

type DispatchResult struct {
	Event    *SignalEvent     `json:"event,omitempty"`
	Delivery *WebhookDelivery `json:"delivery,omitempty"`
}

func toWebhookEndpoint(record *WebhookEndpointRecord, hasSecret bool) *WebhookEndpoint {
	if record == nil {
		return nil
	}
	return &WebhookEndpoint{
		URL:       record.URL,
		HasSecret: hasSecret,
		IsEnabled: record.IsEnabled,
		TimeoutMS: record.TimeoutMS,
		UpdatedAt: record.UpdatedAt.UTC().Format(time.RFC3339),
	}
}

func toSymbolSignalConfig(record SymbolSignalConfigRecord) (*SymbolSignalConfig, error) {
	thresholds := map[string]any{}
	if err := decodeJSONMap(record.ThresholdsJSON, &thresholds); err != nil {
		return nil, fmt.Errorf("decode thresholds failed: %w", err)
	}
	return &SymbolSignalConfig{
		Symbol:              record.Symbol,
		StrategyID:          record.StrategyID,
		IsEnabled:           record.IsEnabled,
		CooldownSeconds:     record.CooldownSeconds,
		EvalIntervalSeconds: record.EvalIntervalSeconds,
		Thresholds:          thresholds,
		UpdatedAt:           record.UpdatedAt.UTC().Format(time.RFC3339),
	}, nil
}

func toSignalEvent(record SignalEventRecord) (*SignalEvent, error) {
	reason := map[string]any{}
	if err := decodeJSONMap(record.ReasonJSON, &reason); err != nil {
		return nil, fmt.Errorf("decode reason failed: %w", err)
	}
	return &SignalEvent{
		EventID:     record.EventID,
		Symbol:      record.Symbol,
		StrategyID:  record.StrategyID,
		Side:        record.Side,
		SignalScore: record.SignalScore,
		IsTest:      record.IsTest,
		EventTime:   record.EventTime.UTC().Format(time.RFC3339),
		Reason:      reason,
	}, nil
}

func toWebhookDelivery(record WebhookDeliveryRecord, symbol string) *WebhookDelivery {
	nextRetryAt := ""
	deliveredAt := ""
	if record.NextRetryAt != nil {
		nextRetryAt = record.NextRetryAt.UTC().Format(time.RFC3339)
	}
	if record.DeliveredAt != nil {
		deliveredAt = record.DeliveredAt.UTC().Format(time.RFC3339)
	}
	return &WebhookDelivery{
		EventID:      record.EventID,
		Symbol:       symbol,
		AttemptNo:    record.AttemptNo,
		Status:       record.Status,
		HTTPStatus:   record.HTTPStatus,
		LatencyMS:    record.LatencyMS,
		ErrorMessage: record.ErrorMessage,
		NextRetryAt:  nextRetryAt,
		DeliveredAt:  deliveredAt,
		CreatedAt:    record.CreatedAt.UTC().Format(time.RFC3339),
		UpdatedAt:    record.UpdatedAt.UTC().Format(time.RFC3339),
	}
}

func encodeJSONMap(input map[string]any) (string, error) {
	target := input
	if target == nil {
		target = map[string]any{}
	}
	encoded, err := json.Marshal(target)
	if err != nil {
		return "", err
	}
	return string(encoded), nil
}

func decodeJSONMap(raw string, target *map[string]any) error {
	if target == nil {
		return nil
	}
	if raw == "" {
		raw = "{}"
	}
	return json.Unmarshal([]byte(raw), target)
}
