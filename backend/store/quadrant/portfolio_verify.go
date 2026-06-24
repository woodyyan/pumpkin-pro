package quadrant

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"strings"
	"sync"
	"time"

	"gorm.io/gorm"
)

// ── verify token store ────────────────────────────────────────────────────────

const verifyTokenTTL = 10 * time.Minute

type verifyTokenEntry struct {
	result    *RankingPortfolioVerifyResult
	expiresAt time.Time
}

var (
	verifyTokenMu    sync.Mutex
	verifyTokenStore = map[string]verifyTokenEntry{}
)

func generateVerifyToken() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

func storeVerifyToken(token string, result *RankingPortfolioVerifyResult) {
	verifyTokenMu.Lock()
	defer verifyTokenMu.Unlock()
	// prune expired tokens
	now := time.Now()
	for k, v := range verifyTokenStore {
		if now.After(v.expiresAt) {
			delete(verifyTokenStore, k)
		}
	}
	verifyTokenStore[token] = verifyTokenEntry{
		result:    result,
		expiresAt: now.Add(verifyTokenTTL),
	}
}

// ConsumeVerifyToken retrieves and removes a verify token. Returns nil if not
// found or expired.
func ConsumeVerifyToken(token string) *RankingPortfolioVerifyResult {
	verifyTokenMu.Lock()
	defer verifyTokenMu.Unlock()
	entry, ok := verifyTokenStore[token]
	if !ok {
		return nil
	}
	delete(verifyTokenStore, token)
	if time.Now().After(entry.expiresAt) {
		return nil
	}
	return entry.result
}

// ── data types ────────────────────────────────────────────────────────────────

// RankingPortfolioVerifyDiff records the discrepancy for a single series point.
type RankingPortfolioVerifyDiff struct {
	Date              string  `json:"date"`
	StoredNav         float64 `json:"stored_nav"`
	RecomputedNav     float64 `json:"recomputed_nav"`
	NavDelta          float64 `json:"nav_delta"`
	StoredDailyPct    float64 `json:"stored_daily_pct"`
	RecomputedDailyPct float64 `json:"recomputed_daily_pct"`
}

// RankingPortfolioVerifyResult is the full result of a verify run for one
// definition.
type RankingPortfolioVerifyResult struct {
	DefinitionID   string                       `json:"definition_id"`
	DefinitionName string                       `json:"definition_name"`
	Exchange       string                       `json:"exchange"`
	TotalPoints    int                          `json:"total_points"`
	DiffCount      int                          `json:"diff_count"`
	HasDiff        bool                         `json:"has_diff"`
	Diffs          []RankingPortfolioVerifyDiff `json:"diffs,omitempty"`
	// VerifyToken is populated when HasDiff==true; pass it to the fix endpoint.
	VerifyToken string `json:"verify_token,omitempty"`
	// Message is a human-readable summary.
	Message string `json:"message"`
}

// RankingPortfolioVerifyAllResult bundles verify results for all definitions.
type RankingPortfolioVerifyAllResult struct {
	Items     []RankingPortfolioVerifyResult `json:"items"`
	HasDiff   bool                           `json:"has_diff"`
	CheckedAt string                         `json:"checked_at"`
}

// ── epsilon for float comparison ─────────────────────────────────────────────

const navEpsilon = 1e-4 // 0.01% tolerance for NAV comparison

func navDiffers(a, b float64) bool {
	if b == 0 {
		return math.Abs(a) > navEpsilon
	}
	return math.Abs(a-b)/math.Abs(b) > navEpsilon
}

// ── core verify logic ─────────────────────────────────────────────────────────

// VerifyRankingPortfolioResult replays the NAV calculation from DB data and
// compares it against the stored series_json.  It is strictly read-only.
func VerifyRankingPortfolioResult(tx *gorm.DB, definition RankingPortfolioDefinition) (*RankingPortfolioVerifyResult, error) {
	result := &RankingPortfolioVerifyResult{
		DefinitionID:   definition.ID,
		DefinitionName: definition.Name,
		Exchange:       definition.Exchange,
	}

	// Load stored result (latest snapshot version).
	var stored RankingPortfolioResult
	if err := tx.Where("definition_id = ?", definition.ID).
		Order("snapshot_date DESC, id DESC").
		First(&stored).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			result.Message = "暂无已存储结果，无法验证"
			return result, nil
		}
		return nil, fmt.Errorf("load stored result: %w", err)
	}

	// Decode stored series.
	var storedSeries []RankingPortfolioSeriesPoint
	if strings.TrimSpace(stored.SeriesJSON) != "" {
		if err := json.Unmarshal([]byte(stored.SeriesJSON), &storedSeries); err != nil {
			return nil, fmt.Errorf("decode stored series: %w", err)
		}
	}
	if len(storedSeries) == 0 {
		result.Message = "已存储序列为空，无需验证"
		return result, nil
	}
	result.TotalPoints = len(storedSeries)

	// Replay calculation using buildRankingPortfolioResult (read-only: uses a
	// nested read-only transaction so no data is written).
	var recomputed *RankingPortfolioResult
	if err := tx.Transaction(func(nested *gorm.DB) error {
		var err error
		recomputed, err = buildRankingPortfolioResult(nested, definition, stored.SnapshotVersion, time.Now().UTC())
		return err
	}); err != nil {
		return nil, fmt.Errorf("recompute result: %w", err)
	}

	var recomputedSeries []RankingPortfolioSeriesPoint
	if strings.TrimSpace(recomputed.SeriesJSON) != "" {
		if err := json.Unmarshal([]byte(recomputed.SeriesJSON), &recomputedSeries); err != nil {
			return nil, fmt.Errorf("decode recomputed series: %w", err)
		}
	}

	// Build index by date for efficient lookup.
	recomputedByDate := make(map[string]RankingPortfolioSeriesPoint, len(recomputedSeries))
	for _, p := range recomputedSeries {
		recomputedByDate[p.Date] = p
	}

	diffs := make([]RankingPortfolioVerifyDiff, 0)
	for _, stored := range storedSeries {
		recomp, ok := recomputedByDate[stored.Date]
		if !ok {
			// Point exists in stored but not recomputed — treat as diff.
			diffs = append(diffs, RankingPortfolioVerifyDiff{
				Date:              stored.Date,
				StoredNav:         stored.Nav,
				RecomputedNav:     0,
				NavDelta:          -stored.Nav,
				StoredDailyPct:    stored.DailyPortfolioReturnPct,
				RecomputedDailyPct: 0,
			})
			continue
		}
		if navDiffers(stored.Nav, recomp.Nav) || navDiffers(stored.DailyPortfolioReturnPct, recomp.DailyPortfolioReturnPct) {
			diffs = append(diffs, RankingPortfolioVerifyDiff{
				Date:              stored.Date,
				StoredNav:         stored.Nav,
				RecomputedNav:     recomp.Nav,
				NavDelta:          recomp.Nav - stored.Nav,
				StoredDailyPct:    stored.DailyPortfolioReturnPct,
				RecomputedDailyPct: recomp.DailyPortfolioReturnPct,
			})
		}
	}

	result.DiffCount = len(diffs)
	result.HasDiff = len(diffs) > 0
	if len(diffs) > 0 {
		result.Diffs = diffs
		result.Message = fmt.Sprintf("发现 %d 个差异点（共 %d 点）", len(diffs), len(storedSeries))
		// Issue a short-lived verify token for the fix endpoint.
		token, err := generateVerifyToken()
		if err != nil {
			return nil, fmt.Errorf("generate verify token: %w", err)
		}
		result.VerifyToken = token
		storeVerifyToken(token, result)
	} else {
		result.Message = fmt.Sprintf("全部 %d 个点均一致，无需修复", len(storedSeries))
	}
	return result, nil
}
