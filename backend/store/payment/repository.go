package payment

import (
	"context"
	"errors"
	"strings"

	"gorm.io/gorm"
)

type Repository struct {
	db *gorm.DB
}

func NewRepository(db *gorm.DB) *Repository {
	return &Repository{db: db}
}

func (r *Repository) CreatePayment(ctx context.Context, record *PaymentRecord) error {
	normalizePaymentRecord(record)
	return r.db.WithContext(ctx).Create(record).Error
}

func (r *Repository) UpdatePayment(ctx context.Context, record *PaymentRecord) error {
	normalizePaymentRecord(record)
	result := r.db.WithContext(ctx).
		Model(&PaymentRecord{}).
		Where("id = ?", strings.TrimSpace(record.ID)).
		Updates(map[string]any{
			"provider":                record.Provider,
			"purpose":                 record.Purpose,
			"scenario_type":           record.ScenarioType,
			"mode":                    record.Mode,
			"status":                  record.Status,
			"trigger_admin_id":        record.TriggerAdminID,
			"user_id":                 record.UserID,
			"title":                   record.Title,
			"amount_minor":            record.AmountMinor,
			"currency":                record.Currency,
			"payment_method_request":  record.PaymentMethodRequest,
			"payment_method_selected": record.PaymentMethodSelected,
			"checkout_session_id":     record.CheckoutSessionID,
			"checkout_url":            record.CheckoutURL,
			"payment_intent_id":       record.PaymentIntentID,
			"charge_id":               record.ChargeID,
			"refund_id":               record.RefundID,
			"customer_id":             record.CustomerID,
			"success_url":             record.SuccessURL,
			"cancel_url":              record.CancelURL,
			"idempotency_key":         record.IdempotencyKey,
			"last_stripe_event_id":    record.LastStripeEventID,
			"last_error_code":         record.LastErrorCode,
			"last_error_message":      record.LastErrorMessage,
			"metadata_json":           record.MetadataJSON,
			"checkout_opened_at":      record.CheckoutOpenedAt,
			"session_expires_at":      record.SessionExpiresAt,
			"completed_at":            record.CompletedAt,
			"failed_at":               record.FailedAt,
			"expired_at":              record.ExpiredAt,
			"refunded_at":             record.RefundedAt,
			"updated_at":              record.UpdatedAt,
		})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *Repository) GetPaymentByID(ctx context.Context, id string) (*PaymentRecord, error) {
	var record PaymentRecord
	if err := r.db.WithContext(ctx).First(&record, "id = ?", strings.TrimSpace(id)).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &record, nil
}

func (r *Repository) GetPaymentByCheckoutSessionID(ctx context.Context, sessionID string) (*PaymentRecord, error) {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return nil, ErrNotFound
	}
	var record PaymentRecord
	if err := r.db.WithContext(ctx).First(&record, "checkout_session_id = ?", sessionID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &record, nil
}

func (r *Repository) GetPaymentByPaymentIntentID(ctx context.Context, paymentIntentID string) (*PaymentRecord, error) {
	paymentIntentID = strings.TrimSpace(paymentIntentID)
	if paymentIntentID == "" {
		return nil, ErrNotFound
	}
	var record PaymentRecord
	if err := r.db.WithContext(ctx).First(&record, "payment_intent_id = ?", paymentIntentID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &record, nil
}

func (r *Repository) GetPaymentByChargeID(ctx context.Context, chargeID string) (*PaymentRecord, error) {
	chargeID = strings.TrimSpace(chargeID)
	if chargeID == "" {
		return nil, ErrNotFound
	}
	var record PaymentRecord
	if err := r.db.WithContext(ctx).First(&record, "charge_id = ?", chargeID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &record, nil
}

func (r *Repository) ListPayments(ctx context.Context, input ListPaymentsInput) ([]PaymentRecord, int64, error) {
	limit := input.Limit
	if limit <= 0 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}
	offset := input.Offset
	if offset < 0 {
		offset = 0
	}

	query := r.db.WithContext(ctx).Model(&PaymentRecord{})
	if value := strings.TrimSpace(input.Purpose); value != "" {
		query = query.Where("purpose = ?", value)
	}
	if value := strings.TrimSpace(input.Status); value != "" {
		query = query.Where("status = ?", value)
	}

	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	var items []PaymentRecord
	if err := query.Order("created_at DESC").Limit(limit).Offset(offset).Find(&items).Error; err != nil {
		return nil, 0, err
	}
	return items, total, nil
}

func normalizePaymentRecord(record *PaymentRecord) {
	if record == nil {
		return
	}
	record.CheckoutSessionID = nullableString(stringValue(record.CheckoutSessionID))
	record.PaymentIntentID = nullableString(stringValue(record.PaymentIntentID))
	record.ChargeID = nullableString(stringValue(record.ChargeID))
}

func normalizePaymentEventRecord(record *PaymentEventRecord) {
	if record == nil {
		return
	}
	record.StripeEventID = nullableString(stringValue(record.StripeEventID))
}

func (r *Repository) CreateEvent(ctx context.Context, record *PaymentEventRecord) error {
	normalizePaymentEventRecord(record)
	return r.db.WithContext(ctx).Create(record).Error
}

func (r *Repository) GetEventByStripeEventID(ctx context.Context, stripeEventID string) (*PaymentEventRecord, error) {
	stripeEventID = strings.TrimSpace(stripeEventID)
	if stripeEventID == "" {
		return nil, ErrNotFound
	}
	var record PaymentEventRecord
	if err := r.db.WithContext(ctx).First(&record, "stripe_event_id = ?", stripeEventID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &record, nil
}

func (r *Repository) ListEventsByPaymentID(ctx context.Context, paymentID string, limit int) ([]PaymentEventRecord, error) {
	if limit <= 0 {
		limit = 50
	}
	if limit > 200 {
		limit = 200
	}
	var items []PaymentEventRecord
	if err := r.db.WithContext(ctx).
		Where("payment_id = ?", strings.TrimSpace(paymentID)).
		Order("received_at DESC").
		Limit(limit).
		Find(&items).Error; err != nil {
		return nil, err
	}
	return items, nil
}

func isUniqueConstraintError(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, gorm.ErrDuplicatedKey) {
		return true
	}
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "unique") || strings.Contains(message, "duplicate") || strings.Contains(message, "constraint failed")
}
