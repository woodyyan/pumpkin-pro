package strategy

import (
	"context"
	"strings"
	"testing"

	"github.com/woodyyan/pumpkin-pro/backend/tests/testutil"
)

func setupStrategySvcTest(t *testing.T) (*Repository, func()) {
	t.Helper()
	db := testutil.InMemoryDB(t)
	testutil.AutoMigrateModels(t, db, &StrategyRecord{})
	repo := NewRepository(db)
	return repo, func() {}
}

func TestStrategyService_ImplementationKeys(t *testing.T) {
	s := NewService(nil)
	keys := s.ImplementationKeys()
	if len(keys) == 0 {
		t.Error("expected at least one implementation key")
	}
	for _, k := range keys {
		if strings.TrimSpace(k) == "" {
			t.Errorf("got empty implementation key in list: %q", k)
		}
	}
}

func TestStrategyService_GetByID_EmptyID(t *testing.T) {
	repo, cleanup := setupStrategySvcTest(t)
	defer cleanup()

	s := NewService(repo)
	_, err := s.GetByID(context.Background(), "u1", "")
	if err != ErrInvalid {
		t.Errorf("expected ErrInvalid for empty ID, got %v", err)
	}
}

func TestStrategyService_Create_EmptyUserID(t *testing.T) {
	repo, cleanup := setupStrategySvcTest(t)
	defer cleanup()

	s := NewService(repo)
	_, err := s.Create(context.Background(), "", StrategyPayload{
		ID: "x", Key: "k", Name: "N",
		Description: "D", Category: "C", ImplementationKey: "trend_cross",
		Status: "draft", Version: 1, ParamSchema: []ParamSchemaItem{},
	})
	if err != ErrForbidden {
		t.Errorf("expected ErrForbidden for empty user, got %v", err)
	}
}

func TestStrategyService_Create_NormalizePayload(t *testing.T) {
	repo, cleanup := setupStrategySvcTest(t)
	defer cleanup()
	ctx := context.Background()

	s := NewService(repo)
	payload := StrategyPayload{
		ID:   t.Name() + "-norm-01",
		Key:  "  norm-key  ",
		Name: "  Normalized Strategy  ",
		Description: "  test desc  ",
		Category:          "",
		ImplementationKey: "  rsi_range  ",
		Status:            "",
		Version:           0,
		ParamSchema: []ParamSchemaItem{
			{Key: "period", Label: "Period", Type: "integer", Required: true, Default: float64(14)},
			{Key: "threshold", Label: "Threshold", Type: "number", Min: ptrFloat(0), Max: ptrFloat(100)},
		},
		DefaultParams: map[string]any{"period": 14},
	}

	strategy, err := s.Create(ctx, "user-norm", payload)
	if err != nil {
		t.Fatalf("Create with normalize failed: %v", err)
	}

	if strategy.Key != "norm-key" {
		t.Errorf("expected trimmed key 'norm-key', got %q", strategy.Key)
	}
	if strategy.Name != "Normalized Strategy" {
		t.Errorf("expected trimmed name, got %q", strategy.Name)
	}
	if strategy.Category != "通用" {
		t.Errorf("expected default category '通用', got %q", strategy.Category)
	}
	if strategy.Status != "draft" {
		t.Errorf("expected default status 'draft', got %q", strategy.Status)
	}
	if strategy.Version < 1 {
		t.Errorf("expected Version >= 1, got %d", strategy.Version)
	}
}

func TestStrategyService_Create_InvalidStatus(t *testing.T) {
	repo, cleanup := setupStrategySvcTest(t)
	defer cleanup()
	s := NewService(repo)

	payload := StrategyPayload{
		ID: "inv-stat-01", Key: "k", Name: "N",
		Description: "D", Category: "C", ImplementationKey: "trend_cross",
		Status: "bogus_status", Version: 1, ParamSchema: []ParamSchemaItem{},
	}
	_, err := s.Create(context.Background(), "user-x", payload)
	if err == nil || !strings.Contains(err.Error(), "不支持") {
		t.Errorf("expected invalid status error, got %v", err)
	}
}

func TestStrategyService_Create_InvalidImplKey(t *testing.T) {
	repo, cleanup := setupStrategySvcTest(t)
	defer cleanup()
	s := NewService(repo)

	payload := StrategyPayload{
		ID: "inv-key-01", Key: "k", Name: "N",
		Description: "D", Category: "C",
		ImplementationKey: "nonexistent_key",
		Status: "active", Version: 1, ParamSchema: []ParamSchemaItem{},
	}
	_, err := s.Create(context.Background(), "user-x", payload)
	if err == nil || !strings.Contains(err.Error(), "未注册") {
		t.Errorf("expected unregistered impl key error, got %v", err)
	}
}

func TestStrategyService_Delete_EmptyUserID(t *testing.T) {
	repo, cleanup := setupStrategySvcTest(t)
	defer cleanup()
	s := NewService(repo)

	err := s.Delete(context.Background(), "", "some-id")
	if err != ErrForbidden {
		t.Errorf("expected ErrForbidden for empty userID on delete, got %v", err)
	}
}

func TestStrategyService_Delete_EmptyID(t *testing.T) {
	repo, cleanup := setupStrategySvcTest(t)
	defer cleanup()
	s := NewService(repo)

	err := s.Delete(context.Background(), "u1", "")
	if err != ErrInvalid {
		t.Errorf("expected ErrInvalid for empty ID on delete, got %v", err)
	}
}

func TestStrategyService_ListSummaries(t *testing.T) {
	repo, cleanup := setupStrategySvcTest(t)
	defer cleanup()
	ctx := context.Background()
	s := NewService(repo)

	longDesc := strings.Repeat("这是一段非常长的策略描述文字，用于测试摘要截断功能是否正常工作。", 10)
	payload := StrategyPayload{
		ID: "summ-01", Key: "k1", Name: "SummaryStrat",
		Description: longDesc,
		Category: "趋势", ImplementationKey: "macd_cross",
		Status: "active", Version: 1, ParamSchema: []ParamSchemaItem{},
	}
	_, _ = s.Create(ctx, "sum-user", payload)

	summaries, err := s.ListSummaries(ctx, "sum-user")
	if err != nil {
		t.Fatalf("ListSummaries failed: %v", err)
	}
	if len(summaries) != 1 {
		t.Fatalf("expected 1 summary, got %d", len(summaries))
	}
	if summaries[0].Name != "SummaryStrat" {
		t.Errorf("expected name SummaryStrat, got %s", summaries[0].Name)
	}
	if len([]rune(summaries[0].DescriptionSummary)) > 72 {
		t.Errorf("description_summary should be <= 72 runes, got %d", len([]rune(summaries[0].DescriptionSummary)))
	}
}

// ── Pure function tests (no DB needed) ──

func TestNormalizeStrategyName(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"MACD Cross", "macdcross"},
		{"  RSI Range  ", "rsirange"},
		{"布林带（上轨）", "布林带(上轨)"},
		{"Bollinger（下轨）Reversion", "bollinger(下轨)reversion"},
	}
	for _, tc := range tests {
		got := normalizeStrategyName(tc.input)
		if got != tc.expected {
			t.Errorf("normalizeStrategyName(%q) = %q, want %q", tc.input, got, tc.expected)
		}
	}
}

func TestToFloat(t *testing.T) {
	tests := []struct {
		input    any
		expected float64
		wantErr  bool
	}{
		{float64(3.14), 3.14, false},
		{int(42), 42.0, false},
		{int64(100), 100.0, false},
		{"3.5", 3.5, false},
		{"not-a-number", 0, true},
		{nil, 0, true},
	}
	for _, tc := range tests {
		got, err := toFloat(tc.input)
		if tc.wantErr && err == nil {
			t.Errorf("toFloat(%v): expected error, got %v", tc.input, got)
		}
		if !tc.wantErr && err != nil {
			t.Errorf("toFloat(%v): unexpected error %v", tc.input, err)
		}
		if !tc.wantErr && got != tc.expected {
			t.Errorf("toFloat(%v) = %f, want %f", tc.input, got, tc.expected)
		}
	}
}

func TestToInt(t *testing.T) {
	tests := []struct {
		input   any
		want    int
		wantErr bool
	}{
		{int(7), 7, false},
		{float64(5.0), 5, false},
		{float64(5.5), 0, true}, // not integer
		{"3", 3, false},
	}
	for _, tc := range tests {
		got, err := toInt(tc.input)
		if tc.wantErr && err == nil {
			t.Errorf("toInt(%v): expected error", tc.input)
		}
		if !tc.wantErr && err != nil {
			t.Errorf("toInt(%v): unexpected error %v", tc.input, err)
		}
		if !tc.wantErr && got != tc.want {
			t.Errorf("toInt(%v) = %d, want %d", tc.input, got, tc.want)
		}
	}
}

func TestToBool(t *testing.T) {
	tests := []struct {
		input   any
		want    bool
		wantErr bool
	}{
		{true, true, false},
		{false, false, false},
		{"true", true, false},
		{"TRUE", true, false},
		{"yes", true, false},
		{"1", true, false},
		{"false", false, false},
		{"0", false, false},
		{"no", false, false},
		{123, false, true},
	}
	for _, tc := range tests {
		got, err := toBool(tc.input)
		if tc.wantErr && err == nil {
			t.Errorf("toBool(%v): expected error", tc.input)
		}
		if !tc.wantErr && err != nil {
			t.Errorf("toBool(%v): unexpected error %v", tc.input, err)
		}
		if !tc.wantErr && got != tc.want {
			t.Errorf("toBool(%v) = %v, want %v", tc.input, got, tc.want)
		}
	}
}

func TestEnsureHelpers(t *testing.T) {
	// ensureMap
	m := ensureMap(nil)
	if m == nil || len(m) != 0 {
		t.Errorf("ensureMap(nil) should return non-nil empty map")
	}
	m2 := ensureMap(map[string]any{"a": 1})
	if len(m2) != 1 {
		t.Errorf("ensureMap should preserve existing data")
	}

	// ensureSliceMap
	s := ensureSliceMap(nil)
	if s == nil || len(s) != 0 {
		t.Errorf("ensureSliceMap(nil) should return non-nil empty slice")
	}
	s2 := ensureSliceMap([]map[string]any{{"b": 2}})
	if len(s2) != 1 {
		t.Errorf("ensureSliceMap should preserve existing data")
	}

	// copyMap
	copied := copyMap(map[string]any{"x": 1, "y": 2})
	if len(copied) != 2 {
		t.Errorf("copyMap should preserve size")
	}
	copied["z"] = 3 // mutation of copy shouldn't affect original

	emptyCopy := copyMap(nil)
	if emptyCopy == nil || len(emptyCopy) != 0 {
		t.Errorf("copyMap(nil) should return non-nil empty map")
	}
}

func TestMarshalJSON_UnmarshalJSON_RoundTrip(t *testing.T) {
	original := map[string]any{"key": "value", "num": 42, "nested": map[string]any{"inner": true}}

	encoded, err := marshalJSON(original, map[string]any{})
	if err != nil {
		t.Fatalf("marshalJSON failed: %v", err)
	}
	if encoded == "" {
		t.Fatal("marshalJSON returned empty string")
	}

	var decoded map[string]any
	err = unmarshalJSON(encoded, &decoded)
	if err != nil {
		t.Fatalf("unmarshalJSON failed: %v", err)
	}
	if decoded["key"] != "value" {
		t.Errorf("round-trip key mismatch: got %v", decoded["key"])
	}
}

func TestUnmarshalJSON_EmptyString(t *testing.T) {
	var target map[string]any
	err := unmarshalJSON("", &target)
	if err != nil {
		t.Fatalf("unmarshalJSON('') should succeed: %v", err)
	}
	if target != nil {
		t.Errorf("unmarshalJSON('') should produce nil, got %v", target)
	}
}

// ── Helpers ──

func ptrFloat(v float64) *float64 { return &v }
