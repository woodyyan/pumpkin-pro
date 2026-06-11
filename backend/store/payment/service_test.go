package payment

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/woodyyan/pumpkin-pro/backend/config"
	"github.com/woodyyan/pumpkin-pro/backend/tests/testutil"
)

type stubStripeGateway struct {
	createFn func(ctx context.Context, req stripeCheckoutSessionRequest) (*stripeCheckoutSessionResponse, error)
	expireFn func(ctx context.Context, sessionID string) (*stripeCheckoutSessionResponse, error)
	parseFn  func(payload []byte, signatureHeader string) (*stripeWebhookEvent, error)
}

func (s *stubStripeGateway) CreateCheckoutSession(ctx context.Context, req stripeCheckoutSessionRequest) (*stripeCheckoutSessionResponse, error) {
	return s.createFn(ctx, req)
}

func (s *stubStripeGateway) ExpireCheckoutSession(ctx context.Context, sessionID string) (*stripeCheckoutSessionResponse, error) {
	return s.expireFn(ctx, sessionID)
}

func (s *stubStripeGateway) ParseWebhook(payload []byte, signatureHeader string) (*stripeWebhookEvent, error) {
	return s.parseFn(payload, signatureHeader)
}

func newTestService(t *testing.T) (*Service, *Repository) {
	t.Helper()
	db := testutil.InMemoryDB(t)
	testutil.AutoMigrateModels(t, db, &PaymentRecord{}, &PaymentEventRecord{})
	repo := NewRepository(db)
	svc := NewService(repo, ServiceConfig{
		AppPublicBaseURL: "http://localhost:3000",
		Stripe: config.StripeConfig{
			Mode:                  "test",
			SecretKey:             "sk_test_demo",
			WebhookSecret:         "whsec_demo",
			DefaultCurrency:       "cny",
			AllowedPaymentMethods: []string{"card", "alipay", "wechat_pay"},
			SuccessPath:           "/admin?tab=payments&checkout=success",
			CancelPath:            "/admin?tab=payments&checkout=cancel",
			CheckoutExpireMinutes: 60,
		},
	})
	return svc, repo
}

func TestCreateAdminCheckoutSessionPersistsPayment(t *testing.T) {
	svc, repo := newTestService(t)
	svc.SetGateway(&stubStripeGateway{
		createFn: func(ctx context.Context, req stripeCheckoutSessionRequest) (*stripeCheckoutSessionResponse, error) {
			if req.Currency != "cny" {
				t.Fatalf("expected cny, got %s", req.Currency)
			}
			if len(req.PaymentMethodTypes) != 1 || req.PaymentMethodTypes[0] != "card" {
				t.Fatalf("unexpected payment methods: %#v", req.PaymentMethodTypes)
			}
			return &stripeCheckoutSessionResponse{
				SessionID:          "cs_test_123",
				URL:                "https://checkout.stripe.com/c/pay/test",
				PaymentIntentID:    "pi_test_123",
				PaymentMethodTypes: []string{"card"},
				ExpiresAt:          ptrTime(time.Now().Add(30 * time.Minute).UTC()),
			}, nil
		},
	})

	result, err := svc.CreateAdminCheckoutSession(context.Background(), "admin_1", AdminCreateCheckoutSessionInput{
		AmountMinor:        1999,
		Currency:           "cny",
		PaymentMethodTypes: []string{"card"},
		Title:              "Admin Test Payment",
	})
	if err != nil {
		t.Fatalf("CreateAdminCheckoutSession() error = %v", err)
	}
	if result.PaymentID == "" || result.CheckoutSessionID != "cs_test_123" {
		t.Fatalf("unexpected result: %+v", result)
	}
	record, err := repo.GetPaymentByID(context.Background(), result.PaymentID)
	if err != nil {
		t.Fatalf("GetPaymentByID() error = %v", err)
	}
	if record.Status != StatusCheckoutOpen {
		t.Fatalf("expected status %s, got %s", StatusCheckoutOpen, record.Status)
	}
	if stringValue(record.PaymentIntentID) != "pi_test_123" {
		t.Fatalf("expected payment intent id persisted, got %s", stringValue(record.PaymentIntentID))
	}
	events, err := repo.ListEventsByPaymentID(context.Background(), result.PaymentID, 10)
	if err != nil {
		t.Fatalf("ListEventsByPaymentID() error = %v", err)
	}
	if len(events) != 1 || events[0].EventType != "checkout.session.created" {
		t.Fatalf("unexpected events: %+v", events)
	}
}

func TestCreateAdminCheckoutSessionSupportsAlipayAdminTesting(t *testing.T) {
	svc, _ := newTestService(t)
	svc.SetGateway(&stubStripeGateway{
		createFn: func(ctx context.Context, req stripeCheckoutSessionRequest) (*stripeCheckoutSessionResponse, error) {
			if req.PaymentMethodCode != PaymentMethodAlipay {
				t.Fatalf("expected alipay payment method code, got %s", req.PaymentMethodCode)
			}
			if len(req.PaymentMethodTypes) != 1 || req.PaymentMethodTypes[0] != PaymentMethodAlipay {
				t.Fatalf("unexpected payment methods: %#v", req.PaymentMethodTypes)
			}
			if req.WeChatPayClient != "" {
				t.Fatalf("expected no wechat pay client for alipay, got %s", req.WeChatPayClient)
			}
			return &stripeCheckoutSessionResponse{
				SessionID:          "cs_test_alipay",
				URL:                "https://checkout.stripe.com/c/pay/alipay",
				PaymentMethodTypes: []string{PaymentMethodAlipay},
			}, nil
		},
	})

	result, err := svc.CreateAdminCheckoutSession(context.Background(), "admin_alipay", AdminCreateCheckoutSessionInput{
		AmountMinor:   2000,
		Currency:      "cny",
		PaymentMethod: PaymentMethodAlipay,
		Title:         "Alipay Admin Test",
	})
	if err != nil {
		t.Fatalf("CreateAdminCheckoutSession() error = %v", err)
	}
	if result.PaymentMethod != PaymentMethodAlipay {
		t.Fatalf("expected alipay result, got %+v", result)
	}
	if result.AllowedPaymentNote == "" {
		t.Fatalf("expected testing note returned for alipay")
	}
}

func TestCreateAdminCheckoutSessionBuildsWeChatPayCheckoutOptions(t *testing.T) {
	svc, _ := newTestService(t)
	svc.SetGateway(&stubStripeGateway{
		createFn: func(ctx context.Context, req stripeCheckoutSessionRequest) (*stripeCheckoutSessionResponse, error) {
			if req.PaymentMethodCode != PaymentMethodWeChatPay {
				t.Fatalf("expected wechat_pay code, got %s", req.PaymentMethodCode)
			}
			if len(req.PaymentMethodTypes) != 1 || req.PaymentMethodTypes[0] != PaymentMethodWeChatPay {
				t.Fatalf("unexpected payment methods: %#v", req.PaymentMethodTypes)
			}
			if req.WeChatPayClient != "web" {
				t.Fatalf("expected wechat_pay client web, got %s", req.WeChatPayClient)
			}
			return &stripeCheckoutSessionResponse{
				SessionID:          "cs_test_wechat",
				URL:                "https://checkout.stripe.com/c/pay/wechat",
				PaymentMethodTypes: []string{PaymentMethodWeChatPay},
			}, nil
		},
	})

	result, err := svc.CreateAdminCheckoutSession(context.Background(), "admin_wechat", AdminCreateCheckoutSessionInput{
		AmountMinor:   888,
		Currency:      "cny",
		PaymentMethod: PaymentMethodWeChatPay,
		Title:         "WeChat Pay Admin Test",
	})
	if err != nil {
		t.Fatalf("CreateAdminCheckoutSession() error = %v", err)
	}
	if result.PaymentMethod != PaymentMethodWeChatPay {
		t.Fatalf("expected wechat_pay result, got %+v", result)
	}
}

func TestCreateAdminCheckoutSessionRejectsDisabledOrUnsupportedLocalWalletRequests(t *testing.T) {
	svc, _ := newTestService(t)
	svc.cfg.Stripe.AllowedPaymentMethods = []string{PaymentMethodCard}

	_, err := svc.CreateAdminCheckoutSession(context.Background(), "admin_card_only", AdminCreateCheckoutSessionInput{
		AmountMinor:   100,
		PaymentMethod: PaymentMethodAlipay,
	})
	if !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("expected invalid input for disabled alipay, got %v", err)
	}

	svc.cfg.Stripe.AllowedPaymentMethods = []string{PaymentMethodCard, PaymentMethodWeChatPay}
	_, err = svc.CreateAdminCheckoutSession(context.Background(), "admin_wechat", AdminCreateCheckoutSessionInput{
		AmountMinor:   100,
		PaymentMethod: PaymentMethodWeChatPay,
		Currency:      "usd",
	})
	if !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("expected invalid input for unsupported wechat currency, got %v", err)
	}
}

func TestHandleWebhookUpdatesSucceededPayment(t *testing.T) {
	svc, repo := newTestService(t)
	now := time.Now().UTC()
	record := &PaymentRecord{
		ID:                   "pay_test_1",
		Provider:             ProviderStripe,
		Purpose:              PurposeAdminTest,
		ScenarioType:         ScenarioHostedCheckout,
		Mode:                 "test",
		Status:               StatusCheckoutOpen,
		TriggerAdminID:       "admin_1",
		Title:                "Admin Test",
		AmountMinor:          1000,
		Currency:             "cny",
		PaymentMethodRequest: "card",
		CheckoutSessionID:    nullableString("cs_test_123"),
		PaymentIntentID:      nullableString("pi_test_123"),
		IdempotencyKey:       "idem_1",
		CreatedAt:            now,
		UpdatedAt:            now,
	}
	if err := repo.CreatePayment(context.Background(), record); err != nil {
		t.Fatalf("CreatePayment() error = %v", err)
	}
	svc.SetGateway(&stubStripeGateway{
		parseFn: func(payload []byte, signatureHeader string) (*stripeWebhookEvent, error) {
			return &stripeWebhookEvent{
				ID:                  "evt_1",
				Type:                "payment_intent.succeeded",
				ObjectType:          "payment_intent",
				PayloadJSON:         string(payload),
				Livemode:            false,
				PaymentID:           "pay_test_1",
				PaymentIntentID:     "pi_test_123",
				ChargeID:            "ch_test_123",
				PaymentIntentStatus: "succeeded",
				OccurredAt:          ptrTime(now.Add(1 * time.Minute)),
				PaymentMethodType:   "card",
			}, nil
		},
	})

	if err := svc.HandleWebhook(context.Background(), []byte(`{"id":"evt_1"}`), "signature"); err != nil {
		t.Fatalf("HandleWebhook() error = %v", err)
	}
	updated, err := repo.GetPaymentByID(context.Background(), "pay_test_1")
	if err != nil {
		t.Fatalf("GetPaymentByID() error = %v", err)
	}
	if updated.Status != StatusSucceeded {
		t.Fatalf("expected status succeeded, got %s", updated.Status)
	}
	if stringValue(updated.ChargeID) != "ch_test_123" {
		t.Fatalf("expected charge id updated, got %s", stringValue(updated.ChargeID))
	}
	events, err := repo.ListEventsByPaymentID(context.Background(), "pay_test_1", 10)
	if err != nil {
		t.Fatalf("ListEventsByPaymentID() error = %v", err)
	}
	if len(events) != 1 || events[0].StripeEventID != "evt_1" || !events[0].Processed {
		t.Fatalf("unexpected webhook events: %+v", events)
	}
}

func TestHandleWebhookIsIdempotent(t *testing.T) {
	svc, repo := newTestService(t)
	now := time.Now().UTC()
	record := &PaymentRecord{
		ID:                   "pay_test_2",
		Provider:             ProviderStripe,
		Purpose:              PurposeAdminTest,
		ScenarioType:         ScenarioHostedCheckout,
		Mode:                 "test",
		Status:               StatusCheckoutOpen,
		TriggerAdminID:       "admin_1",
		Title:                "Admin Test",
		AmountMinor:          1000,
		Currency:             "cny",
		PaymentMethodRequest: "card",
		CheckoutSessionID:    nullableString("cs_test_456"),
		PaymentIntentID:      nullableString("pi_test_456"),
		IdempotencyKey:       "idem_2",
		CreatedAt:            now,
		UpdatedAt:            now,
	}
	if err := repo.CreatePayment(context.Background(), record); err != nil {
		t.Fatalf("CreatePayment() error = %v", err)
	}
	event := &stripeWebhookEvent{
		ID:                  "evt_repeat",
		Type:                "payment_intent.succeeded",
		ObjectType:          "payment_intent",
		PayloadJSON:         `{}`,
		Livemode:            false,
		PaymentID:           "pay_test_2",
		PaymentIntentID:     "pi_test_456",
		PaymentIntentStatus: "succeeded",
		OccurredAt:          ptrTime(now.Add(2 * time.Minute)),
		PaymentMethodType:   "card",
	}
	svc.SetGateway(&stubStripeGateway{parseFn: func(payload []byte, signatureHeader string) (*stripeWebhookEvent, error) {
		return event, nil
	}})
	if err := svc.HandleWebhook(context.Background(), []byte(`{"id":"evt_repeat"}`), "sig"); err != nil {
		t.Fatalf("first HandleWebhook() error = %v", err)
	}
	if err := svc.HandleWebhook(context.Background(), []byte(`{"id":"evt_repeat"}`), "sig"); err != nil {
		t.Fatalf("second HandleWebhook() error = %v", err)
	}
	events, err := repo.ListEventsByPaymentID(context.Background(), "pay_test_2", 10)
	if err != nil {
		t.Fatalf("ListEventsByPaymentID() error = %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("expected one persisted event, got %d", len(events))
	}
}

func TestGetConfigViewUsesNormalizedDefaults(t *testing.T) {
	svc, _ := newTestService(t)
	view := svc.GetConfigView(context.Background())
	if !view.Ready {
		t.Fatalf("expected ready view, got %+v", view)
	}
	if view.DefaultCurrency != "cny" {
		t.Fatalf("expected cny default currency, got %s", view.DefaultCurrency)
	}
	if len(view.AllowedPaymentMethods) != 3 {
		t.Fatalf("unexpected allowed methods: %#v", view.AllowedPaymentMethods)
	}
	if len(view.AdminTestPaymentMethods) != 3 {
		t.Fatalf("expected three admin test methods, got %#v", view.AdminTestPaymentMethods)
	}
	if view.AdminTestPaymentMethods[1].Code != PaymentMethodAlipay || !view.AdminTestPaymentMethods[1].Enabled {
		t.Fatalf("unexpected alipay config: %#v", view.AdminTestPaymentMethods[1])
	}
	if view.AdminTestPaymentMethods[2].Code != PaymentMethodWeChatPay || view.AdminTestPaymentMethods[2].CheckoutFlow != "qr_code" {
		t.Fatalf("unexpected wechat pay config: %#v", view.AdminTestPaymentMethods[2])
	}
}

func TestCreateAdminCheckoutSessionStoresMetadataJSON(t *testing.T) {
	svc, repo := newTestService(t)
	svc.SetGateway(&stubStripeGateway{createFn: func(ctx context.Context, req stripeCheckoutSessionRequest) (*stripeCheckoutSessionResponse, error) {
		return &stripeCheckoutSessionResponse{SessionID: "cs_test_meta", URL: "https://checkout.stripe.com/meta", PaymentMethodTypes: []string{"card"}}, nil
	}})
	result, err := svc.CreateAdminCheckoutSession(context.Background(), "admin_meta", AdminCreateCheckoutSessionInput{AmountMinor: 500})
	if err != nil {
		t.Fatalf("CreateAdminCheckoutSession() error = %v", err)
	}
	record, err := repo.GetPaymentByID(context.Background(), result.PaymentID)
	if err != nil {
		t.Fatalf("GetPaymentByID() error = %v", err)
	}
	var payload map[string]string
	if err := json.Unmarshal([]byte(record.MetadataJSON), &payload); err != nil {
		t.Fatalf("metadata json should be valid: %v", err)
	}
	if payload["payment_id"] != result.PaymentID || payload["trigger_admin_id"] != "admin_meta" {
		t.Fatalf("unexpected metadata payload: %#v", payload)
	}
}

func TestCreateAdminCheckoutSessionAllowsRepeatedFailuresWithoutUniqueExternalIDConflicts(t *testing.T) {
	svc, repo := newTestService(t)
	expectedErr := errors.New("stripe create failed")
	svc.SetGateway(&stubStripeGateway{createFn: func(ctx context.Context, req stripeCheckoutSessionRequest) (*stripeCheckoutSessionResponse, error) {
		return nil, expectedErr
	}})

	for i := 0; i < 2; i++ {
		_, err := svc.CreateAdminCheckoutSession(context.Background(), "admin_retry", AdminCreateCheckoutSessionInput{AmountMinor: 1200})
		if !errors.Is(err, expectedErr) {
			t.Fatalf("attempt %d: expected stripe error, got %v", i+1, err)
		}
	}

	items, total, err := repo.ListPayments(context.Background(), ListPaymentsInput{Purpose: PurposeAdminTest, Limit: 10})
	if err != nil {
		t.Fatalf("ListPayments() error = %v", err)
	}
	if total != 2 || len(items) != 2 {
		t.Fatalf("expected two failed payments persisted, total=%d len=%d", total, len(items))
	}
	for _, item := range items {
		if item.Status != StatusFailed {
			t.Fatalf("expected failed status, got %s", item.Status)
		}
		if item.CheckoutSessionID != nil || item.PaymentIntentID != nil || item.ChargeID != nil {
			t.Fatalf("expected nullable external ids to stay nil after create failure, got checkout=%v intent=%v charge=%v", item.CheckoutSessionID, item.PaymentIntentID, item.ChargeID)
		}
	}
}

func TestRepositoryNormalizesBlankExternalIDsToNil(t *testing.T) {
	_, repo := newTestService(t)
	now := time.Now().UTC()
	blankSession := "   "
	blankIntent := ""
	blankCharge := "\t"
	record := &PaymentRecord{
		ID:                   "pay_blank_ids",
		Provider:             ProviderStripe,
		Purpose:              PurposeAdminTest,
		ScenarioType:         ScenarioHostedCheckout,
		Mode:                 "test",
		Status:               StatusFailed,
		TriggerAdminID:       "admin_1",
		Title:                "Blank IDs",
		AmountMinor:          100,
		Currency:             "cny",
		PaymentMethodRequest: "card",
		CheckoutSessionID:    &blankSession,
		PaymentIntentID:      &blankIntent,
		ChargeID:             &blankCharge,
		IdempotencyKey:       "idem_blank_ids",
		CreatedAt:            now,
		UpdatedAt:            now,
	}
	if err := repo.CreatePayment(context.Background(), record); err != nil {
		t.Fatalf("CreatePayment() error = %v", err)
	}
	stored, err := repo.GetPaymentByID(context.Background(), record.ID)
	if err != nil {
		t.Fatalf("GetPaymentByID() error = %v", err)
	}
	if stored.CheckoutSessionID != nil || stored.PaymentIntentID != nil || stored.ChargeID != nil {
		t.Fatalf("expected blank external ids normalized to nil, got checkout=%v intent=%v charge=%v", stored.CheckoutSessionID, stored.PaymentIntentID, stored.ChargeID)
	}
}

func ptrTime(value time.Time) *time.Time {
	return &value
}
