package strategy

import (
	"encoding/json"
	"fmt"
	"time"
)

type StrategyRecord struct {
	ID                string    `gorm:"primaryKey;size:128"`
	Key               string    `gorm:"size:128;not null;uniqueIndex"`
	Name              string    `gorm:"size:255;not null"`
	Description       string    `gorm:"type:text;not null"`
	Category          string    `gorm:"size:64;not null"`
	ImplementationKey string    `gorm:"size:128;not null;index"`
	Status            string    `gorm:"size:32;not null;index"`
	Version           int       `gorm:"not null;default:1"`
	ParamSchemaJSON   string    `gorm:"type:text;not null"`
	DefaultParamsJSON string    `gorm:"type:text;not null"`
	RequiredIndicatorsJSON string `gorm:"type:text;not null"`
	ChartOverlaysJSON string    `gorm:"type:text;not null"`
	UISchemaJSON      string    `gorm:"type:text;not null"`
	ExecutionOptionsJSON string `gorm:"type:text;not null"`
	MetadataJSON      string    `gorm:"type:text;not null"`
	CreatedAt         time.Time `gorm:"not null"`
	UpdatedAt         time.Time `gorm:"not null"`
}

func (StrategyRecord) TableName() string {
	return "strategy_definitions"
}

type Strategy struct {
	ID                string           `json:"id"`
	Key               string           `json:"key"`
	Name              string           `json:"name"`
	Description       string           `json:"description"`
	Category          string           `json:"category"`
	ImplementationKey string           `json:"implementation_key"`
	Status            string           `json:"status"`
	Version           int              `json:"version"`
	CreatedAt         string           `json:"created_at"`
	UpdatedAt         string           `json:"updated_at"`
	ParamSchema       []ParamSchemaItem `json:"param_schema"`
	DefaultParams     map[string]any   `json:"default_params"`
	RequiredIndicators []map[string]any `json:"required_indicators"`
	ChartOverlays     []map[string]any `json:"chart_overlays"`
	UISchema          map[string]any   `json:"ui_schema"`
	ExecutionOptions  map[string]any   `json:"execution_options"`
	Metadata          map[string]any   `json:"metadata"`
}

type ParamSchemaItem struct {
	Key         string         `json:"key"`
	Label       string         `json:"label"`
	Type        string         `json:"type"`
	Required    bool           `json:"required"`
	Default     any            `json:"default,omitempty"`
	Min         *float64       `json:"min,omitempty"`
	Max         *float64       `json:"max,omitempty"`
	Step        *float64       `json:"step,omitempty"`
	Description string         `json:"description"`
	Options     []map[string]any `json:"options"`
}

type StrategyPayload struct {
	ID                string           `json:"id"`
	Key               string           `json:"key"`
	Name              string           `json:"name"`
	Description       string           `json:"description"`
	Category          string           `json:"category"`
	ImplementationKey string           `json:"implementation_key"`
	Status            string           `json:"status"`
	Version           int              `json:"version"`
	ParamSchema       []ParamSchemaItem `json:"param_schema"`
	DefaultParams     map[string]any   `json:"default_params"`
	RequiredIndicators []map[string]any `json:"required_indicators"`
	ChartOverlays     []map[string]any `json:"chart_overlays"`
	UISchema          map[string]any   `json:"ui_schema"`
	ExecutionOptions  map[string]any   `json:"execution_options"`
	Metadata          map[string]any   `json:"metadata"`
}

func (r StrategyRecord) toStrategy() (*Strategy, error) {
	paramSchema := make([]ParamSchemaItem, 0)
	defaultParams := make(map[string]any)
	requiredIndicators := make([]map[string]any, 0)
	chartOverlays := make([]map[string]any, 0)
	uiSchema := make(map[string]any)
	executionOptions := make(map[string]any)
	metadata := make(map[string]any)

	if err := unmarshalJSON(r.ParamSchemaJSON, &paramSchema); err != nil {
		return nil, fmt.Errorf("decode param_schema failed: %w", err)
	}
	if err := unmarshalJSON(r.DefaultParamsJSON, &defaultParams); err != nil {
		return nil, fmt.Errorf("decode default_params failed: %w", err)
	}
	if err := unmarshalJSON(r.RequiredIndicatorsJSON, &requiredIndicators); err != nil {
		return nil, fmt.Errorf("decode required_indicators failed: %w", err)
	}
	if err := unmarshalJSON(r.ChartOverlaysJSON, &chartOverlays); err != nil {
		return nil, fmt.Errorf("decode chart_overlays failed: %w", err)
	}
	if err := unmarshalJSON(r.UISchemaJSON, &uiSchema); err != nil {
		return nil, fmt.Errorf("decode ui_schema failed: %w", err)
	}
	if err := unmarshalJSON(r.ExecutionOptionsJSON, &executionOptions); err != nil {
		return nil, fmt.Errorf("decode execution_options failed: %w", err)
	}
	if err := unmarshalJSON(r.MetadataJSON, &metadata); err != nil {
		return nil, fmt.Errorf("decode metadata failed: %w", err)
	}

	return &Strategy{
		ID:                 r.ID,
		Key:                r.Key,
		Name:               r.Name,
		Description:        r.Description,
		Category:           r.Category,
		ImplementationKey:  r.ImplementationKey,
		Status:             r.Status,
		Version:            r.Version,
		CreatedAt:          r.CreatedAt.UTC().Format(time.RFC3339),
		UpdatedAt:          r.UpdatedAt.UTC().Format(time.RFC3339),
		ParamSchema:        paramSchema,
		DefaultParams:      defaultParams,
		RequiredIndicators: requiredIndicators,
		ChartOverlays:      chartOverlays,
		UISchema:           uiSchema,
		ExecutionOptions:   executionOptions,
		Metadata:           metadata,
	}, nil
}

func buildRecord(payload StrategyPayload, createdAt, updatedAt time.Time) (StrategyRecord, error) {
	paramSchemaJSON, err := marshalJSON(payload.ParamSchema, make([]ParamSchemaItem, 0))
	if err != nil {
		return StrategyRecord{}, fmt.Errorf("encode param_schema failed: %w", err)
	}
	defaultParamsJSON, err := marshalJSON(payload.DefaultParams, map[string]any{})
	if err != nil {
		return StrategyRecord{}, fmt.Errorf("encode default_params failed: %w", err)
	}
	requiredIndicatorsJSON, err := marshalJSON(payload.RequiredIndicators, make([]map[string]any, 0))
	if err != nil {
		return StrategyRecord{}, fmt.Errorf("encode required_indicators failed: %w", err)
	}
	chartOverlaysJSON, err := marshalJSON(payload.ChartOverlays, make([]map[string]any, 0))
	if err != nil {
		return StrategyRecord{}, fmt.Errorf("encode chart_overlays failed: %w", err)
	}
	uiSchemaJSON, err := marshalJSON(payload.UISchema, map[string]any{})
	if err != nil {
		return StrategyRecord{}, fmt.Errorf("encode ui_schema failed: %w", err)
	}
	executionOptionsJSON, err := marshalJSON(payload.ExecutionOptions, map[string]any{})
	if err != nil {
		return StrategyRecord{}, fmt.Errorf("encode execution_options failed: %w", err)
	}
	metadataJSON, err := marshalJSON(payload.Metadata, map[string]any{})
	if err != nil {
		return StrategyRecord{}, fmt.Errorf("encode metadata failed: %w", err)
	}

	return StrategyRecord{
		ID:                    payload.ID,
		Key:                   payload.Key,
		Name:                  payload.Name,
		Description:           payload.Description,
		Category:              payload.Category,
		ImplementationKey:     payload.ImplementationKey,
		Status:                payload.Status,
		Version:               payload.Version,
		ParamSchemaJSON:       paramSchemaJSON,
		DefaultParamsJSON:     defaultParamsJSON,
		RequiredIndicatorsJSON: requiredIndicatorsJSON,
		ChartOverlaysJSON:     chartOverlaysJSON,
		UISchemaJSON:          uiSchemaJSON,
		ExecutionOptionsJSON:  executionOptionsJSON,
		MetadataJSON:          metadataJSON,
		CreatedAt:             createdAt.UTC(),
		UpdatedAt:             updatedAt.UTC(),
	}, nil
}

func marshalJSON(value any, fallback any) (string, error) {
	target := value
	if target == nil {
		target = fallback
	}
	encoded, err := json.Marshal(target)
	if err != nil {
		return "", err
	}
	return string(encoded), nil
}

func unmarshalJSON(raw string, target any) error {
	if raw == "" {
		raw = "null"
	}
	return json.Unmarshal([]byte(raw), target)
}
