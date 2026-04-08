package strategy

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/woodyyan/pumpkin-pro/backend/store/portfolio"
)

// ── AI 个股诊断：输入 / 输出结构体 ──

// StockAnalysisInput 由前端组装，包含 6 大类数据
type StockAnalysisInput struct {
	SymbolMeta     map[string]any `json:"symbol_meta"`
	Market         map[string]any `json:"market"`
	Technical      map[string]any `json:"technical"`
	Fundamentals   map[string]any `json:"fundamentals"`
	MarketOverview map[string]any `json:"market_overview"`
	Portfolio      map[string]any `json:"portfolio"`
}

// StockAnalysisOutput LLM 返回的结构化分析结果
type StockAnalysisOutput struct {
	Signal            string                 `json:"signal"`
	ConfidenceScore   int                    `json:"confidence_score"`
	ConfidenceLevel   string                 `json:"confidence_level"`
	LogicSummary      string                 `json:"logic_summary"`
	RiskWarnings      []string               `json:"risk_warnings"`
	TradingSuggestions map[string]any        `json:"trading_suggestions"`
	DataTimestamp     string                 `json:"data_timestamp"`
}

// AnalysisResponse API 返回给前端的完整响应
type AnalysisResponse struct {
	Analysis *StockAnalysisOutput `json:"analysis"`
	Meta     map[string]any       `json:"meta"`
}

// ── System Prompt（固定不变） ──

const stockAnalysisSystemPrompt = `你是一位资深量化投资顾问，擅长从技术面、基本面和市场环境三个维度综合分析 A 股和港股个股，给出客观的交易参考意见。

## 核心原则
1. 你必须严格以提供的客观数据为依据进行分析，不得编造数据
2. 信号判断必须自洽：signal、logic_summary、risk_warnings、trading_suggestions 四者之间不能矛盾
3. A 股遵循涨红跌绿惯例，港股同理
4. 给出的价格建议（买入区间、止损位、目标位）必须基于当前价格和技术支撑/压力位给出合理区间，不能偏离现价过远（一般止损不超过 ±10%，目标位不超过 ±25%）
5. 如果关键数据大量缺失（特别是价格和核心技术指标），confidence_score 不应超过 40，且 logic_summary 必须说明数据局限性

## 输出格式要求
你只能输出一个 JSON 对象，包含以下字段，不要有任何其他文字：

{
  "signal": "buy 或 sell 或 hold 三选一",
  "confidence_score": 0-100 的整数，
  "confidence_level": "high(≥70) / medium(40-69) / low(<40)",
  "logic_summary": "3-8句中文分析，综合技术面+基本面+市场环境+持仓状态的判断逻辑。
    必须解释为什么给出该 signal。提及具体数值（如'MA5=1738 高于 MA20=1712 形成短期支撑'）。
    如果某类数据有缺失，需说明'因XX数据缺失，主要依据YY判断'。",
  "risk_warnings": ["1-4条具体风险", "每条一句话，避免空洞表述"],
  "trading_suggestions": {
    "action_suggestion": "一段话总结操作建议（2-4句）",
    "entry_zone": {"low": 数值, "high": 数值, "currency": "CNY或HKD"},
    "stop_loss": {"price": 数值, "pct": 百分比数值如-4.8},
    "take_profit": {"price": 数值, "pct": 百分比数值如8.5},
    "position_size_pct": "如 '15-20%'",
    "time_horizon": "短期(1-2周) / 中期(1-3月) / 长期(3月以上)"
  },
  "data_timestamp": "当前时间戳 ISO 格式"}
## 信号判断参考框架（不是绝对规则，需结合多因素）
### 偏向 buy 的情形
- 均线多头排列 + 价格站稳 MA20 以上 + RSI 50-70（非超买）+ MACD 金叉或柱状图转正 + 成交量温和放大
- 基本面支撑：PE 低于行业均值 / PEG < 1 / 净利润增长率为正
- 大盘环境配合：至少 2/3 主要指数上涨
- 结合持仓：若已亏损但出现企稳信号，可考虑补仓

### 偏向 sell 的情形
- 均线空头排列 + 价格跌破 MA60 + RSI > 70 后回落或持续低于 30 + MACD 死叉 + 放量下跌
- 基本面恶化：PE 远超行业均值 / 净利润负增长 / PEG > 2
- 大盘环境恶劣：主要指数普遍下跌超过 2%
- 结合持仓：若已盈利较多且出现顶部信号，建议止盈

### 偏向 hold 的情形
- 方向不明：均线纠缠 / RSI 中性区(40-60) / MACD 在零轴附近
- 缺乏明确催化剂，等待方向选择
- 已有持仓且盈亏不大，无明显加减仓信号`

// ── 核心：执行 AI 个股诊断 ──

func AnalyzeStock(ctx context.Context, cfg AIConfig, input *StockAnalysisInput, profile *portfolio.InvestmentProfile) (*AnalysisResponse, error) {
	if !cfg.Enabled() {
		return nil, fmt.Errorf("%w: AI 功能未启用，请联系管理员配置 AI_API_KEY", ErrInvalid)
	}

	// 拼接 User Prompt
	userPrompt := buildStockUserPrompt(input, profile)

	body := aiChatRequest{
		Model: cfg.Model,
		Messages: []aiChatMessage{
			{Role: "system", Content: stockAnalysisSystemPrompt},
			{Role: "user", Content: userPrompt},
		},
		Temperature: 0.2,
		MaxTokens:   2048,
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

	client := &http.Client{Timeout: 45 * time.Second}
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

	content := stripAICodeFence(strings.TrimSpace(chatResp.Choices[0].Message.Content))

	var output StockAnalysisOutput
	if err := json.Unmarshal([]byte(content), &output); err != nil {
		return nil, fmt.Errorf("AI 返回的 JSON 格式不正确: %w", err)
	}

	// 后置校验
	warnings := validateStockAnalysis(input, &output)

	now := time.Now().UTC().Format(time.RFC3339)

	// 构建 data_completeness
	completeness := buildDataCompleteness(input, profile)

	return &AnalysisResponse{
		Analysis: &output,
		Meta: map[string]any{
			"model":             cfg.Model,
			"generated_at":      now,
			"data_completeness": completeness,
			"validation":        warnings,
		},
	}, nil
}

// ── User Prompt 模板拼装 ──

func buildStockUserPrompt(input *StockAnalysisInput, profile *portfolio.InvestmentProfile) string {
	var sb strings.Builder

	sb.WriteString("请分析以下股票并给出交易参考意见。\n\n")

	// 股票基本信息
	sm := input.SymbolMeta
	name, _ := sm["name"].(string)
	symbol, _ := sm["symbol"].(string)
	exchange, _ := sm["exchange"].(string)
	currency, _ := sm["currency"].(string)
	mkt := input.Market
	price := asFloat(mkt["price"])
	changePct := asFloat(mkt["change_pct"])

	exLabel := "A 股"
	if exchange == "HKEX" {
		exLabel = "港股"
	}

	fmt.Fprintf(&sb, "## 股票基本信息\n")
	fmt.Fprintf(&sb, "- 股票代码：%s\n", symbol)
	fmt.Fprintf(&sb, "- 股票名称：%s\n", name)
	fmt.Fprintf(&sb, "市场：%s（%s）\n", exchange, exLabel)
	fmt.Fprintf(&sb, "- 当前价：%.2f %s\n", price, currency)
	fmt.Fprintf(&sb, "- 今日涨跌：%.2f%%\n", changePct)
	if v := mkt["volume"]; v != nil {
		fmt.Fprintf(&sb, "- 成交量：%.0f\n", asFloat(v))
	}
	if v := mkt["turnover_rate"]; v != nil {
		fmt.Fprintf(&sb, "- 换手率：%.2f%%\n", asFloat(v))
	}

	// 技术指标
	techValid := boolField(input.Technical, "_valid")
	if techValid {
		sb.WriteString("\n## 技术指标\n")
		fmt.Fprintf(&sb, "- MA 组合：MA5=%.2f / MA20=%.2f / MA60=%.2f / MA200=%.2f\n",
			asFloat(input.Technical["ma5"]),
			asFloat(input.Technical["ma20"]),
			asFloat(input.Technical["ma60"]),
			asFloat(input.Technical["ma200"]))
		maStatus, _ := input.Technical["ma_status"].(string)
		fmt.Fprintf(&sb, "- 均线排列状态：%s\n", maStatus)
		rsi14 := asFloat(input.Technical["rsi14"])
		rsi14Status, _ := input.Technical["rsi14_status"].(string)
		fmt.Fprintf(&sb, "- RSI14：%.2f（%s）\n", rsi14, rsi14Status)
		fmt.Fprintf(&sb, "- MACD：DIF=%.4f, DEA=%.4f, 柱状图=%.4f\n",
			asFloat(input.Technical["macd"]),
			asFloat(input.Technical["macd_signal"]),
			asFloat(input.Technical["macd_histogram"]))
		fmt.Fprintf(&sb, "- 布林带：上轨=%.2f / 中轨=%.2f / 下轨=%.2f\n",
			asFloat(input.Technical["bollinger_upper"]),
			asFloat(input.Technical["bollinger_middle"]),
			asFloat(input.Technical["bollinger_lower"]))
		fmt.Fprintf(&sb, "- 布林带宽：%.2f%% / %%B位置：%.2f\n",
			asFloat(input.Technical["bollinger_bandwidth"]),
			asFloat(input.Technical["bollinger_percent_b"]))
		fmt.Fprintf(&sb, "- 60日涨跌幅：%.2f%%\n", asFloat(input.Technical["change_pct_60d"]))
		fmt.Fprintf(&sb, "- 20日年化波动率：.2f%%\n", asFloat(input.Technical["volatility_20d"]))
		fmt.Fprintf(&sb, "- 均量比（MA5/MA20）：%.2f\n", asFloat(input.Technical["volume_ma5_to_ma20"]))
	} else {
		sb.WriteString("\n## 技术指标\n⚠️ 技术指标数据暂不可用（可能正在初始化计算），请仅从价格行为角度分析。\n")
	}

	// 基本面
	fundValid := boolField(input.Fundamentals, "_valid")
	if fundValid {
		sb.WriteString("\n## 基本面数据\n")
		mcapText, _ := input.Fundamentals["market_cap_text"].(string)
		fmt.Fprintf(&sb, "- 市值：%s\n", mcapText)
		fmt.Fprintf(&sb, "- 市盈率（PE TTM）：%.2f\n", asFloat(input.Fundamentals["pe_ttm"]))
		fmt.Fprintf(&sb, "- 市净率（PB）：%.2f\n", asFloat(input.Fundamentals["pb"]))
		peg := asFloat(input.Fundamentals["peg"])
		pegUnavailable := boolField(input.Fundamentals, "peg_unavailable")
		if pegUnavailable {
			sb.WriteString("- PEG 指数：N/A（增长率≤0 无法计算）\n")
		} else {
			fmt.Fprintf(&sb, "- PEG 指数：%.2f\n", peg)
		}
		fmt.Fprintf(&sb, "- 股息收益率：%.2f%%\n", asFloat(input.Fundamentals["dividend_yield"]))
		netProfitText, _ := input.Fundamentals["net_profit_text"].(string)
		fmt.Fprintf(&sb, "- 净利润：%s\n", netProfitText)
		revenueText, _ := input.Fundamentals["revenue_text"].(string)
		fmt.Fprintf(&sb, "- 营业收入：%s\n", revenueText)
		sharesText, _ := input.Fundamentals["shares_outstanding_text"].(string)
		fmt.Fprintf(&sb, "- 流通股：%s\n", sharesText)
	} else {
		sb.WriteString("\n## 基本面数据\n⚠️ 基本面数据暂不可用（新股或数据源异常），请侧重技术面分析。\n")
	}

	// 大盘环境
	mvValid := boolField(input.MarketOverview, "_valid")
	if mvValid {
		sb.WriteString("\n## 市场环境（大盘指数）\n")
		indexes, _ := input.MarketOverview["indexes"].([]any)
		for i, idxRaw := range indexes {
			idx, ok := idxRaw.(map[string]any)
			if !ok || i >= 3 {
				continue
			}
			iname, _ := idx["name"].(string)
			ilast := asFloat(idx["last"])
			ichg := asFloat(idx["change_pct"])
			fmt.Fprintf(&sb, "- %s：%.2f（%.2f%%）\n", iname, ilast, ichg)
		}
		trendSummary, _ := input.MarketOverview["trend_summary"].(string)
		if trendSummary != "" {
			fmt.Fprintf(&sb, "- 大盘整体态势：%s\n", trendSummary)
		}
	} else {
		sb.WriteString("\n## 市场环境\n⚠️ 大盘指数数据暂不可用，请仅从个股维度分析。\n")
	}

	// 用户持仓状态
	hasPos := boolField(input.Portfolio, "has_position")
	if hasPos {
		sb.WriteString("\n## 用户持仓状态\n")
		shares := asFloat(input.Portfolio["shares"])
		avgCost := asFloat(input.Portfolio["avg_cost_price"])
		buyDate, _ := input.Portfolio["buy_date"].(string)
		pnlText, _ := input.Portfolio["unrealized_pnl_text"].(string)
		pnlPct := asFloat(input.Portfolio["unrealized_pnl_pct"])
		fmt.Fprintf(&sb, "- 持仓数量：%.0f 股\n", shares)
		fmt.Fprintf(&sb, "- 买入均价：%.2f %s\n", avgCost, currency)
		if buyDate != "" {
			fmt.Fprintf(&sb, "- 买入日期：%s\n", buyDate)
		}
		fmt.Fprintf(&sb, "- 当前浮动盈亏：%s（%.2f%%）\n", pnlText, pnlPct)
	} else {
		sb.WriteString("\n## 用户持仓状态\n- 当前未持有该股票（首次买入视角）\n")
	}

	// 投资画像
	if profile != nil && profile.RiskPreference != "" {
		sb.WriteString("\n## 投资画像\n")
		capLabel := formatCapital(profile.TotalCapital)
		fmt.Fprintf(&sb, "- 风险偏好：%s\n", profile.RiskPreference)
		fmt.Fprintf(&sb, "- 投资目标：%s\n", profile.InvestmentGoal)
		fmt.Fprintf(&sb, "- 投资周期：%s\n", profile.InvestmentHorizon)
		fmt.Fprintf(&sb, "- 投资经验：%s\n", profile.ExperienceLevel)
		fmt.Fprintf(&sb, "- 最大回撤容忍：%.0f%%\n", profile.MaxDrawdownPct)
		fmt.Fprintf(&sb, "- 总资金量：%s\n", capLabel)
	} else {
		sb.WriteString("\n## 投资画像\n- 用户未设置投资画像，请给出通用型中等风险偏好建议\n")
	}

	sb.WriteString("\n---\n请基于以上全部数据输出 JSON 分析结果。")

	return sb.String()
}

// ── 后置校验 ──

func validateStockAnalysis(output *StockAnalysisInput, result *StockAnalysisOutput) []string {
	var warnings []string
	validSignals := map[string]bool{"buy": true, "sell": true, "hold": true}
	currentPrice := asFloat(output.Market["price"])

	if !validSignals[result.Signal] {
		result.Signal = "hold"
		warnings = append(warnings, "invalid_signal_fallback")
	}
	if result.ConfidenceScore < 0 || result.ConfidenceScore > 100 {
		result.ConfidenceScore = 50
		warnings = append(warnings, "confidence_clamped")
	}
	if result.ConfidenceLevel != "high" && result.ConfidenceLevel != "medium" && result.ConfidenceLevel != "low" {
		if result.ConfidenceScore >= 70 {
			result.ConfidenceLevel = "high"
		} else if result.ConfidenceScore >= 40 {
			result.ConfidenceLevel = "medium"
		} else {
			result.ConfidenceLevel = "low"
		}
		warnings = append(warnings, "confidence_level_fixed")
	}
	if len(result.RiskWarnings) == 0 {
		result.RiskWarnings = []string{"请关注市场波动风险"}
		warnings = append(warnings, "empty_risks_padded")
	}
	if result.DataTimestamp == "" {
		result.DataTimestamp = time.Now().UTC().Format(time.RFC3339)
	}

	// 价格合理性校验（如果 trading_suggestions 存在）
	if ts := result.TradingSuggestions; ts != nil {
		currency, _ := output.SymbolMeta["currency"].(string)
		if currency == "" {
			currency = "CNY"
		}
		entryZone, _ := ts["entry_zone"].(map[string]any)
		if entryZone != nil {
			low := asFloat(entryZone["low"])
			high := asFloat(entryZone["high"])
			if low <= 0 || high <= 0 || low > high {
				ts["entry_zone"] = map[string]any{"low": currentPrice * 0.97, "high": currentPrice * 1.03, "currency": currency}
				warnings = append(warnings, "entry_zone_sanitized")
			}
		}
		sl, _ := ts["stop_loss"].(map[string]any)
		if sl != nil {
			sp := asFloat(sl["price"])
			if sp <= 0 {
				ts["stop_loss"] = map[string]any{"price": currentPrice * 0.95, "pct": -5.0}
				warnings = append(warnings, "stop_loss_sanitized")
			}
		}
		tp, _ := ts["take_profit"].(map[string]any)
		if tp != nil {
			tpp := asFloat(tp["price"])
			if tpp <= 0 {
				ts["take_profit"] = map[string]any{"price": currentPrice * 1.08, "pct": 8.0}
				warnings = append(warnings, "take_profit_sanitized")
			}
		}
	}

	return warnings
}

// ── 数据完整性报告 ──

func buildDataCompleteness(input *StockAnalysisInput, profile *portfolio.InvestmentProfile) map[string]string {
	c := make(map[string]string)
	if mkt := input.Market; mkt != nil && mkt["price"] != nil {
		c["market"] = "complete"
	} else {
		c["market"] = "missing"
	}
	if boolField(input.Technical, "_valid") {
		c["technical"] = "complete"
	} else {
		c["technical"] = "missing"
	}
	if boolField(input.Fundamentals, "_valid") {
		c["fundamentals"] = "complete"
	} else {
		c["fundamentals"] = "missing"
	}
	if boolField(input.MarketOverview, "_valid") {
		c["market_overview"] = "complete"
	} else {
		c["market_overview"] = "missing"
	}
	if boolField(input.Portfolio, "has_position") {
		c["portfolio"] = "has_position"
	} else {
		c["portfolio"] = "no_position"
	}
	if profile != nil && profile.RiskPreference != "" {
		c["profile"] = "set"
	} else {
		c["profile"] = "not_set"
	}
	return c
}

// ── helpers ──

func boolField(m map[string]any, key string) bool {
	if m == nil {
		return false
	}
	v, ok := m[key]
	if !ok {
		return false
	}
	switch b := v.(type) {
	case bool:
		return b
	default:
		return false
	}
}

func formatCapital(cap float64) string {
	if cap <= 0 {
		return "未填写"
	}
	if cap >= 1e8 {
		return fmt.Sprintf("%.2f 亿", cap/1e8)
	}
	if cap >= 1e4 {
		return fmt.Sprintf("%.2f 万", cap/1e4)
	}
	return fmt.Sprintf("%.0f", cap)
}
