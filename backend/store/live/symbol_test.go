package live

import (
	"strings"
	"testing"
)

// ── NormalizeSymbol ──

func TestNormalizeSymbol_HK5Digit(t *testing.T) {
	sym, ex, err := NormalizeSymbol("00700")
	if err != nil || sym != "00700.HK" || ex != "HKEX" {
		t.Errorf("00700 → got (%s, %s, %v), want (00700.HK, HKEX, nil)", sym, ex, err)
	}
}

func TestNormalizeSymbol_HKWithSuffix(t *testing.T) {
	sym, ex, err := NormalizeSymbol("00700.HK")
	if err != nil || sym != "00700.HK" || ex != "HKEX" {
		t.Errorf("00700.HK → got (%s, %s, %v)", sym, ex, err)
	}
}

func TestNormalizeSymbol_HKWithPrefix(t *testing.T) {
	sym, _, err := NormalizeSymbol("HK00700")
	if err != nil || sym != "00700.HK" {
		t.Errorf("HK00700 → got (%s, %v), want 00700.HK", sym, err)
	}
}

func TestNormalizeSymbol_HKShortCode(t *testing.T) {
	// Short code < 5 digits should zero-pad
	sym, ex, err := NormalizeSymbol("700")
	if err != nil || sym != "00700.HK" || ex != "HKEX" {
		t.Errorf("700 → got (%s, %s, %v), want (00700.HK, HKEX, nil)", sym, ex, err)
	}
}

func TestNormalizeSymbol_AShareSSE(t *testing.T) {
	sym, ex, err := NormalizeSymbol("600519")
	if err != nil || sym != "600519.SH" || ex != "SSE" {
		t.Errorf("600519 → got (%s, %s, %v)", sym, ex, err)
	}
}

func TestNormalizeSymbol_AShareSZSE(t *testing.T) {
	cases := []struct {
		input string
		want  string
	}{
		{"000001", "000001.SZ"},
		{"300750", "300750.SZ"},
		{"001234", "001234.SZ"}, // 0-prefix → SZ
		{"399001", "399001.SZ"}, // 3-prefix → SZ
	}
	for _, c := range cases {
		got, ex, err := NormalizeSymbol(c.input)
		if err != nil || got != c.want || ex != "SZSE" {
			t.Errorf("%s → got (%s, %s, %v), want (%s, SZSE, nil)", c.input, got, ex, err, c.want)
		}
	}
}

func TestNormalizeSymbol_WithExistingSuffix(t *testing.T) {
	cases := []struct {
		input   string
		wantSym  string
		wantEx  string
	}{
		{"600519.SH", "600519.SH", "SSE"},
		{"000001.SZ", "000001.SZ", "SZSE"},
		{"00700.HK", "00700.HK", "HKEX"},
	}
	for _, c := range cases {
		got, ex, err := NormalizeSymbol(c.input)
		if err != nil || got != c.wantSym || ex != c.wantEx {
			t.Errorf("%s → got (%s, %s, %v)", c.input, got, ex, err)
		}
	}
}

func TestNormalizeSymbol_Empty(t *testing.T) {
	_, _, err := NormalizeSymbol("")
	if err == nil {
		t.Error("empty input should return error")
	}
}

func TestNormalizeSymbol_WhitespaceOnly(t *testing.T) {
	_, _, err := NormalizeSymbol("   ")
	if err == nil {
		t.Error("whitespace-only should return error")
	}
}

func TestNormalizeSymbol_InvalidChars(t *testing.T) {
	_, _, err := NormalizeSymbol("ABC123")
	if err == nil {
		t.Error("non-digit chars should return error")
	}
}

func TestNormalizeSymbol_TooLong(t *testing.T) {
	// 7 digits — not valid for HK or A-share
	_, _, err := NormalizeSymbol("1234567")
	if err == nil {
		t.Error("7 digits should be invalid")
	}
}

func TestNormalizeSymbol_CaseInsensitive(t *testing.T) {
	sym, _, err := NormalizeSymbol("00700.hk")
	if err != nil || !strings.HasSuffix(sym, ".HK") {
		t.Errorf("lowercase .hk should work, got %s", sym)
	}
}

// ── ExchangeFromSymbol ──

func TestExchangeFromSymbol(t *testing.T) {
	cases := []struct {
		input string
		want  string
	}{
		{"00700.HK", "HKEX"},
		{"600519.SH", "SSE"},
		{"000001.SZ", "SZSE"},
		{"unknown.XX", ""},
		{"nosuffix", ""},
	}
	for _, c := range cases {
		got := ExchangeFromSymbol(c.input)
		if got != c.want {
			t.Errorf("ExchangeFromSymbol(%q) = %q, want %q", c.input, got, c.want)
		}
	}
}

// ── IsAShare ──

func TestIsAShare(t *testing.T) {
	if !IsAShare("600519.SH") { t.Error("SSE should be A-share") }
	if !IsAShare("000001.SZ") { t.Error("SZSE should be A-share") }
	if IsAShare("00700.HK") { t.Error("HK should NOT be A-share") }
	if IsAShare("unknown") { t.Error("unknown should NOT be A-share") }
}
