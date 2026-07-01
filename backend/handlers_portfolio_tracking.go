package main

import (
	"encoding/json"
	"net/http"
	"strings"
)

func (a *appServer) handlePortfolioTrackingOverview(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "Only GET method is allowed")
		return
	}
	resp, err := a.quadrantService.GetSimPortfolioOverview(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

func (a *appServer) handlePortfolioTrackingSubroutes(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "Only GET method is allowed")
		return
	}
	path := strings.TrimPrefix(r.URL.Path, "/api/portfolio-tracking/")
	path = strings.Trim(path, "/")
	parts := strings.Split(path, "/")
	if len(parts) != 2 {
		writeError(w, http.StatusNotFound, "Not found")
		return
	}
	portfolioID := strings.TrimSpace(parts[0])
	if portfolioID == "" {
		writeError(w, http.StatusBadRequest, "portfolio_id 不能为空")
		return
	}
	switch parts[1] {
	case "daily":
		resp, err := a.quadrantService.GetSimPortfolioDaily(r.Context(), portfolioID, r.URL.Query().Get("from"), r.URL.Query().Get("to"))
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, resp)
	case "positions":
		resp, err := a.quadrantService.GetSimPortfolioPositions(r.Context(), portfolioID, r.URL.Query().Get("trade_date"))
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, resp)
	case "trades":
		resp, err := a.quadrantService.GetSimPortfolioTrades(r.Context(), portfolioID, r.URL.Query().Get("from"), r.URL.Query().Get("to"), r.URL.Query().Get("action"))
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, resp)
	case "metrics":
		resp, err := a.quadrantService.GetSimPortfolioMetrics(r.Context(), portfolioID)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, resp)
	default:
		writeError(w, http.StatusNotFound, "Not found")
	}
}

func (a *appServer) handleAdminPortfolioTrackingStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "Only GET method is allowed")
		return
	}
	resp, err := a.quadrantService.GetSimPortfolioAdminStatus(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

func (a *appServer) handleAdminPortfolioTrackingVerify(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "Only POST method is allowed")
		return
	}
	var req struct {
		PortfolioID string `json:"portfolio_id"`
	}
	if r.Body != nil {
		_ = json.NewDecoder(r.Body).Decode(&req)
	}
	resp, err := a.quadrantService.VerifySimPortfolios(r.Context(), req.PortfolioID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

func (a *appServer) handleAdminPortfolioTrackingSync(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "Only POST method is allowed")
		return
	}
	resp, err := a.quadrantService.SyncSimPortfolios(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

func (a *appServer) handleAdminPortfolioTrackingRecompute(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "Only POST method is allowed")
		return
	}
	var req struct {
		PortfolioID string `json:"portfolio_id"`
		FromDate    string `json:"from_date"`
		ToDate      string `json:"to_date"`
		Reset       *bool  `json:"reset"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "请求格式错误")
		return
	}
	reset := true
	if req.Reset != nil {
		reset = *req.Reset
	}
	if err := a.quadrantService.RecomputeSimPortfolios(r.Context(), req.PortfolioID, req.FromDate, req.ToDate, reset); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":      true,
		"message": "模拟组合已完成重算。",
	})
}

func (a *appServer) handleAdminPortfolioTrackingReset(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "Only POST method is allowed")
		return
	}
	var req struct {
		PortfolioID string `json:"portfolio_id"`
	}
	if r.Body != nil {
		_ = json.NewDecoder(r.Body).Decode(&req)
	}
	if err := a.quadrantService.ResetSimPortfolios(r.Context(), req.PortfolioID); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":      true,
		"message": "模拟组合数据已清空。",
	})
}

func (a *appServer) handleAdminPortfolioTrackingBackfillOpenPrices(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "Only POST method is allowed")
		return
	}
	var req struct {
		PortfolioID string `json:"portfolio_id"`
		Exchange    string `json:"exchange"`
		LatestOnly  *bool  `json:"latest_only"`
	}
	if r.Body != nil {
		_ = json.NewDecoder(r.Body).Decode(&req)
	}
	latestOnly := true
	if req.LatestOnly != nil {
		latestOnly = *req.LatestOnly
	}
	resp, err := a.quadrantService.BackfillSimPortfolioOpenPrices(r.Context(), req.PortfolioID, req.Exchange, latestOnly)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, resp)
}
