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
	Filters map[string]AIFilterRange `json:"filters"`
	Summary string                   `json:"summary"` // LLM 对理解的总结
}

// AIParseResponse 是给前端的完整响应
type AIParseResponse struct {
	Filters map[string]AIFilterRange `json:"filters"`
	Summary string                   `json:"summary"`
}

// ── System Prompt ──────────────────────────────────────────

const systemPrompt = `你是一个 A 股选股助手。用户会用自然语言描述选股条件，你需要将其翻译为结构化的 JSON 筛选条件。

可用的筛选字段如下（key → 含义、单位）：

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

你必须严格按以下 JSON 格式输出，不要输出任何其他内容：

{
  "filters": {
    "字段key": { "min": 数值或null, "max": 数值或null },
    ...
  },
  "summary": "一句话中文总结你理解的筛选意图"
}

规则：
1. 只使用上面列出的字段 key，不要创造新字段
2. min/max 如果用户没指定方向就用 null 表示不限
3. 总市值和流通市值的单位是「元」，用户说"50亿"你要转换为 5000000000
4. 成交额的单位是「元」，用户说"1亿"你要转换为 100000000
5. 涨跌幅、换手率、振幅等百分比字段，用户说"5%"你输出 5（不是 0.05）
6. 如果用户的描述你无法理解或不涉及任何筛选条件，返回空 filters 并在 summary 中说明
7. summary 字段用中文，简洁地描述你理解的选股意图`

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
