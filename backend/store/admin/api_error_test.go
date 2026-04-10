package admin

import (
	"strings"
	"testing"
)

// ── SanitizeQueryString ──

func TestSanitizeQueryString_Empty(t *testing.T) {
	if got := SanitizeQueryString(""); got != "" {
		t.Errorf("empty → %q, want empty", got)
	}
}

func TestSanitizeQueryString_NoSensitive(t *testing.T) {
	input := "page=1&limit=20&sort=desc"
	if got := SanitizeQueryString(input); got != input {
		t.Errorf("safe params should pass through: %q", got)
	}
}

func TestSanitizeQueryString_StripsToken(t *testing.T) {
	input := "page=1&token=abc123&sort=desc"
	got := SanitizeQueryString(input)
	if contains(got, "token") || contains(got, "abc123") {
		t.Errorf("should strip token param: %q", got)
	}
	if !contains(got, "page=1") || !contains(got, "sort=desc") {
		t.Error("should keep safe params")
	}
}

func TestSanitizeQueryString_MultipleSensitive(t *testing.T) {
	input := "access_token=xyz&password=secret&id=5"
	got := SanitizeQueryString(input)
	sensitiveWords := []string{"token", "password", "secret"}
	for _, w := range sensitiveWords {
		if containsCaseInsensitive(got, w) {
			t.Errorf("should not contain sensitive word %q in: %q", w, got)
		}
	}
	if !contains(got, "id=5") {
		t.Error("should keep id=5")
	}
}

func TestSanitizeQueryString_SensitiveKeys(t *testing.T) {
	keys := []string{
		"token", "password", "passwd", "secret", "key",
		"api_key", "access_token", "refresh_token",
		"authorization", "credential",
	}
	for _, k := range keys {
		input := k + "=sensitive_val&page=1"
		got := SanitizeQueryString(input)
		if containsCaseInsensitive(got, k) {
			t.Errorf("should strip key %q from query string, got: %q", k, got)
		}
		if !contains(got, "page=1") {
			t.Errorf("should keep page param after stripping %q", k)
		}
	}
}

func TestSanitizeQueryString_CaseInsensitiveKey(t *testing.T) {
	input := "TOKEN=upper_token&Password=MyPass"
	got := SanitizeQueryString(input)
	if containsCaseInsensitive(got, "token") || containsCaseInsensitive(got, "password") {
		t.Errorf("case-insensitive strip failed: %q", got)
	}
}

func TestSanitizeQueryString_NoValue(t *testing.T) {
	input := "page=&limit="
	got := SanitizeQueryString(input)
	if !contains(got, "page=") || !contains(got, "limit=") {
		t.Error("should keep empty-value params")
	}
}

// ── NormalizePath ──

func TestNormalizePath_TrailingSlash(t *testing.T) {
	cases := []struct {
		input string
		want  string
	}{
		{"/api/live/snapshots/", "/api/live/snapshots"},
		{"/api/users/", "/api/users"},
		{"/api/users", "/api/users"},
		{"", ""},
		{"/", ""},
		{"/api/", "/api"},
	}
	for _, c := range cases {
		got := NormalizePath(c.input)
		if got != c.want {
			t.Errorf("NormalizePath(%q) = %q, want %q", c.input, got, c.want)
		}
	}
}

func TestNormalizePath_Whitespace(t *testing.T) {
	cases := []struct {
		input string
		want  string
	}{
		{" /api/ ", "/api"},
		{"  ", ""},
	}
	for _, c := range cases {
		got := NormalizePath(c.input)
		if got != c.want {
			t.Errorf("NormalizePath(%q) = %q, want %q", c.input, got, c.want)
		}
	}
}

// ── APIErrorLogItem truncation (from GetSystemHealthStats logic) ──

func TestErrorMessageTruncation_Logic(t *testing.T) {
	// Replicate the service-level truncation logic for testing
	longMsg := ""
	for i := 0; i < 300; i++ {
		longMsg += "x"
	}
	runes := []rune(longMsg)
	if len(runes) > 200 {
		truncated := string(runes[:200]) + "…"
		if len([]rune(truncated)) != 201 { // 200 + ellipsis
			t.Errorf("truncated message rune count = %d, expected 201", len([]rune(truncated)))
		}
	}
}

// ── Helpers ──

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsHelper(s, substr))
}

func containsHelper(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

func containsCaseInsensitive(s, sub string) bool {
	return containsLower(strings.ToLower(s), strings.ToLower(sub))
}

func containsLower(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
