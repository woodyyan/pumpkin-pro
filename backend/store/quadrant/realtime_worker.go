package quadrant

import (
	"context"
	"log"
	"strings"
	"sync"
	"time"
)

// RealtimeQuote is a single intraday quote for a symbol, used to refresh the
// "current constituent" gain display. It is decoupled from the live module to
// avoid an import cycle; callers inject a fetcher that returns these values.
type RealtimeQuote struct {
	Code      string
	Exchange  string
	LastPrice float64
	// IsOpen marks the 09:25 call-auction open price, which is additionally
	// persisted as the simulated buy (entry) price for the trading day.
	IsOpen bool
}

// RealtimeQuoteFetcher fetches the latest quotes for the given symbols.
// Symbols are passed as "code|exchange" via RealtimeSymbol values.
type RealtimeQuoteFetcher func(ctx context.Context, symbols []RealtimeSymbol) ([]RealtimeQuote, error)

// RealtimeSymbol identifies a stock to quote.
type RealtimeSymbol struct {
	Code     string
	Exchange string
}

const (
	realtimeWorkerLogPrefix = "[ranking-portfolio-realtime]"
	realtimeScopeAShare     = "ASHARE"
	realtimeScopeHK         = "HKEX"
)

// defaultAShareRefreshPoints lists the Beijing-time refresh points for A-share.
// 09:25 = call-auction open (entry price); intraday every half hour; 15:30 =
// post-close delayed final refresh to capture the settled close price.
func defaultAShareRefreshPoints() []string {
	return []string{
		"09:25", "09:30", "10:00", "10:30", "11:00", "11:30",
		"13:00", "13:30", "14:00", "14:30", "15:00", "15:30",
	}
}

// defaultHKRefreshPoints lists the Beijing-time refresh points for HK (UTC+8).
// 09:25 = call-auction open; intraday every half hour (incl. 12:00 before the
// 12:00–13:00 lunch break); 16:30 = post-close delayed final refresh.
func defaultHKRefreshPoints() []string {
	return []string{
		"09:25", "09:30", "10:00", "10:30", "11:00", "11:30", "12:00",
		"13:00", "13:30", "14:00", "14:30", "15:00", "15:30", "16:00", "16:30",
	}
}

const realtimeOpenPoint = "09:25"

// RealtimeWorkerConfig configures the per-market intraday refresh schedule.
// All times are interpreted in Beijing time (Asia/Shanghai).
type RealtimeWorkerConfig struct {
	Enabled      bool
	ASharePoints []string // "HH:MM" Beijing time
	HKPoints     []string // "HH:MM" Beijing time
	NowFunc      func() time.Time
}

func normalizeRealtimeWorkerConfig(cfg RealtimeWorkerConfig) RealtimeWorkerConfig {
	cfg.ASharePoints = sanitizeRefreshPoints(cfg.ASharePoints)
	if len(cfg.ASharePoints) == 0 {
		cfg.ASharePoints = defaultAShareRefreshPoints()
	}
	cfg.HKPoints = sanitizeRefreshPoints(cfg.HKPoints)
	if len(cfg.HKPoints) == 0 {
		cfg.HKPoints = defaultHKRefreshPoints()
	}
	if cfg.NowFunc == nil {
		cfg.NowFunc = time.Now
	}
	return cfg
}

// sanitizeRefreshPoints keeps only valid "HH:MM" entries, sorted ascending and
// de-duplicated. Invalid entries are dropped.
func sanitizeRefreshPoints(points []string) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(points))
	for _, p := range points {
		p = strings.TrimSpace(p)
		if _, ok := parseRefreshPoint(p); !ok {
			continue
		}
		if _, dup := seen[p]; dup {
			continue
		}
		seen[p] = struct{}{}
		out = append(out, p)
	}
	// simple insertion sort by minutes-of-day for stable ordering
	for i := 1; i < len(out); i++ {
		for j := i; j > 0; j-- {
			a, _ := parseRefreshPoint(out[j-1])
			b, _ := parseRefreshPoint(out[j])
			if a <= b {
				break
			}
			out[j-1], out[j] = out[j], out[j-1]
		}
	}
	return out
}

// parseRefreshPoint parses "HH:MM" into minutes-of-day. ok=false when invalid.
func parseRefreshPoint(p string) (int, bool) {
	parts := strings.Split(strings.TrimSpace(p), ":")
	if len(parts) != 2 {
		return 0, false
	}
	h := atoiSafe(parts[0])
	m := atoiSafe(parts[1])
	if h < 0 || h > 23 || m < 0 || m > 59 {
		return 0, false
	}
	if len(parts[0]) == 0 || len(parts[1]) == 0 {
		return 0, false
	}
	return h*60 + m, true
}

func atoiSafe(s string) int {
	n := 0
	for _, c := range s {
		if c < '0' || c > '9' {
			return -1
		}
		n = n*10 + int(c-'0')
	}
	return n
}

// nextRealtimeTriggerAt returns the next scheduled refresh time (Beijing time)
// strictly after `now`, given the sorted "HH:MM" points. Weekends are skipped.
// The returned time carries the Beijing-time location.
func nextRealtimeTriggerAt(now time.Time, points []string) time.Time {
	loc := beijingLocation()
	nowBJ := now.In(loc)
	for dayOffset := 0; dayOffset < 8; dayOffset++ {
		day := nowBJ.AddDate(0, 0, dayOffset)
		if day.Weekday() == time.Saturday || day.Weekday() == time.Sunday {
			continue
		}
		for _, p := range points {
			mins, ok := parseRefreshPoint(p)
			if !ok {
				continue
			}
			candidate := time.Date(day.Year(), day.Month(), day.Day(), mins/60, mins%60, 0, 0, loc)
			if candidate.After(nowBJ) {
				return candidate
			}
		}
	}
	// Fallback: shouldn't happen, return now+24h
	return nowBJ.Add(24 * time.Hour)
}

// isOpenAuctionPoint reports whether the given Beijing-time instant matches the
// 09:25 call-auction point (used to also persist the entry open price).
func isOpenAuctionPoint(at time.Time) bool {
	bj := at.In(beijingLocation())
	mins, _ := parseRefreshPoint(realtimeOpenPoint)
	return bj.Hour()*60+bj.Minute() == mins
}

func beijingLocation() *time.Location {
	loc, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		return time.FixedZone("CST", 8*3600)
	}
	return loc
}

// RealtimeWorker periodically refreshes intraday quotes for current constituents
// at fixed Beijing-time points, per market. The 09:25 point additionally fills
// the simulated entry (open) price for the trading day.
type RealtimeWorker struct {
	service *Service
	fetcher RealtimeQuoteFetcher
	cfg     RealtimeWorkerConfig
	mu      sync.Mutex
	lastRun map[string]time.Time
}

// NewRealtimeWorker builds a realtime worker. `fetcher` supplies live quotes.
func NewRealtimeWorker(service *Service, fetcher RealtimeQuoteFetcher, cfg RealtimeWorkerConfig) *RealtimeWorker {
	return &RealtimeWorker{
		service: service,
		fetcher: fetcher,
		cfg:     normalizeRealtimeWorkerConfig(cfg),
		lastRun: map[string]time.Time{},
	}
}

// Start launches per-market schedule loops.
func (w *RealtimeWorker) Start(ctx context.Context) {
	if !w.cfg.Enabled || w.service == nil || w.fetcher == nil {
		log.Printf("%s disabled", realtimeWorkerLogPrefix)
		return
	}
	go w.scheduleLoop(ctx, realtimeScopeAShare, w.cfg.ASharePoints)
	go w.scheduleLoop(ctx, realtimeScopeHK, w.cfg.HKPoints)
	log.Printf("%s started (A-share %v / HK %v, Beijing time)", realtimeWorkerLogPrefix, w.cfg.ASharePoints, w.cfg.HKPoints)
}

func (w *RealtimeWorker) scheduleLoop(ctx context.Context, scope string, points []string) {
	for {
		now := w.cfg.NowFunc()
		next := nextRealtimeTriggerAt(now, points)
		wait := next.Sub(now)
		if wait < 0 {
			wait = time.Second
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(wait):
			if err := w.RunOnce(ctx, scope, w.cfg.NowFunc()); err != nil {
				log.Printf("%s %s refresh failed: %v", realtimeWorkerLogPrefix, scope, err)
			}
		}
	}
}

// RunOnce refreshes quotes for the given market scope at instant `at`.
// It persists the latest price for every current constituent, and when `at` is
// the 09:25 call-auction point it also fills the entry open price.
func (w *RealtimeWorker) RunOnce(ctx context.Context, scope string, at time.Time) error {
	symbols, err := w.service.collectCurrentConstituentSymbols(ctx, scope)
	if err != nil {
		return err
	}
	if len(symbols) == 0 {
		return nil
	}
	quotes, err := w.fetcher(ctx, symbols)
	if err != nil {
		return err
	}
	fillOpen := isOpenAuctionPoint(at)
	if fillOpen {
		// At the 09:25 call auction, every fetched last price is the open price.
		for i := range quotes {
			quotes[i].IsOpen = true
		}
	}
	w.markRun(scope, at)
	return w.service.persistRealtimeQuotes(ctx, scope, quotes, fillOpen, at)
}

func (w *RealtimeWorker) markRun(scope string, at time.Time) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.lastRun[scope] = at
}
