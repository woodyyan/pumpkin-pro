package signal

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/woodyyan/pumpkin-pro/backend/store/live"
)

// ──────────────────────────────────────────────────────────────
// 信号订阅的应用服务层：校验、默认值、参数归一。
// 处理器层（main.go）负责鉴权与策略存在性校验。
// ──────────────────────────────────────────────────────────────

const (
	defaultSubscriptionEvalIntervalSeconds = 900 // 15 分钟，与评估器 tick 对齐
	minSubscriptionEvalIntervalSeconds     = 900
	maxSubscriptionEvalIntervalSeconds     = 14400
	minSubscriptionCooldownSeconds         = 60
	maxSubscriptionCooldownSeconds         = 86400
)

// ListTemplates 暴露模板上架列表（API 视图即领域结构，无需转换）。
func (s *Service) ListSignalTemplates(ctx context.Context) ([]SignalTemplate, error) {
	return ListTemplates(), nil
}

func (s *Service) ListSubscriptions(ctx context.Context, userID string) ([]*SignalSubscription, error) {
	userID = strings.TrimSpace(userID)
	if userID == "" {
		return nil, ErrForbidden
	}
	records, err := s.repo.ListSubscriptions(ctx, userID)
	if err != nil {
		return nil, err
	}
	nameMap, err := s.repo.ResolveSymbolNames(ctx, subscriptionSymbols(records))
	if err != nil {
		// 名称解析失败不阻断订阅列表（与四象限名称解析失败不阻断回测同理）。
		nameMap = map[string]string{}
	}
	items := make([]*SignalSubscription, 0, len(records))
	for _, record := range records {
		item, convErr := toSignalSubscription(record, nameMap[record.Symbol])
		if convErr != nil {
			return nil, convErr
		}
		items = append(items, item)
	}
	return items, nil
}

func subscriptionSymbols(records []SignalSubscriptionRecord) []string {
	seen := map[string]bool{}
	symbols := make([]string, 0, len(records))
	for _, record := range records {
		if record.Symbol == "" || seen[record.Symbol] {
			continue
		}
		seen[record.Symbol] = true
		symbols = append(symbols, record.Symbol)
	}
	return symbols
}

// CreateSubscription 创建订阅。默认创建即开启（一键开启主流程）。
// 策略模板必须携带 strategy_id；策略存在性由处理器层校验。
func (s *Service) CreateSubscription(ctx context.Context, userID string, input SignalSubscriptionInput) (*SignalSubscription, error) {
	userID = strings.TrimSpace(userID)
	if userID == "" {
		return nil, ErrForbidden
	}

	tpl, ok := GetTemplate(input.TemplateKey)
	if !ok || !tpl.IsActive {
		return nil, fmt.Errorf("%w: 未知的信号模板", ErrInvalidInput)
	}

	scope, err := normalizeScopeType(input.ScopeType)
	if err != nil {
		return nil, err
	}
	if !containsString(tpl.SupportedScopes, scope) {
		return nil, fmt.Errorf("%w: 模板「%s」不支持该作用范围", ErrInvalidInput, tpl.Name)
	}

	symbol := ""
	if scope == ScopeTypeSymbol {
		normalized, _, normErr := live.NormalizeSymbol(input.Symbol)
		if normErr != nil {
			return nil, normErr
		}
		symbol = normalized
	}

	strategyID := strings.TrimSpace(input.StrategyID)
	if tpl.NeedsStrategy && strategyID == "" {
		return nil, fmt.Errorf("%w: 策略信号必须选择策略", ErrInvalidInput)
	}
	if !tpl.NeedsStrategy {
		strategyID = ""
	}

	params, err := ValidateTemplateParams(tpl, input.Params)
	if err != nil {
		return nil, err
	}
	paramsJSON, err := encodeJSONMap(params)
	if err != nil {
		return nil, fmt.Errorf("%w: params 字段格式错误", ErrInvalidInput)
	}

	isEnabled := true // 一键开启：默认创建即启用
	if input.IsEnabled != nil {
		isEnabled = *input.IsEnabled
	}

	evalInterval := input.EvalIntervalSeconds
	if evalInterval <= 0 {
		evalInterval = defaultSubscriptionEvalIntervalSeconds
	}
	if evalInterval < minSubscriptionEvalIntervalSeconds || evalInterval > maxSubscriptionEvalIntervalSeconds {
		return nil, fmt.Errorf("%w: eval_interval_seconds 必须在 %d~%d 之间", ErrInvalidInput, minSubscriptionEvalIntervalSeconds, maxSubscriptionEvalIntervalSeconds)
	}

	cooldown := input.CooldownSeconds
	if cooldown <= 0 {
		cooldown = defaultCooldownSeconds
	}
	if cooldown < minSubscriptionCooldownSeconds || cooldown > maxSubscriptionCooldownSeconds {
		return nil, fmt.Errorf("%w: cooldown_seconds 必须在 %d~%d 之间", ErrInvalidInput, minSubscriptionCooldownSeconds, maxSubscriptionCooldownSeconds)
	}

	// 查重：同五元组视为重复订阅。
	existing, err := s.repo.GetSubscriptionByUniqueKey(ctx, userID, tpl.Key, strategyID, scope, symbol)
	if err != nil && err != ErrNotFound {
		return nil, err
	}
	if existing != nil {
		return nil, fmt.Errorf("%w: 该信号已存在，可直接在列表中开启", ErrConflict)
	}

	now := time.Now().UTC()
	record := SignalSubscriptionRecord{
		ID:                   uuid.NewString(),
		UserID:               userID,
		TemplateKey:          tpl.Key,
		StrategyID:           strategyID,
		ScopeType:            scope,
		Symbol:               symbol,
		ParamsJSON:           paramsJSON,
		IsEnabled:            isEnabled,
		EvalIntervalSeconds:  evalInterval,
		CooldownSeconds:      cooldown,
		PositionAwareEnabled: true,
		CreatedAt:            now,
		UpdatedAt:            now,
	}

	saved, err := s.repo.CreateSubscription(ctx, record)
	if err != nil {
		return nil, err
	}
	nameMap, _ := s.repo.ResolveSymbolNames(ctx, []string{saved.Symbol})
	return toSignalSubscription(*saved, nameMap[saved.Symbol])
}

// UpdateSubscription 部分更新；nil 字段保持原值。开关即点即生效走同一入口。
func (s *Service) UpdateSubscription(ctx context.Context, userID, id string, input SignalSubscriptionUpdateInput) (*SignalSubscription, error) {
	userID = strings.TrimSpace(userID)
	if userID == "" {
		return nil, ErrForbidden
	}
	existing, err := s.repo.GetSubscriptionByID(ctx, userID, strings.TrimSpace(id))
	if err != nil {
		return nil, err
	}
	tpl, ok := GetTemplate(existing.TemplateKey)
	if !ok {
		return nil, fmt.Errorf("%w: 订阅引用的模板已下线", ErrInvalidInput)
	}

	updates := map[string]any{}

	if input.Params != nil {
		params, paramErr := ValidateTemplateParams(tpl, input.Params)
		if paramErr != nil {
			return nil, paramErr
		}
		paramsJSON, encErr := encodeJSONMap(params)
		if encErr != nil {
			return nil, fmt.Errorf("%w: params 字段格式错误", ErrInvalidInput)
		}
		updates["params_json"] = paramsJSON
	}
	if input.IsEnabled != nil {
		updates["is_enabled"] = *input.IsEnabled
	}
	if input.EvalIntervalSeconds != nil {
		value := *input.EvalIntervalSeconds
		if value < minSubscriptionEvalIntervalSeconds || value > maxSubscriptionEvalIntervalSeconds {
			return nil, fmt.Errorf("%w: eval_interval_seconds 必须在 %d~%d 之间", ErrInvalidInput, minSubscriptionEvalIntervalSeconds, maxSubscriptionEvalIntervalSeconds)
		}
		updates["eval_interval_seconds"] = value
	}
	if input.CooldownSeconds != nil {
		value := *input.CooldownSeconds
		if value < minSubscriptionCooldownSeconds || value > maxSubscriptionCooldownSeconds {
			return nil, fmt.Errorf("%w: cooldown_seconds 必须在 %d~%d 之间", ErrInvalidInput, minSubscriptionCooldownSeconds, maxSubscriptionCooldownSeconds)
		}
		updates["cooldown_seconds"] = value
	}

	if err := applySubscriptionRiskInput(updates, input); err != nil {
		return nil, err
	}

	saved, err := s.repo.UpdateSubscription(ctx, userID, existing.ID, updates)
	if err != nil {
		return nil, err
	}
	nameMap, _ := s.repo.ResolveSymbolNames(ctx, []string{saved.Symbol})
	return toSignalSubscription(*saved, nameMap[saved.Symbol])
}

// applySubscriptionRiskInput 校验风控入参并写入更新 map（显式 0 = 关闭该规则）。
func applySubscriptionRiskInput(updates map[string]any, input SignalSubscriptionUpdateInput) error {
	if input.PositionAwareEnabled != nil {
		updates["position_aware_enabled"] = *input.PositionAwareEnabled
	}
	if input.MaxPositionPct != nil {
		if *input.MaxPositionPct < 0 || *input.MaxPositionPct > 100 {
			return fmt.Errorf("%w: max_position_pct 必须在 0~100 之间", ErrInvalidInput)
		}
		updates["max_position_pct"] = *input.MaxPositionPct
	}
	if input.MaxAddTimes != nil {
		if *input.MaxAddTimes < 0 || *input.MaxAddTimes > 100 {
			return fmt.Errorf("%w: max_add_times 必须在 0~100 之间", ErrInvalidInput)
		}
		updates["max_add_times"] = *input.MaxAddTimes
	}
	if input.StopLossPct != nil {
		if *input.StopLossPct < 0 || *input.StopLossPct > 100 {
			return fmt.Errorf("%w: stop_loss_pct 必须在 0~100 之间", ErrInvalidInput)
		}
		updates["stop_loss_pct"] = *input.StopLossPct
	}
	if input.TrailingStopPct != nil {
		if *input.TrailingStopPct < 0 || *input.TrailingStopPct > 100 {
			return fmt.Errorf("%w: trailing_stop_pct 必须在 0~100 之间", ErrInvalidInput)
		}
		updates["trailing_stop_pct"] = *input.TrailingStopPct
	}
	return nil
}

func (s *Service) DeleteSubscription(ctx context.Context, userID, id string) error {
	userID = strings.TrimSpace(userID)
	if userID == "" {
		return ErrForbidden
	}
	return s.repo.DeleteSubscription(ctx, userID, strings.TrimSpace(id))
}

// CountSignalRefsByStrategy 统计策略被信号引用的总数（旧配置 + 新订阅）。
func (s *Service) CountSignalRefsByStrategy(ctx context.Context, userID, strategyID string) (int64, error) {
	userID = strings.TrimSpace(userID)
	if userID == "" {
		return 0, ErrForbidden
	}
	strategyID = strings.TrimSpace(strategyID)
	if strategyID == "" {
		return 0, fmt.Errorf("%w: strategy_id 不能为空", ErrInvalidInput)
	}
	legacyCount, err := s.repo.CountSymbolConfigsByStrategy(ctx, userID, strategyID)
	if err != nil {
		return 0, err
	}
	subCount, err := s.repo.CountSubscriptionsByStrategy(ctx, userID, strategyID)
	if err != nil {
		return 0, err
	}
	return legacyCount + subCount, nil
}

// ListSignalRefs 返回引用某策略的全部标的（旧配置 + 新订阅，去重）。
func (s *Service) ListSignalRefs(ctx context.Context, userID, strategyID string) ([]SymbolRef, error) {
	userID = strings.TrimSpace(userID)
	if userID == "" {
		return nil, ErrForbidden
	}
	strategyID = strings.TrimSpace(strategyID)
	if strategyID == "" {
		return nil, fmt.Errorf("%w: strategy_id 不能为空", ErrInvalidInput)
	}
	legacyRefs, err := s.repo.ListSymbolConfigRefs(ctx, userID, strategyID)
	if err != nil {
		return nil, err
	}
	subRefs, err := s.repo.ListSubscriptionRefsByStrategy(ctx, userID, strategyID)
	if err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	merged := make([]SymbolRef, 0, len(legacyRefs)+len(subRefs))
	for _, ref := range append(legacyRefs, subRefs...) {
		if seen[ref.Symbol] {
			continue
		}
		seen[ref.Symbol] = true
		merged = append(merged, ref)
	}
	return merged, nil
}

// ── 信号记录（事件流 / 未读 / 已读）──

func (s *Service) ListSignalEventsFiltered(ctx context.Context, userID, symbol, barState, side string, limit int) ([]*SignalEvent, error) {
	userID = strings.TrimSpace(userID)
	if userID == "" {
		return nil, ErrForbidden
	}
	if strings.TrimSpace(symbol) != "" {
		normalized, _, err := live.NormalizeSymbol(symbol)
		if err != nil {
			return nil, err
		}
		symbol = normalized
	}
	if trimmed := strings.TrimSpace(barState); trimmed != "" {
		switch trimmed {
		case BarStateIntradayProvisional, BarStateCloseConfirmed, BarStateRealtime:
		default:
			return nil, fmt.Errorf("%w: bar_state 不合法", ErrInvalidInput)
		}
		barState = trimmed
	}
	records, err := s.repo.ListSignalEventsFiltered(ctx, userID, symbol, barState, side, limit)
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

func (s *Service) CountUnreadEvents(ctx context.Context, userID string) (int64, error) {
	userID = strings.TrimSpace(userID)
	if userID == "" {
		return 0, ErrForbidden
	}
	return s.repo.CountUnreadEvents(ctx, userID)
}

func (s *Service) MarkEventsRead(ctx context.Context, userID, symbol string) (int64, error) {
	userID = strings.TrimSpace(userID)
	if userID == "" {
		return 0, ErrForbidden
	}
	if strings.TrimSpace(symbol) != "" {
		normalized, _, err := live.NormalizeSymbol(symbol)
		if err != nil {
			return 0, err
		}
		symbol = normalized
	}
	return s.repo.MarkEventsRead(ctx, userID, symbol)
}

func containsString(items []string, target string) bool {
	for _, item := range items {
		if item == target {
			return true
		}
	}
	return false
}
