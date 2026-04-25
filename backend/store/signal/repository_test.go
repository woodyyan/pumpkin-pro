package signal

import (
	"context"
	"testing"
	"time"

	"github.com/woodyyan/pumpkin-pro/backend/store/quadrant"
	"github.com/woodyyan/pumpkin-pro/backend/tests/testutil"
)

func setupSignalTest(t *testing.T) (*Repository, func()) {
	t.Helper()
	db := testutil.InMemoryDB(t)
	testutil.AutoMigrateModels(t, db,
		&WebhookEndpointRecord{},
		&SymbolSignalConfigRecord{},
		&SignalEventRecord{},
		&WebhookDeliveryRecord{},
		&quadrant.QuadrantScoreRecord{},
	)
	repo := NewRepository(db)
	return repo, func() {}
}

func makeWebhookRecord(userID string) WebhookEndpointRecord {
	return WebhookEndpointRecord{
		ID:               "wh-" + userID,
		UserID:           userID,
		URL:              "https://hooks.example.com/pumpkin",
		SecretCipherText: "encrypted-secret",
		IsEnabled:        true,
		TimeoutMS:        5000,
		CreatedAt:        time.Now().UTC(),
		UpdatedAt:        time.Now().UTC(),
	}
}

func makeSignalConfigRecord(userID, symbol string) SymbolSignalConfigRecord {
	return SymbolSignalConfigRecord{
		ID:                  "sc-" + userID + "-" + symbol,
		UserID:              userID,
		Symbol:              symbol,
		StrategyID:          "strat-001",
		IsEnabled:           true,
		CooldownSeconds:     3600,
		EvalIntervalSeconds: 3600,
		ThresholdsJSON:      "{}",
		CreatedAt:           time.Now().UTC(),
		UpdatedAt:           time.Now().UTC(),
	}
}

// ── Repository Tests ──

func TestSignalRepo_WebhookEndpointCRUD(t *testing.T) {
	repo, cleanup := setupSignalTest(t)
	defer cleanup()
	ctx := context.Background()

	// Save new endpoint
	record := makeWebhookRecord("user-wh")
	saved, err := repo.SaveWebhookEndpoint(ctx, record)
	if err != nil {
		t.Fatalf("SaveWebhookEndpoint (create) failed: %v", err)
	}
	if saved.ID == "" {
		t.Error("saved webhook ID should not be empty")
	}
	if saved.URL != record.URL {
		t.Errorf("expected URL %s, got %s", record.URL, saved.URL)
	}

	// Get by user
	got, err := repo.GetWebhookEndpoint(ctx, "user-wh")
	if err != nil {
		t.Fatalf("GetWebhookEndpoint failed: %v", err)
	}
	if got.URL != "https://hooks.example.com/pumpkin" {
		t.Errorf("expected stored URL, got %s", got.URL)
	}

	// Update endpoint
	record2 := *saved
	record2.URL = "https://hooks.example.com/pumpkin-v2"
	record2.TimeoutMS = 8000
	record2.UpdatedAt = time.Now().UTC()
	updated, err := repo.SaveWebhookEndpoint(ctx, record2)
	if err != nil {
		t.Fatalf("SaveWebhookEndpoint (update) failed: %v", err)
	}
	if updated.URL != "https://hooks.example.com/pumpkin-v2" {
		t.Errorf("expected updated URL, got %s", updated.URL)
	}
}

func TestSignalRepo_GetWebhookEndpoint_NotFound(t *testing.T) {
	repo, cleanup := setupSignalTest(t)
	defer cleanup()

	got, err := repo.GetWebhookEndpoint(context.Background(), "nonexistent")
	if err != ErrNotFound {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
	if got != nil {
		t.Error("expected nil record for nonexistent user")
	}
}

func TestSignalRepo_SymbolConfigCRUD(t *testing.T) {
	repo, cleanup := setupSignalTest(t)
	defer cleanup()
	ctx := context.Background()

	// Create
	record := makeSignalConfigRecord("user-sc", "00700.HK")
	saved, err := repo.SaveSymbolConfig(ctx, record)
	if err != nil {
		t.Fatalf("SaveSymbolConfig (create) failed: %v", err)
	}
	if saved.Symbol != "00700.HK" {
		t.Errorf("expected symbol 00700.HK, got %s", saved.Symbol)
	}

	// Get
	got, err := repo.GetSymbolConfig(ctx, "user-sc", "00700.HK")
	if err != nil {
		t.Fatalf("GetSymbolConfig failed: %v", err)
	}
	if !got.IsEnabled {
		t.Error("expected IsEnabled = true")
	}

	// Update
	record2 := *got
	record2.StrategyID = "strat-updated"
	record2.CooldownSeconds = 1800
	record2.ThresholdsJSON = `{"score": 0.8}`
	record2.UpdatedAt = time.Now().UTC()
	updated, err := repo.SaveSymbolConfig(ctx, record2)
	if err != nil {
		t.Fatalf("SaveSymbolConfig (update) failed: %v", err)
	}
	if updated.StrategyID != "strat-updated" {
		t.Errorf("expected updated strategy_id, got %s", updated.StrategyID)
	}

	// List
	list, err := repo.ListSymbolConfigs(ctx, "user-sc")
	if err != nil {
		t.Fatalf("ListSymbolConfigs failed: %v", err)
	}
	if len(list) != 1 {
		t.Errorf("expected 1 config in list, got %d", len(list))
	}

	// Delete
	err = repo.DeleteSymbolConfig(ctx, "user-sc", "00700.HK")
	if err != nil {
		t.Fatalf("DeleteSymbolConfig failed: %v", err)
	}
	_, err = repo.GetSymbolConfig(ctx, "user-sc", "00700.HK")
	if err != ErrNotFound {
		t.Errorf("expected ErrNotFound after delete, got %v", err)
	}
}

func TestSignalRepo_DeleteSymbolConfig_NotFound(t *testing.T) {
	repo, cleanup := setupSignalTest(t)
	defer cleanup()

	err := repo.DeleteSymbolConfig(context.Background(), "u", "NO.SUCH")
	if err != ErrNotFound {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

func TestSignalRepo_CountSymbolConfigsByStrategy(t *testing.T) {
	repo, cleanup := setupSignalTest(t)
	defer cleanup()
	ctx := context.Background()

	for _, sym := range []string{"000001.SZ", "600000.SH", "00700.HK"} {
		r := makeSignalConfigRecord("count-user", sym)
		r.StrategyID = "counted-strat"
		_, _ = repo.SaveSymbolConfig(ctx, r)
	}

	count, err := repo.CountSymbolConfigsByStrategy(ctx, "count-user", "counted-strat")
	if err != nil {
		t.Fatalf("CountSymbolConfigsByStrategy failed: %v", err)
	}
	if count != 3 {
		t.Errorf("expected count 3, got %d", count)
	}
}

func TestSignalRepo_SignalEventsAndDeliveries(t *testing.T) {
	repo, cleanup := setupSignalTest(t)
	defer cleanup()
	ctx := context.Background()

	now := time.Now().UTC()
	event := SignalEventRecord{
		ID:          "evt-test-01",
		EventID:     "sig_testevent_001",
		UserID:      "evt-user",
		Symbol:      "600036.SH",
		StrategyID:  "strat-01",
		Side:        "BUY",
		SignalScore: 0.85,
		ReasonJSON:  `{"kind":"macd_cross"}`,
		Fingerprint: "fp123",
		IsTest:      false,
		EventTime:   now,
		CreatedAt:   now,
	}
	delivery := WebhookDeliveryRecord{
		ID:         "del-test-01",
		EventID:    "sig_testevent_001",
		UserID:     "evt-user",
		EndpointID: "wh-evt-user",
		AttemptNo:  1,
		Status:     "pending",
		CreatedAt:  now,
		UpdatedAt:  now,
	}

	// Create event + delivery together
	if err := repo.CreateEventWithDelivery(ctx, event, delivery); err != nil {
		t.Fatalf("CreateEventWithDelivery failed: %v", err)
	}

	// Get event by EventID
	gotEvent, err := repo.GetSignalEventByEventID(ctx, "sig_testevent_001")
	if err != nil {
		t.Fatalf("GetSignalEventByEventID failed: %v", err)
	}
	if gotEvent.Side != "BUY" {
		t.Errorf("expected side BUY, got %s", gotEvent.Side)
	}

	// List events with limit
	events, err := repo.ListSignalEvents(ctx, "evt-user", "", 20)
	if err != nil {
		t.Fatalf("ListSignalEvents failed: %v", err)
	}
	if len(events) != 1 {
		t.Errorf("expected 1 event, got %d", len(events))
	}

	// List deliveries
	deliveries, err := repo.ListDeliveries(ctx, "evt-user", "", 20)
	if err != nil {
		t.Fatalf("ListDeliveries failed: %v", err)
	}
	if len(deliveries) != 1 {
		t.Errorf("expected 1 delivery, got %d", len(deliveries))
	}
}

func TestSignalRepo_DeliveryStateTransitions(t *testing.T) {
	repo, cleanup := setupSignalTest(t)
	defer cleanup()
	ctx := context.Background()
	now := time.Now().UTC()

	// Create a delivery record
	delivery := WebhookDeliveryRecord{
		ID:         "del-state-01",
		EventID:    "sig_state_001",
		UserID:     "state-user",
		EndpointID: "wh-state-user",
		AttemptNo:  1,
		Status:     "pending",
		CreatedAt:  now,
		UpdatedAt:  now,
	}
	_ = repo.CreateEventWithDelivery(ctx, SignalEventRecord{
		ID: "evt-state-01", EventID: "sig_state_001", UserID: "state-user",
		Symbol: "000001.SZ", Side: "BUY", EventTime: now, CreatedAt: now,
	}, delivery)

	// Claim delivery
	ok, err := repo.ClaimDelivery(ctx, "del-state-01", now.Add(time.Second))
	if err != nil {
		t.Fatalf("ClaimDelivery failed: %v", err)
	}
	if !ok {
		t.Error("expected claim to succeed")
	}

	// Mark delivered
	err = repo.MarkDeliveryDelivered(ctx, "del-state-01", 200, 150, now.Add(2*time.Second))
	if err != nil {
		t.Fatalf("MarkDeliveryDelivered failed: %v", err)
	}

	// Verify state
	got, err := repo.GetDeliveryByID(ctx, "del-state-01")
	if err != nil {
		t.Fatalf("GetDeliveryByID failed: %v", err)
	}
	if got.Status != "delivered" {
		t.Errorf("expected status 'delivered', got %s", got.Status)
	}
	if got.HTTPStatus != 200 {
		t.Errorf("expected HTTPStatus 200, got %d", got.HTTPStatus)
	}

	// Double claim should fail
	ok2, err := repo.ClaimDelivery(ctx, "del-state-01", now.Add(3*time.Second))
	if err != nil || ok2 {
		t.Errorf("expected double claim to return (false, nil), got (%v, %v)", ok2, err)
	}
}

func TestSignalRepo_ListAllEnabledConfigs(t *testing.T) {
	repo, cleanup := setupSignalTest(t)
	defer cleanup()
	ctx := context.Background()

	// Insert one enabled, one disabled
	enabled := makeSignalConfigRecord("enabled-user", "A")
	disabled := makeSignalConfigRecord("enabled-user", "B")
	disabled.IsEnabled = false
	_, _ = repo.SaveSymbolConfig(ctx, enabled)
	_, _ = repo.SaveSymbolConfig(ctx, disabled)

	allEnabled, err := repo.ListAllEnabledConfigs(ctx)
	if err != nil {
		t.Fatalf("ListAllEnabledConfigs failed: %v", err)
	}
	if len(allEnabled) != 1 {
		t.Errorf("expected 1 enabled config, got %d", len(allEnabled))
	}
}

func TestSignalRepo_GetLastSignalEventTime(t *testing.T) {
	repo, cleanup := setupSignalTest(t)
	defer cleanup()
	ctx := context.Background()
	now := time.Now().UTC()

	// No events yet → nil
	got, err := repo.GetLastSignalEventTime(ctx, "time-user", "TIME.TEST")
	if err != nil {
		t.Fatalf("GetLastSignalEventTime failed: %v", err)
	}
	if got != nil {
		t.Errorf("expected nil when no events exist, got %v", got)
	}

	// Insert a non-test event
	event := SignalEventRecord{
		ID: "evt-time-01", EventID: "sig_time_001", UserID: "time-user",
		Symbol: "TIME.TEST", Side: "SELL", IsTest: false,
		EventTime: now.Add(-1 * time.Hour), CreatedAt: now,
	}
	delivery := WebhookDeliveryRecord{
		ID: "del-time-01", EventID: "sig_time_001", UserID: "time-user",
		Status: "delivered", CreatedAt: now, UpdatedAt: now,
	}
	_ = repo.CreateEventWithDelivery(ctx, event, delivery)

	got2, err := repo.GetLastSignalEventTime(ctx, "time-user", "TIME.TEST")
	if err != nil {
		t.Fatalf("GetLastSignalEventTime after insert failed: %v", err)
	}
	if got2 == nil {
		t.Fatal("expected non-nil time after inserting an event")
	}
}

func TestSignalRepo_UpdateLastEvaluatedAt(t *testing.T) {
	repo, cleanup := setupSignalTest(t)
	defer cleanup()
	ctx := context.Background()

	config := makeSignalConfigRecord("eval-user", "EVAL.STK")
	saved, _ := repo.SaveSymbolConfig(ctx, config)

	evalAt := time.Now().UTC()
	err := repo.UpdateLastEvaluatedAt(ctx, saved.ID, evalAt)
	if err != nil {
		t.Fatalf("UpdateLastEvaluatedAt failed: %v", err)
	}

	// Verify via Get
	updated, _ := repo.GetSymbolConfig(ctx, "eval-user", "EVAL.STK")
	if updated.LastEvaluatedAt == nil {
		t.Error("expected LastEvaluatedAt to be set")
	}
}

// ── Model conversion tests ──

func TestModelConversions(t *testing.T) {
	t.Run("toSymbolSignalConfig empty thresholds", func(t *testing.T) {
		record := SymbolSignalConfigRecord{ThresholdsJSON: "", Symbol: "TEST"}
		cfg, err := toSymbolSignalConfig(record)
		if err != nil {
			t.Fatalf("toSymbolSignalConfig failed: %v", err)
		}
		if cfg.Symbol != "TEST" {
			t.Errorf("expected symbol TEST, got %s", cfg.Symbol)
		}
	})

	t.Run("toSignalEvent with reason JSON", func(t *testing.T) {
		record := SignalEventRecord{ReasonJSON: `{"signal":"buy","score":0.9}`, Side: "BUY"}
		ev, err := toSignalEvent(record)
		if err != nil {
			t.Fatalf("toSignalEvent failed: %v", err)
		}
		if ev.Reason["signal"] != "buy" {
			t.Errorf("expected signal=buy in reason, got %v", ev.Reason)
		}
	})

	t.Run("encode/decode JSON map roundtrip", func(t *testing.T) {
		original := map[string]any{"a": 1, "b": true, "c": "hello"}
		encoded, err := encodeJSONMap(original)
		if err != nil {
			t.Fatalf("encodeJSONMap failed: %v", err)
		}
		var decoded map[string]any
		err = decodeJSONMap(encoded, &decoded)
		if err != nil {
			t.Fatalf("decodeJSONMap failed: %v", err)
		}
		if decoded["a"] != float64(1) { // json.Unmarshal converts numbers to float64
			t.Errorf("round-trip mismatch for key a: got %T=%v", decoded["a"], decoded["a"])
		}
	})
}

func TestSignalRepo_ListSymbolConfigRefs(t *testing.T) {
	repo, cleanup := setupSignalTest(t)
	defer cleanup()
	ctx := context.Background()

	for _, sym := range []string{"000001.SZ", "600000.SH"} {
		r := makeSignalConfigRecord("refs-user", sym)
		r.StrategyID = "ref-strategy"
		_, _ = repo.SaveSymbolConfig(ctx, r)
	}

	refs, err := repo.ListSymbolConfigRefs(ctx, "refs-user", "ref-strategy")
	if err != nil {
		t.Fatalf("ListSymbolConfigRefs failed: %v", err)
	}
	if len(refs) != 2 {
		t.Fatalf("expected 2 refs, got %d", len(refs))
	}
	got := map[string]bool{}
	for _, r := range refs {
		got[r.Symbol] = true
	}
	if !got["000001.SZ"] || !got["600000.SH"] {
		t.Errorf("expected symbols 000001.SZ and 600000.SH, got %v", got)
	}
}

func TestSignalRepo_ListSymbolConfigRefs_NoRefs(t *testing.T) {
	repo, cleanup := setupSignalTest(t)
	defer cleanup()
	ctx := context.Background()

	refs, err := repo.ListSymbolConfigRefs(ctx, "noref-user", "no-ref-strategy")
	if err != nil {
		t.Fatalf("ListSymbolConfigRefs failed: %v", err)
	}
	if len(refs) != 0 {
		t.Errorf("expected 0 refs, got %d", len(refs))
	}
}
