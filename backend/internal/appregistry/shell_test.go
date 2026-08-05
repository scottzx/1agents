package appregistry_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/scottzx/1Agents/backend/internal/appregistry"
)

// The shell registry is process-wide; the three built-in shells are always
// present. Tests use a fresh ONEAGENTS_HOME per test (tempHome from
// capability_test.go) so persisted state never leaks between cases.

func shellByID(t *testing.T, shells []appregistry.ProductShellManifest, id string) appregistry.ProductShellManifest {
	t.Helper()
	for _, s := range shells {
		if s.ID == id {
			return s
		}
	}
	t.Fatalf("shell %q not registered; got %+v", id, shells)
	return appregistry.ProductShellManifest{}
}

// ── Registration ──────────────────────────────────────────────────────────

func TestBuiltinShellsRegistered(t *testing.T) {
	tempHome(t)
	shells := appregistry.ListShells(appregistry.DefaultTenantID)
	for _, id := range []string{appregistry.ShellPersonal, appregistry.ShellPresales, appregistry.ShellCommerce} {
		s := shellByID(t, shells, id)
		if !s.Enabled {
			t.Errorf("shell %q should be enabled by default", id)
		}
		if s.Name == "" {
			t.Errorf("shell %q has no name", id)
		}
	}
	// personal is the fallback default (first registered, enabled).
	if got := appregistry.EffectiveDefaultShell(appregistry.DefaultTenantID, "owner"); got != appregistry.ShellPersonal {
		t.Errorf("EffectiveDefaultShell = %q, want %q", got, appregistry.ShellPersonal)
	}
}

// ── Enable/disable (启停) — flag only, never deletes data ─────────────────

func TestShellEnableDisable(t *testing.T) {
	tempHome(t)
	tenant := appregistry.DefaultTenantID

	if err := appregistry.SetShellEnabled(tenant, "no-such-shell", false); err == nil {
		t.Fatal("disabling an unknown shell should fail")
	}

	// Set a default and a user preference first, then prove disabling the
	// shell flips only the enabled flag: both rows survive and re-enabling
	// restores the exact same product profile.
	if err := appregistry.SetDefaultShell(tenant, appregistry.ShellCommerce); err != nil {
		t.Fatalf("set default: %v", err)
	}
	if err := appregistry.SetUserPreferredShell(tenant, "owner", appregistry.ShellCommerce); err != nil {
		t.Fatalf("set preference: %v", err)
	}
	if err := appregistry.SetShellEnabled(tenant, appregistry.ShellCommerce, false); err != nil {
		t.Fatalf("disable: %v", err)
	}

	s := shellByID(t, appregistry.ListShells(tenant), appregistry.ShellCommerce)
	if s.Enabled {
		t.Fatal("commerce should be disabled")
	}
	// Data preserved: default + preference rows still report the same values.
	if got := appregistry.DefaultShell(tenant); got != appregistry.ShellCommerce {
		t.Errorf("default shell lost after disable: %q", got)
	}
	if got := appregistry.UserPreferredShell(tenant, "owner"); got != appregistry.ShellCommerce {
		t.Errorf("user preference lost after disable: %q", got)
	}
	// Resolution now bypasses the disabled shell.
	if got := appregistry.EffectiveDefaultShell(tenant, "owner"); got == appregistry.ShellCommerce {
		t.Errorf("disabled shell must not resolve as effective default")
	}

	// Re-enable: the stored profile applies again — nothing was deleted.
	if err := appregistry.SetShellEnabled(tenant, appregistry.ShellCommerce, true); err != nil {
		t.Fatalf("re-enable: %v", err)
	}
	if got := appregistry.EffectiveDefaultShell(tenant, "owner"); got != appregistry.ShellCommerce {
		t.Errorf("effective default after re-enable = %q, want commerce (preference survived)", got)
	}
}

// ── Default selection ─────────────────────────────────────────────────────

func TestSetDefaultShell(t *testing.T) {
	tempHome(t)
	tenant := appregistry.DefaultTenantID

	if err := appregistry.SetDefaultShell(tenant, "no-such-shell"); err == nil {
		t.Fatal("unknown shell as default should fail")
	}
	if err := appregistry.SetDefaultShell(tenant, appregistry.ShellPresales); err != nil {
		t.Fatalf("set default: %v", err)
	}
	if got := appregistry.DefaultShell(tenant); got != appregistry.ShellPresales {
		t.Fatalf("DefaultShell = %q", got)
	}
	if got := appregistry.EffectiveDefaultShell(tenant, "owner"); got != appregistry.ShellPresales {
		t.Fatalf("EffectiveDefaultShell = %q, want presales", got)
	}

	// A disabled shell cannot be chosen as the entry point.
	if err := appregistry.SetShellEnabled(tenant, appregistry.ShellCommerce, false); err != nil {
		t.Fatal(err)
	}
	if err := appregistry.SetDefaultShell(tenant, appregistry.ShellCommerce); err == nil {
		t.Fatal("disabled shell as default should fail")
	}
}

// ── User preference overrides tenant default within the allowed range ─────

func TestUserPreferenceOverridesTenantDefault(t *testing.T) {
	tempHome(t)
	tenant := appregistry.DefaultTenantID
	const user = "scott"

	if err := appregistry.SetDefaultShell(tenant, appregistry.ShellPersonal); err != nil {
		t.Fatal(err)
	}
	// No preference yet → tenant default.
	if got := appregistry.EffectiveDefaultShell(tenant, user); got != appregistry.ShellPersonal {
		t.Fatalf("effective = %q, want personal (tenant default)", got)
	}

	// Preference inside the allowed range overrides the tenant default.
	if err := appregistry.SetUserPreferredShell(tenant, user, appregistry.ShellPresales); err != nil {
		t.Fatal(err)
	}
	if got := appregistry.EffectiveDefaultShell(tenant, user); got != appregistry.ShellPresales {
		t.Fatalf("effective = %q, want presales (user preference)", got)
	}
	// Other users are unaffected.
	if got := appregistry.EffectiveDefaultShell(tenant, "someone-else"); got != appregistry.ShellPersonal {
		t.Fatalf("other user effective = %q, want personal", got)
	}

	// Tenant disables the preferred shell → preference is kept but ignored.
	if err := appregistry.SetShellEnabled(tenant, appregistry.ShellPresales, false); err != nil {
		t.Fatal(err)
	}
	if got := appregistry.EffectiveDefaultShell(tenant, user); got != appregistry.ShellPersonal {
		t.Fatalf("effective = %q, want fallback to tenant default while preference disabled", got)
	}
	// Re-enable → the kept preference applies again.
	if err := appregistry.SetShellEnabled(tenant, appregistry.ShellPresales, true); err != nil {
		t.Fatal(err)
	}
	if got := appregistry.EffectiveDefaultShell(tenant, user); got != appregistry.ShellPresales {
		t.Fatalf("effective = %q, want presales after re-enable", got)
	}

	// Unknown shells are rejected as preference.
	if err := appregistry.SetUserPreferredShell(tenant, user, "no-such-shell"); err == nil {
		t.Fatal("unknown shell preference should fail")
	}
	// Clearing the preference falls back to the tenant default.
	if err := appregistry.SetUserPreferredShell(tenant, user, ""); err != nil {
		t.Fatal(err)
	}
	if got := appregistry.EffectiveDefaultShell(tenant, user); got != appregistry.ShellPersonal {
		t.Fatalf("effective = %q, want personal after clearing preference", got)
	}
}

// ── Composition by shell / slot / order ───────────────────────────────────

// Register the composition fixture apps once (process-global registry).
var composeFixture = func() func() {
	registered := false
	return func() {
		if registered {
			return
		}
		registered = true
		appregistry.Register(appregistry.AppManifest{
			ID: "shelltest-compose-a", Name: "Compose A", Version: "0.0.1", Enabled: true,
			MountPoints: []appregistry.MountPoint{
				// Legacy mount: no shells/slot/order → every shell, type slot.
				{Type: "l1-page", ID: "legacy-page", Label: "Legacy", View: "ShellTestLegacy"},
				// Presales-only, explicit slot + order.
				{Type: "l1-page", ID: "presales-late", Label: "Late", View: "ShellTestPresalesLate",
					Shells: []string{appregistry.ShellPresales}, Slot: "nav", Order: 20},
				{Type: "l1-page", ID: "presales-early", Label: "Early", View: "ShellTestPresalesEarly",
					Shells: []string{appregistry.ShellPresales}, Slot: "nav", Order: 10},
			},
		})
		appregistry.Register(appregistry.AppManifest{
			ID: "shelltest-compose-b", Name: "Compose B", Version: "0.0.1", Enabled: true,
			MountPoints: []appregistry.MountPoint{
				// Commerce-only home widget.
				{Type: "lens", ID: "commerce-widget", Label: "Widget", View: "ShellTestCommerceWidget",
					Shells: []string{appregistry.ShellCommerce}, Slot: "home"},
			},
		})
	}
}()

func mountsFor(t *testing.T, tenant, user, shell string) []appregistry.ComposedMount {
	t.Helper()
	mounts, err := appregistry.ComposeShell(tenant, user, shell)
	if err != nil {
		t.Fatalf("ComposeShell(%s): %v", shell, err)
	}
	return mounts
}

func findComposed(mounts []appregistry.ComposedMount, mountID string) *appregistry.ComposedMount {
	for i := range mounts {
		if mounts[i].Mount.ID == mountID {
			return &mounts[i]
		}
	}
	return nil
}

func TestComposeShellTargetingSlotOrder(t *testing.T) {
	tempHome(t)
	composeFixture()
	tenant := appregistry.DefaultTenantID

	// Personal shell: the legacy (untargeted) mount appears; targeted mounts
	// go only to their own shells. Legacy mounts keep their existing zone —
	// slot falls back to the mount type. (Other test files and the built-in
	// roundtable app also contribute untargeted mounts, so assert on our own
	// ids rather than exact counts.)
	personal := mountsFor(t, tenant, "owner", appregistry.ShellPersonal)
	legacy := findComposed(personal, "legacy-page")
	if legacy == nil {
		t.Fatalf("legacy mount missing from personal composition: %+v", personal)
	}
	if legacy.Slot != "l1-page" {
		t.Errorf("legacy slot = %q, want type fallback %q", legacy.Slot, "l1-page")
	}
	if findComposed(personal, "presales-early") != nil || findComposed(personal, "commerce-widget") != nil {
		t.Error("shell-targeted mounts leaked into the personal shell")
	}

	// Presales: legacy mount (all shells) + the two targeted nav entries,
	// sorted by order inside the slot.
	presales := mountsFor(t, tenant, "owner", appregistry.ShellPresales)
	nav := []string{}
	for _, m := range presales {
		if m.Slot == "nav" {
			nav = append(nav, m.Mount.ID)
		}
	}
	if strings.Join(nav, ",") != "presales-early,presales-late" {
		t.Errorf("nav slot order = %v, want [presales-early presales-late]", nav)
	}
	if findComposed(presales, "legacy-page") == nil {
		t.Error("legacy mount missing from presales shell")
	}
	if findComposed(presales, "commerce-widget") != nil {
		t.Error("commerce-only mount leaked into presales shell")
	}

	// Commerce: legacy + its home widget.
	commerce := mountsFor(t, tenant, "owner", appregistry.ShellCommerce)
	widget := findComposed(commerce, "commerce-widget")
	if widget == nil || widget.Slot != "home" {
		t.Fatalf("commerce composition = %+v, want commerce-widget in slot home", commerce)
	}
}

func TestComposeShellDisabledOrUnknown(t *testing.T) {
	tempHome(t)
	tenant := appregistry.DefaultTenantID

	if _, err := appregistry.ComposeShell(tenant, "owner", "no-such-shell"); err == nil {
		t.Fatal("unknown shell should fail composition")
	}
	if err := appregistry.SetShellEnabled(tenant, appregistry.ShellPresales, false); err != nil {
		t.Fatal(err)
	}
	if _, err := appregistry.ComposeShell(tenant, "owner", appregistry.ShellPresales); err == nil {
		t.Fatal("disabled shell should refuse composition")
	}
	// Other shells keep composing while one is disabled.
	if mounts := mountsFor(t, tenant, "owner", appregistry.ShellPersonal); len(mounts) == 0 {
		t.Fatal("personal composition empty while presales is disabled")
	}
}

// ── Permission gating: denied pages never enter navigation ────────────────

var permFixture = func() func() {
	registered := false
	return func() {
		if registered {
			return
		}
		registered = true
		appregistry.Register(appregistry.AppManifest{
			ID: "shelltest-perm", Name: "Perm App", Version: "0.0.1", Enabled: true,
			MountPoints: []appregistry.MountPoint{
				{Type: "l1-page", ID: "public-page", Label: "Public", View: "ShellTestPublic"},
				{Type: "l1-page", ID: "restricted-page", Label: "Restricted", View: "ShellTestRestricted",
					Permission: "shelltest.restricted.view"},
			},
		})
	}
}()

func TestPermissionDeniedMountsLeaveNavigation(t *testing.T) {
	tempHome(t)
	permFixture()
	tenant := appregistry.DefaultTenantID

	// Deny the restricted permission for user "limited".
	appregistry.SetPermissionResolver(func(userID, permission string) bool {
		return !(userID == "limited" && permission == "shelltest.restricted.view")
	})
	defer appregistry.SetPermissionResolver(nil)

	mounts := mountsFor(t, tenant, "limited", appregistry.ShellPersonal)
	if findComposed(mounts, "restricted-page") != nil {
		t.Fatal("permission-denied mount must not be composed")
	}
	if findComposed(mounts, "public-page") == nil {
		t.Fatal("ungated mount must stay composed")
	}
	// A user holding the permission still sees the page.
	if mounts := mountsFor(t, tenant, "admin", appregistry.ShellPersonal); findComposed(mounts, "restricted-page") == nil {
		t.Fatal("permitted user lost the restricted page")
	}
}

func TestHandleListHidesPermissionDeniedMounts(t *testing.T) {
	tempHome(t)
	permFixture()

	appregistry.SetPermissionResolver(func(userID, permission string) bool {
		return !(userID == "limited" && permission == "shelltest.restricted.view")
	})
	defer appregistry.SetPermissionResolver(nil)

	listFor := func(user string) []appregistry.AppManifest {
		req := httptest.NewRequest(http.MethodGet, "/api/apps?user="+user, nil)
		rec := httptest.NewRecorder()
		appregistry.HandleList(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("GET /api/apps (%s): status %d", user, rec.Code)
		}
		var body struct {
			Apps []appregistry.AppManifest `json:"apps"`
		}
		if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		return body.Apps
	}
	hasMount := func(apps []appregistry.AppManifest, mountID string) bool {
		for _, a := range apps {
			if a.ID != "shelltest-perm" {
				continue
			}
			for _, mp := range a.MountPoints {
				if mp.ID == mountID {
					return true
				}
			}
		}
		return false
	}

	if apps := listFor("limited"); hasMount(apps, "restricted-page") {
		t.Fatal("denied mount leaked into /api/apps — it would enter navigation")
	}
	if apps := listFor("admin"); !hasMount(apps, "restricted-page") {
		t.Fatal("permitted user lost the restricted mount from /api/apps")
	}
}

// ── HTTP surface ──────────────────────────────────────────────────────────

func doJSON(t *testing.T, h http.HandlerFunc, method, target string, body any) *httptest.ResponseRecorder {
	t.Helper()
	var reader *bytes.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
		reader = bytes.NewReader(raw)
	} else {
		reader = bytes.NewReader(nil)
	}
	req := httptest.NewRequest(method, target, reader)
	rec := httptest.NewRecorder()
	h(rec, req)
	return rec
}

func TestShellHTTPSurface(t *testing.T) {
	tempHome(t)

	// GET /api/shells lists the three shells and the profile block.
	rec := doJSON(t, appregistry.HandleShellList, http.MethodGet, "/api/shells", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body=%s", rec.Code, rec.Body.String())
	}
	var listed struct {
		Shells           []appregistry.ProductShellManifest `json:"shells"`
		DefaultShell     string                             `json:"defaultShell"`
		UserPreference   string                             `json:"userPreference"`
		EffectiveDefault string                             `json:"effectiveDefault"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&listed); err != nil {
		t.Fatal(err)
	}
	if len(listed.Shells) != 3 {
		t.Fatalf("shells = %+v, want the 3 built-ins", listed.Shells)
	}
	if listed.EffectiveDefault != appregistry.ShellPersonal {
		t.Errorf("effectiveDefault = %q, want personal", listed.EffectiveDefault)
	}

	// Enable/disable round trip via the HTTP handlers.
	if rec := doJSON(t, appregistry.HandleShellDisable, http.MethodPost, "/api/shells/presales/disable", nil); rec.Code != http.StatusOK {
		t.Fatalf("disable: %d %s", rec.Code, rec.Body.String())
	}
	if rec := doJSON(t, appregistry.HandleShellEnable, http.MethodPost, "/api/shells/presales/enable", nil); rec.Code != http.StatusOK {
		t.Fatalf("enable: %d %s", rec.Code, rec.Body.String())
	}
	if rec := doJSON(t, appregistry.HandleShellDisable, http.MethodPost, "/api/shells/nope/disable", nil); rec.Code != http.StatusNotFound {
		t.Fatalf("disable unknown shell: status = %d, want 404", rec.Code)
	}

	// PUT default, then preference overriding it.
	rec = doJSON(t, appregistry.HandleShellDefault, http.MethodPut, "/api/shells/default",
		map[string]string{"shell": appregistry.ShellPresales})
	if rec.Code != http.StatusOK {
		t.Fatalf("put default: %d %s", rec.Code, rec.Body.String())
	}
	rec = doJSON(t, appregistry.HandleShellPreference, http.MethodPut, "/api/shells/preference?user=scott",
		map[string]string{"shell": appregistry.ShellCommerce})
	if rec.Code != http.StatusOK {
		t.Fatalf("put preference: %d %s", rec.Code, rec.Body.String())
	}
	var pref struct {
		EffectiveDefault string `json:"effectiveDefault"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&pref); err != nil {
		t.Fatal(err)
	}
	if pref.EffectiveDefault != appregistry.ShellCommerce {
		t.Fatalf("effectiveDefault after preference = %q, want commerce", pref.EffectiveDefault)
	}
	// Invalid default (unknown shell) → 404.
	if rec := doJSON(t, appregistry.HandleShellDefault, http.MethodPut, "/api/shells/default",
		map[string]string{"shell": "ghost"}); rec.Code != http.StatusNotFound {
		t.Fatalf("put unknown default: status = %d, want 404", rec.Code)
	}

	// GET composition reflects shell targeting.
	rec = doJSON(t, appregistry.HandleShellComposition, http.MethodGet,
		"/api/shells/composition?shell="+appregistry.ShellPersonal, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("composition: %d %s", rec.Code, rec.Body.String())
	}
	var comp struct {
		Shell  string                      `json:"shell"`
		Mounts []appregistry.ComposedMount `json:"mounts"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&comp); err != nil {
		t.Fatal(err)
	}
	for _, m := range comp.Mounts {
		if len(m.Mount.Shells) > 0 && !contains(m.Mount.Shells, appregistry.ShellPersonal) {
			t.Fatalf("composition contains mount %q not targeted at personal", m.Mount.ID)
		}
	}
	// Missing shell param → 400; unknown shell → 404.
	if rec := doJSON(t, appregistry.HandleShellComposition, http.MethodGet, "/api/shells/composition", nil); rec.Code != http.StatusBadRequest {
		t.Fatalf("composition without shell: status = %d, want 400", rec.Code)
	}
	if rec := doJSON(t, appregistry.HandleShellComposition, http.MethodGet, "/api/shells/composition?shell=ghost", nil); rec.Code != http.StatusNotFound {
		t.Fatalf("composition unknown shell: status = %d, want 404", rec.Code)
	}
}

func contains(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}
