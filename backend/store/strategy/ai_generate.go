package strategy

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"
)

// ── AI 策略生成配置（复用 screener 包的 AI 配置格式） ──

type AIConfig struct {
	APIKey  string
	BaseURL string
	Model   string
}

func (c AIConfig) Enabled() bool {
	return strings.TrimSpace(c.APIKey) != ""
}

// ── 限流器 ──

type AIRateLimiter struct {
	mu     sync.Mutex
	counts map[string]int
	limit  int
}

func NewAIRateLimiter(limit int) *AIRateLimiter {
	rl := &AIRateLimiter{
		counts: make(map[string]int),
		limit:  limit,
	}
	go rl.cleanupLoop()
	return rl
}

func (rl *AIRateLimiter) Allow(userID string) bool {
	key := userID + ":" + time.Now().Format("2006010215")
	rl.mu.Lock()
	defer rl.mu.Unlock()
	if rl.counts[key] >= rl.limit {
		return false
	}
	rl.counts[key]++
	return true
}

func (rl *AIRateLimiter) cleanupLoop() {
	ticker := time.NewTicker(10 * time.Minute)
	defer ticker.Stop()
	for range ticker.C {
		currentHour := time.Now().Format("2006010215")
		rl.mu.Lock()
		for key := range rl.counts {
			parts := strings.SplitN(key, ":", 2)
			if len(parts) == 2 && parts[1] != currentHour {
				delete(rl.counts, key)
			}
		}
		rl.mu.Unlock()
	}
}

// ── 市场特征摘要（从 MovingAverages API 聚合） ──

type MarketSummary struct {
	Ticker          string  `json:"ticker"`
	Name            string  `json:"name"`
	Price           float64 `json:"price"`
	ChangePct60D    float64 `json:"change_pct_60d"`
	Volatility20D   float64 `json:"volatility_20d"`
	VolumeMA5toMA20 float64 `json:"volume_ma5_to_ma20"`
	RSI14           float64 `json:"rsi14"`
	RSI14Status     string  `json:"rsi14_status"`
	MACD            float64 `json:"macd"`
	MACDSignal      float64 `json:"macd_signal"`
	MACDHistogram   float64 `json:"macd_histogram"`
	BollingerBW     float64 `json:"bollinger_bandwidth"`
	BollingerPctB   float64 `json:"bollinger_percent_b"`
	MA5             float64 `json:"ma5"`
	MA20            float64 `json:"ma20"`
	MA60            float64 `json:"ma60"`
	MA200           float64 `json:"ma200"`
	MAStatus        string  `json:"ma_status"`
}

// ── AI 推荐结果 ──

type AIRecommendation struct {
	ImplementationKey string         `json:"implementation_key"`
	StrategyLabel     string         `json:"strategy_label"`
	Category          string         `json:"category"`
	Params            map[string]any `json:"params"`
	Reason            string         `json:"reason"`
	Confidence        string         `json:"confidence"`
	MarketSummary     MarketSummary  `json:"market_summary"`
}

// ── 回测预览 ──

type BacktestPreview struct {
	TotalReturn    float64 `json:"total_return"`
	MaxDrawdown    float64 `json:"max_drawdown"`
	SharpeRatio    float64 `json:"sharpe_ratio"`
	WinRate        float64 `json:"win_rate"`
	TradeCount     int     `json:"trade_count"`
	AnnualReturn   float64 `json:"annual_return"`
	BacktestPeriod string  `json:"backtest_period"`
}

// ── 迭代轮次 ──

type IterationRound struct {
	Round           int             `json:"round"`
	Params          map[string]any  `json:"params"`
	BacktestPreview BacktestPreview `json:"backtest_preview"`
	Adjustment      string          `json:"adjustment"`
}

type AIGenerateResponse struct {
	Recommendation AIRecommendation `json:"recommendation"`
	BacktestPreview *BacktestPreview `json:"backtest_preview,omitempty"`
	Iterations      []IterationRound `json:"iterations,omitempty"`
	FinalRound      int              `json:"final_round"`
}

// ── LLM 调用结构 ──

type aiChatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type aiChatRequest struct {
	Model       string          `json:"model"`
	Messages    []aiChatMessage `json:"messages"`
	Temperature float64         `json:"temperature"`
	MaxTokens   int             `json:"max_tokens,omitempty"`
}

type aiChatChoice struct {
	Message aiChatMessage `json:"message"`
}

type aiChatResponse struct {
	Choices []aiChatChoice `json:"choices"`
	Error   *struct {
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

// ── LLM 输出解析 ──

type llmStrategyOutput struct {
	ImplementationKey string         `json:"implementation_key"`
	Params            map[string]any `json:"params"`
	Reason            string         `json:"reason"`
	Confidence        string         `json:"confidence"`
}

// ── 策略标签映射 ──

var strategyLabelMap = map[string]struct {
	Label    string
	Category string
}{
	"trend_cross":         {"趋势跟踪（双均线）", "趋势"},
	"grid":                {"网格交易", "震荡"},
	"bollinger_reversion": {"均值回归（布林带）", "均值回归"},
	"rsi_range":           {"区间交易（RSI）", "区间"},
	"macd_cross":          {"MACD 趋势策略", "趋势"},
	"volume_breakout":     {"放量突破", "量价"},
	"dual_confirm":        {"双重确认（趋势+动量）", "组合"},
	"bollinger_macd":      {"布林带+MACD 组合", "组合"},
}

// ── System Prompt ──

const aiGenerateSystemPrompt = `你是一个量化策略顾问。根据以下股票的技术面数据，推荐最合适的交易策略并给出参数。

## 可用策略（只能从这 8 个中选择）

1. trend_cross（趋势跟踪·双均线）
   - 适用：均线多头或空头排列明确，趋势方向清晰
   - 参数：ma_short (2-250, 整数, 天), ma_long (3-500, 整数, 天)
   - 约束：ma_short < ma_long

2. grid（网格交易）
   - 适用：价格在窄幅区间震荡，无明确趋势，波动率低
   - 参数：grid_count (2-20, 整数, 层), grid_step (0.001-0.5, 小数, 比例)

3. bollinger_reversion（均值回归·布林带）
   - 适用：价格大幅偏离布林带，波动率放大后有回归中轨倾向
   - 参数：bb_period (5-250, 整数, 天), bb_std (0.1-5, 小数, 倍)

4. rsi_range（区间交易·RSI）
   - 适用：RSI 在超买超卖区间反复震荡，弱趋势
   - 参数：rsi_period (2-120, 整数, 天), rsi_low (1-50, 数值), rsi_high (50-99, 数值)
   - 约束：rsi_low < rsi_high

5. macd_cross（MACD趋势策略）
   - 适用：趋势在加速或减速，DIF/DEA 有交叉倾向
   - 参数：fast_period (2-50, 整数, 天), slow_period (5-100, 整数, 天), signal_period (2-30, 整数, 天)
   - 约束：fast_period < slow_period

6. volume_breakout（放量突破）
   - 适用：成交量显著放大（均量比>1.5），股价创阶段新高
   - 参数：lookback (5-120, 整数, 天), volume_multiple (1.2-5.0, 小数, 倍), exit_ma_period (5-120, 整数, 天)

7. dual_confirm（双重确认·趋势+动量）
   - 适用：需要更稳妥的信号确认，同时满足趋势和动量条件
   - 参数：ma_short (2-120, 整数, 天), ma_long (5-250, 整数, 天), rsi_period (2-60, 整数, 天), rsi_low (10-50, 数值), rsi_high (50-90, 数值), confirm_window (1-20, 整数, 天), logic_mode ("and"或"or")
   - 约束：ma_short < ma_long, rsi_low < rsi_high

8. bollinger_macd（布林带+MACD组合）
   - 适用：寻找底部共振（下轨触及+MACD翻正）或顶部共振信号
   - 参数：bb_period (5-100, 整数, 天), bb_std (0.5-4.0, 小数, 倍), fast_period (2-50, 整数, 天), slow_period (5-100, 整数, 天), signal_period (2-30, 整数, 天), logic_mode ("and"或"or")
   - 约束：fast_period < slow_period

## 决策规则

- 均线多头排列(MA5>MA20>MA60) + RSI 40-70 → 优先 trend_cross 或 macd_cross
- 均线空头排列 + RSI < 30 + 价格接近布林带下轨(%B<0.2) → 考虑 bollinger_reversion 或 bollinger_macd(and)
- 波动率低(<20%) + 无明确趋势(MA交织) → 考虑 grid 或 rsi_range
- 均量比 > 1.5 + 60日涨幅 > 0 + 价格在布林带上半部 → 考虑 volume_breakout
- MACD 柱状图由负转正 + 价格在布林带下半部 → bollinger_macd(and)
- 不确定或指标矛盾时 → dual_confirm(and) 最稳妥
- 如果股票数据不足（如次新股），推荐 grid 或 dual_confirm(and) 并标注 confidence=low

## 你的股票技术面数据

{market_data}

## 输出要求

严格按 JSON 格式输出，不要输出任何其他内容：

{
  "implementation_key": "选择的策略 key（必须是上述 8 个之一）",
  "params": { 对应策略的参数，键值对 },
  "reason": "中文推荐理由，2-3 句话，从行情特征角度解释",
  "confidence": "high/medium/low"
}`

// ── 核心函数 ──

func GenerateStrategy(ctx context.Context, cfg AIConfig, summary MarketSummary) (*AIGenerateResponse, error) {
	if !cfg.Enabled() {
		return nil, fmt.Errorf("%w: AI 功能未启用，请联系管理员配置 AI_API_KEY", ErrInvalid)
	}

	summaryJSON, err := json.Marshal(summary)
	if err != nil {
		return nil, fmt.Errorf("序列化市场摘要失败: %w", err)
	}

	prompt := strings.Replace(aiGenerateSystemPrompt, "{market_data}", string(summaryJSON), 1)

	userMessage := fmt.Sprintf("请为 %s（%s）推荐最合适的策略和参数。", summary.Name, summary.Ticker)

	body := aiChatRequest{
		Model: cfg.Model,
		Messages: []aiChatMessage{
			{Role: "system", Content: prompt},
			{Role: "user", Content: userMessage},
		},
		Temperature: 0.2,
		MaxTokens:   1024,
	}

	encoded, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("序列化 AI 请求失败: %w", err)
	}

	endpoint := cfg.BaseURL + "/chat/completions"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(encoded))
	if err != nil {
		return nil, fmt.Errorf("创建 AI 请求失败: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+cfg.APIKey)

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("调用 AI 服务失败: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("读取 AI 响应失败: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("AI 服务返回错误 (HTTP %d): %s", resp.StatusCode, truncateStr(string(respBody), 200))
	}

	var chatResp aiChatResponse
	if err := json.Unmarshal(respBody, &chatResp); err != nil {
		return nil, fmt.Errorf("解析 AI 响应失败: %w", err)
	}

	if chatResp.Error != nil {
		return nil, fmt.Errorf("AI 服务报错: %s", chatResp.Error.Message)
	}

	if len(chatResp.Choices) == 0 {
		return nil, fmt.Errorf("AI 未返回有效结果")
	}

	content := strings.TrimSpace(chatResp.Choices[0].Message.Content)
	content = stripAICodeFence(content)

	var output llmStrategyOutput
	if err := json.Unmarshal([]byte(content), &output); err != nil {
		// 兜底：返回 dual_confirm
		return buildFallbackRecommendation(summary, "AI 返回格式不正确，已使用默认推荐"), nil
	}

	return validateAndBuild(output, summary)
}

func validateAndBuild(output llmStrategyOutput, summary MarketSummary) (*AIGenerateResponse, error) {
	// 校验 implementation_key
	meta, ok := strategyLabelMap[output.ImplementationKey]
	if !ok {
		return buildFallbackRecommendation(summary, "AI 返回了不支持的策略类型，已使用默认推荐"), nil
	}

	// 校验 confidence
	confidence := strings.ToLower(strings.TrimSpace(output.Confidence))
	if confidence != "high" && confidence != "medium" && confidence != "low" {
		confidence = "medium"
	}

	reason := strings.TrimSpace(output.Reason)
	if reason == "" {
		reason = "AI 未给出具体理由。"
	}

	// 清理 params：只保留数值和字符串
	params := make(map[string]any)
	for k, v := range output.Params {
		params[k] = v
	}

	return &AIGenerateResponse{
		Recommendation: AIRecommendation{
			ImplementationKey: output.ImplementationKey,
			StrategyLabel:     meta.Label,
			Category:          meta.Category,
			Params:            params,
			Reason:            reason,
			Confidence:        confidence,
			MarketSummary:     summary,
		},
	}, nil
}

func buildFallbackRecommendation(summary MarketSummary, reason string) *AIGenerateResponse {
	return &AIGenerateResponse{
		Recommendation: AIRecommendation{
			ImplementationKey: "dual_confirm",
			StrategyLabel:     "双重确认（趋势+动量）",
			Category:          "组合",
			Params: map[string]any{
				"ma_short":       10,
				"ma_long":        30,
				"rsi_period":     14,
				"rsi_low":        35,
				"rsi_high":       70,
				"confirm_window": 5,
				"logic_mode":     "and",
			},
			Reason:        reason,
			Confidence:    "low",
			MarketSummary: summary,
		},
	}
}

// ── helpers ──

func stripAICodeFence(s string) string {
	s = strings.TrimSpace(s)
	if strings.HasPrefix(s, "```") {
		idx := strings.Index(s, "\n")
		if idx > 0 {
			s = s[idx+1:]
		}
		if strings.HasSuffix(s, "```") {
			s = s[:len(s)-3]
		}
		s = strings.TrimSpace(s)
	}
	return s
}

func truncateStr(s string, maxLen int) string {
	runes := []rune(s)
	if len(runes) <= maxLen {
		return s
	}
	return string(runes[:maxLen]) + "..."
}

// ── 内部回测调用 ──

// CallQuantBacktest calls the Quant engine's /api/backtest endpoint internally
// and returns the raw response as a map. quantBaseURL should be like "http://localhost:8000".
func CallQuantBacktest(ctx context.Context, quantBaseURL string, ticker string, implKey string, params map[string]any) (map[string]any, error) {
	// 计算近 6 个月的日期范围
	now := time.Now()
	endDate := now.AddDate(0, 0, -1).Format("2006-01-02")
	startDate := now.AddDate(0, -6, 0).Format("2006-01-02")

	payload := map[string]any{
		"data_source": "online",
		"ticker":      ticker,
		"start_date":  startDate,
		"end_date":    endDate,
		"capital":     100000,
		"fee_pct":     0.001,
		"runtime_strategy": map[string]any{
			"id":                 "ai-backtest-" + implKey,
			"key":                "ai_backtest_" + implKey,
			"name":               "AI 回测验证",
			"implementation_key": implKey,
			"params":             params,
		},
	}

	encoded, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("序列化回测请求失败: %w", err)
	}

	endpoint := strings.TrimRight(quantBaseURL, "/") + "/api/backtest"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(encoded))
	if err != nil {
		return nil, fmt.Errorf("创建回测请求失败: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 60 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("调用回测引擎失败: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("读取回测响应失败: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("回测引擎返回错误 (HTTP %d): %s", resp.StatusCode, truncateStr(string(respBody), 200))
	}

	var result map[string]any
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("解析回测结果失败: %w", err)
	}

	return result, nil
}

// ExtractBacktestPreview pulls key metrics from a raw backtest result map.
// Quant returns metrics as a flat dict with _pct suffix values (e.g. total_return_pct = 15.3 means 15.3%).
func ExtractBacktestPreview(result map[string]any) BacktestPreview {
	metrics := asNestedMap(result, "metrics")
	preview := BacktestPreview{
		TotalReturn:    asFloat(metrics["total_return_pct"]) / 100,
		MaxDrawdown:    -asFloat(metrics["max_drawdown_pct"]) / 100,
		SharpeRatio:    asFloat(metrics["sharpe_ratio"]),
		WinRate:        asFloat(metrics["win_rate_pct"]) / 100,
		TradeCount:     asIntValue(metrics["total_trades"]),
		AnnualReturn:   asFloat(metrics["annual_return_pct"]) / 100,
		BacktestPeriod: "近 6 个月",
	}
	return preview
}

// ── 迭代优化 Prompt ──

const aiIterateSystemPrompt = `你是一个量化策略调参优化器。根据策略的回测指标，判断是否需要调参，以及如何调参。

## 当前策略信息

策略类型：{implementation_key}
当前参数：{current_params}
股票技术面摘要：{market_summary}

## 回测结果指标

{backtest_metrics}

## 你的任务

分析回测指标，判断策略表现是否满意：
- 优化目标优先级：夏普比率 > 总收益率 > 最大回撤控制
- 如果夏普比率 > 1.5 且总收益 > 0 且最大回撤 < 20%，可以认为表现良好，返回 action=keep
- 如果表现不佳，分析原因并给出调参建议

## 策略参数约束（重要）

{param_constraints}

## 输出要求

严格按 JSON 格式输出，不要输出任何其他内容：

如果保持当前参数：
{"action": "keep", "reason": "中文说明为什么当前参数已经足够好"}

如果需要调参：
{"action": "adjust", "params": { 调整后的完整参数 }, "reason": "中文说明调整了什么、为什么这样调整"}`

// ── 迭代 LLM 输出结构 ──

type llmIterateOutput struct {
	Action string         `json:"action"`
	Params map[string]any `json:"params,omitempty"`
	Reason string         `json:"reason"`
}

// IterateStrategy takes an initial recommendation + backtest preview, and asks AI whether to adjust params.
// Returns the updated params (or same if keep) and the AI's reasoning.
func IterateStrategy(ctx context.Context, cfg AIConfig, implKey string, currentParams map[string]any, summary MarketSummary, preview BacktestPreview) (*llmIterateOutput, error) {
	if !cfg.Enabled() {
		return &llmIterateOutput{Action: "keep", Reason: "AI 未启用"}, nil
	}

	paramsJSON, _ := json.Marshal(currentParams)
	summaryJSON, _ := json.Marshal(summary)
	metricsJSON, _ := json.Marshal(preview)

	constraints := getParamConstraints(implKey)

	prompt := aiIterateSystemPrompt
	prompt = strings.Replace(prompt, "{implementation_key}", implKey, 1)
	prompt = strings.Replace(prompt, "{current_params}", string(paramsJSON), 1)
	prompt = strings.Replace(prompt, "{market_summary}", string(summaryJSON), 1)
	prompt = strings.Replace(prompt, "{backtest_metrics}", string(metricsJSON), 1)
	prompt = strings.Replace(prompt, "{param_constraints}", constraints, 1)

	body := aiChatRequest{
		Model: cfg.Model,
		Messages: []aiChatMessage{
			{Role: "system", Content: prompt},
			{Role: "user", Content: fmt.Sprintf("请分析回测结果并决定是否需要调参。总收益 %.2f%%，夏普比率 %.2f，最大回撤 %.2f%%。", preview.TotalReturn*100, preview.SharpeRatio, preview.MaxDrawdown*100)},
		},
		Temperature: 0.2,
		MaxTokens:   1024,
	}

	encoded, err := json.Marshal(body)
	if err != nil {
		return &llmIterateOutput{Action: "keep", Reason: "序列化请求失败"}, nil
	}

	endpoint := cfg.BaseURL + "/chat/completions"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(encoded))
	if err != nil {
		return &llmIterateOutput{Action: "keep", Reason: "创建请求失败"}, nil
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+cfg.APIKey)

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return &llmIterateOutput{Action: "keep", Reason: "AI 调用失败"}, nil
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return &llmIterateOutput{Action: "keep", Reason: "读取响应失败"}, nil
	}

	if resp.StatusCode != http.StatusOK {
		return &llmIterateOutput{Action: "keep", Reason: "AI 服务返回错误"}, nil
	}

	var chatResp aiChatResponse
	if err := json.Unmarshal(respBody, &chatResp); err != nil {
		return &llmIterateOutput{Action: "keep", Reason: "解析响应失败"}, nil
	}

	if len(chatResp.Choices) == 0 {
		return &llmIterateOutput{Action: "keep", Reason: "AI 未返回结果"}, nil
	}

	content := stripAICodeFence(strings.TrimSpace(chatResp.Choices[0].Message.Content))

	var output llmIterateOutput
	if err := json.Unmarshal([]byte(content), &output); err != nil {
		return &llmIterateOutput{Action: "keep", Reason: "AI 返回格式不正确，保持当前参数"}, nil
	}

	if output.Action != "adjust" {
		output.Action = "keep"
	}

	return &output, nil
}

func getParamConstraints(implKey string) string {
	constraints := map[string]string{
		"trend_cross":         "ma_short (2-250, 整数, 天), ma_long (3-500, 整数, 天); 约束: ma_short < ma_long",
		"grid":                "grid_count (2-20, 整数, 层), grid_step (0.001-0.5, 小数, 比例)",
		"bollinger_reversion": "bb_period (5-250, 整数, 天), bb_std (0.1-5, 小数, 倍)",
		"rsi_range":           "rsi_period (2-120, 整数, 天), rsi_low (1-50, 数值), rsi_high (50-99, 数值); 约束: rsi_low < rsi_high",
		"macd_cross":          "fast_period (2-50, 整数, 天), slow_period (5-100, 整数, 天), signal_period (2-30, 整数, 天); 约束: fast_period < slow_period",
		"volume_breakout":     "lookback (5-120, 整数, 天), volume_multiple (1.2-5.0, 小数, 倍), exit_ma_period (5-120, 整数, 天)",
		"dual_confirm":        "ma_short (2-120, 整数), ma_long (5-250, 整数), rsi_period (2-60, 整数), rsi_low (10-50), rsi_high (50-90), confirm_window (1-20, 整数), logic_mode (and/or); 约束: ma_short < ma_long, rsi_low < rsi_high",
		"bollinger_macd":      "bb_period (5-100, 整数), bb_std (0.5-4.0, 小数), fast_period (2-50, 整数), slow_period (5-100, 整数), signal_period (2-30, 整数), logic_mode (and/or); 约束: fast_period < slow_period",
	}
	if c, ok := constraints[implKey]; ok {
		return c
	}
	return "无特定约束"
}

// ── helpers ──

func asNestedMap(m map[string]any, key string) map[string]any {
	if m == nil {
		return map[string]any{}
	}
	sub, ok := m[key].(map[string]any)
	if !ok {
		return map[string]any{}
	}
	return sub
}

func asFloat(v any) float64 {
	switch val := v.(type) {
	case float64:
		return val
	case float32:
		return float64(val)
	case int:
		return float64(val)
	case int64:
		return float64(val)
	case json.Number:
		f, _ := val.Float64()
		return f
	default:
		return 0
	}
}

func asIntValue(v any) int {
	switch val := v.(type) {
	case float64:
		return int(val)
	case int:
		return val
	case int64:
		return int(val)
	case json.Number:
		i, _ := val.Int64()
		return int(i)
	default:
		return 0
	}
}

// ── 回测引擎 AI 优化建议 ──

const aiAnalyzeBacktestPrompt = `你是一个量化回测分析顾问。用户已经完成了一次历史回测，请分析回测结果并给出具体的优化建议。

## 当前策略信息

策略名称：{strategy_name}
策略类型：{implementation_key}
当前参数：{current_params}

## 回测配置

股票代码：{ticker}
回测区间：{start_date} ~ {end_date}

## 回测结果指标

{backtest_metrics}

## 参数约束

{param_constraints}

## 你的任务

1. 先诊断当前回测表现的优缺点（2-3 句话）
2. 给出具体的调参建议（必须在参数约束范围内）
3. 解释为什么这样调整能改善表现

## 输出要求

严格按 JSON 格式输出，不要输出任何其他内容：

{
  "diagnosis": "中文诊断：当前策略的优势和问题分析（2-3 句话）",
  "suggestion": "中文建议：应该如何调整参数，为什么（2-3 句话）",
  "suggested_params": { 调整后的完整参数，键值对 },
  "confidence": "high/medium/low"
}`

type BacktestAnalysis struct {
	Diagnosis      string         `json:"diagnosis"`
	Suggestion     string         `json:"suggestion"`
	SuggestedParams map[string]any `json:"suggested_params"`
	Confidence     string         `json:"confidence"`
}

type AnalyzeBacktestInput struct {
	StrategyName      string         `json:"strategy_name"`
	ImplementationKey string         `json:"implementation_key"`
	CurrentParams     map[string]any `json:"current_params"`
	Ticker            string         `json:"ticker"`
	StartDate         string         `json:"start_date"`
	EndDate           string         `json:"end_date"`
	Metrics           map[string]any `json:"metrics"`
}

func AnalyzeBacktest(ctx context.Context, cfg AIConfig, input AnalyzeBacktestInput) (*BacktestAnalysis, error) {
	if !cfg.Enabled() {
		return nil, fmt.Errorf("AI 功能未启用")
	}

	paramsJSON, _ := json.Marshal(input.CurrentParams)
	metricsJSON, _ := json.Marshal(input.Metrics)
	constraints := getParamConstraints(input.ImplementationKey)

	prompt := aiAnalyzeBacktestPrompt
	prompt = strings.Replace(prompt, "{strategy_name}", input.StrategyName, 1)
	prompt = strings.Replace(prompt, "{implementation_key}", input.ImplementationKey, 1)
	prompt = strings.Replace(prompt, "{current_params}", string(paramsJSON), 1)
	prompt = strings.Replace(prompt, "{ticker}", input.Ticker, 1)
	prompt = strings.Replace(prompt, "{start_date}", input.StartDate, 1)
	prompt = strings.Replace(prompt, "{end_date}", input.EndDate, 1)
	prompt = strings.Replace(prompt, "{backtest_metrics}", string(metricsJSON), 1)
	prompt = strings.Replace(prompt, "{param_constraints}", constraints, 1)

	body := aiChatRequest{
		Model: cfg.Model,
		Messages: []aiChatMessage{
			{Role: "system", Content: prompt},
			{Role: "user", Content: "请分析这次回测结果并给出优化建议。"},
		},
		Temperature: 0.3,
		MaxTokens:   1024,
	}

	encoded, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("序列化请求失败: %w", err)
	}

	endpoint := cfg.BaseURL + "/chat/completions"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(encoded))
	if err != nil {
		return nil, fmt.Errorf("创建请求失败: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+cfg.APIKey)

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("AI 调用失败: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("读取响应失败: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("AI 服务返回错误 (HTTP %d): %s", resp.StatusCode, truncateStr(string(respBody), 200))
	}

	var chatResp aiChatResponse
	if err := json.Unmarshal(respBody, &chatResp); err != nil {
		return nil, fmt.Errorf("解析响应失败: %w", err)
	}

	if len(chatResp.Choices) == 0 {
		return nil, fmt.Errorf("AI 未返回有效结果")
	}

	content := stripAICodeFence(strings.TrimSpace(chatResp.Choices[0].Message.Content))

	var analysis BacktestAnalysis
	if err := json.Unmarshal([]byte(content), &analysis); err != nil {
		return &BacktestAnalysis{
			Diagnosis:  "AI 返回格式异常，无法解析优化建议。",
			Suggestion: "建议手动调整参数后重新回测。",
			Confidence: "low",
		}, nil
	}

	if analysis.Confidence != "high" && analysis.Confidence != "medium" && analysis.Confidence != "low" {
		analysis.Confidence = "medium"
	}

	return &analysis, nil
}
