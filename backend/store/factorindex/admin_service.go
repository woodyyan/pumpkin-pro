package factorindex

import (
	"context"
	"fmt"
	"strings"
	"time"
)

const (
	OperationSyncAll        = "sync_all"
	OperationSyncDaily      = "sync_daily"
	OperationSyncRebalances = "sync_rebalances"
)

type ManualRunRequest struct {
	Operation string `json:"operation"`
	FactorKey string `json:"factor_key,omitempty"`
	FromDate  string `json:"from_date,omitempty"`
	ToDate    string `json:"to_date,omitempty"`
	Reset     bool   `json:"reset,omitempty"`
}

type AdminStatusResponse struct {
	LatestSnapshotDate string            `json:"latest_snapshot_date,omitempty"`
	LatestTradeDate    string            `json:"latest_trade_date,omitempty"`
	Worker             WorkerStatus      `json:"worker"`
	Items              []AdminStatusItem `json:"items"`
}

type AdminStatusItem struct {
	IndexID             string     `json:"index_id"`
	FactorKey           string     `json:"factor_key"`
	Name                string     `json:"name"`
	Status              string     `json:"status"`
	WarningText         string     `json:"warning_text,omitempty"`
	NAV                 *float64   `json:"nav,omitempty"`
	LatestTradeDate     string     `json:"latest_trade_date,omitempty"`
	SourceTradeDate     string     `json:"source_trade_date,omitempty"`
	DailyComputedAt     *time.Time `json:"daily_computed_at,omitempty"`
	RebalanceDate       string     `json:"rebalance_date,omitempty"`
	EffectiveStartDate  string     `json:"effective_start_date,omitempty"`
	RebalanceStatus     string     `json:"rebalance_status,omitempty"`
	RebalanceWarning    string     `json:"rebalance_warning,omitempty"`
	RebalanceComputedAt *time.Time `json:"rebalance_computed_at,omitempty"`
	ConstituentCount    int        `json:"constituent_count"`
}

type syncScope struct {
	FactorKey string
	FromDate  string
	ToDate    string
}

func (s *Service) AdminStatus(ctx context.Context, worker WorkerStatus) (*AdminStatusResponse, error) {
	if err := s.EnsureInitialized(ctx); err != nil {
		return nil, err
	}
	definitions, err := s.repo.ListActiveDefinitions(ctx, ExchangeAShare)
	if err != nil {
		return nil, err
	}
	definitionMap := make(map[string]Definition, len(definitions))
	for _, definition := range definitions {
		definitionMap[definition.FactorKey] = definition
	}
	latestSnapshotDate, err := s.repo.LatestSnapshotDate(ctx)
	if err != nil {
		return nil, err
	}
	latestTradeDate, err := s.repo.LatestTradeDate(ctx)
	if err != nil {
		return nil, err
	}
	items := make([]AdminStatusItem, 0, len(defaultDefinitions))
	for _, cfg := range defaultDefinitions {
		definition, ok := definitionMap[cfg.FactorKey]
		if !ok {
			items = append(items, AdminStatusItem{
				IndexID:          cfg.ID,
				FactorKey:        cfg.FactorKey,
				Name:             cfg.Name,
				Status:           StatusPending,
				RebalanceStatus:  StatusPending,
				ConstituentCount: 0,
			})
			continue
		}
		item, buildErr := s.buildAdminStatusItem(ctx, definition)
		if buildErr != nil {
			return nil, buildErr
		}
		items = append(items, item)
	}
	return &AdminStatusResponse{
		LatestSnapshotDate: latestSnapshotDate,
		LatestTradeDate:    latestTradeDate,
		Worker:             worker,
		Items:              items,
	}, nil
}

func (s *Service) buildAdminStatusItem(ctx context.Context, definition Definition) (AdminStatusItem, error) {
	item := AdminStatusItem{
		IndexID:          definition.ID,
		FactorKey:        definition.FactorKey,
		Name:             definition.Name,
		Status:           StatusPending,
		RebalanceStatus:  StatusPending,
		ConstituentCount: 0,
	}
	latestRebalance, err := s.repo.LatestRebalanceBeforeTradeDate(ctx, definition.ID, "9999-12-31")
	if err != nil {
		return AdminStatusItem{}, err
	}
	if latestRebalance != nil {
		item.RebalanceDate = latestRebalance.SignalDate
		item.EffectiveStartDate = latestRebalance.EffectiveStartDate
		item.RebalanceStatus = latestRebalance.Status
		item.RebalanceWarning = latestRebalance.WarningText
		item.ConstituentCount = latestRebalance.ConstituentCount
		computedAt := latestRebalance.ComputedAt
		item.RebalanceComputedAt = &computedAt
		item.Status = latestRebalance.Status
		item.WarningText = latestRebalance.WarningText
	}
	latestDaily, err := s.repo.LatestDaily(ctx, definition.ID)
	if err != nil {
		return AdminStatusItem{}, err
	}
	if latestDaily != nil {
		item.NAV = ptrFloat(latestDaily.NAV)
		item.Status = latestDaily.Status
		item.WarningText = latestDaily.WarningText
		item.LatestTradeDate = latestDaily.TradeDate
		item.SourceTradeDate = latestDaily.SourceTradeDate
		item.ConstituentCount = latestDaily.ConstituentCount
		computedAt := latestDaily.ComputedAt
		item.DailyComputedAt = &computedAt
	}
	return item, nil
}

func (s *Service) RunManualRequest(ctx context.Context, request ManualRunRequest) error {
	normalized, err := normalizeManualRunRequest(request)
	if err != nil {
		return err
	}
	if err := s.EnsureInitialized(ctx); err != nil {
		return err
	}
	scope := syncScope{FactorKey: normalized.FactorKey, FromDate: normalized.FromDate, ToDate: normalized.ToDate}
	indexIDs, err := s.indexIDsForScope(ctx, scope)
	if err != nil {
		return err
	}
	if normalized.Reset {
		switch normalized.Operation {
		case OperationSyncDaily:
			if err := s.repo.DeleteDailyRows(ctx, indexIDs, normalized.FromDate, normalized.ToDate); err != nil {
				return err
			}
		case OperationSyncAll:
			if normalized.FromDate != "" || normalized.ToDate != "" {
				return fmt.Errorf("从头重建暂不支持日期范围，请清空起止日期后重试")
			}
			if err := s.repo.DeleteDailyRows(ctx, indexIDs, "", ""); err != nil {
				return err
			}
			if err := s.repo.DeleteRebalances(ctx, indexIDs, "", ""); err != nil {
				return err
			}
		case OperationSyncRebalances:
			return fmt.Errorf("调仓补算不支持 reset，请使用全链路补算或从头重建")
		default:
			return fmt.Errorf("unsupported factor index operation: %s", normalized.Operation)
		}
	}
	switch normalized.Operation {
	case OperationSyncAll:
		if err := s.syncRebalancesWithScope(ctx, scope); err != nil {
			return err
		}
		return s.syncDailyWithScope(ctx, scope)
	case OperationSyncRebalances:
		return s.syncRebalancesWithScope(ctx, scope)
	case OperationSyncDaily:
		return s.syncDailyWithScope(ctx, scope)
	default:
		return fmt.Errorf("unsupported factor index operation: %s", normalized.Operation)
	}
}

func (s *Service) syncRebalancesWithScope(ctx context.Context, scope syncScope) error {
	definitions, err := s.definitionsForScope(ctx, scope)
	if err != nil {
		return err
	}
	snapshotDates, err := s.repo.ListSnapshotDates(ctx)
	if err != nil {
		return err
	}
	previousDate := ""
	for _, snapshotDate := range snapshotDates {
		if !isFirstTradeDateOfMonth(previousDate, snapshotDate) {
			previousDate = snapshotDate
			continue
		}
		if !dateInRange(snapshotDate, scope.FromDate, scope.ToDate) {
			previousDate = snapshotDate
			continue
		}
		for _, definition := range definitions {
			exists, existsErr := s.repo.RebalanceExists(ctx, definition.ID, snapshotDate)
			if existsErr != nil {
				return existsErr
			}
			if exists {
				continue
			}
			if err := s.createRebalanceForDate(ctx, definition, snapshotDate); err != nil {
				return err
			}
		}
		previousDate = snapshotDate
	}
	return nil
}

func (s *Service) syncDailyWithScope(ctx context.Context, scope syncScope) error {
	definitions, err := s.definitionsForScope(ctx, scope)
	if err != nil {
		return err
	}
	tradeDates, err := s.repo.ListTradeDates(ctx)
	if err != nil {
		return err
	}
	for _, tradeDate := range tradeDates {
		if !dateInRange(tradeDate, scope.FromDate, scope.ToDate) {
			continue
		}
		for _, definition := range definitions {
			if err := s.computeDailyForTradeDate(ctx, definition, tradeDate); err != nil {
				return err
			}
		}
	}
	return nil
}

func (s *Service) definitionsForScope(ctx context.Context, scope syncScope) ([]Definition, error) {
	definitions, err := s.repo.ListActiveDefinitions(ctx, ExchangeAShare)
	if err != nil {
		return nil, err
	}
	factorKey := strings.TrimSpace(scope.FactorKey)
	if factorKey == "" {
		return definitions, nil
	}
	filtered := make([]Definition, 0, 1)
	for _, definition := range definitions {
		if definition.FactorKey == factorKey {
			filtered = append(filtered, definition)
		}
	}
	if len(filtered) == 0 {
		return nil, fmt.Errorf("不支持的因子 key: %s", factorKey)
	}
	return filtered, nil
}

func (s *Service) indexIDsForScope(ctx context.Context, scope syncScope) ([]string, error) {
	definitions, err := s.definitionsForScope(ctx, scope)
	if err != nil {
		return nil, err
	}
	ids := make([]string, 0, len(definitions))
	for _, definition := range definitions {
		ids = append(ids, definition.ID)
	}
	return ids, nil
}

func normalizeManualRunRequest(request ManualRunRequest) (ManualRunRequest, error) {
	request.Operation = strings.TrimSpace(strings.ToLower(request.Operation))
	request.FactorKey = strings.TrimSpace(strings.ToLower(request.FactorKey))
	request.FromDate = strings.TrimSpace(request.FromDate)
	request.ToDate = strings.TrimSpace(request.ToDate)
	if request.Operation == "" {
		request.Operation = OperationSyncAll
	}
	if err := validateManualRunRequest(request); err != nil {
		return ManualRunRequest{}, err
	}
	return request, nil
}

func validateManualRunRequest(request ManualRunRequest) error {
	switch request.Operation {
	case OperationSyncAll, OperationSyncDaily, OperationSyncRebalances:
	default:
		return fmt.Errorf("不支持的补算操作: %s", request.Operation)
	}
	if request.FactorKey != "" && definitionByFactorKey(request.FactorKey) == nil {
		return fmt.Errorf("不支持的因子 key: %s", request.FactorKey)
	}
	if request.FromDate != "" {
		if _, err := time.ParseInLocation("2006-01-02", request.FromDate, time.FixedZone("CST", 8*3600)); err != nil {
			return fmt.Errorf("from_date 必须是 YYYY-MM-DD")
		}
	}
	if request.ToDate != "" {
		if _, err := time.ParseInLocation("2006-01-02", request.ToDate, time.FixedZone("CST", 8*3600)); err != nil {
			return fmt.Errorf("to_date 必须是 YYYY-MM-DD")
		}
	}
	if request.FromDate != "" && request.ToDate != "" && request.FromDate > request.ToDate {
		return fmt.Errorf("from_date 不能晚于 to_date")
	}
	if request.Reset && request.Operation == OperationSyncAll && (request.FromDate != "" || request.ToDate != "") {
		return fmt.Errorf("从头重建暂不支持日期范围，请清空起止日期后重试")
	}
	return nil
}

func dateInRange(value, fromDate, toDate string) bool {
	value = strings.TrimSpace(value)
	fromDate = strings.TrimSpace(fromDate)
	toDate = strings.TrimSpace(toDate)
	if value == "" {
		return false
	}
	if fromDate != "" && value < fromDate {
		return false
	}
	if toDate != "" && value > toDate {
		return false
	}
	return true
}
