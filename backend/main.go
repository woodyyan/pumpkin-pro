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
	"github.com/woodyyan/pumpkin-pro/backend/store/analytics"
	"github.com/woodyyan/pumpkin-pro/backend/store/auth"
	"github.com/woodyyan/pumpkin-pro/backend/store/backtest"
	"github.com/woodyyan/pumpkin-pro/backend/store/live"
	"github.com/woodyyan/pumpkin-pro/backend/store/analysis_history"
	"github.com/woodyyan/pumpkin-pro/backend/store/fundcache"

	"gorm.io/gorm"
	"github.com/woodyyan/pumpkin-pro/backend/store/feedback"
	"github.com/woodyyan/pumpkin-pro/backend/store/portfolio"
	"github.com/woodyyan/pumpkin-pro/backend/store/quadrant"
	"github.com/woodyyan/pumpkin-pro/backend/store/screener"
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
	portfolioService *portfolio.Service
	quadrantService  *quadrant.Service
	adminService     *admin.Service
	backtestService  *backtest.Service
	screenerService  *screener.Service
	analyticsRepo    *analytics.Repository
	feedbackRepo     *feedback.Repository
	aiRateLimiter    *strategy.AIRateLimiter
	fundCacheRepo       *fundcache.Repository
	analysisHistoryRepo *analysis_history.Repository
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
		"service": "Wolong Pro Backend",
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

func (a *appServer) handleStrategyAIGenerate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "Only POST method is allowed")
		return
	}

	userID := currentUserID(r)
	if strings.TrimSpace(userID) == "" {
		writeError(w, http.StatusUnauthorized, "请先登录")
		return
	}

	// 限流检查
	if !a.aiRateLimiter.Allow(userID) {
		writeError(w, http.StatusTooManyRequests, "本小时 AI 生成次数已达上限（20 次/小时），请稍后再试")
		return
	}

	payload, err := decodeBodyAsMap(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "请求格式错误")
		return
	}
	ticker := strings.TrimSpace(asString(payload["ticker"]))
	if ticker == "" {
		writeError(w, http.StatusBadRequest, "请输入股票代码")
		return
	}

	// 获取技术指标数据
	maPayload, err := a.liveService.GetMovingAverages(r.Context(), userID, ticker, "daily", 240)
	if err != nil {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("无法获取该股票的技术指标数据：%v", err))
		return
	}

	// 获取快照（取股票名称）
	snapshot, _, _, snapErr := a.liveService.GetSymbolSnapshot(r.Context(), userID, ticker)
	stockName := ticker
	if snapErr == nil && snapshot != nil && snapshot.Name != "" && snapshot.Name != ticker {
		stockName = snapshot.Name
	}

	// 构建市场摘要
	summary := strategy.MarketSummary{
		Ticker:          maPayload.Symbol,
		Name:            stockName,
		Price:           maPayload.PriceRef,
		ChangePct60D:    maPayload.ChangePct60D,
		Volatility20D:   maPayload.Volatility20D,
		VolumeMA5toMA20: maPayload.VolumeMA5toMA20,
		RSI14:           maPayload.RSI14,
		RSI14Status:     maPayload.RSI14Status,
		MACD:            maPayload.MACD,
		MACDSignal:      maPayload.MACDSignal,
		MACDHistogram:   maPayload.MACDHistogram,
		BollingerBW:     maPayload.BollingerBandwidth,
		BollingerPctB:   maPayload.BollingerPercentB,
		MA5:             maPayload.MA5,
		MA20:            maPayload.MA20,
		MA60:            maPayload.MA60,
		MA200:           maPayload.MA200,
		MAStatus:        maPayload.Status,
	}

	aiCfg := strategy.AIConfig{
		APIKey:  a.cfg.AI.APIKey,
		BaseURL: a.cfg.AI.BaseURL,
		Model:   a.cfg.AI.Model,
	}

	result, err := strategy.GenerateStrategy(r.Context(), aiCfg, summary)
	if err != nil {
		log.Printf("[ai-generate] LLM call failed for %s: %v", ticker, err)
		writeError(w, http.StatusInternalServerError, "AI 服务暂时不可用，请稍后重试。如持续失败，请检查网络连接。")
		return
	}

	// 自动拼接策略名称
	result.Recommendation.StrategyLabel = stockName + " - " + result.Recommendation.StrategyLabel

	writeJSON(w, http.StatusOK, result)
}

// stripSymbolSuffix converts "600519.SH" → "600519", "00700.HK" → "00700".
func stripSymbolSuffix(symbol string) string {
	if idx := strings.Index(symbol, "."); idx > 0 {
		return symbol[:idx]
	}
	return symbol
}

func (a *appServer) handleStrategyAIBacktest(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "Only POST method is allowed")
		return
	}

	userID := currentUserID(r)
	if strings.TrimSpace(userID) == "" {
		writeError(w, http.StatusUnauthorized, "请先登录")
		return
	}
	// 限流检查（该接口内部会循环调用 LLM 多次）
	if !a.aiRateLimiter.Allow(userID) {
		writeError(w, http.StatusTooManyRequests, "本小时 AI 调用次数已达上限（20 次/小时），请稍后再试")
		return
	}

	payload, err := decodeBodyAsMap(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "请求格式错误")
		return
	}

	symbol := strings.TrimSpace(asString(payload["symbol"]))
	implKey := strings.TrimSpace(asString(payload["implementation_key"]))
	paramsRaw := asMap(payload["params"])

	if symbol == "" || implKey == "" {
		writeError(w, http.StatusBadRequest, "缺少必要参数")
		return
	}

	// 回测 API 要求纯数字 ticker
	backtestTicker := stripSymbolSuffix(symbol)

	aiCfg := strategy.AIConfig{
		APIKey:  a.cfg.AI.APIKey,
		BaseURL: a.cfg.AI.BaseURL,
		Model:   a.cfg.AI.Model,
	}

	// 获取市场摘要（用于迭代 Prompt）
	maPayload, _ := a.liveService.GetMovingAverages(r.Context(), userID, symbol, "daily", 240)
	var summary strategy.MarketSummary
	if maPayload != nil {
		summary = strategy.MarketSummary{
			Ticker:          maPayload.Symbol,
			Price:           maPayload.PriceRef,
			ChangePct60D:    maPayload.ChangePct60D,
			Volatility20D:   maPayload.Volatility20D,
			VolumeMA5toMA20: maPayload.VolumeMA5toMA20,
			RSI14:           maPayload.RSI14,
			MAStatus:        maPayload.Status,
		}
	}

	currentParams := paramsRaw
	iterations := []strategy.IterationRound{}
	var bestPreview *strategy.BacktestPreview
	bestParams := currentParams

	const maxRounds = 3

	for round := 1; round <= maxRounds; round++ {
		btResult, btErr := strategy.CallQuantBacktest(r.Context(), a.cfg.QuantServiceURL, backtestTicker, implKey, currentParams)
		if btErr != nil {
			log.Printf("[ai-backtest] round %d failed for %s: %v", round, backtestTicker, btErr)
			// 如果第一轮就失败，返回错误让前端知道
			if round == 1 {
				writeJSON(w, http.StatusOK, map[string]any{
					"backtest_error": "回测引擎暂时不可用，请稍后重试。回测验证已跳过，推荐结果仍可参考。",
				})
				return
			}
			break
		}

		preview := strategy.ExtractBacktestPreview(btResult)

		iterRound := strategy.IterationRound{
			Round:           round,
			Params:          currentParams,
			BacktestPreview: preview,
		}

		if bestPreview == nil || preview.SharpeRatio > bestPreview.SharpeRatio {
			bestPreview = &preview
			bestParams = currentParams
		}

		if preview.SharpeRatio > 1.5 && preview.TotalReturn > 0 && preview.MaxDrawdown > -0.20 {
			iterRound.Adjustment = "表现优秀，无需继续优化"
			iterations = append(iterations, iterRound)
			break
		}

		if round == maxRounds {
			iterRound.Adjustment = "已达最大迭代轮数"
			iterations = append(iterations, iterRound)
			break
		}

		iterResult, iterErr := strategy.IterateStrategy(r.Context(), aiCfg, implKey, currentParams, summary, preview)
		if iterErr != nil || iterResult == nil || iterResult.Action != "adjust" {
			reason := "AI 建议保持当前参数"
			if iterResult != nil && iterResult.Reason != "" {
				reason = iterResult.Reason
			}
			iterRound.Adjustment = reason
			iterations = append(iterations, iterRound)
			break
		}

		iterRound.Adjustment = iterResult.Reason
		iterations = append(iterations, iterRound)
		currentParams = iterResult.Params
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"backtest_preview": bestPreview,
		"iterations":       iterations,
		"final_round":      len(iterations),
		"best_params":      bestParams,
	})
}

func (a *appServer) handleBacktestAIOptimize(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "Only POST method is allowed")
		return
	}

	userID := currentUserID(r)
	if strings.TrimSpace(userID) == "" {
		writeError(w, http.StatusUnauthorized, "请先登录")
		return
	}

	if !a.aiRateLimiter.Allow(userID) {
		writeError(w, http.StatusTooManyRequests, "本小时 AI 分析次数已达上限（20 次/小时），请稍后再试")
		return
	}

	payload, err := decodeBodyAsMap(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "请求格式错误")
		return
	}

	input := strategy.AnalyzeBacktestInput{
		StrategyName:      asString(payload["strategy_name"]),
		ImplementationKey: asString(payload["implementation_key"]),
		CurrentParams:     asMap(payload["current_params"]),
		Ticker:            asString(payload["ticker"]),
		StartDate:         asString(payload["start_date"]),
		EndDate:           asString(payload["end_date"]),
		Metrics:           asMap(payload["metrics"]),
	}

	if input.ImplementationKey == "" {
		writeError(w, http.StatusBadRequest, "缺少策略类型信息")
		return
	}

	aiCfg := strategy.AIConfig{
		APIKey:  a.cfg.AI.APIKey,
		BaseURL: a.cfg.AI.BaseURL,
		Model:   a.cfg.AI.Model,
	}

	analysis, err := strategy.AnalyzeBacktest(r.Context(), aiCfg, input)
	if err != nil {
		log.Printf("[ai-optimize] analysis failed for %s/%s: %v", input.Ticker, input.ImplementationKey, err)
		writeError(w, http.StatusInternalServerError, "AI 服务暂时不可用，请稍后重试。如持续失败，请检查网络连接。")
		return
	}

	writeJSON(w, http.StatusOK, analysis)
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
			StrategyID:          asString(payload["strategy_id"]),
			IsEnabled:           asBoolPtr(payload["is_enabled"]),
			CooldownSeconds:     asInt(payload["cooldown_seconds"]),
			EvalIntervalSeconds: asInt(payload["eval_interval_seconds"]),
			Thresholds:          asMap(payload["thresholds"]),
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

func (a *appServer) handleLiveWatchlistSnapshots(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "Only GET method is allowed")
		return
	}
	userID := currentUserID(r)
	snapshots, err := a.liveService.GetWatchlistSnapshots(r.Context(), userID)
	if err != nil {
		a.writeLiveError(w, err)
		return
	}
	writeLiveJSON(w, http.StatusOK, map[string]any{"items": snapshots})
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
	case "overlay-daily":
		if r.Method != http.MethodGet {
			writeError(w, http.StatusMethodNotAllowed, "Only GET method is allowed")
			return
		}
		lookbackDays, parseErr := parseLookbackDays(r.URL.Query().Get("lookback_days"), 60)
		if parseErr != nil {
			a.writeLiveError(w, fmt.Errorf("%w: lookback_days must be a positive integer", live.ErrInvalidArgument))
			return
		}
		benchmark := strings.TrimSpace(r.URL.Query().Get("benchmark"))
		dailyOverlay, err := a.liveService.GetDailyOverlay(r.Context(), symbol, lookbackDays, benchmark)
		if err != nil {
			a.writeLiveError(w, err)
			return
		}
		writeLiveJSON(w, http.StatusOK, dailyOverlay)
	case "fundamentals":
		if r.Method != http.MethodGet {
			writeError(w, http.StatusMethodNotAllowed, "Only GET method is allowed")
			return
		}
		a.handleFundamentalsWithCache(w, r, strings.ToUpper(symbol))
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
	case "daily-bars":
		if r.Method != http.MethodGet {
			writeError(w, http.StatusMethodNotAllowed, "Only GET method is allowed")
			return
		}
		lookbackDays, parseErr := parseLookbackDays(r.URL.Query().Get("lookback_days"), 130)
		if parseErr != nil {
			a.writeLiveError(w, fmt.Errorf("%w: lookback_days must be a positive integer", live.ErrInvalidArgument))
			return
		}
		bars, err := a.liveService.GetDailyBars(r.Context(), symbol, lookbackDays)
		if err != nil {
			a.writeLiveError(w, err)
			return
		}
		writeLiveJSON(w, http.StatusOK, map[string]any{
			"symbol":     strings.ToUpper(symbol),
			"bars":       bars,
			"count":      len(bars),
			"updated_at": time.Now().UTC().Format(time.RFC3339),
		})
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
	case "ai-analysis":
		if r.Method != http.MethodPost {
			writeError(w, http.StatusMethodNotAllowed, "仅支持 POST 请求")
			return
		}
		a.handleStockAIAnalysis(w, r, symbol)
	case "analysis-history":
		a.handleAnalysisHistorySubroutes(w, r, symbol)
	default:
		http.NotFound(w, r)
	}
}

// handleAnalysisHistorySubroutes 处理分析历史子路由
// GET    /api/live/symbols/{symbol}/analysis-history              → 列表
// GET    /api/live/symbols/{symbol}/analysis-history?id=xxx        → 单条详情（含完整 analysis 内容）
// DELETE /api/live/symbols/{symbol}/analysis-history?id=xxx        → 删除单条
func (a *appServer) handleAnalysisHistorySubroutes(w http.ResponseWriter, r *http.Request, symbol string) {
	userID := currentUserID(r)
	if userID == "" {
		writeError(w, http.StatusUnauthorized, "请先登录")
		return
	}
	symbol = strings.ToUpper(symbol)

	switch r.Method {
	case http.MethodGet:
		// 如果有 id 参数 → 返回单条详情（含完整 analysis）
		id := strings.TrimSpace(r.URL.Query().Get("id"))
		if id != "" {
			rec, err := a.analysisHistoryRepo.GetByID(r.Context(), userID, id)
			if err != nil {
				if errors.Is(err, gorm.ErrRecordNotFound) {
					writeError(w, http.StatusNotFound, "记录不存在")
					return
				}
				writeError(w, http.StatusInternalServerError, "查询失败")
				return
			}
			detail, err := rec.ToDetail()
			if err != nil {
				writeError(w, http.StatusInternalServerError, "解析数据失败")
				return
			}
			writeLiveJSON(w, http.StatusOK, detail)
			return
		}

		// 无 id → 返回列表
		limit := parseLimit(r.URL.Query().Get("limit"), 20)
		records, err := a.analysisHistoryRepo.ListBySymbol(r.Context(), userID, symbol, limit)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "查询分析历史失败")
			return
		}
		items := make([]analysis_history.HistoryListItem, len(records))
		for i, rec := range records {
			items[i] = rec.ToListItem()
		}
		writeLiveJSON(w, http.StatusOK, map[string]any{"items": items})
	case http.MethodDelete:
		// DELETE /api/live/symbols/{symbol}/analysis-history/{id}
		id := strings.TrimSpace(r.URL.Query().Get("id"))
		if id == "" {
			writeError(w, http.StatusBadRequest, "缺少 id 参数")
			return
		}
		if err := a.analysisHistoryRepo.Delete(r.Context(), userID, id); err != nil {
			writeError(w, http.StatusInternalServerError, "删除失败")
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"deleted": true, "id": id})
	default:
		writeError(w, http.StatusMethodNotAllowed, "Only GET and DELETE methods are allowed")
	}
}

// handleStockAIAnalysis 处理 AI 个股诊断请求
// POST /api/live/symbols/{symbol}/ai-analysis
func (a *appServer) handleStockAIAnalysis(w http.ResponseWriter, r *http.Request, symbol string) {
	userID := currentUserID(r)
	if userID == "" {
		writeError(w, http.StatusUnauthorized, "请先登录后使用 AI 分析功能")
		return
	}

	// 限流检查
	if !a.aiRateLimiter.Allow(userID) {
		writeError(w, http.StatusTooManyRequests, "AI 分析次数已达上限，每小时限 20 次，请稍后再试")
		return
	}

	// 解析请求体
	var input strategy.StockAnalysisInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "请求数据格式错误")
		return
	}

	// 查询投资画像（容错：查不到不阻断）
	var profile *portfolio.InvestmentProfile
	pf, err := a.portfolioService.GetInvestmentProfile(r.Context(), userID)
	if err == nil && pf != nil {
		profile = pf
	}

	// 构建 AI 配置
	cfg := strategy.AIConfig{
		APIKey:  a.cfg.AI.APIKey,
		BaseURL: a.cfg.AI.BaseURL,
		Model:   a.cfg.AI.Model,
	}

	// 调用 AI 分析
	result, err := strategy.AnalyzeStock(r.Context(), cfg, &input, profile)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	// 序列化响应用于返回 + 异步保存历史
	respBytes, _ := json.Marshal(result)
	writeLiveJSON(w, http.StatusOK, result)

	// 异步保存分析历史（不阻塞响应）
	if userID != "" && len(respBytes) > 0 {
		go func(body []byte, sym string) {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			if saveErr := a.analysisHistoryRepo.SaveFromAPIResponse(ctx, userID, sym, "", body); saveErr != nil {
				log.Printf("[analysis-history] save failed: %v", saveErr)
			}
		}(respBytes, symbol)
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

// containsExchange checks if a target exchange is in the list (nil means all).
func containsExchange(exchanges []string, target string) bool {
	if exchanges == nil || len(exchanges) == 0 {
		return true
	}
	for _, e := range exchanges {
		if e == target {
			return true
		}
	}
	return false
}

// normalizeWatchlistCode 将关注列表中的股票代码标准化为 DB 存储格式。
// 港股（isHK=true）补零到 5 位（如 "00700"），A 股补零到 6 位（如 "000001"）。
func normalizeWatchlistCode(sym string, isHK bool) string {
	code := sym
	if idx := strings.Index(code, "."); idx > 0 {
		code = code[:idx]
	}
	code = strings.TrimLeft(code, "0")
	if isHK {
		if len(code) < 5 {
			code = strings.Repeat("0", 5-len(code)) + code
		}
	} else {
		if len(code) < 6 {
			code = strings.Repeat("0", 6-len(code)) + code
		}
	}
	return code
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
	case errors.Is(err, live.ErrSymbolNotExist):
		statusCode = http.StatusBadRequest
		code = "SYMBOL_NOT_EXIST"
		message = "该股票代码不存在或暂无行情数据，请检查后重试"
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

// handleFundamentalsWithCache 带缓存的基础面数据代理
// 优先从 SQLite 缓存读取（TTL 2h），未命中时透传 Quant 并写入缓存
func (a *appServer) handleFundamentalsWithCache(w http.ResponseWriter, r *http.Request, symbol string) {
	ctx := r.Context()

	// 1. 尝试从本地缓存读取
	dataJSON, hit, err := a.fundCacheRepo.Get(ctx, symbol)
	if err != nil {
		log.Printf("[fundcache] cache read error for %s: %v", symbol, err)
		// 缓存读取出错不阻塞，继续走透传
	}
	if hit {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("X-Cache", "HIT")
		w.Write([]byte(dataJSON))
		return
	}

	// 2. 缓存未命中，透传到 Quant
	targetPath := "/api/fundamentals/" + symbol
	targetURL := a.cfg.QuantServiceURL + targetPath

	req, err := http.NewRequestWithContext(ctx, r.Method, targetURL, nil)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to create fundamentals request")
		return
	}
	copyForwardHeaders(r, req)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		writeError(w, http.StatusBadGateway, fmt.Sprintf("Quant service unavailable: %v", err))
		return
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		writeError(w, http.StatusBadGateway, "Failed to read fundamentals response")
		return
	}

	// 仅对成功响应写入缓存（2xx）
	if resp.StatusCode >= 200 && resp.StatusCode < 300 && len(respBody) > 0 {
		if cacheErr := a.fundCacheRepo.Upsert(ctx, symbol, string(respBody)); cacheErr != nil {
			log.Printf("[fundcache] cache write error for %s: %v", symbol, cacheErr)
			// 写入失败不影响响应返回
		}
	}

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("X-Cache", "MISS")
	w.WriteHeader(resp.StatusCode)
	w.Write(respBody)
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

func (a *appServer) handlePageView(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "Only POST method is allowed")
		return
	}
	var input analytics.PageViewInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeJSON(w, http.StatusOK, map[string]any{"ok": true})
		return
	}
	pagePath := strings.TrimSpace(input.PagePath)
	if pagePath == "" {
		writeJSON(w, http.StatusOK, map[string]any{"ok": true})
		return
	}
	userID := currentUserID(r)
	record := analytics.PageViewRecord{
		ID:          fmt.Sprintf("pv-%d", time.Now().UnixNano()),
		VisitorID:   strings.TrimSpace(input.VisitorID),
		UserID:      userID,
		PagePath:    pagePath,
		Referrer:    strings.TrimSpace(input.Referrer),
		UserAgent:   r.UserAgent(),
		ScreenWidth: input.ScreenWidth,
		CreatedAt:   time.Now().UTC(),
	}
	go func() {
		_ = a.analyticsRepo.Insert(context.Background(), record)
	}()
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (a *appServer) handleAdminAnalytics(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "Only GET method is allowed")
		return
	}
	ctx := r.Context()
	now := time.Now().UTC()
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	sevenDaysAgo := now.AddDate(0, 0, -7)
	thirtyDaysAgo := now.AddDate(0, 0, -30)

	todayPV, _ := a.analyticsRepo.CountPVSince(ctx, today)
	todayUV, _ := a.analyticsRepo.CountUVSince(ctx, today)
	weekPV, _ := a.analyticsRepo.CountPVSince(ctx, sevenDaysAgo)
	weekUV, _ := a.analyticsRepo.CountUVSince(ctx, sevenDaysAgo)
	monthPV, _ := a.analyticsRepo.CountPVSince(ctx, thirtyDaysAgo)
	monthUV, _ := a.analyticsRepo.CountUVSince(ctx, thirtyDaysAgo)

	dailyPV, _ := a.analyticsRepo.DailyPV(ctx, 30)
	dailyUV, _ := a.analyticsRepo.DailyUV(ctx, 30)
	topPages, _ := a.analyticsRepo.TopPages(ctx, thirtyDaysAgo, 10)
	devices, _ := a.analyticsRepo.DeviceBreakdown(ctx, thirtyDaysAgo)

	if dailyPV == nil {
		dailyPV = []analytics.DailyCount{}
	}
	if dailyUV == nil {
		dailyUV = []analytics.DailyCount{}
	}
	if topPages == nil {
		topPages = []analytics.PageRank{}
	}
	if devices == nil {
		devices = &analytics.DeviceStats{}
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"today_pv": todayPV, "today_uv": todayUV,
		"week_pv": weekPV, "week_uv": weekUV,
		"month_pv": monthPV, "month_uv": monthUV,
		"daily_pv": dailyPV, "daily_uv": dailyUV,
		"top_pages": topPages, "devices": devices,
	})
}

func (a *appServer) handleScreenerAIParse(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "Only POST method is allowed")
		return
	}
	userID := currentUserID(r)
	if strings.TrimSpace(userID) == "" {
		writeError(w, http.StatusUnauthorized, "请先登录后使用 AI 选股功能")
		return
	}
	// 限流检查
	if !a.aiRateLimiter.Allow(userID) {
		writeError(w, http.StatusTooManyRequests, "本小时 AI 调用次数已达上限（20 次/小时），请稍后再试")
		return
	}
	payload, err := decodeBodyAsMap(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "请求格式错误")
		return
	}
	query := asString(payload["query"])
	exchange := asString(payload["exchange"])
	if exchange == "" {
		exchange = "ASHARE"
	}

	aiCfg := screener.AIConfig{
		APIKey:  a.cfg.AI.APIKey,
		BaseURL: a.cfg.AI.BaseURL,
		Model:   a.cfg.AI.Model,
	}

	result, err := screener.ParseNaturalLanguage(r.Context(), aiCfg, query, exchange)
	if err != nil {
		a.writeScreenerError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
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

func (a *appServer) handleScreenerWatchlists(w http.ResponseWriter, r *http.Request) {
	userID := currentUserID(r)
	switch r.Method {
	case http.MethodGet:
		items, err := a.screenerService.List(r.Context(), userID)
		if err != nil {
			a.writeScreenerError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"items": items})
	case http.MethodPost:
		var input screener.CreateWatchlistInput
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
			writeError(w, http.StatusBadRequest, "自选表请求格式错误")
			return
		}
		detail, err := a.screenerService.Create(r.Context(), userID, input)
		if err != nil {
			a.writeScreenerError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"item": detail})
	default:
		writeError(w, http.StatusMethodNotAllowed, "Only GET and POST methods are allowed")
	}
}

func (a *appServer) handleScreenerWatchlistSubroutes(w http.ResponseWriter, r *http.Request) {
	suffix := strings.TrimPrefix(r.URL.Path, "/api/screener/watchlists/")
	suffix = strings.TrimSpace(strings.Trim(suffix, "/"))
	if suffix == "" {
		http.NotFound(w, r)
		return
	}

	userID := currentUserID(r)
	watchlistID := suffix

	switch r.Method {
	case http.MethodGet:
		detail, err := a.screenerService.GetByID(r.Context(), userID, watchlistID)
		if err != nil {
			a.writeScreenerError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"item": detail})
	case http.MethodDelete:
		if err := a.screenerService.Delete(r.Context(), userID, watchlistID); err != nil {
			a.writeScreenerError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"deleted": true, "id": watchlistID})
	default:
		writeError(w, http.StatusMethodNotAllowed, "Only GET and DELETE methods are allowed")
	}
}

func (a *appServer) writeScreenerError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, screener.ErrNotFound):
		writeError(w, http.StatusNotFound, "自选表不存在")
	case errors.Is(err, screener.ErrForbidden):
		writeError(w, http.StatusForbidden, "该操作需要登录后使用")
	case errors.Is(err, screener.ErrConflict):
		writeError(w, http.StatusConflict, "同名自选表已存在")
	case errors.Is(err, screener.ErrInvalid):
		writeError(w, http.StatusBadRequest, err.Error())
	case errors.Is(err, screener.ErrLimit):
		writeError(w, http.StatusConflict, err.Error())
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

// ── Portfolio handlers ──

func (a *appServer) handlePortfolioList(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "Only GET method is allowed")
		return
	}
	userID := currentUserID(r)
	items, err := a.portfolioService.ListByUser(r.Context(), userID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (a *appServer) handlePortfolioBySymbol(w http.ResponseWriter, r *http.Request) {
	symbol := strings.TrimPrefix(r.URL.Path, "/api/portfolio/")
	symbol = strings.TrimSpace(strings.ToUpper(strings.Trim(symbol, "/")))
	if symbol == "" {
		writeError(w, http.StatusBadRequest, "symbol is required")
		return
	}
	userID := currentUserID(r)

	switch r.Method {
	case http.MethodGet:
		item, err := a.portfolioService.GetBySymbol(r.Context(), userID, symbol)
		if err != nil {
			if errors.Is(err, portfolio.ErrNotFound) {
				writeJSON(w, http.StatusOK, map[string]any{"item": nil})
				return
			}
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"item": item})

	case http.MethodPut:
		var input portfolio.UpsertPortfolioInput
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
			writeError(w, http.StatusBadRequest, "invalid request body")
			return
		}
		item, err := a.portfolioService.Upsert(r.Context(), userID, symbol, input)
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"item": item})

	case http.MethodDelete:
		if err := a.portfolioService.Delete(r.Context(), userID, symbol); err != nil {
			if errors.Is(err, portfolio.ErrNotFound) {
				writeError(w, http.StatusNotFound, "portfolio not found")
				return
			}
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true})

	default:
		writeError(w, http.StatusMethodNotAllowed, "Only GET, PUT, DELETE methods are allowed")
	}
}

func (a *appServer) handleInvestmentProfile(w http.ResponseWriter, r *http.Request) {
	userID := currentUserID(r)

	switch r.Method {
	case http.MethodGet:
		profile, err := a.portfolioService.GetInvestmentProfile(r.Context(), userID)
		if err != nil {
			if errors.Is(err, portfolio.ErrNotFound) {
				writeJSON(w, http.StatusOK, map[string]any{"profile": nil})
				return
			}
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"profile": profile})

	case http.MethodPut:
		var input portfolio.UpsertInvestmentProfileInput
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
			writeError(w, http.StatusBadRequest, "invalid request body")
			return
		}
		profile, err := a.portfolioService.UpsertInvestmentProfile(r.Context(), userID, input)
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"profile": profile})

	default:
		writeError(w, http.StatusMethodNotAllowed, "Only GET, PUT methods are allowed")
	}
}

// ── Quadrant handlers ──

func (a *appServer) handleQuadrant(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "Only GET method is allowed")
		return
	}

	// Parse exchange parameter: "ASHARE" (default), "HKEX", or "ALL"
	exchangeParam := strings.TrimSpace(r.URL.Query().Get("exchange"))
	var exchanges []string
	switch strings.ToUpper(exchangeParam) {
	case "HKEX":
		exchanges = []string{"HKEX"}
	case "ALL":
		exchanges = nil // FindByExchange returns all when empty
	default:
		// Default: A-share only (SSE + SZSE)
		exchanges = []string{"SSE", "SZSE"}
	}

	// Parse watchlist_symbols from query
	watchlistSymbols := splitCSV(r.URL.Query().Get("watchlist_symbols"))
	watchlistCodes := make([]string, 0, len(watchlistSymbols))
	for _, sym := range watchlistSymbols {
		sym = strings.TrimSpace(sym)
		if sym == "" {
			continue
		}
		upper := strings.ToUpper(sym)
		isHK := strings.HasSuffix(upper, ".HK")

		// Skip HK symbols when querying A-share data and vice versa
		if isHK && !containsExchange(exchanges, "HKEX") {
			continue
		}
		if !isHK && containsExchange(exchanges, "HKEX") && !containsExchange(exchanges, "SSE") {
			continue
		}

		code := normalizeWatchlistCode(sym, isHK)
		if code != "" {
			watchlistCodes = append(watchlistCodes, code)
		}
	}

	resp, err := a.quadrantService.GetAllWithWatchlist(r.Context(), exchanges, watchlistCodes)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

func (a *appServer) handleQuadrantBulkSave(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "Only POST method is allowed")
		return
	}

	var input quadrant.BulkSaveInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	count, err := a.quadrantService.BulkSave(r.Context(), input)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	log.Printf("[quadrant] bulk-save: wrote %d scores", count)
	writeJSON(w, http.StatusOK, map[string]any{"saved": count})
}

func (a *appServer) handleQuadrantStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "Only GET method is allowed")
		return
	}

	status, err := a.quadrantService.GetStatus(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, status)
}

// ── Stock Search handler ──

func (a *appServer) handleSearchStocks(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "Only GET method is allowed")
		return
	}

	q := strings.TrimSpace(r.URL.Query().Get("q"))
	if len(q) < 2 {
		writeJSON(w, http.StatusOK, map[string]any{"results": []struct{}{}})
		return
	}
	limit := parseLimit(r.URL.Query().Get("limit"), 8)
	if limit > 20 {
		limit = 20 // hard cap to prevent abuse
	}
	if limit < 1 {
		limit = 8
	}

	results, err := a.quadrantService.Search(r.Context(), q, limit)
	if err != nil {
		log.Printf("[search] error: %v", err)
		writeError(w, http.StatusInternalServerError, "search failed")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"results": results})
}

func (a *appServer) handleFeedback(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "Only POST method is allowed")
		return
	}

	userID := currentUserID(r)
	if strings.TrimSpace(userID) == "" {
		writeError(w, http.StatusUnauthorized, "请先登录后再提交反馈")
		return
	}

	user, _ := currentUser(r)

	payload, err := decodeBodyAsMap(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "请求格式错误")
		return
	}

	item, err := a.feedbackRepo.Create(r.Context(), feedback.CreateInput{
		UserID:    userID,
		UserEmail: user.Email,
		Category:  asString(payload["category"]),
		Content:   asString(payload["content"]),
		Contact:   asString(payload["contact"]),
	})
	if err != nil {
		if errors.Is(err, feedback.ErrInvalid) {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		writeError(w, http.StatusInternalServerError, "提交反馈失败，请稍后重试")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"item": item})
}

func (a *appServer) handleAdminFeedback(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "Only GET method is allowed")
		return
	}

	limit := parseLimit(r.URL.Query().Get("limit"), 50)
	offset := parseOffset(r.URL.Query().Get("offset"), 0)

	items, total, err := a.feedbackRepo.List(r.Context(), limit, offset)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "加载反馈列表失败")
		return
	}

	stats, _ := a.feedbackRepo.GetStats(r.Context())

	writeJSON(w, http.StatusOK, map[string]any{
		"items": items,
		"total": total,
		"stats": stats,
	})
}

func (a *appServer) handleAdminFeedbackSubroutes(w http.ResponseWriter, r *http.Request) {
	suffix := strings.TrimPrefix(r.URL.Path, "/api/admin/feedback/")
	suffix = strings.TrimSpace(strings.Trim(suffix, "/"))
	if suffix == "" {
		http.NotFound(w, r)
		return
	}

	if r.Method != http.MethodPatch {
		writeError(w, http.StatusMethodNotAllowed, "Only PATCH method is allowed")
		return
	}

	payload, err := decodeBodyAsMap(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "请求格式错误")
		return
	}

	status := asString(payload["status"])
	if err := a.feedbackRepo.UpdateStatus(r.Context(), suffix, status); err != nil {
		if errors.Is(err, feedback.ErrNotFound) {
			writeError(w, http.StatusNotFound, "反馈记录不存在")
			return
		}
		if errors.Is(err, feedback.ErrInvalid) {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		writeError(w, http.StatusInternalServerError, "更新状态失败")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "id": suffix, "status": status})
}

func (a *appServer) handleAdminQuadrantLogs(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "Only GET method is allowed")
		return
	}
	logs, err := a.quadrantService.ListComputeLogs(r.Context(), 30)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if logs == nil {
		logs = []quadrant.ComputeLogRecord{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": logs})
}

// handleAdminQuadrantOverview returns quadrant statistics grouped by exchange.
// GET /api/admin/quadrant-overview (super admin only)
func (a *appServer) handleAdminQuadrantOverview(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "Only GET method is allowed")
		return
	}
	overview, err := a.quadrantService.GetAdminOverview(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if overview == nil {
		overview = &quadrant.QuadrantOverviewResponse{}
	}
	writeJSON(w, http.StatusOK, overview)
}

func main() {
	cfg := config.Load()
	storeInstance, err := store.New(cfg.DB)
	if err != nil {
		log.Fatalf("Failed to initialize store: %v", err)
	}

	// 初始化 AI 调用日志写入器（异步批量写入）
	store.InitAILogger(storeInstance.DB)
	strategy.SetAILogWriter(store.WriteAICallBatch)

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

	portfolioRepo := portfolio.NewRepository(storeInstance.DB)
	portfolioService := portfolio.NewService(portfolioRepo)

	quadrantRepo := quadrant.NewRepository(storeInstance.DB)
	quadrantService := quadrant.NewService(quadrantRepo)
	quadrantWorker := quadrant.NewWorker(quadrantService, quadrant.WorkerConfig{
		QuantServiceURL: cfg.QuantServiceURL,
		BackendBaseURL:  cfg.BackendCallbackURL,
	}, nil)
	quadrantWorker.Start(context.Background())

	signalEvaluator := signal.NewEvaluator(signalService, liveService, strategyService, signal.EvaluatorConfig{
		QuantServiceURL: cfg.QuantServiceURL,
	})
	signalEvaluator.Start(context.Background())

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

	screenerRepo := screener.NewRepository(storeInstance.DB)
	screenerService := screener.NewService(screenerRepo)

	analyticsRepo := analytics.NewRepository(storeInstance.DB)
	feedbackRepo := feedback.NewRepository(storeInstance.DB)
	fundCacheRepo := fundcache.NewRepository(storeInstance.DB)
	analysisHistoryRepo := analysis_history.NewRepository(storeInstance.DB)

	server := &appServer{
		cfg:              cfg,
		authService:      authService,
		strategyService:  strategyService,
		liveService:      liveService,
		signalService:    signalService,
		portfolioService: portfolioService,
		quadrantService:  quadrantService,
		adminService:     adminService,
		backtestService:  backtestService,
		screenerService:  screenerService,
		analyticsRepo:    analyticsRepo,
		feedbackRepo:     feedbackRepo,
		aiRateLimiter:    strategy.NewAIRateLimiter(20),
		fundCacheRepo:       fundCacheRepo,
		analysisHistoryRepo: analysisHistoryRepo,
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
	mux.HandleFunc("/api/backtest/ai-optimize", server.withRequiredAuth(server.handleBacktestAIOptimize))
	mux.HandleFunc("/api/backtest/runs", server.withRequiredAuth(server.handleBacktestRuns))
	mux.HandleFunc("/api/backtest/runs/", server.withRequiredAuth(server.handleBacktestRunSubroutes))
	mux.HandleFunc("/api/strategies", server.withOptionalAuth(server.handleStrategies))
	mux.HandleFunc("/api/strategies/active", server.withOptionalAuth(server.handleActiveStrategies))
	mux.HandleFunc("/api/strategies/ai-generate", server.withRequiredAuth(server.handleStrategyAIGenerate))
	mux.HandleFunc("/api/strategies/ai-generate/backtest", server.withRequiredAuth(server.handleStrategyAIBacktest))
	mux.HandleFunc("/api/strategies/", server.withOptionalAuth(server.handleStrategySubroutes))

	mux.HandleFunc("/api/webhook", server.withRequiredAuth(server.handleWebhookConfig))
	mux.HandleFunc("/api/webhook/test", server.withRequiredAuth(server.handleWebhookTest))
	mux.HandleFunc("/api/signal-configs", server.withRequiredAuth(server.handleSignalConfigs))
	mux.HandleFunc("/api/signal-configs/", server.withRequiredAuth(server.handleSignalConfigSubroutes))
	mux.HandleFunc("/api/signal-events", server.withRequiredAuth(server.handleSignalEvents))
	mux.HandleFunc("/api/webhook-deliveries", server.withRequiredAuth(server.handleWebhookDeliveries))
	mux.HandleFunc("/api/portfolio", server.withRequiredAuth(server.handlePortfolioList))
	mux.HandleFunc("/api/portfolio/", server.withRequiredAuth(server.handlePortfolioBySymbol))
	mux.HandleFunc("/api/investment-profile", server.withRequiredAuth(server.handleInvestmentProfile))
	mux.HandleFunc("/api/webhook-deliveries/latest", server.withRequiredAuth(server.handleWebhookDeliveriesLatest))

	mux.HandleFunc("/api/live/watchlist", server.withRequiredAuth(server.handleLiveWatchlist))
	mux.HandleFunc("/api/live/watchlist/snapshots", server.withRequiredAuth(server.handleLiveWatchlistSnapshots))
	mux.HandleFunc("/api/live/watchlist/", server.withRequiredAuth(server.handleLiveWatchlistSubroutes))
	mux.HandleFunc("/api/live/market/overview", server.handleLiveMarketOverview)
	mux.HandleFunc("/api/live/symbols/", server.withOptionalAuth(server.handleLiveSymbolsSubroutes))

	mux.HandleFunc("/api/admin/login", server.handleAdminLogin)
	mux.HandleFunc("/api/admin/stats", server.withSuperAdminAuth(server.handleAdminStats))
	mux.HandleFunc("/api/admin/analytics", server.withSuperAdminAuth(server.handleAdminAnalytics))
	mux.HandleFunc("/api/admin/system-health", server.withSuperAdminAuth(server.handleAdminSystemHealth))
	mux.HandleFunc("/api/admin/system-health/logs", server.withSuperAdminAuth(server.handleAdminSystemHealthLogs))
	mux.HandleFunc("/api/admin/system-health/purge", server.withSuperAdminAuth(server.handleAdminSystemHealthPurge))
	mux.HandleFunc("/api/admin/user-funnel", server.withSuperAdminAuth(server.handleAdminUserFunnel))

	mux.HandleFunc("/api/analytics/pageview", server.withOptionalAuth(server.handlePageView))

	mux.HandleFunc("/api/quadrant", server.handleQuadrant)
	mux.HandleFunc("/api/quadrant/bulk-save", server.handleQuadrantBulkSave)
	mux.HandleFunc("/api/quadrant/status", server.handleQuadrantStatus)

	mux.HandleFunc("/api/search", server.withOptionalAuth(server.handleSearchStocks))

	mux.HandleFunc("/api/admin/quadrant-logs", server.withSuperAdminAuth(server.handleAdminQuadrantLogs))
	mux.HandleFunc("/api/admin/quadrant-overview", server.withSuperAdminAuth(server.handleAdminQuadrantOverview))

	mux.HandleFunc("/api/feedback", server.withRequiredAuth(server.handleFeedback))
	mux.HandleFunc("/api/admin/feedback", server.withSuperAdminAuth(server.handleAdminFeedback))
	mux.HandleFunc("/api/admin/feedback/", server.withSuperAdminAuth(server.handleAdminFeedbackSubroutes))

	mux.HandleFunc("/api/screener/scan", server.withOptionalAuth(server.handleScreenerScan))
	mux.HandleFunc("/api/screener/ai-parse", server.withRequiredAuth(server.handleScreenerAIParse))
	mux.HandleFunc("/api/screener/watchlists", server.withRequiredAuth(server.handleScreenerWatchlists))
	mux.HandleFunc("/api/screener/watchlists/", server.withRequiredAuth(server.handleScreenerWatchlistSubroutes))

	handler := corsMiddleware(server.loggingMiddleware(mux))
	log.Printf("🚀 Wolong Pro Backend is running on port %s (db=%s)", cfg.Port, cfg.DB.Type)
	if err := http.ListenAndServe(fmt.Sprintf(":%s", cfg.Port), handler); err != nil {
		log.Fatalf("Server failed to start: %v", err)
	}
}
