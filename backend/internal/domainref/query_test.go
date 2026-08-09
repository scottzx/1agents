package domainref_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/scottzx/1Agents/backend/internal/domainref"
)

// fakeProvider is a QueryProvider with per-object ACLs used to exercise the
// registry's dispatch, version and object-level permission paths.
type fakeProvider struct {
	ns       string
	versions []int
	// objects maps object id → summary title; missing id = not found.
	objects map[string]string
	// acl maps object id → allowed actors; missing entry = public.
	acl map[string][]string
}

func (f *fakeProvider) Namespace() string { return f.ns }
func (f *fakeProvider) Versions() []int   { return f.versions }

func (f *fakeProvider) Query(_ context.Context, req domainref.QueryRequest) (domainref.ObjectSummary, error) {
	title, ok := f.objects[req.Ref.ID]
	if !ok {
		return domainref.ObjectSummary{}, domainref.NewError(domainref.CodeNotFound,
			"%s %q does not exist", req.Ref.Type, req.Ref.ID)
	}
	if allowed, hasACL := f.acl[req.Ref.ID]; hasACL {
		ok := false
		for _, a := range allowed {
			if a == req.Actor {
				ok = true
				break
			}
		}
		if !ok {
			return domainref.ObjectSummary{}, domainref.NewError(domainref.CodePermissionDenied,
				"actor %q may not read %s %q", req.Actor, req.Ref.Type, req.Ref.ID)
		}
	}
	return domainref.ObjectSummary{
		Ref:    req.Ref,
		Title:  title,
		Status: "open",
		Fields: map[string]any{"id": req.Ref.ID},
	}, nil
}

func newCRMProvider() *fakeProvider {
	return &fakeProvider{
		ns:       "crm",
		versions: []int{1},
		objects:  map[string]string{"42": "线索 42", "secret": "受限对象"},
		acl:      map[string][]string{"secret": {"owner"}},
	}
}

// ── happy path: namespace dispatch + summary resolution ────────────────────

func TestRegistryResolveHappyPath(t *testing.T) {
	reg := domainref.NewRegistry()
	if err := reg.Register(newCRMProvider()); err != nil {
		t.Fatalf("Register: %v", err)
	}

	ref, _ := domainref.NewDomainRef("crm", "lead", "42", 0)
	sum, err := reg.Resolve(context.Background(), domainref.QueryRequest{
		Ref: ref, Actor: "agent:claudecode", WorkspaceID: "1agentsapp", CorrelationID: "corr-1",
	})
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if sum.Title != "线索 42" || sum.Status != "open" || sum.Ref != ref {
		t.Errorf("unexpected summary: %+v", sum)
	}
}

func TestRegistryResolveSupportedVersion(t *testing.T) {
	reg := domainref.NewRegistry()
	_ = reg.Register(newCRMProvider()) // supports version 1

	ref, _ := domainref.NewDomainRef("crm", "lead", "42", 1)
	if _, err := reg.Resolve(context.Background(), domainref.QueryRequest{Ref: ref, Actor: "x"}); err != nil {
		t.Fatalf("Resolve v1: %v", err)
	}
}

// ── structured rejection paths ─────────────────────────────────────────────

func TestRegistryUnknownProvider(t *testing.T) {
	reg := domainref.NewRegistry()
	_ = reg.Register(newCRMProvider())

	ref, _ := domainref.NewDomainRef("nosuch", "thing", "1", 0)
	_, err := reg.Resolve(context.Background(), domainref.QueryRequest{Ref: ref, Actor: "x"})
	if err == nil {
		t.Fatal("expected unknown provider error")
	}
	if !domainref.IsCode(err, domainref.CodeUnknownProvider) {
		t.Errorf("code = %v, want unknown_provider", codeOf(t, err))
	}
}

func TestRegistryVersionMismatch(t *testing.T) {
	reg := domainref.NewRegistry()
	_ = reg.Register(newCRMProvider()) // supports only version 1

	ref, _ := domainref.NewDomainRef("crm", "lead", "42", 2)
	_, err := reg.Resolve(context.Background(), domainref.QueryRequest{Ref: ref, Actor: "x"})
	if err == nil {
		t.Fatal("expected version mismatch error")
	}
	if !domainref.IsCode(err, domainref.CodeVersionMismatch) {
		t.Errorf("code = %v, want version_unsupported", codeOf(t, err))
	}
}

func TestRegistryPermissionDenied(t *testing.T) {
	reg := domainref.NewRegistry()
	_ = reg.Register(newCRMProvider()) // "secret" readable only by "owner"

	ref, _ := domainref.NewDomainRef("crm", "lead", "secret", 0)
	_, err := reg.Resolve(context.Background(), domainref.QueryRequest{Ref: ref, Actor: "intruder"})
	if err == nil {
		t.Fatal("expected permission denied")
	}
	if !domainref.IsCode(err, domainref.CodePermissionDenied) {
		t.Errorf("code = %v, want permission_denied", codeOf(t, err))
	}

	// The allowed actor passes the object-level check.
	if _, err := reg.Resolve(context.Background(), domainref.QueryRequest{Ref: ref, Actor: "owner"}); err != nil {
		t.Errorf("owner should pass: %v", err)
	}
}

func TestRegistryNotFound(t *testing.T) {
	reg := domainref.NewRegistry()
	_ = reg.Register(newCRMProvider())

	ref, _ := domainref.NewDomainRef("crm", "lead", "missing", 0)
	_, err := reg.Resolve(context.Background(), domainref.QueryRequest{Ref: ref, Actor: "x"})
	if !domainref.IsCode(err, domainref.CodeNotFound) {
		t.Errorf("code = %v, want not_found", codeOf(t, err))
	}
}

func TestRegistryRejectsInvalidRef(t *testing.T) {
	reg := domainref.NewRegistry()
	_ = reg.Register(newCRMProvider())

	// An invalid ref (empty namespace) never reaches any provider.
	_, err := reg.Resolve(context.Background(), domainref.QueryRequest{
		Ref: domainref.DomainRef{Type: "lead", ID: "42"}, Actor: "x",
	})
	if !domainref.IsCode(err, domainref.CodeInvalidRef) {
		t.Errorf("code = %v, want invalid_ref", codeOf(t, err))
	}
}

// ── registration validation ────────────────────────────────────────────────

func TestRegistryRegisterValidation(t *testing.T) {
	reg := domainref.NewRegistry()

	if err := reg.Register(&fakeProvider{ns: ""}); !domainref.IsCode(err, domainref.CodeInvalidRef) {
		t.Errorf("empty namespace: %v", err)
	}
	if err := reg.Register(&fakeProvider{ns: "Bad-Case"}); !domainref.IsCode(err, domainref.CodeInvalidRef) {
		t.Errorf("malformed namespace: %v", err)
	}

	if err := reg.Register(newCRMProvider()); err != nil {
		t.Fatalf("first Register: %v", err)
	}
	// Exactly one authoritative owner per namespace: duplicates rejected.
	if err := reg.Register(newCRMProvider()); !domainref.IsCode(err, domainref.CodeInvalidRef) {
		t.Errorf("duplicate: %v", err)
	}

	if _, ok := reg.Provider("crm"); !ok {
		t.Error("Provider(crm) should be found")
	}
	if _, ok := reg.Provider("other"); ok {
		t.Error("Provider(other) should not be found")
	}
}

// ── businessRef compat read through the query path ─────────────────────────

func TestCompatBusinessRefQueryEndToEnd(t *testing.T) {
	reg := domainref.NewRegistry()
	reg.Register(&fakeProvider{
		ns:       "sources",
		versions: []int{1},
		objects:  map[string]string{"feishu_chat": "飞书消息同步"},
	})

	// A historical business_ref value from project_items converts and
	// resolves through the owning provider (version 0 = legacy, always
	// accepted).
	legacy := "sources:feishu:feishu_chat"
	ref, err := domainref.DomainRefFromBusinessRef(legacy)
	if err != nil {
		t.Fatalf("convert: %v", err)
	}
	sum, err := reg.Resolve(context.Background(), domainref.QueryRequest{Ref: ref, Actor: "scheduler"})
	if err != nil {
		t.Fatalf("Resolve legacy ref: %v", err)
	}
	if sum.Title != "飞书消息同步" {
		t.Errorf("title = %q", sum.Title)
	}
	if domainref.DomainRefToBusinessRef(ref) != legacy {
		t.Error("business_ref round trip must be byte-identical")
	}
}

// ── structured error shape ─────────────────────────────────────────────────

func TestStructuredErrorHelpers(t *testing.T) {
	err := domainref.NewError(domainref.CodePermissionDenied, "actor %q blocked", "x")
	c, ok := domainref.CodeOf(err)
	if !ok || c != domainref.CodePermissionDenied {
		t.Errorf("CodeOf = %v %v", c, ok)
	}
	if !domainref.IsCode(err, domainref.CodePermissionDenied) {
		t.Error("IsCode should match")
	}
	if domainref.IsCode(err, domainref.CodeNotFound) {
		t.Error("IsCode should not match a different code")
	}

	// Non-contract errors carry no code.
	if _, ok := domainref.CodeOf(context.Canceled); ok {
		t.Error("plain error must have no code")
	}

	// Structured errors serialize with code + message for HTTP envelopes.
	b, jerr := json.Marshal(err)
	if jerr != nil {
		t.Fatalf("marshal: %v", jerr)
	}
	for _, want := range []string{`"code":"permission_denied"`, `"message"`} {
		if !strings.Contains(string(b), want) {
			t.Errorf("JSON %s missing %s", b, want)
		}
	}
}

// ── default process-wide registry ──────────────────────────────────────────

func TestDefaultRegistry(t *testing.T) {
	if domainref.DefaultRegistry() == nil {
		t.Fatal("DefaultRegistry must not be nil")
	}
	if err := domainref.RegisterProvider(&fakeProvider{ns: "glob", objects: map[string]string{"1": "t"}}); err != nil {
		t.Fatalf("RegisterProvider: %v", err)
	}
	ref, _ := domainref.NewDomainRef("glob", "item", "1", 0)
	sum, err := domainref.Resolve(context.Background(), domainref.QueryRequest{Ref: ref, Actor: "test"})
	if err != nil || sum.Title != "t" {
		t.Fatalf("default Resolve: %+v %v", sum, err)
	}
}
