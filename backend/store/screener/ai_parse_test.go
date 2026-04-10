package screener

import (
	"strings"
	"testing"
)

// ── AIConfig.Enabled() ──

func TestAIConfig_Enabled(t *testing.T) {
	cases := []struct {
		key  string
		want bool
	}{
		{"", false},
		{" ", false},
		{"\t", false},
		{"sk-abc123", true},
		{"my-key", true},
	}
	for _, c := range cases {
		cfg := AIConfig{APIKey: c.key}
		got := cfg.Enabled()
		if got != c.want {
			t.Errorf("AIConfig{APIKey:%q}.Enabled() = %v, want %v", c.key, got, c.want)
		}
	}
}

// ── stripCodeFence ──

func TestStripCodeFence_PlainText(t *testing.T) {
	input := `{"industry": "中药", "filters": {}}`
	if got := stripCodeFence(input); got != input {
		t.Errorf("plain text should pass through unchanged, got: %s", got)
	}
}

func TestStripCodeFence_JsonBlock(t *testing.T) {
	input := "```json\n{\"key\": \"value\"}\n```"
	want := "{\"key\": \"value\"}"
	if got := stripCodeFence(input); got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestStripCodeFence_CodeBlock(t *testing.T) {
	input := "```\n{\"no_lang\": true}\n```"
	want := "{\"no_lang\": true}"
	if got := stripCodeFence(input); got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestStripCodeFence_WithWhitespace(t *testing.T) {
	input := "  ```json  \n  {\"x\":1}  \n  ```  "
	want := "{\"x\":1}"
	if got := stripCodeFence(input); got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestStripCodeFence_EmptyBlock(t *testing.T) {
	input := "```\n```"
	want := ""
	if got := stripCodeFence(input); got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestStripCodeFence_NoNewlineAfterOpen(t *testing.T) {
	input := "```json\ncontent only"
	// No closing ``` → should still strip opening line but keep rest
	got := stripCodeFence(input)
	if strings.Contains(got, "```") || !strings.Contains(got, "content only") {
		t.Errorf("unexpected result for unclosed fence: %q", got)
	}
}

// ── truncate ──

func TestTruncate_ShortString(t *testing.T) {
	input := "hello world"
	if got := truncate(input, 20); got != input {
		t.Errorf("short string should not be truncated, got: %q", got)
	}
}

func TestTruncate_ExactLength(t *testing.T) {
	input := "1234567890" // len=10
	if got := truncate(input, 10); got != input {
		t.Errorf("exact-length should be unchanged, got: %q", got)
	}
}

func TestTruncate_OverLimit(t *testing.T) {
	input := "123456789012345" // len=15
	got := truncate(input, 10)
	want := "1234567890..."
	if got != want {
		t.Errorf("got %q, want %q (len=%d)", got, want, len(got))
	}
}

func TestTruncate_ChineseChars(t *testing.T) {
	input := "这是一个很长的中文测试字符串用来验证截断逻辑是否正确工作"
	maxLen := 10
	got := truncate(input, maxLen)
	runes := []rune(got)
	if len(runes) > maxLen+3 { // content + "..."
		t.Errorf("truncated string too long: rune count=%d", len(runes))
	}
	if !strings.HasSuffix(got, "...") {
		t.Error("truncated string should end with ...")
	}
}

func TestTruncate_Empty(t *testing.T) {
	if got := truncate("", 10); got != "" {
		t.Errorf("empty should stay empty, got: %q", got)
	}
}
