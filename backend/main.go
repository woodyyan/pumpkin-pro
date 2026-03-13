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
	"strings"

	"github.com/woodyyan/pumpkin-pro/backend/config"
	"github.com/woodyyan/pumpkin-pro/backend/store"
	"github.com/woodyyan/pumpkin-pro/backend/store/strategy"
)

var supportedDataSources = []string{"online", "csv", "sample"}

type appServer struct {
	cfg             config.Config
	strategyService *strategy.Service
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

	runtimeStrategy, err := a.strategyService.BuildRuntimeStrategy(r.Context(), strategyID, strategyName, strategyParams)
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

	activeStrategies, err := a.strategyService.List(r.Context(), true)
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
		summaries, err := a.strategyService.ListSummaries(r.Context())
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
		created, err := a.strategyService.Create(r.Context(), payload)
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

	items, err := a.strategyService.List(r.Context(), true)
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

	item, err := a.strategyService.GetByID(r.Context(), strategyID)
	if err != nil {
		a.writeStrategyError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"item": item})
}

func (a *appServer) handleStrategyDetail(w http.ResponseWriter, r *http.Request, strategyID string) {
	switch r.Method {
	case http.MethodGet:
		item, err := a.strategyService.GetByID(r.Context(), strategyID)
		if err != nil {
			a.writeStrategyError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"item":               item,
			"implementation_keys": a.strategyService.ImplementationKeys(),
		})
	case http.MethodPut:
		payload, err := decodeStrategyPayload(r)
		if err != nil {
			writeError(w, http.StatusBadRequest, "策略请求格式错误")
			return
		}
		updated, err := a.strategyService.Update(r.Context(), strategyID, payload)
		if err != nil {
			a.writeStrategyError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"item": updated})
	default:
		writeError(w, http.StatusMethodNotAllowed, "Only GET and PUT methods are allowed")
	}
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

	strategyRepo := strategy.NewRepository(storeInstance.DB)
	strategyService := strategy.NewService(strategyRepo)
	if err := strategyService.SeedFromFileIfEmpty(context.Background(), cfg.StrategySeedPath); err != nil {
		log.Printf("Seed strategies skipped: %v", err)
	}

	server := &appServer{
		cfg:             cfg,
		strategyService: strategyService,
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/api/health", server.handleHealth)
	mux.HandleFunc("/api/backtest", server.handleBacktest)
	mux.HandleFunc("/api/backtest/options", server.handleBacktestOptions)
	mux.HandleFunc("/api/strategies", server.handleStrategies)
	mux.HandleFunc("/api/strategies/active", server.handleActiveStrategies)
	mux.HandleFunc("/api/strategies/", server.handleStrategySubroutes)

	handler := corsMiddleware(mux)
	log.Printf("🚀 Pumpkin Go Backend is running on port %s (db=%s)", cfg.Port, cfg.DB.Type)
	if err := http.ListenAndServe(fmt.Sprintf(":%s", cfg.Port), handler); err != nil {
		log.Fatalf("Server failed to start: %v", err)
	}
}
