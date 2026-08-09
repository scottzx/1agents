package appregistry_test

import (
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/scottzx/1Agents/backend/internal/appregistry"
)

// The appregistry registry is a process-wide global: every Register in this
// file accumulates. All tests therefore use unique app ids and capability
// namespaces, and assert per-app rather than global properties.

func tempHome(t *testing.T) {
	t.Helper()
	tmp, err := os.MkdirTemp("", "capability-test-*")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(tmp) })
	t.Setenv("ONEAGENTS_HOME", tmp)
}

func diagFor(t *testing.T, diags []appregistry.AppDiagnostic, id string) appregistry.AppDiagnostic {
	t.Helper()
	for _, d := range diags {
		if d.AppID == id {
			return d
		}
	}
	t.Fatalf("no diagnostic for app %q", id)
	return appregistry.AppDiagnostic{}
}

func rejectedErr(t *testing.T, err error) *appregistry.RejectedError {
	t.Helper()
	var rej *appregistry.RejectedError
	if !errors.As(err, &rej) {
		t.Fatalf("expected *RejectedError, got %T: %v", err, err)
	}
	return rej
}

func hasReason(d appregistry.AppDiagnostic, substr string) bool {
	for _, r := range d.Reasons {
		if strings.Contains(r, substr) {
			return true
		}
	}
	return false
}

// ── Capability reference parsing ──────────────────────────────────────────

func TestParseCapabilityRef(t *testing.T) {
	ref, err := appregistry.ParseCapabilityRef("workcase.commands@1")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if ref.ID != "workcase.commands" || ref.Major != 1 {
		t.Errorf("got %+v", ref)
	}
	if got := ref.String(); got != "workcase.commands@1" {
		t.Errorf("String() = %q", got)
	}

	for _, bad := range []string{"", "nomajor", "@1", "id@", "id@x", "id@-1"} {
		if _, err := appregistry.ParseCapabilityRef(bad); err == nil {
			t.Errorf("ParseCapabilityRef(%q): expected error", bad)
		}
	}
}

// ── Backward compatibility: manifests without the new fields ──────────────

func TestManifestWithoutCapabilityFieldsStaysEnabled(t *testing.T) {
	tempHome(t)
	appregistry.Register(appregistry.AppManifest{
		ID:      "compat-plain",
		Name:    "Compat Plain",
		Version: "0.1.0",
		Enabled: true,
		MountPoints: []appregistry.MountPoint{
			{Type: "project-tab", ID: "main", Label: "Main", View: "MainTab"},
		},
	})

	d := diagFor(t, appregistry.Validate(nil), "compat-plain")
	if !d.Enabled {
		t.Errorf("app without provides/requires must stay enabled, reasons: %v", d.Reasons)
	}
	if len(d.Reasons) != 0 || len(d.DroppedMountPoints) != 0 {
		t.Errorf("expected clean diagnostic, got %+v", d)
	}

	m, ok := appregistry.Get("compat-plain")
	if !ok || !m.Enabled || len(m.MountPoints) != 1 {
		t.Errorf("Get returned %+v ok=%v; manifest must be unchanged", m, ok)
	}
}

// ── Required capability missing → disabled + enable rejected ──────────────

func TestMissingRequiredCapabilityRejectsEnable(t *testing.T) {
	tempHome(t)
	appregistry.Register(appregistry.AppManifest{
		ID:      "cap-miss-consumer",
		Name:    "Miss Consumer",
		Version: "0.1.0",
		Enabled: true,
		Requires: []appregistry.Requirement{
			{Capability: "cap.miss", Major: 1},
		},
	})

	d := diagFor(t, appregistry.Validate(nil), "cap-miss-consumer")
	if d.Enabled {
		t.Fatal("app with missing required capability must be disabled")
	}
	if !hasReason(d, "cap.miss@1") || !hasReason(d, "not provided by any registered app") {
		t.Errorf("reason must name the capability and that no provider exists: %v", d.Reasons)
	}

	err := appregistry.SetEnabled("cap-miss-consumer", true)
	rej := rejectedErr(t, err)
	if rej.Op != "enable" {
		t.Errorf("Op = %q", rej.Op)
	}
	if !strings.Contains(rej.Error(), "cannot enable app") ||
		!strings.Contains(rej.Error(), "not provided by any registered app") {
		t.Errorf("error message must carry the reason: %v", rej.Error())
	}
}

// ── Major version mismatch → disabled + enable rejected ───────────────────

func TestMajorVersionMismatchRejectsEnable(t *testing.T) {
	tempHome(t)
	appregistry.Register(appregistry.AppManifest{
		ID:       "cap-major-provider",
		Name:     "Major Provider",
		Version:  "0.1.0",
		Enabled:  true,
		Provides: []string{"cap.major@2"},
	})
	appregistry.Register(appregistry.AppManifest{
		ID:      "cap-major-consumer",
		Name:    "Major Consumer",
		Version: "0.1.0",
		Enabled: true,
		Requires: []appregistry.Requirement{
			{Capability: "cap.major", Major: 1},
		},
	})

	prov := diagFor(t, appregistry.Validate(nil), "cap-major-provider")
	cons := diagFor(t, appregistry.Validate(nil), "cap-major-consumer")
	if !prov.Enabled {
		t.Errorf("provider must stay enabled: %v", prov.Reasons)
	}
	if cons.Enabled {
		t.Fatal("consumer requiring incompatible major must be disabled")
	}
	if !hasReason(cons, "major version mismatch") || !hasReason(cons, "cap.major@2") {
		t.Errorf("reason must report the mismatch and the available version: %v", cons.Reasons)
	}

	err := appregistry.SetEnabled("cap-major-consumer", true)
	rej := rejectedErr(t, err)
	if !strings.Contains(rej.Error(), "major version mismatch") {
		t.Errorf("rejection must name the mismatch: %v", rej.Error())
	}
}

// ── Unmet optional requirement degrades, does not block ───────────────────

func TestOptionalRequirementDegradesContributionsOnly(t *testing.T) {
	tempHome(t)
	appregistry.Register(appregistry.AppManifest{
		ID:      "cap-opt-consumer",
		Name:    "Opt Consumer",
		Version: "0.1.0",
		Enabled: true,
		MountPoints: []appregistry.MountPoint{
			{Type: "project-tab", ID: "main", Label: "Main", View: "MainTab"},
			{Type: "project-tab", ID: "extra", Label: "Extra", View: "ExtraTab"},
		},
		Requires: []appregistry.Requirement{
			{Capability: "cap.opt", Major: 1, Optional: true, MountPoints: []string{"extra"}},
		},
	})

	d := diagFor(t, appregistry.Validate(nil), "cap-opt-consumer")
	if !d.Enabled {
		t.Fatalf("unmet optional requirement must not disable the app: %v", d.Reasons)
	}
	if len(d.DroppedMountPoints) != 1 || d.DroppedMountPoints[0] != "extra" {
		t.Errorf("expected dropped=[extra], got %v", d.DroppedMountPoints)
	}
	if !hasReason(d, "optional capability cap.opt@1 unmet") {
		t.Errorf("degradation reason missing: %v", d.Reasons)
	}

	m, _ := appregistry.Get("cap-opt-consumer")
	if len(m.MountPoints) != 1 || m.MountPoints[0].ID != "main" {
		t.Errorf("List/Get must hide the gated contribution, got %+v", m.MountPoints)
	}

	// Once a compatible provider appears, the contribution comes back.
	appregistry.Register(appregistry.AppManifest{
		ID:       "cap-opt-provider",
		Name:     "Opt Provider",
		Version:  "0.1.0",
		Enabled:  true,
		Provides: []string{"cap.opt@1"},
	})
	d = diagFor(t, appregistry.Validate(nil), "cap-opt-consumer")
	if len(d.DroppedMountPoints) != 0 {
		t.Errorf("met optional requirement must drop nothing, got %v", d.DroppedMountPoints)
	}
	m, _ = appregistry.Get("cap-opt-consumer")
	if len(m.MountPoints) != 2 {
		t.Errorf("both contributions must be back, got %+v", m.MountPoints)
	}
}

// ── Duplicate providers are rejected ──────────────────────────────────────

func registerPanics(m appregistry.AppManifest) (panicked bool, msg string) {
	defer func() {
		if r := recover(); r != nil {
			panicked = true
			msg = fmt.Sprint(r)
		}
	}()
	appregistry.Register(m)
	return false, ""
}

func TestDuplicateProviderRejected(t *testing.T) {
	tempHome(t)
	appregistry.Register(appregistry.AppManifest{
		ID: "cap-dup-first", Name: "Dup First", Version: "0.1.0", Enabled: true,
		Provides: []string{"cap.dup@1"},
	})
	panicked, msg := registerPanics(appregistry.AppManifest{
		ID: "cap-dup-second", Name: "Dup Second", Version: "0.1.0", Enabled: true,
		Provides: []string{"cap.dup@1"},
	})
	if !panicked {
		t.Fatal("duplicate provider for the same capability@major must be rejected")
	}
	if !strings.Contains(msg, "duplicate provider") || !strings.Contains(msg, "cap.dup@1") {
		t.Errorf("panic message must name the duplicated capability: %q", msg)
	}

	// Same id, different major is NOT a duplicate.
	appregistry.Register(appregistry.AppManifest{
		ID: "cap-dup-v2", Name: "Dup V2", Version: "0.1.0", Enabled: true,
		Provides: []string{"cap.dup@2"},
	})
}

func TestMalformedManifestRejected(t *testing.T) {
	if panicked, _ := registerPanics(appregistry.AppManifest{
		ID: "cap-bad-provides", Name: "Bad Provides", Version: "0.1.0",
		Provides: []string{"no-version"},
	}); !panicked {
		t.Error("malformed provides reference must be rejected")
	}
	if panicked, _ := registerPanics(appregistry.AppManifest{
		ID: "cap-bad-requires", Name: "Bad Requires", Version: "0.1.0",
		Requires: []appregistry.Requirement{{Capability: "", Major: 1}},
	}); !panicked {
		t.Error("malformed requirement must be rejected")
	}
}

// ── Dependency cycles are rejected ────────────────────────────────────────

func TestDependencyCycleDisablesMembers(t *testing.T) {
	tempHome(t)
	appregistry.Register(appregistry.AppManifest{
		ID: "cyc-a", Name: "Cycle A", Version: "0.1.0", Enabled: true,
		Provides: []string{"cap.cyca@1"},
		Requires: []appregistry.Requirement{{Capability: "cap.cycb", Major: 1}},
	})
	appregistry.Register(appregistry.AppManifest{
		ID: "cyc-b", Name: "Cycle B", Version: "0.1.0", Enabled: true,
		Provides: []string{"cap.cycb@1"},
		Requires: []appregistry.Requirement{{Capability: "cap.cyca", Major: 1}},
	})

	diags := appregistry.Validate(nil)
	a := diagFor(t, diags, "cyc-a")
	b := diagFor(t, diags, "cyc-b")
	if a.Enabled || b.Enabled {
		t.Fatalf("cycle members must be disabled: a=%v b=%v", a.Enabled, b.Enabled)
	}
	if !hasReason(a, "circular dependency") || !hasReason(b, "circular dependency") {
		t.Errorf("cycle reasons missing: a=%v b=%v", a.Reasons, b.Reasons)
	}

	// Enabling into the cycle is rejected with the cycle as reason.
	rej := rejectedErr(t, appregistry.SetEnabled("cyc-a", true))
	if !strings.Contains(rej.Error(), "circular dependency") {
		t.Errorf("enable rejection must name the cycle: %v", rej.Error())
	}
}

// ── Disabled provider breaks consumers; enable gated on provider ──────────

func TestDisableBlockedByActiveDependent(t *testing.T) {
	tempHome(t)
	appregistry.Register(appregistry.AppManifest{
		ID: "dep-provider", Name: "Dep Provider", Version: "0.1.0", Enabled: true,
		Provides: []string{"cap.dep@1"},
	})
	appregistry.Register(appregistry.AppManifest{
		ID: "dep-consumer", Name: "Dep Consumer", Version: "0.1.0", Enabled: true,
		Requires: []appregistry.Requirement{{Capability: "cap.dep", Major: 1}},
	})

	// Disabling the provider is rejected while the consumer is active.
	rej := rejectedErr(t, appregistry.SetEnabled("dep-provider", false))
	if rej.Op != "disable" {
		t.Errorf("Op = %q", rej.Op)
	}
	if !strings.Contains(rej.Error(), "dep-consumer") || !strings.Contains(rej.Error(), "cap.dep@1") {
		t.Errorf("disable rejection must name the dependent and capability: %v", rej.Error())
	}

	// Once the dependent is off, the provider can be disabled.
	if err := appregistry.SetEnabled("dep-consumer", false); err != nil {
		t.Fatalf("disable consumer: %v", err)
	}
	if err := appregistry.SetEnabled("dep-provider", false); err != nil {
		t.Fatalf("disable provider after dependent disabled: %v", err)
	}

	// Re-enabling the consumer is rejected while its provider stays disabled.
	rej = rejectedErr(t, appregistry.SetEnabled("dep-consumer", true))
	if !strings.Contains(rej.Error(), "which is disabled") {
		t.Errorf("enable rejection must name the disabled provider: %v", rej.Error())
	}

	// Enabling provider then consumer succeeds, in that order.
	if err := appregistry.SetEnabled("dep-provider", true); err != nil {
		t.Fatalf("re-enable provider: %v", err)
	}
	if err := appregistry.SetEnabled("dep-consumer", true); err != nil {
		t.Fatalf("re-enable consumer: %v", err)
	}
}

// ── Startup-order independence ────────────────────────────────────────────

// Resolution must not depend on registration order: consumers registered
// before their provider validate identically to the provider-first order.
func TestRegistrationOrderIndependence(t *testing.T) {
	tempHome(t)

	// Order 1: provider first.
	appregistry.Register(appregistry.AppManifest{
		ID: "ord-p1", Name: "Ord P1", Version: "0.1.0", Enabled: true,
		Provides: []string{"cap.ord1@1"},
	})
	appregistry.Register(appregistry.AppManifest{
		ID: "ord-c1", Name: "Ord C1", Version: "0.1.0", Enabled: true,
		Requires: []appregistry.Requirement{{Capability: "cap.ord1", Major: 1}},
	})

	// Order 2: consumer first.
	appregistry.Register(appregistry.AppManifest{
		ID: "ord-c2", Name: "Ord C2", Version: "0.1.0", Enabled: true,
		Requires: []appregistry.Requirement{{Capability: "cap.ord2", Major: 1}},
	})
	appregistry.Register(appregistry.AppManifest{
		ID: "ord-p2", Name: "Ord P2", Version: "0.1.0", Enabled: true,
		Provides: []string{"cap.ord2@1"},
	})

	diags := appregistry.Validate(nil)
	for _, id := range []string{"ord-p1", "ord-c1", "ord-p2", "ord-c2"} {
		if d := diagFor(t, diags, id); !d.Enabled {
			t.Errorf("%s must be enabled regardless of registration order: %v", id, d.Reasons)
		}
	}
}

// A provider→consumer chain must cascade identically no matter the
// registration order.
func TestCascadeIndependentOfRegistrationOrder(t *testing.T) {
	tempHome(t)

	// Chain A registered root-first; chain B registered leaf-first.
	appregistry.Register(appregistry.AppManifest{
		ID: "chain-a-root", Name: "A Root", Version: "0.1.0", Enabled: true,
		Provides: []string{"cap.chaina.root@1"},
	})
	appregistry.Register(appregistry.AppManifest{
		ID: "chain-a-leaf", Name: "A Leaf", Version: "0.1.0", Enabled: true,
		Requires: []appregistry.Requirement{{Capability: "cap.chaina.root", Major: 1}},
	})

	appregistry.Register(appregistry.AppManifest{
		ID: "chain-b-leaf", Name: "B Leaf", Version: "0.1.0", Enabled: true,
		Requires: []appregistry.Requirement{{Capability: "cap.chainb.root", Major: 1}},
	})
	appregistry.Register(appregistry.AppManifest{
		ID: "chain-b-root", Name: "B Root", Version: "0.1.0", Enabled: true,
		Provides: []string{"cap.chainb.root@1"},
	})

	// Disable both roots; both leaves must cascade off.
	for _, root := range []string{"chain-a-root", "chain-b-root"} {
		// Roots still have active leaves → disable is rejected first.
		rejectedErr(t, appregistry.SetEnabled(root, false))
	}
	// Persist root-disabled state directly via Validate's states input and
	// check the cascade outcome is identical for both chains.
	states := map[string]bool{"chain-a-root": false, "chain-b-root": false}
	diags := appregistry.Validate(states)
	for _, leaf := range []string{"chain-a-leaf", "chain-b-leaf"} {
		d := diagFor(t, diags, leaf)
		if d.Enabled {
			t.Errorf("%s must cascade-disable when its root is off", leaf)
		}
		if !hasReason(d, "which is disabled") {
			t.Errorf("%s cascade reason missing: %v", leaf, d.Reasons)
		}
	}
}

// ── In-process implementation registry (provider/consumer resolution) ─────

type greeter interface{ Greet() string }

type helloGreeter struct{}

func (helloGreeter) Greet() string { return "hello" }

func TestImplementationRegistrationAndResolution(t *testing.T) {
	appregistry.RegisterImplementation("cap.greeter@1", helloGreeter{})

	g, ok := appregistry.ResolveImplementation[greeter]("cap.greeter@1")
	if !ok || g.Greet() != "hello" {
		t.Fatalf("resolution failed: ok=%v", ok)
	}

	// Wrong major, unknown ref, malformed ref: no resolution.
	if _, ok := appregistry.ResolveImplementation[greeter]("cap.greeter@2"); ok {
		t.Error("different major must not resolve")
	}
	if _, ok := appregistry.ResolveImplementation[greeter]("cap.unknown@1"); ok {
		t.Error("unknown capability must not resolve")
	}
	if _, ok := appregistry.ResolveImplementation[greeter]("malformed"); ok {
		t.Error("malformed reference must not resolve")
	}
	// Wrong type assertion: no resolution.
	if _, ok := appregistry.ResolveImplementation[os.File]("cap.greeter@1"); ok {
		t.Error("wrong implementation type must not resolve")
	}
}

func TestDuplicateImplementationRejected(t *testing.T) {
	appregistry.RegisterImplementation("cap.dupimpl@1", helloGreeter{})
	panicked := false
	func() {
		defer func() {
			if r := recover(); r != nil {
				panicked = true
				if msg, _ := r.(string); !strings.Contains(msg, "duplicate implementation") {
					t.Errorf("panic message: %q", msg)
				}
			}
		}()
		appregistry.RegisterImplementation("cap.dupimpl@1", helloGreeter{})
	}()
	if !panicked {
		t.Error("duplicate implementation for the same capability must be rejected")
	}
}

// ── Startup diagnostics ───────────────────────────────────────────────────

func TestStartupDiagnosticsRunsAndReports(t *testing.T) {
	tempHome(t)
	// Must not panic over the accumulated global registry (which includes
	// apps with unmet requirements and a cycle from earlier tests).
	appregistry.StartupDiagnostics()
}

// ── HTTP surface: rejections surface as 409 with reasons ──────────────────

func TestEnableHandlerRejectedWithConflict(t *testing.T) {
	tempHome(t)
	appregistry.Register(appregistry.AppManifest{
		ID: "http-miss-consumer", Name: "HTTP Miss", Version: "0.1.0", Enabled: false,
		Requires: []appregistry.Requirement{{Capability: "cap.httpmiss", Major: 1}},
	})

	req := httptest.NewRequest(http.MethodPost, "/api/apps/http-miss-consumer/enable", nil)
	rec := httptest.NewRecorder()
	appregistry.HandleEnable(rec, req)

	if rec.Code != http.StatusConflict {
		t.Fatalf("status = %d, want 409; body: %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "not provided by any registered app") {
		t.Errorf("response body must carry the rejection reason: %s", rec.Body.String())
	}
}
