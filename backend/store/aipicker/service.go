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
	"sync"
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
	repo          *Repository
	factorLab     *factorlab.Service
	technicalRepo *TechnicalSnapshotRepository
	technicalSvc  *TechnicalSnapshotService

	dailyMu sync.Mutex
}

const maxGenerateErrorLogs = 20

func beijingNow() time.Time {
	return time.Now().In(time.FixedZone("CST", 8*60*60))
}

func NewService(repo *Repository, factorLab *factorlab.Service, technicalRepo *TechnicalSnapshotRepository) *Service {
	return &Service{repo: repo, factorLab: factorLab, technicalRepo: technicalRepo, technicalSvc: NewTechnicalSnapshotService(technicalRepo)}
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
	result["available"] = meta.HasSnapshot
	result["snapshot_date"] = meta.SnapshotDate
	result["stale"] = meta.Stale
	if !meta.HasSnapshot {
		result["reason"] = "因子快照尚未生成"
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
		if isDailyResultMissing(err) {
			return nil, ErrDailyResultMissing
		}
		return nil, err
	}
	return decodeStoredPayload(record.PayloadJSON)
}

func (s *Service) GetLatestDailyOrGenerate(ctx context.Context, market string, cfg AIConfig) (*PickerResponse, error) {
	result, err := s.GetLatestDaily(ctx, market)
	if err == nil || !errors.Is(err, ErrDailyResultMissing) {
		return result, err
	}
	if strings.ToUpper(strings.TrimSpace(market)) != MarketAShare {
		return nil, fmt.Errorf("%w: 港股 AI 选股即将上线", ErrUnavailable)
	}
	s.dailyMu.Lock()
	defer s.dailyMu.Unlock()
	result, err = s.GetLatestDaily(ctx, market)
	if err == nil || !errors.Is(err, ErrDailyResultMissing) {
		return result, err
	}
	if !cfg.Enabled() {
		return nil, fmt.Errorf("%w: AI 功能未启用", ErrUnavailable)
	}
	return s.GenerateAndStoreDaily(ctx, cfg)
}

func isDailyResultMissing(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, http.ErrMissingFile) || errors.Is(err, io.EOF) {
		return true
	}
	if strings.Contains(strings.ToLower(err.Error()), "record not found") {
		return true
	}
	return false
}

// generateCore 执行核心选股逻辑，返回结果与 LLM 调用元信息。
// trigger 参数决定 prompt 中的触发方式标注（daily_auto / manual）。
func (s *Service) generateCore(ctx context.Context, cfg AIConfig, userID, trigger string) (*PickerResponse, *llmCallMeta, error) {
	if !cfg.Enabled() {
		return nil, nil, fmt.Errorf("%w: AI 功能未启用", ErrUnavailable)
	}
	if s.factorLab == nil {
		return nil, nil, fmt.Errorf("%w: 因子实验室服务未初始化", ErrUnavailable)
	}
	factorMeta, err := s.factorLab.Meta(ctx)
	if err != nil {
		return nil, nil, err
	}
	if !factorMeta.HasSnapshot {
		return nil, nil, fmt.Errorf("%w: 因子数据未就绪", ErrUnavailable)
	}
	candidates, marketSummary, techStats, err := s.buildCandidatePool(ctx, factorMeta.SnapshotDate)
	if err != nil {
		return nil, nil, err
	}
	if len(candidates) == 0 {
		return nil, nil, fmt.Errorf("%w: 候选池为空", ErrUnavailable)
	}
	userPrompt := buildUserPrompt(factorMeta.SnapshotDate, trigger, marketSummary, techStats, candidates)
	result, callMeta, err := callLLM(ctx, cfg, userID, userPrompt)
	if err != nil {
		return nil, callMeta, err
	}
	warnings := validateResponse(result, candidates, factorMeta.SnapshotDate, trigger, cfg.Model)
	if result.Meta == nil {
		result.Meta = map[string]any{}
	}
	result.Meta["candidate_pool_size"] = len(candidates)
	result.Meta["selection_basis"] = SelectionBasisFactorLab
	result.Meta["validation"] = warnings
	result.Meta["cached"] = false
	result.Meta["model"] = cfg.Model
	result.Meta["generated_at"] = time.Now().UTC().Format(time.RFC3339)
	if callMeta != nil {
		result.Meta["finish_reason"] = callMeta.FinishReason
		result.Meta["completion_tokens"] = callMeta.CompletionTokens
		result.Meta["prompt_chars"] = callMeta.PromptChars
		result.Meta["response_ms"] = callMeta.ResponseMS
	}
	return result, callMeta, nil
}

// Generate 是对外暴露的一次性生成接口（不持久化），trigger 固定为 manual。
func (s *Service) Generate(ctx context.Context, cfg AIConfig, userID string, req PickerRequest) (*PickerResponse, error) {
	market := strings.ToUpper(strings.TrimSpace(req.Market))
	if market == "" {
		market = MarketAShare
	}
	if market != MarketAShare {
		return nil, fmt.Errorf("%w: 港股 AI 选股即将上线", ErrUnavailable)
	}
	result, _, err := s.generateCore(ctx, cfg, userID, TriggerManual)
	return result, err
}

func (s *Service) GenerateAndStoreDaily(ctx context.Context, cfg AIConfig) (*PickerResponse, error) {
	return s.generateAndStore(ctx, cfg, TriggerDailyAuto, "system")
}

func (s *Service) GenerateAndStoreManual(ctx context.Context, cfg AIConfig, userID string) (*PickerResponse, error) {
	actor := strings.TrimSpace(userID)
	if actor == "" {
		actor = "admin"
	}
	return s.generateAndStore(ctx, cfg, TriggerManual, actor)
}

func (s *Service) generateAndStore(ctx context.Context, cfg AIConfig, trigger, userID string) (*PickerResponse, error) {
	result, callMeta, err := s.generateCore(ctx, cfg, userID, trigger)
	tradeDate := beijingNow().Format("2006-01-02")
	if err != nil {
		log := GenerateLogRecord{
			TradeDate: tradeDate,
			Trigger:   trigger,
			Status:    GenerateLogStatusFailed,
			Message:   err.Error(),
			UserID:    userID,
		}
		if callMeta != nil {
			log.FinishReason = callMeta.FinishReason
			log.PromptChars = callMeta.PromptChars
			log.CompletionTokens = callMeta.CompletionTokens
			log.TimeoutSeconds = 240
			log.ResponseMS = callMeta.ResponseMS
		}
		s.recordGenerateLog(ctx, log)
		return nil, err
	}
	encoded, err := json.Marshal(result)
	if err != nil {
		s.recordGenerateLog(ctx, GenerateLogRecord{
			TradeDate: tradeDate,
			Trigger:   trigger,
			Status:    GenerateLogStatusFailed,
			Message:   err.Error(),
			UserID:    userID,
		})
		return nil, err
	}
	if s.repo != nil {
		if err := s.repo.SaveDailyResult(ctx, DailyResult{
			Market:         MarketAShare,
			TradeDate:      tradeDate,
			Trigger:        trigger,
			SnapshotDate:   result.Analysis.SnapshotDate,
			SelectionBasis: result.Analysis.SelectionBasis,
			Model:          cfg.Model,
			PayloadJSON:    string(encoded),
		}); err != nil {
			s.recordGenerateLog(ctx, GenerateLogRecord{
				TradeDate: tradeDate,
				Trigger:   trigger,
				Status:    GenerateLogStatusFailed,
				Message:   err.Error(),
				UserID:    userID,
			})
			return nil, err
		}
	}
	successLog := GenerateLogRecord{
		TradeDate:     tradeDate,
		Trigger:       trigger,
		Status:        GenerateLogStatusSuccess,
		SnapshotDate:  result.Analysis.SnapshotDate,
		CandidatePool: toInt(result.Meta["candidate_pool_size"]),
		Model:         strings.TrimSpace(cfg.Model),
		Message:       "ok",
		UserID:        userID,
		TimeoutSeconds: 240,
	}
	if callMeta != nil {
		successLog.FinishReason = callMeta.FinishReason
		successLog.PromptChars = callMeta.PromptChars
		successLog.CompletionTokens = callMeta.CompletionTokens
		successLog.ResponseMS = callMeta.ResponseMS
	}
	s.recordGenerateLog(ctx, successLog)
	return result, nil
}

func (s *Service) recordGenerateLog(ctx context.Context, record GenerateLogRecord) {
	if s.repo == nil {
		return
	}
	if err := s.repo.SaveGenerateLog(ctx, record); err != nil {
		fmt.Printf("[ai-picker] save generate log failed: %v\n", err)
	}
}

type aiChatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type aiChatRequest struct {
	Model          string          `json:"model"`
	Messages       []aiChatMessage `json:"messages"`
	Temperature    float64         `json:"temperature"`
	ResponseFormat *aiResponseFmt  `json:"response_format,omitempty"`
	// MaxTokens は意図的に省略：provider のデフォルト上限に委ねることで、
	// 長い JSON 出力が finish_reason=length で打ち切られる問題を回避する。
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

// llmCallMeta 记录一次 LLM 调用的关键可观测信息，供上层日志使用。
type llmCallMeta struct {
	PromptChars      int
	PromptTokens     int
	CompletionTokens int
	FinishReason     string
	ResponseMS       int
}

// callLLM 调用 LLM 并返回解析后的结果与调用元信息。
//
// MaxTokens 故意不设置：将输出 token 上限交给 provider 默认策略，
// 避免在长 JSON schema + 中文长文本场景下被 finish_reason=length 截断。
// 对应地，HTTP timeout 设置为 240 秒以承受更长的推理耗时。
func callLLM(ctx context.Context, cfg AIConfig, userID, userPrompt string) (*PickerResponse, *llmCallMeta, error) {
	body := aiChatRequest{
		Model: cfg.Model,
		Messages: []aiChatMessage{
			{Role: "system", Content: aShareSystemPrompt},
			{Role: "user", Content: userPrompt},
		},
		Temperature:    0.2,
		ResponseFormat: &aiResponseFmt{Type: "json_object"},
	}
	encoded, err := json.Marshal(body)
	if err != nil {
		return nil, nil, err
	}
	endpoint := strings.TrimRight(cfg.BaseURL, "/") + "/chat/completions"

	// 允许重试一次处理瞬时网络抖动；finish_reason=length 例外（直接失败，重试无意义）。
	const maxRetries = 1
	var lastErr error
	for attempt := 0; attempt <= maxRetries; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return nil, nil, ctx.Err()
			case <-time.After(2 * time.Second):
			}
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(encoded))
		if err != nil {
			return nil, nil, err
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+cfg.APIKey)
		// timeout 提升至 240s：不设 max_tokens 后模型可自由输出完整 JSON，
		// 推理时间相应变长，短超时会造成 EOF / context deadline exceeded 替换旧报错。
		client := &http.Client{Timeout: 240 * time.Second}
		meta := &llmCallMeta{PromptChars: len(userPrompt)}
		logEntry := strategy.AILogEntry{
			UserID:      userID,
			FeatureKey:  "ai_picker",
			FeatureName: "AI 选股",
			Model:       cfg.Model,
			ExtraMeta: map[string]any{
				"attempt":       attempt + 1,
				"prompt_chars":  len(userPrompt),
				"max_tokens":    "unlimited",
				"timeout_secs":  240,
			},
		}
		start := time.Now()
		resp, err := client.Do(req)
		meta.ResponseMS = int(time.Since(start).Milliseconds())
		logEntry.ResponseMS = meta.ResponseMS
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
			return nil, nil, readErr
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
			return nil, nil, err
		}
		logEntry.ApplyUsage(parsed.Usage)
		meta.PromptTokens = parsed.Usage.PromptTokens
		meta.CompletionTokens = parsed.Usage.CompletionTokens
		if parsed.Error != nil {
			logEntry.Status = "error"
			logEntry.ErrorMessage = parsed.Error.Message
			strategy.LogAICall(logEntry)
			return nil, nil, fmt.Errorf("%s", parsed.Error.Message)
		}
		if len(parsed.Choices) == 0 {
			logEntry.Status = "error"
			logEntry.ErrorMessage = "empty choices"
			strategy.LogAICall(logEntry)
			return nil, nil, fmt.Errorf("AI 未返回有效结果")
		}
		choice := parsed.Choices[0]
		meta.FinishReason = choice.FinishReason
		logEntry.ExtraMeta["finish_reason"] = choice.FinishReason
		logEntry.ExtraMeta["completion_tokens"] = parsed.Usage.CompletionTokens
		raw := strings.TrimSpace(choice.Message.Content)

		// finish_reason=length：即使去掉了 max_tokens，provider 仍可能有自身默认上限，
		// 此时 JSON 必然不完整。直接失败，不重试（重试同样会被截断）。
		if choice.FinishReason == "length" {
			logEntry.Status = "error"
			logEntry.ErrorMessage = fmt.Sprintf(
				"output truncated (finish_reason=length, completion_tokens=%d, prompt_chars=%d); provider 仍有默认 token 上限",
				parsed.Usage.CompletionTokens, len(userPrompt),
			)
			strategy.LogAICall(logEntry)
			return nil, meta, fmt.Errorf(
				"AI 输出被 provider 截断 (finish_reason=length, completion_tokens=%d)，请联系管理员确认 provider 默认上限",
				parsed.Usage.CompletionTokens,
			)
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
		return &result, meta, nil
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("AI 选股调用失败")
	}
	return nil, nil, lastErr
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

func buildUserPrompt(snapshotDate, trigger, marketSummary string, techStats technicalCoverageStats, candidates []Candidate) string {
	var sb strings.Builder
	sb.WriteString("请基于以下候选池为 A 股生成 4 只股票的推荐组合。\n")
	fmt.Fprintf(&sb, "快照日期：%s\n触发方式：%s\n市场摘要：%s\n", snapshotDate, trigger, strings.TrimSpace(marketSummary))
	if techStats.HasPartialMissing() {
		fmt.Fprintf(&sb, "技术增强提示：部分技术指标缺失（已覆盖 %d/%d 只），缺失股票仍可基于因子质量纳入推荐，但理由里要明确说明技术面信息不足。\n", techStats.WithTechnicalData, techStats.Total)
	} else {
		fmt.Fprintf(&sb, "技术增强提示：候选池技术指标覆盖完整（%d/%d）。\n", techStats.WithTechnicalData, techStats.Total)
	}
	sb.WriteString("\n候选池如下（只能从中选择）:\n")
	for idx, c := range candidates {
		fmt.Fprintf(&sb, "%d. code=%s, symbol=%s, name=%s, industry=%s, price=%.2f, composite=%s, hit_factors=%s, factor_tags=%s, technical_status=%s, technical_tags=%s, value=%s, dividend=%s, growth=%s, quality=%s, momentum=%s, size=%s, low_vol=%s, ret20=%s, ma20_gap=%s, ma60_gap=%s, rsi14=%s, vol20=%s, volume_ratio=%s\n",
			idx+1, c.Code, c.Symbol, c.Name, c.Industry, c.ClosePrice,
			fmtScore(c.CompositeScore), strings.Join(c.HitFactors, "/"), strings.Join(c.FactorTags, "/"), technicalStatusLabel(c.TechnicalDataComplete), strings.Join(c.TechnicalTags, "/"), fmtScore(c.ValueScore), fmtScore(c.DividendYieldScore), fmtScore(c.GrowthScore), fmtScore(c.QualityScore), fmtScore(c.MomentumScore), fmtScore(c.SizeScore), fmtScore(c.LowVolatilityScore), fmtNullableFloat(c.ChangePct20D), fmtNullableFloat(c.DistanceToMA20Pct), fmtNullableFloat(c.DistanceToMA60Pct), fmtNullableFloat(c.RSI14), fmtNullableFloat(c.Volatility20D), fmtNullableFloat(c.VolumeMA5ToMA20))
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

func fmtNullableFloat(v *float64) string {
	if v == nil {
		return "null"
	}
	return fmt.Sprintf("%.2f", *v)
}

func technicalStatusLabel(complete bool) string {
	if complete {
		return "complete"
	}
	return "missing"
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

type technicalCoverageStats struct {
	Total             int
	WithTechnicalData int
}

func (s technicalCoverageStats) HasPartialMissing() bool {
	return s.Total > 0 && s.WithTechnicalData < s.Total
}

func (s *Service) buildCandidatePool(ctx context.Context, snapshotDate string) ([]Candidate, string, technicalCoverageStats, error) {
	resp, err := s.factorLab.Screen(ctx, factorlab.FactorScreenerRequest{SnapshotDate: snapshotDate, Page: 1, PageSize: 5000})
	if err != nil {
		return nil, "", technicalCoverageStats{}, err
	}
	investable := make([]factorlab.FactorScreenerItem, 0, len(resp.Items))
	composites := make([]float64, 0, len(resp.Items))
	for _, item := range resp.Items {
		if !isInvestable(item) || item.CompositeScore == nil {
			continue
		}
		investable = append(investable, item)
		composites = append(composites, *item.CompositeScore)
	}
	if len(investable) == 0 {
		return nil, "", technicalCoverageStats{}, fmt.Errorf("%w: 无可投资股票", ErrUnavailable)
	}
	threshold := percentile(composites, 0.50)
	pool := s.recallCandidates(investable, threshold, 10, 8)
	if len(pool) < 12 {
		threshold = percentile(composites, 0.45)
		pool = s.recallCandidates(investable, threshold, 15, 12)
	}
	tradeDate := snapshotDate
	if s.technicalSvc != nil {
		if err := s.technicalSvc.EnsureForCandidates(ctx, tradeDate, pool); err != nil {
			fmt.Printf("[ai-picker] ensure technical snapshots failed: %v\n", err)
		}
	}
	techMap := map[string]TechnicalSnapshot{}
	if s.technicalRepo != nil {
		codes := make([]string, 0, len(pool))
		for _, item := range pool {
			codes = append(codes, item.Code)
		}
		techItems, err := s.technicalRepo.GetByTradeDateAndCodes(ctx, tradeDate, codes)
		if err != nil {
			fmt.Printf("[ai-picker] load technical snapshots failed: %v\n", err)
		} else {
			for _, item := range techItems {
				techMap[item.Code] = item
			}
		}
	}
	stats := technicalCoverageStats{Total: len(pool), WithTechnicalData: len(techMap)}
	candidates := make([]Candidate, 0, len(pool))
	for _, item := range pool {
		tech, ok := techMap[item.Code]
		candidates = append(candidates, buildCandidate(item, tech, ok))
	}
	return candidates, buildMarketSummary(candidates, stats), stats, nil
}

func (s *Service) recallCandidates(items []factorlab.FactorScreenerItem, minComposite float64, topN, industryCap int) []FactorCandidate {
	factorExtractors := []struct {
		key   string
		score func(factorlab.FactorScreenerItem) *float64
	}{
		{"value", func(i factorlab.FactorScreenerItem) *float64 { return i.ValueScore }},
		{"dividend_yield", func(i factorlab.FactorScreenerItem) *float64 { return i.DividendYieldScore }},
		{"growth", func(i factorlab.FactorScreenerItem) *float64 { return i.GrowthScore }},
		{"quality", func(i factorlab.FactorScreenerItem) *float64 { return i.QualityScore }},
		{"momentum", func(i factorlab.FactorScreenerItem) *float64 { return i.MomentumScore }},
		{"size", func(i factorlab.FactorScreenerItem) *float64 { return i.SizeScore }},
		{"low_volatility", func(i factorlab.FactorScreenerItem) *float64 { return i.LowVolatilityScore }},
	}
	merged := map[string]FactorCandidate{}
	for _, extractor := range factorExtractors {
		filtered := make([]factorlab.FactorScreenerItem, 0, len(items))
		for _, item := range items {
			score := extractor.score(item)
			if score == nil || item.CompositeScore == nil || *item.CompositeScore < minComposite {
				continue
			}
			filtered = append(filtered, item)
		}
		sort.Slice(filtered, func(i, j int) bool { return deref(extractor.score(filtered[i])) > deref(extractor.score(filtered[j])) })
		if len(filtered) > topN {
			filtered = filtered[:topN]
		}
		for _, item := range filtered {
			existing, ok := merged[item.Code]
			if !ok {
				existing = FactorCandidate{FactorScreenerItem: item, HitFactors: map[string]struct{}{}}
			}
			existing.HitFactors[extractor.key] = struct{}{}
			merged[item.Code] = existing
		}
	}
	byIndustry := map[string][]FactorCandidate{}
	for _, item := range merged {
		byIndustry[item.Industry] = append(byIndustry[item.Industry], item)
	}
	out := make([]FactorCandidate, 0, len(merged))
	for _, items := range byIndustry {
		sort.Slice(items, func(i, j int) bool {
			if len(items[i].HitFactors) != len(items[j].HitFactors) {
				return len(items[i].HitFactors) > len(items[j].HitFactors)
			}
			if deref(items[i].CompositeScore) != deref(items[j].CompositeScore) {
				return deref(items[i].CompositeScore) > deref(items[j].CompositeScore)
			}
			if deref(items[i].QualityScore) != deref(items[j].QualityScore) {
				return deref(items[i].QualityScore) > deref(items[j].QualityScore)
			}
			return deref(items[i].MomentumScore) > deref(items[j].MomentumScore)
		})
		if len(items) > industryCap {
			items = items[:industryCap]
		}
		out = append(out, items...)
	}
	return out
}

func isInvestable(item factorlab.FactorScreenerItem) bool {
	if item.IsNewStock || strings.TrimSpace(item.Industry) == "" {
		return false
	}
	code := strings.TrimSpace(item.Code)
	if strings.HasPrefix(code, "8") || strings.HasPrefix(code, "4") {
		return false
	}
	if strings.HasPrefix(code, "688") || strings.HasPrefix(code, "300") {
		return false
	}
	name := strings.ToUpper(strings.TrimSpace(item.Name))
	if strings.Contains(name, "ST") {
		return false
	}
	return true
}

func buildCandidate(item FactorCandidate, tech TechnicalSnapshot, hasTech bool) Candidate {
	hitFactors := make([]string, 0, len(item.HitFactors))
	for key := range item.HitFactors {
		hitFactors = append(hitFactors, key)
	}
	sort.Strings(hitFactors)
	candidate := Candidate{Code: item.Code, Symbol: item.Symbol, Name: item.Name, Industry: item.Industry, ClosePrice: item.ClosePrice, CompositeScore: item.CompositeScore, ValueScore: item.ValueScore, DividendYieldScore: item.DividendYieldScore, GrowthScore: item.GrowthScore, QualityScore: item.QualityScore, MomentumScore: item.MomentumScore, SizeScore: item.SizeScore, LowVolatilityScore: item.LowVolatilityScore, HitFactors: hitFactors, FactorTags: buildFactorTags(item.FactorScreenerItem), TechnicalDataComplete: hasTech}
	if !hasTech {
		return candidate
	}
	candidate.TechnicalTags = decodeTechTags(tech.TechTagsJSON)
	candidate.ChangePct20D = float64Ptr(tech.ChangePct20D)
	candidate.DistanceToMA20Pct = float64Ptr(tech.DistanceToMA20Pct)
	candidate.DistanceToMA60Pct = float64Ptr(tech.DistanceToMA60Pct)
	candidate.RSI14 = float64Ptr(tech.RSI14)
	candidate.Volatility20D = float64Ptr(tech.Volatility20D)
	candidate.VolumeMA5ToMA20 = float64Ptr(tech.VolumeMA5ToMA20)
	return candidate
}

func buildFactorTags(item factorlab.FactorScreenerItem) []string {
	tags := make([]string, 0, 4)
	if deref(item.QualityScore) >= 80 {
		tags = append(tags, "高质量")
	}
	if deref(item.GrowthScore) >= 80 {
		tags = append(tags, "高成长")
	}
	if deref(item.ValueScore) >= 80 {
		tags = append(tags, "高价值")
	}
	if deref(item.DividendYieldScore) >= 80 {
		tags = append(tags, "高股息")
	}
	if deref(item.LowVolatilityScore) >= 80 {
		tags = append(tags, "低波稳健")
	}
	if deref(item.MomentumScore) >= 80 {
		tags = append(tags, "趋势偏强")
	}
	if len(tags) > 4 {
		tags = tags[:4]
	}
	return tags
}

func buildMarketSummary(candidates []Candidate, techStats technicalCoverageStats) string {
	if len(candidates) == 0 {
		return "当前候选池为空"
	}
	styleCount := map[string]int{}
	industryCount := map[string]int{}
	for _, c := range candidates {
		for _, tag := range c.FactorTags {
			styleCount[tag]++
		}
		industryCount[c.Industry]++
	}
	summary := fmt.Sprintf("当前候选池共%d只，风格以%s为主，行业集中在%s。", len(candidates), topKeys(styleCount, 3), topKeys(industryCount, 3))
	if techStats.HasPartialMissing() {
		summary += fmt.Sprintf(" 其中 %d 只缺少技术快照，需以因子与行业分散度为主进行判断。", techStats.Total-techStats.WithTechnicalData)
	}
	return summary
}

func topKeys(m map[string]int, n int) string {
	type kv struct {
		K string
		V int
	}
	items := make([]kv, 0, len(m))
	for k, v := range m {
		if strings.TrimSpace(k) != "" {
			items = append(items, kv{k, v})
		}
	}
	sort.Slice(items, func(i, j int) bool { return items[i].V > items[j].V })
	if len(items) > n {
		items = items[:n]
	}
	out := make([]string, 0, len(items))
	for _, item := range items {
		out = append(out, item.K)
	}
	if len(out) == 0 {
		return "无明显集中"
	}
	return strings.Join(out, "、")
}

func deref(v *float64) float64 {
	if v == nil {
		return 0
	}
	return *v
}

func float64Ptr(v float64) *float64 {
	value := v
	return &value
}

func fmtFloat(v float64) string { return fmt.Sprintf("%.2f", v) }

func percentile(values []float64, q float64) float64 {
	if len(values) == 0 {
		return 0
	}
	sort.Float64s(values)
	idx := int(float64(len(values)-1) * q)
	if idx < 0 {
		idx = 0
	}
	if idx >= len(values) {
		idx = len(values) - 1
	}
	return values[idx]
}

func toInt(value any) int {
	switch v := value.(type) {
	case int:
		return v
	case int32:
		return int(v)
	case int64:
		return int(v)
	case float64:
		return int(v)
	default:
		return 0
	}
}

func (s *Service) AdminGenerateStatus(ctx context.Context) (*AdminGenerateStatus, error) {
	if s.repo == nil {
		return nil, ErrUnavailable
	}
	result, err := s.repo.GetLatestDailyResult(ctx, MarketAShare)
	if err != nil && !isDailyResultMissing(err) {
		return nil, err
	}
	latestLog, err := s.repo.GetLatestGenerateLog(ctx)
	if err != nil && !isDailyResultMissing(err) {
		return nil, err
	}
	logs, err := s.repo.ListGenerateLogs(ctx, maxGenerateErrorLogs)
	if err != nil {
		return nil, err
	}
	payload := &AdminGenerateStatus{Logs: logs}
	if result != nil {
		payload.LatestResult = result
	}
	if latestLog != nil {
		payload.LatestLog = latestLog
	}
	return payload, nil
}
