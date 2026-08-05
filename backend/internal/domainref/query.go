package domainref

import (
	"context"
	"sync"
)

// ── query envelope ─────────────────────────────────────────────────────────

// QueryRequest is the read-only query envelope (§5.3, C0 in-process subset):
// the target reference, the actor identity for object-level permission
// checks, an optional workspace scope, and a correlation id for audit.
type QueryRequest struct {
	Ref           DomainRef `json:"ref"`
	Actor         string    `json:"actor"`
	WorkspaceID   string    `json:"workspaceId,omitempty"`
	CorrelationID string    `json:"correlationId,omitempty"`
}

// ObjectSummary is the authoritative summary (§4.2) the owning domain
// returns for a DomainRef: enough for cross-domain display and coordination
// without exposing storage. Fields carries the domain-owned typed payload;
// the registry never inspects it.
type ObjectSummary struct {
	Ref    DomainRef      `json:"ref"`
	Title  string         `json:"title"`
	Status string         `json:"status,omitempty"`
	Link   string         `json:"link,omitempty"`
	Fields map[string]any `json:"fields,omitempty"`
}

// ── provider contract ──────────────────────────────────────────────────────

// QueryProvider is implemented by each domain application to expose
// read-only authoritative access to its objects. Cross-domain consumers must
// resolve references through a Registry (which dispatches by namespace) and
// must never read another domain's tables directly (§3.4, §7.1).
type QueryProvider interface {
	// Namespace returns the owning domain this provider answers for. It must
	// equal DomainRef.Namespace of every object it serves.
	Namespace() string
	// Versions enumerates the contract versions the provider supports.
	// Version 0 (legacy/unversioned, e.g. converted business_ref values) is
	// always accepted by the registry.
	Versions() []int
	// Query returns the authoritative summary for req.Ref and enforces
	// object-level permissions for req.Actor: return
	// NewError(CodePermissionDenied, …) when the actor may not read this
	// object, or NewError(CodeNotFound, …) when it does not exist.
	Query(ctx context.Context, req QueryRequest) (ObjectSummary, error)
}

// ── registry ───────────────────────────────────────────────────────────────

// Registry resolves read-only queries by dispatching to the owning
// provider registered under the reference's namespace.
type Registry struct {
	mu        sync.RWMutex
	providers map[string]QueryProvider
}

// NewRegistry returns an empty registry.
func NewRegistry() *Registry {
	return &Registry{providers: make(map[string]QueryProvider)}
}

// Register adds p under p.Namespace(). It fails with CodeInvalidRef when the
// namespace is malformed or already registered (re-registration is a
// programming error; there is exactly one authoritative owner per domain).
func (r *Registry) Register(p QueryProvider) error {
	ns := p.Namespace()
	if err := checkIdent("provider namespace", ns); err != nil {
		return err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, dup := r.providers[ns]; dup {
		return NewError(CodeInvalidRef, "query provider for namespace %q already registered", ns)
	}
	r.providers[ns] = p
	return nil
}

// Provider returns the provider registered for namespace, if any.
func (r *Registry) Provider(namespace string) (QueryProvider, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	p, ok := r.providers[namespace]
	return p, ok
}

// Resolve validates req.Ref, dispatches to the owning provider by namespace,
// and checks contract-version compatibility. Structured error codes:
//
//	CodeInvalidRef       — malformed reference
//	CodeUnknownProvider  — no provider registered for ref.Namespace
//	CodeVersionMismatch  — ref.ContractVersion unsupported by the provider
//	CodePermissionDenied / CodeNotFound — surfaced from the provider
func (r *Registry) Resolve(ctx context.Context, req QueryRequest) (ObjectSummary, error) {
	if err := req.Ref.Validate(); err != nil {
		return ObjectSummary{}, err
	}
	p, ok := r.Provider(req.Ref.Namespace)
	if !ok {
		return ObjectSummary{}, refError(req.Ref.String(), CodeUnknownProvider,
			"no query provider registered for namespace %q", req.Ref.Namespace)
	}
	if v := req.Ref.ContractVersion; v != 0 && !supportsVersion(p.Versions(), v) {
		return ObjectSummary{}, refError(req.Ref.String(), CodeVersionMismatch,
			"provider %q supports contract versions %v, not %d", p.Namespace(), p.Versions(), v)
	}
	return p.Query(ctx, req)
}

func supportsVersion(versions []int, v int) bool {
	for _, s := range versions {
		if s == v {
			return true
		}
	}
	return false
}

// ── process-wide default registry ──────────────────────────────────────────
//
// Apps register their provider from init() / appkit.OnInit, mirroring the
// taskapi global-function pattern. Tests that need isolation construct their
// own Registry via NewRegistry.

var defaultRegistry = NewRegistry()

// DefaultRegistry returns the process-wide query registry.
func DefaultRegistry() *Registry { return defaultRegistry }

// RegisterProvider registers p in the default registry.
func RegisterProvider(p QueryProvider) error { return defaultRegistry.Register(p) }

// Resolve dispatches req through the default registry.
func Resolve(ctx context.Context, req QueryRequest) (ObjectSummary, error) {
	return defaultRegistry.Resolve(ctx, req)
}

// ── businessRef compatibility ──────────────────────────────────────────────

// DomainRefFromBusinessRef explicitly converts a historical
// project_items.business_ref value (e.g. "crm:lead:42",
// "sources:feishu:feishu_chat") into a versioned DomainRef with
// ContractVersion 0 (legacy). Its String() is byte-identical to the input,
// so existing rows, lookups and ListTasksByBusinessRef keep working.
// Malformed historical values fail with a structured CodeInvalidRef error so
// callers can surface them instead of silently mis-binding.
func DomainRefFromBusinessRef(s string) (DomainRef, error) {
	return ParseDomainRef(s)
}

// DomainRefToBusinessRef serializes ref back into the business_ref storage
// shape. Version-0 refs round-trip byte-identically with historical strings.
func DomainRefToBusinessRef(ref DomainRef) string {
	return ref.String()
}
