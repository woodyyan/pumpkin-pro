package capitalmap

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"golang.org/x/sync/singleflight"
)

const QuantProxyCacheTTL = 30 * time.Second

type ProxyService struct {
	quantURL   string
	httpClient *http.Client
	now        func() time.Time

	mu        sync.Mutex
	cached    map[string]any
	cachedAt  time.Time
	lastErr   error
	lastErrAt time.Time
	inflight  singleflight.Group
}

func NewProxyService(quantURL string, httpClient *http.Client) *ProxyService {
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 20 * time.Second}
	}
	return &ProxyService{
		quantURL:   strings.TrimRight(strings.TrimSpace(quantURL), "/"),
		httpClient: httpClient,
		now:        time.Now,
	}
}

func (s *ProxyService) GetPayload(ctx context.Context) (map[string]any, error) {
	if s == nil || s.quantURL == "" {
		return nil, fmt.Errorf("资金星图 quant proxy 未配置")
	}
	now := s.now()
	s.mu.Lock()
	if s.cached != nil && now.Sub(s.cachedAt) < QuantProxyCacheTTL {
		payload := cloneMap(s.cached)
		s.mu.Unlock()
		return payload, nil
	}
	s.mu.Unlock()

	_, err, _ := s.inflight.Do("fetch", func() (any, error) {
		payload, fetchErr := s.fetch(ctx)
		s.mu.Lock()
		defer s.mu.Unlock()
		if fetchErr != nil {
			s.lastErr = fetchErr
			s.lastErrAt = s.now()
			if s.cached != nil {
				s.cached["cacheStatus"] = "stale"
				s.cached["lastError"] = fetchErr.Error()
				return nil, nil
			}
			return nil, fetchErr
		}
		s.cached = cloneMap(payload)
		s.cachedAt = s.now()
		s.lastErr = nil
		s.lastErrAt = time.Time{}
		return nil, nil
	})
	if err != nil {
		return nil, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if s.cached == nil {
		return nil, fmt.Errorf("资金星图数据尚未就绪，请稍后重试")
	}
	return cloneMap(s.cached), nil
}

func (s *ProxyService) Status() ServiceStatus {
	s.mu.Lock()
	defer s.mu.Unlock()
	st := ServiceStatus{}
	if s.cached != nil {
		st.CacheAvailable = true
		st.CachedAt = s.cachedAt
		if value, ok := s.cached["cacheStatus"].(string); ok {
			st.CacheStatus = value
		}
		if value, ok := s.cached["lastError"].(string); ok {
			st.LastError = value
		}
	}
	if s.lastErr != nil {
		st.LastError = s.lastErr.Error()
		st.LastErrAt = s.lastErrAt
	}
	return st
}

func (s *ProxyService) fetch(ctx context.Context) (map[string]any, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, s.quantURL+"/api/capital-map", nil)
	if err != nil {
		return nil, err
	}
	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("quant capital map returned %d: %s", resp.StatusCode, truncateText(string(body), 200))
	}
	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, err
	}
	payload["cacheStatus"] = "fresh"
	delete(payload, "lastError")
	return payload, nil
}

func cloneMap(input map[string]any) map[string]any {
	if input == nil {
		return nil
	}
	body, err := json.Marshal(input)
	if err != nil {
		copy := make(map[string]any, len(input))
		for key, value := range input {
			copy[key] = value
		}
		return copy
	}
	var output map[string]any
	if err := json.Unmarshal(body, &output); err != nil {
		return nil
	}
	return output
}

func truncateText(value string, limit int) string {
	if len(value) <= limit {
		return value
	}
	return value[:limit]
}
