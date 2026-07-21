package newskline

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"golang.org/x/sync/singleflight"
)

const (
	QuantProxyCacheTTL = 30 * time.Minute
	QuantProxyTimeout  = 18 * time.Second
	DefaultDays        = 500
	DefaultPages       = 3
	MinDays            = 60
	MaxDays            = 1000
	MinPages           = 1
	MaxPages           = 5
)

type ServiceStatus struct {
	CacheAvailable bool
	CachedAt       time.Time
	CacheStatus    string
	LastError      string
	LastErrAt      time.Time
}

type cacheEntry struct {
	payload  map[string]any
	cachedAt time.Time
}

type ProxyService struct {
	quantURL   string
	httpClient *http.Client
	now        func() time.Time

	mu        sync.Mutex
	cached    map[string]cacheEntry
	lastErr   error
	lastErrAt time.Time
	inflight  singleflight.Group
}

func NewProxyService(quantURL string, httpClient *http.Client) *ProxyService {
	if httpClient == nil {
		httpClient = &http.Client{Timeout: QuantProxyTimeout}
	}
	return &ProxyService{
		quantURL:   strings.TrimRight(strings.TrimSpace(quantURL), "/"),
		httpClient: httpClient,
		now:        time.Now,
		cached:     make(map[string]cacheEntry),
	}
}

func NormalizeDays(value string) int {
	return clampInt(value, DefaultDays, MinDays, MaxDays)
}

func NormalizePages(value string) int {
	return clampInt(value, DefaultPages, MinPages, MaxPages)
}

func (s *ProxyService) GetPayload(ctx context.Context, symbol string, days int, pages int, force bool) (map[string]any, error) {
	if s == nil || s.quantURL == "" {
		return nil, fmt.Errorf("新闻透视 quant proxy 未配置")
	}
	symbol = strings.ToUpper(strings.TrimSpace(symbol))
	if symbol == "" {
		return nil, fmt.Errorf("股票代码不能为空")
	}
	if days <= 0 {
		days = DefaultDays
	}
	if pages <= 0 {
		pages = DefaultPages
	}
	if days < MinDays {
		days = MinDays
	}
	if days > MaxDays {
		days = MaxDays
	}
	if pages < MinPages {
		pages = MinPages
	}
	if pages > MaxPages {
		pages = MaxPages
	}
	cacheKey := buildCacheKey(symbol, days, pages)
	now := s.now()

	if !force {
		s.mu.Lock()
		if entry, ok := s.cached[cacheKey]; ok && now.Sub(entry.cachedAt) < QuantProxyCacheTTL {
			payload := cloneMap(entry.payload)
			s.mu.Unlock()
			return payload, nil
		}
		s.mu.Unlock()
	}

	_, err, _ := s.inflight.Do(cacheKey, func() (any, error) {
		payload, fetchErr := s.fetch(ctx, symbol, days, pages, force)
		s.mu.Lock()
		defer s.mu.Unlock()
		if fetchErr != nil {
			s.lastErr = fetchErr
			s.lastErrAt = s.now()
			if entry, ok := s.cached[cacheKey]; ok && entry.payload != nil {
				stale := cloneMap(entry.payload)
				annotateCacheStatus(stale, "stale", fetchErr.Error())
				s.cached[cacheKey] = cacheEntry{payload: stale, cachedAt: entry.cachedAt}
				return nil, nil
			}
			return nil, fetchErr
		}
		annotateCacheStatus(payload, "fresh", "")
		s.cached[cacheKey] = cacheEntry{payload: cloneMap(payload), cachedAt: s.now()}
		s.lastErr = nil
		s.lastErrAt = time.Time{}
		return nil, nil
	})
	if err != nil {
		return nil, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	entry, ok := s.cached[cacheKey]
	if !ok || entry.payload == nil {
		return nil, fmt.Errorf("新闻透视数据尚未就绪，请稍后重试")
	}
	return cloneMap(entry.payload), nil
}

func (s *ProxyService) Status() ServiceStatus {
	s.mu.Lock()
	defer s.mu.Unlock()
	st := ServiceStatus{}
	for _, entry := range s.cached {
		if entry.payload != nil {
			st.CacheAvailable = true
			if entry.cachedAt.After(st.CachedAt) {
				st.CachedAt = entry.cachedAt
				if meta, ok := entry.payload["META"].(map[string]any); ok {
					if value, ok := meta["cache_status"].(string); ok {
						st.CacheStatus = value
					}
					if value, ok := meta["last_error"].(string); ok {
						st.LastError = value
					}
				}
			}
		}
	}
	if s.lastErr != nil {
		st.LastError = s.lastErr.Error()
		st.LastErrAt = s.lastErrAt
	}
	return st
}

func (s *ProxyService) fetch(ctx context.Context, symbol string, days int, pages int, force bool) (map[string]any, error) {
	endpoint := fmt.Sprintf("%s/api/news-kline/%s", s.quantURL, url.PathEscape(symbol))
	qs := url.Values{}
	qs.Set("days", strconv.Itoa(days))
	qs.Set("pages", strconv.Itoa(pages))
	if force {
		qs.Set("force", "true")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint+"?"+qs.Encode(), nil)
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
		return nil, fmt.Errorf("quant news kline returned %d: %s", resp.StatusCode, truncateText(string(body), 200))
	}
	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, err
	}
	return payload, nil
}

func buildCacheKey(symbol string, days int, pages int) string {
	return fmt.Sprintf("%s:%d:%d", strings.ToUpper(strings.TrimSpace(symbol)), days, pages)
}

func annotateCacheStatus(payload map[string]any, status string, lastError string) {
	if payload == nil {
		return
	}
	meta, _ := payload["META"].(map[string]any)
	if meta == nil {
		meta = map[string]any{}
		payload["META"] = meta
	}
	meta["cache_status"] = status
	meta["cache_ttl_seconds"] = int(QuantProxyCacheTTL.Seconds())
	if strings.TrimSpace(lastError) == "" {
		delete(meta, "last_error")
		return
	}
	meta["last_error"] = lastError
}

func clampInt(value string, defaultValue int, minimum int, maximum int) int {
	parsed, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil {
		return defaultValue
	}
	if parsed < minimum {
		return minimum
	}
	if parsed > maximum {
		return maximum
	}
	return parsed
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
