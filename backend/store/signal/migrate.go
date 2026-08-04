package signal

import (
	"context"
	"log"
	"time"

	"github.com/google/uuid"
)

// ──────────────────────────────────────────────────────────────
// 旧 symbol_signal_configs → 信号订阅的一次性迁移。
//
// 设计约束（spec signal-center T4）：
//  - 幂等：按五元组唯一键查重，已存在的跳过，可重复执行。
//  - 旧表只读保留：不更新、不删除 symbol_signal_configs，作为回滚预案。
//  - 对账：返回迁移前后行数/启用数，由调用方（main.go）打日志。
// ──────────────────────────────────────────────────────────────

// ConfigMigrationReport 迁移对账结果。
type ConfigMigrationReport struct {
	LegacyTotal   int `json:"legacy_total"`
	LegacyEnabled int `json:"legacy_enabled"`
	Created       int `json:"created"`
	Skipped       int `json:"skipped"`
	Failed        int `json:"failed"`
}

// MigrateSymbolConfigsToSubscriptions 把旧单股信号配置迁移为「策略信号」订阅。
// 旧配置的策略绑定、开关、频率、冷却与单票风控字段全量保留。
func (s *Service) MigrateSymbolConfigsToSubscriptions(ctx context.Context) (ConfigMigrationReport, error) {
	report := ConfigMigrationReport{}

	configs, err := s.repo.ListAllSymbolConfigs(ctx)
	if err != nil {
		return report, err
	}
	report.LegacyTotal = len(configs)

	for _, cfg := range configs {
		if cfg.IsEnabled {
			report.LegacyEnabled++
		}

		existing, lookupErr := s.repo.GetSubscriptionByUniqueKey(ctx, cfg.UserID, "strategy", cfg.StrategyID, ScopeTypeSymbol, cfg.Symbol)
		if lookupErr != nil && lookupErr != ErrNotFound {
			report.Failed++
			log.Printf("[signal-migrate] lookup failed user=%s symbol=%s: %v", cfg.UserID, cfg.Symbol, lookupErr)
			continue
		}
		if existing != nil {
			report.Skipped++
			continue
		}

		now := time.Now().UTC()
		record := SignalSubscriptionRecord{
			ID:                   uuid.NewString(),
			UserID:               cfg.UserID,
			TemplateKey:          "strategy",
			StrategyID:           cfg.StrategyID,
			ScopeType:            ScopeTypeSymbol,
			Symbol:               cfg.Symbol,
			ParamsJSON:           cfg.ThresholdsJSON,
			IsEnabled:            cfg.IsEnabled,
			EvalIntervalSeconds:  cfg.EvalIntervalSeconds,
			CooldownSeconds:      cfg.CooldownSeconds,
			PositionAwareEnabled: cfg.PositionAwareEnabled,
			MaxPositionPct:       cfg.MaxPositionPct,
			MaxAddTimes:          cfg.MaxAddTimes,
			StopLossPct:          cfg.StopLossPct,
			TrailingStopPct:      cfg.TrailingStopPct,
			LastEvaluatedAt:      cfg.LastEvaluatedAt,
			CreatedAt:            now,
			UpdatedAt:            now,
		}
		// 旧数据防御：阈值 JSON 损坏时迁移为空参数，不阻断整体迁移。
		if record.ParamsJSON == "" {
			record.ParamsJSON = "{}"
		}
		probe := map[string]any{}
		if decodeErr := decodeJSONMap(record.ParamsJSON, &probe); decodeErr != nil {
			record.ParamsJSON = "{}"
		}
		if record.EvalIntervalSeconds < minSubscriptionEvalIntervalSeconds || record.EvalIntervalSeconds > maxSubscriptionEvalIntervalSeconds {
			record.EvalIntervalSeconds = defaultSubscriptionEvalIntervalSeconds
		}
		if record.CooldownSeconds < minSubscriptionCooldownSeconds || record.CooldownSeconds > maxSubscriptionCooldownSeconds {
			record.CooldownSeconds = defaultCooldownSeconds
		}

		if _, createErr := s.repo.CreateSubscription(ctx, record); createErr != nil {
			report.Failed++
			log.Printf("[signal-migrate] create failed user=%s symbol=%s strategy=%s: %v", cfg.UserID, cfg.Symbol, cfg.StrategyID, createErr)
			continue
		}
		report.Created++
	}

	return report, nil
}
