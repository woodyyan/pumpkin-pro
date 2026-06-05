package main

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"

	"github.com/woodyyan/pumpkin-pro/backend/store/admin"
	"github.com/woodyyan/pumpkin-pro/backend/store/payment"
)

func cleanPaymentErrorDetail(err error) string {
	detail := strings.TrimSpace(err.Error())
	prefixes := []string{
		"payment: invalid input: ",
		"payment: conflict: ",
		"payment: not found: ",
		"payment: stripe disabled: ",
		"payment: stripe webhook misconfigured: ",
		"payment: unsupported stripe mode: ",
		"invalid input: ",
		"conflict: ",
		"not found: ",
		"payment: ",
	}
	for {
		trimmed := detail
		for _, prefix := range prefixes {
			if strings.HasPrefix(trimmed, prefix) {
				trimmed = strings.TrimSpace(strings.TrimPrefix(trimmed, prefix))
				break
			}
		}
		if trimmed == detail {
			return trimmed
		}
		detail = trimmed
	}
}

func (a *appServer) handleAdminPaymentConfig(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "Only GET method is allowed")
		return
	}
	if a.paymentService == nil {
		writeError(w, http.StatusServiceUnavailable, "支付服务未初始化")
		return
	}
	writeJSON(w, http.StatusOK, a.paymentService.GetConfigView(r.Context()))
}

func (a *appServer) handleAdminPaymentCheckoutSessions(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "Only POST method is allowed")
		return
	}
	currentAdmin, ok := admin.CurrentAdminFromContext(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]any{"code": "ADMIN_AUTH_REQUIRED", "detail": "需要超级管理员登录"})
		return
	}
	var input payment.AdminCreateCheckoutSessionInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "支付请求格式错误")
		return
	}
	result, err := a.paymentService.CreateAdminCheckoutSession(r.Context(), currentAdmin.AdminID, input)
	if err != nil {
		detail := cleanPaymentErrorDetail(err)
		switch {
		case errors.Is(err, payment.ErrInvalidInput):
			writeJSON(w, http.StatusBadRequest, map[string]any{"code": "INVALID_INPUT", "detail": detail})
		case errors.Is(err, payment.ErrStripeDisabled), errors.Is(err, payment.ErrStripeWebhookMisconfigured):
			writeJSON(w, http.StatusConflict, map[string]any{"code": "STRIPE_NOT_READY", "detail": "Stripe 测试配置未完成，请检查 Secret Key 与 Webhook Secret"})
		case errors.Is(err, payment.ErrUnsupportedMode):
			writeJSON(w, http.StatusConflict, map[string]any{"code": "STRIPE_MODE_INVALID", "detail": "admin 支付测试仅允许在 test 模式下执行"})
		default:
			writeJSON(w, http.StatusBadGateway, map[string]any{"code": "STRIPE_REQUEST_FAILED", "detail": err.Error()})
		}
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (a *appServer) handleAdminPayments(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "Only GET method is allowed")
		return
	}
	if a.paymentService == nil {
		writeError(w, http.StatusServiceUnavailable, "支付服务未初始化")
		return
	}
	items, total, err := a.paymentService.ListPayments(r.Context(), payment.ListPaymentsInput{
		Purpose: strings.TrimSpace(r.URL.Query().Get("purpose")),
		Status:  strings.TrimSpace(r.URL.Query().Get("status")),
		Limit:   parseLimit(r.URL.Query().Get("limit"), 20),
		Offset:  parseOffset(r.URL.Query().Get("offset"), 0),
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "加载支付记录失败")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"items": items,
		"total": total,
	})
}

func (a *appServer) handleAdminPaymentSubroutes(w http.ResponseWriter, r *http.Request) {
	if a.paymentService == nil {
		writeError(w, http.StatusServiceUnavailable, "支付服务未初始化")
		return
	}
	path := strings.TrimPrefix(r.URL.Path, "/api/admin/payments/")
	path = strings.Trim(path, "/")
	if path == "" {
		writeError(w, http.StatusNotFound, "Not found")
		return
	}
	parts := strings.Split(path, "/")
	paymentID := strings.TrimSpace(parts[0])
	if paymentID == "" {
		writeError(w, http.StatusBadRequest, "payment_id 不能为空")
		return
	}
	if len(parts) == 1 {
		if r.Method != http.MethodGet {
			writeError(w, http.StatusMethodNotAllowed, "Only GET method is allowed")
			return
		}
		detail, err := a.paymentService.GetPaymentDetail(r.Context(), paymentID)
		if err != nil {
			if errors.Is(err, payment.ErrNotFound) {
				writeJSON(w, http.StatusNotFound, map[string]any{"code": "PAYMENT_NOT_FOUND", "detail": "支付记录不存在"})
				return
			}
			writeError(w, http.StatusInternalServerError, "加载支付详情失败")
			return
		}
		writeJSON(w, http.StatusOK, detail)
		return
	}
	if len(parts) == 2 && parts[1] == "expire" {
		if r.Method != http.MethodPost {
			writeError(w, http.StatusMethodNotAllowed, "Only POST method is allowed")
			return
		}
		record, err := a.paymentService.ExpireAdminPayment(r.Context(), paymentID)
		if err != nil {
			switch {
			case errors.Is(err, payment.ErrNotFound):
				writeJSON(w, http.StatusNotFound, map[string]any{"code": "PAYMENT_NOT_FOUND", "detail": "支付记录不存在"})
			case errors.Is(err, payment.ErrConflict):
				detail := cleanPaymentErrorDetail(err)
				writeJSON(w, http.StatusConflict, map[string]any{"code": "PAYMENT_STATE_CONFLICT", "detail": detail})
			default:
				writeJSON(w, http.StatusBadGateway, map[string]any{"code": "STRIPE_REQUEST_FAILED", "detail": err.Error()})
			}
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"payment": record})
		return
	}
	writeError(w, http.StatusNotFound, "Not found")
}

func (a *appServer) handleStripeWebhook(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "Only POST method is allowed")
		return
	}
	if a.paymentService == nil {
		writeError(w, http.StatusServiceUnavailable, "支付服务未初始化")
		return
	}
	payload, err := io.ReadAll(r.Body)
	if err != nil {
		writeError(w, http.StatusBadRequest, "读取 webhook 失败")
		return
	}
	if err := a.paymentService.HandleWebhook(r.Context(), payload, r.Header.Get("Stripe-Signature")); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"code": "STRIPE_WEBHOOK_INVALID", "detail": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"received": true})
}
