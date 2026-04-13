package main

import (
	"testing"
)

// ── normalizeWatchlistCode 测试 ──
// 覆盖港股/A股/边界情况，确保 handleQuadrant 中关注列表代码与 DB 存储一致

func TestNormalizeWatchlistCode_HKStocks(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		// 港股核心用例（对应 DB 存储 5 位）
		{"腾讯 00700", "00700.HK", "00700"},
		{"小米 01810", "01810.HK", "01810"},
		{"美团 03690", "03690.HK", "03690"},
		{"阿里巴巴 09988", "09988.HK", "09988"},

		// 无 .HK 后缀时按 A 股逻辑处理（前端传入的 symbol 始终带后缀，此处为防御）
		{"腾讯无后缀(走A股)", "00700", "000700"},     // 无.HK → 走A股6位
		{"纯数字5位(走A股)", "12345", "012345"},      // 无.HK → 走A股6位

		// A 股标准用例（DB 存储 6 位）
		{"平安银行 000001", "000001", "000001"},
		{"招商银行 600036", "600036", "600036"},
		{"贵州茅台 600519", "600519", "600519"},

		// 边界情况
		{"空字符串", "", "000000"},                    // 空串 → TrimLeft得"" → 补零到6位
		{"全零", "0", "000000"}, // A 股补零到 6 位
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			isHK := len(tt.input) > 3 && (len(tt.input) >= 4 && tt.input[len(tt.input)-3:] == ".HK")
			got := normalizeWatchlistCode(tt.input, isHK)
			if got != tt.expected {
				t.Errorf("normalizeWatchlistCode(%q, %v) = %q; want %q",
					tt.input, isHK, got, tt.expected)
			}
		})
	}
}

func TestNormalizeWatchlistCode_HKvsADiff(t *testing.T) {
	// 验证同一个数字输入在 isHK=true/false 下结果不同
	input := "00700"

	hkResult := normalizeWatchlistCode(input+".HK", true)
	aResult := normalizeWatchlistCode(input, false)

	if hkResult == aResult {
		t.Errorf("HK(%q)=%q should differ from A-share(%q)=%q for same numeric input",
			input, hkResult, input, aResult)
	}
	if hkResult != "00700" {
		t.Errorf("HK result expected '00700', got '%s'", hkResult)
	}
	// A股逻辑会把 00700 → 700 → 000700（这是正确的行为——A股就是6位）
	if aResult != "000700" {
		t.Errorf("A-share result expected '000700', got '%s'", aResult)
	}
}

func TestNormalizeWatchlistCode_LeadingZeros(t *testing.T) {
	// 确保前导零处理在各种位数下都正确
	tests := []struct {
		input    string
		isHK     bool
		expected string
	}{
		{"1.HK", true, "00001"},       // 最小港股 1 位 → 补到 5
		{"12.HK", true, "00012"},      // 2 位
		{"123.HK", true, "00123"},     // 3 位
		{"1234.HK", true, "01234"},    // 4 位
		{"12345.HK", true, "12345"},   // 刚好 5 位，不变
		{"123456.HK", true, "123456"}, // 超过 5 位，不截断

		{"1", false, "000001"},        // A股最小 1 位 → 补到 6
		{"12", false, "000012"},       // 2 位
		{"123", false, "000123"},      // 3 位
		{"1234", false, "001234"},     // 4 位
		{"12345", false, "012345"},   // 5 位 → 补到 6
		{"123456", false, "123456"},   // 刚好 6 位，不变
		{"1234567", false, "1234567"}, // 超过 6 位，不截断
	}

	for _, tt := range tests {
		name := tt.input
		if tt.isHK {
			name += "-hk"
		} else {
			name += "-a"
		}
		t.Run(name, func(t *testing.T) {
			got := normalizeWatchlistCode(tt.input, tt.isHK)
			if got != tt.expected {
				t.Errorf("got '%s', want '%s'", got, tt.expected)
			}
		})
	}
}
