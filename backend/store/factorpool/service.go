package factorpool

import (
	"context"
	"errors"
	"sort"
	"strings"
	"time"

	"github.com/woodyyan/pumpkin-pro/backend/store/factorindex"
	"github.com/woodyyan/pumpkin-pro/backend/store/live"
)

const (
	ExchangeAShare = factorindex.ExchangeAShare
	defaultTopN    = 10
)

// SnapshotProvider decouples the monthly factor facts from the realtime quote
// implementation. Quotes enrich a response only; they never change ranks or
// constituents and a quote failure degrades to an otherwise usable pool.
type SnapshotProvider interface {
	GetDetailedSnapshots(ctx context.Context, symbols []string) ([]live.DetailedSymbolSnapshot, error)
}

type Service struct {
	repo      *factorindex.Repository
	snapshots SnapshotProvider
}

func NewService(repo *factorindex.Repository, snapshots SnapshotProvider) *Service {
	return &Service{repo: repo, snapshots: snapshots}
}

type Quote struct {
	LastPrice  *float64 `json:"last_price"`
	ChangeRate *float64 `json:"change_rate"`
	Turnover   *float64 `json:"turnover"`
	UpdatedAt  string   `json:"updated_at,omitempty"`
	Status     string   `json:"status"`
}

type Item struct {
	Rank             int     `json:"rank"`
	Code             string  `json:"code"`
	Symbol           string  `json:"symbol"`
	Name             string  `json:"name"`
	Exchange         string  `json:"exchange"`
	Industry         string  `json:"industry,omitempty"`
	FactorScore      float64 `json:"factor_score"`
	SignalClosePrice float64 `json:"signal_close_price"`
	Quote            Quote   `json:"quote"`
}

type List struct {
	IndexID                 string `json:"index_id"`
	FactorKey               string `json:"factor_key"`
	Name                    string `json:"name"`
	SourceTradeDate         string `json:"source_trade_date,omitempty"`
	RebalanceDate           string `json:"rebalance_date,omitempty"`
	EffectiveStartDate      string `json:"effective_start_date,omitempty"`
	Status                  string `json:"status"`
	WarningText             string `json:"warning_text,omitempty"`
	CurrentConstituentCount int    `json:"current_constituent_count"`
	Items                   []Item `json:"items"`
}

type Response struct {
	Exchange        string `json:"exchange"`
	SourceTradeDate string `json:"source_trade_date,omitempty"`
	MarketStatus    string `json:"market_status"`
	PriceLabel      string `json:"price_label"`
	QuoteUpdatedAt  string `json:"quote_updated_at,omitempty"`
	QuoteStatus     string `json:"quote_status"`
	Lists           []List `json:"lists"`
}

func (s *Service) GetCurrent(ctx context.Context, exchange string) (*Response, error) {
	if s == nil || s.repo == nil {
		return nil, errors.New("factor pool service is unavailable")
	}
	market := strings.ToUpper(strings.TrimSpace(exchange))
	if market == "" {
		market = ExchangeAShare
	}
	sources, err := s.repo.ListCurrentPoolSources(ctx, market, defaultTopN)
	if err != nil {
		return nil, err
	}
	response := &Response{
		Exchange:     market,
		MarketStatus: "closed",
		PriceLabel:   "最近收盘价",
		QuoteStatus:  "unavailable",
		Lists:        make([]List, 0, len(sources)),
	}
	if live.IsAShareTradingHours() {
		response.MarketStatus = "trading"
		response.PriceLabel = "最新价"
	}

	symbols := make([]string, 0, len(sources)*defaultTopN)
	for _, source := range sources {
		list := buildList(source)
		response.SourceTradeDate = maxDate(response.SourceTradeDate, list.SourceTradeDate)
		for _, item := range list.Items {
			if item.Symbol != "" {
				symbols = append(symbols, item.Symbol)
			}
		}
		response.Lists = append(response.Lists, list)
	}

	quotes, quoteOK := s.fetchQuotes(ctx, symbols)
	if !quoteOK {
		return response, nil
	}
	enrichedCount := 0
	for listIndex := range response.Lists {
		for itemIndex := range response.Lists[listIndex].Items {
			item := &response.Lists[listIndex].Items[itemIndex]
			quote, ok := quotes[item.Symbol]
			if !ok || quote.LastPrice <= 0 {
				continue
			}
			item.Quote = quoteFromSnapshot(quote)
			response.QuoteUpdatedAt = maxTimestamp(response.QuoteUpdatedAt, quote.TS)
			enrichedCount++
		}
	}
	if enrichedCount > 0 {
		response.QuoteStatus = "live"
	}
	return response, nil
}

func (s *Service) fetchQuotes(ctx context.Context, symbols []string) (map[string]live.DetailedSymbolSnapshot, bool) {
	if s.snapshots == nil || len(symbols) == 0 {
		return map[string]live.DetailedSymbolSnapshot{}, false
	}
	unique := make(map[string]struct{}, len(symbols))
	cleanSymbols := make([]string, 0, len(symbols))
	for _, symbol := range symbols {
		value := strings.ToUpper(strings.TrimSpace(symbol))
		if value == "" {
			continue
		}
		if _, exists := unique[value]; exists {
			continue
		}
		unique[value] = struct{}{}
		cleanSymbols = append(cleanSymbols, value)
	}
	sort.Strings(cleanSymbols)
	items, err := s.snapshots.GetDetailedSnapshots(ctx, cleanSymbols)
	if err != nil {
		return map[string]live.DetailedSymbolSnapshot{}, false
	}
	result := make(map[string]live.DetailedSymbolSnapshot, len(items))
	for _, item := range items {
		if strings.TrimSpace(item.Symbol) != "" {
			result[strings.ToUpper(strings.TrimSpace(item.Symbol))] = item
		}
	}
	return result, true
}

func buildList(source factorindex.PoolSource) List {
	list := List{
		IndexID:   source.Definition.ID,
		FactorKey: source.Definition.FactorKey,
		Name:      source.Definition.Name,
		Status:    factorindex.StatusPending,
		Items:     make([]Item, 0, len(source.Items)),
	}
	if source.Daily != nil {
		list.SourceTradeDate = source.Daily.SourceTradeDate
		list.Status = source.Daily.Status
		list.WarningText = source.Daily.WarningText
		list.CurrentConstituentCount = source.Daily.ConstituentCount
	}
	if source.Rebalance != nil {
		list.RebalanceDate = source.Rebalance.SignalDate
		list.EffectiveStartDate = source.Rebalance.EffectiveStartDate
	}
	for _, constituent := range source.Items {
		list.Items = append(list.Items, Item{
			Rank:             constituent.Rank,
			Code:             constituent.StockCode,
			Symbol:           stockSymbol(constituent.StockCode, constituent.Exchange),
			Name:             constituent.StockName,
			Exchange:         constituent.Exchange,
			Industry:         constituent.Industry,
			FactorScore:      constituent.FactorScore,
			SignalClosePrice: constituent.SignalClosePrice,
			Quote:            Quote{Status: "unavailable"},
		})
	}
	return list
}

func stockSymbol(code, exchange string) string {
	code = strings.ToUpper(strings.TrimSpace(code))
	switch strings.ToUpper(strings.TrimSpace(exchange)) {
	case "SSE":
		return code + ".SH"
	case "SZSE":
		return code + ".SZ"
	default:
		return ""
	}
}

func quoteFromSnapshot(snapshot live.DetailedSymbolSnapshot) Quote {
	lastPrice := snapshot.LastPrice
	changeRate := snapshot.ChangeRate
	turnover := snapshot.Turnover
	return Quote{
		LastPrice:  &lastPrice,
		ChangeRate: &changeRate,
		Turnover:   &turnover,
		UpdatedAt:  snapshot.TS,
		Status:     "live",
	}
}

func maxDate(left, right string) string {
	if strings.TrimSpace(right) > strings.TrimSpace(left) {
		return strings.TrimSpace(right)
	}
	return strings.TrimSpace(left)
}

func maxTimestamp(left, right string) string {
	if strings.TrimSpace(right) == "" {
		return strings.TrimSpace(left)
	}
	if strings.TrimSpace(left) == "" {
		return strings.TrimSpace(right)
	}
	leftTime, leftErr := time.Parse(time.RFC3339, left)
	rightTime, rightErr := time.Parse(time.RFC3339, right)
	if leftErr == nil && rightErr == nil && rightTime.After(leftTime) {
		return right
	}
	return left
}
