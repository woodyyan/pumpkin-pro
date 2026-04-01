package signal

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/woodyyan/pumpkin-pro/backend/store/live"
	"github.com/woodyyan/pumpkin-pro/backend/store/strategy"
)

const (
	defaultEvaluatorInterval = 15 * time.Minute
	evaluatorDailyBarsCount  = 120
	evaluatorHTTPTimeout     = 15 * time.Second
)

// Evaluator periodically scans enabled signal configs, runs strategy
// evaluation via the quant service, and emits real signals when BUY/SELL
// is detected (respecting cooldown).
type Evaluator struct {
	signalService   *Service
	liveService     *live.Service
	strategyService *strategy.Service
	quantURL        string
	interval        time.Duration
}

// EvaluatorConfig holds configuration for the signal evaluator.
type EvaluatorConfig struct {
	QuantServiceURL string
	Interval        time.Duration
}

// NewEvaluator creates a new evaluator.
func NewEvaluator(signalService *Service, liveService *live.Service, strategyService *strategy.Service, cfg EvaluatorConfig) *Evaluator {
	interval := cfg.Interval
	if interval <= 0 {
		interval = defaultEvaluatorInterval
	}
	return &Evaluator{
		signalService:   signalService,
		liveService:     liveService,
		strategyService: strategyService,
		quantURL:        strings.TrimRight(cfg.QuantServiceURL, "/"),
		interval:        interval,
	}
}

// Start launches the background evaluation loop.
func (e *Evaluator) Start(ctx context.Context) {
	ticker := time.NewTicker(e.interval)
	go func() {
		defer ticker.Stop()
		// Run once immediately on start.
		e.runCycle(ctx)
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				e.runCycle(ctx)
			}
		}
	}()
	log.Printf("[signal-evaluator] started, interval=%s quant=%s", e.interval, e.quantURL)
}

func (e *Evaluator) runCycle(ctx context.Context) {
	configs, err := e.signalService.repo.ListAllEnabledConfigs(ctx)
	if err != nil {
		log.Printf("[signal-evaluator] list enabled configs error: %v", err)
		return
	}
	if len(configs) == 0 {
		return
	}

	now := time.Now().UTC()
	for _, cfg := range configs {
		e.evaluateOne(ctx, cfg, now)
	}
}

func (e *Evaluator) evaluateOne(ctx context.Context, cfg SymbolSignalConfigRecord, now time.Time) {
	userID := cfg.UserID
	symbol := cfg.Symbol
	strategyID := cfg.StrategyID

	if strings.TrimSpace(strategyID) == "" {
		return
	}

	// Eval interval check: skip if not enough time has passed since last evaluation.
	evalInterval := cfg.EvalIntervalSeconds
	if evalInterval <= 0 {
		evalInterval = defaultEvalIntervalSeconds
	}
	if cfg.LastEvaluatedAt != nil && now.Sub(*cfg.LastEvaluatedAt) < time.Duration(evalInterval)*time.Second {
		return // not yet time to evaluate
	}

	// Resolve strategy definition from database (supports both preset and user-created strategies).
	strat, err := e.strategyService.GetByID(ctx, userID, strategyID)
	if err != nil {
		log.Printf("[signal-evaluator] resolve strategy error user=%s strategy=%s: %v", userID, strategyID, err)
		return
	}
	if strat.Status != "active" {
		return // strategy is not active — skip
	}

	// Mark this config as evaluated (regardless of outcome: BUY/SELL/HOLD).
	_ = e.signalService.repo.UpdateLastEvaluatedAt(ctx, cfg.ID, now)

	// Cooldown check: skip if last signal for this user+symbol is within cooldown window.
	cooldown := cfg.CooldownSeconds
	if cooldown <= 0 {
		cooldown = defaultCooldownSeconds
	}
	lastEventTime, err := e.signalService.repo.GetLastSignalEventTime(ctx, userID, symbol)
	if err != nil {
		log.Printf("[signal-evaluator] get last event time error user=%s symbol=%s: %v", userID, symbol, err)
		return
	}
	if lastEventTime != nil && now.Sub(*lastEventTime) < time.Duration(cooldown)*time.Second {
		return // still in cooldown
	}

	// Fetch daily bars from live service.
	bars, err := e.liveService.GetDailyBars(ctx, symbol, evaluatorDailyBarsCount)
	if err != nil || len(bars) < 10 {
		// Not enough data or data source issue — skip silently.
		return
	}

	// Call quant service to evaluate with implementation_key + params directly.
	result, err := e.callQuantEvaluate(ctx, quantEvaluateInput{
		StrategyID:        strategyID,
		ImplementationKey: strat.ImplementationKey,
		StrategyName:      strat.Name,
		Params:            strat.DefaultParams,
		Symbol:            symbol,
		Bars:              bars,
	})
	if err != nil {
		log.Printf("[signal-evaluator] quant evaluate error user=%s symbol=%s strategy=%s: %v", userID, symbol, strategyID, err)
		return
	}

	side := strings.ToUpper(strings.TrimSpace(result.Side))
	if side != "BUY" && side != "SELL" {
		return // HOLD — no action needed
	}

	// Emit real signal.
	reason := result.Reason
	if reason == nil {
		reason = map[string]any{}
	}
	// Enrich reason with strategy info.
	reason["strategy_name"] = strat.Name
	reason["strategy_params"] = strat.DefaultParams
	reason["latest_close"] = result.LatestClose

	_, emitErr := e.signalService.EmitSignal(ctx, EmitSignalInput{
		UserID:      userID,
		Symbol:      symbol,
		StrategyID:  strategyID,
		Side:        side,
		SignalScore: result.Score,
		Reason:      reason,
		EventTime:   now,
		IsTest:      false,
	})
	if emitErr != nil {
		log.Printf("[signal-evaluator] emit signal error user=%s symbol=%s side=%s: %v", userID, symbol, side, emitErr)
		return
	}

	log.Printf("[signal-evaluator] ✅ emitted %s signal user=%s symbol=%s strategy=%s reason=%s",
		side, userID, symbol, strat.Name, truncate(reason["message"], 80))
}

type quantEvaluateInput struct {
	StrategyID        string
	ImplementationKey string
	StrategyName      string
	Params            map[string]any
	Symbol            string
	Bars              []live.DailyBar
}

type quantEvaluateRequest struct {
	StrategyID        string           `json:"strategy_id"`
	ImplementationKey string           `json:"implementation_key"`
	StrategyName      string           `json:"strategy_name"`
	Params            map[string]any   `json:"params"`
	Symbol            string           `json:"symbol"`
	Bars              []quantBarInput  `json:"bars"`
}

type quantBarInput struct {
	Date   string  `json:"date"`
	Open   float64 `json:"open"`
	High   float64 `json:"high"`
	Low    float64 `json:"low"`
	Close  float64 `json:"close"`
	Volume float64 `json:"volume"`
}

type quantEvaluateResponse struct {
	Side        string          `json:"side"`
	Score       float64         `json:"score"`
	Reason      map[string]any  `json:"reason"`
	Strategy    quantStrategyInfo `json:"strategy"`
	LatestClose float64         `json:"latest_close"`
}

type quantStrategyInfo struct {
	ID                string         `json:"id"`
	Name              string         `json:"name"`
	ImplementationKey string         `json:"implementation_key"`
	Params            map[string]any `json:"params"`
}

func (e *Evaluator) callQuantEvaluate(ctx context.Context, input quantEvaluateInput) (*quantEvaluateResponse, error) {
	inputBars := make([]quantBarInput, 0, len(input.Bars))
	for _, b := range input.Bars {
		inputBars = append(inputBars, quantBarInput{
			Date:   b.Date,
			Open:   b.Open,
			High:   b.High,
			Low:    b.Low,
			Close:  b.Close,
			Volume: b.Volume,
		})
	}

	payload := quantEvaluateRequest{
		StrategyID:        input.StrategyID,
		ImplementationKey: input.ImplementationKey,
		StrategyName:      input.StrategyName,
		Params:            input.Params,
		Symbol:            input.Symbol,
		Bars:              inputBars,
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("marshal evaluate request: %w", err)
	}

	url := e.quantURL + "/api/signal/evaluate"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create evaluate request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: evaluatorHTTPTimeout}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("quant evaluate request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read evaluate response: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("quant evaluate returned %d: %s", resp.StatusCode, truncateBytes(respBody, 200))
	}

	var result quantEvaluateResponse
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("decode evaluate response: %w", err)
	}

	return &result, nil
}

func truncate(v any, maxLen int) string {
	s := fmt.Sprint(v)
	if len(s) > maxLen {
		return s[:maxLen]
	}
	return s
}

func truncateBytes(b []byte, maxLen int) string {
	if len(b) > maxLen {
		return string(b[:maxLen])
	}
	return string(b)
}
