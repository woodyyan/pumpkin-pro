package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/woodyyan/pumpkin-pro/backend/store/admin"
	"github.com/woodyyan/pumpkin-pro/backend/store/analytics"
	"github.com/woodyyan/pumpkin-pro/backend/store/quadrant"
)

// handleAdminSystemHealth returns aggregated error monitoring data.
// GET /api/admin/system-health
func (a *appServer) handleAdminSystemHealth(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "Only GET method is allowed")
		return
	}
	stats, err := a.adminService.GetSystemHealthStats(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "获取系统健康数据失败")
		return
	}
	writeJSON(w, http.StatusOK, stats)
}

// handleAdminSystemHealthLogs returns paginated API error logs.
// GET /api/admin/system-health/logs?limit=20&offset=0
func (a *appServer) handleAdminSystemHealthLogs(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "Only GET method is allowed")
		return
	}

	limit := parseLimit(r.URL.Query().Get("limit"), 50)
	offset := parseOffset(r.URL.Query().Get("offset"), 0)

	items, total, err := a.adminService.ListAPIErrors(r.Context(), limit, offset)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "查询错误日志失败")
		return
	}

	logItems := make([]admin.APIErrorLogItem, len(items))
	for i, e := range items {
		msg := e.ErrorMessage
		if len([]rune(msg)) > 200 {
			msg = string([]rune(msg)[:200]) + "…"
		}
		logItems[i] = admin.APIErrorLogItem{
			ID:           e.ID,
			Method:       e.Method,
			Path:         e.Path,
			StatusCode:   e.StatusCode,
			ErrorCode:    e.ErrorCode,
			ErrorMessage: msg,
			DurationMS:   e.DurationMS,
			ClientIP:     e.ClientIP,
			UserID:       e.UserID,
			CreatedAt:    e.CreatedAt.Format("2006-01-02T15:04:05Z"),
		}
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"items": logItems,
		"total": total,
	})
}

// handleAdminUserFunnel returns the user conversion funnel data (7 layers).
// GET /api/admin/user-funnel
func (a *appServer) handleAdminUserFunnel(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "Only GET method is allowed")
		return
	}
	stats, err := a.adminService.GetFunnelStats(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "获取用户漏斗数据失败")
		return
	}
	writeJSON(w, http.StatusOK, stats)
}

// handleAdminAIConfig manages AI provider settings for the admin panel.
// GET /api/admin/ai-config
// PUT /api/admin/ai-config
func (a *appServer) handleAdminAIConfig(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		view, err := a.adminService.GetAIProviderConfigView(r.Context())
		if err != nil {
			writeError(w, http.StatusInternalServerError, "获取 AI 配置失败")
			return
		}
		writeJSON(w, http.StatusOK, view)
	case http.MethodPut:
		var input admin.SaveAIProviderConfigInput
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
			writeError(w, http.StatusBadRequest, "AI 配置请求格式错误")
			return
		}
		view, err := a.adminService.SaveAIProviderConfig(r.Context(), input)
		if err != nil {
			if errors.Is(err, admin.ErrAIConfigInvalid) {
				writeError(w, http.StatusBadRequest, "请检查 base URL、模型和 API Key 配置")
				return
			}
			if errors.Is(err, admin.ErrAIConfigCipherKeyUnset) {
				writeError(w, http.StatusInternalServerError, "服务器未配置 AI 配置加密密钥，暂时无法保存后台 AI 配置")
				return
			}
			writeError(w, http.StatusInternalServerError, "保存 AI 配置失败")
			return
		}
		writeJSON(w, http.StatusOK, view)
	default:
		writeError(w, http.StatusMethodNotAllowed, "Only GET and PUT methods are allowed")
	}
}

// handleAdminAIConfigTest validates a saved or draft AI provider config.
// POST /api/admin/ai-config/test
func (a *appServer) handleAdminAIConfigTest(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "Only POST method is allowed")
		return
	}
	body, err := io.ReadAll(r.Body)
	if err != nil {
		writeError(w, http.StatusBadRequest, "读取 AI 测试请求失败")
		return
	}
	var input *admin.TestAIProviderConfigInput
	if len(bytes.TrimSpace(body)) > 0 {
		var decoded admin.TestAIProviderConfigInput
		if err := json.Unmarshal(body, &decoded); err != nil {
			writeError(w, http.StatusBadRequest, "AI 测试请求格式错误")
			return
		}
		input = &decoded
	}
	result, err := a.adminService.TestAIProviderConfig(r.Context(), input)
	if err != nil {
		if errors.Is(err, admin.ErrAIConfigInvalid) {
			writeError(w, http.StatusBadRequest, "请检查 base URL、模型和 API Key 配置")
			return
		}
		if errors.Is(err, admin.ErrAIConfigCipherKeyUnset) {
			writeError(w, http.StatusInternalServerError, "服务器未配置 AI 配置加密密钥，暂时无法读取已保存的后台 AI 配置")
			return
		}
		writeError(w, http.StatusInternalServerError, "AI 测试连接失败")
		return
	}
	writeJSON(w, http.StatusOK, result)
}

// handleAdminAITokenUsage returns per-day per-user AI token usage details.
// GET /api/admin/ai-usage?days=30&limit=120
func (a *appServer) handleAdminAITokenUsage(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "Only GET method is allowed")
		return
	}

	days := 30
	if raw := strings.TrimSpace(r.URL.Query().Get("days")); raw != "" {
		value, err := strconv.Atoi(raw)
		if err != nil || value <= 0 || value > 180 {
			writeError(w, http.StatusBadRequest, "days 参数无效")
			return
		}
		days = value
	}
	limit := parseLimit(r.URL.Query().Get("limit"), 120)

	result, err := a.adminService.GetAITokenUsage(r.Context(), days, limit)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "获取 AI token 用量失败")
		return
	}
	writeJSON(w, http.StatusOK, result)
}

// handleAdminSystemHealthPurge triggers cleanup of old error logs (admin-only).
// POST /api/admin/system-health/purge
func (a *appServer) handleAdminSystemHealthPurge(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "Only POST method is allowed")
		return
	}

	daysRaw := r.URL.Query().Get("days")
	retentionDays := 30
	if daysRaw != "" {
		if v, err := strconv.Atoi(daysRaw); err == nil && v > 1 {
			retentionDays = v
		}
	}

	deleted, err := a.adminService.PurgeOldAPIErrors(r.Context(), retentionDays)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "清理失败")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"ok":        true,
		"deleted":   deleted,
		"kept_days": retentionDays,
	})
}

// ── Backup Admin Handlers ──

// handleAdminBackupStatus returns the latest backup status for the admin panel.
func (a *appServer) handleAdminBackupStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "Only GET method is allowed")
		return
	}
	status, err := a.backupService.GetStatus(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "获取备份状态失败")
		return
	}
	writeJSON(w, http.StatusOK, status)
}

// handleAdminBackupHistory returns recent backup log entries.
func (a *appServer) handleAdminBackupHistory(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "Only GET method is allowed")
		return
	}
	limit := parseLimit(r.URL.Query().Get("limit"), 7)
	items, err := a.backupService.GetHistory(limit)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "获取备份历史失败")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"items": items,
	})
}

// handleAdminBackupTrigger manually triggers a backup run.
func (a *appServer) handleAdminBackupTrigger(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "Only POST method is allowed")
		return
	}
	result, err := a.backupService.TriggerAsync(r.Context(), "manual")
	if err != nil {
		writeError(w, http.StatusInternalServerError, "手动备份失败: "+err.Error())
		return
	}
	statusCode := http.StatusAccepted
	if !result.Accepted {
		statusCode = http.StatusConflict
	}
	writeJSON(w, statusCode, result)
}

// handleAdminBackupStats returns storage usage statistics (local + cloud).
func (a *appServer) handleAdminBackupStats(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "Only GET method is allowed")
	}
	stats, err := a.backupService.GetStorageStats(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "获取存储统计失败")
		return
	}
	writeJSON(w, http.StatusOK, stats)
}

// ── Quadrant Monitoring Admin Handlers ──

// handleQuadrantProgress receives progress callbacks from Quant during computation.
// POST /api/quadrant/progress (internal, called by Quant)
func (a *appServer) handleQuadrantProgress(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "Only POST method is allowed")
		return
	}

	var p struct {
		Exchange string `json:"exchange"`
		Current  int    `json:"current"`
		Total    int    `json:"total"`
		Status   string `json:"status"`
		ErrorMsg string `json:"error_msg,omitempty"`
		Message  string `json:"message,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&p); err != nil {
		writeJSON(w, http.StatusOK, map[string]string{"ok": "1"}) //宽容：解析失败不阻塞 Quant
		return
	}

	quadrant.UpdateProgress(p.Exchange, quadrant.ComputeProgress{
		Exchange: p.Exchange,
		Status:   p.Status,
		Current:  p.Current,
		Total:    p.Total,
		ErrorMsg: p.ErrorMsg,
		Message:  p.Message,
	})
	writeJSON(w, http.StatusOK, map[string]string{"ok": "1"})
}

// handleAdminComputeStatus returns current computation progress for both exchanges.
// GET /api/admin/compute-status (super admin only)
func (a *appServer) handleAdminComputeStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "Only GET method is allowed")
		return
	}
	result := quadrant.GetProgress()
	writeJSON(w, http.StatusOK, result)
}

// handleAdminQuadrantTrigger manually triggers a quadrant recomputation.
// POST /api/admin/quadrant-trigger (super admin only)
func (a *appServer) handleAdminQuadrantTrigger(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "Only POST method is allowed")
		return
	}

	var req struct {
		Exchange string `json:"exchange"` // "ASHARE", "HKEX", or "ALL"
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "请求格式错误")
		return
	}

	exchange := strings.ToUpper(strings.TrimSpace(req.Exchange))
	if exchange == "" {
		exchange = "ALL"
	}

	var triggered []string
	switch exchange {
	case "HKEX":
		triggered = []string{"HKEX"}
	case "ASHARE", "ALL":
		triggered = []string{"ASHARE"}
	default:
		writeError(w, http.StatusBadRequest, "无效的 exchange 参数")
		return
	}

	results := make(map[string]any)
	for _, ex := range triggered {
		if ex == "ASHARE" {
			go a.quadrantService.TriggerComputeAShare()
			results["ASHARE"] = "A股四象限计算已触发，请查看进度"
		} else if ex == "HKEX" {
			go a.quadrantService.TriggerComputeHK()
			results["HKEX"] = "港股四象限计算已触发，请查看进度"
		}
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"ok":      true,
		"results": results,
		"message": fmt.Sprintf("已触发 %d 个市场的四象限计算", len(triggered)),
	})
}

// handleAdminQuadrantRecomputeDate triggers a market/date-specific upstream
// quadrant rebuild. Quant currently computes the latest available market data;
// the backend passes source_trade_date so updated Quant workers can publish
// the result back to /api/quadrant/bulk-save?source_trade_date=... without the
// sim-portfolio pipeline guessing from computed_at.
func (a *appServer) handleAdminQuadrantRecomputeDate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "Only POST method is allowed")
		return
	}
	var req struct {
		Market          string `json:"market"`
		SourceTradeDate string `json:"source_trade_date"`
		ForceFull       bool   `json:"force_full"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "请求格式错误")
		return
	}
	market := strings.ToUpper(strings.TrimSpace(req.Market))
	if market == "A" || market == "A股" {
		market = "ASHARE"
	}
	if market != "ASHARE" && market != "HKEX" {
		writeError(w, http.StatusBadRequest, "market 仅支持 ASHARE/HKEX")
		return
	}
	sourceTradeDate := strings.TrimSpace(req.SourceTradeDate)
	if _, err := time.Parse("2006-01-02", sourceTradeDate); err != nil {
		writeError(w, http.StatusBadRequest, "source_trade_date 必须为 YYYY-MM-DD")
		return
	}
	if sourceTradeDate > time.Now().In(time.FixedZone("CST", 8*60*60)).Format("2006-01-02") {
		writeError(w, http.StatusBadRequest, "不能重建未来日期四象限")
		return
	}
	if cal := quadrant.NewSimPortfolioV2CalendarService().CalendarRow(market, sourceTradeDate); !cal.IsTradingDay {
		writeError(w, http.StatusBadRequest, "该市场当日休市，不需要重建四象限")
		return
	}
	quantURL := strings.TrimRight(a.cfg.QuantServiceURL, "/")
	if quantURL == "" {
		writeError(w, http.StatusInternalServerError, "Quant 服务地址未配置")
		return
	}
	endpoint := "/api/quadrant/compute-all"
	if market == "HKEX" {
		endpoint = "/api/quadrant/compute-hk-all"
	}
	// Set progress to "running" so the admin UI progress bar reflects the
	// rebuild. The terminal state (success/failed) will be set by the
	// BulkSave callback when Quant finishes writing results back.
	quadrant.UpdateProgress(market, quadrant.ComputeProgress{
		Exchange: market,
		Status:   "running",
		Message:  fmt.Sprintf("正在重建 %s 四象限 (%s)…", market, sourceTradeDate),
	})

	callback := strings.TrimRight(a.cfg.BackendCallbackURL, "/") + fmt.Sprintf("/api/quadrant/bulk-save?source_trade_date=%s", sourceTradeDate)
	payload := map[string]any{"callback_url": callback, "force_full": req.ForceFull, "source_trade_date": sourceTradeDate}
	body, _ := json.Marshal(payload)
	httpReq, err := http.NewRequestWithContext(r.Context(), http.MethodPost, quantURL+endpoint, bytes.NewReader(body))
	if err != nil {
		quadrant.SetProgressTerminal(market, "failed", fmt.Sprintf("创建请求失败: %v", err))
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	httpReq.Header.Set("Content-Type", "application/json")
	resp, err := (&http.Client{Timeout: 10 * time.Second}).Do(httpReq)
	if err != nil {
		quadrant.SetProgressTerminal(market, "failed", fmt.Sprintf("触发 Quant 重建失败: %v", err))
		writeError(w, http.StatusBadGateway, fmt.Sprintf("触发 Quant 重建失败: %v", err))
		return
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		errMsg := fmt.Sprintf("Quant 返回 HTTP %d: %s", resp.StatusCode, string(respBody))
		quadrant.SetProgressTerminal(market, "failed", errMsg)
		writeError(w, http.StatusBadGateway, errMsg)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "market": market, "source_trade_date": sourceTradeDate, "status": "accepted", "message": "已触发指定日期四象限重建，请在上方进度条查看实时状态；完成后请重新运行对应日期模拟组合 pipeline。"})
}

// LEGACY: handleAdminRankingPortfolioVerify runs a read-only replay of the old
// JSON NAV series and returns per-definition diff reports.
// POST /api/admin/ranking-portfolio-verify
func (a *appServer) handleAdminRankingPortfolioVerify(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "Only POST method is allowed")
		return
	}
	result, err := a.quadrantService.VerifyAllRankingPortfolioResults(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "验证收益曲线失败: "+err.Error())
		return
	}
	writeJSON(w, http.StatusOK, result)
}

// LEGACY: handleAdminRankingPortfolioFix repairs the old JSON result series.
// It requires a valid verify_token issued by the legacy verify endpoint
// (expires in 10 min, single-use).
// POST /api/admin/ranking-portfolio-fix
func (a *appServer) handleAdminRankingPortfolioFix(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "Only POST method is allowed")
		return
	}
	var req struct {
		VerifyToken string `json:"verify_token"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "请求格式错误")
		return
	}
	if strings.TrimSpace(req.VerifyToken) == "" {
		writeError(w, http.StatusBadRequest, "verify_token 不能为空，请先执行验证")
		return
	}
	if err := a.quadrantService.FixAllVerifiedRankingPortfolioResults(r.Context(), req.VerifyToken); err != nil {
		writeError(w, http.StatusBadRequest, "修复失败: "+err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":      true,
		"message": "已完成收益曲线修复，请刷新状态确认。",
	})
}

// LEGACY: handleAdminRankingPortfolioStatus returns admin status for the old JSON result chain.
func (a *appServer) handleAdminRankingPortfolioStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "Only GET method is allowed")
		return
	}
	cutoverDate := a.cfg.RankingPortfolioRealtime.CutoverDate
	resp, err := a.quadrantService.GetRankingPortfolioAdminStatusWithCutover(r.Context(), cutoverDate)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if resp == nil {
		resp = &quadrant.RankingPortfolioAdminStatusResponse{Items: []quadrant.RankingPortfolioAdminStatusItem{}}
	}
	writeJSON(w, http.StatusOK, resp)
}

// LEGACY: handleAdminRankingPortfolioRepair triggers the old open-price backfill and JSON rebuild path.
func (a *appServer) handleAdminRankingPortfolioRepair(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "Only POST method is allowed")
		return
	}
	cutoverDate := a.cfg.RankingPortfolioRealtime.CutoverDate
	result, err := a.quadrantService.TriggerRankingPortfolioRepairWithResult(r.Context(), cutoverDate)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "触发开盘价补齐与曲线重算失败: "+err.Error())
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]any{
		"ok":      true,
		"message": "已触发开盘价补齐与曲线重算，请稍后刷新后台状态。",
		"summary": result,
	})
}

// handleAdminDeviceAnalytics returns device/browser/OS analytics for the admin panel.
// GET /api/admin/device-analytics?days=30
func (a *appServer) handleAdminDeviceAnalytics(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "Only GET method is allowed")
		return
	}

	days := 30
	if raw := strings.TrimSpace(r.URL.Query().Get("days")); raw != "" {
		value, err := strconv.Atoi(raw)
		if err != nil || value < 0 || value > 365 {
			writeError(w, http.StatusBadRequest, "days 参数无效")
			return
		}
		days = value
	}

	var since time.Time
	if days == 0 {
		since = time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC)
	} else {
		since = time.Now().UTC().AddDate(0, 0, -days)
	}

	ctx := r.Context()
	result, err := a.analyticsRepo.GetDeviceAnalytics(ctx, since)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "获取设备分析数据失败")
		return
	}

	// Ensure non-nil arrays
	if result.DeviceTypes == nil {
		result.DeviceTypes = []analytics.CategoryCount{}
	}
	if result.OSFamilies == nil {
		result.OSFamilies = []analytics.CategoryCount{}
	}
	if result.BrowserFamilies == nil {
		result.BrowserFamilies = []analytics.CategoryCount{}
	}

	// Fetch top active users with their devices
	topUsers, err := a.analyticsRepo.GetTopActiveUsersWithDevices(ctx, since, 20)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "获取活跃用户数据失败")
		return
	}
	if topUsers == nil {
		topUsers = []analytics.TopActiveUserDevice{}
	}
	result.TopActiveUsers = topUsers

	writeJSON(w, http.StatusOK, result)
}

// ── Site Config: Community QR ──

// handlePublicCommunityQRConfig returns the community QR config (public, no auth).
// GET /api/site-config/community
func (a *appServer) handlePublicCommunityQRConfig(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "Only GET method is allowed")
		return
	}
	config, err := a.adminService.GetCommunityQRConfig(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "获取交流群二维码配置失败")
		return
	}
	writeJSON(w, http.StatusOK, config)
}

// handleAdminCommunityQRConfig manages the community QR config (super-admin only).
// GET  /api/admin/site-config/community
// PUT  /api/admin/site-config/community
func (a *appServer) handleAdminCommunityQRConfig(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		view, err := a.adminService.GetCommunityQRConfigAdminView(r.Context())
		if err != nil {
			writeError(w, http.StatusInternalServerError, "获取交流群二维码配置失败")
			return
		}
		writeJSON(w, http.StatusOK, view)

	case http.MethodPut:
		var input admin.SaveCommunityQRConfigInput
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
			writeError(w, http.StatusBadRequest, "请求格式错误")
			return
		}
		view, err := a.adminService.SaveCommunityQRConfig(r.Context(), input)
		if err != nil {
			if errors.Is(err, admin.ErrSiteConfigInvalid) {
				writeError(w, http.StatusBadRequest, err.Error())
				return
			}
			writeError(w, http.StatusInternalServerError, "保存交流群二维码配置失败")
			return
		}
		writeJSON(w, http.StatusOK, view)

	default:
		writeError(w, http.StatusMethodNotAllowed, "Only GET and PUT methods are allowed")
	}
}
