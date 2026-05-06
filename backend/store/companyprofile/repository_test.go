package companyprofile

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/woodyyan/pumpkin-pro/backend/store/quadrant"
	"github.com/woodyyan/pumpkin-pro/backend/tests/testutil"
)

func TestRepositoryUpsertAndGetCompanyProfile(t *testing.T) {
	db := testutil.InMemoryDB(t)
	testutil.AutoMigrateModels(t, db, &CompanyProfileRecord{}, &IndustryMappingRecord{})
	repo := NewRepository(db)
	ctx := context.Background()

	now := time.Now().UTC()
	record := CompanyProfileRecord{
		Symbol:                "600519.SH",
		Exchange:              "SSE",
		Code:                  "600519",
		Name:                  "贵州茅台",
		FullName:              "贵州茅台酒股份有限公司",
		BoardCode:             "MAIN",
		BoardName:             "主板",
		RawIndustryName:       "食品饮料Ⅰ",
		IndustryName:          "食品饮料",
		IndustrySource:        "sw",
		Website:               "https://www.moutaichina.com",
		FoundedDate:           "1999-11-20",
		FoundedDatePrecision:  "day",
		IPODate:               "2001-08-27",
		ListingStatus:         ListingStatusListed,
		BusinessSummary:       "贵州茅台主要从事茅台酒及系列酒的生产与销售，所属行业为食品饮料。",
		BusinessSummarySource: SummarySourceExtract,
		ProfileStatus:         ProfileStatusComplete,
		QualityFlags:          `[]`,
		UpdatedAt:             now,
		CreatedAt:             now,
	}
	if err := repo.Upsert(ctx, record); err != nil {
		t.Fatalf("Upsert failed: %v", err)
	}

	got, err := repo.GetBySymbol(ctx, "600519.SH")
	if err != nil {
		t.Fatalf("GetBySymbol failed: %v", err)
	}
	if got == nil {
		t.Fatal("expected record, got nil")
	}
	if got.RawIndustryName != "食品饮料Ⅰ" || got.IndustryName != "食品饮料" {
		t.Fatalf("industry fields = raw %q clean %q", got.RawIndustryName, got.IndustryName)
	}

	record.Website = "https://moutai.example.com"
	record.IndustryName = "白酒"
	if err := repo.Upsert(ctx, record); err != nil {
		t.Fatalf("second Upsert failed: %v", err)
	}
	updated, err := repo.GetBySymbol(ctx, "600519.SH")
	if err != nil {
		t.Fatalf("GetBySymbol after update failed: %v", err)
	}
	if updated.Website != "https://moutai.example.com" || updated.IndustryName != "白酒" {
		t.Fatalf("upsert did not update fields: website=%q industry=%q", updated.Website, updated.IndustryName)
	}
}

func TestRepositoryBulkUpsertAndMissing(t *testing.T) {
	db := testutil.InMemoryDB(t)
	testutil.AutoMigrateModels(t, db, &CompanyProfileRecord{}, &IndustryMappingRecord{})
	repo := NewRepository(db)
	ctx := context.Background()

	if err := repo.BulkUpsert(ctx, []CompanyProfileRecord{
		{Symbol: "00700.HK", Exchange: "HKEX", Code: "00700", Name: "腾讯控股", ListingStatus: ListingStatusListed, ProfileStatus: ProfileStatusPartial},
		{Symbol: "", Name: "bad"},
	}); err != nil {
		t.Fatalf("BulkUpsert failed: %v", err)
	}
	got, err := repo.GetBySymbol(ctx, "00700.HK")
	if err != nil {
		t.Fatalf("GetBySymbol failed: %v", err)
	}
	if got == nil || got.Name != "腾讯控股" {
		t.Fatalf("unexpected record: %#v", got)
	}
	missing, err := repo.GetBySymbol(ctx, "09999.HK")
	if err != nil {
		t.Fatalf("GetBySymbol missing failed: %v", err)
	}
	if missing != nil {
		t.Fatalf("expected nil missing record, got %#v", missing)
	}
}

func TestRepositoryBulkUpsertLargeBatchAvoidsSQLiteVariableLimit(t *testing.T) {
	db := testutil.InMemoryDB(t)
	testutil.AutoMigrateModels(t, db, &CompanyProfileRecord{}, &IndustryMappingRecord{})
	repo := NewRepository(db)
	ctx := context.Background()

	records := make([]CompanyProfileRecord, 0, 1500)
	for i := 0; i < 1500; i++ {
		records = append(records, CompanyProfileRecord{
			Symbol:        buildSymbol("6"+strings.Repeat("0", 5-len(fmt.Sprint(i)))+fmt.Sprint(i), "SSE"),
			Exchange:      "SSE",
			Code:          fmt.Sprintf("6%05d", i),
			Name:          fmt.Sprintf("测试公司%d", i),
			ListingStatus: ListingStatusListed,
			ProfileStatus: ProfileStatusComplete,
			QualityFlags:  `[]`,
		})
	}
	if err := repo.BulkUpsert(ctx, records); err != nil {
		t.Fatalf("BulkUpsert large batch failed: %v", err)
	}
	got, err := repo.GetBySymbol(ctx, "600999.SH")
	if err != nil {
		t.Fatalf("GetBySymbol failed: %v", err)
	}
	if got == nil || got.Name != "测试公司999" {
		t.Fatalf("unexpected profile after large batch: %#v", got)
	}
}

func TestRepositoryMarkSymbolsDelistedLargeBatchAvoidsSQLiteVariableLimit(t *testing.T) {
	db := testutil.InMemoryDB(t)
	testutil.AutoMigrateModels(t, db, &CompanyProfileRecord{}, &IndustryMappingRecord{})
	repo := NewRepository(db)
	ctx := context.Background()

	records := make([]CompanyProfileRecord, 0, 1500)
	symbols := make([]string, 0, 1500)
	for i := 0; i < 1500; i++ {
		code := fmt.Sprintf("6%05d", i)
		symbol := buildSymbol(code, "SSE")
		symbols = append(symbols, symbol)
		records = append(records, CompanyProfileRecord{Symbol: symbol, Exchange: "SSE", Code: code, Name: fmt.Sprintf("测试公司%d", i), ListingStatus: ListingStatusListed, ProfileStatus: ProfileStatusComplete, QualityFlags: `[]`})
	}
	if err := repo.BulkUpsert(ctx, records); err != nil {
		t.Fatalf("seed large profiles failed: %v", err)
	}
	if err := repo.MarkSymbolsDelisted(ctx, symbols); err != nil {
		t.Fatalf("MarkSymbolsDelisted large batch failed: %v", err)
	}
	got, err := repo.GetBySymbol(ctx, "600999.SH")
	if err != nil {
		t.Fatalf("GetBySymbol failed: %v", err)
	}
	if got == nil || got.ListingStatus != ListingStatusDelisted {
		t.Fatalf("expected delisted profile, got %#v", got)
	}
}

func TestRepositoryUpsertIndustryMappingsLargeBatchAvoidsSQLiteVariableLimit(t *testing.T) {
	db := testutil.InMemoryDB(t)
	testutil.AutoMigrateModels(t, db, &CompanyProfileRecord{}, &IndustryMappingRecord{})
	repo := NewRepository(db)
	ctx := context.Background()

	records := make([]IndustryMappingRecord, 0, 1500)
	for i := 0; i < 1500; i++ {
		records = append(records, IndustryMappingRecord{Source: "test", SourceIndustryName: fmt.Sprintf("行业%d", i), StandardIndustryCode: fmt.Sprintf("industry_%d", i), StandardIndustryName: fmt.Sprintf("标准行业%d", i), ExchangeScope: "ALL"})
	}
	if err := repo.UpsertIndustryMappings(ctx, records); err != nil {
		t.Fatalf("UpsertIndustryMappings large batch failed: %v", err)
	}
	got, err := repo.GetIndustryMapping(ctx, "test", "行业999")
	if err != nil {
		t.Fatalf("GetIndustryMapping failed: %v", err)
	}
	if got == nil || got.StandardIndustryCode != "industry_999" {
		t.Fatalf("unexpected mapping after large batch: %#v", got)
	}
}

func TestRepositoryListLatestUniverseUsesLatestPerExchange(t *testing.T) {
	db := testutil.InMemoryDB(t)
	testutil.AutoMigrateModels(t, db, &CompanyProfileRecord{}, &IndustryMappingRecord{}, &quadrant.QuadrantScoreRecord{})
	repo := NewRepository(db)
	ctx := context.Background()
	older := time.Date(2026, 5, 5, 1, 0, 0, 0, time.UTC)
	newer := time.Date(2026, 5, 5, 2, 0, 0, 0, time.UTC)
	if err := db.Create(&[]quadrant.QuadrantScoreRecord{
		{Code: "600519", Name: "贵州茅台", Exchange: "SSE", Board: "MAIN", ComputedAt: older},
		{Code: "000001", Name: "平安银行", Exchange: "SZSE", Board: "MAIN", ComputedAt: older},
		{Code: "00700", Name: "腾讯控股", Exchange: "HKEX", Board: "HK_MAIN", ComputedAt: newer},
	}).Error; err != nil {
		t.Fatalf("seed quadrant scores failed: %v", err)
	}

	items, err := repo.ListLatestUniverse(ctx, "ALL", 0)
	if err != nil {
		t.Fatalf("ListLatestUniverse failed: %v", err)
	}
	if len(items) != 3 {
		t.Fatalf("expected A-share and HK latest universes, got %d: %#v", len(items), items)
	}
	seen := map[string]bool{}
	for _, item := range items {
		seen[item.Symbol] = true
	}
	for _, symbol := range []string{"600519.SH", "000001.SZ", "00700.HK"} {
		if !seen[symbol] {
			t.Fatalf("missing %s in universe: %#v", symbol, items)
		}
	}
}

func TestRepositoryIndustryMapping(t *testing.T) {
	db := testutil.InMemoryDB(t)
	testutil.AutoMigrateModels(t, db, &CompanyProfileRecord{}, &IndustryMappingRecord{})
	repo := NewRepository(db)
	ctx := context.Background()

	records := []IndustryMappingRecord{{
		Source:               "sw",
		SourceIndustryName:   "食品饮料",
		StandardIndustryCode: "consumer_staples_food_beverage",
		StandardIndustryName: "食品饮料",
		StandardLevel:        "l1",
		ExchangeScope:        "ASHARE",
	}}
	if err := repo.UpsertIndustryMappings(ctx, records); err != nil {
		t.Fatalf("UpsertIndustryMappings failed: %v", err)
	}
	got, err := repo.GetIndustryMapping(ctx, "sw", "食品饮料")
	if err != nil {
		t.Fatalf("GetIndustryMapping failed: %v", err)
	}
	if got == nil || got.StandardIndustryCode != "consumer_staples_food_beverage" {
		t.Fatalf("unexpected mapping: %#v", got)
	}

	records[0].StandardIndustryName = "消费品-食品饮料"
	if err := repo.UpsertIndustryMappings(ctx, records); err != nil {
		t.Fatalf("second UpsertIndustryMappings failed: %v", err)
	}
	updated, err := repo.GetIndustryMapping(ctx, "sw", "食品饮料")
	if err != nil {
		t.Fatalf("GetIndustryMapping after update failed: %v", err)
	}
	if updated.StandardIndustryName != "消费品-食品饮料" {
		t.Fatalf("mapping did not update, got %q", updated.StandardIndustryName)
	}
}
