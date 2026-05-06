package companyprofile

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/woodyyan/pumpkin-pro/backend/store/live"
)

type Service struct {
	repo     *Repository
	quantURL string
	mu       sync.Mutex
	refresh  CompanyProfileRefreshStatus
}

func NewService(repo *Repository) *Service {
	return &Service{repo: repo, refresh: CompanyProfileRefreshStatus{Status: "idle"}}
}

func (s *Service) SetQuantServiceURL(url string) {
	s.quantURL = strings.TrimRight(strings.TrimSpace(url), "/")
}

func (s *Service) GetAbout(ctx context.Context, rawSymbol string) (*CompanyAboutPayload, error) {
	symbol, exchange, err := live.NormalizeSymbol(rawSymbol)
	if err != nil {
		return nil, err
	}
	if s == nil || s.repo == nil {
		return pendingPayload(symbol, exchange), nil
	}
	record, err := s.repo.GetBySymbol(ctx, symbol)
	if err != nil {
		return nil, err
	}
	if record == nil {
		return pendingPayload(symbol, exchange), nil
	}
	return payloadFromRecord(record), nil
}

func (s *Service) AdminOverview(ctx context.Context) (*AdminCompanyProfileOverview, error) {
	coverage, err := s.repo.Coverage(ctx)
	if err != nil {
		return nil, err
	}
	failures, err := s.repo.ListFailureItems(ctx, 30)
	if err != nil {
		return nil, err
	}
	return &AdminCompanyProfileOverview{Coverage: coverage, Failures: failures, Refresh: s.RefreshStatus(), UpdatedAt: time.Now().UTC().Format(time.RFC3339)}, nil
}

func (s *Service) RefreshStatus() CompanyProfileRefreshStatus {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.refresh
}

func (s *Service) StartManualRefresh(ctx context.Context, req CompanyProfileRefreshRequest) (CompanyProfileRefreshStatus, error) {
	s.mu.Lock()
	if s.refresh.Running {
		st := s.refresh
		s.mu.Unlock()
		return st, fmt.Errorf("company profile refresh already running")
	}
	started := time.Now().UTC().Format(time.RFC3339)
	s.refresh = CompanyProfileRefreshStatus{Running: true, Status: "running", StartedAt: started, Message: "正在刷新公司静态资料"}
	s.mu.Unlock()
	go s.runRefresh(context.Background(), req)
	return s.RefreshStatus(), nil
}

func (s *Service) runRefresh(ctx context.Context, req CompanyProfileRefreshRequest) {
	finish := func(update func(*CompanyProfileRefreshStatus)) { s.mu.Lock(); update(&s.refresh); s.mu.Unlock() }
	universe, err := s.repo.ListLatestUniverse(ctx, req.Exchange, req.Limit)
	if err != nil {
		finish(func(st *CompanyProfileRefreshStatus) {
			st.Running = false
			st.Status = "failed"
			st.Error = err.Error()
			st.FinishedAt = time.Now().UTC().Format(time.RFC3339)
		})
		return
	}
	existing, _ := s.repo.ListProfiles(ctx)
	existingMap := map[string]CompanyProfileRecord{}
	for _, p := range existing {
		existingMap[p.Symbol] = p
	}
	symbols := make([]string, 0, len(universe))
	universeSet := map[string]struct{}{}
	newCount := 0
	for _, u := range universe {
		symbols = append(symbols, u.Symbol)
		universeSet[u.Symbol] = struct{}{}
		if _, ok := existingMap[u.Symbol]; !ok {
			newCount++
		}
	}
	finish(func(st *CompanyProfileRefreshStatus) { st.TotalCount = len(symbols); st.NewCount = newCount })
	items, err := s.fetchFromQuant(ctx, symbols)
	if err != nil {
		finish(func(st *CompanyProfileRefreshStatus) {
			st.Running = false
			st.Status = "failed"
			st.Error = err.Error()
			st.FinishedAt = time.Now().UTC().Format(time.RFC3339)
		})
		return
	}
	if err := s.repo.BulkUpsert(ctx, items); err != nil {
		finish(func(st *CompanyProfileRefreshStatus) {
			st.Running = false
			st.Status = "failed"
			st.Error = err.Error()
			st.FinishedAt = time.Now().UTC().Format(time.RFC3339)
		})
		return
	}
	delisted := []string{}
	if len(universeSet) > 0 {
		for _, p := range existing {
			if p.ListingStatus != ListingStatusDelisted {
				if _, ok := universeSet[p.Symbol]; !ok {
					delisted = append(delisted, p.Symbol)
				}
			}
		}
	}
	_ = s.repo.MarkSymbolsDelisted(ctx, delisted)
	finish(func(st *CompanyProfileRefreshStatus) {
		st.Running = false
		st.Status = "completed"
		st.SuccessCount = len(items)
		st.FailedCount = len(symbols) - len(items)
		st.DelistedCount = len(delisted)
		st.Message = "公司静态资料刷新完成"
		st.FinishedAt = time.Now().UTC().Format(time.RFC3339)
	})
}

func (s *Service) fetchFromQuant(ctx context.Context, symbols []string) ([]CompanyProfileRecord, error) {
	if strings.TrimSpace(s.quantURL) == "" {
		return nil, fmt.Errorf("quant service url is empty")
	}
	body, _ := json.Marshal(map[string]any{"symbols": symbols})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.quantURL+"/api/company-profiles/sync", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	respBody, readErr := io.ReadAll(resp.Body)
	if readErr != nil {
		return nil, readErr
	}
	var payload struct {
		Items []CompanyProfileRecord `json:"items"`
		Error string                 `json:"detail"`
	}
	decodeErr := json.Unmarshal(respBody, &payload)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		if payload.Error != "" {
			return nil, fmt.Errorf("quant company profile sync failed: %s", payload.Error)
		}
		text := strings.TrimSpace(string(respBody))
		if text != "" {
			return nil, fmt.Errorf("quant company profile sync returned %d: %s", resp.StatusCode, text)
		}
		return nil, fmt.Errorf("quant company profile sync returned %d", resp.StatusCode)
	}
	if decodeErr != nil {
		return nil, fmt.Errorf("decode quant company profile sync response: %w", decodeErr)
	}
	return payload.Items, nil
}

func pendingPayload(symbol, exchange string) *CompanyAboutPayload {
	return &CompanyAboutPayload{
		Symbol:     symbol,
		Exchange:   exchange,
		HasProfile: false,
		Profile:    nil,
		Meta: CompanyAboutMeta{
			ProfileStatus: ProfileStatusPending,
			QualityFlags:  []string{},
			Message:       "资料整理中，暂未收录该公司的静态资料。",
		},
	}
}

func payloadFromRecord(record *CompanyProfileRecord) *CompanyAboutPayload {
	if record == nil {
		return nil
	}
	return &CompanyAboutPayload{
		Symbol:     record.Symbol,
		Exchange:   record.Exchange,
		HasProfile: true,
		Profile: &CompanyAboutProfile{
			Name:                  record.Name,
			FullName:              record.FullName,
			BoardCode:             record.BoardCode,
			BoardName:             record.BoardName,
			RawIndustryName:       record.RawIndustryName,
			IndustryCode:          record.IndustryCode,
			IndustryName:          record.IndustryName,
			IndustryLevel:         record.IndustryLevel,
			IndustrySource:        record.IndustrySource,
			Website:               record.Website,
			FoundedDate:           record.FoundedDate,
			FoundedDatePrecision:  record.FoundedDatePrecision,
			IPODate:               record.IPODate,
			ListingStatus:         normalizeListingStatus(record.ListingStatus),
			DelistedDate:          record.DelistedDate,
			BusinessSummary:       record.BusinessSummary,
			BusinessSummarySource: record.BusinessSummarySource,
			BusinessScope:         record.BusinessScope,
		},
		Meta: CompanyAboutMeta{
			ProfileStatus:   normalizeProfileStatus(record.ProfileStatus),
			Source:          record.Source,
			SourceURL:       record.SourceURL,
			SourceUpdatedAt: formatTime(record.SourceUpdatedAt),
			UpdatedAt:       formatTime(record.UpdatedAt),
			QualityFlags:    parseQualityFlags(record.QualityFlags),
		},
	}
}

func parseQualityFlags(raw string) []string {
	text := strings.TrimSpace(raw)
	if text == "" {
		return []string{}
	}
	var flags []string
	if err := json.Unmarshal([]byte(text), &flags); err == nil && flags != nil {
		return flags
	}
	return []string{text}
}

func formatTime(input time.Time) string {
	if input.IsZero() {
		return ""
	}
	return input.UTC().Format(time.RFC3339)
}
