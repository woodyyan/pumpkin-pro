package main

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/woodyyan/pumpkin-pro/backend/store/companyprofile"
	"github.com/woodyyan/pumpkin-pro/backend/store/quadrant"
	"github.com/woodyyan/pumpkin-pro/backend/tests/testutil"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (fn roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return fn(req)
}

func TestHandleAdminDataSourceHealth(t *testing.T) {
	db := testutil.InMemoryDB(t)
	testutil.AutoMigrateModels(t, db, &companyprofile.CompanyProfileRecord{}, &companyprofile.IndustryMappingRecord{}, &quadrant.QuadrantScoreRecord{})
	repo := companyprofile.NewRepository(db)
	now := time.Now().UTC()
	if err := db.Create(&quadrant.QuadrantScoreRecord{Code: "600519", Name: "贵州茅台", Exchange: "SSE", Board: "MAIN", ComputedAt: now}).Error; err != nil {
		t.Fatalf("seed universe failed: %v", err)
	}
	if err := repo.Upsert(context.Background(), companyprofile.CompanyProfileRecord{
		Symbol:        "600519.SH",
		Exchange:      "SSE",
		Code:          "600519",
		Name:          "贵州茅台",
		ListingStatus: companyprofile.ListingStatusListed,
		ProfileStatus: companyprofile.ProfileStatusComplete,
		QualityFlags:  `[]`,
		CreatedAt:     now,
		UpdatedAt:     now,
	}); err != nil {
		t.Fatalf("seed company profile failed: %v", err)
	}

	oldTransport := http.DefaultTransport
	http.DefaultTransport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.Path != "/api/data-sources/health" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		body := `{"providers":{"eastmoney":{"success":2,"failed":1,"last_status":"success"}},"capabilities":{"company_profile":{"success":1,"failed":0,"last_provider":"eastmoney","last_status":"success","last_market":"ASHARE"}},"total_events":3,"recent":[]}`
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body:       io.NopCloser(strings.NewReader(body)),
		}, nil
	})
	defer func() { http.DefaultTransport = oldTransport }()

	svc := companyprofile.NewService(repo)
	svc.SetQuantServiceURL("http://quant.test")
	server := &appServer{companyProfileService: svc}

	req := httptest.NewRequest(http.MethodGet, "/api/admin/data-source-health", nil)
	resp := httptest.NewRecorder()
	server.handleAdminDataSourceHealth(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.Code)
	}
	var payload map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if payload["data_source_health"] == nil {
		t.Fatalf("expected data_source_health, got %+v", payload)
	}
}

func TestHandleAdminCompanyProfilesRefresh(t *testing.T) {
	db := testutil.InMemoryDB(t)
	testutil.AutoMigrateModels(t, db, &companyprofile.CompanyProfileRecord{}, &companyprofile.IndustryMappingRecord{})
	repo := companyprofile.NewRepository(db)
	svc := companyprofile.NewService(repo)
	svc.SetQuantServiceURL("http://quant.test")
	server := &appServer{companyProfileService: svc}

	req := httptest.NewRequest(http.MethodPost, "/api/admin/company-profiles/refresh", strings.NewReader(`{"exchange":"ALL","limit":10}`))
	req.Header.Set("Content-Type", "application/json")
	resp := httptest.NewRecorder()
	server.handleAdminCompanyProfilesRefresh(resp, req)

	if resp.Code != http.StatusAccepted {
		t.Fatalf("expected 202, got %d", resp.Code)
	}
	var payload map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if payload["ok"] != true {
		t.Fatalf("expected ok=true, got %+v", payload)
	}
}
