package live

import (
	"fmt"
	"strings"
)

// NormalizeSymbol normalises a stock code for both HK and A-share markets.
//
// Accepted inputs and their outputs:
//
//	"00700"      → "00700.HK", "HKEX"
//	"00700.HK"   → "00700.HK", "HKEX"
//	"600519"     → "600519.SH", "SSE"
//	"600519.SH"  → "600519.SH", "SSE"
//	"000001"     → "000001.SZ", "SZSE"
//	"000001.SZ"  → "000001.SZ", "SZSE"
//	"300750"     → "300750.SZ", "SZSE"
//	"300750.SZ"  → "300750.SZ", "SZSE"
func NormalizeSymbol(input string) (symbol string, exchange string, err error) {
	raw := strings.ToUpper(strings.TrimSpace(input))
	if raw == "" {
		return "", "", ErrInvalidSymbol
	}

	// Strip known prefixes / suffixes that users might paste.
	raw = strings.TrimPrefix(raw, "HK")
	raw = strings.TrimSuffix(raw, ".HK")
	raw = strings.TrimSuffix(raw, ".SH")
	raw = strings.TrimSuffix(raw, ".SZ")

	if raw == "" {
		return "", "", ErrInvalidSymbol
	}

	// After stripping, raw should be pure digits.
	for _, ch := range raw {
		if ch < '0' || ch > '9' {
			return "", "", ErrInvalidSymbol
		}
	}

	switch len(raw) {
	case 5:
		// HK stock: 5 digits → pad to 5 is already done by length
		return raw + ".HK", "HKEX", nil

	case 6:
		// A-share: detect exchange by first digit.
		first := raw[0]
		switch {
		case first == '6':
			return raw + ".SH", "SSE", nil
		case first == '0' || first == '3':
			return raw + ".SZ", "SZSE", nil
		default:
			return "", "", fmt.Errorf("%w: 无法识别 A 股交易所，代码首位为 %c", ErrInvalidSymbol, first)
		}

	default:
		// Attempt zero-pad to 5 for HK short codes (e.g. "700" → "00700").
		if len(raw) < 5 {
			raw = fmt.Sprintf("%05s", raw)
			return raw + ".HK", "HKEX", nil
		}
		return "", "", fmt.Errorf("%w: 股票代码长度不合法（港股 5 位 / A 股 6 位）", ErrInvalidSymbol)
	}
}

// ExchangeFromSymbol extracts the exchange from an already-normalised symbol
// such as "00700.HK" or "600519.SH".
func ExchangeFromSymbol(symbol string) string {
	upper := strings.ToUpper(symbol)
	switch {
	case strings.HasSuffix(upper, ".HK"):
		return "HKEX"
	case strings.HasSuffix(upper, ".SH"):
		return "SSE"
	case strings.HasSuffix(upper, ".SZ"):
		return "SZSE"
	default:
		return ""
	}
}

// IsAShare returns true if the normalised symbol belongs to an A-share exchange.
func IsAShare(symbol string) bool {
	ex := ExchangeFromSymbol(symbol)
	return ex == "SSE" || ex == "SZSE"
}
