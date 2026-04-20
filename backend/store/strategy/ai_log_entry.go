package strategy

// ── AI 调用日志入口（供所有 AI 函数使用，通过回调写入 DB）──

// AIUsage 记录模型返回的 token usage 信息。
type AIUsage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

func (u AIUsage) Normalized() AIUsage {
	if u.PromptTokens < 0 {
		u.PromptTokens = 0
	}
	if u.CompletionTokens < 0 {
		u.CompletionTokens = 0
	}
	if u.TotalTokens < 0 {
		u.TotalTokens = 0
	}
	if u.TotalTokens == 0 {
		u.TotalTokens = u.PromptTokens + u.CompletionTokens
	}
	return u
}

// AILogEntry 构造日志条目的输入（各 AI 函数构造后传给 LogAICall）
type AILogEntry struct {
	UserID           string
	FeatureKey       string // stock_analysis / strategy_generate / strategy_iterate / backtest_optimize / screener_parse
	FeatureName      string // 中文功能名
	Status           string // "success" 或 "error"
	ErrorMessage     string
	Model            string
	ResponseMS       int
	PromptTokens     int
	CompletionTokens int
	TotalTokens      int
	ExtraMeta        map[string]any
}

func (e *AILogEntry) ApplyUsage(usage AIUsage) {
	normalized := usage.Normalized()
	e.PromptTokens = normalized.PromptTokens
	e.CompletionTokens = normalized.CompletionTokens
	e.TotalTokens = normalized.TotalTokens
}

// logWriteFn 是回调函数指针，由 main.go 在 InitAILogger 后注入
var logWriteFn func(userID, featureKey, featureName, status, errMsg, model string, responseMs, promptTokens, completionTokens, totalTokens int, extraMeta map[string]any)

// SetAILogWriter 注入写入回调（main.go 启动时调用一次）
func SetAILogWriter(fn func(userID, featureKey, featureName, status, errMsg, model string, responseMs, promptTokens, completionTokens, totalTokens int, extraMeta map[string]any)) {
	logWriteFn = fn
}

// LogAICall 异步记录一次 AI 调用（非阻塞，可安全从任意 goroutine 调用）
func LogAICall(entry AILogEntry) {
	if logWriteFn == nil {
		return
	}
	logWriteFn(
		entry.UserID,
		entry.FeatureKey,
		entry.FeatureName,
		entry.Status,
		entry.ErrorMessage,
		entry.Model,
		entry.ResponseMS,
		entry.PromptTokens,
		entry.CompletionTokens,
		entry.TotalTokens,
		entry.ExtraMeta,
	)
}
