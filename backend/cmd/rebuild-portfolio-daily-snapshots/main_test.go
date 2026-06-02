package main

import "testing"

func TestNormalizeScope(t *testing.T) {
	cases := map[string]string{
		"":        "ASHARE",
		"ashare":  "ASHARE",
		"sse":     "ASHARE",
		"szse":    "ASHARE",
		"hkex":    "HKEX",
		"hkshares": "HKEX",
	}
	for input, want := range cases {
		if got := normalizeScope(input); got != want {
			t.Fatalf("normalizeScope(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestParseOptionsRejectsInvalidScope(t *testing.T) {
	_, err := parseOptions([]string{"--scope", "US"})
	if err == nil {
		t.Fatal("expected invalid scope error")
	}
}
