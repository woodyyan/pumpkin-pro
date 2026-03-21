package signal

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"gorm.io/gorm"
)

type Repository struct {
	db *gorm.DB
}

func NewRepository(db *gorm.DB) *Repository {
	return &Repository{db: db}
}

func (r *Repository) GetWebhookEndpoint(ctx context.Context, userID string) (*WebhookEndpointRecord, error) {
	var record WebhookEndpointRecord
	if err := r.db.WithContext(ctx).Where("user_id = ?", userID).First(&record).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &record, nil
}

func (r *Repository) SaveWebhookEndpoint(ctx context.Context, record WebhookEndpointRecord) (*WebhookEndpointRecord, error) {
	var saved WebhookEndpointRecord
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var existing WebhookEndpointRecord
		err := tx.Where("user_id = ?", record.UserID).First(&existing).Error
		if err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				if err := tx.Create(&record).Error; err != nil {
					return translateWriteError(err)
				}
				saved = record
				return nil
			}
			return err
		}

		existing.URL = record.URL
		existing.SecretCipherText = record.SecretCipherText
		existing.IsEnabled = record.IsEnabled
		existing.TimeoutMS = record.TimeoutMS
		existing.UpdatedAt = record.UpdatedAt

		if err := tx.Model(&WebhookEndpointRecord{}).Where("id = ?", existing.ID).Updates(map[string]any{
			"url":                existing.URL,
			"secret_cipher_text": existing.SecretCipherText,
			"is_enabled":         existing.IsEnabled,
			"timeout_ms":         existing.TimeoutMS,
			"updated_at":         existing.UpdatedAt,
		}).Error; err != nil {
			return translateWriteError(err)
		}
		saved = existing
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &saved, nil
}

func (r *Repository) ListSymbolConfigs(ctx context.Context, userID string) ([]SymbolSignalConfigRecord, error) {
	var records []SymbolSignalConfigRecord
	if err := r.db.WithContext(ctx).
		Where("user_id = ?", userID).
		Order("updated_at DESC").
		Find(&records).Error; err != nil {
		return nil, err
	}
	return records, nil
}

func (r *Repository) GetSymbolConfig(ctx context.Context, userID, symbol string) (*SymbolSignalConfigRecord, error) {
	var record SymbolSignalConfigRecord
	if err := r.db.WithContext(ctx).Where("user_id = ? AND symbol = ?", userID, symbol).First(&record).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &record, nil
}

func (r *Repository) SaveSymbolConfig(ctx context.Context, record SymbolSignalConfigRecord) (*SymbolSignalConfigRecord, error) {
	var saved SymbolSignalConfigRecord
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var existing SymbolSignalConfigRecord
		err := tx.Where("user_id = ? AND symbol = ?", record.UserID, record.Symbol).First(&existing).Error
		if err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				if err := tx.Create(&record).Error; err != nil {
					return translateWriteError(err)
				}
				saved = record
				return nil
			}
			return err
		}

		existing.StrategyID = record.StrategyID
		existing.IsEnabled = record.IsEnabled
		existing.CooldownSeconds = record.CooldownSeconds
		existing.ThresholdsJSON = record.ThresholdsJSON
		existing.UpdatedAt = record.UpdatedAt

		if err := tx.Model(&SymbolSignalConfigRecord{}).Where("id = ?", existing.ID).Updates(map[string]any{
			"strategy_id":      existing.StrategyID,
			"is_enabled":       existing.IsEnabled,
			"cooldown_seconds": existing.CooldownSeconds,
			"thresholds_json":  existing.ThresholdsJSON,
			"updated_at":       existing.UpdatedAt,
		}).Error; err != nil {
			return translateWriteError(err)
		}
		saved = existing
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &saved, nil
}

func (r *Repository) DeleteSymbolConfig(ctx context.Context, userID, symbol string) error {
	result := r.db.WithContext(ctx).Where("user_id = ? AND symbol = ?", userID, symbol).Delete(&SymbolSignalConfigRecord{})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *Repository) CountSymbolConfigsByStrategy(ctx context.Context, userID, strategyID string) (int64, error) {
	var count int64
	if err := r.db.WithContext(ctx).
		Model(&SymbolSignalConfigRecord{}).
		Where("user_id = ? AND strategy_id = ?", userID, strategyID).
		Count(&count).Error; err != nil {
		return 0, err
	}
	return count, nil
}

func (r *Repository) CreateEventWithDelivery(ctx context.Context, event SignalEventRecord, delivery WebhookDeliveryRecord) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&event).Error; err != nil {
			return translateWriteError(err)
		}
		if err := tx.Create(&delivery).Error; err != nil {
			return translateWriteError(err)
		}
		return nil
	})
}

func (r *Repository) GetSignalEventByEventID(ctx context.Context, eventID string) (*SignalEventRecord, error) {
	var record SignalEventRecord
	if err := r.db.WithContext(ctx).Where("event_id = ?", eventID).First(&record).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &record, nil
}

func (r *Repository) ListSignalEvents(ctx context.Context, userID, symbol string, limit int) ([]SignalEventRecord, error) {
	if limit <= 0 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}

	query := r.db.WithContext(ctx).Where("user_id = ?", userID)
	if strings.TrimSpace(symbol) != "" {
		query = query.Where("symbol = ?", symbol)
	}

	var records []SignalEventRecord
	if err := query.Order("event_time DESC").Limit(limit).Find(&records).Error; err != nil {
		return nil, err
	}
	return records, nil
}

func (r *Repository) ListDeliveries(ctx context.Context, userID, symbol string, limit int) ([]WebhookDeliveryRecord, error) {
	if limit <= 0 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}

	query := r.db.WithContext(ctx).Model(&WebhookDeliveryRecord{}).Where("user_id = ?", userID)
	if strings.TrimSpace(symbol) != "" {
		subQuery := r.db.WithContext(ctx).Model(&SignalEventRecord{}).Select("event_id").Where("user_id = ? AND symbol = ?", userID, symbol)
		query = query.Where("event_id IN (?)", subQuery)
	}

	var records []WebhookDeliveryRecord
	if err := query.Order("updated_at DESC").Limit(limit).Find(&records).Error; err != nil {
		return nil, err
	}
	return records, nil
}

func (r *Repository) GetLatestDelivery(ctx context.Context, userID string) (*WebhookDeliveryRecord, error) {
	var record WebhookDeliveryRecord
	if err := r.db.WithContext(ctx).Where("user_id = ?", userID).Order("updated_at DESC").First(&record).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &record, nil
}

func (r *Repository) GetLatestDeliveryByEventID(ctx context.Context, eventID string) (*WebhookDeliveryRecord, error) {
	var record WebhookDeliveryRecord
	if err := r.db.WithContext(ctx).Where("event_id = ?", eventID).Order("updated_at DESC").First(&record).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &record, nil
}

func (r *Repository) GetDeliveryByID(ctx context.Context, deliveryID string) (*WebhookDeliveryRecord, error) {
	var record WebhookDeliveryRecord
	if err := r.db.WithContext(ctx).Where("id = ?", deliveryID).First(&record).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &record, nil
}

func (r *Repository) ListDueDeliveries(ctx context.Context, now time.Time, limit int) ([]WebhookDeliveryRecord, error) {
	if limit <= 0 {
		limit = 20
	}
	if limit > 200 {
		limit = 200
	}

	var records []WebhookDeliveryRecord
	if err := r.db.WithContext(ctx).
		Where("status IN ?", []string{"pending", "retrying"}).
		Where("next_retry_at IS NULL OR next_retry_at <= ?", now.UTC()).
		Order("updated_at ASC").
		Limit(limit).
		Find(&records).Error; err != nil {
		return nil, err
	}
	return records, nil
}

func (r *Repository) ClaimDelivery(ctx context.Context, deliveryID string, now time.Time) (bool, error) {
	result := r.db.WithContext(ctx).
		Model(&WebhookDeliveryRecord{}).
		Where("id = ? AND status IN ?", deliveryID, []string{"pending", "retrying"}).
		Updates(map[string]any{"status": "processing", "updated_at": now.UTC()})
	if result.Error != nil {
		return false, result.Error
	}
	return result.RowsAffected > 0, nil
}

func (r *Repository) MarkDeliveryDelivered(ctx context.Context, deliveryID string, httpStatus int, latencyMS int64, attemptedAt time.Time) error {
	updates := map[string]any{
		"status":          "delivered",
		"http_status":     httpStatus,
		"latency_ms":      latencyMS,
		"error_message":   "",
		"next_retry_at":   nil,
		"last_attempt_at": attemptedAt.UTC(),
		"delivered_at":    attemptedAt.UTC(),
		"updated_at":      attemptedAt.UTC(),
	}
	return r.db.WithContext(ctx).Model(&WebhookDeliveryRecord{}).Where("id = ?", deliveryID).Updates(updates).Error
}

func (r *Repository) MarkDeliveryRetry(ctx context.Context, deliveryID string, nextAttempt int, nextRetryAt time.Time, httpStatus int, latencyMS int64, errMsg string, attemptedAt time.Time) error {
	updates := map[string]any{
		"status":          "retrying",
		"attempt_no":      nextAttempt,
		"http_status":     httpStatus,
		"latency_ms":      latencyMS,
		"error_message":   trimError(errMsg),
		"next_retry_at":   nextRetryAt.UTC(),
		"last_attempt_at": attemptedAt.UTC(),
		"updated_at":      attemptedAt.UTC(),
	}
	return r.db.WithContext(ctx).Model(&WebhookDeliveryRecord{}).Where("id = ?", deliveryID).Updates(updates).Error
}

func (r *Repository) MarkDeliveryFailed(ctx context.Context, deliveryID string, httpStatus int, latencyMS int64, errMsg string, attemptedAt time.Time) error {
	updates := map[string]any{
		"status":          "failed",
		"http_status":     httpStatus,
		"latency_ms":      latencyMS,
		"error_message":   trimError(errMsg),
		"next_retry_at":   nil,
		"last_attempt_at": attemptedAt.UTC(),
		"updated_at":      attemptedAt.UTC(),
	}
	return r.db.WithContext(ctx).Model(&WebhookDeliveryRecord{}).Where("id = ?", deliveryID).Updates(updates).Error
}

func trimError(errMsg string) string {
	text := strings.TrimSpace(errMsg)
	if len(text) <= 1000 {
		return text
	}
	return text[:1000]
}

func translateWriteError(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, gorm.ErrDuplicatedKey) {
		return ErrConflict
	}
	message := strings.ToLower(strings.TrimSpace(err.Error()))
	if strings.Contains(message, "unique") || strings.Contains(message, "duplicate") {
		return ErrConflict
	}
	return fmt.Errorf("write signal failed: %w", err)
}
