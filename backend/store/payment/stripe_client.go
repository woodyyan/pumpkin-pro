package payment

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"github.com/stripe/stripe-go/v83"
	"github.com/woodyyan/pumpkin-pro/backend/config"
)

type stripeGateway interface {
	CreateCheckoutSession(ctx context.Context, req stripeCheckoutSessionRequest) (*stripeCheckoutSessionResponse, error)
	ExpireCheckoutSession(ctx context.Context, sessionID string) (*stripeCheckoutSessionResponse, error)
	ParseWebhook(payload []byte, signatureHeader string) (*stripeWebhookEvent, error)
}

type stripeCheckoutSessionRequest struct {
	PaymentID          string
	AmountMinor        int64
	Currency           string
	PaymentMethodTypes []string
	SuccessURL         string
	CancelURL          string
	Title              string
	Description        string
	IdempotencyKey     string
	Metadata           map[string]string
	ExpiresAt          *time.Time
}

type stripeCheckoutSessionResponse struct {
	SessionID          string
	URL                string
	PaymentIntentID    string
	PaymentStatus      string
	Status             string
	PaymentMethodTypes []string
	ExpiresAt          *time.Time
	Livemode           bool
}

type stripeWebhookEvent struct {
	ID                     string
	Type                   string
	ObjectType             string
	PayloadJSON            string
	OccurredAt             *time.Time
	Livemode               bool
	PaymentID              string
	CheckoutSessionID      string
	PaymentIntentID        string
	ChargeID               string
	RefundID               string
	PaymentMethodType      string
	SessionStatus          string
	SessionPaymentStatus   string
	PaymentIntentStatus    string
	PaymentIntentErrorCode string
	PaymentIntentErrorMsg  string
	RefundStatus           string
}

type stripeClient struct {
	apiKey        string
	webhookSecret string
	client        *stripe.Client
}

func newStripeClient(cfg config.StripeConfig) *stripeClient {
	apiKey := strings.TrimSpace(cfg.SecretKey)
	if apiKey == "" {
		return nil
	}
	return &stripeClient{
		apiKey:        apiKey,
		webhookSecret: strings.TrimSpace(cfg.WebhookSecret),
		client:        stripe.NewClient(apiKey),
	}
}

func (c *stripeClient) CreateCheckoutSession(ctx context.Context, req stripeCheckoutSessionRequest) (*stripeCheckoutSessionResponse, error) {
	params := &stripe.CheckoutSessionCreateParams{
		Mode:               stripe.String(string(stripe.CheckoutSessionModePayment)),
		SuccessURL:         stripe.String(req.SuccessURL),
		CancelURL:          stripe.String(req.CancelURL),
		ClientReferenceID:  stripe.String(req.PaymentID),
		Currency:           stripe.String(req.Currency),
		PaymentMethodTypes: stripe.StringSlice(req.PaymentMethodTypes),
		LineItems: []*stripe.CheckoutSessionCreateLineItemParams{
			{
				Quantity: stripe.Int64(1),
				PriceData: &stripe.CheckoutSessionCreateLineItemPriceDataParams{
					Currency:   stripe.String(req.Currency),
					UnitAmount: stripe.Int64(req.AmountMinor),
					ProductData: &stripe.CheckoutSessionCreateLineItemPriceDataProductDataParams{
						Name:        stripe.String(req.Title),
						Description: stripe.String(req.Description),
						Metadata:    cloneStripeMetadata(req.Metadata),
					},
				},
			},
		},
		Metadata: cloneStripeMetadata(req.Metadata),
		PaymentIntentData: &stripe.CheckoutSessionCreatePaymentIntentDataParams{
			Description: stripe.String(req.Description),
			Metadata:    cloneStripeMetadata(req.Metadata),
		},
	}
	if req.ExpiresAt != nil {
		params.ExpiresAt = stripe.Int64(req.ExpiresAt.UTC().Unix())
	}
	params.SetIdempotencyKey(req.IdempotencyKey)

	session, err := c.client.V1CheckoutSessions.Create(ctx, params)
	if err != nil {
		return nil, err
	}
	return checkoutSessionToResponse(session), nil
}

func (c *stripeClient) ExpireCheckoutSession(ctx context.Context, sessionID string) (*stripeCheckoutSessionResponse, error) {
	session, err := c.client.V1CheckoutSessions.Expire(ctx, strings.TrimSpace(sessionID), nil)
	if err != nil {
		return nil, err
	}
	return checkoutSessionToResponse(session), nil
}

func (c *stripeClient) ParseWebhook(payload []byte, signatureHeader string) (*stripeWebhookEvent, error) {
	event, err := stripe.ConstructEvent(payload, signatureHeader, c.webhookSecret, stripe.WithIgnoreAPIVersionMismatch())
	if err != nil {
		return nil, err
	}
	result := &stripeWebhookEvent{
		ID:          strings.TrimSpace(event.ID),
		Type:        string(event.Type),
		PayloadJSON: string(payload),
		Livemode:    event.Livemode,
	}
	if event.Created > 0 {
		occurredAt := time.Unix(event.Created, 0).UTC()
		result.OccurredAt = &occurredAt
	}

	switch event.Type {
	case stripe.EventTypeCheckoutSessionCompleted, stripe.EventTypeCheckoutSessionExpired:
		var session stripe.CheckoutSession
		if err := json.Unmarshal(event.Data.Raw, &session); err != nil {
			return nil, err
		}
		result.ObjectType = "checkout_session"
		result.PaymentID = strings.TrimSpace(session.Metadata["payment_id"])
		result.CheckoutSessionID = strings.TrimSpace(session.ID)
		result.PaymentIntentID = strings.TrimSpace(extractPaymentIntentID(session.PaymentIntent))
		result.SessionStatus = strings.TrimSpace(string(session.Status))
		result.SessionPaymentStatus = strings.TrimSpace(string(session.PaymentStatus))
		result.PaymentMethodType = firstNonEmptyString(session.PaymentMethodTypes...)
	case stripe.EventTypePaymentIntentSucceeded, stripe.EventTypePaymentIntentPaymentFailed:
		var intent stripe.PaymentIntent
		if err := json.Unmarshal(event.Data.Raw, &intent); err != nil {
			return nil, err
		}
		result.ObjectType = "payment_intent"
		result.PaymentID = strings.TrimSpace(intent.Metadata["payment_id"])
		result.PaymentIntentID = strings.TrimSpace(intent.ID)
		result.ChargeID = strings.TrimSpace(extractChargeID(intent.LatestCharge))
		result.PaymentIntentStatus = strings.TrimSpace(string(intent.Status))
		result.PaymentMethodType = firstNonEmptyString(intent.PaymentMethodTypes...)
		if intent.LastPaymentError != nil {
			result.PaymentIntentErrorCode = strings.TrimSpace(string(intent.LastPaymentError.Code))
			result.PaymentIntentErrorMsg = strings.TrimSpace(intent.LastPaymentError.Msg)
		}
	case stripe.EventTypeChargeRefunded:
		var charge stripe.Charge
		if err := json.Unmarshal(event.Data.Raw, &charge); err != nil {
			return nil, err
		}
		result.ObjectType = "charge"
		result.ChargeID = strings.TrimSpace(charge.ID)
		result.PaymentID = strings.TrimSpace(charge.Metadata["payment_id"])
		if charge.PaymentMethodDetails != nil {
			result.PaymentMethodType = strings.TrimSpace(string(charge.PaymentMethodDetails.Type))
		}
	case stripe.EventTypeRefundUpdated:
		var refund stripe.Refund
		if err := json.Unmarshal(event.Data.Raw, &refund); err != nil {
			return nil, err
		}
		result.ObjectType = "refund"
		result.RefundID = strings.TrimSpace(refund.ID)
		result.ChargeID = strings.TrimSpace(extractChargeID(refund.Charge))
		result.PaymentID = strings.TrimSpace(refund.Metadata["payment_id"])
		result.RefundStatus = strings.TrimSpace(string(refund.Status))
	default:
		result.ObjectType = "event"
	}

	return result, nil
}

func checkoutSessionToResponse(session *stripe.CheckoutSession) *stripeCheckoutSessionResponse {
	if session == nil {
		return nil
	}
	return &stripeCheckoutSessionResponse{
		SessionID:          strings.TrimSpace(session.ID),
		URL:                strings.TrimSpace(session.URL),
		PaymentIntentID:    strings.TrimSpace(extractPaymentIntentID(session.PaymentIntent)),
		PaymentStatus:      strings.TrimSpace(string(session.PaymentStatus)),
		Status:             strings.TrimSpace(string(session.Status)),
		PaymentMethodTypes: cloneStringSlice(session.PaymentMethodTypes),
		ExpiresAt:          unixToUTCTime(session.ExpiresAt),
		Livemode:           session.Livemode,
	}
}

func extractPaymentIntentID(intent *stripe.PaymentIntent) string {
	if intent == nil {
		return ""
	}
	return strings.TrimSpace(intent.ID)
}

func extractChargeID(charge *stripe.Charge) string {
	if charge == nil {
		return ""
	}
	return strings.TrimSpace(charge.ID)
}

func cloneStripeMetadata(input map[string]string) map[string]string {
	if len(input) == 0 {
		return nil
	}
	output := make(map[string]string, len(input))
	for key, value := range input {
		output[key] = value
	}
	return output
}

func cloneStringSlice(input []string) []string {
	if len(input) == 0 {
		return nil
	}
	output := make([]string, len(input))
	copy(output, input)
	return output
}

func unixToUTCTime(value int64) *time.Time {
	if value <= 0 {
		return nil
	}
	t := time.Unix(value, 0).UTC()
	return &t
}

func firstNonEmptyString(values ...string) string {
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			return value
		}
	}
	return ""
}
