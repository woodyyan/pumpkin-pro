package quadrant

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"
)

type SimPortfolioV2Service struct {
	repo                *Repository
	calendar            *SimPortfolioV2CalendarService
	priceResolver       PriceResolver
	priceLookupResolver PriceLookupResolver
	openPriceResolver   OpenPriceResolver
}

func NewSimPortfolioV2Service(repo *Repository) *SimPortfolioV2Service {
	return &SimPortfolioV2Service{repo: repo, calendar: NewSimPortfolioV2CalendarService()}
}

func (s *SimPortfolioV2Service) SetPriceResolver(r PriceResolver) { s.priceResolver = r }
func (s *SimPortfolioV2Service) SetPriceLookupResolver(r PriceLookupResolver) {
	s.priceLookupResolver = r
}
func (s *SimPortfolioV2Service) SetOpenPriceResolver(r OpenPriceResolver) { s.openPriceResolver = r }

type SimPortfolioV2RunRequest struct {
	Market   string   `json:"market"`
	FromDate string   `json:"from_date"`
	ToDate   string   `json:"to_date"`
	Stages   []string `json:"stages"`
	DryRun   bool     `json:"dry_run"`
}

type SimPortfolioV2RunResponse struct {
	OK             bool   `json:"ok"`
	Message        string `json:"message,omitempty"`
	RunID          string `json:"run_id,omitempty"`
	Status         string `json:"status"`
	ProcessedDays  int    `json:"processed_days"`
	BlockedDays    int    `json:"blocked_days"`
	GeneratedFacts int    `json:"generated_facts"`
}

type SimPortfolioV2OverviewResponse struct {
	Items []SimPortfolioV2MarketOverview `json:"items"`
	Runs  []SimPortfolioV2RunItem        `json:"runs,omitempty"`
}

type SimPortfolioV2MarketOverview struct {
	Market                  string `json:"market"`
	Status                  string `json:"status"`
	StatusText              string `json:"status_text"`
	LatestSignalDate        string `json:"latest_signal_date,omitempty"`
	LatestVerifiedTradeDate string `json:"latest_verified_trade_date,omitempty"`
	BlockingStage           string `json:"blocking_stage,omitempty"`
	BlockingMessage         string `json:"blocking_message,omitempty"`
}

type SimPortfolioV2RunItem struct {
	RunID      string `json:"run_id"`
	Market     string `json:"market"`
	Status     string `json:"status"`
	FromDate   string `json:"from_date,omitempty"`
	ToDate     string `json:"to_date,omitempty"`
	StartedAt  string `json:"started_at,omitempty"`
	FinishedAt string `json:"finished_at,omitempty"`
}

type SimPortfolioV2DaysResponse struct {
	Items []SimPortfolioV2DayStatusItem `json:"items"`
}
type SimPortfolioV2DayStatusItem struct {
	Market        string `json:"market"`
	TradeDate     string `json:"trade_date"`
	Stage         string `json:"stage"`
	Status        string `json:"status"`
	ExpectedCount int    `json:"expected_count"`
	ActualCount   int    `json:"actual_count"`
	MissingCount  int    `json:"missing_count"`
	Message       string `json:"message,omitempty"`
	ActionHint    string `json:"action_hint,omitempty"`
}

func (s *SimPortfolioV2Service) Initialize(ctx context.Context, market string) (*SimPortfolioV2RunResponse, error) {
	if err := s.repo.EnsureSimPortfolioV2Definitions(ctx); err != nil {
		return nil, err
	}
	return &SimPortfolioV2RunResponse{OK: true, Status: SimPortfolioV2StatusOK, Message: "Sim Portfolio v2 定义已初始化。"}, nil
}

func (s *SimPortfolioV2Service) Run(ctx context.Context, req SimPortfolioV2RunRequest) (*SimPortfolioV2RunResponse, error) {
	if err := s.repo.EnsureSimPortfolioV2Definitions(ctx); err != nil {
		return nil, err
	}
	market := normalizeSimPortfolioV2Market(req.Market)
	if strings.EqualFold(strings.TrimSpace(req.Market), "ALL") {
		market = "ALL"
	}
	fromDate, toDate := strings.TrimSpace(req.FromDate), strings.TrimSpace(req.ToDate)
	if fromDate == "" {
		fromDate = time.Now().In(beijingLocation()).Format("2006-01-02")
	}
	if toDate == "" {
		toDate = fromDate
	}
	runID := fmt.Sprintf("spv2-%d", time.Now().UnixNano())
	stagesJSON, _ := json.Marshal(req.Stages)
	now := time.Now().UTC()
	run := &SimPortfolioV2PipelineRun{ID: runID, TriggerType: SimPortfolioV2TriggerAdmin, Market: market, FromDate: fromDate, ToDate: toDate, StageScope: string(stagesJSON), Status: SimPortfolioV2StatusRunning, StartedAt: now, CreatedAt: now, UpdatedAt: now}
	if err := s.repo.CreateSimPortfolioV2Run(ctx, run); err != nil {
		return nil, err
	}
	resp := &SimPortfolioV2RunResponse{OK: true, RunID: runID, Status: SimPortfolioV2StatusOK}
	markets := []string{market}
	if market == "ALL" {
		markets = []string{SimPortfolioV2MarketAShare, SimPortfolioV2MarketHKEX}
	}
	for _, m := range markets {
		for _, date := range enumerateDates(fromDate, toDate) {
			resp.ProcessedDays++
			generated, blocked, err := s.runMarketDate(ctx, runID, m, date)
			if err != nil {
				blocked = true
			}
			if generated {
				resp.GeneratedFacts++
			}
			if blocked {
				resp.BlockedDays++
			}
		}
	}
	if resp.BlockedDays > 0 {
		resp.Status = SimPortfolioV2StatusBlocked
		resp.Message = fmt.Sprintf("Pipeline 已运行，%d 天存在阻断。", resp.BlockedDays)
	} else {
		resp.Message = "Pipeline 已运行完成。"
	}
	run.Status = resp.Status
	run.FinishedAt = time.Now().UTC()
	run.UpdatedAt = run.FinishedAt
	summary, _ := json.Marshal(resp)
	run.SummaryJSON = string(summary)
	_ = s.repo.UpdateSimPortfolioV2Run(ctx, run)
	return resp, nil
}

func enumerateDates(fromDate, toDate string) []string {
	from, ok := parseYMD(fromDate)
	if !ok {
		return nil
	}
	to, ok := parseYMD(toDate)
	if !ok || to.Before(from) {
		to = from
	}
	out := []string{}
	for d := from; !d.After(to); d = d.AddDate(0, 0, 1) {
		out = append(out, formatYMD(d))
	}
	return out
}

func (s *SimPortfolioV2Service) writeStatus(ctx context.Context, runID, market, date, stage, status, message, hint string, expected, actual, missing int) {
	now := time.Now().UTC()
	_ = s.repo.UpsertSimPortfolioV2DayStatus(ctx, SimPortfolioV2PipelineDayStatus{Market: market, TradeDate: date, Stage: stage, Status: status, ExpectedCount: expected, ActualCount: actual, MissingCount: missing, Message: message, ActionHint: hint, RunID: runID, CreatedAt: now, UpdatedAt: now})
}

func (s *SimPortfolioV2Service) runMarketDate(ctx context.Context, runID, market, date string) (bool, bool, error) {
	cal := s.calendar.CalendarRow(market, date)
	_ = s.repo.UpsertMarketCalendar(ctx, cal)
	if !cal.IsTradingDay {
		s.writeStatus(ctx, runID, market, date, SimPortfolioV2StageCalendar, SimPortfolioV2StatusSkipped, "休市日，跳过模拟组合 pipeline。", "无需操作。", 0, 0, 0)
		return false, false, nil
	}
	s.writeStatus(ctx, runID, market, date, SimPortfolioV2StageCalendar, SimPortfolioV2StatusOK, "交易日。", "", 1, 1, 0)
	batch, err := s.buildSignalBatch(ctx, runID, market, date)
	if err != nil || batch == nil || batch.Status != SimPortfolioV2StatusOK {
		return false, true, err
	}
	defs, err := s.repo.ListActiveSimPortfolioV2Definitions(ctx)
	if err != nil {
		return false, true, err
	}
	generated := false
	for _, def := range defs {
		if def.Market != market {
			continue
		}
		ok, err := s.runDefinitionDate(ctx, runID, def, date)
		if err != nil {
			return generated, true, err
		}
		generated = generated || ok
	}
	return generated, false, nil
}

func (s *SimPortfolioV2Service) buildSignalBatch(ctx context.Context, runID, market, date string) (*SimPortfolioV2SignalBatch, error) {
	rows, err := s.repo.ListRankingSnapshotsByDate(ctx, market, date, 50)
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	batchID := fmt.Sprintf("sig-%s-%s", strings.ToLower(market), date)
	items := []SimPortfolioV2SignalItem{}
	missingPrice := 0
	for _, row := range rows {
		if row.ClosePrice <= 0 || strings.TrimSpace(row.PriceTradeDate) != date {
			missingPrice++
		}
		items = append(items, SimPortfolioV2SignalItem{BatchID: batchID, Market: market, SourceTradeDate: date, Code: row.Code, Exchange: row.Exchange, Name: row.Name, Rank: row.Rank, Opportunity: row.Opportunity, Risk: row.Risk, ClosePrice: row.ClosePrice, PriceTradeDate: row.PriceTradeDate, Board: inferSimPortfolioBoard(row.Code, row.Exchange), CreatedAt: now})
	}
	status := SimPortfolioV2StatusOK
	message := "信号快照已就绪。"
	if len(items) == 0 {
		status = SimPortfolioV2StatusBlocked
		message = "缺少模拟组合信号快照。"
	}
	if missingPrice > 0 {
		status = SimPortfolioV2StatusBlocked
		message = "信号快照存在缺失或非精确收盘价。"
	}
	batch := SimPortfolioV2SignalBatch{ID: batchID, Market: market, SourceTradeDate: date, ComputedAt: now, Status: status, CandidateCount: len(rows), SignalCount: len(items), MissingPriceCount: missingPrice, Message: message, CreatedAt: now, UpdatedAt: now}
	if err := s.repo.ReplaceSimPortfolioV2SignalBatch(ctx, batch, items); err != nil {
		return nil, err
	}
	if status != SimPortfolioV2StatusOK {
		s.writeStatus(ctx, runID, market, date, SimPortfolioV2StageSignal, status, message, "请先重跑该市场该交易日四象限/排行榜快照。", 1, 0, 1)
	} else {
		s.writeStatus(ctx, runID, market, date, SimPortfolioV2StageSignal, status, message, "", 1, 1, 0)
	}
	return &batch, nil
}

func (s *SimPortfolioV2Service) runDefinitionDate(ctx context.Context, runID string, def SimPortfolioV2Definition, signalDate string) (bool, error) {
	items, err := s.selectV2Items(ctx, def, signalDate)
	if err != nil {
		return false, err
	}
	entryDate := s.calendar.NextTradingDay(def.Market, signalDate)
	now := time.Now().UTC()
	batchID := fmt.Sprintf("sel-%s-%s", def.ID, signalDate)
	status := SimPortfolioV2StatusOK
	warning := ""
	if len(items) < def.MaxHoldings {
		status = SimPortfolioV2StatusBlocked
		warning = defaultRankingPortfolioWarningText
	}
	selItems := []SimPortfolioV2SelectionItem{}
	for i, item := range items {
		selItems = append(selItems, SimPortfolioV2SelectionItem{SelectionBatchID: batchID, PortfolioID: def.ID, SignalDate: signalDate, EntryTradeDate: entryDate, Code: item.Code, Exchange: item.Exchange, Name: item.Name, Rank: i + 1, SourceRank: item.Rank, Weight: 1 / float64(def.MaxHoldings), Board: item.Board, CreatedAt: now})
	}
	batch := SimPortfolioV2SelectionBatch{ID: batchID, PortfolioID: def.ID, Market: def.Market, SignalDate: signalDate, EntryTradeDate: entryDate, Status: status, SelectedCount: len(selItems), WarningText: warning, CreatedAt: now, UpdatedAt: now}
	if err := s.repo.ReplaceSimPortfolioV2Selection(ctx, batch, selItems); err != nil {
		return false, err
	}
	if status != SimPortfolioV2StatusOK {
		s.writeStatus(ctx, runID, def.Market, signalDate, SimPortfolioV2StageSelection, status, warning, "检查选股规则或补齐信号快照。", def.MaxHoldings, len(selItems), def.MaxHoldings-len(selItems))
		return false, nil
	}
	s.writeStatus(ctx, runID, def.Market, signalDate, SimPortfolioV2StageSelection, SimPortfolioV2StatusOK, "选股完成。", "", def.MaxHoldings, len(selItems), 0)
	if entryDate == "" {
		s.writeStatus(ctx, runID, def.Market, signalDate, SimPortfolioV2StagePriceRequirements, SimPortfolioV2StatusBlocked, "缺少下一交易日。", "检查市场交易日历。", 1, 0, 1)
		return false, nil
	}
	reqs := s.buildPriceRequirements(def, signalDate, entryDate, selItems)
	if err := s.repo.ReplaceSimPortfolioV2PriceRequirements(ctx, def.ID, signalDate, reqs); err != nil {
		return false, err
	}
	s.writeStatus(ctx, runID, def.Market, signalDate, SimPortfolioV2StagePriceRequirements, SimPortfolioV2StatusOK, "价格需求已生成。", "", len(reqs), len(reqs), 0)
	missingOpen, missingClose, err := s.resolvePriceRequirements(ctx, def.ID, signalDate)
	if err != nil {
		return false, err
	}
	if missingOpen > 0 {
		s.writeStatus(ctx, runID, def.Market, signalDate, SimPortfolioV2StageEntryOpen, SimPortfolioV2StatusBlocked, "建仓开盘价未就绪。", "等待行情源更新后重新运行 pipeline。", len(selItems), len(selItems)-missingOpen, missingOpen)
		return false, nil
	}
	s.writeStatus(ctx, runID, def.Market, signalDate, SimPortfolioV2StageEntryOpen, SimPortfolioV2StatusOK, "建仓开盘价已就绪。", "", len(selItems), len(selItems), 0)
	if missingClose > 0 {
		s.writeStatus(ctx, runID, def.Market, signalDate, SimPortfolioV2StageValuationClose, SimPortfolioV2StatusBlocked, "估值收盘价未就绪。", "等待收盘行情源更新后重新运行 pipeline。", len(selItems), len(selItems)-missingClose, missingClose)
		return false, nil
	}
	s.writeStatus(ctx, runID, def.Market, signalDate, SimPortfolioV2StageValuationClose, SimPortfolioV2StatusOK, "估值收盘价已就绪。", "", len(selItems), len(selItems), 0)
	if err := s.generateFacts(ctx, def, signalDate, entryDate); err != nil {
		s.writeStatus(ctx, runID, def.Market, signalDate, SimPortfolioV2StageFacts, SimPortfolioV2StatusFailed, err.Error(), "检查价格需求和事实表一致性。", 1, 0, 1)
		return false, err
	}
	s.writeStatus(ctx, runID, def.Market, signalDate, SimPortfolioV2StageFacts, SimPortfolioV2StatusOK, "事实表已生成。", "", 1, 1, 0)
	s.writeStatus(ctx, runID, def.Market, signalDate, SimPortfolioV2StageVerify, SimPortfolioV2StatusOK, "验证通过。", "", 1, 1, 0)
	return true, nil
}

type v2SignalSelectionItem struct {
	Rank                        int
	Code, Name, Exchange, Board string
	ConsecutiveDays             int
}

func (s *SimPortfolioV2Service) selectV2Items(ctx context.Context, def SimPortfolioV2Definition, signalDate string) ([]v2SignalSelectionItem, error) {
	batch, err := s.repo.GetSimPortfolioV2SignalBatch(ctx, def.Market, signalDate)
	if err != nil || batch == nil {
		return nil, err
	}
	rows, err := s.repo.ListSimPortfolioV2SignalItems(ctx, batch.ID)
	if err != nil {
		return nil, err
	}
	excluded := decodeSimPortfolioExcludedBoards(def.ExcludedBoards)
	items := []v2SignalSelectionItem{}
	for _, row := range rows {
		board := inferSimPortfolioBoard(row.Code, row.Exchange)
		if _, ok := excluded[strings.ToUpper(board)]; ok {
			continue
		}
		item := v2SignalSelectionItem{Rank: row.Rank, Code: row.Code, Name: row.Name, Exchange: row.Exchange, Board: board}
		if def.SelectionRule == rankingPortfolioSelectionRuleTop10ByStreak {
			days, err := s.repo.GetConsecutiveDaysAsOf(ctx, item.Code, resolveSimPortfolioExchangeCodes(def.Market), signalDate)
			if err != nil {
				return nil, err
			}
			item.ConsecutiveDays = days
		}
		items = append(items, item)
	}
	if def.SelectionWindow > 0 && len(items) > def.SelectionWindow {
		items = items[:def.SelectionWindow]
	}
	if def.SelectionRule == rankingPortfolioSelectionRuleTop10ByStreak {
		sort.SliceStable(items, func(i, j int) bool {
			if items[i].ConsecutiveDays == items[j].ConsecutiveDays {
				return items[i].Rank < items[j].Rank
			}
			return items[i].ConsecutiveDays > items[j].ConsecutiveDays
		})
	}
	if len(items) > def.MaxHoldings {
		items = items[:def.MaxHoldings]
	}
	return items, nil
}

func (s *SimPortfolioV2Service) buildPriceRequirements(def SimPortfolioV2Definition, signalDate, entryDate string, items []SimPortfolioV2SelectionItem) []SimPortfolioV2PriceRequirement {
	now := time.Now().UTC()
	out := []SimPortfolioV2PriceRequirement{}
	for _, item := range items {
		for _, priceType := range []string{SimPortfolioV2PriceTypeEntryOpen, SimPortfolioV2PriceTypeValuationClose} {
			out = append(out, SimPortfolioV2PriceRequirement{PortfolioID: def.ID, Market: def.Market, SignalDate: signalDate, TradeDate: entryDate, Code: item.Code, Exchange: item.Exchange, PriceType: priceType, Required: true, Status: SimPortfolioV2PriceStatusPending, CreatedAt: now, UpdatedAt: now})
		}
	}
	return out
}

func (s *SimPortfolioV2Service) resolvePriceRequirements(ctx context.Context, portfolioID, signalDate string) (int, int, error) {
	reqs, err := s.repo.ListSimPortfolioV2PriceRequirements(ctx, portfolioID, signalDate)
	if err != nil {
		return 0, 0, err
	}
	missingOpen, missingClose := 0, 0
	for _, req := range reqs {
		price := 0.0
		source := ""
		priceDate := req.TradeDate
		if req.PriceType == SimPortfolioV2PriceTypeEntryOpen {
			if s.openPriceResolver != nil {
				price = s.openPriceResolver(ctx, req.Code, req.Exchange, req.TradeDate)
				source = "open_price_resolver"
			}
		} else {
			if s.priceLookupResolver != nil {
				lookup := s.priceLookupResolver(ctx, req.Code, req.Exchange, req.TradeDate)
				if lookup.ClosePrice > 0 && strings.TrimSpace(lookup.TradeDate) == req.TradeDate {
					price = lookup.ClosePrice
					priceDate = lookup.TradeDate
					source = "price_lookup_resolver"
				}
			}
			if price <= 0 && s.priceResolver != nil {
				price = s.priceResolver(ctx, req.Code, req.Exchange, req.TradeDate)
				source = "price_resolver"
			}
		}
		req.UpdatedAt = time.Now().UTC()
		if price > 0 {
			req.Price = price
			req.PriceTradeDate = priceDate
			req.Source = source
			req.Status = SimPortfolioV2PriceStatusSatisfied
			req.ResolvedAt = req.UpdatedAt
			req.MissingReason = ""
		} else {
			req.Status = SimPortfolioV2PriceStatusMissing
			req.MissingReason = fmt.Sprintf("%s 在 %s 缺少 %s", req.Code, req.TradeDate, req.PriceType)
		}
		if err := s.repo.UpdateSimPortfolioV2PriceRequirement(ctx, req); err != nil {
			return 0, 0, err
		}
		if req.Status != SimPortfolioV2PriceStatusSatisfied {
			if req.PriceType == SimPortfolioV2PriceTypeEntryOpen {
				missingOpen++
			} else {
				missingClose++
			}
		}
	}
	return missingOpen, missingClose, nil
}

func (s *SimPortfolioV2Service) generateFacts(ctx context.Context, def SimPortfolioV2Definition, signalDate, tradeDate string) error {
	items, err := s.repo.ListSimPortfolioV2SelectionItems(ctx, def.ID, signalDate)
	if err != nil {
		return err
	}
	reqs, err := s.repo.ListSimPortfolioV2PriceRequirements(ctx, def.ID, signalDate)
	if err != nil {
		return err
	}
	priceByKey := map[string]float64{}
	for _, req := range reqs {
		if req.Status != SimPortfolioV2PriceStatusSatisfied || req.Price <= 0 {
			return fmt.Errorf("price requirement not satisfied: %s %s %s", req.Code, req.TradeDate, req.PriceType)
		}
		priceByKey[req.Code+"\x00"+req.Exchange+"\x00"+req.PriceType] = req.Price
	}
	previous, err := s.repo.GetLatestSimPortfolioV2Daily(ctx, def.ID)
	if err != nil {
		return err
	}
	previousAssets := def.InitialAssets
	if previous != nil && previous.TotalAssets > 0 {
		previousAssets = previous.TotalAssets
	}
	targetValue := previousAssets / float64(def.MaxHoldings)
	now := time.Now().UTC()
	positions := []SimPortfolioV2Position{}
	trades := []SimPortfolioV2Trade{}
	total := 0.0
	for _, item := range items {
		open := priceByKey[item.Code+"\x00"+item.Exchange+"\x00"+SimPortfolioV2PriceTypeEntryOpen]
		closep := priceByKey[item.Code+"\x00"+item.Exchange+"\x00"+SimPortfolioV2PriceTypeValuationClose]
		shares := targetValue / open
		marketValue := shares * closep
		profit := marketValue - targetValue
		total += marketValue
		positions = append(positions, SimPortfolioV2Position{PortfolioID: def.ID, Market: def.Market, TradeDate: tradeDate, SignalDate: signalDate, Code: item.Code, Exchange: item.Exchange, Name: item.Name, Rank: item.Rank, Weight: item.Weight, TargetValue: targetValue, Shares: shares, BuyPrice: open, ClosePrice: closep, MarketValue: marketValue, Profit: profit, ProfitRate: profit / targetValue, CreatedAt: now, UpdatedAt: now})
		trades = append(trades, SimPortfolioV2Trade{PortfolioID: def.ID, Market: def.Market, TradeDate: tradeDate, SignalDate: signalDate, Code: item.Code, Exchange: item.Exchange, Name: item.Name, Action: simPortfolioActionBuy, NewWeight: item.Weight, TradePrice: open, TargetValue: targetValue, NewShares: shares, ShareDelta: shares, Reason: simPortfolioReasonEnterTop4, CreatedAt: now, UpdatedAt: now})
	}
	dailyReturn := 0.0
	if previousAssets > 0 {
		dailyReturn = total/previousAssets - 1
	}
	daily := SimPortfolioV2Daily{PortfolioID: def.ID, Market: def.Market, TradeDate: tradeDate, SignalDate: signalDate, SourceTradeDate: tradeDate, NAV: roundTo(total/def.InitialAssets, simPortfolioNavPrecision), TotalAssets: total, PreviousAssets: previousAssets, DailyReturn: dailyReturn, TotalReturn: total/def.InitialAssets - 1, PositionCount: len(positions), Rebalance: true, Status: "verified", ComputedAt: now, CreatedAt: now, UpdatedAt: now}
	metrics := SimPortfolioV2Metrics{PortfolioID: def.ID, Market: def.Market, TradeDate: tradeDate, NAV: daily.NAV, CreatedAt: now, UpdatedAt: now}
	return s.repo.ReplaceSimPortfolioV2FactDate(ctx, daily, positions, trades, metrics)
}

func (s *SimPortfolioV2Service) GetAdminOverview(ctx context.Context) (*SimPortfolioV2OverviewResponse, error) {
	defs, err := s.repo.ListActiveSimPortfolioV2Definitions(ctx)
	if err != nil {
		return nil, err
	}
	statuses, _ := s.repo.ListSimPortfolioV2DayStatuses(ctx, "", "", "")
	latestBlock := map[string]SimPortfolioV2PipelineDayStatus{}
	for _, st := range statuses {
		if st.Status == SimPortfolioV2StatusBlocked || st.Status == SimPortfolioV2StatusFailed {
			if old, ok := latestBlock[st.Market]; !ok || st.TradeDate > old.TradeDate {
				latestBlock[st.Market] = st
			}
		}
	}
	_ = latestBlock
	resp := &SimPortfolioV2OverviewResponse{Items: []SimPortfolioV2MarketOverview{}}
	seen := map[string]bool{}
	for _, def := range defs {
		if seen[def.Market] {
			continue
		}
		seen[def.Market] = true
		item := SimPortfolioV2MarketOverview{Market: def.Market, Status: SimPortfolioV2StatusPending, StatusText: "等待初始化"}
		if latest, _ := s.repo.GetLatestSimPortfolioV2Daily(ctx, def.ID); latest != nil {
			item.Status = SimPortfolioV2StatusOK
			item.StatusText = "已有 verified 事实表"
			item.LatestVerifiedTradeDate = latest.TradeDate
			item.LatestSignalDate = latest.SignalDate
		}
		if block, ok := latestBlock[def.Market]; ok {
			item.Status = block.Status
			item.BlockingStage = block.Stage
			item.BlockingMessage = block.Message
			item.StatusText = block.Message
		}
		resp.Items = append(resp.Items, item)
	}
	runs, _ := s.repo.ListSimPortfolioV2Runs(ctx, 10)
	for _, run := range runs {
		resp.Runs = append(resp.Runs, SimPortfolioV2RunItem{RunID: run.ID, Market: run.Market, Status: run.Status, FromDate: run.FromDate, ToDate: run.ToDate, StartedAt: run.StartedAt.Format(time.RFC3339), FinishedAt: run.FinishedAt.Format(time.RFC3339)})
	}
	return resp, nil
}

func (s *SimPortfolioV2Service) GetAdminDays(ctx context.Context, market, fromDate, toDate string) (*SimPortfolioV2DaysResponse, error) {
	rows, err := s.repo.ListSimPortfolioV2DayStatuses(ctx, market, fromDate, toDate)
	if err != nil {
		return nil, err
	}
	resp := &SimPortfolioV2DaysResponse{}
	for _, row := range rows {
		resp.Items = append(resp.Items, SimPortfolioV2DayStatusItem{Market: row.Market, TradeDate: row.TradeDate, Stage: row.Stage, Status: row.Status, ExpectedCount: row.ExpectedCount, ActualCount: row.ActualCount, MissingCount: row.MissingCount, Message: row.Message, ActionHint: row.ActionHint})
	}
	return resp, nil
}

func (s *SimPortfolioV2Service) GetPortfolioOverview(ctx context.Context) (*SimPortfolioOverviewResponse, error) {
	if err := s.repo.EnsureSimPortfolioV2Definitions(ctx); err != nil {
		return nil, err
	}
	defs, err := s.repo.ListActiveSimPortfolioV2Definitions(ctx)
	if err != nil {
		return nil, err
	}
	resp := &SimPortfolioOverviewResponse{Items: []SimPortfolioOverviewItem{}}
	for _, def := range defs {
		latest, err := s.repo.GetLatestSimPortfolioV2Daily(ctx, def.ID)
		if err != nil {
			return nil, err
		}
		metrics, err := s.repo.GetLatestSimPortfolioV2Metrics(ctx, def.ID)
		if err != nil {
			return nil, err
		}
		positions := []SimPortfolioV2Position{}
		trades := []SimPortfolioV2Trade{}
		status, statusText := SimPortfolioV2StatusPending, "等待 Sim Portfolio v2 pipeline 生成 verified 数据"
		if latest != nil {
			status = simPortfolioStatusComplete
			statusText = "已完成最新 verified 收盘估值"
			positions, _ = s.repo.ListSimPortfolioV2PositionsByTradeDate(ctx, def.ID, latest.TradeDate)
			trades, _ = s.repo.ListSimPortfolioV2Trades(ctx, def.ID, "", "", "")
		}
		item := SimPortfolioOverviewItem{PortfolioID: def.ID, Code: def.Code, Name: def.Name, Exchange: def.Market, PortfolioVariant: def.PortfolioVariant, SelectionRule: def.SelectionRule, SelectionWindow: def.SelectionWindow, InitialAssets: def.InitialAssets, PositionCount: def.MaxHoldings, Status: status, StatusText: statusText, CurrentPositions: toV2PositionItems(positions), LatestTrades: toV2TradeItems(limitV2Trades(trades, 6))}
		if latest != nil {
			item.LatestTradeDate = latest.TradeDate
			item.LatestSignalDate = latest.SignalDate
			item.NAV = latest.NAV
			item.TotalAssets = latest.TotalAssets
			item.DailyReturn = latest.DailyReturn
			item.TotalReturn = latest.TotalReturn
			if latest.TradeDate > resp.AsOfTradeDate {
				resp.AsOfTradeDate = latest.TradeDate
			}
		}
		if metrics != nil {
			item.MaxDrawdown = metrics.MaxDrawdown
			item.Volatility = metrics.Volatility
			item.WinRate = metrics.WinRate
			item.TurnoverRate = metrics.TurnoverRate
		}
		resp.Items = append(resp.Items, item)
	}
	return resp, nil
}

func limitV2Trades(rows []SimPortfolioV2Trade, limit int) []SimPortfolioV2Trade {
	if len(rows) > limit {
		return rows[:limit]
	}
	return rows
}

func toV2PositionItems(rows []SimPortfolioV2Position) []SimPortfolioPositionItem {
	out := []SimPortfolioPositionItem{}
	for _, row := range rows {
		out = append(out, SimPortfolioPositionItem{Rank: row.Rank, StockCode: row.Code, StockName: row.Name, Exchange: row.Exchange, Weight: row.Weight, TargetValue: row.TargetValue, Shares: row.Shares, BuyPrice: row.BuyPrice, ClosePrice: row.ClosePrice, MarketValue: row.MarketValue, Profit: row.Profit, ProfitRate: row.ProfitRate})
	}
	return out
}
func toV2TradeItems(rows []SimPortfolioV2Trade) []SimPortfolioTradeItem {
	out := []SimPortfolioTradeItem{}
	for _, row := range rows {
		out = append(out, SimPortfolioTradeItem{TradeDate: row.TradeDate, SignalDate: row.SignalDate, StockCode: row.Code, StockName: row.Name, Exchange: row.Exchange, Action: row.Action, OldWeight: row.OldWeight, NewWeight: row.NewWeight, TradePrice: row.TradePrice, TargetValue: row.TargetValue, OldShares: row.OldShares, NewShares: row.NewShares, ShareDelta: row.ShareDelta, Reason: row.Reason})
	}
	return out
}

func (s *SimPortfolioV2Service) GetPortfolioDaily(ctx context.Context, portfolioID, fromDate, toDate string) (*SimPortfolioDailyResponse, error) {
	rows, err := s.repo.ListSimPortfolioV2DailyRange(ctx, portfolioID, fromDate, toDate)
	if err != nil {
		return nil, err
	}
	resp := &SimPortfolioDailyResponse{PortfolioID: portfolioID}
	for _, row := range rows {
		resp.Items = append(resp.Items, SimPortfolioDailyItem{TradeDate: row.TradeDate, SignalDate: row.SignalDate, SourceTradeDate: row.SourceTradeDate, NAV: row.NAV, TotalAssets: row.TotalAssets, DailyReturn: row.DailyReturn, TotalReturn: row.TotalReturn, PositionCount: row.PositionCount, Rebalance: row.Rebalance, Status: row.Status})
	}
	return resp, nil
}
func (s *SimPortfolioV2Service) GetPortfolioPositions(ctx context.Context, portfolioID, tradeDate string) (*SimPortfolioPositionsResponse, error) {
	if strings.TrimSpace(tradeDate) == "" {
		latest, _ := s.repo.GetLatestSimPortfolioV2Daily(ctx, portfolioID)
		if latest != nil {
			tradeDate = latest.TradeDate
		}
	}
	rows, err := s.repo.ListSimPortfolioV2PositionsByTradeDate(ctx, portfolioID, tradeDate)
	if err != nil {
		return nil, err
	}
	resp := &SimPortfolioPositionsResponse{PortfolioID: portfolioID, TradeDate: tradeDate, Items: toV2PositionItems(rows)}
	if latest, _ := s.repo.GetLatestSimPortfolioV2Daily(ctx, portfolioID); latest != nil && latest.TradeDate == tradeDate {
		resp.TotalAssets = latest.TotalAssets
	}
	return resp, nil
}
func (s *SimPortfolioV2Service) GetPortfolioTrades(ctx context.Context, portfolioID, fromDate, toDate, action string) (*SimPortfolioTradesResponse, error) {
	rows, err := s.repo.ListSimPortfolioV2Trades(ctx, portfolioID, fromDate, toDate, action)
	if err != nil {
		return nil, err
	}
	return &SimPortfolioTradesResponse{PortfolioID: portfolioID, Items: toV2TradeItems(rows)}, nil
}
func (s *SimPortfolioV2Service) GetPortfolioMetrics(ctx context.Context, portfolioID string) (*SimPortfolioMetricsResponse, error) {
	row, err := s.repo.GetLatestSimPortfolioV2Metrics(ctx, portfolioID)
	if err != nil {
		return nil, err
	}
	if row == nil {
		return &SimPortfolioMetricsResponse{PortfolioID: portfolioID}, nil
	}
	return &SimPortfolioMetricsResponse{PortfolioID: portfolioID, TradeDate: row.TradeDate, NAV: row.NAV, AnnualReturn: row.AnnualReturn, MaxDrawdown: row.MaxDrawdown, SharpeRatio: row.SharpeRatio, Volatility: row.Volatility, WinRate: row.WinRate, TurnoverRate: row.TurnoverRate}, nil
}
