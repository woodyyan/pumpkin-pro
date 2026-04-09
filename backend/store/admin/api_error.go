package admin

import (
	"context"
	"strings"
	"time"
)

// ── Model ──

type APIErrorRecord struct {
	ID           int64     `gorm:"primaryKey;autoIncrement"`
	Method       string    `gorm:"size:10;not null;default:''"`
	Path         string    `gorm:"size:512;not null;default:''"`
	QueryParams  string    `gorm:"size:512;default:''"`
	StatusCode   int       `gorm:"index;not null;default:0"`
	ErrorCode    string    `gorm:"size:128;default:''"`
	ErrorMessage string    `gorm:"size:2048;default:''"`
	DurationMS   int64     `gorm:"not null;default:0"`
	ClientIP     string    `gorm:"size:45;default:''"`
	UserAgent    string    `gorm:"size:512;default:''"`
	UserID       string    `gorm:"size:36;default:''"`
	CreatedAt    time.Time `gorm:"not null;index"`
}

func (APIErrorRecord) TableName() string {
	return "api_errors"
}

// ── System Health Response Types ──

type SystemHealthStats struct {
	ErrorSummary      ErrorSummary          `json:"error_summary"`
	ErrorTrends       []DailyCount          `json:"error_trends,omitempty"`
	TopErrorEndpoints []EndpointErrorCount  `json:"top_error_endpoints"`
	RecentErrors      []APIErrorLogItem     `json:"recent_errors"`
	GeneratedAt       string                `json:"generated_at"`
}

type ErrorSummary struct {
	TodayTotal   int64   `json:"today_total"`
	ClientErrors int64   `json:"client_errors"` // 4xx
	ServerErrors int64   `json:"server_errors"` // 5x
	AvgDuration  float64 `json:"avg_duration_ms"`
}

type EndpointErrorCount struct {
	Path       string `json:"path"`
	Method     string `json:"method"`
	Count      int64  `json:"count"`
	LastSeenAt string `json:"last_seen_at"`
}

type APIErrorLogItem struct {
	ID           int64  `json:"id"`
	Method       string `json:"method"`
	Path         string `json:"path"`
	StatusCode   int    `json:"status_code"`
	ErrorCode    string `json:"error_code"`
	ErrorMessage string `json:"error_message"`
	DurationMS   int64  `json:"duration_ms"`
	ClientIP     string `json:"client_ip"`
	UserID       string `json:"user_id"`
	CreatedAt    string `json:"created_at"`
}

// ── Sensitive query params to strip before storing ──

var sensitiveQueryParams = map[string]bool{
	"token": true, "password": true, "passwd": true,
	"secret": true, "key": true, "api_key": true,
	"access_token": true, "refresh_token": true,
	"authorization": true, "credential": true,
}

// SanitizeQueryString strips sensitive key-value pairs from a raw query string.
func SanitizeQueryString(raw string) string {
	if raw == "" {
		return ""
	}
	parts := strings.Split(raw, "&")
	safe := make([]string, 0, len(parts))
	for _, part := range parts {
		idx := strings.Index(part, "=")
		if idx < 0 {
			safe = append(safe, part)
			continue
		}
		key := strings.ToLower(strings.TrimSpace(part[:idx]))
		if sensitiveQueryParams[key] {
			continue // drop this param entirely
		}
		safe = append(safe, part)
	}
	return strings.Join(safe, "&")
}

// NormalizePath returns a cleaned path suitable for grouping/aggregation.
// Strips UUIDs and numeric IDs from common patterns to avoid cardinality explosion.
func NormalizePath(path string) string {
	path = strings.TrimSpace(path)
	// Strip trailing slash
	path = strings.TrimRight(path, "/")
	return path
}

// ── Repository Methods ──

// InsertAPIError records an API error entry (called asynchronously).
func (r *Repository) InsertAPIError(ctx context.Context, record APIErrorRecord) error {
	return r.db.WithContext(ctx).Create(&record).Error
}

// CountAPIErrorsToday returns total error records created today.
func (r *Repository) CountAPIErrorsToday(ctx context.Context) (int64, error) {
	today := time.Now().UTC().Truncate(24 * time.Hour)
	var count int64
	err := r.db.WithContext(ctx).Model(&APIErrorRecord{}).
		Where("created_at >= ?", today).Count(&count).Error
	return count, err
}

// CountAPIErrorsByStatusRange groups errors into client(4xx) vs server(5xx).
func (r *Repository) CountAPIErrorsByStatusRange(ctx context.Context) (client, server int64, err error) {
	today := time.Now().UTC().Truncate(24 * time.Hour)
	err = r.db.WithContext(ctx).Model(&APIErrorRecord{}).
		Where("created_at >= ? AND status_code BETWEEN ? AND ?", today, 400, 499).
		Count(&client).Error
	if err != nil {
		return
	}
	err = r.db.WithContext(ctx).Model(&APIErrorRecord{}).
		Where("created_at >= ? AND status_code BETWEEN ? AND ?", today, 500, 599).
		Count(&server).Error
	return
}

// AvgErrorDurationToday returns average duration_ms for today's errors.
func (r *Repository) AvgErrorDurationToday(ctx context.Context) (float64, error) {
	today := time.Now().UTC().Truncate(24 * time.Hour)
	type result struct { AvgMS float64 }
	var res result
	err := r.db.WithContext(ctx).Model(&APIErrorRecord{}).
		Select("COALESCE(AVG(duration_ms), 0) as avg_ms").
		Where("created_at >= ?", today).
		Scan(&res).Error
	return res.AvgMS, err
}

// TopErrorEndpoints returns the N paths with the most errors (today).
func (r *Repository) TopErrorEndpoints(ctx context.Context, limit int) ([]EndpointErrorCount, error) {
	today := time.Now().UTC().Truncate(24 * time.Hour)
	type rawRow struct {
		Path       string `gorm:"column:path"`
		Method     string `gorm:"column:method"`
		Count      int64  `gorm:"column:cnt"`
		LastSeenAt string `gorm:"column:last_seen_at"`
	}
	var rows []rawRow
	err := r.db.WithContext(ctx).Model(&APIErrorRecord{}).
		Select("path, method, COUNT(*) as cnt, MAX(created_at) as last_seen_at").
		Where("created_at >= ?", today).
		Group("path, method").Order("cnt DESC").Limit(limit).
		Scan(&rows).Error
	if err != nil || len(rows) == 0 {
		return []EndpointErrorCount{}, err
	}
	result := make([]EndpointErrorCount, len(rows))
	for i, row := range rows {
		result[i] = EndpointErrorCount{
			Path:       row.Path,
			Method:     row.Method,
			Count:      row.Count,
			LastSeenAt: row.LastSeenAt,
		}
	}
	return result, nil
}

// ListAPIErrors returns paginated error log entries ordered by newest first.
func (r *Repository) ListAPIErrors(ctx context.Context, limit, offset int) ([]APIErrorRecord, int64, error) {
	var total int64
	r.db.WithContext(ctx).Model(&APIErrorRecord{}).Count(&total)

	var items []APIErrorRecord
	err := r.db.WithContext(ctx).Order("id DESC").Limit(limit).Offset(offset).Find(&items).Error
	return items, total, err
}

// DailyErrorTrend returns error counts per day for the last N days.
func (r *Repository) DailyErrorTrend(ctx context.Context, days int) ([]DailyCount, error) {
	since := time.Now().UTC().AddDate(0, 0, -days)
	var results []DailyCount
	err := r.db.WithContext(ctx).Model(&APIErrorRecord{}).
		Select("DATE(created_at) as date, COUNT(*) as count").
		Where("created_at >= ?", since).
		Group("DATE(created_at)").Order("date ASC").
		Scan(&results).Error
	if err != nil {
		return []DailyCount{}, err
	}
	if results == nil {
		results = []DailyCount{}
	}
	return results, nil
}

// PurgeOldAPIErrors removes records older than retentionDays (e.g. 30).
func (r *Repository) PurgeOldAPIErrors(ctx context.Context, retentionDays int) (int64, error) {
	cutoff := time.Now().UTC().AddDate(0, 0, -retentionDays)
	res := r.db.WithContext(ctx).Where("created_at < ?", cutoff).Delete(&APIErrorRecord{})
	return res.RowsAffected, res.Error
}

// ── Service Method ──

// GetSystemHealthStats aggregates all error monitoring data for the admin panel.
func (s *Service) GetSystemHealthStats(ctx context.Context) (*SystemHealthStats, error) {
	todayTotal, _ := s.repo.CountAPIErrorsToday(ctx)
	clientErr, serverErr, _ := s.repo.CountAPIErrorsByStatusRange(ctx)
	avgDur, _ := s.repo.AvgErrorDurationToday(ctx)
	topEndpoints, _ := s.repo.TopErrorEndpoints(ctx, 10)
	recentRaw, _, _ := s.repo.ListAPIErrors(ctx, 50, 0)
	trend, _ := s.repo.DailyErrorTrend(ctx, 14)

	logItems := make([]APIErrorLogItem, len(recentRaw))
	for i, e := range recentRaw {
		msg := e.ErrorMessage
		if len([]rune(msg)) > 200 {
			msg = string([]rune(msg)[:200]) + "…"
		}
		logItems[i] = APIErrorLogItem{
			ID:           e.ID,
			Method:       e.Method,
			Path:         e.Path,
			StatusCode:   e.StatusCode,
			ErrorCode:    e.ErrorCode,
			ErrorMessage: msg,
			DurationMS:   e.DurationMS,
			ClientIP:     e.ClientIP,
			UserID:       e.UserID,
			CreatedAt:    e.CreatedAt.Format("2006-01-02T15:04:05Z"),
		}
	}

	return &SystemHealthStats{
		ErrorSummary: ErrorSummary{
			TodayTotal:   todayTotal,
			ClientErrors: clientErr,
			ServerErrors: serverErr,
			AvgDuration:  avgDur,
		},
		ErrorTrends:       trend,
		TopErrorEndpoints: topEndpoints,
		RecentErrors:      logItems,
		GeneratedAt:       time.Now().UTC().Format(time.RFC3339),
	}, nil
}
