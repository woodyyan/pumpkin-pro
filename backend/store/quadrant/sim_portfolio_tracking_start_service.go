package quadrant

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"time"

	"gorm.io/gorm"
)

var simPortfolioTrackingStartDateRE = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}$`)

type SimPortfolioTrackingStartService struct {
	service *Service
}

func NewSimPortfolioTrackingStartService(service *Service) *SimPortfolioTrackingStartService {
	return &SimPortfolioTrackingStartService{service: service}
}

func (svc *SimPortfolioTrackingStartService) GetStatus(ctx context.Context) (*SimPortfolioTrackingStartStatusResponse, error) {
	if svc == nil || svc.service == nil || svc.service.repo == nil {
		return nil, fmt.Errorf("sim portfolio tracking start service not configured")
	}
	config, err := svc.service.repo.GetSimPortfolioTrackingConfig(ctx)
	if err != nil {
		return nil, err
	}
	latestJob, err := svc.service.repo.GetLatestSimPortfolioTrackingJob(ctx)
	if err != nil {
		return nil, err
	}
	resp := &SimPortfolioTrackingStartStatusResponse{OK: true}
	if config != nil {
		resp.CurrentStartSignalDate = strings.TrimSpace(config.StartSignalDate)
		resp.AppliedBy = strings.TrimSpace(config.AppliedBy)
		resp.Status = strings.TrimSpace(config.Status)
		resp.Note = strings.TrimSpace(config.Note)
		if !config.AppliedAt.IsZero() {
			resp.AppliedAt = config.AppliedAt.Format(time.RFC3339)
		}
	}
	if latestJob != nil {
		resp.LatestJob = toSimPortfolioTrackingJobSummary(*latestJob)
	}
	return resp, nil
}

func (svc *SimPortfolioTrackingStartService) Preview(ctx context.Context, startSignalDate string) (*SimPortfolioTrackingStartPreviewResponse, error) {
	if svc == nil || svc.service == nil || svc.service.repo == nil {
		return nil, fmt.Errorf("sim portfolio tracking start service not configured")
	}
	return svc.preview(ctx, startSignalDate)
}

func (svc *SimPortfolioTrackingStartService) Apply(ctx context.Context, startSignalDate string, requestedBy string, note string) (*SimPortfolioTrackingStartApplyResponse, error) {
	if svc == nil || svc.service == nil || svc.service.repo == nil {
		return nil, fmt.Errorf("sim portfolio tracking start service not configured")
	}
	s := svc.service
	s.simPortfolioMu.Lock()
	defer s.simPortfolioMu.Unlock()

	preview, err := svc.preview(ctx, startSignalDate)
	if err != nil {
		return nil, err
	}
	job := newSimPortfolioTrackingApplyJob(startSignalDate, requestedBy)
	job.PreviewJSON = mustJSON(preview)
	if !preview.CanApply {
		job.Status = simPortfolioTrackingJobStatusReject
		job.Message = preview.Message
		job.ErrorText = preview.Message
		job.FinishedAt = time.Now().UTC()
		job.ResultJSON = mustJSON(preview.BlockingReasons)
		_ = s.repo.db.WithContext(ctx).Create(job).Error
		return nil, fmt.Errorf(preview.Message)
	}

	resp := &SimPortfolioTrackingStartApplyResponse{
		OK:              true,
		JobID:           job.ID,
		StartSignalDate: preview.StartSignalDate,
		Portfolios:      []SimPortfolioTrackingStartApplyPortfolio{},
		Verify:          SimPortfolioTrackingStartVerifySummary{Status: "ok"},
	}
	definitions, err := s.repo.ListActiveRankingPortfolioDefinitions(ctx)
	if err != nil {
		return nil, err
	}
	if len(definitions) == 0 {
		return nil, fmt.Errorf("没有可用的模拟组合定义")
	}
	if err := s.repo.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("1 = 1").Delete(&SimPortfolioPosition{}).Error; err != nil {
			return err
		}
		if err := tx.Where("1 = 1").Delete(&SimPortfolioTrade{}).Error; err != nil {
			return err
		}
		if err := tx.Where("1 = 1").Delete(&SimPortfolioMetrics{}).Error; err != nil {
			return err
		}
		if err := tx.Where("1 = 1").Delete(&SimPortfolioDaily{}).Error; err != nil {
			return err
		}
		return nil
	}); err != nil {
		return nil, err
	}

	for _, definition := range definitions {
		if err := s.recomputeSimPortfolioDefinition(ctx, definition, preview.StartSignalDate, "", false); err != nil {
			job.Status = simPortfolioTrackingJobStatusFailed
			job.Message = fmt.Sprintf("%s 重算失败：%s", definition.Name, err.Error())
			job.ErrorText = err.Error()
			job.FinishedAt = time.Now().UTC()
			job.ResultJSON = mustJSON(resp)
			_ = s.repo.db.WithContext(ctx).Create(job).Error
			return nil, err
		}
		dailyRows, err := s.repo.ListAllSimPortfolioDaily(ctx, definition.ID)
		if err != nil {
			return nil, err
		}
		portfolioSummary := SimPortfolioTrackingStartApplyPortfolio{
			PortfolioID:     definition.ID,
			Name:            definition.Name,
			Exchange:        definition.Exchange,
			Status:          simPortfolioTrackingJobStatusOK,
			StartSignalDate: preview.StartSignalDate,
			Message:         "重算完成。",
		}
		for _, row := range dailyRows {
			if row.Status == simPortfolioStatusComplete && row.PositionCount > 0 {
				portfolioSummary.GeneratedDailyCount++
				if portfolioSummary.FirstTradeDate == "" {
					portfolioSummary.FirstTradeDate = row.TradeDate
				}
				portfolioSummary.LatestTradeDate = row.TradeDate
			}
		}
		resp.Portfolios = append(resp.Portfolios, portfolioSummary)
	}

	for _, definition := range definitions {
		dailyCount, positionCount, tradeCount, metricsCount, err := svc.countFactRows(ctx, definition.ID)
		if err != nil {
			return nil, err
		}
		resp.Summary.PortfolioCount++
		resp.Summary.DailyRowCount += dailyCount
		resp.Summary.PositionRowCount += positionCount
		resp.Summary.TradeRowCount += tradeCount
		resp.Summary.MetricsRowCount += metricsCount
	}
	verify, err := s.VerifySimPortfolios(ctx, "")
	if err != nil {
		return nil, err
	}
	for _, item := range verify.Items {
		if item.Status != "ok" {
			resp.Verify.IssueCount++
		}
	}
	if resp.Verify.IssueCount > 0 {
		resp.Verify.Status = "warning"
	}
	resp.Message = fmt.Sprintf("已从 %s 开始重置并重算 %d 个模拟组合。", preview.StartSignalDate, resp.Summary.PortfolioCount)

	now := time.Now().UTC()
	if err := s.repo.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		config := SimPortfolioTrackingConfig{
			ConfigKey:        simPortfolioTrackingStartConfigKey,
			StartSignalDate:  preview.StartSignalDate,
			LatestApplyJobID: job.ID,
			Status:           "active",
			AppliedBy:        strings.TrimSpace(requestedBy),
			AppliedAt:        now,
			Note:             strings.TrimSpace(note),
			CreatedAt:        now,
			UpdatedAt:        now,
		}
		var existing SimPortfolioTrackingConfig
		if err := tx.Where("config_key = ?", simPortfolioTrackingStartConfigKey).First(&existing).Error; err != nil {
			if err == gorm.ErrRecordNotFound {
				if err := tx.Create(&config).Error; err != nil {
					return err
				}
			} else {
				return err
			}
		} else {
			if err := tx.Model(&existing).Updates(map[string]any{
				"start_signal_date":   config.StartSignalDate,
				"latest_apply_job_id": config.LatestApplyJobID,
				"status":              config.Status,
				"applied_by":          config.AppliedBy,
				"applied_at":          config.AppliedAt,
				"note":                config.Note,
				"updated_at":          config.UpdatedAt,
			}).Error; err != nil {
				return err
			}
		}
		job.Status = simPortfolioTrackingJobStatusOK
		job.Message = resp.Message
		job.EffectiveStartSignalDate = preview.StartSignalDate
		job.FinishedAt = now
		job.ResultJSON = mustJSON(resp)
		return tx.Create(job).Error
	}); err != nil {
		return nil, err
	}
	return resp, nil
}

func (svc *SimPortfolioTrackingStartService) preview(ctx context.Context, startSignalDate string) (*SimPortfolioTrackingStartPreviewResponse, error) {
	startSignalDate = strings.TrimSpace(startSignalDate)
	resp := &SimPortfolioTrackingStartPreviewResponse{
		OK:              true,
		StartSignalDate: startSignalDate,
		CanApply:        true,
		Severity:        "ok",
		Markets:         []SimPortfolioTrackingStartMarketPreview{},
		Portfolios:      []SimPortfolioTrackingStartPortfolioPreview{},
		BlockingReasons: []SimPortfolioTrackingStartReason{},
		Warnings:        []SimPortfolioTrackingStartReason{},
	}
	if !simPortfolioTrackingStartDateRE.MatchString(startSignalDate) {
		resp.CanApply = false
		resp.Severity = "blocked"
		resp.Message = "开始信号日格式错误，请使用 YYYY-MM-DD。"
		resp.BlockingReasons = append(resp.BlockingReasons, SimPortfolioTrackingStartReason{Scope: "input", Code: "invalid_date", Message: resp.Message})
		return resp, nil
	}
	if _, err := time.ParseInLocation("2006-01-02", startSignalDate, rankingSnapshotLocation); err != nil {
		resp.CanApply = false
		resp.Severity = "blocked"
		resp.Message = "开始信号日不是有效日期。"
		resp.BlockingReasons = append(resp.BlockingReasons, SimPortfolioTrackingStartReason{Scope: "input", Code: "invalid_date", Message: resp.Message})
		return resp, nil
	}
	if startSignalDate > time.Now().In(rankingSnapshotLocation).Format("2006-01-02") {
		resp.CanApply = false
		resp.Severity = "blocked"
		resp.Message = "未来日期尚无排行榜快照。"
		resp.BlockingReasons = append(resp.BlockingReasons, SimPortfolioTrackingStartReason{Scope: "input", Code: "future_date", Message: resp.Message})
		return resp, nil
	}
	definitions, err := svc.service.repo.ListActiveRankingPortfolioDefinitions(ctx)
	if err != nil {
		return nil, err
	}
	if len(definitions) == 0 {
		resp.CanApply = false
		resp.Severity = "blocked"
		resp.Message = "没有可用的模拟组合定义。"
		resp.BlockingReasons = append(resp.BlockingReasons, SimPortfolioTrackingStartReason{Scope: "portfolio", Code: "no_active_portfolio", Message: resp.Message})
		return resp, nil
	}
	marketDates := map[string][]string{}
	marketNext := map[string]string{}
	latestSignals := []string{}
	for _, exchange := range []string{"ASHARE", "HKEX"} {
		dates, err := svc.service.repo.ListRankingSnapshotDatesByExchangeRange(ctx, exchange, startSignalDate, "")
		if err != nil {
			return nil, err
		}
		latest, err := svc.service.repo.GetLatestRankingSnapshotDateByExchange(ctx, exchange)
		if err != nil {
			return nil, err
		}
		if latest != "" {
			latestSignals = append(latestSignals, latest)
		}
		marketDates[exchange] = dates
		count, err := svc.service.repo.CountRankingSnapshotsByExchangeDate(ctx, exchange, startSignalDate)
		if err != nil {
			return nil, err
		}
		market := SimPortfolioTrackingStartMarketPreview{
			Exchange:         exchange,
			Label:            simPortfolioExchangeLabel(exchange),
			HasSnapshot:      count > 0,
			StartSignalDate:  startSignalDate,
			LatestSignalDate: latest,
			Status:           "ok",
		}
		if len(dates) > 0 {
			market.SnapshotCountToLatest = len(dates)
		}
		if count == 0 {
			market.Status = "blocked"
			market.Message = fmt.Sprintf("%s 在 %s 没有排行榜快照。", market.Label, startSignalDate)
			resp.BlockingReasons = append(resp.BlockingReasons, SimPortfolioTrackingStartReason{Scope: "market", Exchange: exchange, Code: "missing_snapshot", Message: market.Message})
		} else if len(dates) < 2 || dates[0] != startSignalDate {
			market.Status = "blocked"
			market.Message = fmt.Sprintf("%s 从 %s 开始缺少下一交易日快照，无法形成 T+1 建仓估值。", market.Label, startSignalDate)
			resp.BlockingReasons = append(resp.BlockingReasons, SimPortfolioTrackingStartReason{Scope: "market", Exchange: exchange, Code: "missing_next_snapshot", Message: market.Message})
		} else {
			market.NextEntryTradeDate = dates[1]
			marketNext[exchange] = dates[1]
			market.Message = fmt.Sprintf("%s 数据满足严格共同起点预检。", market.Label)
		}
		resp.Markets = append(resp.Markets, market)
	}
	resp.LatestSignalDate = commonLatestSignalDate(latestSignals)
	for _, definition := range definitions {
		portfolio := SimPortfolioTrackingStartPortfolioPreview{
			PortfolioID:   definition.ID,
			Name:          definition.Name,
			Exchange:      definition.Exchange,
			Status:        "ok",
			RequiredCount: definition.MaxHoldings,
			Message:       "可重算。",
		}
		tradeDate := marketNext[strings.ToUpper(strings.TrimSpace(definition.Exchange))]
		portfolio.FirstEntryTradeDate = tradeDate
		portfolio.FirstValuationTradeDate = tradeDate
		signalItems, warningText, err := svc.service.selectSimPortfolioSignal(ctx, definition, startSignalDate)
		if err != nil {
			return nil, err
		}
		portfolio.SelectedCount = len(signalItems)
		if len(signalItems) < definition.MaxHoldings {
			portfolio.Status = "blocked"
			portfolio.Message = fmt.Sprintf("%s 在 %s 只能选出 %d 只，少于要求 %d 只。", definition.Name, startSignalDate, len(signalItems), definition.MaxHoldings)
			if warningText != "" {
				portfolio.Message += warningText
			}
			resp.BlockingReasons = append(resp.BlockingReasons, SimPortfolioTrackingStartReason{Scope: "portfolio", Exchange: definition.Exchange, PortfolioID: definition.ID, Code: "insufficient_constituents", Message: portfolio.Message})
		}
		if tradeDate == "" && portfolio.Status == "ok" {
			portfolio.Status = "blocked"
			portfolio.Message = "缺少下一交易日，无法检查建仓开盘价。"
			resp.BlockingReasons = append(resp.BlockingReasons, SimPortfolioTrackingStartReason{Scope: "portfolio", Exchange: definition.Exchange, PortfolioID: definition.ID, Code: "missing_next_trade_date", Message: portfolio.Message})
		}
		if portfolio.Status != "blocked" {
			for dateIndex := 0; dateIndex < len(marketDates[strings.ToUpper(strings.TrimSpace(definition.Exchange))])-1; dateIndex++ {
				checkSignalDate := marketDates[strings.ToUpper(strings.TrimSpace(definition.Exchange))][dateIndex]
				checkTradeDate := marketDates[strings.ToUpper(strings.TrimSpace(definition.Exchange))][dateIndex+1]
				checkItems, _, err := svc.service.selectSimPortfolioSignal(ctx, definition, checkSignalDate)
				if err != nil {
					return nil, err
				}
				if len(checkItems) < definition.MaxHoldings {
					portfolio.Status = "blocked"
					portfolio.Message = fmt.Sprintf("%s 在 %s 后续重算中成分股不足。", definition.Name, checkSignalDate)
					resp.BlockingReasons = append(resp.BlockingReasons, SimPortfolioTrackingStartReason{Scope: "portfolio", Exchange: definition.Exchange, PortfolioID: definition.ID, Code: "insufficient_constituents", Message: portfolio.Message})
					break
				}
				for _, item := range checkItems {
					openPrice, _, err := svc.service.repo.GetRankingPortfolioSelectionOpenPrice(ctx, definition.ID, checkSignalDate, item.Code, item.Exchange)
					if err != nil {
						return nil, err
					}
					if openPrice <= 0 && svc.service.openPriceResolver != nil {
						openPrice = svc.service.openPriceResolver(ctx, item.Code, item.Exchange, checkTradeDate)
					}
					if openPrice <= 0 {
						portfolio.MissingOpenPriceCount++
					}
					closePrice, err := svc.service.repo.GetClosePriceByTradeDate(ctx, item.Code, item.Exchange, checkTradeDate)
					if err != nil {
						return nil, err
					}
					if closePrice <= 0 {
						portfolio.MissingClosePriceCount++
					}
				}
			}
			if portfolio.MissingOpenPriceCount > 0 {
				portfolio.Status = "blocked"
				portfolio.Message = fmt.Sprintf("首次建仓日 %s 有 %d 只成分股缺少开盘价。", tradeDate, portfolio.MissingOpenPriceCount)
				resp.BlockingReasons = append(resp.BlockingReasons, SimPortfolioTrackingStartReason{Scope: "portfolio", Exchange: definition.Exchange, PortfolioID: definition.ID, Code: "missing_open_price", Message: portfolio.Message})
			} else if portfolio.MissingClosePriceCount > 0 {
				portfolio.Status = "blocked"
				portfolio.Message = fmt.Sprintf("首次估值日 %s 有 %d 只成分股缺少收盘价。", tradeDate, portfolio.MissingClosePriceCount)
				resp.BlockingReasons = append(resp.BlockingReasons, SimPortfolioTrackingStartReason{Scope: "portfolio", Exchange: definition.Exchange, PortfolioID: definition.ID, Code: "missing_close_price", Message: portfolio.Message})
			}
		}
		resp.Portfolios = append(resp.Portfolios, portfolio)
	}
	if len(resp.BlockingReasons) > 0 {
		resp.CanApply = false
		resp.Severity = "blocked"
		resp.Message = "该日期不能作为 4 个模拟组合的统一开始信号日。"
	} else {
		resp.Message = "该日期可作为 4 个模拟组合的统一开始信号日。"
	}
	return resp, nil
}

func (svc *SimPortfolioTrackingStartService) countFactRows(ctx context.Context, portfolioID string) (int, int, int, int, error) {
	dailyRows, err := svc.service.repo.ListAllSimPortfolioDaily(ctx, portfolioID)
	if err != nil {
		return 0, 0, 0, 0, err
	}
	positionCount, err := svc.service.repo.CountSimPortfolioPositions(ctx, portfolioID)
	if err != nil {
		return 0, 0, 0, 0, err
	}
	tradeCount, err := svc.service.repo.CountSimPortfolioTrades(ctx, portfolioID)
	if err != nil {
		return 0, 0, 0, 0, err
	}
	metricsCount, err := svc.service.repo.CountSimPortfolioMetrics(ctx, portfolioID)
	if err != nil {
		return 0, 0, 0, 0, err
	}
	return len(dailyRows), positionCount, tradeCount, metricsCount, nil
}

func newSimPortfolioTrackingApplyJob(startSignalDate string, requestedBy string) *SimPortfolioTrackingJob {
	now := time.Now().UTC()
	return &SimPortfolioTrackingJob{
		ID:                       fmt.Sprintf("sim_start_%s_%d", strings.ReplaceAll(startSignalDate, "-", ""), now.UnixNano()),
		JobType:                  simPortfolioTrackingJobApply,
		RequestedStartSignalDate: strings.TrimSpace(startSignalDate),
		Status:                   "running",
		RequestedBy:              strings.TrimSpace(requestedBy),
		StartedAt:                now,
		CreatedAt:                now,
		UpdatedAt:                now,
	}
}

func toSimPortfolioTrackingJobSummary(job SimPortfolioTrackingJob) *SimPortfolioTrackingJobSummary {
	out := &SimPortfolioTrackingJobSummary{JobID: job.ID, Status: job.Status, Message: job.Message}
	if !job.StartedAt.IsZero() {
		out.StartedAt = job.StartedAt.Format(time.RFC3339)
	}
	if !job.FinishedAt.IsZero() {
		out.FinishedAt = job.FinishedAt.Format(time.RFC3339)
	}
	return out
}

func mustJSON(value any) string {
	data, err := json.Marshal(value)
	if err != nil {
		return "{}"
	}
	return string(data)
}

func simPortfolioExchangeLabel(exchange string) string {
	switch strings.ToUpper(strings.TrimSpace(exchange)) {
	case "HKEX":
		return "港股"
	default:
		return "A股"
	}
}

func commonLatestSignalDate(values []string) string {
	out := ""
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if out == "" || value < out {
			out = value
		}
	}
	return out
}
