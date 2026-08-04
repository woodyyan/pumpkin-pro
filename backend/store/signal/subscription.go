package signal

import (
	"fmt"
	"strings"
	"time"
)

// ──────────────────────────────────────────────────────────────
// 信号订阅（Signal Subscription）
//
// 订阅是用户配置的事实表：template_key 指向系统模板，scope 决定作用标的。
// 与旧 symbol_signal_configs 的关键差异：一只股票可订阅多个模板
// （唯一约束为 user+template+strategy+scope+symbol 五元组）。
//
// 风控字段与 SymbolSignalConfigRecord 同语义（单票 > 账户 > 默认，0=未设置），
// 评估时经 Config 兼容层喂给 RiskGate，保证持仓感知门控不回归。
// ──────────────────────────────────────────────────────────────

type SignalSubscriptionRecord struct {
	ID          string `gorm:"primaryKey;size:36"`
	UserID      string `gorm:"size:36;not null;index;uniqueIndex:idx_signal_sub_unique,priority:1"`
	TemplateKey string `gorm:"size:64;not null;uniqueIndex:idx_signal_sub_unique,priority:2"`
	StrategyID  string `gorm:"size:128;not null;default:'';uniqueIndex:idx_signal_sub_unique,priority:3"`
	ScopeType   string `gorm:"size:16;not null;default:'symbol';uniqueIndex:idx_signal_sub_unique,priority:4"`
	Symbol      string `gorm:"size:16;not null;default:'';index;uniqueIndex:idx_signal_sub_unique,priority:5"`

	ParamsJSON          string `gorm:"type:text;not null;default:'{}'"`
	IsEnabled           bool   `gorm:"not null;default:false;index"`
	EvalIntervalSeconds int    `gorm:"not null;default:900"`
	CooldownSeconds     int    `gorm:"not null;default:3600"`

	// ── 持仓感知 / 风控（单票级；0 表示"未设置"回退上层或关闭）──
	PositionAwareEnabled bool    `gorm:"not null;default:true"`
	MaxPositionPct       float64 `gorm:"not null;default:0"`
	MaxAddTimes          int     `gorm:"not null;default:0"`
	StopLossPct          float64 `gorm:"not null;default:0"` // 止损线(相对成本 %)，0=关闭
	TrailingStopPct      float64 `gorm:"not null;default:0"` // 移动止盈回撤(%)，0=关闭

	LastEvaluatedAt *time.Time `gorm:"index"`
	CreatedAt       time.Time  `gorm:"not null"`
	UpdatedAt       time.Time  `gorm:"not null"`
}

func (SignalSubscriptionRecord) TableName() string {
	return "signal_subscriptions"
}

// SignalSubscription 订阅的 API 视图。
type SignalSubscription struct {
	ID          string         `json:"id"`
	TemplateKey string         `json:"template_key"`
	TemplateName string        `json:"template_name"`
	Category    string         `json:"category"`
	StrategyID  string         `json:"strategy_id,omitempty"`
	ScopeType   string         `json:"scope_type"`
	Symbol      string         `json:"symbol"`
	SymbolName  string         `json:"symbol_name,omitempty"`
	Params      map[string]any `json:"params"`
	IsEnabled   bool           `json:"is_enabled"`
	EvalIntervalSeconds int    `json:"eval_interval_seconds"`
	CooldownSeconds     int    `json:"cooldown_seconds"`

	PositionAwareEnabled bool    `json:"position_aware_enabled"`
	MaxPositionPct       float64 `json:"max_position_pct"`
	MaxAddTimes          int     `json:"max_add_times"`
	StopLossPct          float64 `json:"stop_loss_pct"`
	TrailingStopPct      float64 `json:"trailing_stop_pct"`

	CreatedAt string `json:"created_at"`
	UpdatedAt string `json:"updated_at"`
}

// SignalSubscriptionInput 创建订阅的入参。
type SignalSubscriptionInput struct {
	TemplateKey string         `json:"template_key"`
	StrategyID  string         `json:"strategy_id"`
	ScopeType   string         `json:"scope_type"`
	Symbol      string         `json:"symbol"`
	Params      map[string]any `json:"params"`
	IsEnabled   *bool          `json:"is_enabled"`
	EvalIntervalSeconds int    `json:"eval_interval_seconds"`
	CooldownSeconds     int    `json:"cooldown_seconds"`
}

// SignalSubscriptionUpdateInput 更新订阅的入参（指针=未传保持原值）。
type SignalSubscriptionUpdateInput struct {
	Params              map[string]any `json:"params"`
	IsEnabled           *bool          `json:"is_enabled"`
	EvalIntervalSeconds *int           `json:"eval_interval_seconds"`
	CooldownSeconds     *int           `json:"cooldown_seconds"`

	PositionAwareEnabled *bool    `json:"position_aware_enabled"`
	MaxPositionPct       *float64 `json:"max_position_pct"`
	MaxAddTimes          *int     `json:"max_add_times"`
	StopLossPct          *float64 `json:"stop_loss_pct"`
	TrailingStopPct      *float64 `json:"trailing_stop_pct"`
}

func toSignalSubscription(record SignalSubscriptionRecord, symbolName string) (*SignalSubscription, error) {
	params := map[string]any{}
	if err := decodeJSONMap(record.ParamsJSON, &params); err != nil {
		return nil, fmt.Errorf("decode subscription params failed: %w", err)
	}
	templateName := record.TemplateKey
	category := ""
	if tpl, ok := GetTemplate(record.TemplateKey); ok {
		templateName = tpl.Name
		category = tpl.Category
	}
	return &SignalSubscription{
		ID:                  record.ID,
		TemplateKey:         record.TemplateKey,
		TemplateName:        templateName,
		Category:            category,
		StrategyID:          record.StrategyID,
		ScopeType:           record.ScopeType,
		Symbol:              record.Symbol,
		SymbolName:          symbolName,
		Params:              params,
		IsEnabled:           record.IsEnabled,
		EvalIntervalSeconds: record.EvalIntervalSeconds,
		CooldownSeconds:     record.CooldownSeconds,
		PositionAwareEnabled: record.PositionAwareEnabled,
		MaxPositionPct:       record.MaxPositionPct,
		MaxAddTimes:          record.MaxAddTimes,
		StopLossPct:          record.StopLossPct,
		TrailingStopPct:      record.TrailingStopPct,
		CreatedAt:           record.CreatedAt.UTC().Format(time.RFC3339),
		UpdatedAt:           record.UpdatedAt.UTC().Format(time.RFC3339),
	}, nil
}

// riskConfigRecord 把订阅的风控字段适配为 RiskGate 装配层使用的 Config 形态。
// Config 兼容层：mergeRiskConfig 只读取这些字段，其余字段不参与门控。
func (r SignalSubscriptionRecord) riskConfigRecord() SymbolSignalConfigRecord {
	return SymbolSignalConfigRecord{
		PositionAwareEnabled: r.PositionAwareEnabled,
		MaxPositionPct:       r.MaxPositionPct,
		MaxAddTimes:          r.MaxAddTimes,
		StopLossPct:          r.StopLossPct,
		TrailingStopPct:      r.TrailingStopPct,
	}
}

// effectiveParams 返回模板默认值 + 订阅覆盖后的完整参数。
func (r SignalSubscriptionRecord) effectiveParams() map[string]any {
	params := map[string]any{}
	if tpl, ok := GetTemplate(r.TemplateKey); ok {
		for key, value := range tpl.DefaultParams {
			params[key] = value
		}
	}
	overrides := map[string]any{}
	if err := decodeJSONMap(r.ParamsJSON, &overrides); err == nil {
		for key, value := range overrides {
			params[key] = value
		}
	}
	return params
}

func normalizeScopeType(raw string) (string, error) {
	scope := strings.ToLower(strings.TrimSpace(raw))
	if scope == "" {
		scope = ScopeTypeSymbol
	}
	switch scope {
	case ScopeTypeSymbol:
		return scope, nil
	case ScopeTypeWatchlist:
		// P2 批量信号预留，当前不接受创建。
		return "", fmt.Errorf("%w: 暂不支持自选股批量信号", ErrInvalidInput)
	default:
		return "", fmt.Errorf("%w: scope_type 仅支持 symbol", ErrInvalidInput)
	}
}
