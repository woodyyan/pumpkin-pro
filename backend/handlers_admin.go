package main

import (
	"net/http"
	"strconv"

	"github.com/woodyyan/pumpkin-pro/backend/store/admin"
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
		"ok":          true,
		"deleted":     deleted,
		"kept_days":   retentionDays,
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
	result, err := a.backupService.Run(r.Context(), "manual")
	if err != nil {
		writeError(w, http.StatusInternalServerError, "手动备份失败: "+err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":     true,
		"status": result.Status,
		"result": result,
	})
}

// handleAdminBackupStats returns storage usage statistics (local + cloud).
func (a *appServer) handleAdminBackupStats(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "Only GET method is allowed")
		return
	}
	stats, err := a.backupService.GetStorageStats(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "获取存储统计失败")
		return
	}
	writeJSON(w, http.StatusOK, stats)
}
