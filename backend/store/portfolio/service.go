package portfolio

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

type Service struct {
	repo *Repository
}

func NewService(repo *Repository) *Service {
	return &Service{repo: repo}
}

func (s *Service) ListByUser(ctx context.Context, userID string) ([]PortfolioItem, error) {
	records, err := s.repo.ListByUser(ctx, userID)
	if err != nil {
		return nil, err
	}
	items := make([]PortfolioItem, 0, len(records))
	for _, r := range records {
		items = append(items, r.toItem())
	}
	return items, nil
}

func (s *Service) GetBySymbol(ctx context.Context, userID, symbol string) (*PortfolioItem, error) {
	record, err := s.repo.GetBySymbol(ctx, userID, symbol)
	if err != nil {
		return nil, err
	}
	item := record.toItem()
	return &item, nil
}

func (s *Service) Upsert(ctx context.Context, userID, symbol string, input UpsertPortfolioInput) (*PortfolioItem, error) {
	symbol = strings.TrimSpace(strings.ToUpper(symbol))
	if symbol == "" {
		return nil, fmt.Errorf("symbol is required")
	}
	if input.Shares < 0 {
		return nil, fmt.Errorf("shares must be >= 0")
	}
	if input.AvgCostPrice < 0 {
		return nil, fmt.Errorf("avg_cost_price must be >= 0")
	}

	now := time.Now().UTC()
	record := &PortfolioRecord{
		ID:           uuid.New().String(),
		UserID:       userID,
		Symbol:       symbol,
		Shares:       input.Shares,
		AvgCostPrice: input.AvgCostPrice,
		BuyDate:      strings.TrimSpace(input.BuyDate),
		Note:         strings.TrimSpace(input.Note),
		CreatedAt:    now,
		UpdatedAt:    now,
	}

	if err := s.repo.Upsert(ctx, record); err != nil {
		return nil, err
	}

	saved, err := s.repo.GetBySymbol(ctx, userID, symbol)
	if err != nil {
		return nil, err
	}
	item := saved.toItem()
	return &item, nil
}

func (s *Service) Delete(ctx context.Context, userID, symbol string) error {
	return s.repo.Delete(ctx, userID, symbol)
}

// ── Investment Profile ──

func (s *Service) GetInvestmentProfile(ctx context.Context, userID string) (*InvestmentProfile, error) {
	record, err := s.repo.GetInvestmentProfile(ctx, userID)
	if err != nil {
		return nil, err
	}
	profile := record.toProfile()
	return &profile, nil
}

func (s *Service) UpsertInvestmentProfile(ctx context.Context, userID string, input UpsertInvestmentProfileInput) (*InvestmentProfile, error) {
	if input.TotalCapital < 0 {
		return nil, fmt.Errorf("total_capital must be >= 0")
	}
	if input.MaxDrawdownPct < 0 || input.MaxDrawdownPct > 100 {
		return nil, fmt.Errorf("max_drawdown_pct must be between 0 and 100")
	}

	now := time.Now().UTC()
	record := &InvestmentProfileRecord{
		UserID:            userID,
		TotalCapital:      input.TotalCapital,
		RiskPreference:    strings.TrimSpace(input.RiskPreference),
		InvestmentGoal:    strings.TrimSpace(input.InvestmentGoal),
		InvestmentHorizon: strings.TrimSpace(input.InvestmentHorizon),
		MaxDrawdownPct:    input.MaxDrawdownPct,
		ExperienceLevel:   strings.TrimSpace(input.ExperienceLevel),
		Note:              strings.TrimSpace(input.Note),
		UpdatedAt:         now,
	}

	if err := s.repo.UpsertInvestmentProfile(ctx, record); err != nil {
		return nil, err
	}

	saved, err := s.repo.GetInvestmentProfile(ctx, userID)
	if err != nil {
		return nil, err
	}
	profile := saved.toProfile()
	return &profile, nil
}
