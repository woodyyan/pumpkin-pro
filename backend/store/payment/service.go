package payment

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/stripe/stripe-go/v83"
	"github.com/woodyyan/pumpkin-pro/backend/config"
	"gorm.io/gorm"
)

type ServiceConfig struct {
	AppPublicBaseURL string
	Stripe           config.StripeConfig
}

type Service struct {
	repo    *Repository
	cfg     ServiceConfig
	gateway stripeGateway
}

func NewService(repo *Repository, cfg ServiceConfig) *Service {
	service := &Service{repo: repo, cfg: cfg}
	service.gateway = newStripeClient(cfg.Stripe)
	return service
}

func (s *Service) SetGateway(gateway stripeGateway) {
	s.gateway = gateway
}

func (s *Service) GetConfigView(_ context.Context) *PaymentConfigView {
	allowedMethods := normalizeAllowedPaymentMethods(s.cfg.Stripe.AllowedPaymentMethods)
	if len(allowedMethods) == 0 {
		allowedMethods = []string{PaymentMethodCard}
	}
	secretConfigured := strings.TrimSpace(s.cfg.Stripe.SecretKey) != ""
	webhookConfigured := strings.TrimSpace(s.cfg.Stripe.WebhookSecret) != ""
	return &PaymentConfigView{
		Provider:                ProviderStripe,
		Mode:                    normalizeStripeMode(s.cfg.Stripe.Mode),
		Ready:                   secretConfigured && webhookConfigured,
		SecretKeyConfigured:     secretConfigured,
		WebhookSecretConfigured: webhookConfigured,
		DefaultCurrency:         normalizeCurrency(s.cfg.Stripe.DefaultCurrency),
		AllowedPaymentMethods:   allowedMethods,
		AdminTestPaymentMethods: buildAdminTestPaymentMethodViews(allowedMethods),
		SupportedScenarios:      []string{ScenarioHostedCheckout},
	}
}

func (s *Service) CreateAdminCheckoutSession(ctx context.Context, adminID string, input AdminCreateCheckoutSessionInput) (*CreateCheckoutSessionResult, error) {
	if s.gateway == nil || strings.TrimSpace(s.cfg.Stripe.SecretKey) == "" {
		return nil, ErrStripeDisabled
	}
	if strings.TrimSpace(s.cfg.Stripe.WebhookSecret) == "" {
		return nil, ErrStripeWebhookMisconfigured
	}
	if normalizeStripeMode(s.cfg.Stripe.Mode) != "test" {
		return nil, ErrUnsupportedMode
	}
	adminID = strings.TrimSpace(adminID)
	if adminID == "" {
		return nil, fmt.Errorf("%w: missing admin id", ErrInvalidInput)
	}
	amountMinor := input.AmountMinor
	if amountMinor <= 0 {
		return nil, fmt.Errorf("%w: 支付金额必须大于 0", ErrInvalidInput)
	}
	purpose := strings.TrimSpace(input.Purpose)
	if purpose == "" {
		purpose = PurposeAdminTest
	}
	if purpose != PurposeAdminTest {
		return nil, fmt.Errorf("%w: 一期仅支持 admin_test", ErrInvalidInput)
	}
	paymentMethod, err := resolveAdminPaymentMethod(input, s.cfg.Stripe.AllowedPaymentMethods)
	if err != nil {
		return nil, err
	}
	currency := normalizeCurrency(firstNonEmptyString(input.Currency, paymentMethod.RecommendedCurrency, s.cfg.Stripe.DefaultCurrency, "cny"))
	if !paymentMethod.SupportsCurrency(currency) {
		return nil, fmt.Errorf("%w: %s 当前仅支持 %s 测试", ErrInvalidInput, paymentMethod.Label, strings.ToUpper(strings.Join(paymentMethod.SupportedCurrencies, "/")))
	}
	title := strings.TrimSpace(input.Title)
	if title == "" {
		title = "Stripe Admin Test Payment"
	}
	description := strings.TrimSpace(input.Description)
	if description == "" {
		description = fmt.Sprintf("%s (%s)", title, strings.ToUpper(currency))
	}

	now := time.Now().UTC()
	paymentID := "pay_" + strings.ReplaceAll(uuid.NewString(), "-", "")[:24]
	idempotencyKey := "admin-checkout:" + paymentID
	expiresAt := now.Add(time.Duration(s.cfg.Stripe.CheckoutExpireMinutes) * time.Minute)
	if s.cfg.Stripe.CheckoutExpireMinutes <= 0 {
		expiresAt = now.Add(60 * time.Minute)
	}
	successURL := buildAdminReturnURL(s.cfg.AppPublicBaseURL, s.cfg.Stripe.SuccessPath, paymentID, "success")
	cancelURL := buildAdminReturnURL(s.cfg.AppPublicBaseURL, s.cfg.Stripe.CancelPath, paymentID, "cancel")
	metadata := map[string]string{
		"payment_id":               paymentID,
		"purpose":                  purpose,
		"scenario_type":            ScenarioHostedCheckout,
		"trigger_admin_id":         adminID,
		"requested_payment_method": paymentMethod.Code,
	}
	metadataJSON, _ := json.Marshal(metadata)

	record := &PaymentRecord{
		ID:                   paymentID,
		Provider:             ProviderStripe,
		Purpose:              purpose,
		ScenarioType:         ScenarioHostedCheckout,
		Mode:                 normalizeStripeMode(s.cfg.Stripe.Mode),
		Status:               StatusInitiated,
		TriggerAdminID:       adminID,
		Title:                title,
		AmountMinor:          amountMinor,
		Currency:             currency,
		PaymentMethodRequest: paymentMethod.Code,
		SuccessURL:           successURL,
		CancelURL:            cancelURL,
		IdempotencyKey:       idempotencyKey,
		MetadataJSON:         string(metadataJSON),
		SessionExpiresAt:     &expiresAt,
		CreatedAt:            now,
		UpdatedAt:            now,
	}
	if err := s.repo.CreatePayment(ctx, record); err != nil {
		return nil, err
	}

	session, err := s.gateway.CreateCheckoutSession(ctx, stripeCheckoutSessionRequest{
		PaymentID:          paymentID,
		AmountMinor:        amountMinor,
		Currency:           currency,
		PaymentMethodCode:  paymentMethod.Code,
		PaymentMethodTypes: cloneStringSlice(paymentMethod.CheckoutPaymentMethodTypes),
		WeChatPayClient:    paymentMethod.WeChatPayClient,
		SuccessURL:         successURL,
		CancelURL:          cancelURL,
		Title:              title,
		Description:        description,
		IdempotencyKey:     idempotencyKey,
		Metadata:           metadata,
		ExpiresAt:          &expiresAt,
	})
	if err != nil {
		code, message := stripeErrorInfo(err)
		record.Status = StatusFailed
		record.LastErrorCode = code
		record.LastErrorMessage = message
		record.FailedAt = &now
		record.UpdatedAt = now
		_ = s.repo.UpdatePayment(ctx, record)
		_ = s.repo.CreateEvent(ctx, &PaymentEventRecord{
			ID:                uuid.NewString(),
			PaymentID:         record.ID,
			Provider:          ProviderStripe,
			Source:            EventSourceAdmin,
			EventType:         "checkout.session.create_failed",
			ObjectType:        "checkout_session",
			ObjectID:          "",
			StatusBefore:      StatusInitiated,
			StatusAfter:       StatusFailed,
			SignatureVerified: false,
			Processed:         false,
			ErrorMessage:      message,
			PayloadJSON:       "",
			ReceivedAt:        now,
			CreatedAt:         now,
		})
		return nil, err
	}

	record.Status = StatusCheckoutOpen
	record.CheckoutSessionID = nullableString(session.SessionID)
	record.CheckoutURL = session.URL
	record.PaymentIntentID = nullableString(session.PaymentIntentID)
	record.UpdatedAt = now
	if session.ExpiresAt != nil {
		record.SessionExpiresAt = session.ExpiresAt
	}
	if len(session.PaymentMethodTypes) > 0 {
		record.PaymentMethodRequest = strings.Join(session.PaymentMethodTypes, ",")
	}
	if err := s.repo.UpdatePayment(ctx, record); err != nil {
		return nil, err
	}
	eventPayload, _ := json.Marshal(session)
	if err := s.repo.CreateEvent(ctx, &PaymentEventRecord{
		ID:                uuid.NewString(),
		PaymentID:         record.ID,
		Provider:          ProviderStripe,
		Source:            EventSourceAdmin,
		EventType:         "checkout.session.created",
		ObjectType:        "checkout_session",
		ObjectID:          stringValue(record.CheckoutSessionID),
		StatusBefore:      StatusInitiated,
		StatusAfter:       StatusCheckoutOpen,
		SignatureVerified: false,
		Processed:         true,
		PayloadJSON:       string(eventPayload),
		ReceivedAt:        now,
		CreatedAt:         now,
		OccurredAt:        &now,
	}); err != nil {
		return nil, err
	}

	return &CreateCheckoutSessionResult{
		PaymentID:          record.ID,
		Status:             record.Status,
		PaymentMethod:      paymentMethod.Code,
		CheckoutSessionID:  stringValue(record.CheckoutSessionID),
		CheckoutURL:        record.CheckoutURL,
		PaymentIntentID:    stringValue(record.PaymentIntentID),
		SessionExpiresAt:   formatTimePtr(record.SessionExpiresAt),
		AllowedPaymentNote: paymentMethod.TestingNote,
	}, nil
}

func (s *Service) ExpireAdminPayment(ctx context.Context, paymentID string) (*PaymentRecord, error) {
	if s.gateway == nil || strings.TrimSpace(s.cfg.Stripe.SecretKey) == "" {
		return nil, ErrStripeDisabled
	}
	record, err := s.repo.GetPaymentByID(ctx, paymentID)
	if err != nil {
		return nil, err
	}
	if record.Status != StatusCheckoutOpen {
		return nil, fmt.Errorf("%w: 当前状态不支持过期操作", ErrConflict)
	}
	checkoutSessionID := stringValue(record.CheckoutSessionID)
	if checkoutSessionID == "" {
		return nil, fmt.Errorf("%w: 缺少 checkout session", ErrConflict)
	}
	now := time.Now().UTC()
	session, err := s.gateway.ExpireCheckoutSession(ctx, checkoutSessionID)
	if err != nil {
		return nil, err
	}
	before := record.Status
	record.Status = StatusExpired
	record.ExpiredAt = &now
	record.LastStripeEventID = ""
	record.CheckoutURL = ""
	record.UpdatedAt = now
	if session != nil && session.ExpiresAt != nil {
		record.SessionExpiresAt = session.ExpiresAt
	}
	if err := s.repo.UpdatePayment(ctx, record); err != nil {
		return nil, err
	}
	payload, _ := json.Marshal(session)
	if err := s.repo.CreateEvent(ctx, &PaymentEventRecord{
		ID:                uuid.NewString(),
		PaymentID:         record.ID,
		Provider:          ProviderStripe,
		Source:            EventSourceAdmin,
		EventType:         "checkout.session.expire_requested",
		ObjectType:        "checkout_session",
		ObjectID:          checkoutSessionID,
		StatusBefore:      before,
		StatusAfter:       record.Status,
		SignatureVerified: false,
		Processed:         true,
		PayloadJSON:       string(payload),
		ReceivedAt:        now,
		CreatedAt:         now,
		OccurredAt:        &now,
	}); err != nil {
		return nil, err
	}
	return record, nil
}

func (s *Service) ListPayments(ctx context.Context, input ListPaymentsInput) ([]PaymentRecord, int64, error) {
	return s.repo.ListPayments(ctx, input)
}

func (s *Service) GetPaymentDetail(ctx context.Context, paymentID string) (*PaymentDetail, error) {
	record, err := s.repo.GetPaymentByID(ctx, paymentID)
	if err != nil {
		return nil, err
	}
	events, err := s.repo.ListEventsByPaymentID(ctx, paymentID, 100)
	if err != nil {
		return nil, err
	}
	return &PaymentDetail{Payment: record, Events: events}, nil
}

func (s *Service) HandleWebhook(ctx context.Context, payload []byte, signatureHeader string) error {
	if s.gateway == nil || strings.TrimSpace(s.cfg.Stripe.WebhookSecret) == "" {
		return ErrStripeWebhookMisconfigured
	}
	event, err := s.gateway.ParseWebhook(payload, signatureHeader)
	if err != nil {
		return err
	}
	return s.repo.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		repo := &Repository{db: tx}
		if strings.TrimSpace(event.ID) != "" {
			_, err := repo.GetEventByStripeEventID(ctx, event.ID)
			if err == nil {
				return nil
			}
			if err != nil && !errors.Is(err, ErrNotFound) {
				return err
			}
		}
		now := time.Now().UTC()
		eventRecord := &PaymentEventRecord{
			ID:                uuid.NewString(),
			Provider:          ProviderStripe,
			Source:            EventSourceWebhook,
			StripeEventID:     nullableString(event.ID),
			EventType:         event.Type,
			ObjectType:        event.ObjectType,
			ObjectID:          firstNonEmptyString(event.CheckoutSessionID, event.PaymentIntentID, event.ChargeID, event.RefundID),
			SignatureVerified: true,
			Processed:         false,
			PayloadJSON:       event.PayloadJSON,
			ReceivedAt:        now,
			CreatedAt:         now,
			OccurredAt:        event.OccurredAt,
		}

		record, err := resolvePaymentForWebhook(ctx, repo, event)
		if err != nil {
			if !errors.Is(err, ErrNotFound) {
				return err
			}
			eventRecord.ErrorMessage = "关联支付记录不存在"
			if err := repo.CreateEvent(ctx, eventRecord); err != nil {
				if isUniqueConstraintError(err) && stringValue(eventRecord.StripeEventID) != "" {
					return nil
				}
				return err
			}
			return nil
		}

		before := record.Status
		applyWebhookEvent(record, event, now)
		eventRecord.PaymentID = record.ID
		eventRecord.StatusBefore = before
		eventRecord.StatusAfter = record.Status
		eventRecord.Processed = true

		if err := repo.CreateEvent(ctx, eventRecord); err != nil {
			if isUniqueConstraintError(err) && stringValue(eventRecord.StripeEventID) != "" {
				return nil
			}
			return err
		}
		return repo.UpdatePayment(ctx, record)
	})
}

func resolvePaymentForWebhook(ctx context.Context, repo *Repository, event *stripeWebhookEvent) (*PaymentRecord, error) {
	if paymentID := strings.TrimSpace(event.PaymentID); paymentID != "" {
		record, err := repo.GetPaymentByID(ctx, paymentID)
		if err == nil {
			return record, nil
		}
		if err != nil && !errors.Is(err, ErrNotFound) {
			return nil, err
		}
	}
	if sessionID := strings.TrimSpace(event.CheckoutSessionID); sessionID != "" {
		record, err := repo.GetPaymentByCheckoutSessionID(ctx, sessionID)
		if err == nil {
			return record, nil
		}
		if err != nil && !errors.Is(err, ErrNotFound) {
			return nil, err
		}
	}
	if paymentIntentID := strings.TrimSpace(event.PaymentIntentID); paymentIntentID != "" {
		record, err := repo.GetPaymentByPaymentIntentID(ctx, paymentIntentID)
		if err == nil {
			return record, nil
		}
		if err != nil && !errors.Is(err, ErrNotFound) {
			return nil, err
		}
	}
	if chargeID := strings.TrimSpace(event.ChargeID); chargeID != "" {
		record, err := repo.GetPaymentByChargeID(ctx, chargeID)
		if err == nil {
			return record, nil
		}
		if err != nil && !errors.Is(err, ErrNotFound) {
			return nil, err
		}
	}
	return nil, ErrNotFound
}

func applyWebhookEvent(record *PaymentRecord, event *stripeWebhookEvent, now time.Time) {
	effectiveAt := now
	if event.OccurredAt != nil && !event.OccurredAt.IsZero() {
		effectiveAt = event.OccurredAt.UTC()
	}
	record.Mode = map[bool]string{true: "live", false: "test"}[event.Livemode]
	record.LastStripeEventID = strings.TrimSpace(event.ID)
	if event.CheckoutSessionID != "" {
		record.CheckoutSessionID = nullableString(event.CheckoutSessionID)
	}
	if event.PaymentIntentID != "" {
		record.PaymentIntentID = nullableString(event.PaymentIntentID)
	}
	if event.ChargeID != "" {
		record.ChargeID = nullableString(event.ChargeID)
	}
	if event.RefundID != "" {
		record.RefundID = strings.TrimSpace(event.RefundID)
	}
	if event.PaymentMethodType != "" {
		record.PaymentMethodSelected = strings.TrimSpace(event.PaymentMethodType)
	}

	switch event.Type {
	case string(stripe.EventTypeCheckoutSessionCompleted):
		if event.SessionPaymentStatus == "paid" {
			record.Status = StatusSucceeded
			record.CompletedAt = &effectiveAt
			record.LastErrorCode = ""
			record.LastErrorMessage = ""
		} else {
			record.Status = StatusProcessing
		}
	case string(stripe.EventTypePaymentIntentSucceeded):
		record.Status = StatusSucceeded
		record.CompletedAt = &effectiveAt
		record.LastErrorCode = ""
		record.LastErrorMessage = ""
	case string(stripe.EventTypePaymentIntentPaymentFailed):
		if !IsTerminalStatus(record.Status) || record.Status == StatusFailed {
			record.Status = StatusFailed
			record.FailedAt = &effectiveAt
			record.LastErrorCode = strings.TrimSpace(event.PaymentIntentErrorCode)
			record.LastErrorMessage = strings.TrimSpace(event.PaymentIntentErrorMsg)
		}
	case string(stripe.EventTypeCheckoutSessionExpired):
		if !IsTerminalStatus(record.Status) {
			record.Status = StatusExpired
			record.ExpiredAt = &effectiveAt
			record.CheckoutURL = ""
		}
	case string(stripe.EventTypeChargeRefunded):
		record.Status = StatusRefunded
		record.RefundedAt = &effectiveAt
	case string(stripe.EventTypeRefundUpdated):
		if strings.EqualFold(strings.TrimSpace(event.RefundStatus), "succeeded") {
			record.Status = StatusRefunded
			record.RefundedAt = &effectiveAt
		}
	}
	record.UpdatedAt = now
}

func stripeErrorInfo(err error) (string, string) {
	if err == nil {
		return "", ""
	}
	var stripeErr *stripe.Error
	if errors.As(err, &stripeErr) {
		return strings.TrimSpace(string(stripeErr.Code)), firstNonEmptyString(strings.TrimSpace(stripeErr.Msg), err.Error())
	}
	return "", err.Error()
}

func normalizeStripeMode(mode string) string {
	mode = strings.ToLower(strings.TrimSpace(mode))
	if mode == "live" {
		return "live"
	}
	return "test"
}

func normalizeCurrency(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "" {
		return "cny"
	}
	return value
}

func normalizeAllowedPaymentMethods(values []string) []string {
	seen := make(map[string]bool)
	result := make([]string, 0, len(values))
	for _, value := range values {
		normalized := strings.ToLower(strings.TrimSpace(value))
		if normalized == "" || seen[normalized] {
			continue
		}
		seen[normalized] = true
		result = append(result, normalized)
	}
	return result
}

type adminPaymentMethodSpec struct {
	Code                       string
	Label                      string
	CheckoutPaymentMethodTypes []string
	SupportedCurrencies        []string
	RecommendedCurrency        string
	CheckoutFlow               string
	Description                string
	TestingNote                string
	WeChatPayClient            string
}

func (s adminPaymentMethodSpec) SupportsCurrency(currency string) bool {
	currency = normalizeCurrency(currency)
	if len(s.SupportedCurrencies) == 0 {
		return true
	}
	for _, item := range s.SupportedCurrencies {
		if normalizeCurrency(item) == currency {
			return true
		}
	}
	return false
}

func buildAdminTestPaymentMethodViews(allowed []string) []AdminTestPaymentMethodView {
	allowedSet := make(map[string]bool)
	for _, code := range normalizeAllowedPaymentMethods(allowed) {
		allowedSet[code] = true
	}
	result := make([]AdminTestPaymentMethodView, 0, len(adminPaymentMethodOrder))
	for _, code := range adminPaymentMethodOrder {
		spec, ok := adminPaymentMethodSpecs[code]
		if !ok {
			continue
		}
		result = append(result, AdminTestPaymentMethodView{
			Code:                spec.Code,
			Label:               spec.Label,
			Enabled:             allowedSet[spec.Code],
			SupportedCurrencies: cloneStringSlice(spec.SupportedCurrencies),
			RecommendedCurrency: spec.RecommendedCurrency,
			CheckoutFlow:        spec.CheckoutFlow,
			Description:         spec.Description,
			TestingNote:         spec.TestingNote,
		})
	}
	return result
}

func resolveAdminPaymentMethod(input AdminCreateCheckoutSessionInput, allowed []string) (adminPaymentMethodSpec, error) {
	allowedMethods := normalizeAllowedPaymentMethods(allowed)
	if len(allowedMethods) == 0 {
		allowedMethods = []string{PaymentMethodCard}
	}
	allowedSet := make(map[string]bool, len(allowedMethods))
	for _, item := range allowedMethods {
		allowedSet[item] = true
	}
	requested := strings.ToLower(strings.TrimSpace(input.PaymentMethod))
	if requested == "" {
		for _, item := range input.PaymentMethodTypes {
			requested = strings.ToLower(strings.TrimSpace(item))
			if requested != "" {
				break
			}
		}
	}
	if requested == "" {
		requested = allowedMethods[0]
	}
	spec, ok := adminPaymentMethodSpecs[requested]
	if !ok {
		return adminPaymentMethodSpec{}, fmt.Errorf("%w: 不支持的支付方式 %s", ErrInvalidInput, requested)
	}
	if !allowedSet[requested] {
		return adminPaymentMethodSpec{}, fmt.Errorf("%w: 当前环境未启用 %s", ErrInvalidInput, spec.Label)
	}
	return spec, nil
}

var adminPaymentMethodOrder = []string{
	PaymentMethodCard,
	PaymentMethodAlipay,
	PaymentMethodWeChatPay,
}

var adminPaymentMethodSpecs = map[string]adminPaymentMethodSpec{
	PaymentMethodCard: {
		Code:                       PaymentMethodCard,
		Label:                      "银行卡",
		CheckoutPaymentMethodTypes: []string{PaymentMethodCard},
		SupportedCurrencies:        []string{"cny", "hkd"},
		RecommendedCurrency:        "cny",
		CheckoutFlow:               "hosted_checkout",
		Description:                "Stripe Hosted Checkout 直接收单，适合验证银行卡一次性支付链路。",
		TestingNote:                "银行卡支付会直接在 Stripe Hosted Checkout 内完成，不涉及跳转或扫码。",
	},
	PaymentMethodAlipay: {
		Code:                       PaymentMethodAlipay,
		Label:                      "支付宝",
		CheckoutPaymentMethodTypes: []string{PaymentMethodAlipay},
		SupportedCurrencies:        []string{"cny", "hkd"},
		RecommendedCurrency:        "cny",
		CheckoutFlow:               "redirect",
		Description:                "支付宝在 Hosted Checkout 中走跳转授权，仅用于一次性支付内测。",
		TestingNote:                "选择支付宝后会从 Stripe Checkout 跳转到支付宝授权页，完成授权后再回到 admin。",
	},
	PaymentMethodWeChatPay: {
		Code:                       PaymentMethodWeChatPay,
		Label:                      "微信支付",
		CheckoutPaymentMethodTypes: []string{PaymentMethodWeChatPay},
		SupportedCurrencies:        []string{"cny", "hkd"},
		RecommendedCurrency:        "cny",
		CheckoutFlow:               "qr_code",
		Description:                "微信支付在 PC 端 Hosted Checkout 下展示二维码，适合 admin 内测扫码支付链路。",
		TestingNote:                "PC 端会展示二维码，请使用手机微信扫码完成测试支付。",
		WeChatPayClient:            "web",
	},
}

func buildAdminReturnURL(baseURL, path, paymentID, checkoutState string) string {
	base, err := url.Parse(strings.TrimSpace(baseURL))
	if err != nil || base == nil || base.Scheme == "" || base.Host == "" {
		return ""
	}
	path = strings.TrimSpace(path)
	if path == "" {
		path = "/admin"
	}
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	target, err := url.Parse(path)
	if err != nil {
		target = &url.URL{Path: "/admin"}
	}
	resolved := base.ResolveReference(target)
	query := resolved.Query()
	if paymentID != "" {
		query.Set("payment_id", paymentID)
	}
	if checkoutState != "" {
		query.Set("checkout", checkoutState)
	}
	resolved.RawQuery = query.Encode()
	return resolved.String()
}

func formatTimePtr(value *time.Time) *string {
	if value == nil || value.IsZero() {
		return nil
	}
	formatted := value.UTC().Format(time.RFC3339)
	return &formatted
}
