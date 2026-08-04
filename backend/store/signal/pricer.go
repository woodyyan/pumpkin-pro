package signal

import (
	"context"
	"fmt"
	"log"
	"math"
	"strings"
	"time"

	"github.com/woodyyan/pumpkin-pro/backend/store/live"
)

// ──────────────────────────────────────────────────────────────
// 价格提醒评估器（Pricer）
//
// 职责：评估 price_alert 类订阅（涨破/跌破/单日涨跌幅）。
// 与指标/策略信号的差异：不需要 120 根日线，直接复用 live 实时快照
// （10 秒缓存）做阈值比对，避免每订阅一次 backend→quant HTTP 扇出。
//
// 口径边界：pricer 只做阈值比对，所有「指标计算」仍在 quant。
// 事件 bar_state=realtime：价格触碰是事实，无「收盘反转」语义。
// ──────────────────────────────────────────────────────────────

const defaultPricerInterval = time.Minute

// Pricer 价格提醒评估器。
type Pricer struct {
	signalService  *Service
	liveService    *live.Service
	contextBuilder *PositionContextBuilder
	interval       time.Duration
}

// PricerConfig 价格提醒配置。
type PricerConfig struct {
	Interval time.Duration
	// PositionReader 为 nil 时门控降级为透传（fail-open），与评估器一致。
	PositionReader PositionReader
}

func NewPricer(signalService *Service, liveService *live.Service, cfg PricerConfig) *Pricer {
	interval := cfg.Interval
	if interval <= 0 {
		interval = defaultPricerInterval
	}
	return &Pricer{
		signalService:  signalService,
		liveService:    liveService,
		contextBuilder: NewPositionContextBuilder(cfg.PositionReader),
		interval:       interval,
	}
}

// Start 启动价格提醒评估循环。
func (p *Pricer) Start(ctx context.Context) {
	ticker := time.NewTicker(p.interval)
	go func() {
		defer ticker.Stop()
		p.RunOnce(ctx)
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				p.RunOnce(ctx)
			}
		}
	}()
	log.Printf("[signal-pricer] started, interval=%s", p.interval)
}

// RunOnce 执行一轮价格提醒评估。
func (p *Pricer) RunOnce(ctx context.Context) {
	subscriptions, err := p.signalService.repo.ListEnabledSubscriptions(ctx)
	if err != nil {
		log.Printf("[signal-pricer] list enabled subscriptions error: %v", err)
		return
	}

	now := time.Now().UTC()
	targets := make([]SignalSubscriptionRecord, 0, len(subscriptions))
	symbols := make([]string, 0, len(subscriptions))
	seenSymbols := map[string]bool{}
	for _, sub := range subscriptions {
		tpl, ok := GetTemplate(sub.TemplateKey)
		if !ok || !tpl.IsActive || tpl.Category != TemplateCategoryPriceAlert {
			continue
		}
		if sub.ScopeType != ScopeTypeSymbol || strings.TrimSpace(sub.Symbol) == "" {
			continue
		}
		// 非交易时段快照不变，跳过以节省外部请求。
		if !live.IsTradingHoursAt(sub.Symbol, now) {
			continue
		}
		targets = append(targets, sub)
		if !seenSymbols[sub.Symbol] {
			seenSymbols[sub.Symbol] = true
			symbols = append(symbols, sub.Symbol)
		}
	}
	if len(targets) == 0 || len(symbols) == 0 {
		return
	}

	snapshots, err := p.liveService.GetDetailedSnapshots(ctx, symbols)
	if err != nil {
		log.Printf("[signal-pricer] fetch snapshots error: %v", err)
		return
	}
	snapshotBySymbol := make(map[string]live.DetailedSymbolSnapshot, len(snapshots))
	for _, snapshot := range snapshots {
		snapshotBySymbol[strings.ToUpper(strings.TrimSpace(snapshot.Symbol))] = snapshot
	}

	for _, sub := range targets {
		snapshot, ok := snapshotBySymbol[strings.ToUpper(strings.TrimSpace(sub.Symbol))]
		if !ok || snapshot.LastPrice <= 0 {
			continue
		}
		p.evaluateOne(ctx, sub, snapshot, now)
	}
}

// priceAlertOutcome 价格提醒的评估结果。
type priceAlertOutcome struct {
	triggered bool
	side      string
	message   string
}

func (p *Pricer) evaluateOne(ctx context.Context, sub SignalSubscriptionRecord, snapshot live.DetailedSymbolSnapshot, now time.Time) {
	params := sub.effectiveParams()
	outcome := evaluatePriceAlert(sub.TemplateKey, params, snapshot)
	if !outcome.triggered {
		return
	}

	// Cooldown check：按订阅 + bar_state=realtime 维度。
	cooldown := sub.CooldownSeconds
	if cooldown <= 0 {
		cooldown = defaultCooldownSeconds
	}
	lastEventTime, err := p.signalService.repo.GetLastSignalEventTimeBySubscription(ctx, sub.UserID, sub.ID, BarStateRealtime)
	if err != nil {
		log.Printf("[signal-pricer] get last event time error user=%s symbol=%s: %v", sub.UserID, sub.Symbol, err)
		return
	}
	if lastEventTime != nil && now.Sub(*lastEventTime) < time.Duration(cooldown)*time.Second {
		return
	}

	// ── 持仓感知门控（与指标/策略信号同一条链路）──
	positionCtx := p.contextBuilder.Build(ctx, BuildInput{
		UserID:      sub.UserID,
		Symbol:      sub.Symbol,
		LatestPrice: snapshot.LastPrice,
		IsTradable:  snapshot.LastPrice > 0,
		Config:      sub.riskConfigRecord(),
		Now:         now,
	})
	gateDecision := EvaluateGate(outcome.side, positionCtx)
	finalSide := gateDecision.FinalSide

	tpl, _ := GetTemplate(sub.TemplateKey)
	reason := map[string]any{
		"kind":          "price_alert",
		"message":       outcome.message,
		"template_key":  sub.TemplateKey,
		"template_name": tpl.Name,
		"last_price":    snapshot.LastPrice,
		"prev_close":    snapshot.PrevClosePrice,
		"change_rate":   snapshot.ChangeRate,
		"bar_state":     BarStateRealtime,
		"params":        params,
		"disclaimer":    SignalComplianceDisclaimer,
	}
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

	_, emitErr := p.signalService.EmitSignal(ctx, EmitSignalInput{
		UserID:         sub.UserID,
		Symbol:         sub.Symbol,
		Side:           finalSide,
		SignalScore:    1.0,
		Reason:         reason,
		EventTime:      now,
		IsTest:         false,
		SubscriptionID: sub.ID,
		TemplateKey:    sub.TemplateKey,
		BarState:       BarStateRealtime,
		Gate: &EmitGateInfo{
			RawSide:            outcome.side,
			FinalSide:          finalSide,
			Decision:           gateDecision.Decision,
			SuppressedReason:   gateDecision.SuppressedReason,
			SemanticLabel:      gateDecision.SemanticLabel,
			MatchedRules:       gateDecision.MatchedRules,
			PositionSnapshot:   ToSnapshotMap(positionCtx.Snapshot),
			ReferencePrice:     snapshot.LastPrice,
			PositionDataStatus: positionCtx.Snapshot.DataStatus,
			SkipDelivery:       gateDecision.SkipDelivery(),
		},
	})
	if emitErr != nil {
		log.Printf("[signal-pricer] emit signal error user=%s symbol=%s: %v", sub.UserID, sub.Symbol, emitErr)
		return
	}
	if gateDecision.SkipDelivery() {
		log.Printf("[signal-pricer] ⛔ suppressed %s alert user=%s symbol=%s rule=%s", outcome.side, sub.UserID, sub.Symbol, gateDecision.SuppressedReason)
		return
	}
	log.Printf("[signal-pricer] ✅ price alert user=%s symbol=%s template=%s: %s", sub.UserID, sub.Symbol, sub.TemplateKey, outcome.message)
}

// evaluatePriceAlert 纯函数：按模板类型比对实时快照，返回是否触发与方向。
func evaluatePriceAlert(templateKey string, params map[string]any, snapshot live.DetailedSymbolSnapshot) priceAlertOutcome {
	switch templateKey {
	case "price_above":
		target := paramFloat(params, "price")
		if target > 0 && snapshot.LastPrice >= target {
			return priceAlertOutcome{
				triggered: true,
				side:      SideBuy,
				message:   fmt.Sprintf("最新价 %.2f 已涨破提醒价 %.2f", snapshot.LastPrice, target),
			}
		}
	case "price_below":
		target := paramFloat(params, "price")
		if target > 0 && snapshot.LastPrice <= target {
			return priceAlertOutcome{
				triggered: true,
				side:      SideSell,
				message:   fmt.Sprintf("最新价 %.2f 已跌破提醒价 %.2f", snapshot.LastPrice, target),
			}
		}
	case "pct_change":
		threshold := paramFloat(params, "pct")
		if threshold > 0 && math.Abs(snapshot.ChangeRate) >= threshold {
			side := SideBuy
			if snapshot.ChangeRate < 0 {
				side = SideSell
			}
			return priceAlertOutcome{
				triggered: true,
				side:      side,
				message:   fmt.Sprintf("当日涨跌幅 %+.2f%% 已达到提醒幅度 %.1f%%", snapshot.ChangeRate, threshold),
			}
		}
	}
	return priceAlertOutcome{}
}

func paramFloat(params map[string]any, key string) float64 {
	raw, ok := params[key]
	if !ok || raw == nil {
		return 0
	}
	value, err := coerceParamNumber(raw)
	if err != nil {
		return 0
	}
	return value
}
