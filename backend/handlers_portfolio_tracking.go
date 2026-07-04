package main

import (
	"net/http"
	"strings"
)

func (a *appServer) handlePortfolioTrackingOverview(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "Only GET method is allowed")
		return
	}
	resp, err := a.simPortfolioV2Service.GetPortfolioOverview(r.Context())
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
		resp, err := a.simPortfolioV2Service.GetPortfolioDaily(r.Context(), portfolioID, r.URL.Query().Get("from"), r.URL.Query().Get("to"))
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, resp)
	case "positions":
		resp, err := a.simPortfolioV2Service.GetPortfolioPositions(r.Context(), portfolioID, r.URL.Query().Get("trade_date"))
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, resp)
	case "trades":
		resp, err := a.simPortfolioV2Service.GetPortfolioTrades(r.Context(), portfolioID, r.URL.Query().Get("from"), r.URL.Query().Get("to"), r.URL.Query().Get("action"))
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, resp)
	case "metrics":
		resp, err := a.simPortfolioV2Service.GetPortfolioMetrics(r.Context(), portfolioID)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, resp)
	default:
		writeError(w, http.StatusNotFound, "Not found")
	}
}
