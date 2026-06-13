package live

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestBenchmarkCodeFromQuoteCode(t *testing.T) {
	cases := map[string]string{
		"sh000001": "SHCI",
		"sz399001": "SZCI",
		"sz399006": "CYBZ",
		"hkHSI":    "HSI",
		"hkHSCEI":  "HSCEI",
		"hkHSTECH": "HSTECH",
	}
	for input, want := range cases {
		if got := benchmarkCodeFromQuoteCode(input); got != want {
			t.Fatalf("benchmarkCodeFromQuoteCode(%q) = %q, want %q", input, got, want)
		}
	}
	if got := benchmarkCodeFromQuoteCode("unknown"); got != "" {
		t.Fatalf("benchmarkCodeFromQuoteCode unknown = %q, want empty", got)
	}
}

func TestFetchMarketOverviewTrendPoints(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/appstock/app/fqkline/get", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"code": 0,
			"data": map[string]any{
				"sh000001": map[string]any{
					"qfqday": [][]any{
						{"2026-06-10", "3300", "3310", "3320", "3290", "1000"},
						{"2026-06-11", "3310", "3330", "3340", "3300", "1100"},
					},
				},
			},
		})
	})
	server := httptest.NewServer(mux)
	defer server.Close()

	client := NewMarketClient()
	client.httpClient = server.Client()
	oldMap := supportedBenchmarksMap
	supportedBenchmarksMap = map[string]string{"SHCI": "sh000001"}
	defer func() { supportedBenchmarksMap = oldMap }()

	oldTransport := client.httpClient.Transport
	client.httpClient.Transport = rewriteTransport{base: oldTransport, targetBase: server.URL}

	points, err := client.fetchMarketOverviewTrendPoints(context.Background(), "sh000001")
	if err != nil {
		t.Fatalf("fetchMarketOverviewTrendPoints error: %v", err)
	}
	if len(points) != 2 {
		t.Fatalf("points length = %d, want 2", len(points))
	}
	if points[0].Date != "2026-06-10" || points[1].Count != 3330 {
		t.Fatalf("unexpected points: %#v", points)
	}
}

type rewriteTransport struct {
	base       http.RoundTripper
	targetBase string
}

func (t rewriteTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	clone := req.Clone(req.Context())
	target, _ := http.NewRequest(req.Method, t.targetBase+req.URL.Path+"?"+req.URL.RawQuery, nil)
	clone.URL = target.URL
	transport := t.base
	if transport == nil {
		transport = http.DefaultTransport
	}
	return transport.RoundTrip(clone)
}
