package main

import (
	"context"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/woodyyan/pumpkin-pro/backend/store/admin"
)

// ── ResponseWriter wrapper to capture status code ──

type statusResponseWriter struct {
	http.ResponseWriter
	statusCode int
	written    bool
}

func newStatusResponseWriter(w http.ResponseWriter) *statusResponseWriter {
	return &statusResponseWriter{ResponseWriter: w, statusCode: http.StatusOK}
}

func (w *statusResponseWriter) WriteHeader(code int) {
	if !w.written {
		w.statusCode = code
	}
	w.ResponseWriter.WriteHeader(code)
}

func (w *statusResponseWriter) Write(b []byte) (int, error) {
	w.written = true
	return w.ResponseWriter.Write(b)
}

// ── Logging Middleware ──
// Records API errors (status >= 400) into api_errors table asynchronously.
// Normal requests (2xx/3xx) pass through with no DB write.

func (a *appServer) loggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rw := newStatusResponseWriter(w)

		next.ServeHTTP(rw, r)
		duration := time.Since(start)

		// Only record errors (4xx / 5xx)
		if rw.statusCode < 400 || rw.statusCode >= 600 {
			return
		}

		go func() {
			record := admin.APIErrorRecord{
				Method:       strings.ToUpper(r.Method),
				Path:         admin.NormalizePath(r.URL.Path),
				QueryParams:  admin.SanitizeQueryString(r.URL.RawQuery),
				StatusCode:   rw.statusCode,
				DurationMS:   duration.Milliseconds(),
				ClientIP:     clientIP(r),
				UserAgent:    truncateString(r.UserAgent(), 256),
				CreatedAt:    start.UTC(),
			}

			// Try to extract error code/message from context or skip if not available.
			// For most writeError calls, we rely on the handler having already written a JSON body;
			// the middleware records what it sees.

			ctx, cancel := defaultContext(5 * time.Second)
			defer cancel()

			if err := a.adminService.InsertAPIError(ctx, record); err != nil {
				log.Printf("[api-error] failed to persist: %v", err)
			}
		}()

		// Note: we intentionally do NOT record device snapshots for API errors.
		// API errors often come from server-to-server calls, health checks, or
		// scrapers with UA strings like "node-fetch" that pollute device analytics.
	})
}

// clientIP extracts client IP from X-Forwarded-For or RemoteAddr.
func clientIP(r *http.Request) string {
	xff := r.Header.Get("X-Forwarded-For")
	if xff != "" {
		parts := strings.SplitN(xff, ",", 2)
		return strings.TrimSpace(parts[0])
	}
	idx := strings.LastIndex(r.RemoteAddr, ":")
	if idx < 0 {
		return r.RemoteAddr
	}
	return r.RemoteAddr[:idx]
}

// truncateString limits string length.
func truncateString(s string, maxLen int) string {
	runes := []rune(s)
	if len(runes) <= maxLen {
		return s
	}
	return string(runes[:maxLen])
}

// defaultContext creates a background context with timeout.
func defaultContext(d time.Duration) (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), d)
}
