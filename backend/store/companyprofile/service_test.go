package companyprofile

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/woodyyan/pumpkin-pro/backend/store/quadrant"
	"github.com/woodyyan/pumpkin-pro/backend/tests/testutil"
)

func TestServiceGetAboutReturnsProfile(t *testing.T) {
	db := testutil.InMemoryDB(t)
	testutil.AutoMigrateModels(t, db, &CompanyProfileRecord{}, &IndustryMappingRecord{})
	repo := NewRepository(db)
	svc := NewService(repo)
	ctx := context.Background()
	now := time.Now().UTC()

	if err := repo.Upsert(ctx, CompanyProfileRecord{
		Symbol:                "600519.SH",
		Exchange:              "SSE",
		Code:                  "600519",
		Name:                  "贵州茅台",
		BoardName:             "主板",
		RawIndustryName:       "食品饮料Ⅰ",
		IndustryName:          "食品饮料",
		IndustrySource:        "sw",
		ListingStatus:         ListingStatusListed,
		BusinessSummary:       "贵州茅台主要从事茅台酒及系列酒的生产与销售，所属行业为食品饮料。",
		BusinessSummarySource: SummarySourceExtract,
		ProfileStatus:         ProfileStatusComplete,
		QualityFlags:          `["missing_website"]`,
		Source:                "eastmoney",
		SourceUpdatedAt:       now,
		CreatedAt:             now,
		UpdatedAt:             now,
	}); err != nil {
		t.Fatalf("seed profile failed: %v", err)
	}

	payload, err := svc.GetAbout(ctx, "600519")
	if err != nil {
		t.Fatalf("GetAbout failed: %v", err)
	}
	if !payload.HasProfile {
		t.Fatal("expected has_profile=true")
	}
	if payload.Symbol != "600519.SH" || payload.Exchange != "SSE" {
		t.Fatalf("symbol/exchange = %s/%s", payload.Symbol, payload.Exchange)
	}
	if payload.Profile.RawIndustryName != "食品饮料Ⅰ" || payload.Profile.IndustryName != "食品饮料" {
		t.Fatalf("industry = raw %q clean %q", payload.Profile.RawIndustryName, payload.Profile.IndustryName)
	}
	if len(payload.Meta.QualityFlags) != 1 || payload.Meta.QualityFlags[0] != "missing_website" {
		t.Fatalf("quality flags = %#v", payload.Meta.QualityFlags)
	}
}

func TestServiceGetAboutPendingAndInvalidSymbol(t *testing.T) {
	db := testutil.InMemoryDB(t)
	testutil.AutoMigrateModels(t, db, &CompanyProfileRecord{}, &IndustryMappingRecord{})
	svc := NewService(NewRepository(db))
	ctx := context.Background()

	payload, err := svc.GetAbout(ctx, "00700")
	if err != nil {
		t.Fatalf("GetAbout pending failed: %v", err)
	}
	if payload.HasProfile {
		t.Fatal("expected pending has_profile=false")
	}
	if payload.Symbol != "00700.HK" || payload.Exchange != "HKEX" {
		t.Fatalf("pending symbol/exchange = %s/%s", payload.Symbol, payload.Exchange)
	}
	if payload.Meta.ProfileStatus != ProfileStatusPending || payload.Meta.Message == "" {
		t.Fatalf("unexpected pending meta: %#v", payload.Meta)
	}

	if _, err := svc.GetAbout(ctx, "ABC"); err == nil {
		t.Fatal("expected invalid symbol error")
	}
}

func TestServiceManualRefreshUsesQuantAndMarksDelisted(t *testing.T) {
	db := testutil.InMemoryDB(t)
	testutil.AutoMigrateModels(t, db, &CompanyProfileRecord{}, &IndustryMappingRecord{}, &quadrant.QuadrantScoreRecord{})
	repo := NewRepository(db)
	now := time.Now().UTC()
	if err := db.Create(&quadrant.QuadrantScoreRecord{Code: "600519", Name: "贵州茅台", Exchange: "SSE", Board: "MAIN", ComputedAt: now}).Error; err != nil {
		t.Fatalf("seed universe failed: %v", err)
	}
	if err := repo.Upsert(context.Background(), CompanyProfileRecord{Symbol: "000001.SZ", Exchange: "SZSE", Code: "000001", Name: "旧股票", ListingStatus: ListingStatusListed, ProfileStatus: ProfileStatusComplete, QualityFlags: `[]`, CreatedAt: now, UpdatedAt: now}); err != nil {
		t.Fatalf("seed old profile failed: %v", err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/company-profiles/sync" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"items":[{"symbol":"600519.SH","exchange":"SSE","code":"600519","name":"贵州茅台","listing_status":"LISTED","profile_status":"COMPLETE","quality_flags":"[]"}]}`))
	}))
	defer server.Close()
	svc := NewService(repo)
	svc.SetQuantServiceURL(server.URL)
	if _, err := svc.StartManualRefresh(context.Background(), CompanyProfileRefreshRequest{}); err != nil {
		t.Fatalf("StartManualRefresh failed: %v", err)
	}
	for i := 0; i < 50 && svc.RefreshStatus().Running; i++ {
		time.Sleep(10 * time.Millisecond)
	}
	st := svc.RefreshStatus()
	if st.Status != "completed" || st.SuccessCount != 1 || st.DelistedCount != 1 {
		t.Fatalf("unexpected refresh status: %#v", st)
	}
	profile, _ := repo.GetBySymbol(context.Background(), "600519.SH")
	if profile == nil || profile.Name != "贵州茅台" {
		t.Fatalf("missing refreshed profile: %#v", profile)
	}
	old, _ := repo.GetBySymbol(context.Background(), "000001.SZ")
	if old.ListingStatus != ListingStatusDelisted {
		t.Fatalf("old symbol should be delisted, got %s", old.ListingStatus)
	}
}

func TestServiceGetAboutDelisted(t *testing.T) {
	db := testutil.InMemoryDB(t)
	testutil.AutoMigrateModels(t, db, &CompanyProfileRecord{}, &IndustryMappingRecord{})
	repo := NewRepository(db)
	svc := NewService(repo)
	ctx := context.Background()
	now := time.Now().UTC()

	if err := repo.Upsert(ctx, CompanyProfileRecord{
		Symbol:        "000003.SZ",
		Exchange:      "SZSE",
		Code:          "000003",
		Name:          "退市示例",
		ListingStatus: ListingStatusDelisted,
		DelistedDate:  "2024-12-31",
		ProfileStatus: ProfileStatusPartial,
		QualityFlags:  `[]`,
		CreatedAt:     now,
		UpdatedAt:     now,
	}); err != nil {
		t.Fatalf("seed delisted profile failed: %v", err)
	}
	payload, err := svc.GetAbout(ctx, "000003.SZ")
	if err != nil {
		t.Fatalf("GetAbout delisted failed: %v", err)
	}
	if payload.Profile.ListingStatus != ListingStatusDelisted || payload.Profile.DelistedDate != "2024-12-31" {
		t.Fatalf("delisted fields = %#v", payload.Profile)
	}
}
