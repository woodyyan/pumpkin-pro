package aipicker

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/woodyyan/pumpkin-pro/backend/store/factorlab"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestExtractJSONObject(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"plain", `{"a":1}`, `{"a":1}`},
		{"with_prefix_suffix", "以下是结果：\n{\"a\":1}\n谢谢", `{"a":1}`},
		{"nested", `{"a":{"b":2},"c":3}`, `{"a":{"b":2},"c":3}`},
		{"brace_in_string", `{"reason":"含有 } 符号"}`, `{"reason":"含有 } 符号"}`},
		{"escaped_quote_in_string", `{"reason":"他说\"买入\""}`, `{"reason":"他说\"买入\""}`},
		{"truncated", `{"a":1,"b":`, ""},
		{"no_object", `hello world`, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := extractJSONObject(c.in); got != c.want {
				t.Fatalf("extractJSONObject(%q) = %q, want %q", c.in, got, c.want)
			}
		})
	}
}

func TestValidateResponseNormalizesWeightsAndFallbacks(t *testing.T) {
	c1, c2, c3, c4 := 91.2, 88.4, 83.5, 79.1
	candidates := []Candidate{
		{Code: "600519", Symbol: "600519.SH", Name: "贵州茅台", Industry: "白酒", ClosePrice: 100, CompositeScore: &c1},
		{Code: "000333", Symbol: "000333.SZ", Name: "美的集团", Industry: "家电", ClosePrice: 50, CompositeScore: &c2},
		{Code: "300750", Symbol: "300750.SZ", Name: "宁德时代", Industry: "电池", ClosePrice: 200, CompositeScore: &c3},
		{Code: "601318", Symbol: "601318.SH", Name: "中国平安", Industry: "保险", ClosePrice: 60, CompositeScore: &c4},
	}
	result := &PickerResponse{Analysis: &AnalysisPayload{Picks: []PickItem{{Rank: 1, Code: "600519", PositionPct: 50}, {Rank: 2, Code: "000333", PositionPct: 30}, {Rank: 3, Code: "300750", PositionPct: 20}}}}
	warnings := validateResponse(result, candidates, "2026-06-14", TriggerManual, "test-model")
	if len(result.Analysis.Picks) != 4 {
		t.Fatalf("expected 4 picks, got %d", len(result.Analysis.Picks))
	}
	if result.Analysis.PortfolioAllocation.TotalPositionPct > 80 {
		t.Fatalf("expected total position <= 80, got %d", result.Analysis.PortfolioAllocation.TotalPositionPct)
	}
	if result.Analysis.PortfolioAllocation.CashReservePct != 100-result.Analysis.PortfolioAllocation.TotalPositionPct {
		t.Fatalf("cash reserve mismatch")
	}
	if result.Analysis.SelectionBasis != SelectionBasisFactorLab {
		t.Fatalf("unexpected selection basis: %s", result.Analysis.SelectionBasis)
	}
	if len(warnings) == 0 {
		t.Fatalf("expected normalization warnings")
	}
}

func TestRepositoryGetLatestDailyResultPrefersManualWhenNewer(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file::memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := db.AutoMigrate(&DailyResult{}, &GenerateLogRecord{}); err != nil {
		t.Fatalf("migrate db: %v", err)
	}
	repo := NewRepository(db)
	ctx := context.Background()
	if err := db.Create(&DailyResult{Market: MarketAShare, TradeDate: "2026-06-15", Trigger: TriggerDailyAuto, SnapshotDate: "2026-06-14", SelectionBasis: SelectionBasisFactorLab, Model: "auto", PayloadJSON: `{"analysis":{"snapshot_date":"2026-06-14"}}`}).Error; err != nil {
		t.Fatalf("save auto: %v", err)
	}
	if err := db.Create(&DailyResult{Market: MarketAShare, TradeDate: "2026-06-16", Trigger: TriggerManual, SnapshotDate: "2026-06-15", SelectionBasis: SelectionBasisFactorLab, Model: "manual", PayloadJSON: `{"analysis":{"snapshot_date":"2026-06-15"}}`}).Error; err != nil {
		t.Fatalf("save manual: %v", err)
	}
	item, err := repo.GetLatestDailyResult(ctx, MarketAShare)
	if err != nil {
		t.Fatalf("get latest: %v", err)
	}
	if item.Trigger != TriggerManual {
		t.Fatalf("expected manual result, got %s", item.Trigger)
	}
	if item.TradeDate != "2026-06-16" {
		t.Fatalf("expected latest trade date, got %s", item.TradeDate)
	}
}

func TestBuildCandidatePoolKeepsCandidatesWhenTechnicalSnapshotsMissing(t *testing.T) {
	c1, c2 := 91.0, 88.0
	service := &Service{}
	pool := []FactorCandidate{
		{FactorScreenerItem: factorlab.FactorScreenerItem{Code: "600519", Symbol: "600519.SH", Name: "贵州茅台", Industry: "白酒", ClosePrice: 100, CompositeScore: &c1, QualityScore: &c1}, HitFactors: map[string]struct{}{"quality": {}}},
		{FactorScreenerItem: factorlab.FactorScreenerItem{Code: "000333", Symbol: "000333.SZ", Name: "美的集团", Industry: "家电", ClosePrice: 50, CompositeScore: &c2, MomentumScore: &c2}, HitFactors: map[string]struct{}{"momentum": {}}},
	}
	candidates := make([]Candidate, 0, len(pool))
	for _, item := range pool {
		candidates = append(candidates, buildCandidate(item, TechnicalSnapshot{}, false))
	}
	stats := technicalCoverageStats{Total: len(pool), WithTechnicalData: 0}
	if len(candidates) != 2 {
		t.Fatalf("expected 2 candidates, got %d", len(candidates))
	}
	for _, candidate := range candidates {
		if candidate.TechnicalDataComplete {
			t.Fatalf("expected missing technical data for %s", candidate.Code)
		}
		if candidate.ChangePct20D != nil || candidate.RSI14 != nil || candidate.DistanceToMA20Pct != nil {
			t.Fatalf("expected nil technical metrics for %s", candidate.Code)
		}
	}
	summary := buildMarketSummary(candidates, stats)
	if !strings.Contains(summary, "缺少技术快照") {
		t.Fatalf("expected summary to mention missing technical data, got %s", summary)
	}
	prompt := buildUserPrompt("2026-06-15", TriggerManual, summary, stats, candidates)
	if !strings.Contains(prompt, "部分技术指标缺失") {
		t.Fatalf("expected prompt to mention partial technical coverage, got %s", prompt)
	}
	if !strings.Contains(prompt, "technical_status=missing") {
		t.Fatalf("expected prompt to mark missing technical status, got %s", prompt)
	}
	_ = service
}

func TestBuildCandidateIncludesTechnicalMetricsWhenAvailable(t *testing.T) {
	c1 := 91.0
	candidate := buildCandidate(
		FactorCandidate{FactorScreenerItem: factorlab.FactorScreenerItem{Code: "600519", Symbol: "600519.SH", Name: "贵州茅台", Industry: "白酒", ClosePrice: 100, CompositeScore: &c1, QualityScore: &c1}, HitFactors: map[string]struct{}{"quality": {}}},
		TechnicalSnapshot{TechTagsJSON: `["站上MA20"]`, ChangePct20D: 12.3, DistanceToMA20Pct: 5.6, DistanceToMA60Pct: 8.9, RSI14: 66.6, Volatility20D: 18.1, VolumeMA5ToMA20: 1.25},
		true,
	)
	if !candidate.TechnicalDataComplete {
		t.Fatalf("expected technical data complete")
	}
	if candidate.ChangePct20D == nil || *candidate.ChangePct20D != 12.3 {
		t.Fatalf("expected change pct to be populated, got %+v", candidate.ChangePct20D)
	}
	if len(candidate.TechnicalTags) != 1 || candidate.TechnicalTags[0] != "站上MA20" {
		t.Fatalf("unexpected technical tags: %+v", candidate.TechnicalTags)
	}
}

func TestAdminGenerateStatusReturnsLogs(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file::memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := db.AutoMigrate(&DailyResult{}, &GenerateLogRecord{}, &GenerateTraceRecord{}); err != nil {
		t.Fatalf("migrate db: %v", err)
	}
	repo := NewRepository(db)
	service := &Service{repo: repo}
	ctx := context.Background()
	if err := db.Create(&DailyResult{Market: MarketAShare, TradeDate: "2026-06-16", Trigger: TriggerManual, SnapshotDate: "2026-06-15", SelectionBasis: SelectionBasisFactorLab, Model: "manual", PayloadJSON: `{"analysis":{"snapshot_date":"2026-06-15"}}`}).Error; err != nil {
		t.Fatalf("save result: %v", err)
	}
	if err := repo.SaveGenerateLog(ctx, GenerateLogRecord{TradeDate: "2026-06-16", Trigger: TriggerManual, Status: GenerateLogStatusFailed, Message: "boom", Model: "manual"}); err != nil {
		t.Fatalf("save log: %v", err)
	}
	status, err := service.AdminGenerateStatus(ctx)
	if err != nil {
		t.Fatalf("admin status: %v", err)
	}
	if status.LatestResult == nil {
		t.Fatalf("expected latest result")
	}
	if status.LatestLog == nil || status.LatestLog.Message != "boom" {
		t.Fatalf("expected latest log message boom, got %+v", status.LatestLog)
	}
	if len(status.Logs) != 1 {
		t.Fatalf("expected 1 log, got %d", len(status.Logs))
	}
}

func TestAdminGenerateStatusLimitsToTenLogs(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file::memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := db.AutoMigrate(&DailyResult{}, &GenerateLogRecord{}, &GenerateTraceRecord{}); err != nil {
		t.Fatalf("migrate db: %v", err)
	}
	repo := NewRepository(db)
	service := &Service{repo: repo}
	ctx := context.Background()
	for i := 0; i < 12; i++ {
		record := GenerateLogRecord{
			TradeDate: fmt.Sprintf("2026-06-%02d", i+1),
			Trigger:   TriggerManual,
			Status:    GenerateLogStatusSuccess,
			Message:   fmt.Sprintf("log-%d", i),
			Model:     "manual",
		}
		if _, err := repo.CreateGenerateLog(ctx, record); err != nil {
			t.Fatalf("create log %d: %v", i, err)
		}
	}
	status, err := service.AdminGenerateStatus(ctx)
	if err != nil {
		t.Fatalf("admin status: %v", err)
	}
	if len(status.Logs) != 10 {
		t.Fatalf("expected 10 logs, got %d", len(status.Logs))
	}
	if status.Logs[0].Message != "log-11" {
		t.Fatalf("expected latest log to be log-11, got %q", status.Logs[0].Message)
	}
}

func TestAdminLatestGenerateRunReturnsTrace(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file::memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := db.AutoMigrate(&DailyResult{}, &GenerateLogRecord{}, &GenerateTraceRecord{}); err != nil {
		t.Fatalf("migrate db: %v", err)
	}
	repo := NewRepository(db)
	service := &Service{repo: repo}
	ctx := context.Background()
	logRecord, err := repo.CreateGenerateLog(ctx, GenerateLogRecord{TradeDate: "2026-06-16", Trigger: TriggerManual, Status: GenerateLogStatusSuccess, Message: "ok", Model: "manual"})
	if err != nil {
		t.Fatalf("create log: %v", err)
	}
	if err := repo.SaveGenerateTrace(ctx, GenerateTraceRecord{GenerateLogID: logRecord.ID, SystemPrompt: "system", UserPrompt: "user", AssistantReasoning: "reasoning", AssistantContent: "content"}); err != nil {
		t.Fatalf("save trace: %v", err)
	}
	run, err := service.AdminLatestGenerateRun(ctx)
	if err != nil {
		t.Fatalf("latest run: %v", err)
	}
	if run.LatestLog == nil || run.LatestLog.ID != logRecord.ID {
		t.Fatalf("expected latest log %d, got %+v", logRecord.ID, run.LatestLog)
	}
	if run.Trace == nil || run.Trace.AssistantContent != "content" {
		t.Fatalf("expected trace content, got %+v", run.Trace)
	}
}

// TestCallLLMNoMaxTokensInRequest 验证请求体中不含 max_tokens 字段。
func TestCallLLMNoMaxTokensInRequest(t *testing.T) {
	var capturedBody []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedBody = make([]byte, r.ContentLength)
		_, _ = r.Body.Read(capturedBody)
		r.Body.Close()
		// 返回一个合法的最小 JSON 响应（finish_reason=stop）
		resp := `{"choices":[{"message":{"role":"assistant","content":"{\"analysis\":{\"picks\":[]}}"},"finish_reason":"stop"}],"usage":{"prompt_tokens":100,"completion_tokens":200,"total_tokens":300}}`
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, resp)
	}))
	defer srv.Close()

	cfg := AIConfig{APIKey: "test", BaseURL: srv.URL, Model: "test-model"}
	_, meta, _, err := callLLM(context.Background(), cfg, "user1", "test prompt")
	if err != nil && !strings.Contains(err.Error(), "JSON") {
		// 允许 JSON 解析失败（picks 为空会触发 validate 错误），此处只关心请求结构
		t.Logf("callLLM returned (expected for minimal payload): %v", err)
	}

	// 核心断言：请求体不含 max_tokens
	var reqMap map[string]any
	if jsonErr := json.Unmarshal(capturedBody, &reqMap); jsonErr != nil {
		t.Fatalf("failed to parse captured request body: %v", jsonErr)
	}
	if _, exists := reqMap["max_tokens"]; exists {
		t.Fatalf("request body must NOT contain max_tokens, got: %v", reqMap)
	}

	// meta 不应为 nil（即使 callLLM 失败，在 length 之外的场景 meta 也只在解析到 choice 后才非 nil）
	_ = meta
}

// TestCallLLMFinishReasonLengthFails 验证 finish_reason=length 时不重试，直接失败并返回可观测 meta。
func TestCallLLMFinishReasonLengthFails(t *testing.T) {
	callCount := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		resp := `{"choices":[{"message":{"role":"assistant","content":"{\"analysis\":"},"finish_reason":"length"}],"usage":{"prompt_tokens":50,"completion_tokens":4096,"total_tokens":4146}}`
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, resp)
	}))
	defer srv.Close()

	cfg := AIConfig{APIKey: "test", BaseURL: srv.URL, Model: "test-model"}
	_, meta, trace, err := callLLM(context.Background(), cfg, "user1", "test prompt")

	if err == nil {
		t.Fatal("expected error for finish_reason=length, got nil")
	}
	if !strings.Contains(err.Error(), "finish_reason=length") {
		t.Fatalf("expected error to mention finish_reason=length, got: %v", err)
	}
	// finish_reason=length 不应重试 —— 只应发出 1 次请求
	if callCount != 1 {
		t.Fatalf("expected exactly 1 attempt (no retry for length), got %d", callCount)
	}
	// meta 应包含可观测信息
	if meta == nil {
		t.Fatal("expected non-nil llmCallMeta on finish_reason=length")
	}
	if meta.FinishReason != "length" {
		t.Fatalf("expected meta.FinishReason=length, got %q", meta.FinishReason)
	}
	if meta.CompletionTokens != 4096 {
		t.Fatalf("expected meta.CompletionTokens=4096, got %d", meta.CompletionTokens)
	}
	if trace == nil || !strings.Contains(trace.AssistantContent, `{"analysis":`) {
		t.Fatalf("expected trace content to be captured, got %+v", trace)
	}
}

// TestCallLLMTransientErrorRetries 验证瞬时网络错误（5xx）仍会重试一次。
func TestCallLLMTransientErrorRetries(t *testing.T) {
	callCount := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		if callCount == 1 {
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = fmt.Fprint(w, `{"error":"temporary"}`)
			return
		}
		resp := `{"choices":[{"message":{"role":"assistant","content":"{\"analysis\":{\"picks\":[]}}"},"finish_reason":"stop"}],"usage":{"prompt_tokens":100,"completion_tokens":200,"total_tokens":300}}`
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, resp)
	}))
	defer srv.Close()

	cfg := AIConfig{APIKey: "test", BaseURL: srv.URL, Model: "test-model"}
	_, _, _, _ = callLLM(context.Background(), cfg, "user1", "test prompt")

	if callCount < 2 {
		t.Fatalf("expected retry on 5xx, got only %d call(s)", callCount)
	}
}

// TestGenerateLogRecordContainsLLMFields 验证 GenerateLogRecord 新增字段能正常写入和读取。
func TestGenerateLogRecordContainsLLMFields(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file::memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := db.AutoMigrate(&DailyResult{}, &GenerateLogRecord{}, &GenerateTraceRecord{}); err != nil {
		t.Fatalf("migrate db: %v", err)
	}
	repo := NewRepository(db)
	ctx := context.Background()
	record := GenerateLogRecord{
		TradeDate:        "2026-06-16",
		Trigger:          TriggerManual,
		Status:           GenerateLogStatusFailed,
		Message:          "output truncated (finish_reason=length, completion_tokens=4096)",
		Model:            "test-model",
		FinishReason:     "length",
		PromptChars:      28000,
		CompletionTokens: 4096,
		TimeoutSeconds:   240,
		ResponseMS:       35000,
	}
	if err := repo.SaveGenerateLog(ctx, record); err != nil {
		t.Fatalf("save log: %v", err)
	}
	got, err := repo.GetLatestGenerateLog(ctx)
	if err != nil {
		t.Fatalf("get log: %v", err)
	}
	if got.FinishReason != "length" {
		t.Errorf("expected FinishReason=length, got %q", got.FinishReason)
	}
	if got.PromptChars != 28000 {
		t.Errorf("expected PromptChars=28000, got %d", got.PromptChars)
	}
	if got.CompletionTokens != 4096 {
		t.Errorf("expected CompletionTokens=4096, got %d", got.CompletionTokens)
	}
	if got.TimeoutSeconds != 240 {
		t.Errorf("expected TimeoutSeconds=240, got %d", got.TimeoutSeconds)
	}
	if got.ResponseMS != 35000 {
		t.Errorf("expected ResponseMS=35000, got %d", got.ResponseMS)
	}
}

func TestFlattenAIMessageField(t *testing.T) {
	value := []any{
		map[string]any{"text": "first"},
		map[string]any{"content": []any{map[string]any{"text": "second"}}},
	}
	got := flattenAIMessageField(value)
	if got != "first\n\nsecond" {
		t.Fatalf("unexpected flattened content: %q", got)
	}
}

func TestCallLLMCapturesReasoningField(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := `{"choices":[{"message":{"role":"assistant","content":"{\"analysis\":{\"picks\":[]}}","reasoning_content":[{"text":"step-1"},{"text":"step-2"}]},"finish_reason":"stop"}],"usage":{"prompt_tokens":100,"completion_tokens":200,"total_tokens":300}}`
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, resp)
	}))
	defer srv.Close()

	cfg := AIConfig{APIKey: "test", BaseURL: srv.URL, Model: "test-model"}
	_, _, trace, err := callLLM(context.Background(), cfg, "user1", "test prompt")
	if err != nil && !strings.Contains(err.Error(), "JSON") {
		t.Fatalf("callLLM returned unexpected error: %v", err)
	}
	if trace == nil {
		t.Fatal("expected non-nil trace")
	}
	if trace.AssistantReasoning != "step-1\n\nstep-2" {
		t.Fatalf("expected reasoning to be flattened, got %q", trace.AssistantReasoning)
	}
}
