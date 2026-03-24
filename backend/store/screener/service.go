package screener

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

const maxWatchlistsPerUser = 20

type Service struct {
	repo *Repository
}

func NewService(repo *Repository) *Service {
	return &Service{repo: repo}
}

// List returns all watchlists for the user.
func (s *Service) List(ctx context.Context, userID string) ([]Watchlist, error) {
	if strings.TrimSpace(userID) == "" {
		return nil, ErrForbidden
	}
	return s.repo.List(ctx, userID)
}

// Create saves a new watchlist.
func (s *Service) Create(ctx context.Context, userID string, input CreateWatchlistInput) (*WatchlistDetail, error) {
	if strings.TrimSpace(userID) == "" {
		return nil, ErrForbidden
	}
	name := strings.TrimSpace(input.Name)
	if name == "" {
		return nil, fmt.Errorf("%w: 自选表名称不能为空", ErrInvalid)
	}
	if len([]rune(name)) > 64 {
		return nil, fmt.Errorf("%w: 自选表名称过长", ErrInvalid)
	}
	if len(input.Stocks) == 0 {
		return nil, fmt.Errorf("%w: 至少需要包含一只股票", ErrInvalid)
	}
	if len(input.Stocks) > 500 {
		return nil, fmt.Errorf("%w: 单个自选表最多 500 只股票", ErrInvalid)
	}

	// Check per-user limit
	count, err := s.repo.CountByUser(ctx, userID)
	if err != nil {
		return nil, err
	}
	if count >= maxWatchlistsPerUser {
		return nil, fmt.Errorf("%w: 每个用户最多保存 %d 个自选表", ErrLimit, maxWatchlistsPerUser)
	}

	now := time.Now().UTC()
	wlID := uuid.New().String()

	wl := WatchlistRecord{
		ID:        wlID,
		UserID:    userID,
		Name:      name,
		CreatedAt: now,
		UpdatedAt: now,
	}

	// Deduplicate stocks by code
	seen := map[string]struct{}{}
	stocks := make([]WatchlistStockRecord, 0, len(input.Stocks))
	for _, s := range input.Stocks {
		code := strings.TrimSpace(s.Code)
		if code == "" {
			continue
		}
		if _, ok := seen[code]; ok {
			continue
		}
		seen[code] = struct{}{}
		stocks = append(stocks, WatchlistStockRecord{
			ID:          uuid.New().String(),
			WatchlistID: wlID,
			Code:        code,
			Name:        strings.TrimSpace(s.Name),
			CreatedAt:   now,
		})
	}

	if err := s.repo.Create(ctx, wl, stocks); err != nil {
		return nil, err
	}

	detail := wl.toDetail(stocks)
	return &detail, nil
}

// GetByID loads a watchlist with all its stocks.
func (s *Service) GetByID(ctx context.Context, userID, id string) (*WatchlistDetail, error) {
	if strings.TrimSpace(userID) == "" {
		return nil, ErrForbidden
	}
	wl, stocks, err := s.repo.GetByID(ctx, userID, id)
	if err != nil {
		return nil, err
	}
	detail := wl.toDetail(stocks)
	return &detail, nil
}

// Delete removes a watchlist.
func (s *Service) Delete(ctx context.Context, userID, id string) error {
	if strings.TrimSpace(userID) == "" {
		return ErrForbidden
	}
	return s.repo.Delete(ctx, userID, id)
}
