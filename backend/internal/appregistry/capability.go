// C0 Capability Registry (design: docs/architecture/enterprise-foundation-v1.0.0.md §6.2/§6.3, D5).
//
// Capabilities are referenced as "id@major" (e.g. "workcase.commands@1").
// C0 matches on major version only — full SemVer range solving, dynamic
// installation, remote discovery and marketplaces are explicitly out of scope.
// Everything here is in-process: providers register via Register (manifest
// declarations) plus RegisterImplementation (typed Go values); consumers
// resolve through Validate / ResolveImplementation.
package appregistry

import (
	"fmt"
	"log"
	"sort"
	"strconv"
	"strings"
	"sync"

	"github.com/scottzx/1Agents/backend/internal/meta"
)

// CapabilityRef is a versioned capability reference, e.g. "workcase.commands@1".
type CapabilityRef struct {
	ID    string // capability id, e.g. "workcase.commands"
	Major int    // major version; C0 compatibility unit
}

// ParseCapabilityRef parses "id@major". The id must be non-empty and the
// major version a non-negative integer.
func ParseCapabilityRef(s string) (CapabilityRef, error) {
	i := strings.LastIndex(s, "@")
	if i <= 0 || i == len(s)-1 {
		return CapabilityRef{}, fmt.Errorf("capability reference %q must be \"id@major\"", s)
	}
	id := s[:i]
	major, err := strconv.Atoi(s[i+1:])
	if err != nil || major < 0 {
		return CapabilityRef{}, fmt.Errorf("capability reference %q: invalid major version", s)
	}
	return CapabilityRef{ID: id, Major: major}, nil
}

func (r CapabilityRef) String() string { return fmt.Sprintf("%s@%d", r.ID, r.Major) }

// Requirement declares a capability an app needs.
//
// Optional requirements do not block enabling the app: when unmet, only the
// contributions (mount points) listed in MountPoints are disabled. Required
// (non-optional) unmet requirements keep the app effectively disabled.
type Requirement struct {
	Capability  string   `json:"capability"`            // capability id without version
	Major       int      `json:"major"`                 // required major version
	Optional    bool     `json:"optional,omitempty"`    // true → degrade instead of blocking
	MountPoints []string `json:"mountPoints,omitempty"` // contribution ids gated by this requirement
}

// Ref returns the versioned reference of this requirement.
func (r Requirement) Ref() CapabilityRef { return CapabilityRef{ID: r.Capability, Major: r.Major} }

// RejectedError is returned by SetEnabled when capability validation rejects
// the state change (unmet requirements, dependency cycle, active dependents).
type RejectedError struct {
	Op      string // "enable" or "disable"
	AppID   string
	Reasons []string
}

func (e *RejectedError) Error() string {
	return fmt.Sprintf("appregistry: cannot %s app %q: %s", e.Op, e.AppID, strings.Join(e.Reasons, "; "))
}

// AppDiagnostic is the effective post-validation state of one app.
type AppDiagnostic struct {
	AppID              string
	Enabled            bool     // effective state after validation
	Reasons            []string // why the app is disabled or degraded
	DroppedMountPoints []string // contributions disabled by unmet optional requirements
}

// registrySnapshot is an immutable copy of the registry state, so validation
// never holds the registry lock while computing.
type registrySnapshot struct {
	order     []string
	manifests map[string]AppManifest
	providers map[string]string // ref.String() → providing app id
}

func takeSnapshot() registrySnapshot {
	mu.RLock()
	defer mu.RUnlock()
	snap := registrySnapshot{
		order:     append([]string(nil), order...),
		manifests: make(map[string]AppManifest, len(registry)),
		providers: make(map[string]string, len(providersByRef)),
	}
	for id, m := range registry {
		cp := *m
		cp.MountPoints = append([]MountPoint(nil), m.MountPoints...)
		for i := range cp.MountPoints {
			cp.MountPoints[i].Shells = append([]string(nil), m.MountPoints[i].Shells...)
		}
		cp.TaskTypes = append([]string(nil), m.TaskTypes...)
		cp.DomainTables = append([]string(nil), m.DomainTables...)
		cp.Provides = append([]string(nil), m.Provides...)
		cp.Requires = append([]Requirement(nil), m.Requires...)
		for i := range cp.Requires {
			cp.Requires[i].MountPoints = append([]string(nil), m.Requires[i].MountPoints...)
		}
		cp.Permissions = append([]string(nil), m.Permissions...)
		snap.manifests[id] = cp
	}
	for k, v := range providersByRef {
		snap.providers[k] = v
	}
	return snap
}

// Validate computes the effective enabled state of every registered app.
//
// states carries the persisted enable/disable intent (app_state table); apps
// absent from it fall back to their manifest default. The result is
// registration-order independent. Rules:
//
//   - an app whose REQUIRED capability is not provided, provided only with an
//     incompatible major version, or provided by a disabled app is
//     effectively disabled, with the reason recorded;
//   - dependency cycles among enabled apps disable every cycle member;
//   - rules re-apply until a fixpoint (disabling a provider may break its
//     consumers);
//   - an unmet OPTIONAL requirement only drops the mount points it gates.
func Validate(states map[string]bool) []AppDiagnostic {
	snap := takeSnapshot()

	eff := make(map[string]bool, len(snap.order))
	for _, id := range snap.order {
		eff[id] = snap.manifests[id].Enabled
		if v, ok := states[id]; ok {
			eff[id] = v
		}
	}
	reasons := map[string][]string{}

	for {
		changed := false

		// Rule 1: required capabilities must resolve to an enabled provider.
		for _, id := range snap.order {
			if !eff[id] {
				continue
			}
			for _, req := range snap.manifests[id].Requires {
				if req.Optional {
					continue
				}
				ref := req.Ref().String()
				prov, registered := snap.providers[ref]
				switch {
				case !registered:
					reasons[id] = append(reasons[id], missingCapabilityReason(snap, req))
					eff[id] = false
					changed = true
				case !eff[prov]:
					reasons[id] = append(reasons[id], fmt.Sprintf(
						"required capability %s is provided by %q, which is disabled", ref, prov))
					eff[id] = false
					changed = true
				}
				if !eff[id] {
					break
				}
			}
		}

		// Rule 2: dependency cycles are rejected.
		if cycle := findCycle(snap, eff); cycle != nil {
			members := cycle[:len(cycle)-1] // last element repeats the first
			for _, id := range members {
				if !eff[id] {
					continue
				}
				eff[id] = false
				changed = true
				reasons[id] = append(reasons[id], fmt.Sprintf(
					"circular dependency: %s", strings.Join(cycle, " -> ")))
			}
		}

		if !changed {
			break
		}
	}

	// Rule 3: unmet optional requirements only disable gated contributions.
	dropped := map[string][]string{}
	for _, id := range snap.order {
		if !eff[id] {
			continue
		}
		for _, req := range snap.manifests[id].Requires {
			if !req.Optional {
				continue
			}
			prov, registered := snap.providers[req.Ref().String()]
			if registered && eff[prov] {
				continue
			}
			dropped[id] = append(dropped[id], req.MountPoints...)
			if len(req.MountPoints) > 0 {
				reasons[id] = append(reasons[id], fmt.Sprintf(
					"optional capability %s unmet; contributions %v disabled",
					req.Ref(), req.MountPoints))
			} else {
				reasons[id] = append(reasons[id], fmt.Sprintf(
					"optional capability %s unmet", req.Ref()))
			}
		}
	}

	out := make([]AppDiagnostic, 0, len(snap.order))
	for _, id := range snap.order {
		out = append(out, AppDiagnostic{
			AppID:              id,
			Enabled:            eff[id],
			Reasons:            reasons[id],
			DroppedMountPoints: dropped[id],
		})
	}
	return out
}

// missingCapabilityReason distinguishes "not provided at all" from "provided
// only with an incompatible major version".
func missingCapabilityReason(snap registrySnapshot, req Requirement) string {
	var alternatives []string
	for refStr, app := range snap.providers {
		ref, err := ParseCapabilityRef(refStr)
		if err != nil || ref.ID != req.Capability {
			continue
		}
		alternatives = append(alternatives, fmt.Sprintf("%s (%s)", refStr, app))
	}
	if len(alternatives) > 0 {
		sort.Strings(alternatives)
		return fmt.Sprintf("required capability %s has no compatible provider (major version mismatch; available: %s)",
			req.Ref(), strings.Join(alternatives, ", "))
	}
	return fmt.Sprintf("required capability %s is not provided by any registered app", req.Ref())
}

// findCycle returns one dependency cycle among effectively-enabled apps as a
// path whose last element repeats the first (e.g. [a b a]), or nil when the
// graph is acyclic. Edges: consumer → provider, for every requirement whose
// provider is registered and enabled (required and optional alike).
func findCycle(snap registrySnapshot, eff map[string]bool) []string {
	const (
		white = 0 // unvisited
		gray  = 1 // on the current DFS stack
		black = 2 // done
	)
	color := make(map[string]int, len(snap.order))
	var stack []string
	var cycle []string

	var dfs func(id string) bool
	dfs = func(id string) bool {
		color[id] = gray
		stack = append(stack, id)
		for _, req := range snap.manifests[id].Requires {
			prov, ok := snap.providers[req.Ref().String()]
			if !ok || !eff[prov] {
				continue
			}
			switch color[prov] {
			case white:
				if dfs(prov) {
					return true
				}
			case gray:
				start := 0
				for i, v := range stack {
					if v == prov {
						start = i
						break
					}
				}
				cycle = append(append([]string{}, stack[start:]...), prov)
				return true
			}
		}
		stack = stack[:len(stack)-1]
		color[id] = black
		return false
	}

	for _, id := range snap.order {
		if eff[id] && color[id] == white && dfs(id) {
			return cycle
		}
	}
	return nil
}

// ── Enable/disable validation (used by SetEnabled) ────────────────────────

// currentPersistedStates loads the persisted enable/disable intent from the
// app_state table; apps absent from it fall back to their manifest default.
func currentPersistedStates() map[string]bool {
	states := map[string]bool{}
	if db, err := meta.OpenDefault(); err == nil {
		states = loadStates(db)
	}
	return states
}

// validateStateChange gates a SetEnabled call:
//
//   - enabling id is rejected (*RejectedError, Op "enable") when id would not
//     be effectively enabled — unmet required capability (missing, major
//     mismatch, disabled provider) or dependency cycle;
//   - disabling id is rejected (*RejectedError, Op "disable") while
//     effectively-enabled apps still require a capability id provides.
func validateStateChange(id string, enabled bool) error {
	states := currentPersistedStates()
	if enabled {
		states[id] = true
		for _, d := range Validate(states) {
			if d.AppID != id || d.Enabled {
				continue
			}
			reasons := d.Reasons
			if len(reasons) == 0 {
				reasons = []string{"capability validation failed"}
			}
			return &RejectedError{Op: "enable", AppID: id, Reasons: reasons}
		}
		return nil
	}

	diags := Validate(states)
	enabledByID := make(map[string]bool, len(diags))
	selfEnabled := false
	for _, d := range diags {
		enabledByID[d.AppID] = d.Enabled
		if d.AppID == id {
			selfEnabled = d.Enabled
		}
	}
	if !selfEnabled {
		return nil // disabling an effectively-disabled app breaks nothing new
	}
	snap := takeSnapshot()
	var reasons []string
	for _, other := range snap.order {
		if other == id || !enabledByID[other] {
			continue
		}
		for _, req := range snap.manifests[other].Requires {
			if req.Optional {
				continue
			}
			if snap.providers[req.Ref().String()] == id {
				reasons = append(reasons, fmt.Sprintf(
					"required capability %s is still used by app %q", req.Ref(), other))
			}
		}
	}
	if len(reasons) > 0 {
		return &RejectedError{Op: "disable", AppID: id, Reasons: reasons}
	}
	return nil
}

// ── In-process implementation registry ────────────────────────────────────

var (
	implMu sync.RWMutex
	impls  = map[string]any{} // ref.String() → implementation
)

// RegisterImplementation binds a typed in-process implementation to a
// capability reference ("id@major"). Call from the provider app's init(),
// after Register. Panics on a malformed reference, nil implementation, or a
// duplicate registration of the same reference.
//
// C0 resolution is exact: consumers must request the same id and major.
func RegisterImplementation(ref string, impl any) {
	r, err := ParseCapabilityRef(ref)
	if err != nil {
		panic(fmt.Sprintf("appregistry: %v", err))
	}
	if impl == nil {
		panic(fmt.Sprintf("appregistry: nil implementation for capability %s", r))
	}
	implMu.Lock()
	defer implMu.Unlock()
	if _, dup := impls[r.String()]; dup {
		panic(fmt.Sprintf("appregistry: duplicate implementation for capability %s", r))
	}
	impls[r.String()] = impl
}

// ResolveImplementation returns the implementation registered under ref
// (exact id + major match), asserted to T. ok=false when the reference is
// malformed, unregistered, or registered under a different type.
func ResolveImplementation[T any](ref string) (T, bool) {
	var zero T
	r, err := ParseCapabilityRef(ref)
	if err != nil {
		return zero, false
	}
	implMu.RLock()
	v, ok := impls[r.String()]
	implMu.RUnlock()
	if !ok {
		return zero, false
	}
	t, ok := v.(T)
	return t, ok
}

// ── Startup diagnostics ───────────────────────────────────────────────────

// StartupDiagnostics validates all registered apps against the persisted
// enable/disable state and logs the outcome. Apps with unmet required
// capabilities or dependency cycles are reported and stay effectively
// disabled until the problem is fixed (the persisted intent is untouched).
// Call once at server startup.
func StartupDiagnostics() {
	diags := Validate(currentPersistedStates())
	enabled, disabled := 0, 0
	for _, d := range diags {
		switch {
		case !d.Enabled:
			disabled++
			log.Printf("[appregistry] app %q disabled by capability check: %s",
				d.AppID, strings.Join(d.Reasons, "; "))
		case len(d.Reasons) > 0:
			log.Printf("[appregistry] app %q degraded: %s", d.AppID, strings.Join(d.Reasons, "; "))
		}
		if d.Enabled {
			enabled++
		}
	}
	log.Printf("[appregistry] startup capability check: %d app(s), %d enabled, %d disabled by validation",
		len(diags), enabled, disabled)
}
