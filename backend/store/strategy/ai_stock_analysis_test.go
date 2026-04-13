package strategy

import (
	"strings"
	"testing"
)

// ── Helper ──

func makeStockInput() *StockAnalysisInput {
	return &StockAnalysisInput{
		SymbolMeta: map[string]any{
			"symbol":   "600519",
			"name":     "贵州茅台",
			"exchange": "SSE",
			"currency": "CNY",
		},
		Market:       map[string]any{},
		Technical:    map[string]any{},
		Fundamentals: map[string]any{},
		MarketOverview: map[string]any{
			"_valid": true,
			"indexes": []any{
				map[string]any{"name": "上证指数", "last": 3100.0, "change_pct": 0.5},
			},
			"trend_summary": "震荡上行",
		},
		Portfolio: map[string]any{"_valid": false},
	}
}

// 完整 market 数据（happy path）
func makeFullMarket() map[string]any {
	return map[string]any{
		"price":         1800.50,
		"change_pct":    1.23,
		"volume":        123456,
		"turnover_rate": 3.45,
		"open":          1790.00,
		"high":          1810.00,
		"low":           1785.00,
	}
}

// 完整 fundamentals 数据（happy path）
func makeFullFund() map[string]any {
	return map[string]any{
		"_valid":                true,
		"market_cap_text":       "2.25万亿",
		"pe_ttm":                28.5,
		"pb":                    8.50,
		"peg":                   1.20,
		"dividend_yield":        2.50,
		"net_profit_text":       "净利润：780亿",
		"revenue_text":         "营业收入：1500亿",
		"shares_outstanding_text": "12.5亿股",
	}
}

// 完整 technical 数据
func makeFullTechnical() map[string]any {
	return map[string]any{
		"_valid":                  true,
		"ma5":                     1750.0,
		"ma20":                    1720.0,
		"ma60":                    1700.0,
		"ma200":                   1650.0,
		"ma_status":               "多头排列",
		"rsi14":                   58.0,
		"rsi14_status":            "中性偏强",
		"macd":                    15.0,
		"macd_signal":            10.0,
		"macd_histogram":         5.0,
		"bollinger_upper":        1850.0,
		"bollinger_middle":       1780.0,
		"bollinger_lower":        1710.0,
		"bollinger_bandwidth":     8.0,
		"bollinger_percent_b":     60.0,
		"change_pct_60d":          12.5,
		"volatility_20d":          22.0,
		"volume_ma5_to_ma20":      1.3,
	}
}

// ── T1-T4: Market 区缺失值处理 ──

func TestBuildStockUserPrompt_Market_FullData(t *testing.T) {
	input := makeStockInput()
	input.Market = makeFullMarket()
	input.Fundamentals = makeFullFund()
	input.Technical = makeFullTechnical()

	prompt := buildStockUserPrompt(input, nil)

	for _, expected := range []string{
		"成交量：123456",
		"换手率：3.45%",
		"开盘价：1790.00",
		"最高价：1810.00",
		"最低价：1785.00",
	} {
		if !strings.Contains(prompt, expected) {
			t.Errorf("expected %q in prompt (full data)", expected)
		}
	}
}

func TestBuildStockUserPrompt_Market_TurnoverRate_Null(t *testing.T) {
	input := makeStockInput()
	input.Market = map[string]any{
		"price":      1800.50,
		"change_pct": 1.23,
		"volume":     100000,
		// turnover_rate 缺失（nil）
		"open": 1790.00,
		"high": 1810.00,
		"low":  1785.00,
	}
	input.Fundamentals = makeFullFund()

	prompt := buildStockUserPrompt(input, nil)

	if strings.Contains(prompt, "换手率：") {
		t.Error("missing turnover_rate must NOT produce any 换手率 line")
	}
	// 其他字段应正常出现
	if !strings.Contains(prompt, "成交量：100000") {
		t.Error("expected volume line present")
	}
}

func TestBuildStockUserPrompt_Market_Volume_Null(t *testing.T) {
	input := makeStockInput()
	input.Market = map[string]any{
		"price":         1800.50,
		"change_pct":    1.23,
		// volume 缺失
		"turnover_rate": 3.45,
		"open":          1790.00,
		"high":          1810.00,
		"low":           1785.00,
	}
	input.Fundamentals = makeFullFund()

	prompt := buildStockUserPrompt(input, nil)

	if strings.Contains(prompt, "成交量：") {
		t.Error("missing volume must NOT produce any 成交量 line")
	}
	if !strings.Contains(prompt, "换手率：3.45%") {
		t.Error("expected turnover rate line present")
	}
}

func TestBuildStockUserPrompt_Market_OHL_Null(t *testing.T) {
	input := makeStockInput()
	input.Market = map[string]any{
		"price":      1800.50,
		"change_pct": 1.23,
		"volume":     100000,
		// open/high/low 全部缺失
	}
	input.Fundamentals = makeFullFund()

	prompt := buildStockUserPrompt(input, nil)

	for _, forbidden := range []string{"开盘价：", "最高价：", "最低价："} {
		if strings.Contains(prompt, forbidden) {
			t.Errorf("missing O/H/L must NOT produce %q line", forbidden)
		}
	}
	// 非缺失字段应正常出现
	if !strings.Contains(prompt, "成交量：100000") {
		t.Error("expected volume line present")
	}
}

// ── T5-T8: Fundamentals 单字段 unavailable ──

func TestBuildStockUserPrompt_Fundamentals_PE_Unavailable(t *testing.T) {
	input := makeStockInput()
	input.Market = makeFullMarket()
	fund := makeFullFund()
	fund["pe_ttm"] = "N/A"
	fund["pe_unavailable"] = true
	input.Fundamentals = fund

	prompt := buildStockUserPrompt(input, nil)

	if strings.Contains(prompt, "PE TTM：0.00") {
		t.Error("unavailable PE must NOT render as 0.00")
	}
	if strings.Contains(prompt, "PE TTM：N/A") && !strings.Contains(prompt, "PE TTM）：暂无数据") {
		// 如果走的是 else 分支，asFloat("N/A")=0 会输出 PE TTM：0.00 — 已被上面拦截
	} else if !strings.Contains(prompt, "PE TTM）：暂无数据") {
		t.Error("expected 'PE TTM）：暂无数据' when pe_unavailable=true")
	}
	// PB 正常不受影响 — 源码格式为 "PB）：8.50" （全角右括号+全角冒号）
	if !strings.Contains(prompt, "PB）：8.50") {
		t.Error("PB should render normally")
	}
}

func TestBuildStockUserPrompt_Fundamentals_PB_Unavailable(t *testing.T) {
	input := makeStockInput()
	input.Market = makeFullMarket()
	fund := makeFullFund()
	fund["pb"] = "N/A"
	fund["pb_unavailable"] = true
	input.Fundamentals = fund

	prompt := buildStockUserPrompt(input, nil)

	if strings.Contains(prompt, "PB：0.00") {
		t.Error("unavailable PB must NOT render as 0.00")
	}
	if !strings.Contains(prompt, "PB）：暂无数据") {
		t.Error("expected 'PB）：暂无数据' when pb_unavailable=true")
	}
	// PE 正常 — 源码格式为 "PE TTM）：28.50"（全角右括号+全角冒号）
	if !strings.Contains(prompt, "PE TTM）：28.50") {
		t.Error("PE should render normally")
	}
}

func TestBuildStockUserPrompt_Fundamentals_DivYield_Unavailable(t *testing.T) {
	input := makeStockInput()
	input.Market = makeFullMarket()
	fund := makeFullFund()
	fund["dividend_yield"] = -1 // 负数表示不可用
	fund["div_yield_unavailable"] = true
	input.Fundamentals = fund

	prompt := buildStockUserPrompt(input, nil)

	if strings.Contains(prompt, "股息收益率：-1.00%") || strings.Contains(prompt, "股息收益率：0.00%") {
		t.Error("unavailable div yield must NOT render as numeric value")
	}
	if !strings.Contains(prompt, "股息收益率：暂无数据") {
		t.Error("expected '股息收益率：暂无数据'")
	}
}

func TestBuildStockUserPrompt_Fundamentals_AllPresent(t *testing.T) {
	input := makeStockInput()
	input.Market = makeFullMarket()
	input.Fundamentals = makeFullFund()

	prompt := buildStockUserPrompt(input, nil)

	// 注意：源码 fmt 格式为 "市盈率（PE TTM）%.2f" — 输出为 "PE TTM）：28.50"
	// （全角右括号 U+FF09 + 全角冒号 U+FF1A + 数值）
	// 而 unavailable 分支用的是 "市盈率（PE TTM）：暂无数据" — 同样有 ）：前缀
	// 这是已有格式不一致，本次修复保持原样不动
	for _, expected := range []string{
		"PE TTM）：28.50",
		"PB）：8.50",
		"PEG 指数：1.20",       // PEG 有冒号（原始格式）
		"股息收益率：2.50%",    // 股息率有冒号（原始格式）
	} {
		if !strings.Contains(prompt, expected) {
			t.Errorf("expected %q in prompt (all present)", expected)
		}
	}
	// 不应出现"暂无数据"
	if strings.Contains(prompt, "暂无数据") {
		t.Error("'暂无数据' must not appear when all data is present")
	}
}

// ── T9: PEG 现有模式回归验证 ──

func TestBuildStockUserPrompt_Fundamentals_PEG_Unavailable(t *testing.T) {
	input := makeStockInput()
	input.Market = makeFullMarket()
	fund := makeFullFund()
	fund["peg"] = "N/A"
	fund["peg_unavailable"] = true
	input.Fundamentals = fund

	prompt := buildStockUserPrompt(input, nil)

	// PEG 应显示已有的 N/A 模式
	if !strings.Contains(prompt, "PEG 指数：N/A") {
		t.Error("PEG unavailable must show 'N/A' pattern")
	}
	if !strings.Contains(prompt, "增长率≤0 无法计算") {
		t.Error("PEG unavailable reason text missing")
	}
}

// ── T10: Technical _valid=false ──

func TestBuildStockUserPrompt_Technical_Invalid(t *testing.T) {
	input := makeStockInput()
	input.Market = makeFullMarket()
	input.Fundamentals = makeFullFund()
	input.Technical = map[string]any{"_valid": false}

	prompt := buildStockUserPrompt(input, nil)

	if !strings.Contains(prompt, "⚠️ 技术指标数据暂不可用") {
		t.Error("invalid technical must show warning placeholder")
	}
	// 不应有具体技术指标数值
	if strings.Contains(prompt, "MA5=") || strings.Contains(prompt, "RSI14") {
		t.Error("technical values must not appear when _valid=false")
	}
}

// ── T11: 大面积缺失 ──

func TestBuildStockUserPrompt_MassiveMissing_MarketPlusFund(t *testing.T) {
	input := makeStockInput()
	// Market 只有 price 和 change_pct
	input.Market = map[string]any{
		"price":      1800.50,
		"change_pct": -2.30,
	}
	// Fundamentals 多个 unavailable
	input.Fundamentals = map[string]any{
		"_valid":              true,
		"market_cap_text":     "未知",
		"pe_ttm":              0,
		"pe_unavailable":      true,
		"pb":                  0,
		"pb_unavailable":      true,
		"peg":                 "N/A",
		"peg_unavailable":     true,
		"dividend_yield":      0,
		"div_yield_unavailable": true,
		"net_profit_text":     "暂无",
		"revenue_text":       "暂无",
		"shares_outstanding_text": "暂无",
	}
	// Technical 无效
	input.Technical = map[string]any{"_valid": false}

	prompt := buildStockUserPrompt(input, nil)

	// 验证多处"暂无数据"/"⚠️"
	placeholderCount := strings.Count(prompt, "暂无数据")
	if placeholderCount < 3 { // PE+PB+股息率至少3个
		t.Errorf("expected multiple '暂无数据' placeholders, got %d", placeholderCount)
	}
	if !strings.Contains(prompt, "⚠️") {
		t.Error("expected at least one warning emoji for technical/fund invalid sections")
	}
}

// ── T12: Happy Path 完整数据 ──

func TestBuildStockUserPrompt_HappyPath_CompleteData(t *testing.T) {
	input := makeStockInput()
	input.Market = makeFullMarket()
	input.Fundamentals = makeFullFund()
	input.Technical = makeFullTechnical()

	prompt := buildStockUserPrompt(input, nil)

	// 无"暂无数据"
	if strings.Contains(prompt, "暂无数据") {
		t.Error("no '暂无数据' should appear in happy path")
	}
	// 无"⚠️"
	if strings.Contains(prompt, "⚠️") {
		t.Error("no warning emoji should appear in happy path")
	}
	// 关键数值都应存在
	for _, expected := range []string{
		"当前价：1800.50 CNY",
		"今日涨跌：1.23%",
		"成交量：123456",
		"换手率：3.45%",
		"开盘价：1790.00",
		"PE TTM）：28.50",   // 源码格式：全角右括号+全角冒号
		"PB）：8.50",         // 源码格式：全角右括号+全角冒号
		"MA5=1750.00",
		"RSI14：58.00",
		"贵州茅台",
		"600519",
	} {
		if !strings.Contains(prompt, expected) {
			t.Errorf("happy path missing: %q", expected)
		}
	}
}

// ── 边界情况补充 ──

func TestBuildStockUserPrompt_Market_ZeroVolumeIsValid(t *testing.T) {
	input := makeStockInput()
	input.Market = map[string]any{
		"price":         1800.50,
		"change_pct":    0.0,
		"volume":        0,    // 停牌时 volume=0 是合法值
		"turnover_rate": 0,    // 同理
		"open":          1800.50,
		"high":          1800.50,
		"low":           1800.50,
	}
	input.Fundamentals = makeFullFund()

	prompt := buildStockUserPrompt(input, nil)

	// 0 是合法值（非 nil），应正常输出为 0
	if !strings.Contains(prompt, "成交量：0") {
		t.Error("zero volume (suspended stock) should render as 0")
	}
	if !strings.Contains(prompt, "换手率：0.00%") {
		t.Error("zero turnover rate should render as 0.00%%")
	}
}

func TestBuildStockUserPrompt_Fundamentals_AllUnavailable(t *testing.T) {
	input := makeStockInput()
	input.Market = makeFullMarket()
	input.Fundamentals = map[string]any{
		"_valid":              true,
		"market_cap_text":     "未知",
		"pe_ttm":              "N/A",
		"pe_unavailable":      true,
		"pb":                  "N/A",
		"pb_unavailable":      true,
		"peg":                 "N/A",
		"peg_unavailable":     true,
		"dividend_yield":      "N/A",
		"div_yield_unavailable": true,
		"net_profit_text":     "暂无",
		"revenue_text":       "暂无",
		"shares_outstanding_text": "暂无",
	}

	prompt := buildStockUserPrompt(input, nil)

	// 四个估值指标全部应为占位文本
	// unavailable 分支格式为 "PE TTM）：暂无数据"（全角右括号+全角冒号）
	for _, expected := range []string{
		"PE TTM）：暂无数据",
		"PB）：暂无数据",
		"PEG 指数：N/A",
		"股息收益率：暂无数据",
	} {
		if !strings.Contains(prompt, expected) {
			t.Errorf("all-unavailable: expected %q", expected)
		}
	}
}
