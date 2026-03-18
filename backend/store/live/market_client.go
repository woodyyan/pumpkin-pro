package live

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

var (
	symbolPattern          = regexp.MustCompile(`^\d{5}\.HK$`)
	supportedBenchmarksMap = map[string]string{
		"HSI":    "hkHSI",
		"HSCEI":  "hkHSCEI",
		"HSTECH": "hkHSTECH",
	}
)

const (
	dailyBarsCacheTTL = 5 * time.Minute
)

type quoteData struct {
	Code       string
	Name       string
	Last       float64
	PrevClose  float64
	High       float64
	Low        float64
	Volume     float64
	Turnover   float64
	ChangePct  float64
	VolumeRate float64
	TS         time.Time
}

type MarketClient struct {
	httpClient *http.Client

	cacheMu        sync.Mutex
	dailyBarsCache map[string]dailyBarsCacheEntry
}

type dailyBarsCacheEntry struct {
	bars     []DailyBar
	expireAt time.Time
}

type dailyBarsNode struct {
	Day    [][]any `json:"day"`
	QFQDay [][]any `json:"qfqday"`
	HFQDay [][]any `json:"hfqday"`
}

func NewMarketClient() *MarketClient {
	return &MarketClient{
		httpClient:     &http.Client{Timeout: 4 * time.Second},
		dailyBarsCache: map[string]dailyBarsCacheEntry{},
	}
}

func normalizeHKSymbol(input string) (string, error) {
	raw := strings.ToUpper(strings.TrimSpace(input))
	raw = strings.TrimPrefix(raw, "HK")
	raw = strings.TrimSuffix(raw, ".HK")
	if raw == "" {
		return "", ErrInvalidSymbol
	}
	if len(raw) < 5 {
		raw = fmt.Sprintf("%05s", raw)
	}
	candidate := raw + ".HK"
	if !symbolPattern.MatchString(candidate) {
		return "", ErrInvalidSymbol
	}
	return candidate, nil
}

func quoteCodeFromSymbol(symbol string) string {
	digits := strings.TrimSuffix(strings.ToUpper(symbol), ".HK")
	return "hk" + digits
}

func normalizeBenchmark(input string) string {
	candidate := strings.ToUpper(strings.TrimSpace(input))
	if candidate == "" {
		candidate = "HSI"
	}
	if _, ok := supportedBenchmarksMap[candidate]; !ok {
		return "HSI"
	}
	return candidate
}

func quoteCodeFromBenchmark(benchmark string) string {
	key := normalizeBenchmark(benchmark)
	return supportedBenchmarksMap[key]
}

func buildSymbolSnapshot(normalized string, quote *quoteData) *SymbolSnapshot {
	amplitude := 0.0
	if quote.PrevClose > 0 {
		amplitude = (quote.High - quote.Low) / quote.PrevClose
	}
	name := strings.TrimSpace(quote.Name)
	if name == "" {
		name = normalized
	}
	return &SymbolSnapshot{
		Symbol:      normalized,
		Name:        name,
		LastPrice:   quote.Last,
		ChangeRate:  quote.ChangePct / 100,
		Volume:      quote.Volume,
		Turnover:    quote.Turnover,
		Amplitude:   amplitude,
		VolumeRatio: quote.VolumeRate,
		TS:          quote.TS.UTC().Format(time.RFC3339),
		Source:      "tencent-qt",
	}
}

func (c *MarketClient) FetchSymbolSnapshot(ctx context.Context, symbol string) (*SymbolSnapshot, error) {
	normalized, err := normalizeHKSymbol(symbol)
	if err != nil {
		return nil, err
	}
	code := quoteCodeFromSymbol(normalized)
	fields, err := c.fetchFields(ctx, []string{code})
	if err != nil {
		return nil, err
	}
	raw, ok := fields[code]
	if !ok {
		return nil, ErrDataSourceDown
	}
	quote, err := parseQuote(code, raw)
	if err != nil {
		return nil, err
	}
	return buildSymbolSnapshot(normalized, quote), nil
}

func (c *MarketClient) FetchOverlaySnapshot(ctx context.Context, symbol, benchmark string) (*SymbolSnapshot, *BenchmarkSnapshot, error) {
	normalizedSymbol, err := normalizeHKSymbol(symbol)
	if err != nil {
		return nil, nil, err
	}
	normalizedBenchmark := normalizeBenchmark(benchmark)
	symbolCode := quoteCodeFromSymbol(normalizedSymbol)
	benchmarkCode := quoteCodeFromBenchmark(normalizedBenchmark)

	fields, err := c.fetchFields(ctx, []string{symbolCode, benchmarkCode})
	if err != nil {
		return nil, nil, err
	}

	symbolRaw, ok := fields[symbolCode]
	if !ok {
		return nil, nil, ErrDataSourceDown
	}
	symbolQuote, err := parseQuote(symbolCode, symbolRaw)
	if err != nil {
		return nil, nil, err
	}

	benchmarkRaw, ok := fields[benchmarkCode]
	if !ok {
		return nil, nil, ErrDataSourceDown
	}
	benchmarkQuote, err := parseQuote(benchmarkCode, benchmarkRaw)
	if err != nil {
		return nil, nil, err
	}

	symbolSnapshot := buildSymbolSnapshot(normalizedSymbol, symbolQuote)
	benchmarkSnapshot := &BenchmarkSnapshot{
		Code:       normalizedBenchmark,
		Name:       strings.TrimSpace(benchmarkQuote.Name),
		Last:       benchmarkQuote.Last,
		ChangeRate: benchmarkQuote.ChangePct / 100,
		TS:         benchmarkQuote.TS.UTC().Format(time.RFC3339),
	}
	if benchmarkSnapshot.Name == "" {
		benchmarkSnapshot.Name = normalizedBenchmark
	}

	return symbolSnapshot, benchmarkSnapshot, nil
}

func (c *MarketClient) FetchMarketOverview(ctx context.Context) (*MarketOverview, error) {
	codes := []string{"hkHSI", "hkHSCEI", "hkHSTECH"}
	fields, err := c.fetchFields(ctx, codes)
	if err != nil {
		return nil, err
	}

	indexes := make([]IndexSnapshot, 0, len(codes))
	totalTurnover := 0.0
	latestTS := time.Now().UTC()
	for _, code := range codes {
		raw, ok := fields[code]
		if !ok {
			continue
		}
		quote, parseErr := parseQuote(code, raw)
		if parseErr != nil {
			continue
		}
		if quote.TS.After(latestTS) {
			latestTS = quote.TS
		}
		totalTurnover += quote.Turnover
		indexes = append(indexes, IndexSnapshot{
			Code:       strings.TrimPrefix(strings.ToUpper(code), "HK"),
			Name:       strings.TrimSpace(quote.Name),
			Last:       quote.Last,
			ChangeRate: quote.ChangePct / 100,
		})
	}
	if len(indexes) == 0 {
		return nil, ErrDataSourceDown
	}

	return &MarketOverview{
		TS:             latestTS.UTC().Format(time.RFC3339),
		Indexes:        indexes,
		MarketTurnover: totalTurnover,
		Advancers:      0,
		Decliners:      0,
	}, nil
}

func (c *MarketClient) FetchSymbolDailyBars(ctx context.Context, symbol string, lookbackDays int) ([]DailyBar, error) {
	normalized, err := normalizeHKSymbol(symbol)
	if err != nil {
		return nil, err
	}
	if lookbackDays <= 0 {
		lookbackDays = 120
	}

	cacheKey := fmt.Sprintf("%s:%d", normalized, lookbackDays)
	now := time.Now().UTC()
	if bars := c.getDailyBarsCache(cacheKey, now); len(bars) > 0 {
		return bars, nil
	}

	bars, err := c.fetchDailyBarsFromTencent(ctx, normalized, lookbackDays)
	if err != nil {
		return nil, err
	}
	if len(bars) == 0 {
		return nil, ErrDataSourceDown
	}

	c.setDailyBarsCache(cacheKey, bars, now)
	return cloneDailyBars(bars), nil
}

func (c *MarketClient) getDailyBarsCache(key string, now time.Time) []DailyBar {
	c.cacheMu.Lock()
	defer c.cacheMu.Unlock()
	entry, ok := c.dailyBarsCache[key]
	if !ok || now.After(entry.expireAt) {
		if ok {
			delete(c.dailyBarsCache, key)
		}
		return nil
	}
	return cloneDailyBars(entry.bars)
}

func (c *MarketClient) setDailyBarsCache(key string, bars []DailyBar, now time.Time) {
	c.cacheMu.Lock()
	defer c.cacheMu.Unlock()
	c.dailyBarsCache[key] = dailyBarsCacheEntry{
		bars:     cloneDailyBars(bars),
		expireAt: now.Add(dailyBarsCacheTTL),
	}
}

func cloneDailyBars(bars []DailyBar) []DailyBar {
	if len(bars) == 0 {
		return nil
	}
	cloned := make([]DailyBar, len(bars))
	copy(cloned, bars)
	return cloned
}

func (c *MarketClient) fetchDailyBarsFromTencent(ctx context.Context, symbol string, lookbackDays int) ([]DailyBar, error) {
	digits := strings.TrimSuffix(strings.ToLower(symbol), ".hk")
	code := "hk" + digits
	window := lookbackDays + 20
	if window < 120 {
		window = 120
	}

	urls := []string{
		fmt.Sprintf("https://web.ifzq.gtimg.cn/appstock/app/hkfqkline/get?param=%s,day,,,%d,qfq", code, window),
		fmt.Sprintf("https://web.ifzq.gtimg.cn/appstock/app/kline/kline?param=%s,day,,,%d", code, window),
	}

	var lastErr error
	for _, targetURL := range urls {
		bars, err := c.fetchDailyBarsByURL(ctx, targetURL, code)
		if err != nil {
			lastErr = err
			continue
		}
		if len(bars) == 0 {
			continue
		}
		if len(bars) > lookbackDays {
			bars = bars[len(bars)-lookbackDays:]
		}
		return bars, nil
	}
	if lastErr != nil {
		return nil, fmt.Errorf("%w: %v", ErrDataSourceDown, lastErr)
	}
	return nil, ErrDataSourceDown
}

func (c *MarketClient) fetchDailyBarsByURL(ctx context.Context, targetURL, code string) ([]DailyBar, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, targetURL, nil)
	if err != nil {
		return nil, err
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("daily bars status=%d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var payload struct {
		Code int                      `json:"code"`
		Msg  string                   `json:"msg"`
		Data map[string]dailyBarsNode `json:"data"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, err
	}
	if payload.Code != 0 || len(payload.Data) == 0 {
		if strings.TrimSpace(payload.Msg) != "" {
			return nil, fmt.Errorf("daily bars api code=%d msg=%s", payload.Code, strings.TrimSpace(payload.Msg))
		}
		return nil, ErrDataSourceDown
	}

	rows := [][]any{}
	if node, ok := payload.Data[code]; ok {
		rows = pickDailyRows(node)
	} else {
		for _, node := range payload.Data {
			rows = pickDailyRows(node)
			if len(rows) > 0 {
				break
			}
		}
	}
	if len(rows) == 0 {
		return nil, ErrDataSourceDown
	}

	bars := make([]DailyBar, 0, len(rows))
	for _, row := range rows {
		if len(row) < 6 {
			continue
		}
		date := parseDailyBarText(row[0])
		if date == "" {
			continue
		}
		open, ok1 := parseDailyBarFloat(row[1])
		closeValue, ok2 := parseDailyBarFloat(row[2])
		high, ok3 := parseDailyBarFloat(row[3])
		low, ok4 := parseDailyBarFloat(row[4])
		volume, ok5 := parseDailyBarFloat(row[5])
		if !ok1 || !ok2 || !ok3 || !ok4 || !ok5 {
			continue
		}
		if low <= 0 || high <= 0 || closeValue <= 0 {
			continue
		}
		if high < low {
			high, low = low, high
		}
		bars = append(bars, DailyBar{
			Date:   date,
			Open:   open,
			High:   high,
			Low:    low,
			Close:  closeValue,
			Volume: math.Max(volume, 0),
		})
	}
	if len(bars) == 0 {
		return nil, ErrDataSourceDown
	}

	deduped := dedupeAndSortDailyBars(bars)
	if len(deduped) == 0 {
		return nil, ErrDataSourceDown
	}
	return deduped, nil
}

func pickDailyRows(node dailyBarsNode) [][]any {
	if len(node.QFQDay) > 0 {
		return node.QFQDay
	}
	if len(node.Day) > 0 {
		return node.Day
	}
	return node.HFQDay
}

func parseDailyBarText(value any) string {
	switch typed := value.(type) {
	case string:
		return strings.TrimSpace(typed)
	case json.Number:
		return strings.TrimSpace(typed.String())
	case float64:
		return strconv.FormatFloat(typed, 'f', -1, 64)
	case float32:
		return strconv.FormatFloat(float64(typed), 'f', -1, 32)
	case int:
		return strconv.Itoa(typed)
	case int64:
		return strconv.FormatInt(typed, 10)
	case uint64:
		return strconv.FormatUint(typed, 10)
	default:
		return ""
	}
}

func parseDailyBarFloat(value any) (float64, bool) {
	switch typed := value.(type) {
	case float64:
		return typed, true
	case float32:
		return float64(typed), true
	case int:
		return float64(typed), true
	case int64:
		return float64(typed), true
	case uint64:
		return float64(typed), true
	case json.Number:
		parsed, err := typed.Float64()
		if err != nil {
			return 0, false
		}
		return parsed, true
	case string:
		parsed, err := strconv.ParseFloat(strings.TrimSpace(typed), 64)
		if err != nil {
			return 0, false
		}
		return parsed, true
	default:
		return 0, false
	}
}

func dedupeAndSortDailyBars(input []DailyBar) []DailyBar {
	if len(input) == 0 {
		return nil
	}
	byDate := make(map[string]DailyBar, len(input))
	for _, bar := range input {
		if strings.TrimSpace(bar.Date) == "" {
			continue
		}
		byDate[bar.Date] = bar
	}
	if len(byDate) == 0 {
		return nil
	}

	dates := make([]string, 0, len(byDate))
	for date := range byDate {
		dates = append(dates, date)
	}
	sort.Slice(dates, func(i, j int) bool {
		t1, err1 := time.Parse("2006-01-02", dates[i])
		t2, err2 := time.Parse("2006-01-02", dates[j])
		if err1 != nil || err2 != nil {
			return dates[i] < dates[j]
		}
		return t1.Before(t2)
	})

	bars := make([]DailyBar, 0, len(dates))
	for _, date := range dates {
		bars = append(bars, byDate[date])
	}
	return bars
}

func (c *MarketClient) fetchFields(ctx context.Context, codes []string) (map[string][]string, error) {
	if len(codes) == 0 {
		return map[string][]string{}, nil
	}
	url := "https://qt.gtimg.cn/q=" + strings.Join(codes, ",")
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("create market request failed: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrDataSourceDown, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("%w: status=%d", ErrDataSourceDown, resp.StatusCode)
	}

	payload, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrDataSourceDown, err)
	}
	lines := strings.Split(string(payload), "\n")
	result := make(map[string][]string, len(lines))
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || !strings.HasPrefix(line, "v_") {
			continue
		}
		left := strings.Index(line, "=\"")
		right := strings.LastIndex(line, "\"")
		if left <= 2 || right <= left+2 {
			continue
		}
		code := strings.TrimSpace(line[2:left])
		rawBody := line[left+2 : right]
		result[code] = strings.Split(rawBody, "~")
	}
	if len(result) == 0 {
		return nil, fmt.Errorf("%w: empty payload", ErrDataSourceDown)
	}
	return result, nil
}

func parseQuote(code string, fields []string) (*quoteData, error) {
	if len(fields) < 44 {
		return nil, fmt.Errorf("%w: quote fields too short", ErrDataSourceDown)
	}

	last, err := parseFloat(fields[3])
	if err != nil {
		return nil, err
	}
	prevClose, err := parseFloat(fields[4])
	if err != nil {
		return nil, err
	}
	high, err := parseFloat(fields[33])
	if err != nil {
		return nil, err
	}
	low, err := parseFloat(fields[34])
	if err != nil {
		return nil, err
	}
	volume, err := parseFloat(fields[36])
	if err != nil {
		return nil, err
	}
	turnover, err := parseFloat(fields[37])
	if err != nil {
		return nil, err
	}
	changePct, err := parseFloat(fields[32])
	if err != nil {
		return nil, err
	}
	volumeRate, err := parseFloat(fields[43])
	if err != nil {
		volumeRate = 0
	}
	ts, err := time.ParseInLocation("2006/01/02 15:04:05", strings.TrimSpace(fields[30]), time.Local)
	if err != nil {
		ts = time.Now()
	}

	name := strings.TrimSpace(fields[46])
	if name == "" {
		name = strings.TrimSpace(fields[1])
	}

	return &quoteData{
		Code:       code,
		Name:       name,
		Last:       math.Max(last, 0),
		PrevClose:  math.Max(prevClose, 0),
		High:       math.Max(high, 0),
		Low:        math.Max(low, 0),
		Volume:     math.Max(volume, 0),
		Turnover:   math.Max(turnover, 0),
		ChangePct:  changePct,
		VolumeRate: math.Max(volumeRate, 0),
		TS:         ts,
	}, nil
}

func parseFloat(raw string) (float64, error) {
	value, err := strconv.ParseFloat(strings.TrimSpace(raw), 64)
	if err != nil {
		return 0, fmt.Errorf("%w: parse number failed", ErrDataSourceDown)
	}
	return value, nil
}
