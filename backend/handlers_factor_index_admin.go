package main

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/woodyyan/pumpkin-pro/backend/store/factorindex"
)

func (a *appServer) handleAdminFactorIndexStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "Only GET method is allowed")
		return
	}
	if a.factorIndexService == nil || a.factorIndexWorker == nil {
		writeError(w, http.StatusServiceUnavailable, "单因子指数运维服务未初始化")
		return
	}
	status, err := a.factorIndexService.AdminStatus(r.Context(), a.factorIndexWorker.Snapshot(time.Now()))
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, status)
}

func (a *appServer) handleAdminFactorIndexRecompute(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "Only POST method is allowed")
		return
	}
	if a.factorIndexWorker == nil {
		writeError(w, http.StatusServiceUnavailable, "单因子指数运维服务未初始化")
		return
	}
	var req factorindex.ManualRunRequest
	if r.Body != nil {
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil && err != io.EOF {
			writeError(w, http.StatusBadRequest, "请求格式错误")
			return
		}
	}
	run, err := a.factorIndexWorker.StartManual(r.Context(), req)
	if err != nil {
		statusCode := http.StatusBadRequest
		switch {
		case strings.Contains(err.Error(), "already running"):
			statusCode = http.StatusConflict
		case strings.Contains(err.Error(), "disabled"), strings.Contains(err.Error(), "not initialized"):
			statusCode = http.StatusServiceUnavailable
		}
		writeError(w, statusCode, err.Error())
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]any{
		"ok":      true,
		"message": "已触发单因子指数补算任务，请稍后刷新状态。",
		"run":     run,
	})
}
