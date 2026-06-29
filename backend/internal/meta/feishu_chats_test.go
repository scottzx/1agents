package meta

import "testing"

func TestTrackedChatsCRUD(t *testing.T) {
	db := newTestDB(t)
	s := NewFeishuChatStore(db)

	// Track two chats; one with auto-sync, one we'll turn off.
	if err := s.UpsertTrackedChat(TrackedChat{ChatID: "oc_a", ChatName: "投资群", External: false, AutoSync: true}); err != nil {
		t.Fatalf("upsert a: %v", err)
	}
	if err := s.UpsertTrackedChat(TrackedChat{ChatID: "oc_b", ChatName: "外部群", External: true, AutoSync: true}); err != nil {
		t.Fatalf("upsert b: %v", err)
	}

	all, err := s.ListTrackedChats(false)
	if err != nil || len(all) != 2 {
		t.Fatalf("list all: len=%d err=%v", len(all), err)
	}

	// Upsert preserves created_at / last_synced_at while refreshing name.
	a0, _, _ := s.GetTrackedChat("oc_a")
	if err := s.UpdateTrackedSynced("oc_a", "投资群2", 12345); err != nil {
		t.Fatalf("update synced: %v", err)
	}
	if err := s.UpsertTrackedChat(TrackedChat{ChatID: "oc_a", ChatName: "投资群3", AutoSync: true}); err != nil {
		t.Fatalf("re-upsert a: %v", err)
	}
	a1, ok, _ := s.GetTrackedChat("oc_a")
	if !ok {
		t.Fatal("oc_a missing after re-upsert")
	}
	if a1.LastSyncedAt != 12345 {
		t.Fatalf("last_synced_at not preserved: %d", a1.LastSyncedAt)
	}
	if !a1.CreatedAt.Equal(a0.CreatedAt) {
		t.Fatalf("created_at not preserved: %v vs %v", a1.CreatedAt, a0.CreatedAt)
	}
	if a1.ChatName != "投资群3" {
		t.Fatalf("name not refreshed: %q", a1.ChatName)
	}

	// SetAutoSync off → autoOnly list drops it.
	if err := s.SetTrackedAutoSync("oc_b", false); err != nil {
		t.Fatalf("set auto off: %v", err)
	}
	auto, err := s.ListTrackedChats(true)
	if err != nil || len(auto) != 1 || auto[0].ChatID != "oc_a" {
		t.Fatalf("autoOnly: %+v err=%v", auto, err)
	}

	// SetAutoSync on a missing chat → ErrNotFound.
	if err := s.SetTrackedAutoSync("nope", true); err != ErrNotFound {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}

	// Names overlay map only carries non-empty names.
	names, err := s.TrackedNamesBySession()
	if err != nil {
		t.Fatalf("names: %v", err)
	}
	if names["oc_a"] != "投资群3" || names["oc_b"] != "外部群" {
		t.Fatalf("names map wrong: %+v", names)
	}

	// Delete (untrack).
	if err := s.DeleteTrackedChat("oc_a"); err != nil {
		t.Fatalf("delete: %v", err)
	}
	all, _ = s.ListTrackedChats(false)
	if len(all) != 1 {
		t.Fatalf("after delete len=%d", len(all))
	}
}

func TestSyncConfigDefaultAndRoundTrip(t *testing.T) {
	db := newTestDB(t)
	s := NewFeishuChatStore(db)

	// First read inserts + returns the default.
	cfg, err := s.GetSyncConfig()
	if err != nil {
		t.Fatalf("get default: %v", err)
	}
	if !cfg.Enabled || cfg.IntervalMinutes != 180 {
		t.Fatalf("unexpected default: %+v", cfg)
	}

	// Round-trip a new value.
	if err := s.SetSyncConfig(false, 360); err != nil {
		t.Fatalf("set: %v", err)
	}
	cfg, err = s.GetSyncConfig()
	if err != nil {
		t.Fatalf("get after set: %v", err)
	}
	if cfg.Enabled || cfg.IntervalMinutes != 360 {
		t.Fatalf("round-trip wrong: %+v", cfg)
	}

	// Non-positive interval clamps to the default 180.
	if err := s.SetSyncConfig(true, 0); err != nil {
		t.Fatalf("set zero: %v", err)
	}
	cfg, _ = s.GetSyncConfig()
	if cfg.IntervalMinutes != 180 {
		t.Fatalf("zero interval not clamped: %d", cfg.IntervalMinutes)
	}
}
