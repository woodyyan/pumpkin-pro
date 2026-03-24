package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/woodyyan/pumpkin-pro/backend/config"
	"github.com/woodyyan/pumpkin-pro/backend/store"
	"github.com/woodyyan/pumpkin-pro/backend/store/admin"
	"github.com/woodyyan/pumpkin-pro/backend/store/auth"
	"github.com/woodyyan/pumpkin-pro/backend/store/backtest"
	"github.com/woodyyan/pumpkin-pro/backend/store/live"
	"github.com/woodyyan/pumpkin-pro/backend/store/signal"
	"github.com/woodyyan/pumpkin-pro/backend/store/strategy"
)

var supportedDataSources = []string{"online", "csv", "sample"}

type appServer struct {
	cfg              config.Config
	authService      *auth.Service
	strategyService  *strategy.Service
	liveService      *live.Service
	signalService    *signal.Service
	adminService     *admin.Service
	backtestService  *backtest.Service
}

func corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "POST, GET, OPTIONS, PUT, DELETE")
		w.Header().Set("Access-Control-Allow-Headers", "Accept, Content-Type, Content-Length, Accept-Encoding, X-CSRF-Token, Authorization")

		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusOK)
			return
		}

		next.ServeHTTP(w, r)
	})
}

func (a *appServer) withOptionalAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		token := parseBearerToken(r.Header.Get("Authorization"))
		if token == "" {
			next(w, r)
			return
		}

		claims, err := a.authService.ParseAccessToken(token)
		if err != nil {
			writeAuthRequired(w)
			return
		}
		ctx := auth.WithCurrentUser(r.Context(), auth.CurrentUser{UserID: claims.UserID, Email: claims.Email})
		next(w, r.WithContext(ctx))
	}
}

func (a *appServer) withRequiredAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		token := parseBearerToken(r.Header.Get("Authorization"))
		if token == "" {
			writeAuthRequired(w)
			return
		}
		claims, err := a.authService.ParseAccessToken(token)
		if err != nil {
			writeAuthRequired(w)
			return
		}
		ctx := auth.WithCurrentUser(r.Context(), auth.CurrentUser{UserID: claims.UserID, Email: claims.Email})
		next(w, r.WithContext(ctx))
	}
}

func parseBearerToken(header string) string {
	text := strings.TrimSpace(header)
	if text == "" {
		return ""
	}
	if !strings.HasPrefix(strings.ToLower(text), "bearer ") {
		return ""
	}
	return strings.TrimSpace(text[7:])
}

func currentUser(r *http.Request) (auth.CurrentUser, bool) {
	if r == nil {
		return auth.CurrentUser{}, false
	}
	return auth.CurrentUserFromContext(r.Context())
}

func currentUserID(r *http.Request) string {
	user, ok := currentUser(r)
	if !ok {
		return ""
	}
	return user.UserID
}

func writeAuthRequired(w http.ResponseWriter) {
	writeJSON(w, http.StatusUnauthorized, map[string]any{
		"code":   "AUTH_REQUIRED",
		"detail": "该操作需要登录后使用",
	})
}

func (a *appServer) handleAuthRegister(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "Only POST method is allowed")
		return
	}
	var input auth.RegisterInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "注册请求格式错误")
		return
	}
	result, err := a.authService.Register(r.Context(), input, auth.ClientIP(r), r.UserAgent())
	if err != nil {
		a.writeAuthError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (a *appServer) handleAuthLogin(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "Only POST method is allowed")
		return
	}
	var input auth.LoginInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "登录请求格式错误")
		return
	}
	result, err := a.authService.Login(r.Context(), input, auth.ClientIP(r), r.UserAgent())
	if err != nil {
		a.writeAuthError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (a *appServer) handleAuthRefresh(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "Only POST method is allowed")
		return
	}
	var input auth.RefreshInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "刷新请求格式错误")
		return
	}
	result, err := a.authService.Refresh(r.Context(), input, auth.ClientIP(r), r.UserAgent())
	if err != nil {
		a.writeAuthError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (a *appServer) handleAuthLogout(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "Only POST method is allowed")
		return
	}
	var input auth.LogoutInput
	_ = json.NewDecoder(r.Body).Decode(&input)
	if err := a.authService.Logout(r.Context(), currentUserID(r), input, auth.ClientIP(r), r.UserAgent()); err != nil {
		a.writeAuthError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (a *appServer) handleUserMe(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		profile, err := a.authService.GetProfile(r.Context(), currentUserID(r))
		if err != nil {
			a.writeAuthError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"user": profile})
	case http.MethodPut:
		var input auth.UpdateProfileInput
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
			writeError(w, http.StatusBadRequest, "更新资料请求格式错误")
			return
		}
		profile, err := a.authService.UpdateProfile(r.Context(), currentUserID(r), input)
		if err != nil {
			a.writeAuthError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"user": profile})
	default:
		writeError(w, http.StatusMethodNotAllowed, "Only GET and PUT methods are allowed")
	}
}

func (a *appServer) handleUserChangePassword(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "Only POST method is allowed")
		return
	}
	var input auth.ChangePasswordInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "修改密码请求格式错误")
		return
	}
	if err := a.authService.ChangePassword(r.Context(), currentUserID(r), input); err != nil {
		a.writeAuthError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (a *appServer) writeAuthError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, auth.ErrInvalidInput):
		writeJSON(w, http.StatusBadRequest, map[string]any{"code": "INVALID_INPUT", "detail": err.Error()})
	case errors.Is(err, auth.ErrInvalidCredential):
		writeJSON(w, http.StatusUnauthorized, map[string]any{"code": "INVALID_CREDENTIAL", "detail": "邮箱或密码错误"})
	case errors.Is(err, auth.ErrEmailAlreadyExists):
		writeJSON(w, http.StatusConflict, map[string]any{"code": "EMAIL_EXISTS", "detail": "邮箱已被注册"})
	case errors.Is(err, auth.ErrUnauthorized):
		writeJSON(w, http.StatusUnauthorized, map[string]any{"code": "AUTH_REQUIRED", "detail": "登录已失效，请重新登录"})
	case errors.Is(err, auth.ErrForbidden):
		writeJSON(w, http.StatusForbidden, map[string]any{"code": "FORBIDDEN", "detail": "当前账号不可用"})
	default:
		writeJSON(w, http.StatusInternalServerError, map[string]any{"code": "INTERNAL_ERROR", "detail": err.Error()})
	}
}

func (a *appServer) handleHealth(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "Only GET method is allowed")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"status":  "online",
		"service": "Pumpkin Go Backend",
		"db_type": a.cfg.DB.Type,
	})
}

func (a *appServer) handleBacktest(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "Only POST method is allowed")
		return
	}

	payload, err := decodeBodyAsMap(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "请求体格式错误")
		return
	}

	strategyID := asString(payload["strategy_id"])
	strategyName := asString(payload["strategy_name"])
	strategyParams := asMap(payload["strategy_params"])

	runtimeStrategy, err := a.strategyService.BuildRuntimeStrategy(r.Context(), currentUserID(r), strategyID, strategyName, strategyParams)
	if err != nil {
		a.writeStrategyError(w, err)
		return
	}
	payload["runtime_strategy"] = runtimeStrategy

	encodedBody, err := json.Marshal(payload)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "序列化回测请求失败")
		return
	}

	userID := currentUserID(r)
	startTime := time.Now()

	// If logged-in user, intercept the response to save it
	if strings.TrimSpace(userID) != "" {
		a.proxyToQuantAndSave(w, r, "/api/backtest", encodedBody, userID, payload, startTime)
		return
	}

	a.proxyToQuant(w, r, "/api/backtest", encodedBody)
}

func (a *appServer) proxyToQuantAndSave(w http.ResponseWriter, r *http.Request, targetPath string, body []byte, userID string, requestPayload map[string]any, startTime time.Time) {
	targetURL := a.cfg.QuantServiceURL + targetPath
	req, err := http.NewRequestWithContext(r.Context(), r.Method, targetURL, bytes.NewReader(body))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to create proxy request")
		return
	}

	copyForwardHeaders(r, req)
	if req.Header.Get("Content-Type") == "" {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		log.Printf("Error calling quant service: %v", err)
		writeError(w, http.StatusServiceUnavailable, "Failed to connect to quant engine")
		return
	}
	defer resp.Body.Close()

	// Read the full response body
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Printf("Error reading quant response: %v", err)
		writeError(w, http.StatusInternalServerError, "Failed to read quant engine response")
		return
	}

	durationMS := time.Since(startTime).Milliseconds()

	// Write back to the client
	for key, values := range resp.Header {
		for _, value := range values {
			w.Header().Add(key, value)
		}
	}
	if w.Header().Get("Content-Type") == "" {
		w.Header().Set("Content-Type", "application/json")
	}
	w.WriteHeader(resp.StatusCode)
	if _, err := w.Write(respBody); err != nil {
		log.Printf("Error writing response: %v", err)
	}

	// Async save to DB
	go func() {
		var result map[string]any
		if err := json.Unmarshal(respBody, &result); err != nil {
			log.Printf("[backtest] failed to unmarshal response for saving: %v", err)
			return
		}

		status := "success"
		if resp.StatusCode >= 400 {
			status = "failed"
		}

		a.backtestService.SaveRunAsync(userID, requestPayload, result, durationMS, status)
	}()
}

func (a *appServer) handleBacktestRuns(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "Only GET method is allowed")
		return
	}

	userID := currentUserID(r)
	limit := parseLimit(r.URL.Query().Get("limit"), 20)
	offset := parseOffset(r.URL.Query().Get("offset"), 0)

	items, total, err := a.backtestService.List(r.Context(), userID, limit, offset)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "加载回测历史失败")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"items": items,
		"total": total,
	})
}

func (a *appServer) handleBacktestRunSubroutes(w http.ResponseWriter, r *http.Request) {
	suffix := strings.TrimPrefix(r.URL.Path, "/api/backtest/runs/")
	suffix = strings.TrimSpace(strings.Trim(suffix, "/"))
	if suffix == "" {
		http.NotFound(w, r)
		return
	}

	runID := suffix
	userID := currentUserID(r)

	switch r.Method {
	case http.MethodGet:
		detail, err := a.backtestService.GetByID(r.Context(), userID, runID)
		if err != nil {
			a.writeBacktestError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, detail)
	case http.MethodDelete:
		if err := a.backtestService.Delete(r.Context(), userID, runID); err != nil {
			a.writeBacktestError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"deleted": true, "id": runID})
	default:
		writeError(w, http.StatusMethodNotAllowed, "Only GET and DELETE methods are allowed")
	}
}

func (a *appServer) writeBacktestError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, backtest.ErrNotFound):
		writeError(w, http.StatusNotFound, "回测记录不存在")
	case errors.Is(err, backtest.ErrForbidden):
		writeError(w, http.StatusForbidden, "该操作需要登录后使用")
	default:
		writeError(w, http.StatusInternalServerError, err.Error())
	}
}

func (a *appServer) handleBacktestOptions(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "Only GET method is allowed")
		return
	}

	userID := currentUserID(r)
	activeStrategies, err := a.strategyService.List(r.Context(), userID, true, strings.TrimSpace(userID) != "")
	if err != nil {
		writeError(w, http.StatusInternalServerError, "加载回测配置失败")
		return
	}

	summaries := make([]strategy.StrategySummary, 0, len(activeStrategies))
	for _, item := range activeStrategies {
		summary := item.Description
		if len([]rune(summary)) > 72 {
			summary = string([]rune(summary)[:72])
		}
		summaries = append(summaries, strategy.StrategySummary{
			ID:                 item.ID,
			Key:                item.Key,
			Name:               item.Name,
			Category:           item.Category,
			Status:             item.Status,
			Description:        item.Description,
			DescriptionSummary: summary,
			ImplementationKey:  item.ImplementationKey,
			Version:            item.Version,
			UpdatedAt:          item.UpdatedAt,
		})
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"strategies":   summaries,
		"data_sources": supportedDataSources,
	})
}

func (a *appServer) handleStrategies(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		summaries, err := a.strategyService.ListSummaries(r.Context(), currentUserID(r))
		if err != nil {
			writeError(w, http.StatusInternalServerError, "加载策略列表失败")
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"items":               summaries,
			"implementation_keys": a.strategyService.ImplementationKeys(),
		})
	case http.MethodPost:
		payload, err := decodeStrategyPayload(r)
		if err != nil {
			writeError(w, http.StatusBadRequest, "策略请求格式错误")
			return
		}
		created, err := a.strategyService.Create(r.Context(), currentUserID(r), payload)
		if err != nil {
			a.writeStrategyError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"item": created})
	default:
		writeError(w, http.StatusMethodNotAllowed, "Only GET and POST methods are allowed")
	}
}

func (a *appServer) handleActiveStrategies(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "Only GET method is allowed")
		return
	}

	userID := currentUserID(r)
	items, err := a.strategyService.List(r.Context(), userID, true, strings.TrimSpace(userID) != "")
	if err != nil {
		writeError(w, http.StatusInternalServerError, "加载启用策略失败")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (a *appServer) handleStrategySubroutes(w http.ResponseWriter, r *http.Request) {
	suffix := strings.TrimPrefix(r.URL.Path, "/api/strategies/")
	suffix = strings.TrimSpace(suffix)
	if suffix == "" || suffix == r.URL.Path {
		http.NotFound(w, r)
		return
	}

	if strings.HasSuffix(suffix, "/definition") {
		strategyID := strings.TrimSuffix(suffix, "/definition")
		a.handleStrategyDefinition(w, r, strategyID)
		return
	}

	a.handleStrategyDetail(w, r, suffix)
}

func (a *appServer) handleStrategyDefinition(w http.ResponseWriter, r *http.Request, strategyID string) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "Only GET method is allowed")
		return
	}

	item, err := a.strategyService.GetByID(r.Context(), currentUserID(r), strategyID)
	if err != nil {
		a.writeStrategyError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"item": item})
}

func (a *appServer) handleStrategyDetail(w http.ResponseWriter, r *http.Request, strategyID string) {
	switch r.Method {
	case http.MethodGet:
		item, err := a.strategyService.GetByID(r.Context(), currentUserID(r), strategyID)
		if err != nil {
			a.writeStrategyError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"item":                item,
			"implementation_keys": a.strategyService.ImplementationKeys(),
		})
	case http.MethodPut:
		payload, err := decodeStrategyPayload(r)
		if err != nil {
			writeError(w, http.StatusBadRequest, "策略请求格式错误")
			return
		}
		updated, err := a.strategyService.Update(r.Context(), currentUserID(r), strategyID, payload)
		if err != nil {
			a.writeStrategyError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"item": updated})
	case http.MethodDelete:
		userID := currentUserID(r)
		if strings.TrimSpace(userID) == "" {
			writeError(w, http.StatusForbidden, "该操作需要登录后使用")
			return
		}

		refCount, err := a.signalService.CountSymbolConfigRefsByStrategy(r.Context(), userID, strategyID)
		if err != nil {
			a.writeSignalError(w, err)
			return
		}
		if refCount > 0 {
			writeError(w, http.StatusConflict, fmt.Sprintf("该策略已被 %d 个股票信号配置引用，请先在实盘页更换后再删除", refCount))
			return
		}

		if err := a.strategyService.Delete(r.Context(), userID, strategyID); err != nil {
			a.writeStrategyError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"deleted": true, "id": strategyID})
	default:
		writeError(w, http.StatusMethodNotAllowed, "Only GET, PUT and DELETE methods are allowed")
	}
}

func (a *appServer) handleWebhookConfig(w http.ResponseWriter, r *http.Request) {
	userID := currentUserID(r)
	switch r.Method {
	case http.MethodGet:
		item, err := a.signalService.GetWebhookEndpoint(r.Context(), userID)
		if err != nil {
			a.writeSignalError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"item": item})
	case http.MethodPut:
		payload, err := decodeBodyAsMap(r)
		if err != nil {
			writeError(w, http.StatusBadRequest, "Webhook 配置请求格式错误")
			return
		}
		input := signal.WebhookConfigInput{
			URL:       asString(payload["url"]),
			Secret:    asString(payload["secret"]),
			IsEnabled: asBoolPtr(payload["is_enabled"]),
			TimeoutMS: asInt(payload["timeout_ms"]),
		}
		item, err := a.signalService.UpsertWebhookEndpoint(r.Context(), userID, input)
		if err != nil {
			a.writeSignalError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"item": item})
	default:
		writeError(w, http.StatusMethodNotAllowed, "Only GET and PUT methods are allowed")
	}
}

func (a *appServer) handleWebhookTest(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "Only POST method is allowed")
		return
	}
	payload, err := decodeBodyAsMap(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "Webhook 测试请求格式错误")
		return
	}
	result, err := a.signalService.SendTestSignal(r.Context(), currentUserID(r), signal.TestSignalInput{
		Symbol: asString(payload["symbol"]),
		Side:   asString(payload["side"]),
	})
	if err != nil {
		a.writeSignalError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (a *appServer) handleSignalConfigs(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "Only GET method is allowed")
		return
	}
	items, err := a.signalService.ListSymbolConfigs(r.Context(), currentUserID(r))
	if err != nil {
		a.writeSignalError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (a *appServer) handleSignalConfigSubroutes(w http.ResponseWriter, r *http.Request) {
	suffix := strings.TrimPrefix(r.URL.Path, "/api/signal-configs/")
	suffix = strings.Trim(strings.TrimSpace(suffix), "/")
	if suffix == "" {
		http.NotFound(w, r)
		return
	}

	if strings.HasSuffix(suffix, "/test") {
		symbol := strings.TrimSpace(strings.TrimSuffix(suffix, "/test"))
		if r.Method != http.MethodPost {
			writeError(w, http.StatusMethodNotAllowed, "Only POST method is allowed")
			return
		}
		payload, err := decodeBodyAsMap(r)
		if err != nil {
			writeError(w, http.StatusBadRequest, "测试信号请求格式错误")
			return
		}
		result, err := a.signalService.SendTestSignal(r.Context(), currentUserID(r), signal.TestSignalInput{
			Symbol: symbol,
			Side:   asString(payload["side"]),
		})
		if err != nil {
			a.writeSignalError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, result)
		return
	}

	symbol := suffix
	switch r.Method {
	case http.MethodPut:
		payload, err := decodeBodyAsMap(r)
		if err != nil {
			writeError(w, http.StatusBadRequest, "信号配置请求格式错误")
			return
		}
		input := signal.SymbolSignalConfigInput{
			StrategyID:      asString(payload["strategy_id"]),
			IsEnabled:       asBoolPtr(payload["is_enabled"]),
			CooldownSeconds: asInt(payload["cooldown_seconds"]),
			Thresholds:      asMap(payload["thresholds"]),
		}

		if strings.TrimSpace(input.StrategyID) != "" {
			if _, err := a.strategyService.GetByID(r.Context(), currentUserID(r), input.StrategyID); err != nil {
				a.writeStrategyError(w, err)
				return
			}
		}

		item, err := a.signalService.UpsertSymbolConfig(r.Context(), currentUserID(r), symbol, input)
		if err != nil {
			a.writeSignalError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"item": item})
	case http.MethodDelete:
		if err := a.signalService.DeleteSymbolConfig(r.Context(), currentUserID(r), symbol); err != nil {
			a.writeSignalError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"deleted": true, "symbol": strings.ToUpper(symbol)})
	default:
		writeError(w, http.StatusMethodNotAllowed, "Only PUT, DELETE and POST methods are allowed")
	}
}

func (a *appServer) handleSignalEvents(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "Only GET method is allowed")
		return
	}
	symbol := strings.TrimSpace(r.URL.Query().Get("symbol"))
	limit := parseLimit(r.URL.Query().Get("limit"), 20)
	items, err := a.signalService.ListSignalEvents(r.Context(), currentUserID(r), symbol, limit)
	if err != nil {
		a.writeSignalError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (a *appServer) handleWebhookDeliveries(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "Only GET method is allowed")
		return
	}
	symbol := strings.TrimSpace(r.URL.Query().Get("symbol"))
	limit := parseLimit(r.URL.Query().Get("limit"), 20)
	items, err := a.signalService.ListDeliveries(r.Context(), currentUserID(r), symbol, limit)
	if err != nil {
		a.writeSignalError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (a *appServer) handleWebhookDeliveriesLatest(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "Only GET method is allowed")
		return
	}
	item, err := a.signalService.GetLatestDelivery(r.Context(), currentUserID(r))
	if err != nil {
		a.writeSignalError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"item": item})
}

func (a *appServer) handleLiveWatchlist(w http.ResponseWriter, r *http.Request) {
	userID := currentUserID(r)
	switch r.Method {
	case http.MethodGet:
		state, err := a.liveService.ListWatchlist(r.Context(), userID)
		if err != nil {
			a.writeLiveError(w, err)
			return
		}
		writeLiveJSON(w, http.StatusOK, map[string]any{
			"session_state": state.SessionState,
			"active_symbol": state.ActiveSymbol,
			"items":         state.Items,
		})
	case http.MethodPost:
		payload, err := decodeBodyAsMap(r)
		if err != nil {
			a.writeLiveError(w, live.ErrInvalidSymbol)
			return
		}
		item, err := a.liveService.AddWatchlist(r.Context(), userID, asString(payload["symbol"]), asString(payload["name"]))
		if err != nil {
			a.writeLiveError(w, err)
			return
		}
		writeLiveJSON(w, http.StatusOK, map[string]any{"item": item})
	default:
		writeError(w, http.StatusMethodNotAllowed, "Only GET and POST methods are allowed")
	}
}

func (a *appServer) handleLiveWatchlistSubroutes(w http.ResponseWriter, r *http.Request) {
	suffix := strings.TrimPrefix(r.URL.Path, "/api/live/watchlist/")
	suffix = strings.TrimSpace(strings.Trim(suffix, "/"))
	if suffix == "" {
		http.NotFound(w, r)
		return
	}

	userID := currentUserID(r)
	if strings.HasSuffix(suffix, "/activate") {
		symbol := strings.TrimSuffix(suffix, "/activate")
		if r.Method != http.MethodPatch {
			writeError(w, http.StatusMethodNotAllowed, "Only PATCH method is allowed")
			return
		}
		resetWindow := true
		payload, err := decodeBodyAsMap(r)
		if err == nil {
			if raw, ok := payload["reset_window"]; ok {
				if value, ok := raw.(bool); ok {
					resetWindow = value
				}
			}
		}
		result, err := a.liveService.ActivateSymbol(r.Context(), userID, symbol, resetWindow)
		if err != nil {
			a.writeLiveError(w, err)
			return
		}
		writeLiveJSON(w, http.StatusOK, result)
		return
	}

	if r.Method != http.MethodDelete {
		writeError(w, http.StatusMethodNotAllowed, "Only DELETE method is allowed")
		return
	}
	if err := a.liveService.DeleteWatchlist(r.Context(), userID, suffix); err != nil {
		a.writeLiveError(w, err)
		return
	}
	writeLiveJSON(w, http.StatusOK, map[string]any{"deleted": true, "symbol": strings.ToUpper(suffix)})
}

func (a *appServer) handleLiveMarketOverview(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "Only GET method is allowed")
		return
	}
	overview, err := a.liveService.GetMarketOverview(r.Context(), r.URL.Query().Get("exchange"))
	if err != nil {
		a.writeLiveError(w, err)
		return
	}
	writeLiveJSON(w, http.StatusOK, overview)
}

func (a *appServer) handleLiveSymbolsSubroutes(w http.ResponseWriter, r *http.Request) {
	suffix := strings.TrimPrefix(r.URL.Path, "/api/live/symbols/")
	suffix = strings.Trim(strings.TrimSpace(suffix), "/")
	parts := strings.Split(suffix, "/")
	if len(parts) < 2 {
		http.NotFound(w, r)
		return
	}

	symbol := parts[0]
	userID := currentUserID(r)
	route := strings.Join(parts[1:], "/")
	switch route {
	case "snapshot":
		if r.Method != http.MethodGet {
			writeError(w, http.StatusMethodNotAllowed, "Only GET method is allowed")
			return
		}
		snapshot, isActive, sessionState, err := a.liveService.GetSymbolSnapshot(r.Context(), userID, symbol)
		if err != nil {
			a.writeLiveError(w, err)
			return
		}
		writeLiveJSON(w, http.StatusOK, map[string]any{
			"is_active_symbol": isActive,
			"session_state":    sessionState,
			"snapshot":         snapshot,
		})
	case "overlay":
		if r.Method != http.MethodGet {
			writeError(w, http.StatusMethodNotAllowed, "Only GET method is allowed")
			return
		}
		windowMinutes := parseWindowMinutes(r.URL.Query().Get("window_minutes"), 60)
		benchmark := strings.TrimSpace(r.URL.Query().Get("benchmark"))
		overlay, err := a.liveService.GetOverlay(r.Context(), userID, symbol, windowMinutes, benchmark)
		if err != nil {
			a.writeLiveError(w, err)
			return
		}
		writeLiveJSON(w, http.StatusOK, overlay)
	case "support-levels":
		if r.Method != http.MethodGet {
			writeError(w, http.StatusMethodNotAllowed, "Only GET method is allowed")
			return
		}
		period := strings.TrimSpace(r.URL.Query().Get("period"))
		lookbackDays, parseErr := parseLookbackDays(r.URL.Query().Get("lookback_days"), 120)
		if parseErr != nil {
			a.writeLiveError(w, fmt.Errorf("%w: lookback_days must be a positive integer", live.ErrInvalidArgument))
			return
		}
		supportPayload, err := a.liveService.GetSupportLevels(r.Context(), userID, symbol, period, lookbackDays)
		if err != nil {
			a.writeLiveError(w, err)
			return
		}
		writeLiveJSON(w, http.StatusOK, supportPayload)
	case "resistance-levels":
		if r.Method != http.MethodGet {
			writeError(w, http.StatusMethodNotAllowed, "Only GET method is allowed")
			return
		}
		period := strings.TrimSpace(r.URL.Query().Get("period"))
		lookbackDays, parseErr := parseLookbackDays(r.URL.Query().Get("lookback_days"), 120)
		if parseErr != nil {
			a.writeLiveError(w, fmt.Errorf("%w: lookback_days must be a positive integer", live.ErrInvalidArgument))
			return
		}
		resistancePayload, err := a.liveService.GetResistanceLevels(r.Context(), userID, symbol, period, lookbackDays)
		if err != nil {
			a.writeLiveError(w, err)
			return
		}
		writeLiveJSON(w, http.StatusOK, resistancePayload)
	case "moving-averages":
		if r.Method != http.MethodGet {
			writeError(w, http.StatusMethodNotAllowed, "Only GET method is allowed")
			return
		}
		period := strings.TrimSpace(r.URL.Query().Get("period"))
		lookbackDays, parseErr := parseLookbackDays(r.URL.Query().Get("lookback_days"), 240)
		if parseErr != nil {
			a.writeLiveError(w, fmt.Errorf("%w: lookback_days must be a positive integer", live.ErrInvalidArgument))
			return
		}
		movingAveragesPayload, err := a.liveService.GetMovingAverages(r.Context(), userID, symbol, period, lookbackDays)
		if err != nil {
			a.writeLiveError(w, err)
			return
		}
		writeLiveJSON(w, http.StatusOK, movingAveragesPayload)
	case "anomalies/price-volume":
		if r.Method != http.MethodGet {
			writeError(w, http.StatusMethodNotAllowed, "Only GET method is allowed")
			return
		}
		since := parseSince(r.URL.Query().Get("since"))
		limit := parseLimit(r.URL.Query().Get("limit"), 50)
		types := splitCSV(r.URL.Query().Get("types"))
		items, sessionState, err := a.liveService.ListPriceVolumeAnomalies(r.Context(), userID, symbol, since, limit, types)
		if err != nil {
			a.writeLiveError(w, err)
			return
		}
		writeLiveJSON(w, http.StatusOK, map[string]any{
			"symbol":        strings.ToUpper(symbol),
			"session_state": sessionState,
			"items":         items,
		})
	case "anomalies/block-flow":
		if r.Method != http.MethodGet {
			writeError(w, http.StatusMethodNotAllowed, "Only GET method is allowed")
			return
		}
		since := parseSince(r.URL.Query().Get("since"))
		limit := parseLimit(r.URL.Query().Get("limit"), 50)
		items, sessionState, err := a.liveService.ListBlockFlowAnomalies(r.Context(), userID, symbol, since, limit)
		if err != nil {
			a.writeLiveError(w, err)
			return
		}
		writeLiveJSON(w, http.StatusOK, map[string]any{
			"symbol":        strings.ToUpper(symbol),
			"session_state": sessionState,
			"items":         items,
		})
	default:
		http.NotFound(w, r)
	}
}

func parseSince(raw string) time.Time {
	if strings.TrimSpace(raw) == "" {
		return time.Time{}
	}
	parsed, err := time.Parse(time.RFC3339, raw)
	if err != nil {
		return time.Time{}
	}
	return parsed
}

func parseLimit(raw string, fallback int) int {
	value, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil || value <= 0 {
		return fallback
	}
	if value > 200 {
		return 200
	}
	return value
}

func parseOffset(raw string, fallback int) int {
	value, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil || value < 0 {
		return fallback
	}
	return value
}

func parseWindowMinutes(raw string, fallback int) int {
	value, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil || value <= 0 {
		return fallback
	}
	if value > 240 {
		return 240
	}
	return value
}

func parseLookbackDays(raw string, fallback int) (int, error) {
	text := strings.TrimSpace(raw)
	if text == "" {
		return fallback, nil
	}
	value, err := strconv.Atoi(text)
	if err != nil || value <= 0 {
		return 0, fmt.Errorf("invalid lookback_days")
	}
	return value, nil
}

func splitCSV(raw string) []string {
	parts := strings.Split(raw, ",")
	items := make([]string, 0, len(parts))
	for _, part := range parts {
		text := strings.TrimSpace(part)
		if text != "" {
			items = append(items, text)
		}
	}
	return items
}

func writeLiveJSON(w http.ResponseWriter, statusCode int, payload any) {
	requestID := fmt.Sprintf("live-%d", time.Now().UnixNano())
	if payload == nil {
		writeJSON(w, statusCode, map[string]any{"request_id": requestID})
		return
	}

	if mapped, ok := payload.(map[string]any); ok {
		mapped["request_id"] = requestID
		writeJSON(w, statusCode, mapped)
		return
	}

	encoded, err := json.Marshal(payload)
	if err != nil {
		writeJSON(w, statusCode, map[string]any{"request_id": requestID})
		return
	}

	wrapper := map[string]any{"request_id": requestID}
	if err := json.Unmarshal(encoded, &wrapper); err != nil {
		wrapper["data"] = payload
	}
	writeJSON(w, statusCode, wrapper)
}

func (a *appServer) writeLiveError(w http.ResponseWriter, err error) {
	requestID := fmt.Sprintf("live-%d", time.Now().UnixNano())
	statusCode := http.StatusInternalServerError
	code := "INTERNAL_ERROR"
	message := err.Error()
	switch {
	case errors.Is(err, live.ErrInvalidSymbol):
		statusCode = http.StatusBadRequest
		code = "INVALID_SYMBOL"
		message = "股票代码格式无效，支持港股（如 00700.HK）或 A 股（如 600519.SH、000001.SZ）"
	case errors.Is(err, live.ErrConflict):
		statusCode = http.StatusConflict
		code = "SYMBOL_ALREADY_EXISTS"
		message = "该股票已在关注池中"
	case errors.Is(err, live.ErrNotFound):
		statusCode = http.StatusNotFound
		code = "ACTIVE_SYMBOL_NOT_FOUND"
		message = "关注股票不存在"
	case errors.Is(err, live.ErrInvalidArgument):
		statusCode = http.StatusBadRequest
		code = "INVALID_ARGUMENT"
		message = err.Error()
	case errors.Is(err, live.ErrDataSourceDown):
		statusCode = http.StatusServiceUnavailable
		code = "DATA_SOURCE_UNAVAILABLE"
		message = "行情数据源暂时不可用"
	case errors.Is(err, live.ErrWarmupNotReady):
		statusCode = http.StatusUnprocessableEntity
		code = "WARMUP_NOT_READY"
		message = "样本不足，指标计算仍在预热中"
	}
	writeJSON(w, statusCode, map[string]any{
		"request_id": requestID,
		"code":       code,
		"message":    message,
		"details":    map[string]any{"error": err.Error()},
	})
}

func (a *appServer) proxyToQuant(w http.ResponseWriter, r *http.Request, targetPath string, body []byte) {
	targetURL := a.cfg.QuantServiceURL + targetPath
	req, err := http.NewRequestWithContext(r.Context(), r.Method, targetURL, bytes.NewReader(body))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to create proxy request")
		return
	}

	copyForwardHeaders(r, req)
	if req.Header.Get("Content-Type") == "" {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		log.Printf("Error calling quant service: %v", err)
		writeError(w, http.StatusServiceUnavailable, "Failed to connect to quant engine")
		return
	}
	defer resp.Body.Close()

	for key, values := range resp.Header {
		for _, value := range values {
			w.Header().Add(key, value)
		}
	}
	if w.Header().Get("Content-Type") == "" {
		w.Header().Set("Content-Type", "application/json")
	}
	w.WriteHeader(resp.StatusCode)
	if _, err := io.Copy(w, resp.Body); err != nil {
		log.Printf("Error copying response: %v", err)
	}
}

func copyForwardHeaders(src *http.Request, dst *http.Request) {
	headers := []string{"Content-Type", "Accept", "Authorization"}
	for _, header := range headers {
		value := strings.TrimSpace(src.Header.Get(header))
		if value != "" {
			dst.Header.Set(header, value)
		}
	}
}

func decodeBodyAsMap(r *http.Request) (map[string]any, error) {
	defer r.Body.Close()
	decoder := json.NewDecoder(r.Body)
	decoder.UseNumber()

	payload := map[string]any{}
	if err := decoder.Decode(&payload); err != nil {
		return nil, err
	}
	return payload, nil
}

func decodeStrategyPayload(r *http.Request) (strategy.StrategyPayload, error) {
	defer r.Body.Close()
	decoder := json.NewDecoder(r.Body)
	decoder.UseNumber()

	var payload strategy.StrategyPayload
	if err := decoder.Decode(&payload); err != nil {
		return strategy.StrategyPayload{}, err
	}
	return payload, nil
}

func asMap(input any) map[string]any {
	if input == nil {
		return map[string]any{}
	}
	mapped, ok := input.(map[string]any)
	if !ok {
		return map[string]any{}
	}
	return mapped
}

func asString(input any) string {
	text, ok := input.(string)
	if !ok {
		return ""
	}
	return strings.TrimSpace(text)
}

func asBoolPtr(input any) *bool {
	if input == nil {
		return nil
	}
	value, ok := input.(bool)
	if !ok {
		return nil
	}
	return &value
}

func asInt(input any) int {
	switch value := input.(type) {
	case int:
		return value
	case int8:
		return int(value)
	case int16:
		return int(value)
	case int32:
		return int(value)
	case int64:
		return int(value)
	case float64:
		return int(value)
	case json.Number:
		parsed, err := strconv.Atoi(value.String())
		if err != nil {
			return 0
		}
		return parsed
	case string:
		parsed, err := strconv.Atoi(strings.TrimSpace(value))
		if err != nil {
			return 0
		}
		return parsed
	default:
		return 0
	}
}

func (a *appServer) writeStrategyError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, strategy.ErrNotFound):
		writeError(w, http.StatusNotFound, err.Error())
	case errors.Is(err, strategy.ErrConflict):
		writeError(w, http.StatusConflict, err.Error())
	case errors.Is(err, strategy.ErrInvalid):
		writeError(w, http.StatusBadRequest, err.Error())
	case errors.Is(err, strategy.ErrForbidden):
		writeError(w, http.StatusForbidden, "该操作需要登录后使用")
	default:
		writeError(w, http.StatusInternalServerError, err.Error())
	}
}

func (a *appServer) writeSignalError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, signal.ErrNotFound):
		writeError(w, http.StatusNotFound, err.Error())
	case errors.Is(err, signal.ErrConflict):
		writeError(w, http.StatusConflict, err.Error())
	case errors.Is(err, signal.ErrInvalidInput):
		writeError(w, http.StatusBadRequest, err.Error())
	case errors.Is(err, signal.ErrWebhookMissing):
		writeError(w, http.StatusBadRequest, "请先配置并保存 Webhook")
	case errors.Is(err, signal.ErrWebhookOff):
		writeError(w, http.StatusConflict, "Webhook 已禁用，请先启用")
	case errors.Is(err, signal.ErrWebhookDeliveryUndelivered):
		detail := strings.TrimPrefix(err.Error(), signal.ErrWebhookDeliveryUndelivered.Error()+": ")
		if strings.TrimSpace(detail) == "" {
			detail = "Webhook 未送达，请查看投递结果"
		}
		writeError(w, http.StatusBadGateway, detail)
	case errors.Is(err, signal.ErrForbidden):
		writeError(w, http.StatusForbidden, "该操作需要登录后使用")
	default:
		writeError(w, http.StatusInternalServerError, err.Error())
	}
}

// ── Super Admin ──

func (a *appServer) withSuperAdminAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		token := parseBearerToken(r.Header.Get("Authorization"))
		if token == "" {
			writeJSON(w, http.StatusUnauthorized, map[string]any{"code": "ADMIN_AUTH_REQUIRED", "detail": "需要超级管理员登录"})
			return
		}
		claims, err := a.adminService.ParseAdminToken(token)
		if err != nil {
			writeJSON(w, http.StatusUnauthorized, map[string]any{"code": "ADMIN_AUTH_REQUIRED", "detail": "超管登录已失效，请重新登录"})
			return
		}
		_ = claims
		next(w, r)
	}
}

func (a *appServer) handleAdminLogin(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "Only POST method is allowed")
		return
	}
	var input admin.AdminLoginInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "登录请求格式错误")
		return
	}
	result, err := a.adminService.Login(r.Context(), input)
	if err != nil {
		switch {
		case errors.Is(err, admin.ErrInvalidInput):
			writeJSON(w, http.StatusBadRequest, map[string]any{"code": "INVALID_INPUT", "detail": "请输入邮箱和密码"})
		case errors.Is(err, admin.ErrInvalidCredential):
			writeJSON(w, http.StatusUnauthorized, map[string]any{"code": "INVALID_CREDENTIAL", "detail": "邮箱或密码错误"})
		case errors.Is(err, admin.ErrForbidden):
			writeJSON(w, http.StatusForbidden, map[string]any{"code": "FORBIDDEN", "detail": "当前管理员账号不可用"})
		default:
			writeJSON(w, http.StatusInternalServerError, map[string]any{"code": "INTERNAL_ERROR", "detail": err.Error()})
		}
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (a *appServer) handleAdminStats(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "Only GET method is allowed")
		return
	}
	stats, err := a.adminService.GetStats(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "获取统计数据失败")
		return
	}
	writeJSON(w, http.StatusOK, stats)
}

func (a *appServer) handleScreenerScan(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "Only POST method is allowed")
		return
	}
	body, err := io.ReadAll(r.Body)
	if err != nil {
		writeError(w, http.StatusBadRequest, "请求体读取失败")
		return
	}
	defer r.Body.Close()
	a.proxyToQuant(w, r, "/api/screener/scan", body)
}

func writeError(w http.ResponseWriter, statusCode int, detail string) {
	writeJSON(w, statusCode, map[string]string{"detail": detail})
}

func writeJSON(w http.ResponseWriter, statusCode int, payload any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(statusCode)
	if err := json.NewEncoder(w).Encode(payload); err != nil {
		log.Printf("write json failed: %v", err)
	}
}

func main() {
	cfg := config.Load()
	storeInstance, err := store.New(cfg.DB)
	if err != nil {
		log.Fatalf("Failed to initialize store: %v", err)
	}

	authRepo := auth.NewRepository(storeInstance.DB)
	authService := auth.NewService(authRepo, auth.ServiceConfig{
		JWTSecret:  strings.TrimSpace(cfg.Auth.JWTSecret),
		AccessTTL:  time.Duration(cfg.Auth.AccessTokenTTLMinutes) * time.Minute,
		RefreshTTL: time.Duration(cfg.Auth.RefreshTokenTTLHours) * time.Hour,
	})

	strategyRepo := strategy.NewRepository(storeInstance.DB)
	strategyService := strategy.NewService(strategyRepo)
	if err := strategyService.SeedFromFileIfEmpty(context.Background(), cfg.StrategySeedPath); err != nil {
		log.Printf("Seed strategies skipped: %v", err)
	}

	liveRepo := live.NewRepository(storeInstance.DB)
	liveService := live.NewService(liveRepo)

	signalRepo := signal.NewRepository(storeInstance.DB)
	signalService := signal.NewService(signalRepo, signal.ServiceConfig{
		SecretKey: cfg.Auth.JWTSecret,
	})
	signalService.StartDispatcher(context.Background())

	adminRepo := admin.NewRepository(storeInstance.DB)
	adminService := admin.NewService(adminRepo, admin.ServiceConfig{
		JWTSecret: strings.TrimSpace(cfg.Auth.JWTSecret),
		AccessTTL: 2 * time.Hour,
	})
	if err := adminService.SeedAdmin(context.Background(), cfg.AdminSeed.Email, cfg.AdminSeed.Password); err != nil {
		log.Printf("Admin seed skipped: %v", err)
	}

	backtestRepo := backtest.NewRepository(storeInstance.DB)
	backtestService := backtest.NewService(backtestRepo)

	server := &appServer{
		cfg:              cfg,
		authService:      authService,
		strategyService:  strategyService,
		liveService:      liveService,
		signalService:    signalService,
		adminService:     adminService,
		backtestService:  backtestService,
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/api/health", server.handleHealth)

	mux.HandleFunc("/api/auth/register", server.handleAuthRegister)
	mux.HandleFunc("/api/auth/login", server.handleAuthLogin)
	mux.HandleFunc("/api/auth/refresh", server.handleAuthRefresh)
	mux.HandleFunc("/api/auth/logout", server.withRequiredAuth(server.handleAuthLogout))
	mux.HandleFunc("/api/user/me", server.withRequiredAuth(server.handleUserMe))
	mux.HandleFunc("/api/user/change-password", server.withRequiredAuth(server.handleUserChangePassword))

	mux.HandleFunc("/api/backtest", server.withOptionalAuth(server.handleBacktest))
	mux.HandleFunc("/api/backtest/options", server.withOptionalAuth(server.handleBacktestOptions))
	mux.HandleFunc("/api/backtest/runs", server.withRequiredAuth(server.handleBacktestRuns))
	mux.HandleFunc("/api/backtest/runs/", server.withRequiredAuth(server.handleBacktestRunSubroutes))
	mux.HandleFunc("/api/strategies", server.withOptionalAuth(server.handleStrategies))
	mux.HandleFunc("/api/strategies/active", server.withOptionalAuth(server.handleActiveStrategies))
	mux.HandleFunc("/api/strategies/", server.withOptionalAuth(server.handleStrategySubroutes))

	mux.HandleFunc("/api/webhook", server.withRequiredAuth(server.handleWebhookConfig))
	mux.HandleFunc("/api/webhook/test", server.withRequiredAuth(server.handleWebhookTest))
	mux.HandleFunc("/api/signal-configs", server.withRequiredAuth(server.handleSignalConfigs))
	mux.HandleFunc("/api/signal-configs/", server.withRequiredAuth(server.handleSignalConfigSubroutes))
	mux.HandleFunc("/api/signal-events", server.withRequiredAuth(server.handleSignalEvents))
	mux.HandleFunc("/api/webhook-deliveries", server.withRequiredAuth(server.handleWebhookDeliveries))
	mux.HandleFunc("/api/webhook-deliveries/latest", server.withRequiredAuth(server.handleWebhookDeliveriesLatest))

	mux.HandleFunc("/api/live/watchlist", server.withRequiredAuth(server.handleLiveWatchlist))
	mux.HandleFunc("/api/live/watchlist/", server.withRequiredAuth(server.handleLiveWatchlistSubroutes))
	mux.HandleFunc("/api/live/market/overview", server.handleLiveMarketOverview)
	mux.HandleFunc("/api/live/symbols/", server.withOptionalAuth(server.handleLiveSymbolsSubroutes))

	mux.HandleFunc("/api/admin/login", server.handleAdminLogin)
	mux.HandleFunc("/api/admin/stats", server.withSuperAdminAuth(server.handleAdminStats))

	mux.HandleFunc("/api/screener/scan", server.withOptionalAuth(server.handleScreenerScan))

	handler := corsMiddleware(mux)
	log.Printf("🚀 Pumpkin Go Backend is running on port %s (db=%s)", cfg.Port, cfg.DB.Type)
	if err := http.ListenAndServe(fmt.Sprintf(":%s", cfg.Port), handler); err != nil {
		log.Fatalf("Server failed to start: %v", err)
	}
}
