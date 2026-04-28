package live

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"gorm.io/gorm"
)

const (
	newsTradingCacheTTL    = 10 * time.Minute
	newsOffTradingCacheTTL = 30 * time.Minute
	newsIdleNightCacheTTL  = 2 * time.Hour
	newsUpstreamTimeout    = 25 * time.Second
)

var newsCacheLocation = time.FixedZone("CST", 8*60*60)

func currentNewsCacheTTL(now time.Time) time.Duration {
	cst := now.In(newsCacheLocation)
	totalMinutes := cst.Hour()*60 + cst.Minute()
	if totalMinutes < 7*60 {
		return newsIdleNightCacheTTL
	}
	if isNewsMarketSession(cst) {
		return newsTradingCacheTTL
	}
	return newsOffTradingCacheTTL
}

func isNewsMarketSession(now time.Time) bool {
	if now.Weekday() == time.Saturday || now.Weekday() == time.Sunday {
		return false
	}
	totalMinutes := now.Hour()*60 + now.Minute()
	return (totalMinutes >= 9*60+15 && totalMinutes <= 12*60) || (totalMinutes >= 13*60 && totalMinutes <= 16*60+10)
}

type StockNewsRecord struct {
	Symbol         string    `gorm:"size:16;index:idx_stock_news_symbol_published,priority:1;not null"`
	ItemID         string    `gorm:"primaryKey;size:64"`
	ItemType       string    `gorm:"size:24;index;not null"`
	SourceType     string    `gorm:"size:24;not null"`
	SourceName     string    `gorm:"size:64;not null;default:''"`
	Title          string    `gorm:"size:512;not null"`
	Summary        string    `gorm:"type:text;not null;default:''"`
	SourceURL      string    `gorm:"size:1024;not null;default:''"`
	PublishedAt    time.Time `gorm:"index:idx_stock_news_symbol_published,priority:2;not null"`
	ReportPeriod   string    `gorm:"size:32;not null;default:''"`
	ReportType     string    `gorm:"size:32;not null;default:''"`
	Importance     int       `gorm:"not null;default:0"`
	IsAIRelevant   bool      `gorm:"not null;default:false"`
	DedupeKey      string    `gorm:"size:64;index;not null;default:''"`
	RawPayloadJSON string    `gorm:"type:text;not null;default:''"`
	CreatedAt      time.Time `gorm:"not null"`
	UpdatedAt      time.Time `gorm:"not null"`
}

func (StockNewsRecord) TableName() string {
	return "stock_news_items"
}

type StockNewsCacheRecord struct {
	Symbol      string    `gorm:"primaryKey;size:16"`
	Scope       string    `gorm:"primaryKey;size:24"`
	PayloadJSON string    `gorm:"type:text;not null"`
	FetchedAt   time.Time `gorm:"index;not null"`
}

func (StockNewsCacheRecord) TableName() string {
	return "stock_news_cache"
}

type StockNewsItem struct {
	ID              string `json:"id"`
	Type            string `json:"type"`
	SourceType      string `json:"source_type"`
	SourceName      string `json:"source_name"`
	Title           string `json:"title"`
	Summary         string `json:"summary"`
	PublishedAt     string `json:"published_at"`
	URL             string `json:"url"`
	ReportPeriod    string `json:"report_period,omitempty"`
	ReportType      string `json:"report_type,omitempty"`
	ImportanceScore int    `json:"importance_score"`
	IsAIRelevant    bool   `json:"is_ai_relevant"`
}

type StockNewsSummary struct {
	Last24hCount      int      `json:"last_24h_count"`
	AnnouncementCount int      `json:"announcement_count"`
	FilingCount       int      `json:"filing_count"`
	LatestHeadline    string   `json:"latest_headline"`
	HighlightTags     []string `json:"highlight_tags"`
}

type StockNewsPayload struct {
	Symbol    string           `json:"symbol"`
	Exchange  string           `json:"exchange,omitempty"`
	UpdatedAt string           `json:"updated_at"`
	Summary   StockNewsSummary `json:"summary"`
	Items     []StockNewsItem  `json:"items"`
	Meta      map[string]any   `json:"meta,omitempty"`
}

type StockNewsListOptions struct {
	Type  string
	Limit int
}

type StockNewsAIDigest struct {
	Summary map[string]any   `json:"summary"`
	Items   []map[string]any `json:"items"`
}

func (r *Repository) GetStockNewsCache(ctx context.Context, symbol, scope string, ttl time.Duration) (string, bool, error) {
	var row StockNewsCacheRecord
	err := r.db.WithContext(ctx).Where("symbol = ? AND scope = ?", symbol, scope).First(&row).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return "", false, nil
		}
		return "", false, err
	}
	if ttl > 0 && time.Since(row.FetchedAt) > ttl {
		return "", false, nil
	}
	return row.PayloadJSON, true, nil
}

func (r *Repository) UpsertStockNewsCache(ctx context.Context, symbol, scope, payloadJSON string) error {
	row := StockNewsCacheRecord{
		Symbol:      symbol,
		Scope:       scope,
		PayloadJSON: payloadJSON,
		FetchedAt:   time.Now().UTC(),
	}
	return r.db.WithContext(ctx).Save(&row).Error
}

func (r *Repository) ReplaceStockNewsItems(ctx context.Context, symbol string, items []StockNewsItem, rawItems []map[string]any) error {
	if r == nil || r.db == nil {
		return nil
	}
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("symbol = ?", symbol).Delete(&StockNewsRecord{}).Error; err != nil {
			return err
		}
		now := time.Now().UTC()
		records := make([]StockNewsRecord, 0, len(items))
		for idx, item := range items {
			rawJSON := "{}"
			if idx < len(rawItems) {
				if encoded, err := json.Marshal(rawItems[idx]); err == nil {
					rawJSON = string(encoded)
				}
			}
			publishedAt := parseNewsPublishedAt(item.PublishedAt)
			records = append(records, StockNewsRecord{
				Symbol:         symbol,
				ItemID:         item.ID,
				ItemType:       item.Type,
				SourceType:     item.SourceType,
				SourceName:     item.SourceName,
				Title:          item.Title,
				Summary:        item.Summary,
				SourceURL:      item.URL,
				PublishedAt:    publishedAt,
				ReportPeriod:   item.ReportPeriod,
				ReportType:     item.ReportType,
				Importance:     item.ImportanceScore,
				IsAIRelevant:   item.IsAIRelevant,
				DedupeKey:      item.ID,
				RawPayloadJSON: rawJSON,
				CreatedAt:      now,
				UpdatedAt:      now,
			})
		}
		if len(records) == 0 {
			return nil
		}
		return tx.Create(&records).Error
	})
}

func (r *Repository) ListStockNewsItems(ctx context.Context, symbol string, limit int) ([]StockNewsItem, time.Time, error) {
	if r == nil || r.db == nil {
		return nil, time.Time{}, nil
	}
	if limit <= 0 {
		limit = 60
	}
	var rows []StockNewsRecord
	err := r.db.WithContext(ctx).
		Where("symbol = ?", symbol).
		Order("importance DESC").
		Order("published_at DESC").
		Order("updated_at DESC").
		Limit(limit).
		Find(&rows).Error
	if err != nil {
		return nil, time.Time{}, err
	}
	items := make([]StockNewsItem, 0, len(rows))
	latestUpdatedAt := time.Time{}
	for _, row := range rows {
		if row.UpdatedAt.After(latestUpdatedAt) {
			latestUpdatedAt = row.UpdatedAt
		}
		items = append(items, StockNewsItem{
			ID:              row.ItemID,
			Type:            normalizeNewsType(row.ItemType),
			SourceType:      normalizeNewsSourceType(row.SourceType),
			SourceName:      row.SourceName,
			Title:           row.Title,
			Summary:         row.Summary,
			PublishedAt:     row.PublishedAt.UTC().Format(time.RFC3339),
			URL:             row.SourceURL,
			ReportPeriod:    row.ReportPeriod,
			ReportType:      row.ReportType,
			ImportanceScore: row.Importance,
			IsAIRelevant:    row.IsAIRelevant,
		})
	}
	return items, latestUpdatedAt, nil
}

func parseNewsPublishedAt(input string) time.Time {
	text := strings.TrimSpace(input)
	if text == "" {
		return time.Now().UTC()
	}
	layouts := []string{time.RFC3339, "2006-01-02 15:04:05", "2006-01-02"}
	for _, layout := range layouts {
		if parsed, err := time.Parse(layout, text); err == nil {
			return parsed.UTC()
		}
	}
	return time.Now().UTC()
}

func hasUsableNewsPayload(payload *StockNewsPayload) bool {
	if payload == nil {
		return false
	}
	if len(payload.Items) > 0 {
		return true
	}
	if payload.Summary.Last24hCount > 0 || payload.Summary.AnnouncementCount > 0 || payload.Summary.FilingCount > 0 {
		return true
	}
	if strings.TrimSpace(payload.Summary.LatestHeadline) != "" {
		return true
	}
	return len(payload.Summary.HighlightTags) > 0
}

func annotateStoredPayload(payload *StockNewsPayload, symbol string) *StockNewsPayload {
	if payload == nil {
		return nil
	}
	if strings.TrimSpace(payload.Symbol) == "" {
		payload.Symbol = symbol
	}
	payload.Summary = buildSummaryFromItems(payload.Items, payload.Summary)
	if payload.Meta == nil {
		payload.Meta = map[string]any{}
	}
	payload.Meta["storage"] = "database"
	return payload
}

func isDegradedNewsPayload(payload *StockNewsPayload) bool {
	if payload == nil || payload.Meta == nil {
		return false
	}
	flag, ok := payload.Meta["degraded"].(bool)
	return ok && flag
}

func buildUnavailableNewsPayload(symbol string, err error) *StockNewsPayload {
	message := "新闻源暂时不可用，稍后会自动重试。"
	if err != nil && strings.Contains(strings.ToLower(err.Error()), "timeout") {
		message = "新闻源响应较慢，已先返回空结果并等待后台自动补齐。"
	}
	return &StockNewsPayload{
		Symbol:    symbol,
		UpdatedAt: time.Now().UTC().Format(time.RFC3339),
		Summary: StockNewsSummary{
			HighlightTags: []string{},
		},
		Items: []StockNewsItem{},
		Meta: map[string]any{
			"degraded": true,
			"warnings": []string{message},
		},
	}
}

type NewsService struct {
	repo         *Repository
	quantBaseURL string
	client       *http.Client
	refreshMu    sync.Mutex
	refreshing   map[string]struct{}
}

func NewNewsService(repo *Repository, quantBaseURL string) *NewsService {
	return &NewsService{
		repo:         repo,
		quantBaseURL: strings.TrimRight(strings.TrimSpace(quantBaseURL), "/"),
		client:       &http.Client{Timeout: newsUpstreamTimeout},
		refreshing:   make(map[string]struct{}),
	}
}

func (s *NewsService) GetSymbolNews(ctx context.Context, symbol string, opts StockNewsListOptions) (*StockNewsPayload, error) {
	normalized, exchange, err := NormalizeSymbol(symbol)
	if err != nil {
		return nil, err
	}
	payload, err := s.loadPayload(ctx, normalized)
	if err != nil {
		return nil, err
	}
	payload.Exchange = exchange
	filteredItems := filterNewsItems(payload.Items, opts)
	payload.Summary = buildSummaryFromItems(payload.Items, payload.Summary)
	payload.Items = filteredItems
	return payload, nil
}

func (s *NewsService) GetSymbolNewsSummary(ctx context.Context, symbol string) (*StockNewsPayload, error) {
	normalized, exchange, err := NormalizeSymbol(symbol)
	if err != nil {
		return nil, err
	}
	cacheScope := "summary"
	cacheTTL := currentNewsCacheTTL(time.Now())
	if s.repo != nil {
		if cached, hit, cacheErr := s.repo.GetStockNewsCache(ctx, normalized, cacheScope, cacheTTL); cacheErr == nil && hit {
			var payload StockNewsPayload
			if err := json.Unmarshal([]byte(cached), &payload); err == nil && hasUsableNewsPayload(&payload) {
				payload.Exchange = exchange
				annotateStoredPayload(&payload, normalized)
				return &payload, nil
			}
		}
	}
	payload, err := s.loadPayload(ctx, normalized)
	if err != nil {
		return nil, err
	}
	payload.Exchange = exchange
	payload.Items = nil
	if s.repo != nil && !isDegradedNewsPayload(payload) {
		if encoded, err := json.Marshal(payload); err == nil {
			_ = s.repo.UpsertStockNewsCache(ctx, normalized, cacheScope, string(encoded))
		}
	}
	return payload, nil
}

func (s *NewsService) BuildAIDigest(ctx context.Context, symbol string, maxItems int) (*StockNewsAIDigest, error) {
	payload, err := s.GetSymbolNews(ctx, symbol, StockNewsListOptions{Type: "all", Limit: 24})
	if err != nil {
		return nil, err
	}
	relevant := make([]StockNewsItem, 0, len(payload.Items))
	for _, item := range payload.Items {
		if item.IsAIRelevant {
			relevant = append(relevant, item)
		}
	}
	if maxItems <= 0 {
		maxItems = 6
	}
	if len(relevant) > maxItems {
		relevant = relevant[:maxItems]
	}
	digestItems := make([]map[string]any, 0, len(relevant))
	for _, item := range relevant {
		digestItems = append(digestItems, map[string]any{
			"type":          item.Type,
			"source":        item.SourceName,
			"published_at":  item.PublishedAt,
			"title":         item.Title,
			"summary":       item.Summary,
			"official":      item.SourceType == "official",
			"report_period": item.ReportPeriod,
			"report_type":   item.ReportType,
		})
	}
	return &StockNewsAIDigest{
		Summary: map[string]any{
			"last_24h_count":     payload.Summary.Last24hCount,
			"announcement_count": payload.Summary.AnnouncementCount,
			"filing_count":       payload.Summary.FilingCount,
			"highlight_tags":     payload.Summary.HighlightTags,
		},
		Items: digestItems,
	}, nil
}

func (s *NewsService) loadPayload(ctx context.Context, symbol string) (*StockNewsPayload, error) {
	cacheScope := "list"
	cacheTTL := currentNewsCacheTTL(time.Now())
	if s.repo != nil {
		if cached, hit, cacheErr := s.repo.GetStockNewsCache(ctx, symbol, cacheScope, cacheTTL); cacheErr == nil && hit {
			var payload StockNewsPayload
			if err := json.Unmarshal([]byte(cached), &payload); err == nil && hasUsableNewsPayload(&payload) {
				return annotateStoredPayload(&payload, symbol), nil
			}
		}
	}
	if stored, err := s.loadPersistedPayload(ctx, symbol); err == nil && stored != nil {
		s.triggerRefresh(symbol)
		return stored, nil
	}
	payload, rawItems, err := s.fetchFromQuant(ctx, symbol)
	if err != nil {
		return buildUnavailableNewsPayload(symbol, err), nil
	}
	s.persistPayload(ctx, symbol, payload, rawItems)
	return payload, nil
}

func (s *NewsService) loadPersistedPayload(ctx context.Context, symbol string) (*StockNewsPayload, error) {
	if s.repo == nil {
		return nil, nil
	}
	if cached, hit, err := s.repo.GetStockNewsCache(ctx, symbol, "list", 0); err == nil && hit {
		var payload StockNewsPayload
		if err := json.Unmarshal([]byte(cached), &payload); err == nil && hasUsableNewsPayload(&payload) {
			return annotateStoredPayload(&payload, symbol), nil
		}
	}
	items, updatedAt, err := s.repo.ListStockNewsItems(ctx, symbol, 60)
	if err != nil {
		return nil, err
	}
	if len(items) == 0 {
		return nil, nil
	}
	if updatedAt.IsZero() {
		updatedAt = parseNewsPublishedAt(items[0].PublishedAt)
	}
	payload := &StockNewsPayload{
		Symbol:    symbol,
		UpdatedAt: updatedAt.UTC().Format(time.RFC3339),
		Summary:   buildSummaryFromItems(items, StockNewsSummary{}),
		Items:     items,
	}
	return annotateStoredPayload(payload, symbol), nil
}

func (s *NewsService) persistPayload(ctx context.Context, symbol string, payload *StockNewsPayload, rawItems []map[string]any) {
	if s.repo == nil || payload == nil {
		return
	}
	if encoded, err := json.Marshal(payload); err == nil {
		_ = s.repo.UpsertStockNewsCache(ctx, symbol, "list", string(encoded))
	}
	_ = s.repo.ReplaceStockNewsItems(ctx, symbol, payload.Items, rawItems)
	summaryPayload := *payload
	summaryPayload.Items = nil
	if encoded, err := json.Marshal(summaryPayload); err == nil {
		_ = s.repo.UpsertStockNewsCache(ctx, symbol, "summary", string(encoded))
	}
}

func (s *NewsService) triggerRefresh(symbol string) {
	if s.repo == nil || s.quantBaseURL == "" {
		return
	}
	s.refreshMu.Lock()
	if _, exists := s.refreshing[symbol]; exists {
		s.refreshMu.Unlock()
		return
	}
	s.refreshing[symbol] = struct{}{}
	s.refreshMu.Unlock()

	go func() {
		defer func() {
			s.refreshMu.Lock()
			delete(s.refreshing, symbol)
			s.refreshMu.Unlock()
		}()
		ctx, cancel := context.WithTimeout(context.Background(), newsUpstreamTimeout)
		defer cancel()
		payload, rawItems, err := s.fetchFromQuant(ctx, symbol)
		if err != nil {
			return
		}
		s.persistPayload(ctx, symbol, payload, rawItems)
	}()
}

func (s *NewsService) fetchFromQuant(ctx context.Context, symbol string) (*StockNewsPayload, []map[string]any, error) {
	if s.quantBaseURL == "" {
		return nil, nil, fmt.Errorf("news service unavailable: quant base URL is empty")
	}
	endpoint := s.quantBaseURL + "/api/news/" + url.PathEscape(symbol)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, nil, err
	}
	resp, err := s.client.Do(req)
	if err != nil {
		return nil, nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, nil, fmt.Errorf("news upstream returned status %d", resp.StatusCode)
	}
	var raw struct {
		Symbol    string           `json:"symbol"`
		Exchange  string           `json:"exchange"`
		UpdatedAt string           `json:"updated_at"`
		Summary   StockNewsSummary `json:"summary"`
		Items     []map[string]any `json:"items"`
		Meta      map[string]any   `json:"meta"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return nil, nil, err
	}
	items := make([]StockNewsItem, 0, len(raw.Items))
	for _, row := range raw.Items {
		items = append(items, StockNewsItem{
			ID:              asMapString(row, "id"),
			Type:            normalizeNewsType(asMapString(row, "type")),
			SourceType:      normalizeNewsSourceType(asMapString(row, "source_type")),
			SourceName:      asMapString(row, "source_name"),
			Title:           asMapString(row, "title"),
			Summary:         asMapString(row, "summary"),
			PublishedAt:     asMapString(row, "published_at"),
			URL:             asMapString(row, "url"),
			ReportPeriod:    asMapString(row, "report_period"),
			ReportType:      asMapString(row, "report_type"),
			ImportanceScore: asMapInt(row, "importance_score"),
			IsAIRelevant:    asMapBool(row, "is_ai_relevant"),
		})
	}
	payload := &StockNewsPayload{
		Symbol:    symbol,
		Exchange:  raw.Exchange,
		UpdatedAt: raw.UpdatedAt,
		Summary:   raw.Summary,
		Items:     items,
		Meta:      raw.Meta,
	}
	return payload, raw.Items, nil
}

func asMapString(m map[string]any, key string) string {
	if m == nil {
		return ""
	}
	value, ok := m[key]
	if !ok {
		return ""
	}
	text, ok := value.(string)
	if !ok {
		return ""
	}
	return strings.TrimSpace(text)
}

func asMapInt(m map[string]any, key string) int {
	if m == nil {
		return 0
	}
	value, ok := m[key]
	if !ok {
		return 0
	}
	switch typed := value.(type) {
	case float64:
		return int(typed)
	case int:
		return typed
	default:
		return 0
	}
}

func asMapBool(m map[string]any, key string) bool {
	if m == nil {
		return false
	}
	value, ok := m[key]
	if !ok {
		return false
	}
	typed, ok := value.(bool)
	return ok && typed
}

func normalizeNewsType(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "announcement":
		return "announcement"
	case "filing":
		return "filing"
	default:
		return "news"
	}
}

func normalizeNewsSourceType(raw string) string {
	if strings.EqualFold(strings.TrimSpace(raw), "official") {
		return "official"
	}
	return "media"
}

func filterNewsItems(items []StockNewsItem, opts StockNewsListOptions) []StockNewsItem {
	limit := opts.Limit
	if limit <= 0 {
		limit = 20
	}
	wantedType := strings.ToLower(strings.TrimSpace(opts.Type))
	if wantedType == "" {
		wantedType = "all"
	}
	filtered := make([]StockNewsItem, 0, len(items))
	for _, item := range items {
		if wantedType != "all" && item.Type != wantedType {
			continue
		}
		filtered = append(filtered, item)
		if len(filtered) >= limit {
			break
		}
	}
	return filtered
}

func buildSummaryFromItems(items []StockNewsItem, fallback StockNewsSummary) StockNewsSummary {
	if len(items) == 0 {
		return fallback
	}
	summary := StockNewsSummary{HighlightTags: []string{}}
	now := time.Now().UTC()
	seenTags := map[string]struct{}{}
	for idx, item := range items {
		if idx == 0 {
			summary.LatestHeadline = item.Title
		}
		switch item.Type {
		case "announcement":
			summary.AnnouncementCount++
		case "filing":
			summary.FilingCount++
		}
		publishedAt := parseNewsPublishedAt(item.PublishedAt)
		if publishedAt.IsZero() || now.Sub(publishedAt) <= 24*time.Hour {
			summary.Last24hCount++
		}
		for _, tag := range inferNewsTags(item) {
			if _, ok := seenTags[tag]; ok {
				continue
			}
			seenTags[tag] = struct{}{}
			summary.HighlightTags = append(summary.HighlightTags, tag)
		}
	}
	if summary.Last24hCount == 0 {
		summary.Last24hCount = len(items)
	}
	if len(summary.HighlightTags) == 0 {
		summary.HighlightTags = fallback.HighlightTags
	}
	if summary.LatestHeadline == "" {
		summary.LatestHeadline = fallback.LatestHeadline
	}
	return summary
}

func inferNewsTags(item StockNewsItem) []string {
	tags := make([]string, 0, 4)
	if item.Type == "filing" {
		tags = append(tags, "财报")
	}
	if item.Type == "announcement" {
		tags = append(tags, "公告")
	}
	title := item.Title
	for _, candidate := range []struct{ keyword, tag string }{
		{"回购", "回购"},
		{"分红", "分红"},
		{"调研", "机构调研"},
		{"业绩", "业绩"},
		{"新品", "新品"},
		{"停牌", "停牌"},
	} {
		if strings.Contains(title, candidate.keyword) {
			tags = append(tags, candidate.tag)
		}
	}
	return dedupeStrings(tags)
}

func dedupeStrings(values []string) []string {
	result := make([]string, 0, len(values))
	seen := map[string]struct{}{}
	for _, value := range values {
		text := strings.TrimSpace(value)
		if text == "" {
			continue
		}
		if _, ok := seen[text]; ok {
			continue
		}
		seen[text] = struct{}{}
		result = append(result, text)
	}
	return result
}
