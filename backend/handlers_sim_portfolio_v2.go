package main

import (
	"encoding/json"
	"net/http"

	"github.com/woodyyan/pumpkin-pro/backend/store/quadrant"
)

func (a *appServer) handleAdminSimPortfolioV2Overview(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "Only GET method is allowed")
		return
	}
	if a.simPortfolioV2Service == nil {
		writeError(w, http.StatusInternalServerError, "Sim Portfolio v2 服务未配置")
		return
	}
	resp, err := a.simPortfolioV2Service.GetAdminOverview(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

func (a *appServer) handleAdminSimPortfolioV2Days(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "Only GET method is allowed")
		return
	}
	if a.simPortfolioV2Service == nil {
		writeError(w, http.StatusInternalServerError, "Sim Portfolio v2 服务未配置")
		return
	}
	resp, err := a.simPortfolioV2Service.GetAdminDays(r.Context(), r.URL.Query().Get("market"), r.URL.Query().Get("from"), r.URL.Query().Get("to"))
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

func (a *appServer) handleAdminSimPortfolioV2Initialize(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "Only POST method is allowed")
		return
	}
	if a.simPortfolioV2Service == nil {
		writeError(w, http.StatusInternalServerError, "Sim Portfolio v2 服务未配置")
		return
	}
	var req struct {
		Market string `json:"market"`
	}
	if r.Body != nil {
		_ = json.NewDecoder(r.Body).Decode(&req)
	}
	resp, err := a.simPortfolioV2Service.Initialize(r.Context(), req.Market)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

func (a *appServer) handleAdminSimPortfolioV2Run(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "Only POST method is allowed")
		return
	}
	if a.simPortfolioV2Service == nil {
		writeError(w, http.StatusInternalServerError, "Sim Portfolio v2 服务未配置")
		return
	}
	var req quadrant.SimPortfolioV2RunRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "请求格式错误")
		return
	}
	resp, err := a.simPortfolioV2Service.Run(r.Context(), req)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, resp)
}
