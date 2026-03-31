package quadrant

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// Service provides business logic for quadrant scores.
type Service struct {
	repo *Repository
}

func NewService(repo *Repository) *Service {
	return &Service{repo: repo}
}

// BulkSave writes all quadrant scores from the Quant callback.
func (s *Service) BulkSave(ctx context.Context, input BulkSaveInput) (int, error) {
	computedAt := time.Now().UTC()
	if input.ComputedAt != "" {
		if parsed, err := time.Parse(time.RFC3339, input.ComputedAt); err == nil {
			computedAt = parsed.UTC()
		}
	}

	records := make([]QuadrantScoreRecord, 0, len(input.Items))
	for _, item := range input.Items {
		code := strings.TrimSpace(item.Code)
		if code == "" {
			continue
		}
		records = append(records, QuadrantScoreRecord{
			Code:        code,
			Name:        strings.TrimSpace(item.Name),
			Opportunity: item.Opportunity,
			Risk:        item.Risk,
			Quadrant:    strings.TrimSpace(item.Quadrant),
			Trend:       item.Trend,
			Flow:        item.Flow,
			Revision:    item.Revision,
			Volatility:  item.Volatility,
			Drawdown:    item.Drawdown,
			Crowding:    item.Crowding,
			ComputedAt:  computedAt,
		})
	}

	if len(records) == 0 {
		return 0, fmt.Errorf("no valid items to save")
	}

	if err := s.repo.BulkUpsert(ctx, records); err != nil {
		return 0, err
	}
	return len(records), nil
}

// GetAllWithWatchlist returns all scores (compact) + watchlist details.
func (s *Service) GetAllWithWatchlist(ctx context.Context, watchlistCodes []string) (*QuadrantResponse, error) {
	allRecords, err := s.repo.FindAll(ctx)
	if err != nil {
		return nil, err
	}

	// Build compact list + summary
	allStocks := make([]QuadrantScoreCompact, 0, len(allRecords))
	summary := QuadrantSummary{}
	var latestComputedAt time.Time

	for _, r := range allRecords {
		allStocks = append(allStocks, r.ToCompact())

		switch r.Quadrant {
		case "机会":
			summary.OpportunityZone++
		case "拥挤":
			summary.CrowdedZone++
		case "泡沫":
			summary.BubbleZone++
		case "防御":
			summary.DefensiveZone++
		default:
			summary.NeutralZone++
		}

		if r.ComputedAt.After(latestComputedAt) {
			latestComputedAt = r.ComputedAt
		}
	}

	// Build watchlist details
	watchlistDetails := make([]QuadrantScoreDetail, 0, len(watchlistCodes))
	if len(watchlistCodes) > 0 {
		watchlistRecords, err := s.repo.FindBySymbols(ctx, watchlistCodes)
		if err == nil {
			for _, r := range watchlistRecords {
				watchlistDetails = append(watchlistDetails, r.ToDetail())
			}
		}
	}

	computedAtStr := ""
	if !latestComputedAt.IsZero() {
		computedAtStr = latestComputedAt.UTC().Format(time.RFC3339)
	}

	return &QuadrantResponse{
		Meta: QuadrantMeta{
			ComputedAt: computedAtStr,
			TotalCount: len(allRecords),
		},
		AllStocks:        allStocks,
		WatchlistDetails: watchlistDetails,
		Summary:          summary,
	}, nil
}

// GetStatus returns the current computation status.
func (s *Service) GetStatus(ctx context.Context) (*QuadrantStatusResponse, error) {
	count, err := s.repo.Count(ctx)
	if err != nil {
		return nil, err
	}

	latestAt, err := s.repo.GetLatestComputedAt(ctx)
	if err != nil {
		return nil, err
	}

	computedAtStr := ""
	if latestAt != nil {
		computedAtStr = latestAt.UTC().Format(time.RFC3339)
	}

	return &QuadrantStatusResponse{
		LastComputedAt: computedAtStr,
		StockCount:     int(count),
		LastError:      "",
	}, nil
}
