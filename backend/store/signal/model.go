package signal

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

type WebhookEndpointRecord struct {
	ID               string    `gorm:"primaryKey;size:36"`
	UserID           string    `gorm:"size:36;not null;uniqueIndex"`
	URL              string    `gorm:"size:1024;not null"`
	Channel          string    `gorm:"size:16;not null;default:'wecom'"`
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
	ID                  string `gorm:"primaryKey;size:36"`
	UserID              string `gorm:"size:36;not null;index;uniqueIndex:idx_signal_config_user_symbol,priority:1"`
	Symbol              string `gorm:"size:16;not null;index;uniqueIndex:idx_signal_config_user_symbol,priority:2"`
	StrategyID          string `gorm:"size:128;not null;default:'';index"`
	IsEnabled           bool   `gorm:"not null;default:false;index"`
	CooldownSeconds     int    `gorm:"not null;default:300"`
	EvalIntervalSeconds int    `gorm:"not null;default:3600"`
	ThresholdsJSON      string `gorm:"type:text;not null;default:'{}'"`

	// ── 持仓感知 / 风控（单票级；优先级 单票 > 账户 > 默认，0 表示"未设置"回退上层或关闭）──
	// PositionAwareEnabled 默认 true：持仓感知门控只是"不推送无意义信号"，无交易决策风险。
	PositionAwareEnabled bool    `gorm:"not null;default:true"`
	MaxPositionPct       float64 `gorm:"not null;default:0"` // 单票市值占总资金上限(%)，0=回退账户级
	MaxAddTimes          int     `gorm:"not null;default:0"` // 最大加仓次数，0=不限
	// StopLossPct / TrailingStopPct 默认 0 = 关闭。
	// 止损属于 override（会强制发出卖出信号），默认开启等于替用户做重大交易决策，必须显式设置。
	StopLossPct     float64 `gorm:"not null;default:0"` // 止损线(相对成本 %)，0=关闭
	TrailingStopPct float64 `gorm:"not null;default:0"` // 移动止盈回撤(%)，0=关闭

	LastEvaluatedAt *time.Time `gorm:"index"`
	CreatedAt       time.Time  `gorm:"not null"`
	UpdatedAt       time.Time  `gorm:"not null"`
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
	Side        string    `gorm:"size:16;not null;index"` // == FinalSide，保留以兼容既有 webhook / 查询逻辑
	SignalScore float64   `gorm:"not null;default:0"`
	ReasonJSON  string    `gorm:"type:text;not null;default:'{}'"`
	Fingerprint string    `gorm:"size:128;not null;uniqueIndex"`
	IsTest      bool      `gorm:"not null;default:false;index"`
	EventTime   time.Time `gorm:"not null;index"`
	CreatedAt   time.Time `gorm:"not null;index"`

	// ── 信号中心（模板 + 订阅 + 双状态）──
	// BarState：intraday_provisional（盘中试算）/ close_confirmed（收盘确认）/
	// realtime（价格提醒实时触发）/ test（测试）。空串为订阅体系上线前的存量事件。
	SubscriptionID string `gorm:"size:36;not null;default:'';index"`
	TemplateKey    string `gorm:"size:64;not null;default:'';index"`
	BarState       string `gorm:"size:24;not null;default:'';index"`
	TradeDate      string `gorm:"size:10;not null;default:'';index"`
	IsRead         bool   `gorm:"not null;default:false;index"`

	// ── 持仓感知门控（全量生成 + 推送门控）──
	// 设计要点：所有信号一律落库（保留策略胜率可复盘性），仅 IsDelivered 决定是否投递 webhook。
	RawSide              string  `gorm:"size:16;not null;default:''"`     // 策略原始输出（未经门控）
	FinalSide            string  `gorm:"size:16;not null;default:''"`     // 门控后最终动作（override 可改写）
	GateDecision         string  `gorm:"size:24;not null;default:''"`     // pass / suppressed / overridden
	SuppressedReason     string  `gorm:"size:64;not null;default:''"`     // 拦截规则码
	SemanticLabel        string  `gorm:"size:32;not null;default:''"`     // 首次建仓/加仓/减仓/清仓/止损离场
	MatchedRulesJSON     string  `gorm:"type:text;not null;default:'[]'"` // 命中的全部规则码
	IsDelivered          bool    `gorm:"not null;default:false;index"`    // 是否实际创建了投递
	PositionSnapshotJSON string  `gorm:"type:text;not null;default:'{}'"` // 决策时点持仓快照
	ReferencePrice       float64 `gorm:"not null;default:0"`              // 决策参考价
	SuggestedShares      float64 `gorm:"not null;default:0"`              // 参考数量（二期完善）
	PositionDataStatus   string  `gorm:"size:16;not null;default:''"`     // known / unknown / stale
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
	Channel   string `json:"channel"`
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

	// 持仓感知 / 风控（单票级）
	PositionAwareEnabled bool    `json:"position_aware_enabled"`
	MaxPositionPct       float64 `json:"max_position_pct"`
	MaxAddTimes          int     `json:"max_add_times"`
	StopLossPct          float64 `json:"stop_loss_pct"`
	TrailingStopPct      float64 `json:"trailing_stop_pct"`
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

	// 信号中心（模板 + 订阅 + 双状态）
	SubscriptionID string `json:"subscription_id,omitempty"`
	TemplateKey    string `json:"template_key,omitempty"`
	TemplateName   string `json:"template_name,omitempty"`
	BarState       string `json:"bar_state,omitempty"`
	TradeDate      string `json:"trade_date,omitempty"`
	IsRead         bool   `json:"is_read"`

	// 持仓感知门控信息（供前端信号历史展示"为什么没推送"）
	RawSide            string            `json:"raw_side,omitempty"`
	FinalSide          string            `json:"final_side,omitempty"`
	GateDecision       string            `json:"gate_decision,omitempty"`
	SuppressedReason   string            `json:"suppressed_reason,omitempty"`
	SuppressedMessage  string            `json:"suppressed_message,omitempty"`
	SemanticLabel      string            `json:"semantic_label,omitempty"`
	MatchedRules       []string          `json:"matched_rules,omitempty"`
	IsDelivered        bool              `json:"is_delivered"`
	PositionSnapshot   map[string]any    `json:"position_snapshot,omitempty"`
	ReferencePrice     float64           `json:"reference_price,omitempty"`
	SuggestedShares    float64           `json:"suggested_shares,omitempty"`
	PositionDataStatus string            `json:"position_data_status,omitempty"`
	Compliance         *ComplianceNotice `json:"compliance,omitempty"`
}

// ComplianceNotice 内联合规声明。设计要求：免责声明必须随每条信号内联下发，
// 而不是只放在页面角落，避免个性化建议被误解为投资顾问服务。
type ComplianceNotice struct {
	Disclaimer string `json:"disclaimer"`
}

// SignalComplianceDisclaimer 全站统一的信号免责声明文案（研究参考口径）。
const SignalComplianceDisclaimer = "本提示由你配置的策略与持仓参数自动生成，仅供投研参考，不构成投资建议。请结合自身情况独立决策。"

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
	Channel   string `json:"channel"`
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

	// ── 持仓感知 / 风控（单票级）──
	// 全部使用指针：nil 表示"未传，保持原值"；显式 0 表示"关闭该规则"。
	PositionAwareEnabled *bool    `json:"position_aware_enabled"`
	MaxPositionPct       *float64 `json:"max_position_pct"`
	MaxAddTimes          *int     `json:"max_add_times"`
	StopLossPct          *float64 `json:"stop_loss_pct"`
	TrailingStopPct      *float64 `json:"trailing_stop_pct"`
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

	// ── 信号中心（可选；为空时保持存量语义）──
	SubscriptionID string
	TemplateKey    string
	BarState       string // intraday_provisional / close_confirmed / realtime / test

	// ── 持仓感知门控（可选；为空时行为与改造前完全一致，保证向后兼容）──
	// Gate 为 nil 表示"未经门控"，EmitSignal 会按老逻辑要求 webhook 并正常投递。
	Gate *EmitGateInfo
}

// EmitGateInfo 承载门控结果，决定 signal_event 的门控字段与是否投递。
type EmitGateInfo struct {
	RawSide            string
	FinalSide          string
	Decision           string // pass / suppressed / overridden
	SuppressedReason   string
	SemanticLabel      string
	MatchedRules       []string
	PositionSnapshot   map[string]any
	ReferencePrice     float64
	SuggestedShares    float64
	PositionDataStatus string
	// SkipDelivery=true 时只落库不投递（被门控拦截的信号）。
	SkipDelivery bool
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
		Channel:   record.Channel,
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

		PositionAwareEnabled: record.PositionAwareEnabled,
		MaxPositionPct:       record.MaxPositionPct,
		MaxAddTimes:          record.MaxAddTimes,
		StopLossPct:          record.StopLossPct,
		TrailingStopPct:      record.TrailingStopPct,
	}, nil
}

func toSignalEvent(record SignalEventRecord) (*SignalEvent, error) {
	reason := map[string]any{}
	if err := decodeJSONMap(record.ReasonJSON, &reason); err != nil {
		return nil, fmt.Errorf("decode reason failed: %w", err)
	}
	positionSnapshot := map[string]any{}
	// 快照解析失败不应让整条信号查询失败——降级为空快照。
	if err := decodeJSONMap(record.PositionSnapshotJSON, &positionSnapshot); err != nil {
		positionSnapshot = map[string]any{}
	}
	matchedRules := []string{}
	if strings.TrimSpace(record.MatchedRulesJSON) != "" {
		_ = json.Unmarshal([]byte(record.MatchedRulesJSON), &matchedRules)
	}
	templateName := ""
	if record.TemplateKey != "" {
		if tpl, ok := GetTemplate(record.TemplateKey); ok {
			templateName = tpl.Name
		}
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

		SubscriptionID: record.SubscriptionID,
		TemplateKey:    record.TemplateKey,
		TemplateName:   templateName,
		BarState:       record.BarState,
		TradeDate:      record.TradeDate,
		IsRead:         record.IsRead,

		RawSide:            record.RawSide,
		FinalSide:          record.FinalSide,
		GateDecision:       record.GateDecision,
		SuppressedReason:   record.SuppressedReason,
		SuppressedMessage:  GateRuleMessage(record.SuppressedReason),
		SemanticLabel:      record.SemanticLabel,
		MatchedRules:       matchedRules,
		IsDelivered:        record.IsDelivered,
		PositionSnapshot:   positionSnapshot,
		ReferencePrice:     record.ReferencePrice,
		SuggestedShares:    record.SuggestedShares,
		PositionDataStatus: record.PositionDataStatus,
		Compliance:         &ComplianceNotice{Disclaimer: SignalComplianceDisclaimer},
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
