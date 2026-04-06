package screener

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// ── AI 配置 ──────────────────────────────────────────────────

type AIConfig struct {
	APIKey  string
	BaseURL string
	Model   string
}

func (c AIConfig) Enabled() bool {
	return strings.TrimSpace(c.APIKey) != ""
}

// ── 解析结果 ────────────────────────────────────────────────

// AIFilterRange 表示单个筛选条件的 min/max 范围
type AIFilterRange struct {
	Min *float64 `json:"min"`
	Max *float64 `json:"max"`
}

// AIParseResult 是 LLM 返回的结构化筛选条件
type AIParseResult struct {
	Industry *string                  `json:"industry,omitempty"`
	Filters  map[string]AIFilterRange `json:"filters"`
	Summary  string                   `json:"summary"` // LLM 对理解的总结
}

// AIParseResponse 是给前端的完整响应
type AIParseResponse struct {
	Industry *string                  `json:"industry,omitempty"`
	Filters  map[string]AIFilterRange `json:"filters"`
	Summary  string                   `json:"summary"`
}

// ── System Prompt ──────────────────────────────────────────

const systemPrompt = `你是一个 A 股选股助手，既能处理精确的筛选条件，也能理解用户的口语化、模糊描述。你需要将用户的自然语言翻译为结构化的 JSON 筛选条件。

可用的筛选字段如下（key → 含义、单位）：

- industry: 行业名称（字符串，如「游戏」「中药」「IT服务」；优先返回用户常用名称，不必带 Ⅰ / Ⅱ / Ⅲ 后缀）
- price: 最新价（元）
- change_pct: 涨跌幅（%，如 5 表示 5%）
- total_mv: 总市值（元，如 20亿 = 2000000000）
- pe: PE 市盈率（动态，倍数）
- pb: PB 市净率（倍数）
- turnover_rate: 换手率（%）
- volume_ratio: 量比（无单位）
- amplitude: 振幅（%）
- turnover: 成交额（元，如 1亿 = 100000000）
- change_pct_60d: 60日涨幅（%）
- change_pct_ytd: 年初至今涨幅（%）
- float_mv: 流通市值（元，如 50亿 = 5000000000）
- profit_growth_rate: 利润增长率 / 净利润同比增长率（%，如 30 表示 30%）

你必须严格按以下 JSON 格式输出，不要输出任何其他内容：

{
  "industry": "行业名称或null",
  "filters": {
    "字段key": { "min": 数值或null, "max": 数值或null },
    ...
  },
  "summary": "一句话中文总结你理解的筛选意图"
}

## 基础规则
1. 只使用上面列出的字段 key；行业只能放在顶层 industry，不要塞进 filters，也不要创造新字段
2. min/max 如果用户没指定方向就用 null 表示不限
3. 总市值和流通市值的单位是「元」，用户说"50亿"你要转换为 5000000000
4. 成交额的单位是「元」，用户说"1亿"你要转换为 100000000
5. 涨跌幅、换手率、振幅、利润增长率等百分比字段，用户说"5%"你输出 5（不是 0.05）
6. 用户说"利润增长率、净利润增长率、净利润同比、利润同比增长"等，都优先映射到 profit_growth_rate
7. 用户说"游戏、中药、其他电子、IT服务、银行"等行业词时，优先提取到 industry；如果只出现一个明确行业，尽量返回该行业
8. 如果原始行业常带「Ⅱ / Ⅲ」等后缀，返回更自然的行业名即可，例如返回「中药」而不是「中药Ⅱ」
9. 如果用户一次说了多个不兼容行业，或行业表述太模糊无法确定，就把 industry 设为 null，并在 summary 中说明
10. summary 字段用中文，简洁地描述你理解的选股意图

## 投资概念 → 筛选条件映射指南

当用户使用模糊或口语化描述时，请根据以下常识进行合理推断：

### 盈利能力
- "挣钱的""赚钱的""盈利好的" → profit_growth_rate min:20 + pe min:0（排除亏损）
- "利润高的""高增长" → profit_growth_rate min:50
- "亏损的""不赚钱的" → pe max:0

### 估值
- "便宜的""低估值""被低估" → pe min:0 max:20 + pb max:2
- "贵的""高估值" → pe min:50
- "破净""低于净资产" → pb max:1

### 规模
- "蓝筹""大盘股""龙头" → total_mv min:50000000000（500亿）
- "小盘""小票" → total_mv max:5000000000（50亿）
- "中盘" → total_mv min:5000000000 max:50000000000

### 走势/动量
- "涨得好的""最近涨的""强势" → change_pct_60d min:10
- "跌了很多的""超跌""抄底" → change_pct_60d max:-20
- "稳定的""波动小的" → amplitude max:3
- "今天涨的" → change_pct min:1
- "涨停的" → change_pct min:9.8

### 活跃度
- "活跃的""热门的" → turnover_rate min:5 + volume_ratio min:2
- "冷门的" → turnover_rate max:1
- "放量的" → volume_ratio min:2

### 行业
- "科技股""科技" → industry 匹配「计算机」「电子」「通信」等（大类行业 → 选最接近的或设 null 在 summary 说明）
- "新能源" → industry 匹配「电力设备」相关
- "医药""医疗" → industry 匹配「医药」「医疗」相关
- "金融""银行" → industry 匹配具体子行业
- "消费""白酒" → industry 匹配「食品饮料」相关

### 常见组合场景
- "稳健的蓝筹股" → total_mv min:50000000000 + pe min:0 max:25 + amplitude max:5
- "高增长小盘股" → total_mv max:10000000000 + profit_growth_rate min:30
- "被低估的绩优股" → pe min:0 max:15 + profit_growth_rate min:20 + pb max:3
- "最近跌多了的好公司" → change_pct_60d max:-15 + profit_growth_rate min:10 + pe min:0

### 兜底规则
- 如果用户描述过于模糊（如"推荐几只""帮我选股"），设置温和的默认筛选：pe min:0（排除亏损）+ profit_growth_rate min:10（基本盈利增长），并在 summary 中解释你的推断逻辑
- 永远不要返回完全空的 filters，至少给一个合理的默认条件
- 无法理解的描述，也要尝试给出最接近的推断，在 summary 中说明不确定的部分`

// ── LLM 调用 ────────────────────────────────────────────────

type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type chatRequest struct {
	Model       string        `json:"model"`
	Messages    []chatMessage `json:"messages"`
	Temperature float64       `json:"temperature"`
	MaxTokens   int           `json:"max_tokens,omitempty"`
}

type chatChoice struct {
	Message chatMessage `json:"message"`
}

type chatResponse struct {
	Choices []chatChoice `json:"choices"`
	Error   *struct {
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

// ParseNaturalLanguage 调用 LLM 将自然语言翻译为结构化筛选条件
func ParseNaturalLanguage(ctx context.Context, cfg AIConfig, userInput string) (*AIParseResponse, error) {
	if !cfg.Enabled() {
		return nil, fmt.Errorf("%w: AI 选股功能未启用，请联系管理员配置 AI_API_KEY", ErrInvalid)
	}

	trimmed := strings.TrimSpace(userInput)
	if trimmed == "" {
		return nil, fmt.Errorf("%w: 请输入选股描述", ErrInvalid)
	}
	if len([]rune(trimmed)) > 500 {
		return nil, fmt.Errorf("%w: 输入内容过长，最多 500 字", ErrInvalid)
	}

	body := chatRequest{
		Model: cfg.Model,
		Messages: []chatMessage{
			{Role: "system", Content: systemPrompt},
			{Role: "user", Content: trimmed},
		},
		Temperature: 0.1,
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
		return nil, fmt.Errorf("AI 服务返回错误 (HTTP %d): %s", resp.StatusCode, truncate(string(respBody), 200))
	}

	var chatResp chatResponse
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
	// 处理可能的 markdown 代码块包裹
	content = stripCodeFence(content)

	var result AIParseResult
	if err := json.Unmarshal([]byte(content), &result); err != nil {
		return nil, fmt.Errorf("AI 返回内容格式不正确: %w", err)
	}

	// 过滤非法字段
	validKeys := map[string]bool{
		"price": true, "change_pct": true, "total_mv": true, "pe": true,
		"pb": true, "turnover_rate": true, "volume_ratio": true, "amplitude": true,
		"turnover": true, "change_pct_60d": true, "change_pct_ytd": true, "float_mv": true,
		"profit_growth_rate": true,
	}
	cleaned := make(map[string]AIFilterRange)
	for k, v := range result.Filters {
		if validKeys[k] && (v.Min != nil || v.Max != nil) {
			cleaned[k] = v
		}
	}

	return &AIParseResponse{
		Filters: cleaned,
		Summary: result.Summary,
	}, nil
}

// ── helpers ─────────────────────────────────────────────────

func stripCodeFence(s string) string {
	s = strings.TrimSpace(s)
	if strings.HasPrefix(s, "```") {
		// 去掉第一行（```json 或 ```）
		idx := strings.Index(s, "\n")
		if idx > 0 {
			s = s[idx+1:]
		}
		// 去掉末尾 ```
		if strings.HasSuffix(s, "```") {
			s = s[:len(s)-3]
		}
		s = strings.TrimSpace(s)
	}
	return s
}

func truncate(s string, maxLen int) string {
	runes := []rune(s)
	if len(runes) <= maxLen {
		return s
	}
	return string(runes[:maxLen]) + "..."
}
