package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// ── parseBearerToken ──

func TestParseBearerToken(t *testing.T) {
	tests := []struct {
		name   string
		input  string
		expect string
	}{
		{"empty", "", ""},
		{"whitespace only", "   ", ""},
		{"no bearer prefix", "some-token", ""},
		{"bearer lowercase", "Bearer abc123", "abc123"},
		{"bearer mixed case", "BEARER token-xyz", "token-xyz"},
		{"bearer with extra spaces", "  bearer   my-token  ", "my-token"},
		{"bearer empty after", "bearer ", ""},
		{"complex token", "Bearer eyJhbGciOiJIUzI1NiJ9.abc.def", "eyJhbGciOiJIUzI1NiJ9.abc.def"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := parseBearerToken(tc.input)
			if got != tc.expect {
				t.Errorf("parseBearerToken(%q) = %q; want %q", tc.input, got, tc.expect)
			}
		})
	}
}

// ── parseLimit ──

func TestParseLimit(t *testing.T) {
	tests := []struct {
		name   string
		input  string
		expect int
	}{
		{"empty string", "", 20},
		{"valid number", "50", 50},
		{"zero", "0", 20},
		{"negative", "-5", 20},
		{"over max", "999", 200},
		{"exactly max", "200", 200},
		{"non-number", "abc", 20},
		{"with whitespace", "  30  ", 30},
		{"one", "1", 1},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := parseLimit(tc.input, 20)
			if got != tc.expect {
				t.Errorf("parseLimit(%q, 20) = %d; want %d", tc.input, got, tc.expect)
			}
		})
	}
}

// ── parseOffset ──

func TestParseOffset(t *testing.T) {
	tests := []struct {
		name   string
		input  string
		expect int
	}{
		{"empty", "", 0},
		{"zero", "0", 0},
		{"positive", "10", 10},
		{"negative", "-5", 0},
		{"non-number", "abc", 0},
		{"large", "10000", 10000},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := parseOffset(tc.input, 0)
			if got != tc.expect {
				t.Errorf("parseOffset(%q, 0) = %d; want %d", tc.input, got, tc.expect)
			}
		})
	}
}

// ── parseSince ──

func TestParseSince(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  bool // want non-zero time?
	}{
		{"empty returns zero", "", false},
		{"RFC3339 valid", "2024-06-15T10:00:00Z", true},
		{"invalid format", "not-a-date", false},
		{"garbage", "2024-13-99", false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := parseSince(tc.input)
			isZero := got.IsZero()
			if isZero == tc.want {
				t.Errorf("parseSince(%q).IsZero() = %v; opposite expected", tc.input, isZero)
			}
		})
	}
}

// ── parseLookbackDays ──

func TestParseLookbackDays(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		fallback int
		wantVal  int
		wantErr  bool
	}{
		{"uses fallback on empty", "", 60, 60, false},
		{"valid positive", "120", 60, 120, false},
		{"zero returns error", "0", 60, 0, true},
		{"negative returns error", "-5", 60, 0, true},
		{"non-number returns error", "abc", 60, 0, true},
		{"valid with different fallback", "30", 90, 30, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := parseLookbackDays(tc.input, tc.fallback)
			if err != nil != tc.wantErr {
				t.Errorf("parseLookbackDays(%q, %d) error = %v; wantErr %v", tc.input, tc.fallback, err, tc.wantErr)
			}
			if got != tc.wantVal {
				t.Errorf("parseLookbackDays(%q, %d) = %d; want %d", tc.input, tc.fallback, got, tc.wantVal)
			}
		})
	}
}

// ── parseWindowMinutes ──

func TestParseWindowMinutes(t *testing.T) {
	tests := []struct {
		name   string
		input  string
		expect int
	}{
		{"empty uses fallback", "", 60},
		{"valid", "120", 120},
		{"zero uses fallback", "0", 60},
		{"negative uses fallback", "-10", 60},
		{"over max capped", "999", 240},
		{"exactly max", "240", 240},
		{"non-number uses fallback", "abc", 60},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := parseWindowMinutes(tc.input, 60)
			if got != tc.expect {
				t.Errorf("parseWindowMinutes(%q, 60) = %d; want %d", tc.input, got, tc.expect)
			}
		})
	}
}

// ── splitCSV ──

func TestSplitCSV(t *testing.T) {
	tests := []struct {
		name   string
		input  string
		expect []string
	}{
		{"empty", "", nil},
		{"single value", "foo", []string{"foo"}},
		{"two values", "a,b,c", []string{"a", "b", "c"}},
		{"spaces trimmed", " x , y , z ", []string{"x", "y", "z"}},
		{"empty items skipped", "a,,b,,c", []string{"a", "b", "c"}},
		{"whitespace-only items skipped", "a,  , b", []string{"a", "b"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := splitCSV(tc.input)
			if len(got) != len(tc.expect) {
				t.Fatalf("splitCSV(%q) = %v; want %v", tc.input, got, tc.expect)
			}
			for i := range got {
				if got[i] != tc.expect[i] {
					t.Errorf("splitCSV(%q)[%d] = %q; want %q", tc.input, i, got[i], tc.expect[i])
				}
			}
		})
	}
}

// ── asMap ──

func TestAsMap(t *testing.T) {
	t.Run("nil returns empty map", func(t *testing.T) {
		got := asMap(nil)
		if len(got) != 0 {
			t.Errorf("asMap(nil) = %v; want empty", got)
		}
	})
	t.Run("actual map returns as-is", func(t *testing.T) {
		m := map[string]any{"key": "val"}
		got := asMap(m)
		if got["key"] != "val" {
			t.Errorf("asMap(map)[key] = %v; want val", got["key"])
		}
	})
	t.Run("non-map type returns empty map", func(t *testing.T) {
		got := asMap("hello")
		if len(got) != 0 {
			t.Errorf("asMap(string) = %v; want empty", got)
		}
	})
}

// ── asString ──

func TestAsString(t *testing.T) {
	tests := []struct {
		name   string
		input  any
		expect string
	}{
		{"string", "hello", "hello"},
		{"trimmed", "  world  ", "world"},
		{"nil", nil, ""},
		{"integer", 42, ""},
		{"map", map[string]any{}, ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := asString(tc.input)
			if got != tc.expect {
				t.Errorf("asString(%v) = %q; want %q", tc.input, got, tc.expect)
			}
		})
	}
}

// ── asBoolPtr ──

func TestAsBoolPtr(t *testing.T) {
	t.Run("nil input", func(t *testing.T) {
		if got := asBoolPtr(nil); got != nil {
			t.Errorf("asBoolPtr(nil) = %v; want nil", got)
		}
	})
	t.Run("true", func(t *testing.T) {
		if got := asBoolPtr(true); got == nil || !*got {
			t.Errorf("asBoolPtr(true) = %v; want ptr(true)", got)
		}
	})
	t.Run("false", func(t *testing.T) {
		if got := asBoolPtr(false); got == nil || *got {
			t.Errorf("asBoolPtr(false) = %v; want ptr(false)", got)
		}
	})
	t.Run("non-bool returns nil", func(t *testing.T) {
		if got := asBoolPtr("true"); got != nil {
			t.Errorf("asBoolPtr('true') = %v; want nil", got)
		}
	})
}

// ── asInt ──

func TestAsInt(t *testing.T) {
	tests := []struct {
		name   string
		input  any
		expect int
	}{
		{"int", int(42), 42},
		{"int8", int8(10), 10},
		{"int16", int16(300), 300},
		{"int32", int32(50000), 50000},
		{"int64", int64(100000), 100000},
		{"float64", float64(3.7), 3},
		{"json.Number integer", json.Number("123"), 123},
		{"json.Number string", json.Number("456"), 456},
		{"valid numeric string", "789", 789},
		{"zero", 0, 0},
		{"nil", nil, 0},
		{"bool", true, 0},
		{"string NaN", "abc", 0},
		{"map", map[string]any{}, 0},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := asInt(tc.input)
			if got != tc.expect {
				t.Errorf("asInt(%v) = %d; want %d", tc.input, got, tc.expect)
			}
		})
	}
}

// ── stripSymbolSuffix ──

func TestStripSymbolSuffix(t *testing.T) {
	tests := []struct {
		name   string
		input  string
		expect string
	}{
		{"A-share SH suffix", "600519.SH", "600519"},
		{"A-share SZ suffix", "000001.SZ", "000001"},
		{"HK suffix", "00700.HK", "00700"},
		{"no suffix", "600519", "600519"},
		{"already bare code", "AAPL", "AAPL"},
		{"empty string", "", ""},
		{"dot at start (idx=0, not >0) returns as-is", ".SH", ".SH"},
		{"multiple dots returns up to first dot only", "a.b.c", "a"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := stripSymbolSuffix(tc.input)
			if got != tc.expect {
				t.Errorf("stripSymbolSuffix(%q) = %q; want %q", tc.input, got, tc.expect)
			}
		})
	}
}

// ── copyForwardHeaders ──

func TestCopyForwardHeaders(t *testing.T) {
	srcReq := httptest.NewRequest(http.MethodGet, "/test", nil)
	srcReq.Header.Set("Content-Type", "application/json")
	srcReq.Header.Set("Accept", "text/html")
	srcReq.Header.Set("Authorization", "Bearer xyz")
	srcReq.Header.Set("X-Custom", "should-not-copy")

	dstReq := httptest.NewRequest(http.MethodPost, "/dest", bytes.NewReader([]byte("{}")))
	copyForwardHeaders(srcReq, dstReq)

	if got := dstReq.Header.Get("Content-Type"); got != "application/json" {
		t.Errorf("Content-Type = %q; want application/json", got)
	}
	if got := dstReq.Header.Get("Accept"); got != "text/html" {
		t.Errorf("Accept = %q; want text/html", got)
	}
	if got := dstReq.Header.Get("Authorization"); got != "Bearer xyz" {
		t.Errorf("Authorization = %q; want Bearer xyz", got)
	}
	if got := dstReq.Header.Get("X-Custom"); got != "" {
		t.Errorf("X-Custom should not be copied, got %q", got)
	}
}

func TestCopyForwardHeadersEmptySource(t *testing.T) {
	srcReq := httptest.NewRequest(http.MethodGet, "/test", nil)
	dstReq := httptest.NewRequest(http.MethodPost, "/dst", nil)
	copyForwardHeaders(srcReq, dstReq)
	// Should not panic, headers remain empty
	if dstReq.Header.Get("Content-Type") != "" {
		t.Error("expected no Content-Type from empty source")
	}
}

// ── writeError / writeJSON (integration-level smoke test) ──

func TestWriteErrorAndWriteJSON(t *testing.T) {
	t.Run("writeError sets status code and JSON body", func(t *testing.T) {
		w := httptest.NewRecorder()
		writeError(w, http.StatusBadRequest, "bad request")

		if w.Code != http.StatusBadRequest {
			t.Errorf("status = %d; want %d", w.Code, http.StatusBadRequest)
		}
		ct := w.Header().Get("Content-Type")
		if ct != "application/json; charset=utf-8" {
			t.Errorf("Content-Type = %q", ct)
		}
		var body map[string]string
		if err := json.NewDecoder(w.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		if body["detail"] != "bad request" {
			t.Errorf("detail = %q; want bad request", body["detail"])
		}
	})

	t.Run("writeJSON serializes payload", func(t *testing.T) {
		w := httptest.NewRecorder()
		payload := map[string]any{"ok": true, "count": 42}
		writeJSON(w, http.StatusOK, payload)

		if w.Code != http.StatusOK {
			t.Errorf("status = %d; want %d", w.Code, http.StatusOK)
		}
		var body map[string]any
		if err := json.NewDecoder(w.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		if body["ok"] != true || body["count"] != float64(42) {
			t.Errorf("body = %v; want ok=true count=42", body)
		}
	})
}

// ── writeLiveJSON ──

func TestWriteLiveJSON(t *testing.T) {
	t.Run("nil payload produces request_id only", func(t *testing.T) {
		w := httptest.NewRecorder()
		writeLiveJSON(w, http.StatusOK, nil)

		var body map[string]any
		json.NewDecoder(w.Body).Decode(&body)
		if _, ok := body["request_id"]; !ok {
			t.Error("expected request_id in body")
		}
	})

	t.Run("map payload gets request_id injected", func(t *testing.T) {
		w := httptest.NewRecorder()
		writeLiveJSON(w, http.StatusOK, map[string]any{"items": []string{"a", "b"}})

		var body map[string]any
		json.NewDecoder(w.Body).Decode(&body)
		if body["request_id"] == nil {
			t.Error("expected request_id injected")
		}
		if body["items"] == nil {
			t.Error("expected items preserved")
		}
	})
}

// ── corsMiddleware ──

func TestCorsMiddleware(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	mw := corsMiddleware(handler)

	t.Run("OPTIONS preflight returns 200", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodOptions, "/api/test", nil)
		w := httptest.NewRecorder()
		mw.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("OPTIONS status = %d; want 200", w.Code)
		}
		origin := w.Header().Get("Access-Control-Allow-Origin")
		if origin != "*" {
			t.Errorf("CORS Origin = %q; want *", origin)
		}
	})

	t.Run("GET request passes through", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/test", nil)
		w := httptest.NewRecorder()
		mw.ServeHTTP(w, req)

		if w.Code != http.StatusNoContent {
			t.Errorf("GET status = %d; want 204", w.Code)
		}
		origin := w.Header().Get("Access-Control-Allow-Origin")
		if origin != "*" {
			t.Errorf("CORS Origin = %q; want *", origin)
		}
	})
}

// ── Middleware helpers from middleware.go ──

func TestClientIP(t *testing.T) {
	t.Run("from X-Forwarded-For", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.Header.Set("X-Forwarded-For", "1.2.3.4, 5.6.7.8")
		got := clientIP(req)
		if got != "1.2.3.4" {
			t.Errorf("clientIP = %q; want 1.2.3.4", got)
		}
	})
	t.Run("fallback to RemoteAddr", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		// RemoteAddr is set automatically by httptest
		got := clientIP(req)
		if got == "" {
			t.Error("clientIP should fallback to RemoteAddr")
		}
	})
}

// ── TruncateString / DefaultContext from middleware.go (re-tested here for coverage) ──

func TestTruncateStringUtil(t *testing.T) {
	tests := []struct {
		name   string
		s      string
		maxLen int
		want   string
	}{
		{"short text", "short text", 20, "short text"},
		{"exactly at limit", "abc", 3, "abc"},
		{"exceeds limit by 2", "hello world", 5, "hello"},
		{"empty string", "", 10, ""},
		{"unicode chinese chars", "中文测试", 3, "中文测"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := truncateString(tc.s, tc.maxLen)
			if got != tc.want {
				t.Errorf("truncateString(%q, %d) = %q; want %q", tc.s, tc.maxLen, got, tc.want)
			}
		})
	}
}

func TestDefaultContextUtil(t *testing.T) {
	ctx, cancel := defaultContext(5 * time.Second)
	defer cancel()
	select {
	case <-ctx.Done():
		t.Error("context should not be done immediately")
	default:
		// OK
	}
}

// ── currentUser / currentUserID helpers (require context) ──

func TestCurrentUserNilRequest(t *testing.T) {
	u, ok := currentUser(nil)
	if ok {
		t.Error("currentUser(nil) should return ok=false")
	}
	if u.UserID != "" {
		t.Error("UserID should be empty for nil request")
	}
}

func TestCurrentUserIDEmpty(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	got := currentUserID(req)
	if got != "" {
		t.Errorf("currentUserID(no-context req) = %q; want empty", got)
	}
}

// ── decodeBodyAsMap ──

func TestDecodeBodyAsMap(t *testing.T) {
	t.Run("valid JSON object", func(t *testing.T) {
		body := `{"name":"test","value":42}`
		req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body))
		got, err := decodeBodyAsMap(req)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got["name"] != "test" {
			t.Errorf("name = %v; want test", got["name"])
		}
	})

	t.Run("invalid JSON", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader("not-json"))
		_, err := decodeBodyAsMap(req)
		if err == nil {
			t.Error("expected error for invalid JSON")
		}
	})

	t.Run("empty body returns EOF error", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(""))
		_, err := decodeBodyAsMap(req)
		if err == nil {
			t.Error("expected error for empty body")
		}
	})
}
