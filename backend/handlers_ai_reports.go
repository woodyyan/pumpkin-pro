package main

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/woodyyan/pumpkin-pro/backend/store/aireport"
)

func (a *appServer) handleAIReports(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "Only GET method is allowed")
		return
	}
	items, err := a.aiReportService.ListPublicReports(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "获取 AI 研报列表失败")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (a *appServer) handleAIReportSubroutes(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "Only GET method is allowed")
		return
	}
	id, action := parseAIReportSubroute(r.URL.Path, "/api/ai/reports/")
	if id == "" || action != "preview" {
		writeError(w, http.StatusNotFound, "AI 研报不存在")
		return
	}
	preview, err := a.aiReportService.GetPreview(r.Context(), id)
	if err != nil {
		if errors.Is(err, aireport.ErrReportNotFound) {
			writeError(w, http.StatusNotFound, "AI 研报不存在")
			return
		}
		writeError(w, http.StatusInternalServerError, "获取 AI 研报预览失败")
		return
	}
	writeJSON(w, http.StatusOK, preview)
}

func (a *appServer) handleAIReportServiceConfig(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "Only GET method is allowed")
		return
	}
	view, err := a.aiReportService.GetServiceConfig(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "获取 AI 研报服务配置失败")
		return
	}
	writeJSON(w, http.StatusOK, view)
}

func (a *appServer) handleAdminAIReports(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		items, err := a.aiReportService.ListAdminReports(r.Context())
		if err != nil {
			writeError(w, http.StatusInternalServerError, "获取 AI 研报管理列表失败")
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"items": items})
	case http.MethodPost:
		var input aireport.SaveReportInput
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
			writeError(w, http.StatusBadRequest, "AI 研报请求格式错误")
			return
		}
		item, err := a.aiReportService.CreateReport(r.Context(), input)
		if err != nil {
			a.writeAIReportError(w, err, "创建 AI 研报失败")
			return
		}
		writeJSON(w, http.StatusCreated, item)
	default:
		writeError(w, http.StatusMethodNotAllowed, "Only GET and POST methods are allowed")
	}
}

func (a *appServer) handleAdminAIReportSubroutes(w http.ResponseWriter, r *http.Request) {
	id, action := parseAIReportSubroute(r.URL.Path, "/api/admin/ai-reports/")
	if id == "" || action != "" {
		writeError(w, http.StatusNotFound, "AI 研报不存在")
		return
	}
	switch r.Method {
	case http.MethodPut:
		var input aireport.SaveReportInput
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
			writeError(w, http.StatusBadRequest, "AI 研报请求格式错误")
			return
		}
		item, err := a.aiReportService.UpdateReport(r.Context(), id, input)
		if err != nil {
			a.writeAIReportError(w, err, "更新 AI 研报失败")
			return
		}
		writeJSON(w, http.StatusOK, item)
	case http.MethodDelete:
		if err := a.aiReportService.DeleteReport(r.Context(), id); err != nil {
			a.writeAIReportError(w, err, "删除 AI 研报失败")
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true})
	default:
		writeError(w, http.StatusMethodNotAllowed, "Only PUT and DELETE methods are allowed")
	}
}

func (a *appServer) handleAdminAIReportServiceConfig(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		view, err := a.aiReportService.GetServiceConfig(r.Context())
		if err != nil {
			writeError(w, http.StatusInternalServerError, "获取 AI 研报服务配置失败")
			return
		}
		writeJSON(w, http.StatusOK, view)
	case http.MethodPut:
		var input aireport.SaveServiceConfigInput
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
			writeError(w, http.StatusBadRequest, "AI 研报服务配置请求格式错误")
			return
		}
		view, err := a.aiReportService.SaveServiceConfig(r.Context(), input)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "保存 AI 研报服务配置失败")
			return
		}
		writeJSON(w, http.StatusOK, view)
	default:
		writeError(w, http.StatusMethodNotAllowed, "Only GET and PUT methods are allowed")
	}
}

func (a *appServer) writeAIReportError(w http.ResponseWriter, err error, fallback string) {
	switch {
	case errors.Is(err, aireport.ErrInvalidInput):
		writeError(w, http.StatusBadRequest, "请完整填写股票名称、代码、市场、数据截至日期和三类 COS 图片 Key")
	case errors.Is(err, aireport.ErrReportNotFound):
		writeError(w, http.StatusNotFound, "AI 研报不存在")
	default:
		writeError(w, http.StatusInternalServerError, fallback)
	}
}

func parseAIReportSubroute(path string, prefix string) (string, string) {
	suffix := strings.Trim(strings.TrimPrefix(path, prefix), "/")
	if suffix == "" || suffix == path {
		return "", ""
	}
	parts := strings.Split(suffix, "/")
	id := strings.TrimSpace(parts[0])
	if len(parts) == 1 {
		return id, ""
	}
	return id, strings.TrimSpace(parts[1])
}
