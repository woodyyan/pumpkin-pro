package payment

import (
	"strings"
	"time"
)

const (
	ProviderStripe = "stripe"

	PurposeAdminTest = "admin_test"

	ScenarioHostedCheckout = "hosted_checkout"

	PaymentMethodCard      = "card"
	PaymentMethodAlipay    = "alipay"
	PaymentMethodWeChatPay = "wechat_pay"

	StatusInitiated         = "initiated"
	StatusCheckoutOpen      = "checkout_open"
	StatusProcessing        = "processing"
	StatusSucceeded         = "succeeded"
	StatusFailed            = "failed"
	StatusExpired           = "expired"
	StatusCanceled          = "canceled"
	StatusRefunded          = "refunded"
	StatusPartiallyRefunded = "partially_refunded"

	EventSourceWebhook = "webhook"
	EventSourceAdmin   = "admin_api"
	EventSourceSystem  = "system"
)

var terminalStatuses = map[string]bool{
	StatusSucceeded:         true,
	StatusFailed:            true,
	StatusExpired:           true,
	StatusCanceled:          true,
	StatusRefunded:          true,
	StatusPartiallyRefunded: true,
}

type PaymentRecord struct {
	ID                    string     `gorm:"primaryKey;size:36" json:"id"`
	Provider              string     `gorm:"size:20;not null;default:'stripe';index" json:"provider"`
	Purpose               string     `gorm:"size:32;not null;default:'admin_test';index" json:"purpose"`
	ScenarioType          string     `gorm:"size:32;not null;default:'hosted_checkout'" json:"scenario_type"`
	Mode                  string     `gorm:"size:16;not null;default:'test';index" json:"mode"`
	Status                string     `gorm:"size:24;not null;default:'initiated';index" json:"status"`
	TriggerAdminID        string     `gorm:"size:36;index" json:"trigger_admin_id,omitempty"`
	UserID                string     `gorm:"size:36;index" json:"user_id,omitempty"`
	Title                 string     `gorm:"size:255;not null;default:''" json:"title"`
	AmountMinor           int64      `gorm:"not null;default:0" json:"amount_minor"`
	Currency              string     `gorm:"size:12;not null;default:'cny'" json:"currency"`
	PaymentMethodRequest  string     `gorm:"size:255;not null;default:''" json:"payment_method_request"`
	PaymentMethodSelected string     `gorm:"size:64;not null;default:''" json:"payment_method_selected"`
	CheckoutSessionID     *string    `gorm:"size:128;uniqueIndex" json:"checkout_session_id,omitempty"`
	CheckoutURL           string     `gorm:"type:text;not null;default:''" json:"checkout_url,omitempty"`
	PaymentIntentID       *string    `gorm:"size:128;uniqueIndex" json:"payment_intent_id,omitempty"`
	ChargeID              *string    `gorm:"size:128;uniqueIndex" json:"charge_id,omitempty"`
	RefundID              string     `gorm:"size:128" json:"refund_id,omitempty"`
	CustomerID            string     `gorm:"size:128" json:"customer_id,omitempty"`
	SuccessURL            string     `gorm:"type:text;not null;default:''" json:"success_url"`
	CancelURL             string     `gorm:"type:text;not null;default:''" json:"cancel_url"`
	IdempotencyKey        string     `gorm:"size:128;not null;default:'';uniqueIndex" json:"idempotency_key"`
	LastStripeEventID     string     `gorm:"size:128;index" json:"last_stripe_event_id,omitempty"`
	LastErrorCode         string     `gorm:"size:128;not null;default:''" json:"last_error_code,omitempty"`
	LastErrorMessage      string     `gorm:"type:text;not null;default:''" json:"last_error_message,omitempty"`
	MetadataJSON          string     `gorm:"type:text;not null;default:''" json:"metadata_json,omitempty"`
	CheckoutOpenedAt      *time.Time `json:"checkout_opened_at,omitempty"`
	SessionExpiresAt      *time.Time `json:"session_expires_at,omitempty"`
	CompletedAt           *time.Time `json:"completed_at,omitempty"`
	FailedAt              *time.Time `json:"failed_at,omitempty"`
	ExpiredAt             *time.Time `json:"expired_at,omitempty"`
	RefundedAt            *time.Time `json:"refunded_at,omitempty"`
	CreatedAt             time.Time  `gorm:"not null;index" json:"created_at"`
	UpdatedAt             time.Time  `gorm:"not null" json:"updated_at"`
}

func (PaymentRecord) TableName() string {
	return "payments"
}

type PaymentEventRecord struct {
	ID                string     `gorm:"primaryKey;size:36" json:"id"`
	PaymentID         string     `gorm:"size:36;index" json:"payment_id,omitempty"`
	Provider          string     `gorm:"size:20;not null;default:'stripe';index" json:"provider"`
	Source            string     `gorm:"size:24;not null;default:'webhook';index" json:"source"`
	StripeEventID     *string    `gorm:"size:128;uniqueIndex" json:"stripe_event_id,omitempty"`
	EventType         string     `gorm:"size:80;not null;default:'';index" json:"event_type"`
	ObjectType        string     `gorm:"size:48;not null;default:''" json:"object_type"`
	ObjectID          string     `gorm:"size:128;not null;default:'';index" json:"object_id"`
	StatusBefore      string     `gorm:"size:24;not null;default:''" json:"status_before,omitempty"`
	StatusAfter       string     `gorm:"size:24;not null;default:''" json:"status_after,omitempty"`
	SignatureVerified bool       `gorm:"not null;default:false" json:"signature_verified"`
	Processed         bool       `gorm:"not null;default:false;index" json:"processed"`
	ErrorMessage      string     `gorm:"type:text;not null;default:''" json:"error_message,omitempty"`
	PayloadJSON       string     `gorm:"type:text;not null;default:''" json:"payload_json,omitempty"`
	OccurredAt        *time.Time `json:"occurred_at,omitempty"`
	ReceivedAt        time.Time  `gorm:"not null;index" json:"received_at"`
	CreatedAt         time.Time  `gorm:"not null" json:"created_at"`
}

func (PaymentEventRecord) TableName() string {
	return "payment_events"
}

type AdminTestPaymentMethodView struct {
	Code                string   `json:"code"`
	Label               string   `json:"label"`
	Enabled             bool     `json:"enabled"`
	SupportedCurrencies []string `json:"supported_currencies,omitempty"`
	RecommendedCurrency string   `json:"recommended_currency,omitempty"`
	CheckoutFlow        string   `json:"checkout_flow,omitempty"`
	Description         string   `json:"description,omitempty"`
	TestingNote         string   `json:"testing_note,omitempty"`
}

type PaymentConfigView struct {
	Provider                string                       `json:"provider"`
	Mode                    string                       `json:"mode"`
	Ready                   bool                         `json:"ready"`
	SecretKeyConfigured     bool                         `json:"secret_key_configured"`
	WebhookSecretConfigured bool                         `json:"webhook_secret_configured"`
	DefaultCurrency         string                       `json:"default_currency"`
	AllowedPaymentMethods   []string                     `json:"allowed_payment_methods"`
	AdminTestPaymentMethods []AdminTestPaymentMethodView `json:"admin_test_payment_methods,omitempty"`
	SupportedScenarios      []string                     `json:"supported_scenarios"`
}

type AdminCreateCheckoutSessionInput struct {
	Purpose            string   `json:"purpose"`
	AmountMinor        int64    `json:"amount_minor"`
	Currency           string   `json:"currency"`
	PaymentMethod      string   `json:"payment_method"`
	PaymentMethodTypes []string `json:"payment_method_types"`
	Title              string   `json:"title"`
	Description        string   `json:"description"`
}

type CreateCheckoutSessionResult struct {
	PaymentID          string  `json:"payment_id"`
	Status             string  `json:"status"`
	PaymentMethod      string  `json:"payment_method,omitempty"`
	CheckoutSessionID  string  `json:"checkout_session_id"`
	CheckoutURL        string  `json:"checkout_url"`
	PaymentIntentID    string  `json:"payment_intent_id,omitempty"`
	SessionExpiresAt   *string `json:"expires_at,omitempty"`
	AllowedPaymentNote string  `json:"allowed_payment_note,omitempty"`
}

type PaymentDetail struct {
	Payment *PaymentRecord       `json:"payment"`
	Events  []PaymentEventRecord `json:"events"`
}

type ListPaymentsInput struct {
	Purpose string
	Status  string
	Limit   int
	Offset  int
}

func IsTerminalStatus(status string) bool {
	return terminalStatuses[status]
}

func nullableString(value string) *string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return nil
	}
	return &trimmed
}

func stringValue(value *string) string {
	if value == nil {
		return ""
	}
	return strings.TrimSpace(*value)
}
