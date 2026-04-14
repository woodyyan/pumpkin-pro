package screener

import (
	"strings"
	"testing"
)

// ── BuildSystemPrompt ──────────────────────────────────────

func TestBuildSystemPrompt_AShare(t *testing.T) {
	prompt := BuildSystemPrompt("ASHARE")
	if prompt == "" {
		t.Fatal("ASHARE prompt should not be empty")
	}
	// A股 prompt 应包含 A 股特征关键词
	checks := []struct {
		name     string
		contains string
	}{
		{"A 股选股助手", "A 股选股助手"},
		{"元单位", "（元"},
		{"涨停概念", "涨停"},
		{"industry 字段", "industry"},
		{"profit_growth_rate", "profit_growth_rate"},
	}
	for _, c := range checks {
		if !strings.Contains(prompt, c.contains) {
			t.Errorf("ASHARE prompt missing %q", c.name)
		}
	}
}

func TestBuildSystemPrompt_HKEX(t *testing.T) {
	prompt := BuildSystemPrompt("HKEX")
	if prompt == "" {
		t.Fatal("HKEX prompt should not be empty")
	}
	// 港股 prompt 应包含港股特征关键词
	checks := []struct {
		name     string
		contains string
	}{
		{"港股选股助手", "港股选股助手"},
		{"HKD 单位", "HKD"},
		{"港币", "港币"},
		{"无 industry 字段", "\"filters\""}, // JSON 模板只有 filters，无 industry 顶层字段
		{"无 profit_growth_rate 字段", "排除亏损"}, // 港股盈利映射不含 pgr
		{"蓝筹量级不同", "1000亿"}, // 港股蓝筹门槛更高
		{"无涨停机制", "没有涨停"},
	}
	for _, c := range checks {
		if !strings.Contains(prompt, c.contains) {
			t.Errorf("HKEX prompt missing %q (got substring: %q)", c.name, c.contains)
		}
	}
}

func TestBuildSystemPrompt_DefaultFallback(t *testing.T) {
	// 空字符串或未知 exchange 应回退到 A 股 prompt
	prompt := BuildSystemPrompt("")
	if !strings.Contains(prompt, "A 股选股助手") {
		t.Error("empty exchange should fallback to ASHARE prompt")
	}

	prompt2 := BuildSystemPrompt("UNKNOWN")
	if !strings.Contains(prompt2, "A 股选股助手") {
		t.Error("unknown exchange should fallback to ASHARE prompt")
	}
}

// ── Prompt 结构差异验证 ────────────────────────────────────

func TestBuildSystemPrompt_HKEX_NoIndustryField(t *testing.T) {
	hk := BuildSystemPrompt("HKEX")
	as := BuildSystemPrompt("ASHARE")

	// HKEX 的 JSON 模板不应包含顶层 industry 字段
	hkJsonTemplate := extractJSONTemplate(hk)
	if strings.Contains(hkJsonTemplate, `"industry"`) {
		t.Error("HKEX JSON template should NOT include top-level 'industry' field")
	}

	// ASHARE 的 JSON 模板应包含顶层 industry 字段
	asJsonTemplate := extractJSONTemplate(as)
	if !strings.Contains(asJsonTemplate, `"industry"`) {
		t.Error("ASHARE JSON template SHOULD include top-level 'industry' field")
	}
}

func TestBuildSystemPrompt_HKEX_NoProfitGrowthRate(t *testing.T) {
	hk := BuildSystemPrompt("HKEX")
	if strings.Contains(hk, "profit_growth_rate") {
		t.Error("HKEX prompt should NOT contain profit_growth_rate")
	}
}

func TestBuildSystemPrompt_HKEX_NoLimitUpDown(t *testing.T) {
	hk := BuildSystemPrompt("HKEX")
	// 港股不应有涨停/跌停的映射规则（如 → change_pct min:9.8）
	// 注意：prompt 可能提到"没有涨停机制"，这是说明性文字，不是映射规则
	// 正常涨跌映射（如 change_pct min:1）是允许的
	if strings.Contains(hk, "9.8") || strings.Contains(hk, "涨停的") {
		t.Error("HKEX prompt should NOT contain limit-up specific mappings")
	}
}

func TestBuildSystemPrompt_HKEX_CurrencyUnits(t *testing.T) {
	hk := BuildSystemPrompt("HKEX")
	// 港股金额单位应该是 HKD/港币，不应是「元」
	if strings.Count(hk, "（元") > 0 && !strings.Contains(hk, "注意") {
		t.Error("HKEX prompt should use HKD/港币, not 元 for amounts")
	}
	// 应该有港币相关说明
	if !strings.Contains(hk, "港币") && !strings.Contains(hk, "HKD") {
		t.Error("HKEX prompt must mention HKD/港币 currency")
	}
}

func TestBuildSystemPrompt_HKEX_MarketCapScale(t *testing.T) {
	hk := BuildSystemPrompt("HKEX")
	// 港股蓝筹门槛应该比 A 股高（腾讯、汇丰级别 ~1000亿+ HKD）
	if !strings.Contains(hk, "100000000000") {
		t.Error("HKEX blue-chip threshold should reflect HK market cap scale (~100B+ HKD)")
	}
}

// ── helpers for test ───────────────────────────────────────

// extractJSONTemplate 从 prompt 中提取 JSON 示例部分（在 ``` 或 { 之后）
func extractJSONTemplate(prompt string) string {
	start := strings.Index(prompt, "{")
	if start == -1 {
		return ""
	}
	// 找到对应的 }（简化处理，取第一段）
	end := strings.Index(prompt[start:], "\n}")
	if end == -1 {
		return prompt[start:]
	}
	return prompt[start : start+end+1]
}
