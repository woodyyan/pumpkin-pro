package aipicker

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/woodyyan/pumpkin-pro/backend/store/factorlab"
	"github.com/woodyyan/pumpkin-pro/backend/store/strategy"
)

var (
	ErrInvalid            = errors.New("invalid ai picker request")
	ErrUnavailable        = errors.New("ai picker unavailable")
	ErrDailyResultMissing = errors.New("daily ai picker result missing")
)

type AIConfig struct {
	APIKey  string
	BaseURL string
	Model   string
}

func (c AIConfig) Enabled() bool {
	return strings.TrimSpace(c.APIKey) != "" && strings.TrimSpace(c.BaseURL) != "" && strings.TrimSpace(c.Model) != ""
}

type Service struct {
	repo      *Repository
	factorLab *factorlab.Service
}

func NewService(repo *Repository, factorLab *factorlab.Service) *Service {
	return &Service{repo: repo, factorLab: factorLab}
}

func (s *Service) Meta(ctx context.Context, market string) map[string]any {
	result := map[string]any{"available": false, "market": strings.ToUpper(strings.TrimSpace(market))}
	if strings.ToUpper(strings.TrimSpace(market)) != MarketAShare {
		result["reason"] = "港股 AI 选股即将上线"
		return result
	}
	if s.factorLab == nil {
		result["reason"] = "因子实验室服务未初始化"
		return result
	}
	meta, err := s.factorLab.Meta(ctx)
	if err != nil {
		result["reason"] = err.Error()
		return result
	}
	result["available"] = meta.HasSnapshot && !meta.Stale
	result["snapshot_date"] = meta.SnapshotDate
	result["stale"] = meta.Stale
	if !meta.HasSnapshot {
		result["reason"] = "因子快照尚未生成"
	} else if meta.Stale {
		result["reason"] = "因子快照已过期，请等待每日计算完成"
	}
	return result
}

func (s *Service) GetLatestDaily(ctx context.Context, market string) (*PickerResponse, error) {
	if s.repo == nil {
		return nil, ErrUnavailable
	}
	if strings.ToUpper(strings.TrimSpace(market)) != MarketAShare {
		return nil, fmt.Errorf("%w: 港股 AI 选股即将上线", ErrUnavailable)
	}
	record, err := s.repo.GetLatestDailyResult(ctx, market)
	if err != nil {
		if errors.Is(err, http.ErrMissingFile) || errors.Is(err, io.EOF) {
			return nil, ErrDailyResultMissing
		}
		if errors.Is(err, errors.New("record not found")) {
			return nil, ErrDailyResultMissing
		}
		if strings.Contains(strings.ToLower(err.Error()), "record not found") {
			return nil, ErrDailyResultMissing
		}
		return nil, err
	}
	return decodeStoredPayload(record.PayloadJSON)
}

func (s *Service) Generate(ctx context.Context, cfg AIConfig, userID string, req PickerRequest) (*PickerResponse, error) {
	market := strings.ToUpper(strings.TrimSpace(req.Market))
	if market == "" {
		market = MarketAShare
	}
	if market != MarketAShare {
		return nil, fmt.Errorf("%w: 港股 AI 选股即将上线", ErrUnavailable)
	}
	if !cfg.Enabled() {
		return nil, fmt.Errorf("%w: AI 功能未启用", ErrUnavailable)
	}
	if s.factorLab == nil {
		return nil, fmt.Errorf("%w: 因子实验室服务未初始化", ErrUnavailable)
	}
	meta, err := s.factorLab.Meta(ctx)
	if err != nil {
		return nil, err
	}
	if !meta.HasSnapshot || meta.Stale {
		return nil, fmt.Errorf("%w: 因子数据未就绪", ErrUnavailable)
	}
	candidatesResp, err := s.factorLab.Screen(ctx, factorlab.FactorScreenerRequest{
		SnapshotDate: meta.SnapshotDate,
		SortBy:       "composite_score",
		SortOrder:    "desc",
		Page:         1,
		PageSize:     24,
	})
	if err != nil {
		return nil, err
	}
	if len(candidatesResp.Items) == 0 {
		return nil, fmt.Errorf("%w: 候选池为空", ErrUnavailable)
	}
	candidates := buildCandidates(candidatesResp.Items)
	trigger := TriggerManual
	userPrompt := buildUserPrompt(meta.SnapshotDate, trigger, candidates)
	result, err := callLLM(ctx, cfg, userID, userPrompt)
	if err != nil {
		return nil, err
	}
	warnings := validateResponse(result, candidates, meta.SnapshotDate, trigger, cfg.Model)
	if result.Meta == nil {
		result.Meta = map[string]any{}
	}
	result.Meta["candidate_pool_size"] = len(candidates)
	result.Meta["selection_basis"] = SelectionBasisFactorLab
	result.Meta["validation"] = warnings
	result.Meta["cached"] = false
	result.Meta["model"] = cfg.Model
	result.Meta["generated_at"] = time.Now().UTC().Format(time.RFC3339)
	return result, nil
}

func (s *Service) GenerateAndStoreDaily(ctx context.Context, cfg AIConfig) (*PickerResponse, error) {
	result, err := s.Generate(ctx, cfg, "system", PickerRequest{Market: MarketAShare})
	if err != nil {
		return nil, err
	}
	encoded, err := json.Marshal(result)
	if err != nil {
		return nil, err
	}
	tradeDate := time.Now().In(time.FixedZone("CST", 8*60*60)).Format("2006-01-02")
	if s.repo != nil {
		_ = s.repo.SaveDailyResult(ctx, DailyResult{
			Market:         MarketAShare,
			TradeDate:      tradeDate,
			Trigger:        TriggerDailyAuto,
			SnapshotDate:   result.Analysis.SnapshotDate,
			SelectionBasis: result.Analysis.SelectionBasis,
			Model:          cfg.Model,
			PayloadJSON:    string(encoded),
		})
	}
	return result, nil
}

type aiChatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type aiChatRequest struct {
	Model          string          `json:"model"`
	Messages       []aiChatMessage `json:"messages"`
	Temperature    float64         `json:"temperature"`
	MaxTokens      int             `json:"max_tokens,omitempty"`
	ResponseFormat *aiResponseFmt  `json:"response_format,omitempty"`
}

type aiResponseFmt struct {
	Type string `json:"type"`
}

type aiChatResponse struct {
	Choices []struct {
		Message      aiChatMessage `json:"message"`
		FinishReason string        `json:"finish_reason"`
	} `json:"choices"`
	Usage strategy.AIUsage `json:"usage"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

func callLLM(ctx context.Context, cfg AIConfig, userID, userPrompt string) (*PickerResponse, error) {
	body := aiChatRequest{
		Model: cfg.Model,
		Messages: []aiChatMessage{
			{Role: "system", Content: aShareSystemPrompt},
			{Role: "user", Content: userPrompt},
		},
		Temperature:    0.2,
		MaxTokens:      4096,
		ResponseFormat: &aiResponseFmt{Type: "json_object"},
	}
	encoded, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	endpoint := strings.TrimRight(cfg.BaseURL, "/") + "/chat/completions"

	const maxRetries = 1
	var lastErr error
	for attempt := 0; attempt <= maxRetries; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(2 * time.Second):
			}
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(encoded))
		if err != nil {
			return nil, err
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+cfg.APIKey)
		client := &http.Client{Timeout: 90 * time.Second}
		logEntry := strategy.AILogEntry{UserID: userID, FeatureKey: "ai_picker", FeatureName: "AI 选股", Model: cfg.Model, ExtraMeta: map[string]any{"attempt": attempt + 1}}
		start := time.Now()
		resp, err := client.Do(req)
		logEntry.ResponseMS = int(time.Since(start).Milliseconds())
		if err != nil {
			logEntry.Status = "error"
			logEntry.ErrorMessage = err.Error()
			strategy.LogAICall(logEntry)
			lastErr = err
			continue
		}
		respBody, readErr := io.ReadAll(resp.Body)
		resp.Body.Close()
		if readErr != nil {
			logEntry.Status = "error"
			logEntry.ErrorMessage = readErr.Error()
			strategy.LogAICall(logEntry)
			return nil, readErr
		}
		if resp.StatusCode != http.StatusOK {
			logEntry.Status = "error"
			logEntry.ErrorMessage = string(respBody)
			strategy.LogAICall(logEntry)
			lastErr = fmt.Errorf("AI 服务返回错误 (HTTP %d)", resp.StatusCode)
			continue
		}
		var parsed aiChatResponse
		if err := json.Unmarshal(respBody, &parsed); err != nil {
			logEntry.Status = "error"
			logEntry.ErrorMessage = err.Error()
			strategy.LogAICall(logEntry)
			return nil, err
		}
		logEntry.ApplyUsage(parsed.Usage)
		if parsed.Error != nil {
			logEntry.Status = "error"
			logEntry.ErrorMessage = parsed.Error.Message
			strategy.LogAICall(logEntry)
			return nil, fmt.Errorf(parsed.Error.Message)
		}
		if len(parsed.Choices) == 0 {
			logEntry.Status = "error"
			logEntry.ErrorMessage = "empty choices"
			strategy.LogAICall(logEntry)
			return nil, fmt.Errorf("AI 未返回有效结果")
		}
		choice := parsed.Choices[0]
		raw := strings.TrimSpace(choice.Message.Content)
		// 输出被 token 上限截断时，JSON 必然不完整，直接重试或明确报错
		if choice.FinishReason == "length" {
			logEntry.Status = "error"
			logEntry.ErrorMessage = "output truncated (finish_reason=length)"
			strategy.LogAICall(logEntry)
			lastErr = fmt.Errorf("AI 输出超长被截断，请稍后重试")
			continue
		}
		if raw == "" {
			logEntry.Status = "error"
			logEntry.ErrorMessage = "empty content"
			strategy.LogAICall(logEntry)
			lastErr = fmt.Errorf("AI 返回内容为空，请稍后重试")
			continue
		}
		content := extractJSONObject(stripCodeFence(raw))
		if content == "" {
			logEntry.Status = "error"
			logEntry.ErrorMessage = "no json object in content"
			strategy.LogAICall(logEntry)
			lastErr = fmt.Errorf("AI 返回内容中未找到有效 JSON，请稍后重试")
			continue
		}
		var result PickerResponse
		if err := json.Unmarshal([]byte(content), &result); err != nil {
			logEntry.Status = "error"
			logEntry.ErrorMessage = "JSON parse error: " + err.Error()
			strategy.LogAICall(logEntry)
			lastErr = fmt.Errorf("AI 返回 JSON 格式错误: %w", err)
			continue
		}
		logEntry.Status = "success"
		strategy.LogAICall(logEntry)
		return &result, nil
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("AI 选股调用失败")
	}
	return nil, lastErr
}

func buildCandidates(items []factorlab.FactorScreenerItem) []Candidate {
	out := make([]Candidate, 0, len(items))
	for _, item := range items {
		out = append(out, Candidate{
			Code:               strings.TrimSpace(item.Code),
			Symbol:             strings.TrimSpace(item.Symbol),
			Name:               strings.TrimSpace(item.Name),
			Industry:           strings.TrimSpace(item.Industry),
			ClosePrice:         item.ClosePrice,
			CompositeScore:     item.CompositeScore,
			ValueScore:         item.ValueScore,
			DividendYieldScore: item.DividendYieldScore,
			GrowthScore:        item.GrowthScore,
			QualityScore:       item.QualityScore,
			MomentumScore:      item.MomentumScore,
			SizeScore:          item.SizeScore,
			LowVolatilityScore: item.LowVolatilityScore,
		})
	}
	return out
}

func buildUserPrompt(snapshotDate, trigger string, candidates []Candidate) string {
	var sb strings.Builder
	sb.WriteString("请基于以下候选池为 A 股生成 4 只股票的推荐组合。\n")
	fmt.Fprintf(&sb, "快照日期：%s\n触发方式：%s\n\n候选池如下（只能从中选择）:\n", snapshotDate, trigger)
	for idx, c := range candidates {
		fmt.Fprintf(&sb, "%d. code=%s, symbol=%s, name=%s, industry=%s, price=%.2f, composite=%s, value=%s, dividend=%s, growth=%s, quality=%s, momentum=%s, size=%s, low_vol=%s\n",
			idx+1, c.Code, c.Symbol, c.Name, c.Industry, c.ClosePrice,
			fmtScore(c.CompositeScore), fmtScore(c.ValueScore), fmtScore(c.DividendYieldScore), fmtScore(c.GrowthScore), fmtScore(c.QualityScore), fmtScore(c.MomentumScore), fmtScore(c.SizeScore), fmtScore(c.LowVolatilityScore))
	}
	sb.WriteString("\n请严格按 system prompt 要求输出 JSON。")
	return sb.String()
}

func fmtScore(v *float64) string {
	if v == nil {
		return "null"
	}
	return fmt.Sprintf("%.1f", *v)
}

func stripCodeFence(s string) string {
	s = strings.TrimSpace(s)
	if strings.HasPrefix(s, "```") {
		if idx := strings.Index(s, "\n"); idx > 0 {
			s = s[idx+1:]
		}
		if strings.HasSuffix(s, "```") {
			s = s[:len(s)-3]
		}
	}
	return strings.TrimSpace(s)
}

// extractJSONObject 从可能夹带说明文字的内容中，提取第一个完整、括号配对的 JSON 对象。
// 用于兜底处理模型在 JSON 前后输出多余文本的情况；若内容被截断（无法配对）则返回空串。
func extractJSONObject(s string) string {
	start := strings.IndexByte(s, '{')
	if start < 0 {
		return ""
	}
	depth := 0
	inString := false
	escaped := false
	for i := start; i < len(s); i++ {
		c := s[i]
		if inString {
			switch {
			case escaped:
				escaped = false
			case c == '\\':
				escaped = true
			case c == '"':
				inString = false
			}
			continue
		}
		switch c {
		case '"':
			inString = true
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return s[start : i+1]
			}
		}
	}
	// 花括号未配对 → JSON 不完整（被截断）
	return ""
}

func decodeStoredPayload(payload string) (*PickerResponse, error) {
	var result PickerResponse
	if err := json.Unmarshal([]byte(payload), &result); err != nil {
		return nil, err
	}
	return &result, nil
}

func validateResponse(result *PickerResponse, candidates []Candidate, snapshotDate, trigger, model string) []string {
	warnings := make([]string, 0)
	if result.Analysis == nil {
		result.Analysis = &AnalysisPayload{}
		warnings = append(warnings, "analysis_missing_initialized")
	}
	analysis := result.Analysis
	analysis.FormatVersion = "1.0"
	analysis.Market = MarketAShare
	analysis.SelectionBasis = SelectionBasisFactorLab
	analysis.SnapshotDate = snapshotDate
	analysis.Trigger = trigger
	analysis.Disclaimer = "本结果由 卧龙AI 基于历史数据生成，仅供学习参考，不构成投资建议"
	analysis.DataTimestamp = time.Now().Local().Format("2006/01/02 15:04:05")
	candidateMap := map[string]Candidate{}
	for _, c := range candidates {
		candidateMap[strings.ToUpper(strings.TrimSpace(c.Code))] = c
		candidateMap[strings.ToUpper(strings.TrimSpace(c.Symbol))] = c
	}
	picked := make([]PickItem, 0, 4)
	used := map[string]bool{}
	for _, item := range analysis.Picks {
		key := strings.ToUpper(strings.TrimSpace(item.Code))
		cand, ok := candidateMap[key]
		if !ok {
			cand, ok = candidateMap[strings.ToUpper(strings.TrimSpace(item.Symbol))]
		}
		if !ok || used[cand.Code] {
			warnings = append(warnings, "hallucination_filtered")
			continue
		}
		used[cand.Code] = true
		item.Code = cand.Code
		if item.Symbol == "" {
			item.Symbol = cand.Symbol
		}
		item.Name = firstNonEmpty(item.Name, cand.Name)
		item.Industry = firstNonEmpty(item.Industry, cand.Industry)
		if item.CurrentPrice <= 0 {
			item.CurrentPrice = cand.ClosePrice
		}
		item.Currency = "CNY"
		if item.ConvictionScore < 0 || item.ConvictionScore > 100 {
			item.ConvictionScore = 60
			warnings = append(warnings, "conviction_score_sanitized")
		}
		if item.Conviction == "" {
			switch {
			case item.ConvictionScore >= 70:
				item.Conviction = ConvictionHigh
			case item.ConvictionScore >= 40:
				item.Conviction = ConvictionMedium
			default:
				item.Conviction = ConvictionLow
			}
		}
		if item.PositionPct < 10 {
			item.PositionPct = 10
			warnings = append(warnings, "position_pct_clamped")
		}
		if item.PositionPct > 35 {
			item.PositionPct = 35
			warnings = append(warnings, "position_pct_clamped")
		}
		if item.EntryZone.Low <= 0 || item.EntryZone.High <= 0 || item.EntryZone.Low > item.EntryZone.High {
			item.EntryZone = PriceRange{Low: round2(item.CurrentPrice * 0.97), High: round2(item.CurrentPrice * 1.01), Currency: "CNY"}
			warnings = append(warnings, "entry_zone_sanitized")
		}
		if item.StopLoss.Price <= 0 || item.StopLoss.Price >= item.CurrentPrice {
			item.StopLoss = PricePoint{Price: round2(item.CurrentPrice * 0.94), Pct: -6}
			warnings = append(warnings, "stop_loss_sanitized")
		}
		if item.TakeProfit.Price <= item.CurrentPrice {
			item.TakeProfit = PricePoint{Price: round2(item.CurrentPrice * 1.12), Pct: 12}
			warnings = append(warnings, "take_profit_sanitized")
		}
		if item.TimeHorizon == "" {
			item.TimeHorizon = "中期(1-3月)"
		}
		if strings.TrimSpace(item.Reason) == "" {
			item.Reason = "综合因子得分靠前，兼顾行业分散与风险收益比。"
			warnings = append(warnings, "reason_padded")
		}
		if strings.TrimSpace(item.RiskNote) == "" {
			item.RiskNote = "需关注市场波动与业绩兑现风险。"
		}
		item.CompositeScore = cand.CompositeScore
		if len(item.FactorHighlights) == 0 {
			item.FactorHighlights = defaultHighlights(cand)
		}
		picked = append(picked, item)
		if len(picked) == 4 {
			break
		}
	}
	if len(picked) < 4 {
		for _, cand := range candidates {
			if used[cand.Code] {
				continue
			}
			picked = append(picked, fallbackPick(len(picked)+1, cand))
			if len(picked) == 4 {
				warnings = append(warnings, "fallback_candidates_padded")
				break
			}
		}
	}
	sort.SliceStable(picked, func(i, j int) bool { return picked[i].Rank < picked[j].Rank })
	for i := range picked {
		picked[i].Rank = i + 1
	}
	analysis.Picks = picked
	total := 0
	for _, item := range analysis.Picks {
		total += item.PositionPct
	}
	if total > 80 {
		scale := 80.0 / float64(total)
		total = 0
		for i := range analysis.Picks {
			analysis.Picks[i].PositionPct = maxInt(int(float64(analysis.Picks[i].PositionPct)*scale+0.5), 10)
			total += analysis.Picks[i].PositionPct
		}
		warnings = append(warnings, "weights_normalized")
	}
	for total > 80 && len(analysis.Picks) > 0 {
		for i := range analysis.Picks {
			if analysis.Picks[i].PositionPct > 10 && total > 80 {
				analysis.Picks[i].PositionPct--
				total--
			}
		}
	}
	analysis.PortfolioAllocation.TotalPositionPct = total
	analysis.PortfolioAllocation.CashReservePct = 100 - total
	if analysis.PortfolioAllocation.ExpectedStyle == "" {
		analysis.PortfolioAllocation.ExpectedStyle = "均衡偏价值"
	}
	if analysis.PortfolioAllocation.DiversificationNote == "" {
		analysis.PortfolioAllocation.DiversificationNote = diversificationNote(analysis.Picks)
	}
	if len(analysis.KeyRisks) == 0 {
		analysis.KeyRisks = []string{"市场整体波动可能导致组合短期回撤", "因子有效性会随市场风格切换而波动"}
		warnings = append(warnings, "key_risks_padded")
	}
	if strings.TrimSpace(analysis.MarketView) == "" {
		analysis.MarketView = "当前组合基于 A 股最新因子快照生成，偏向选择综合得分更高且行业相对分散的标的。"
	}
	if strings.TrimSpace(analysis.StrategySummary) == "" {
		analysis.StrategySummary = "本次优先选择综合得分领先、质量与价值更均衡的 A 股，并通过保留现金仓位控制整体波动。"
	}
	if result.Meta == nil {
		result.Meta = map[string]any{}
	}
	result.Meta["model"] = model
	result.Meta["generated_at"] = time.Now().UTC().Format(time.RFC3339)
	return warnings
}

func defaultHighlights(c Candidate) []FactorHighlight {
	items := []FactorHighlight{
		{Key: "quality", Label: "质量", Score: c.QualityScore},
		{Key: "value", Label: "价值", Score: c.ValueScore},
		{Key: "growth", Label: "成长", Score: c.GrowthScore},
		{Key: "momentum", Label: "动量", Score: c.MomentumScore},
	}
	out := make([]FactorHighlight, 0, 2)
	for _, item := range items {
		if item.Score != nil {
			out = append(out, item)
		}
		if len(out) == 2 {
			break
		}
	}
	return out
}

func fallbackPick(rank int, c Candidate) PickItem {
	position := []int{25, 20, 20, 15}
	pct := 15
	if rank-1 >= 0 && rank-1 < len(position) {
		pct = position[rank-1]
	}
	return PickItem{
		Rank:             rank,
		Code:             c.Code,
		Symbol:           c.Symbol,
		Name:             c.Name,
		Industry:         c.Industry,
		CurrentPrice:     c.ClosePrice,
		Currency:         "CNY",
		PositionPct:      pct,
		Conviction:       ConvictionMedium,
		ConvictionScore:  65,
		Reason:           "该股位于因子候选池前列，具备一定配置价值，作为回退补位纳入组合。",
		FactorHighlights: defaultHighlights(c),
		CompositeScore:   c.CompositeScore,
		EntryZone:        PriceRange{Low: round2(c.ClosePrice * 0.97), High: round2(c.ClosePrice * 1.01), Currency: "CNY"},
		StopLoss:         PricePoint{Price: round2(c.ClosePrice * 0.94), Pct: -6},
		TakeProfit:       PricePoint{Price: round2(c.ClosePrice * 1.12), Pct: 12},
		TimeHorizon:      "中期(1-3月)",
		RiskNote:         "需关注行业景气度变化和市场波动。",
	}
}

func diversificationNote(items []PickItem) string {
	set := map[string]bool{}
	for _, item := range items {
		if strings.TrimSpace(item.Industry) != "" {
			set[item.Industry] = true
		}
	}
	return fmt.Sprintf("组合覆盖 %d 个行业，保留部分现金以控制波动。", len(set))
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func round2(v float64) float64 {
	return float64(int(v*100+0.5)) / 100
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
