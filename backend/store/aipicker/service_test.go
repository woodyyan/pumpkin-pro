package aipicker

import "testing"

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
