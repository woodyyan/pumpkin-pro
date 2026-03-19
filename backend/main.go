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
	"github.com/woodyyan/pumpkin-pro/backend/store/auth"
	"github.com/woodyyan/pumpkin-pro/backend/store/live"
	"github.com/woodyyan/pumpkin-pro/backend/store/strategy"
)

var supportedDataSources = []string{"online", "csv", "sample"}

type appServer struct {
	cfg             config.Config
	authService     *auth.Service
	strategyService *strategy.Service
	liveService     *live.Service
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
			next(w, r)
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

	a.proxyToQuant(w, r, "/api/backtest", encodedBody)
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
	default:
		writeError(w, http.StatusMethodNotAllowed, "Only GET and PUT methods are allowed")
	}
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
	overview, err := a.liveService.GetMarketOverview(r.Context())
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
		message = "股票代码格式无效，需为 5 位港股代码（如 00700.HK）"
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

	server := &appServer{
		cfg:             cfg,
		authService:     authService,
		strategyService: strategyService,
		liveService:     liveService,
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
	mux.HandleFunc("/api/strategies", server.withOptionalAuth(server.handleStrategies))
	mux.HandleFunc("/api/strategies/active", server.withOptionalAuth(server.handleActiveStrategies))
	mux.HandleFunc("/api/strategies/", server.withOptionalAuth(server.handleStrategySubroutes))

	mux.HandleFunc("/api/live/watchlist", server.withRequiredAuth(server.handleLiveWatchlist))
	mux.HandleFunc("/api/live/watchlist/", server.withRequiredAuth(server.handleLiveWatchlistSubroutes))
	mux.HandleFunc("/api/live/market/overview", server.handleLiveMarketOverview)
	mux.HandleFunc("/api/live/symbols/", server.withOptionalAuth(server.handleLiveSymbolsSubroutes))

	handler := corsMiddleware(mux)
	log.Printf("🚀 Pumpkin Go Backend is running on port %s (db=%s)", cfg.Port, cfg.DB.Type)
	if err := http.ListenAndServe(fmt.Sprintf(":%s", cfg.Port), handler); err != nil {
		log.Fatalf("Server failed to start: %v", err)
	}
}
