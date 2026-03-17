package strategy

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"sort"
	"strconv"
	"strings"
)

var allowedStatuses = map[string]struct{}{
	"draft":    {},
	"active":   {},
	"archived": {},
}

var allowedParamTypes = map[string]struct{}{
	"integer": {},
	"number":  {},
	"string":  {},
	"boolean": {},
}

var allowedImplementationKeys = map[string]struct{}{
	"trend_cross":         {},
	"grid":                {},
	"bollinger_reversion": {},
	"rsi_range":           {},
}

type Service struct {
	repo *Repository
}

type RuntimeStrategy struct {
	ID                string         `json:"id"`
	Key               string         `json:"key"`
	Name              string         `json:"name"`
	ImplementationKey string         `json:"implementation_key"`
	Params            map[string]any `json:"params"`
}

type StrategySummary struct {
	ID                string `json:"id"`
	Key               string `json:"key"`
	Name              string `json:"name"`
	Category          string `json:"category"`
	Status            string `json:"status"`
	Description       string `json:"description"`
	DescriptionSummary string `json:"description_summary"`
	ImplementationKey string `json:"implementation_key"`
	Version           int    `json:"version"`
	UpdatedAt         string `json:"updated_at"`
}

type seedDocument struct {
	Items []StrategyPayload `json:"items"`
}

func NewService(repo *Repository) *Service {
	return &Service{repo: repo}
}

func (s *Service) List(ctx context.Context, userID string, activeOnly bool, includeSystem bool) ([]*Strategy, error) {
	return s.repo.List(ctx, userID, activeOnly, includeSystem)
}

func (s *Service) ListSummaries(ctx context.Context, userID string) ([]StrategySummary, error) {
	includeSystem := strings.TrimSpace(userID) != ""
	items, err := s.repo.List(ctx, userID, false, includeSystem)
	if err != nil {
		return nil, err
	}
	summaries := make([]StrategySummary, 0, len(items))
	for _, item := range items {
		description := item.Description
		summary := description
		if len([]rune(summary)) > 72 {
			summary = string([]rune(summary)[:72])
		}
		summaries = append(summaries, StrategySummary{
			ID:                 item.ID,
			Key:                item.Key,
			Name:               item.Name,
			Category:           item.Category,
			Status:             item.Status,
			Description:        description,
			DescriptionSummary: summary,
			ImplementationKey:  item.ImplementationKey,
			Version:            item.Version,
			UpdatedAt:          item.UpdatedAt,
		})
	}
	return summaries, nil
}

func (s *Service) ImplementationKeys() []string {
	keys := make([]string, 0, len(allowedImplementationKeys))
	for key := range allowedImplementationKeys {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func (s *Service) GetByID(ctx context.Context, userID string, strategyID string) (*Strategy, error) {
	if strings.TrimSpace(strategyID) == "" {
		return nil, ErrInvalid
	}
	includeSystem := strings.TrimSpace(userID) != ""
	return s.repo.GetByID(ctx, strategyID, userID, includeSystem)
}

func (s *Service) Create(ctx context.Context, userID string, payload StrategyPayload) (*Strategy, error) {
	if strings.TrimSpace(userID) == "" {
		return nil, ErrForbidden
	}
	normalized, err := s.normalizePayload(payload, nil)
	if err != nil {
		return nil, err
	}
	return s.repo.Create(ctx, userID, normalized)
}

func (s *Service) Update(ctx context.Context, userID string, strategyID string, payload StrategyPayload) (*Strategy, error) {
	existing, err := s.repo.GetByID(ctx, strategyID, userID, false)
	if err != nil {
		return nil, err
	}
	normalized, err := s.normalizePayload(payload, existing)
	if err != nil {
		return nil, err
	}
	normalized.ID = existing.ID
	return s.repo.Update(ctx, existing.ID, userID, normalized)
}

func (s *Service) BuildRuntimeStrategy(ctx context.Context, userID string, strategyID string, strategyName string, overrideParams map[string]any) (*RuntimeStrategy, error) {
	var selected *Strategy
	var err error
	if strategyID != "" {
		selected, err = s.repo.GetByID(ctx, strategyID, userID, strings.TrimSpace(userID) != "")
	} else if strategyName != "" {
		selected, err = s.findByNameOrAlias(ctx, userID, strategyName)
	} else {
		return nil, fmt.Errorf("strategy_id or strategy_name is required")
	}
	if err != nil {
		return nil, err
	}
	if selected.Status != "active" {
		return nil, fmt.Errorf("策略 %s 当前不是启用状态，无法用于回测", selected.Name)
	}

	merged := copyMap(selected.DefaultParams)
	for key, value := range overrideParams {
		merged[key] = value
	}

	normalized, err := normalizeParams(selected.ParamSchema, merged)
	if err != nil {
		return nil, err
	}
	return &RuntimeStrategy{
		ID:                selected.ID,
		Key:               selected.Key,
		Name:              selected.Name,
		ImplementationKey: selected.ImplementationKey,
		Params:            normalized,
	}, nil
}

func (s *Service) SeedFromFileIfEmpty(ctx context.Context, seedPath string) error {
	count, err := s.repo.CountSystem(ctx)
	if err != nil {
		return err
	}
	if count > 0 {
		return nil
	}
	if strings.TrimSpace(seedPath) == "" {
		return nil
	}

	raw, err := os.ReadFile(seedPath)
	if err != nil {
		return fmt.Errorf("read seed file failed: %w", err)
	}

	var doc seedDocument
	if err := json.Unmarshal(raw, &doc); err != nil {
		return fmt.Errorf("decode seed file failed: %w", err)
	}
	for _, item := range doc.Items {
		if _, err := s.repo.Create(ctx, "", item); err != nil {
			return fmt.Errorf("seed strategy %s failed: %w", item.ID, err)
		}
	}
	return nil
}

func (s *Service) normalizePayload(payload StrategyPayload, existing *Strategy) (StrategyPayload, error) {
	payload.ID = strings.TrimSpace(payload.ID)
	payload.Key = strings.TrimSpace(payload.Key)
	payload.Name = strings.TrimSpace(payload.Name)
	payload.Description = strings.TrimSpace(payload.Description)
	payload.Category = strings.TrimSpace(payload.Category)
	payload.ImplementationKey = strings.TrimSpace(payload.ImplementationKey)
	payload.Status = strings.ToLower(strings.TrimSpace(payload.Status))

	if existing == nil {
		if payload.ID == "" {
			return StrategyPayload{}, fmt.Errorf("%w: 策略 ID 不能为空", ErrInvalid)
		}
	} else if payload.ID != "" && payload.ID != existing.ID {
		return StrategyPayload{}, fmt.Errorf("%w: 更新策略时不允许修改策略 ID", ErrInvalid)
	}

	if payload.Key == "" {
		return StrategyPayload{}, fmt.Errorf("%w: 策略 key 不能为空", ErrInvalid)
	}
	if payload.Name == "" {
		return StrategyPayload{}, fmt.Errorf("%w: 策略名称不能为空", ErrInvalid)
	}
	if payload.Category == "" {
		payload.Category = "通用"
	}
	if payload.Status == "" {
		payload.Status = "draft"
	}
	if _, ok := allowedStatuses[payload.Status]; !ok {
		return StrategyPayload{}, fmt.Errorf("%w: 不支持的策略状态: %s", ErrInvalid, payload.Status)
	}
	if payload.ImplementationKey == "" {
		return StrategyPayload{}, fmt.Errorf("%w: 执行映射 key 不能为空", ErrInvalid)
	}
	if existing != nil && payload.ImplementationKey != existing.ImplementationKey {
		return StrategyPayload{}, fmt.Errorf("%w: 更新策略时不允许修改策略类型", ErrInvalid)
	}
	if _, ok := allowedImplementationKeys[payload.ImplementationKey]; !ok {
		return StrategyPayload{}, fmt.Errorf("%w: 未注册的策略实现: %s", ErrInvalid, payload.ImplementationKey)
	}

	if payload.Version <= 0 {
		payload.Version = 1
	}
	if existing != nil && payload.Version <= existing.Version {
		payload.Version = existing.Version + 1
	}

	normalizedSchema, err := normalizeSchema(payload.ParamSchema)
	if err != nil {
		return StrategyPayload{}, err
	}
	normalizedDefaults, err := normalizeParams(normalizedSchema, payload.DefaultParams)
	if err != nil {
		return StrategyPayload{}, err
	}

	payload.ParamSchema = normalizedSchema
	payload.DefaultParams = normalizedDefaults
	payload.RequiredIndicators = ensureSliceMap(payload.RequiredIndicators)
	payload.ChartOverlays = ensureSliceMap(payload.ChartOverlays)
	payload.UISchema = ensureMap(payload.UISchema)
	payload.ExecutionOptions = ensureMap(payload.ExecutionOptions)
	payload.Metadata = ensureMap(payload.Metadata)
	return payload, nil
}

func (s *Service) findByNameOrAlias(ctx context.Context, userID string, strategyName string) (*Strategy, error) {
	strategyName = strings.TrimSpace(strategyName)
	if strategyName == "" {
		return nil, ErrInvalid
	}
	includeSystem := strings.TrimSpace(userID) != ""
	item, err := s.repo.GetByName(ctx, strategyName, userID, includeSystem)
	if err == nil {
		return item, nil
	}
	if !errors.Is(err, ErrNotFound) {
		return nil, err
	}

	items, listErr := s.repo.List(ctx, userID, false, includeSystem)
	if listErr != nil {
		return nil, listErr
	}
	normalizedTarget := normalizeStrategyName(strategyName)
	for _, candidate := range items {
		if normalizeStrategyName(candidate.Name) == normalizedTarget {
			return candidate, nil
		}
		aliases, ok := candidate.Metadata["aliases"].([]any)
		if !ok {
			continue
		}
		for _, alias := range aliases {
			text, ok := alias.(string)
			if ok && normalizeStrategyName(text) == normalizedTarget {
				return candidate, nil
			}
		}
	}
	return nil, ErrNotFound
}

func normalizeSchema(items []ParamSchemaItem) ([]ParamSchemaItem, error) {
	if items == nil {
		return []ParamSchemaItem{}, nil
	}
	seen := map[string]struct{}{}
	normalized := make([]ParamSchemaItem, 0, len(items))
	for _, item := range items {
		key := strings.TrimSpace(item.Key)
		if key == "" {
			return nil, fmt.Errorf("%w: 参数 key 不能为空", ErrInvalid)
		}
		if _, ok := seen[key]; ok {
			return nil, fmt.Errorf("%w: 参数 key 重复: %s", ErrInvalid, key)
		}
		seen[key] = struct{}{}

		itemType := strings.TrimSpace(strings.ToLower(item.Type))
		if itemType == "" {
			itemType = "number"
		}
		if _, ok := allowedParamTypes[itemType]; !ok {
			return nil, fmt.Errorf("%w: 不支持的参数类型: %s", ErrInvalid, itemType)
		}

		label := strings.TrimSpace(item.Label)
		if label == "" {
			label = key
		}

		normalized = append(normalized, ParamSchemaItem{
			Key:         key,
			Label:       label,
			Type:        itemType,
			Required:    item.Required,
			Default:     item.Default,
			Min:         item.Min,
			Max:         item.Max,
			Step:        item.Step,
			Description: strings.TrimSpace(item.Description),
			Options:     ensureSliceMap(item.Options),
		})
	}
	return normalized, nil
}

func normalizeParams(schema []ParamSchemaItem, params map[string]any) (map[string]any, error) {
	if params == nil {
		params = map[string]any{}
	}
	if len(schema) == 0 {
		return copyMap(params), nil
	}

	schemaMap := make(map[string]ParamSchemaItem, len(schema))
	for _, item := range schema {
		schemaMap[item.Key] = item
	}
	for key := range params {
		if _, ok := schemaMap[key]; !ok {
			return nil, fmt.Errorf("%w: 存在未定义的策略参数: %s", ErrInvalid, key)
		}
	}

	normalized := make(map[string]any, len(schema))
	for _, item := range schema {
		raw, exists := params[item.Key]
		if !exists {
			raw = item.Default
		}
		if raw == nil {
			if item.Required {
				return nil, fmt.Errorf("%w: 缺少必填参数: %s", ErrInvalid, item.Label)
			}
			continue
		}

		value, err := coerceParamValue(item, raw)
		if err != nil {
			return nil, err
		}
		normalized[item.Key] = value
	}
	return normalized, nil
}

func coerceParamValue(item ParamSchemaItem, raw any) (any, error) {
	switch item.Type {
	case "integer":
		value, err := toInt(raw)
		if err != nil {
			return nil, fmt.Errorf("%w: 参数 %s 的值格式不正确", ErrInvalid, item.Label)
		}
		if item.Min != nil && float64(value) < *item.Min {
			return nil, fmt.Errorf("%w: 参数 %s 不能小于 %v", ErrInvalid, item.Label, *item.Min)
		}
		if item.Max != nil && float64(value) > *item.Max {
			return nil, fmt.Errorf("%w: 参数 %s 不能大于 %v", ErrInvalid, item.Label, *item.Max)
		}
		return value, nil
	case "number":
		value, err := toFloat(raw)
		if err != nil {
			return nil, fmt.Errorf("%w: 参数 %s 的值格式不正确", ErrInvalid, item.Label)
		}
		if item.Min != nil && value < *item.Min {
			return nil, fmt.Errorf("%w: 参数 %s 不能小于 %v", ErrInvalid, item.Label, *item.Min)
		}
		if item.Max != nil && value > *item.Max {
			return nil, fmt.Errorf("%w: 参数 %s 不能大于 %v", ErrInvalid, item.Label, *item.Max)
		}
		return value, nil
	case "boolean":
		value, err := toBool(raw)
		if err != nil {
			return nil, fmt.Errorf("%w: 参数 %s 的值格式不正确", ErrInvalid, item.Label)
		}
		return value, nil
	default:
		return fmt.Sprintf("%v", raw), nil
	}
}

func ensureMap(input map[string]any) map[string]any {
	if input == nil {
		return map[string]any{}
	}
	return input
}

func ensureSliceMap(input []map[string]any) []map[string]any {
	if input == nil {
		return []map[string]any{}
	}
	return input
}

func copyMap(input map[string]any) map[string]any {
	if input == nil {
		return map[string]any{}
	}
	out := make(map[string]any, len(input))
	for key, value := range input {
		out[key] = value
	}
	return out
}

func toFloat(value any) (float64, error) {
	switch v := value.(type) {
	case float64:
		return v, nil
	case float32:
		return float64(v), nil
	case int:
		return float64(v), nil
	case int64:
		return float64(v), nil
	case int32:
		return float64(v), nil
	case int16:
		return float64(v), nil
	case int8:
		return float64(v), nil
	case uint:
		return float64(v), nil
	case uint64:
		return float64(v), nil
	case uint32:
		return float64(v), nil
	case uint16:
		return float64(v), nil
	case uint8:
		return float64(v), nil
	case json.Number:
		return v.Float64()
	case string:
		return strconv.ParseFloat(strings.TrimSpace(v), 64)
	default:
		return 0, fmt.Errorf("unsupported number type")
	}
}

func toInt(value any) (int, error) {
	number, err := toFloat(value)
	if err != nil {
		return 0, err
	}
	if math.Mod(number, 1) != 0 {
		return 0, fmt.Errorf("not integer")
	}
	return int(number), nil
}

func toBool(value any) (bool, error) {
	switch v := value.(type) {
	case bool:
		return v, nil
	case string:
		switch strings.ToLower(strings.TrimSpace(v)) {
		case "true", "1", "yes", "on":
			return true, nil
		case "false", "0", "no", "off":
			return false, nil
		default:
			return false, fmt.Errorf("unsupported bool string")
		}
	default:
		return false, fmt.Errorf("unsupported bool type")
	}
}

func normalizeStrategyName(name string) string {
	replacer := strings.NewReplacer("（", "(", "）", ")", " ", "")
	return strings.ToLower(replacer.Replace(strings.TrimSpace(name)))
}
