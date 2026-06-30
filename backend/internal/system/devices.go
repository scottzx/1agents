package system

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"time"
)

// device registry + heartbeat (issue #110, Phase 2 of the multi-device mesh #115).
//
// This is the *local* mirror of the device directory: each 1agents host keeps a
// small registry of the devices it knows about (itself + peers discovered via
// Tailscale or paired through the relay). It records the last heartbeat per
// device so the UI (#113) can show an online/offline dot without a round-trip
// to the Happy server.
//
// Storage matches the file-based pattern established by #109
// (relay_credentials.go): a single JSON file under ~/.1agents/. We deliberately
// avoid a meta.db table here — the system.Handler is stateless and does not
// carry a *meta.DB, and the data is a flat list with no relational queries.

// heartbeatTTL is how long a device may go without a heartbeat before it is
// considered offline. The spec (#110) sends heartbeats every 30s; a 90s TTL
// tolerates two missed beats before flipping to offline.
const heartbeatTTL = 90 * time.Second

// Device is one entry in the local device registry.
type Device struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	OS          string `json:"os,omitempty"`
	Arch        string `json:"arch,omitempty"`
	Address     string `json:"address,omitempty"`     // reachable host:port or IP
	TailscaleIP string `json:"tailscaleIp,omitempty"` // tailnet IP, if known
	Version     string `json:"version,omitempty"`
	Self        bool   `json:"self,omitempty"`     // true for this host's own record
	LastSeen    int64  `json:"lastSeen,omitempty"` // unix millis of last heartbeat/registration
	// Active is derived (not persisted) from LastSeen vs heartbeatTTL at read time.
	Active bool `json:"active"`
}

// deviceStore is the file-backed registry. A process-wide mutex serializes the
// read-modify-write cycle so concurrent heartbeats don't clobber each other.
type deviceStore struct {
	mu   sync.Mutex
	path string
	now  func() time.Time // injectable clock for tests
}

func defaultDeviceStore() *deviceStore {
	return &deviceStore{
		path: filepath.Join(oneAgentsHome(), "devices.json"),
		now:  time.Now,
	}
}

// load reads the registry file. A missing/unreadable/corrupt file is treated as
// an empty registry rather than an error — same forgiving model as #109.
func (s *deviceStore) load() []Device {
	data, err := os.ReadFile(s.path)
	if err != nil {
		return nil
	}
	var devices []Device
	if json.Unmarshal(data, &devices) != nil {
		return nil
	}
	return devices
}

// save persists the registry (0600 — not secret, but keep it user-private).
func (s *deviceStore) save(devices []Device) error {
	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(devices, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(s.path, data, 0o600)
}

// withActive returns a copy of the list with Active derived from LastSeen.
func (s *deviceStore) withActive(devices []Device) []Device {
	cutoff := s.now().Add(-heartbeatTTL).UnixMilli()
	out := make([]Device, len(devices))
	for i, d := range devices {
		d.Active = d.LastSeen >= cutoff
		out[i] = d
	}
	return out
}

// list returns all devices with their derived Active state.
func (s *deviceStore) list() []Device {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.withActive(s.load())
}

// upsert inserts or updates a device by ID, stamping LastSeen with now. It
// returns the resulting device (with Active derived).
func (s *deviceStore) upsert(in Device) (Device, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	devices := s.load()
	in.LastSeen = s.now().UnixMilli()
	found := false
	for i := range devices {
		if devices[i].ID == in.ID {
			// Preserve fields the caller left blank (e.g. a heartbeat that only
			// carries an ID should not wipe the stored name/address).
			in = mergeDevice(devices[i], in)
			devices[i] = in
			found = true
			break
		}
	}
	if !found {
		devices = append(devices, in)
	}
	if err := s.save(devices); err != nil {
		return Device{}, err
	}
	in.Active = true
	return in, nil
}

// heartbeat updates only LastSeen (and Address if supplied) for an existing
// device. Returns false if the device id is unknown.
func (s *deviceStore) heartbeat(id, address string) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	devices := s.load()
	for i := range devices {
		if devices[i].ID == id {
			devices[i].LastSeen = s.now().UnixMilli()
			if address != "" {
				devices[i].Address = address
			}
			return true, s.save(devices)
		}
	}
	return false, nil
}

// remove deletes a device by ID. Returns false if it was not present.
func (s *deviceStore) remove(id string) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	devices := s.load()
	out := devices[:0]
	removed := false
	for _, d := range devices {
		if d.ID == id {
			removed = true
			continue
		}
		out = append(out, d)
	}
	if !removed {
		return false, nil
	}
	return true, s.save(out)
}

// mergeDevice overlays non-empty fields of in onto base, keeping base values for
// fields the caller omitted. LastSeen/Self always come from in.
func mergeDevice(base, in Device) Device {
	if in.Name == "" {
		in.Name = base.Name
	}
	if in.OS == "" {
		in.OS = base.OS
	}
	if in.Arch == "" {
		in.Arch = base.Arch
	}
	if in.Address == "" {
		in.Address = base.Address
	}
	if in.TailscaleIP == "" {
		in.TailscaleIP = base.TailscaleIP
	}
	if in.Version == "" {
		in.Version = base.Version
	}
	return in
}

// selfDevice builds this host's own registry record from runtime + happy info.
func selfDevice() Device {
	return Device{
		ID:          selfDeviceID(),
		Name:        deviceHostname(),
		OS:          runtime.GOOS,
		Arch:        runtime.GOARCH,
		TailscaleIP: tailscaleSelfIP(),
		Version:     getLocalVersion(),
		Self:        true,
	}
}

// selfDeviceID returns a stable identifier for this host. We use the device
// hostname; if empty we fall back to a runtime descriptor so the id is never "".
func selfDeviceID() string {
	if h := deviceHostname(); h != "" {
		return h
	}
	return runtime.GOOS + "-" + runtime.GOARCH
}

// heartbeatInterval is how often the self-heartbeat goroutine refreshes this
// host's registry record. Half of heartbeatTTL so a single skipped tick never
// flips us offline.
const heartbeatInterval = 30 * time.Second

// StartSelfHeartbeat registers this host in the local device registry, refreshes
// its lastSeen every 30s, and deregisters on shutdown (ctx cancel). Run as a
// goroutine from main. This satisfies the #110 "register on startup / heartbeat
// every 30s / deregister on exit" lifecycle for the local registry mirror.
func StartSelfHeartbeat(ctx context.Context) {
	store := defaultDeviceStore()
	self := selfDevice()
	if _, err := store.upsert(self); err != nil {
		log.Printf("[devices] initial self-registration failed: %v", err)
	}

	go func() {
		ticker := time.NewTicker(heartbeatInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				// Graceful deregister: drop our record so peers see us go offline
				// promptly instead of waiting out the TTL.
				if _, err := store.remove(self.ID); err != nil {
					log.Printf("[devices] deregister on shutdown failed: %v", err)
				}
				return
			case <-ticker.C:
				if _, err := store.heartbeat(self.ID, ""); err != nil {
					log.Printf("[devices] self-heartbeat failed: %v", err)
				}
			}
		}
	}()
}

// ── HTTP handlers ──────────────────────────────────────────────────────────

// DevicesHandler handles GET/POST/DELETE /api/devices.
//
//	GET    → list known devices (with derived active state)
//	POST   → register/upsert a device (body: Device JSON)
//	DELETE → remove a device (body: {"id": "..."})
//
// Only reachable from the local 1agents server (same trust model as the rest of
// /api/system and /api/relay).
func (h *Handler) DevicesHandler(w http.ResponseWriter, r *http.Request) {
	store := defaultDeviceStore()
	switch r.Method {
	case http.MethodGet:
		// Ensure our own record exists so a fresh host still lists itself.
		_, _ = store.upsert(selfDevice())
		writeJSON(w, store.list())
	case http.MethodPost:
		var in Device
		if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
			jsonError(w, "invalid JSON body: "+err.Error(), http.StatusBadRequest)
			return
		}
		if in.ID == "" {
			jsonError(w, "id is required", http.StatusBadRequest)
			return
		}
		dev, err := store.upsert(in)
		if err != nil {
			jsonError(w, "save device: "+err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, dev)
	case http.MethodDelete:
		var body struct {
			ID string `json:"id"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.ID == "" {
			jsonError(w, "id is required", http.StatusBadRequest)
			return
		}
		ok, err := store.remove(body.ID)
		if err != nil {
			jsonError(w, "remove device: "+err.Error(), http.StatusInternalServerError)
			return
		}
		if !ok {
			jsonError(w, "device not found", http.StatusNotFound)
			return
		}
		writeJSON(w, map[string]bool{"ok": true})
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

// DeviceHeartbeat handles POST /api/devices/heartbeat (body: {"id","address"}).
// Refreshes the device's LastSeen so it stays active.
func (h *Handler) DeviceHeartbeat(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var body struct {
		ID      string `json:"id"`
		Address string `json:"address"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.ID == "" {
		jsonError(w, "id is required", http.StatusBadRequest)
		return
	}
	store := defaultDeviceStore()
	ok, err := store.heartbeat(body.ID, body.Address)
	if err != nil {
		jsonError(w, "heartbeat: "+err.Error(), http.StatusInternalServerError)
		return
	}
	if !ok {
		jsonError(w, "device not found — register it first", http.StatusNotFound)
		return
	}
	writeJSON(w, map[string]bool{"ok": true})
}

// DevicesRefresh handles POST /api/devices/refresh. It runs Tailscale discovery,
// merges any newly found 1agents nodes into the registry, and returns the full
// device list. This is the manual "scan for devices" trigger from #110.
func (h *Handler) DevicesRefresh(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	store := defaultDeviceStore()
	// Keep our own record fresh on every refresh.
	_, _ = store.upsert(selfDevice())

	found := discoverTailscaleDevices(r.Context())
	for _, d := range found {
		_, _ = store.upsert(d)
	}
	writeJSON(w, store.list())
}

func writeJSON(w http.ResponseWriter, v interface{}) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(v)
}
