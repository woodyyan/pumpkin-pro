package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/woodyyan/pumpkin-pro/backend/store/admin"
	"github.com/woodyyan/pumpkin-pro/backend/store/payment"
	"github.com/woodyyan/pumpkin-pro/backend/tests/testutil"
)

type stubPaymentService struct {
	configView        *payment.PaymentConfigView
	createResult      *payment.CreateCheckoutSessionResult
	createErr         error
	listItems         []payment.PaymentRecord
	listTotal         int64
	listErr           error
	detail            *payment.PaymentDetail
	detailErr         error
	expireRecord      *payment.PaymentRecord
	expireErr         error
	webhookErr        error
	lastCreateAdminID string
	lastCreateInput   payment.AdminCreateCheckoutSessionInput
	lastListInput     payment.ListPaymentsInput
	lastDetailID      string
	lastExpireID      string
	webhookCalls      int
}

func (s *stubPaymentService) GetConfigView(ctx context.Context) *payment.PaymentConfigView {
	return s.configView
}

func (s *stubPaymentService) CreateAdminCheckoutSession(ctx context.Context, adminID string, input payment.AdminCreateCheckoutSessionInput) (*payment.CreateCheckoutSessionResult, error) {
	s.lastCreateAdminID = adminID
	s.lastCreateInput = input
	return s.createResult, s.createErr
}

func (s *stubPaymentService) ListPayments(ctx context.Context, input payment.ListPaymentsInput) ([]payment.PaymentRecord, int64, error) {
	s.lastListInput = input
	return s.listItems, s.listTotal, s.listErr
}

func (s *stubPaymentService) GetPaymentDetail(ctx context.Context, paymentID string) (*payment.PaymentDetail, error) {
	s.lastDetailID = paymentID
	return s.detail, s.detailErr
}

func (s *stubPaymentService) ExpireAdminPayment(ctx context.Context, paymentID string) (*payment.PaymentRecord, error) {
	s.lastExpireID = paymentID
	return s.expireRecord, s.expireErr
}

func (s *stubPaymentService) HandleWebhook(ctx context.Context, payload []byte, signatureHeader string) error {
	s.webhookCalls += 1
	return s.webhookErr
}

func TestHandleAdminPaymentCheckoutSessionsUsesCurrentAdmin(t *testing.T) {
	service := &stubPaymentService{
		createResult: &payment.CreateCheckoutSessionResult{PaymentID: "pay_1", Status: payment.StatusCheckoutOpen, CheckoutSessionID: "cs_1", CheckoutURL: "https://checkout.stripe.com/test"},
	}
	server := &appServer{paymentService: service}
	body := bytes.NewBufferString(`{"amount_minor":100,"currency":"cny","payment_method_types":["card"],"title":"Admin Test"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/admin/payments/checkout-sessions", body)
	req = req.WithContext(admin.WithCurrentAdmin(req.Context(), admin.CurrentAdmin{AdminID: "admin_123"}))
	req.Header.Set("Content-Type", "application/json")
	resp := httptest.NewRecorder()

	server.handleAdminPaymentCheckoutSessions(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.Code)
	}
	if service.lastCreateAdminID != "admin_123" {
		t.Fatalf("expected admin id propagated, got %s", service.lastCreateAdminID)
	}
	if service.lastCreateInput.AmountMinor != 100 {
		t.Fatalf("expected amount persisted in input, got %+v", service.lastCreateInput)
	}
}

func TestHandleAdminPaymentCheckoutSessionsSupportsExplicitLocalWalletSelection(t *testing.T) {
	service := &stubPaymentService{
		createResult: &payment.CreateCheckoutSessionResult{PaymentID: "pay_alipay", Status: payment.StatusCheckoutOpen, PaymentMethod: payment.PaymentMethodAlipay, CheckoutSessionID: "cs_alipay", CheckoutURL: "https://checkout.stripe.com/alipay"},
	}
	server := &appServer{paymentService: service}
	body := bytes.NewBufferString(`{"amount_minor":200,"currency":"cny","payment_method":"alipay","title":"Alipay Test"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/admin/payments/checkout-sessions", body)
	req = req.WithContext(admin.WithCurrentAdmin(req.Context(), admin.CurrentAdmin{AdminID: "admin_456"}))
	req.Header.Set("Content-Type", "application/json")
	resp := httptest.NewRecorder()

	server.handleAdminPaymentCheckoutSessions(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.Code)
	}
	if service.lastCreateInput.PaymentMethod != payment.PaymentMethodAlipay {
		t.Fatalf("expected explicit payment method forwarded, got %+v", service.lastCreateInput)
	}
}

func TestHandleAdminPaymentsListsRecords(t *testing.T) {
	service := &stubPaymentService{
		listItems: []payment.PaymentRecord{{ID: "pay_1", Status: payment.StatusSucceeded}},
		listTotal: 1,
	}
	server := &appServer{paymentService: service}
	req := httptest.NewRequest(http.MethodGet, "/api/admin/payments?purpose=admin_test&status=succeeded&limit=5&offset=2", nil)
	resp := httptest.NewRecorder()

	server.handleAdminPayments(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.Code)
	}
	if service.lastListInput.Purpose != payment.PurposeAdminTest || service.lastListInput.Status != payment.StatusSucceeded {
		t.Fatalf("unexpected list input: %+v", service.lastListInput)
	}
	var body map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body["total"].(float64) != 1 {
		t.Fatalf("unexpected total payload: %+v", body)
	}
}

func TestHandleAdminPaymentSubroutesDetail(t *testing.T) {
	service := &stubPaymentService{
		detail: &payment.PaymentDetail{Payment: &payment.PaymentRecord{ID: "pay_1", Status: payment.StatusCheckoutOpen}},
	}
	server := &appServer{paymentService: service}
	req := httptest.NewRequest(http.MethodGet, "/api/admin/payments/pay_1", nil)
	resp := httptest.NewRecorder()

	server.handleAdminPaymentSubroutes(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.Code)
	}
	if service.lastDetailID != "pay_1" {
		t.Fatalf("expected detail lookup on pay_1, got %s", service.lastDetailID)
	}
}

func TestHandleAdminPaymentSubroutesExpireConflict(t *testing.T) {
	service := &stubPaymentService{expireErr: payment.ErrConflict}
	server := &appServer{paymentService: service}
	req := httptest.NewRequest(http.MethodPost, "/api/admin/payments/pay_2/expire", nil)
	resp := httptest.NewRecorder()

	server.handleAdminPaymentSubroutes(resp, req)

	if resp.Code != http.StatusConflict {
		t.Fatalf("expected 409, got %d", resp.Code)
	}
}

func TestHandleAdminPaymentCheckoutSessionsStripsWrappedInvalidInputPrefix(t *testing.T) {
	service := &stubPaymentService{createErr: fmt.Errorf("%w: 支付金额必须大于 0", payment.ErrInvalidInput)}
	server := &appServer{paymentService: service}
	body := bytes.NewBufferString(`{"amount_minor":0}`)
	req := httptest.NewRequest(http.MethodPost, "/api/admin/payments/checkout-sessions", body)
	req = req.WithContext(admin.WithCurrentAdmin(req.Context(), admin.CurrentAdmin{AdminID: "admin_123"}))
	req.Header.Set("Content-Type", "application/json")
	resp := httptest.NewRecorder()

	server.handleAdminPaymentCheckoutSessions(resp, req)

	if resp.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", resp.Code)
	}
	var payload map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if payload["detail"] != "支付金额必须大于 0" {
		t.Fatalf("unexpected detail payload: %+v", payload)
	}
}

func TestHandleStripeWebhookReturnsBadRequestOnError(t *testing.T) {
	service := &stubPaymentService{webhookErr: errors.New("invalid signature")}
	server := &appServer{paymentService: service}
	req := httptest.NewRequest(http.MethodPost, "/api/stripe/webhook", bytes.NewBufferString(`{}`))
	resp := httptest.NewRecorder()

	server.handleStripeWebhook(resp, req)

	if resp.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", resp.Code)
	}
	if service.webhookCalls != 1 {
		t.Fatalf("expected webhook handler invoked once, got %d", service.webhookCalls)
	}
}

func TestWithSuperAdminAuthInjectsCurrentAdminIntoContext(t *testing.T) {
	db := testutil.InMemoryDB(t)
	if err := admin.NewMigrator().AutoMigrate(db); err != nil {
		t.Fatalf("auto migrate admin models: %v", err)
	}
	svc := admin.NewService(admin.NewRepository(db), admin.ServiceConfig{JWTSecret: "test-secret", AccessTTL: time.Hour})
	if err := svc.SeedAdmin(context.Background(), "admin@example.com", "password123"); err != nil {
		t.Fatalf("seed admin: %v", err)
	}
	login, err := svc.Login(context.Background(), admin.AdminLoginInput{Email: "admin@example.com", Password: "password123"})
	if err != nil {
		t.Fatalf("login: %v", err)
	}
	server := &appServer{adminService: svc}
	var seenAdmin admin.CurrentAdmin
	handler := server.withSuperAdminAuth(func(w http.ResponseWriter, r *http.Request) {
		value, ok := admin.CurrentAdminFromContext(r.Context())
		if !ok {
			t.Fatalf("expected current admin in context")
		}
		seenAdmin = value
		writeJSON(w, http.StatusOK, map[string]any{"ok": true})
	})
	req := httptest.NewRequest(http.MethodGet, "/api/admin/payments/config", nil)
	req.AddCookie(&http.Cookie{Name: adminSessionCookieName, Value: login.AccessToken})
	resp := httptest.NewRecorder()

	handler(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.Code)
	}
	if seenAdmin.AdminID == "" || seenAdmin.Email != "admin@example.com" {
		t.Fatalf("unexpected current admin: %+v", seenAdmin)
	}
}
