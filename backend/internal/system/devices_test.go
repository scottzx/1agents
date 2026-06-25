package system

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// newTestStore returns a deviceStore backed by a temp file with a fixed clock.
func newTestStore(t *testing.T, now time.Time) *deviceStore {
	t.Helper()
	return &deviceStore{
		path: filepath.Join(t.TempDir(), "devices.json"),
		now:  func() time.Time { return now },
	}
}

func TestDeviceStoreUpsertAndList(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	s := newTestStore(t, now)

	dev, err := s.upsert(Device{ID: "mac", Name: "MacBook", OS: "darwin"})
	if err != nil {
		t.Fatalf("upsert: %v", err)
	}
	if !dev.Active {
		t.Errorf("freshly upserted device should be active")
	}
	if dev.LastSeen != now.UnixMilli() {
		t.Errorf("LastSeen = %d, want %d", dev.LastSeen, now.UnixMilli())
	}

	list := s.list()
	if len(list) != 1 || list[0].ID != "mac" || list[0].Name != "MacBook" {
		t.Fatalf("unexpected list: %+v", list)
	}
	if !list[0].Active {
		t.Errorf("device within TTL should be active")
	}
}

func TestDeviceStoreUpsertPreservesFields(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	s := newTestStore(t, now)

	if _, err := s.upsert(Device{ID: "mac", Name: "MacBook", OS: "darwin", Address: "100.1.1.1:38080"}); err != nil {
		t.Fatal(err)
	}
	// Second upsert carrying only an ID must not wipe name/os/address.
	dev, err := s.upsert(Device{ID: "mac"})
	if err != nil {
		t.Fatal(err)
	}
	if dev.Name != "MacBook" || dev.OS != "darwin" || dev.Address != "100.1.1.1:38080" {
		t.Errorf("upsert wiped preserved fields: %+v", dev)
	}
}

func TestDeviceStoreActiveExpiry(t *testing.T) {
	base := time.Unix(1_700_000_000, 0)
	s := newTestStore(t, base)
	if _, err := s.upsert(Device{ID: "mac", Name: "MacBook"}); err != nil {
		t.Fatal(err)
	}

	// Advance the clock past the TTL — device should now read offline.
	s.now = func() time.Time { return base.Add(heartbeatTTL + time.Second) }
	list := s.list()
	if len(list) != 1 {
		t.Fatalf("want 1 device, got %d", len(list))
	}
	if list[0].Active {
		t.Errorf("device past TTL should be inactive")
	}
}

func TestDeviceStoreHeartbeatRefreshesActive(t *testing.T) {
	base := time.Unix(1_700_000_000, 0)
	s := newTestStore(t, base)
	if _, err := s.upsert(Device{ID: "mac", Name: "MacBook"}); err != nil {
		t.Fatal(err)
	}

	// Move just before expiry and heartbeat.
	later := base.Add(heartbeatTTL - time.Second)
	s.now = func() time.Time { return later }
	ok, err := s.heartbeat("mac", "100.2.2.2:38080")
	if err != nil || !ok {
		t.Fatalf("heartbeat ok=%v err=%v", ok, err)
	}

	// Now advance just past the original TTL; the heartbeat should keep it alive.
	s.now = func() time.Time { return base.Add(heartbeatTTL + time.Second) }
	list := s.list()
	if !list[0].Active {
		t.Errorf("device should remain active after heartbeat")
	}
	if list[0].Address != "100.2.2.2:38080" {
		t.Errorf("heartbeat did not update address: %+v", list[0])
	}
}

func TestDeviceStoreHeartbeatUnknown(t *testing.T) {
	s := newTestStore(t, time.Now())
	ok, err := s.heartbeat("ghost", "")
	if err != nil {
		t.Fatal(err)
	}
	if ok {
		t.Errorf("heartbeat for unknown device should report not found")
	}
}

func TestDeviceStoreRemove(t *testing.T) {
	s := newTestStore(t, time.Now())
	if _, err := s.upsert(Device{ID: "mac"}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.upsert(Device{ID: "linux"}); err != nil {
		t.Fatal(err)
	}

	ok, err := s.remove("mac")
	if err != nil || !ok {
		t.Fatalf("remove ok=%v err=%v", ok, err)
	}
	list := s.list()
	if len(list) != 1 || list[0].ID != "linux" {
		t.Fatalf("after remove, list = %+v", list)
	}

	ok, _ = s.remove("mac")
	if ok {
		t.Errorf("removing an absent device should report not found")
	}
}

func TestDeviceStoreCorruptFileTreatedEmpty(t *testing.T) {
	s := newTestStore(t, time.Now())
	if err := os.WriteFile(s.path, []byte("{not json"), 0o600); err != nil {
		t.Fatal(err)
	}
	if list := s.list(); len(list) != 0 {
		t.Errorf("corrupt file should yield empty list, got %+v", list)
	}
}

// ── HTTP handler tests ──────────────────────────────────────────────────────

// withTempHome points oneAgentsHome() at a temp dir for the duration of a test
// so the handlers (which build their own defaultDeviceStore) write there.
func withTempHome(t *testing.T) {
	t.Helper()
	t.Setenv("ONEAGENTS_HOME", t.TempDir())
}

func TestDevicesHandlerPostGetDelete(t *testing.T) {
	withTempHome(t)
	h := NewHandler()

	// POST a device.
	body, _ := json.Marshal(Device{ID: "linux", Name: "Server", OS: "linux"})
	req := httptest.NewRequest(http.MethodPost, "/api/devices", bytes.NewReader(body))
	rec := httptest.NewRecorder()
	h.DevicesHandler(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("POST status = %d, body=%s", rec.Code, rec.Body.String())
	}

	// GET should include the posted device plus the self record.
	req = httptest.NewRequest(http.MethodGet, "/api/devices", nil)
	rec = httptest.NewRecorder()
	h.DevicesHandler(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET status = %d", rec.Code)
	}
	var list []Device
	if err := json.Unmarshal(rec.Body.Bytes(), &list); err != nil {
		t.Fatal(err)
	}
	if findDevice(list, "linux") == nil {
		t.Errorf("GET did not return posted device: %+v", list)
	}
	if !anySelf(list) {
		t.Errorf("GET should auto-include the self device: %+v", list)
	}

	// DELETE the posted device.
	del, _ := json.Marshal(map[string]string{"id": "linux"})
	req = httptest.NewRequest(http.MethodDelete, "/api/devices", bytes.NewReader(del))
	rec = httptest.NewRecorder()
	h.DevicesHandler(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("DELETE status = %d, body=%s", rec.Code, rec.Body.String())
	}
}

func TestDevicesHandlerPostRequiresID(t *testing.T) {
	withTempHome(t)
	h := NewHandler()
	body, _ := json.Marshal(Device{Name: "no id"})
	req := httptest.NewRequest(http.MethodPost, "/api/devices", bytes.NewReader(body))
	rec := httptest.NewRecorder()
	h.DevicesHandler(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}

func TestDeviceHeartbeatHandler(t *testing.T) {
	withTempHome(t)
	h := NewHandler()

	// Register first.
	body, _ := json.Marshal(Device{ID: "mac", Name: "MacBook"})
	rec := httptest.NewRecorder()
	h.DevicesHandler(rec, httptest.NewRequest(http.MethodPost, "/api/devices", bytes.NewReader(body)))

	// Heartbeat known device → 200.
	hb, _ := json.Marshal(map[string]string{"id": "mac"})
	rec = httptest.NewRecorder()
	h.DeviceHeartbeat(rec, httptest.NewRequest(http.MethodPost, "/api/devices/heartbeat", bytes.NewReader(hb)))
	if rec.Code != http.StatusOK {
		t.Fatalf("heartbeat status = %d, body=%s", rec.Code, rec.Body.String())
	}

	// Heartbeat unknown device → 404.
	hb, _ = json.Marshal(map[string]string{"id": "ghost"})
	rec = httptest.NewRecorder()
	h.DeviceHeartbeat(rec, httptest.NewRequest(http.MethodPost, "/api/devices/heartbeat", bytes.NewReader(hb)))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("unknown heartbeat status = %d, want 404", rec.Code)
	}
}

func TestDevicesRefreshNoTailscale(t *testing.T) {
	withTempHome(t)
	h := NewHandler()

	// Force discovery to find nothing (simulate no tailscale CLI).
	orig := runTailscaleStatus
	t.Cleanup(func() { runTailscaleStatus = orig })
	runTailscaleStatus = func(context.Context) ([]byte, error) {
		return nil, context.Canceled
	}

	rec := httptest.NewRecorder()
	h.DevicesRefresh(rec, httptest.NewRequest(http.MethodPost, "/api/devices/refresh", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("refresh status = %d, body=%s", rec.Code, rec.Body.String())
	}
	var list []Device
	if err := json.Unmarshal(rec.Body.Bytes(), &list); err != nil {
		t.Fatal(err)
	}
	// Should still contain the self device even with discovery disabled.
	if !anySelf(list) {
		t.Errorf("refresh should keep the self device: %+v", list)
	}
}

func TestStartSelfHeartbeatDeregistersOnCancel(t *testing.T) {
	withTempHome(t)
	store := defaultDeviceStore()

	ctx, cancel := context.WithCancel(context.Background())
	StartSelfHeartbeat(ctx)

	// Self should be registered synchronously before the goroutine starts.
	if !anySelf(store.list()) {
		t.Fatalf("self not registered after StartSelfHeartbeat")
	}

	cancel()
	// Give the goroutine a moment to process the cancel + remove.
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if !anySelf(store.list()) {
			return // deregistered as expected
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Errorf("self device was not deregistered on shutdown")
}

func findDevice(list []Device, id string) *Device {
	for i := range list {
		if list[i].ID == id {
			return &list[i]
		}
	}
	return nil
}

func anySelf(list []Device) bool {
	for _, d := range list {
		if d.Self {
			return true
		}
	}
	return false
}
