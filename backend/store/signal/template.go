package signal

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

// ──────────────────────────────────────────────────────────────
// 信号模板注册表（Signal Template Registry）
//
// 模板是系统级能力的事实源，代码常量维护（对齐 D-021：第一期不做
// Admin 可编辑配置）。新增信号类型 = 在这里注册一条模板 + quant 侧
// 注册对应 implementation_key（price_alert 类为 backend 内置评估）。
//
// 模板分类：
//   - price_alert：价格提醒（涨破/跌破/单日涨跌幅），backend pricer 实时快照比对。
//   - indicator：指标信号（MACD/RSI/均线/布林/放量），quant 日线评估。
//   - strategy：策略信号，绑定用户策略库中的 active 策略（兼容存量配置）。
// ──────────────────────────────────────────────────────────────

// 模板分类。
const (
	TemplateCategoryPriceAlert = "price_alert"
	TemplateCategoryIndicator  = "indicator"
	TemplateCategoryStrategy   = "strategy"
)

// 盘中评估模式。
const (
	// IntradayModeRealtime 实时快照比对（价格提醒），无 K 线语义，事件 bar_state=realtime。
	IntradayModeRealtime = "realtime"
	// IntradayModeProvisionalOK 盘中可试算（形成中 K 线），事件 bar_state=intraday_provisional。
	IntradayModeProvisionalOK = "provisional_ok"
	// IntradayModeCloseOnly 仅收盘确认（放量类：盘中累计成交量上午偏小，盘中评估会失真）。
	IntradayModeCloseOnly = "close_only"
)

// 订阅范围。P0 仅支持 symbol；watchlist 为 P2 批量信号预留。
const (
	ScopeTypeSymbol    = "symbol"
	ScopeTypeWatchlist = "watchlist"
)

// 信号事件 bar 状态。
const (
	BarStateIntradayProvisional = "intraday_provisional"
	BarStateCloseConfirmed      = "close_confirmed"
	BarStateRealtime            = "realtime"
	BarStateTest                = "test"
)

// TemplateParamField 参数表单项（schema 驱动前端渲染，与策略库 ParamSchemaItem 同构）。
type TemplateParamField struct {
	Key         string   `json:"key"`
	Label       string   `json:"label"`
	Type        string   `json:"type"` // number
	Required    bool     `json:"required"`
	Default     any      `json:"default,omitempty"`
	Min         *float64 `json:"min,omitempty"`
	Max         *float64 `json:"max,omitempty"`
	Step        *float64 `json:"step,omitempty"`
	Unit        string   `json:"unit,omitempty"`
	Description string   `json:"description,omitempty"`
}

// SignalTemplate 信号模板定义。
type SignalTemplate struct {
	Key               string              `json:"key"`
	Name              string              `json:"name"`
	Description       string              `json:"description"`
	Category          string              `json:"category"`
	ImplementationKey string              `json:"implementation_key"`
	ParamSchema       []TemplateParamField `json:"param_schema"`
	DefaultParams     map[string]any      `json:"default_params"`
	IntradayMode      string              `json:"intraday_mode"`
	SupportedScopes   []string            `json:"supported_scopes"`
	NeedsStrategy     bool                `json:"needs_strategy"`
	SortOrder         int                 `json:"sort_order"`
	IsActive          bool                `json:"is_active"`
}

func floatPtr(v float64) *float64 { return &v }

// signalTemplates 首期模板清单（sort_order 即展示顺序）。
var signalTemplates = []SignalTemplate{
	{
		Key:         "price_above",
		Name:        "涨破提醒",
		Description: "最新价涨破目标价时提醒你，适合盯突破买入或止盈价位。",
		Category:    TemplateCategoryPriceAlert,
		ParamSchema: []TemplateParamField{
			{Key: "price", Label: "目标价", Type: "number", Required: true, Min: floatPtr(0.01), Step: floatPtr(0.01), Unit: "元", Description: "最新价涨破该价格时触发"},
		},
		DefaultParams:   map[string]any{},
		IntradayMode:    IntradayModeRealtime,
		SupportedScopes: []string{ScopeTypeSymbol},
		SortOrder:       10,
		IsActive:        true,
	},
	{
		Key:         "price_below",
		Name:        "跌破提醒",
		Description: "最新价跌破目标价时提醒你，适合盯止损价位或回踩买入价。",
		Category:    TemplateCategoryPriceAlert,
		ParamSchema: []TemplateParamField{
			{Key: "price", Label: "目标价", Type: "number", Required: true, Min: floatPtr(0.01), Step: floatPtr(0.01), Unit: "元", Description: "最新价跌破该价格时触发"},
		},
		DefaultParams:   map[string]any{},
		IntradayMode:    IntradayModeRealtime,
		SupportedScopes: []string{ScopeTypeSymbol},
		SortOrder:       20,
		IsActive:        true,
	},
	{
		Key:         "pct_change",
		Name:        "单日涨跌幅提醒",
		Description: "当日涨跌幅超过设定幅度时提醒你，适合捕捉异动。",
		Category:    TemplateCategoryPriceAlert,
		ParamSchema: []TemplateParamField{
			{Key: "pct", Label: "涨跌幅", Type: "number", Required: true, Default: 5.0, Min: floatPtr(1), Max: floatPtr(20), Step: floatPtr(0.5), Unit: "%", Description: "当日涨跌幅绝对值达到该幅度时触发"},
		},
		DefaultParams:   map[string]any{"pct": 5.0},
		IntradayMode:    IntradayModeRealtime,
		SupportedScopes: []string{ScopeTypeSymbol},
		SortOrder:       30,
		IsActive:        true,
	},
	{
		Key:               "macd_cross",
		Name:              "MACD 金叉死叉",
		Description:       "MACD 快线自下而上穿越慢线为金叉（买入提示），反之为死叉（卖出提示）。",
		Category:          TemplateCategoryIndicator,
		ImplementationKey: "macd_cross",
		ParamSchema: []TemplateParamField{
			{Key: "fast_period", Label: "快线周期", Type: "number", Required: true, Default: 12.0, Min: floatPtr(2), Max: floatPtr(60), Step: floatPtr(1)},
			{Key: "slow_period", Label: "慢线周期", Type: "number", Required: true, Default: 26.0, Min: floatPtr(5), Max: floatPtr(120), Step: floatPtr(1)},
			{Key: "signal_period", Label: "信号线周期", Type: "number", Required: true, Default: 9.0, Min: floatPtr(2), Max: floatPtr(60), Step: floatPtr(1)},
		},
		DefaultParams:   map[string]any{"fast_period": 12.0, "slow_period": 26.0, "signal_period": 9.0},
		IntradayMode:    IntradayModeProvisionalOK,
		SupportedScopes: []string{ScopeTypeSymbol},
		SortOrder:       40,
		IsActive:        true,
	},
	{
		Key:               "rsi_range",
		Name:              "RSI 超买超卖",
		Description:       "RSI 跌破超卖线（默认 30）提示买入机会，涨破超买线（默认 70）提示风险。",
		Category:          TemplateCategoryIndicator,
		ImplementationKey: "rsi_range",
		ParamSchema: []TemplateParamField{
			{Key: "rsi_period", Label: "RSI 周期", Type: "number", Required: true, Default: 14.0, Min: floatPtr(5), Max: floatPtr(30), Step: floatPtr(1)},
			{Key: "rsi_low", Label: "超卖线", Type: "number", Required: true, Default: 30.0, Min: floatPtr(10), Max: floatPtr(45), Step: floatPtr(1), Unit: ""},
			{Key: "rsi_high", Label: "超买线", Type: "number", Required: true, Default: 70.0, Min: floatPtr(55), Max: floatPtr(90), Step: floatPtr(1), Unit: ""},
		},
		DefaultParams:   map[string]any{"rsi_period": 14.0, "rsi_low": 30.0, "rsi_high": 70.0},
		IntradayMode:    IntradayModeProvisionalOK,
		SupportedScopes: []string{ScopeTypeSymbol},
		SortOrder:       50,
		IsActive:        true,
	},
	{
		Key:               "ma_breakout",
		Name:              "均线突破",
		Description:       "短期均线上穿长期均线提示趋势转强，下穿提示趋势转弱（默认 20/60 日线）。",
		Category:          TemplateCategoryIndicator,
		ImplementationKey: "trend_cross",
		ParamSchema: []TemplateParamField{
			{Key: "ma_short", Label: "短期均线", Type: "number", Required: true, Default: 20.0, Min: floatPtr(5), Max: floatPtr(60), Step: floatPtr(1), Unit: "日"},
			{Key: "ma_long", Label: "长期均线", Type: "number", Required: true, Default: 60.0, Min: floatPtr(20), Max: floatPtr(250), Step: floatPtr(1), Unit: "日"},
		},
		DefaultParams:   map[string]any{"ma_short": 20.0, "ma_long": 60.0},
		IntradayMode:    IntradayModeProvisionalOK,
		SupportedScopes: []string{ScopeTypeSymbol},
		SortOrder:       60,
		IsActive:        true,
	},
	{
		Key:               "bollinger_reversion",
		Name:              "布林带回归",
		Description:       "价格触及布林下轨提示超卖回归机会，触及上轨提示超买回落风险。",
		Category:          TemplateCategoryIndicator,
		ImplementationKey: "bollinger_reversion",
		ParamSchema: []TemplateParamField{
			{Key: "bb_period", Label: "布林周期", Type: "number", Required: true, Default: 20.0, Min: floatPtr(10), Max: floatPtr(60), Step: floatPtr(1)},
			{Key: "bb_std", Label: "标准差倍数", Type: "number", Required: true, Default: 2.0, Min: floatPtr(1), Max: floatPtr(3), Step: floatPtr(0.1)},
		},
		DefaultParams:   map[string]any{"bb_period": 20.0, "bb_std": 2.0},
		IntradayMode:    IntradayModeProvisionalOK,
		SupportedScopes: []string{ScopeTypeSymbol},
		SortOrder:       70,
		IsActive:        true,
	},
	{
		Key:               "volume_breakout",
		Name:              "放量突破",
		Description:       "成交量放大到近期均量倍数以上且价格上涨时提示。盘中累计成交量失真，仅收盘确认。",
		Category:          TemplateCategoryIndicator,
		ImplementationKey: "volume_breakout",
		ParamSchema: []TemplateParamField{
			{Key: "lookback", Label: "均量周期", Type: "number", Required: true, Default: 20.0, Min: floatPtr(5), Max: floatPtr(60), Step: floatPtr(1), Unit: "日"},
			{Key: "volume_multiple", Label: "放量倍数", Type: "number", Required: true, Default: 2.0, Min: floatPtr(1.2), Max: floatPtr(5), Step: floatPtr(0.1), Unit: "倍"},
			{Key: "exit_ma_period", Label: "离场均线", Type: "number", Required: true, Default: 20.0, Min: floatPtr(5), Max: floatPtr(60), Step: floatPtr(1), Unit: "日"},
		},
		DefaultParams:   map[string]any{"lookback": 20.0, "volume_multiple": 2.0, "exit_ma_period": 20.0},
		IntradayMode:    IntradayModeCloseOnly,
		SupportedScopes: []string{ScopeTypeSymbol},
		SortOrder:       80,
		IsActive:        true,
	},
	{
		Key:         "strategy",
		Name:        "策略信号",
		Description: "绑定策略库中已激活的策略，按策略逻辑产出买卖提示（进阶）。",
		Category:    TemplateCategoryStrategy,
		// implementation_key 在评估时由所绑定策略解析。
		ParamSchema:     []TemplateParamField{},
		DefaultParams:   map[string]any{},
		IntradayMode:    IntradayModeProvisionalOK,
		SupportedScopes: []string{ScopeTypeSymbol},
		NeedsStrategy:   true,
		SortOrder:       90,
		IsActive:        true,
	},
}

// ListTemplates 返回已上架模板（按 sort_order 排序后的副本）。
func ListTemplates() []SignalTemplate {
	items := make([]SignalTemplate, 0, len(signalTemplates))
	for _, tpl := range signalTemplates {
		if !tpl.IsActive {
			continue
		}
		items = append(items, tpl)
	}
	sort.SliceStable(items, func(i, j int) bool { return items[i].SortOrder < items[j].SortOrder })
	return items
}

// GetTemplate 按 key 查找模板（含未上架模板，供订阅记录解析展示名）。
func GetTemplate(key string) (SignalTemplate, bool) {
	for _, tpl := range signalTemplates {
		if tpl.Key == strings.TrimSpace(key) {
			return tpl, true
		}
	}
	return SignalTemplate{}, false
}

// ValidateTemplateParams 校验并归一化订阅参数：合并模板默认值，按 schema 做类型与范围校验。
// 返回的 map 是「模板默认值 + 用户覆盖」的完整参数集。
func ValidateTemplateParams(tpl SignalTemplate, overrides map[string]any) (map[string]any, error) {
	effective := map[string]any{}
	for key, value := range tpl.DefaultParams {
		effective[key] = value
	}

	for _, field := range tpl.ParamSchema {
		raw, provided := overrides[field.Key]
		if !provided || raw == nil {
			if field.Required {
				if _, hasDefault := effective[field.Key]; !hasDefault {
					return nil, fmt.Errorf("%w: 参数「%s」不能为空", ErrInvalidInput, field.Label)
				}
			}
			continue
		}
		value, err := coerceParamNumber(raw)
		if err != nil {
			return nil, fmt.Errorf("%w: 参数「%s」必须是数字", ErrInvalidInput, field.Label)
		}
		if field.Min != nil && value < *field.Min {
			return nil, fmt.Errorf("%w: 参数「%s」不能小于 %v", ErrInvalidInput, field.Label, *field.Min)
		}
		if field.Max != nil && value > *field.Max {
			return nil, fmt.Errorf("%w: 参数「%s」不能大于 %v", ErrInvalidInput, field.Label, *field.Max)
		}
		effective[field.Key] = value
	}

	// 拒绝 schema 之外的未知参数，避免静默忽略用户输入。
	for key := range overrides {
		if _, ok := effective[key]; ok {
			continue
		}
		return nil, fmt.Errorf("%w: 模板不支持参数「%s」", ErrInvalidInput, key)
	}
	return effective, nil
}

func coerceParamNumber(raw any) (float64, error) {
	switch typed := raw.(type) {
	case float64:
		return typed, nil
	case float32:
		return float64(typed), nil
	case int:
		return float64(typed), nil
	case int64:
		return float64(typed), nil
	case json.Number:
		return typed.Float64()
	default:
		return 0, fmt.Errorf("not a number")
	}
}
