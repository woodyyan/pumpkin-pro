package signal

import (
	"context"
	"errors"
	"strings"
	"time"

	"gorm.io/gorm"
)

// ──────────────────────────────────────────────────────────────
// 信号订阅的持久化操作。
//
// 注意（BP：GORM 零值坑）：
//  - PositionAwareEnabled 带 default:true，Create 传 false 会被 DB 默认值覆盖，
//    必须同事务显式 map UPDATE 回写。
//  - 更新一律用 map 显式列出所有列，允许业务显式写 0/false 关闭规则。
// ──────────────────────────────────────────────────────────────

func (r *Repository) CreateSubscription(ctx context.Context, record SignalSubscriptionRecord) (*SignalSubscriptionRecord, error) {
	desiredPositionAware := record.PositionAwareEnabled
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&record).Error; err != nil {
			return translateWriteError(err)
		}
		if err := tx.Model(&SignalSubscriptionRecord{}).Where("id = ?", record.ID).Updates(map[string]any{
			"position_aware_enabled": desiredPositionAware,
		}).Error; err != nil {
			return translateWriteError(err)
		}
		record.PositionAwareEnabled = desiredPositionAware
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &record, nil
}

func (r *Repository) GetSubscriptionByID(ctx context.Context, userID, id string) (*SignalSubscriptionRecord, error) {
	var record SignalSubscriptionRecord
	if err := r.db.WithContext(ctx).Where("id = ? AND user_id = ?", id, userID).First(&record).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &record, nil
}

// GetSubscriptionByUniqueKey 按五元组唯一键查找（迁移幂等 / 创建前查重）。
func (r *Repository) GetSubscriptionByUniqueKey(ctx context.Context, userID, templateKey, strategyID, scopeType, symbol string) (*SignalSubscriptionRecord, error) {
	var record SignalSubscriptionRecord
	err := r.db.WithContext(ctx).
		Where("user_id = ? AND template_key = ? AND strategy_id = ? AND scope_type = ? AND symbol = ?",
			userID, templateKey, strategyID, scopeType, symbol).
		First(&record).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &record, nil
}

func (r *Repository) ListSubscriptions(ctx context.Context, userID string) ([]SignalSubscriptionRecord, error) {
	var records []SignalSubscriptionRecord
	if err := r.db.WithContext(ctx).
		Where("user_id = ?", userID).
		Order("symbol ASC, template_key ASC").
		Find(&records).Error; err != nil {
		return nil, err
	}
	return records, nil
}

// ListEnabledSubscriptions 评估器使用：全部用户的启用订阅。
func (r *Repository) ListEnabledSubscriptions(ctx context.Context) ([]SignalSubscriptionRecord, error) {
	var records []SignalSubscriptionRecord
	if err := r.db.WithContext(ctx).
		Where("is_enabled = ?", true).
		Find(&records).Error; err != nil {
		return nil, err
	}
	return records, nil
}

// UpdateSubscription 按 ID 更新可变字段（map 形式显式更新，允许写 0/false）。
func (r *Repository) UpdateSubscription(ctx context.Context, userID, id string, updates map[string]any) (*SignalSubscriptionRecord, error) {
	if len(updates) == 0 {
		return r.GetSubscriptionByID(ctx, userID, id)
	}
	updates["updated_at"] = time.Now().UTC()
	result := r.db.WithContext(ctx).
		Model(&SignalSubscriptionRecord{}).
		Where("id = ? AND user_id = ?", id, userID).
		Updates(updates)
	if result.Error != nil {
		return nil, translateWriteError(result.Error)
	}
	if result.RowsAffected == 0 {
		return nil, ErrNotFound
	}
	return r.GetSubscriptionByID(ctx, userID, id)
}

func (r *Repository) DeleteSubscription(ctx context.Context, userID, id string) error {
	result := r.db.WithContext(ctx).
		Where("id = ? AND user_id = ?", id, userID).
		Delete(&SignalSubscriptionRecord{})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *Repository) CountSubscriptionsByStrategy(ctx context.Context, userID, strategyID string) (int64, error) {
	var count int64
	if err := r.db.WithContext(ctx).
		Model(&SignalSubscriptionRecord{}).
		Where("user_id = ? AND strategy_id = ?", userID, strategyID).
		Count(&count).Error; err != nil {
		return 0, err
	}
	return count, nil
}

// ListSubscriptionRefsByStrategy 返回引用某策略的订阅标的（含名称，供删除确认弹窗）。
func (r *Repository) ListSubscriptionRefsByStrategy(ctx context.Context, userID, strategyID string) ([]SymbolRef, error) {
	type result struct {
		Symbol string
		Name   string
	}
	var rows []result
	if err := r.db.WithContext(ctx).
		Table("signal_subscriptions ss").
		Select("ss.symbol as symbol, COALESCE(qs.name, '') as name").
		Joins("LEFT JOIN quadrant_scores qs ON (qs.code = ss.symbol OR qs.code = CASE WHEN instr(ss.symbol, '.') > 0 THEN substr(ss.symbol, 1, instr(ss.symbol, '.') - 1) ELSE ss.symbol END)").
		Where("ss.user_id = ? AND ss.strategy_id = ?", userID, strategyID).
		Order("ss.symbol ASC").
		Scan(&rows).Error; err != nil {
		return nil, err
	}
	refs := make([]SymbolRef, 0, len(rows))
	for _, row := range rows {
		refs = append(refs, SymbolRef{Symbol: row.Symbol, Name: row.Name})
	}
	return refs, nil
}

func (r *Repository) UpdateSubscriptionLastEvaluatedAt(ctx context.Context, id string, at time.Time) error {
	return r.db.WithContext(ctx).
		Model(&SignalSubscriptionRecord{}).
		Where("id = ?", id).
		Update("last_evaluated_at", at.UTC()).Error
}

// ListAllSymbolConfigs 迁移使用：全量旧配置（不限用户）。
func (r *Repository) ListAllSymbolConfigs(ctx context.Context) ([]SymbolSignalConfigRecord, error) {
	var records []SymbolSignalConfigRecord
	if err := r.db.WithContext(ctx).Find(&records).Error; err != nil {
		return nil, err
	}
	return records, nil
}

// ── 信号事件：信号中心新增查询 ──

// ListSignalEventsFiltered 支持 bar_state / side 过滤的事件流。
func (r *Repository) ListSignalEventsFiltered(ctx context.Context, userID, symbol, barState, side string, limit int) ([]SignalEventRecord, error) {
	if limit <= 0 {
		limit = 50
	}
	if limit > 200 {
		limit = 200
	}

	query := r.db.WithContext(ctx).Where("user_id = ? AND is_test = ?", userID, false)
	if strings.TrimSpace(symbol) != "" {
		query = query.Where("symbol = ?", symbol)
	}
	if strings.TrimSpace(barState) != "" {
		query = query.Where("bar_state = ?", barState)
	}
	if strings.TrimSpace(side) != "" {
		query = query.Where("side = ?", strings.ToUpper(strings.TrimSpace(side)))
	}

	var records []SignalEventRecord
	if err := query.Order("event_time DESC").Limit(limit).Find(&records).Error; err != nil {
		return nil, err
	}
	return records, nil
}

// CountUnreadEvents 站内未读信号数（导航角标）。
func (r *Repository) CountUnreadEvents(ctx context.Context, userID string) (int64, error) {
	var count int64
	if err := r.db.WithContext(ctx).
		Model(&SignalEventRecord{}).
		Where("user_id = ? AND is_test = ? AND is_read = ?", userID, false, false).
		Count(&count).Error; err != nil {
		return 0, err
	}
	return count, nil
}

// MarkEventsRead 标记已读；symbol 为空时标记该用户全部。
func (r *Repository) MarkEventsRead(ctx context.Context, userID, symbol string) (int64, error) {
	query := r.db.WithContext(ctx).
		Model(&SignalEventRecord{}).
		Where("user_id = ? AND is_read = ?", userID, false)
	if strings.TrimSpace(symbol) != "" {
		query = query.Where("symbol = ?", symbol)
	}
	result := query.Updates(map[string]any{"is_read": true})
	if result.Error != nil {
		return 0, result.Error
	}
	return result.RowsAffected, nil
}

// ResolveSymbolNames 批量解析股票名称（quadrant_scores 左连，缺失时返回空串）。
func (r *Repository) ResolveSymbolNames(ctx context.Context, symbols []string) (map[string]string, error) {
	result := map[string]string{}
	if len(symbols) == 0 {
		return result, nil
	}
	type row struct {
		Code string
		Name string
	}
	var rows []row
	if err := r.db.WithContext(ctx).
		Table("quadrant_scores").
		Select("code, name").
		Where("code IN ?", symbols).
		Group("code").
		Scan(&rows).Error; err != nil {
		return nil, err
	}
	for _, item := range rows {
		result[item.Code] = item.Name
	}
	// quadrant_scores.code 可能是不带后缀的代码，补一轮裸代码匹配。
	missing := make([]string, 0)
	for _, symbol := range symbols {
		if result[symbol] == "" {
			missing = append(missing, bareSymbolCode(symbol))
		}
	}
	if len(missing) > 0 {
		var bareRows []row
		if err := r.db.WithContext(ctx).
			Table("quadrant_scores").
			Select("code, name").
			Where("code IN ?", missing).
			Group("code").
			Scan(&bareRows).Error; err == nil {
			bareNames := map[string]string{}
			for _, item := range bareRows {
				bareNames[item.Code] = item.Name
			}
			for _, symbol := range symbols {
				if result[symbol] == "" {
					result[symbol] = bareNames[bareSymbolCode(symbol)]
				}
			}
		}
	}
	return result, nil
}

func bareSymbolCode(symbol string) string {
	trimmed := strings.TrimSpace(symbol)
	if idx := strings.Index(trimmed, "."); idx > 0 {
		return trimmed[:idx]
	}
	return trimmed
}

// GetLastSignalEventTimeBySubscription 冷却判定：同一订阅同一 bar 状态的最近一次事件时间。
// bar_state 维度隔离：盘中试算的冷却不阻塞收盘确认。
func (r *Repository) GetLastSignalEventTimeBySubscription(ctx context.Context, userID, subscriptionID, barState string) (*time.Time, error) {
	var record SignalEventRecord
	err := r.db.WithContext(ctx).
		Where("user_id = ? AND subscription_id = ? AND bar_state = ? AND is_test = ?", userID, subscriptionID, barState, false).
		Order("event_time DESC").
		First(&record).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	t := record.EventTime.UTC()
	return &t, nil
}
