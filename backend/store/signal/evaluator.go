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

// evalOptions 控制单次评估的门控与事件语义。
// 盘中试算与收盘确认共享同一条评估链路，仅选项不同。
type evalOptions struct {
	barState               string // 写入事件的 bar_state
	enforceTradingHours    bool   // 盘中评估：仅交易时段内执行
	enforceEvalInterval    bool   // 盘中评估：按订阅频率门控
	skipCloseOnlyTemplates bool   // 盘中评估：跳过 close_only 模板（放量类盘中失真）
	requireCurrentTradeDate bool  // 收盘确认：最后一根 bar 必须是当日（隐含市场日历门控）
}

var (
	intradayEvalOptions = evalOptions{
		barState:               BarStateIntradayProvisional,
		enforceTradingHours:    true,
		enforceEvalInterval:    true,
		skipCloseOnlyTemplates: true,
	}
	closeConfirmEvalOptions = evalOptions{
		barState:               BarStateCloseConfirmed,
		requireCurrentTradeDate: true,
	}
)

// Evaluator periodically scans enabled signal subscriptions, runs strategy
// evaluation via the quant service, and emits real signals when BUY/SELL
// is detected (respecting cooldown).
//
// 信号采用「全量生成 + 推送门控」：策略命中的信号一律落库，
// 由 RiskGate 决定是否推送，以保留策略胜率的可复盘性。
//
// 双状态语义：盘中 tick 产出 intraday_provisional（基于形成中 K 线，收盘可能反转）；
// 收盘确认由 CloseConfirmer 以 closeConfirmEvalOptions 驱动同一评估逻辑。
type Evaluator struct {
	signalService   *Service
	liveService     *live.Service
	strategyService *strategy.Service
	contextBuilder  *PositionContextBuilder
	quantURL        string
	interval        time.Duration
}

// EvaluatorConfig holds configuration for the signal evaluator.
type EvaluatorConfig struct {
	QuantServiceURL string
	Interval        time.Duration
	// PositionReader 为 nil 时门控降级为透传（fail-open），行为与改造前一致。
	PositionReader PositionReader
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
		contextBuilder:  NewPositionContextBuilder(cfg.PositionReader),
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
		e.RunIntraday(ctx)
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				e.RunIntraday(ctx)
			}
		}
	}()
	log.Printf("[signal-evaluator] started, interval=%s quant=%s", e.interval, e.quantURL)
}

// RunIntraday 盘中试算：扫描全部启用订阅，按 intradayEvalOptions 评估。
func (e *Evaluator) RunIntraday(ctx context.Context) {
	e.runPass(ctx, nil, intradayEvalOptions)
}

// RunCloseConfirm 收盘确认：仅评估指定市场（ashare/hk）的订阅。
// 通过 requireCurrentTradeDate 隐含市场日历门控：休市时最后一根 bar 不是当日，自动跳过。
func (e *Evaluator) RunCloseConfirm(ctx context.Context, market string) {
	e.runPass(ctx, &market, closeConfirmEvalOptions)
}

// runPass 评估主循环；market 为 nil 表示不按市场过滤。
func (e *Evaluator) runPass(ctx context.Context, market *string, opts evalOptions) {
	subscriptions, err := e.signalService.repo.ListEnabledSubscriptions(ctx)
	if err != nil {
		log.Printf("[signal-evaluator] list enabled subscriptions error: %v", err)
		return
	}
	if len(subscriptions) == 0 {
		return
	}

	now := time.Now().UTC()
	for _, sub := range subscriptions {
		if market != nil && !marketMatches(sub.Symbol, *market) {
			continue
		}
		e.evaluateSubscription(ctx, sub, now, opts)
	}
}

// marketMatches 按 symbol 判断市场归属；market 取值 ashare / hk。
func marketMatches(symbol, market string) bool {
	switch strings.ToLower(strings.TrimSpace(market)) {
	case "ashare":
		return live.IsAShare(symbol)
	case "hk":
		return !live.IsAShare(symbol)
	default:
		return true
	}
}

func (e *Evaluator) evaluateSubscription(ctx context.Context, sub SignalSubscriptionRecord, now time.Time, opts evalOptions) {
	tpl, ok := GetTemplate(sub.TemplateKey)
	if !ok || !tpl.IsActive {
		return
	}
	// 价格提醒由 pricer 实时快照链路评估，不走日线评估。
	if tpl.Category == TemplateCategoryPriceAlert {
		return
	}
	// P2 批量信号预留：当前仅评估单股订阅。
	if sub.ScopeType != ScopeTypeSymbol || strings.TrimSpace(sub.Symbol) == "" {
		return
	}
	if opts.skipCloseOnlyTemplates && tpl.IntradayMode == IntradayModeCloseOnly {
		return
	}
	if opts.enforceTradingHours && !live.IsTradingHoursAt(sub.Symbol, now) {
		return
	}

	// Eval interval check: skip if not enough time has passed since last evaluation.
	if opts.enforceEvalInterval {
		evalInterval := sub.EvalIntervalSeconds
		if evalInterval <= 0 {
			evalInterval = defaultSubscriptionEvalIntervalSeconds
		}
		if sub.LastEvaluatedAt != nil && now.Sub(*sub.LastEvaluatedAt) < time.Duration(evalInterval)*time.Second {
			return // not yet time to evaluate
		}
		// Mark this subscription as evaluated (regardless of outcome: BUY/SELL/HOLD).
		_ = e.signalService.repo.UpdateSubscriptionLastEvaluatedAt(ctx, sub.ID, now)
	}

	// 解析评估实现：策略模板由绑定策略解析，指标模板由模板注册表解析。
	implementationKey := tpl.ImplementationKey
	params := sub.effectiveParams()
	displayName := tpl.Name
	strategyID := strings.TrimSpace(sub.StrategyID)
	if tpl.NeedsStrategy {
		if strategyID == "" {
			return
		}
		strat, err := e.strategyService.GetByID(ctx, sub.UserID, strategyID)
		if err != nil {
			log.Printf("[signal-evaluator] resolve strategy error user=%s strategy=%s: %v", sub.UserID, strategyID, err)
			return
		}
		if strat.Status != "active" {
			return // strategy is not active — skip
		}
		implementationKey = strat.ImplementationKey
		params = strat.DefaultParams
		displayName = strat.Name
	}

	// Cooldown check: 按订阅 + bar_state 维度（盘中试算的冷却不阻塞收盘确认）。
	cooldown := sub.CooldownSeconds
	if cooldown <= 0 {
		cooldown = defaultCooldownSeconds
	}
	lastEventTime, err := e.signalService.repo.GetLastSignalEventTimeBySubscription(ctx, sub.UserID, sub.ID, opts.barState)
	if err != nil {
		log.Printf("[signal-evaluator] get last event time error user=%s symbol=%s: %v", sub.UserID, sub.Symbol, err)
		return
	}
	if lastEventTime != nil && now.Sub(*lastEventTime) < time.Duration(cooldown)*time.Second {
		return // still in cooldown
	}

	// Fetch daily bars from live service.
	// 盘中：腾讯日线接口最后一根为形成中 K 线（close=最新价），试算语义真实。
	bars, err := e.liveService.GetDailyBars(ctx, sub.Symbol, evaluatorDailyBarsCount)
	if err != nil || len(bars) < 10 {
		// Not enough data or data source issue — skip silently.
		return
	}

	// 收盘确认：最后一根 bar 必须是当日，否则视为休市/停牌，跳过。
	// 这也是市场日历门控的隐含实现（无需额外依赖日历服务）。
	if opts.requireCurrentTradeDate {
		lastBarDate := bars[len(bars)-1].Date
		if lastBarDate != live.TradeDateAt(now) {
			return
		}
	}

	// Call quant service to evaluate with implementation_key + params directly.
	result, err := e.callQuantEvaluate(ctx, quantEvaluateInput{
		StrategyID:        strategyID,
		ImplementationKey: implementationKey,
		StrategyName:      displayName,
		Params:            params,
		Symbol:            sub.Symbol,
		Bars:              bars,
	})
	if err != nil {
		log.Printf("[signal-evaluator] quant evaluate error user=%s symbol=%s template=%s: %v", sub.UserID, sub.Symbol, sub.TemplateKey, err)
		return
	}

	side := strings.ToUpper(strings.TrimSpace(result.Side))
	if side != SideBuy && side != SideSell {
		return // HOLD — no action needed
	}

	// ── 持仓感知门控（全量生成 + 推送门控）──
	// 装配持仓上下文（IO 集中在装配层），再由纯函数 EvaluateGate 判定三态决策。
	// 参考价复用已取到的最新收盘价，避免额外行情请求。
	referencePrice := result.LatestClose
	if referencePrice <= 0 && len(bars) > 0 {
		referencePrice = bars[len(bars)-1].Close
	}
	positionCtx := e.contextBuilder.Build(ctx, BuildInput{
		UserID:      sub.UserID,
		Symbol:      sub.Symbol,
		LatestPrice: referencePrice,
		// 能取到有效价格即视为可交易；停牌等场景下 referencePrice 为 0。
		IsTradable: referencePrice > 0,
		Config:     sub.riskConfigRecord(),
		Now:        now,
	})
	gateDecision := EvaluateGate(side, positionCtx)
	finalSide := gateDecision.FinalSide

	// Emit real signal.
	reason := result.Reason
	if reason == nil {
		reason = map[string]any{}
	}
	// Enrich reason with template/strategy info.
	reason["template_key"] = sub.TemplateKey
	reason["template_name"] = displayName
	reason["strategy_name"] = displayName
	reason["strategy_params"] = params
	reason["latest_close"] = result.LatestClose
	reason["bar_state"] = opts.barState
	if opts.barState == BarStateIntradayProvisional {
		reason["bar_state_note"] = "盘中试算基于当日未完成 K 线，收盘前信号可能反转。"
	}
	// 门控上下文写入 reason，供 webhook 文案与站内展示使用。
	reason["position_summary"] = BuildGateSummary(positionCtx)
	if gateDecision.SemanticLabel != "" {
		reason["semantic_label"] = gateDecision.SemanticLabel
	}
	if msg := gateDecision.Message(); msg != "" {
		reason["gate_message"] = msg
	}
	if notes := collectGateNotes(gateDecision.Notes); len(notes) > 0 {
		reason["gate_notes"] = notes
	}
	// 合规：免责声明随每条信号内联下发，而不是只放在页面角落。
	reason["disclaimer"] = SignalComplianceDisclaimer

	_, emitErr := e.signalService.EmitSignal(ctx, EmitSignalInput{
		UserID:         sub.UserID,
		Symbol:         sub.Symbol,
		StrategyID:     strategyID,
		Side:           finalSide,
		SignalScore:    result.Score,
		Reason:         reason,
		EventTime:      now,
		IsTest:         false,
		SubscriptionID: sub.ID,
		TemplateKey:    sub.TemplateKey,
		BarState:       opts.barState,
		Gate: &EmitGateInfo{
			RawSide:            side,
			FinalSide:          finalSide,
			Decision:           gateDecision.Decision,
			SuppressedReason:   gateDecision.SuppressedReason,
			SemanticLabel:      gateDecision.SemanticLabel,
			MatchedRules:       gateDecision.MatchedRules,
			PositionSnapshot:   ToSnapshotMap(positionCtx.Snapshot),
			ReferencePrice:     referencePrice,
			PositionDataStatus: positionCtx.Snapshot.DataStatus,
			SkipDelivery:       gateDecision.SkipDelivery(),
		},
	})
	if emitErr != nil {
		log.Printf("[signal-evaluator] emit signal error user=%s symbol=%s side=%s: %v", sub.UserID, sub.Symbol, finalSide, emitErr)
		return
	}

	if gateDecision.SkipDelivery() {
		log.Printf("[signal-evaluator] ⛔ suppressed %s signal user=%s symbol=%s rule=%s (已归档，未推送)",
			side, sub.UserID, sub.Symbol, gateDecision.SuppressedReason)
		return
	}
	log.Printf("[signal-evaluator] ✅ emitted %s signal user=%s symbol=%s template=%s bar_state=%s decision=%s reason=%s",
		finalSide, sub.UserID, sub.Symbol, displayName, opts.barState, gateDecision.Decision, truncate(reason["message"], 80))
}

// collectGateNotes 把非决策性提示规则码翻译成用户可读文案。
func collectGateNotes(codes []string) []string {
	if len(codes) == 0 {
		return nil
	}
	notes := make([]string, 0, len(codes))
	for _, code := range codes {
		if text := GateRuleMessage(code); text != "" {
			notes = append(notes, text)
		}
	}
	return notes
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
	StrategyID        string          `json:"strategy_id"`
	ImplementationKey string          `json:"implementation_key"`
	StrategyName      string          `json:"strategy_name"`
	Params            map[string]any  `json:"params"`
	Symbol            string          `json:"symbol"`
	Bars              []quantBarInput `json:"bars"`
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
	Side        string            `json:"side"`
	Score       float64           `json:"score"`
	Reason      map[string]any    `json:"reason"`
	Strategy    quantStrategyInfo `json:"strategy"`
	LatestClose float64           `json:"latest_close"`
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
		return string(b)
	}
	return string(b[:maxLen])
}
