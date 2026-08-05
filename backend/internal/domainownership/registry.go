package domainownership

import (
	"fmt"
	"strings"
	"sync"
)

// Code is the machine-readable category of an ownership violation. Mirrors
// the domainref/commandbus structured-error convention so gates, HTTP
// surfaces and audits speak one vocabulary.
type Code string

const (
	CodeUnknownNamespace   Code = "unknown_namespace"   // namespace not registered
	CodeNamespaceOwned     Code = "namespace_owned"     // namespace already owned by another domain
	CodeDuplicateOwner     Code = "duplicate_owner"     // table/write API already owned by another domain
	CodeCrossDomainWrite   Code = "cross_domain_write"  // write to a table owned by another domain
	CodeCrossDomainRead    Code = "cross_domain_read"   // direct read of a table owned by another domain
	CodeUnownedTable       Code = "unowned_table"       // table not in the ownership registry or ledger
	CodeRepositoryAccess   Code = "repository_access"   // direct access to another domain's repository
	CodePermissionDenied   Code = "permission_denied"   // query-path permission denial (audited via hook)
	CodeInvalidDeclaration Code = "invalid_declaration" // malformed namespace/table/contract declaration
)

// Error is the structured error returned for every ownership violation.
type Error struct {
	Code            Code   `json:"code"`
	Message         string `json:"message"`
	CallerNamespace string `json:"callerNamespace,omitempty"`
	TargetNamespace string `json:"targetNamespace,omitempty"`
	Target          string `json:"target,omitempty"` // table / contract / repository name
}

func (e *Error) Error() string {
	return fmt.Sprintf("domainownership: [%s] %s", e.Code, e.Message)
}

// NewError builds a structured ownership error.
func NewError(code Code, format string, args ...any) *Error {
	return &Error{Code: code, Message: fmt.Sprintf(format, args...)}
}

// CodeOf extracts the ownership error code from err. ok=false when err is
// not an ownership error.
func CodeOf(err error) (Code, bool) {
	if e, ok := err.(*Error); ok {
		return e.Code, true
	}
	return "", false
}

// IsCode reports whether err is an ownership error carrying code.
func IsCode(err error, code Code) bool {
	c, ok := CodeOf(err)
	return ok && c == code
}

// Registry records the unique owner of every namespace, table and write API
// (command contract). There is exactly one owner per item: duplicate
// registrations by a different owner are rejected (§7.1: 每个权威事实只有
// 一个所有者和一个写入口).
//
// The zero value is not usable; construct with NewRegistry or use Default.
type Registry struct {
	mu         sync.RWMutex
	nsOwner    map[string]string // namespace → owning domain id
	tableOwner map[string]string // table name → owning namespace
	apiOwner   map[string]string // command contract → owning namespace
	repoOwner  map[string]string // "ns/name" → owning namespace
}

// NewRegistry returns an empty registry.
func NewRegistry() *Registry {
	return &Registry{
		nsOwner:    map[string]string{},
		tableOwner: map[string]string{},
		apiOwner:   map[string]string{},
		repoOwner:  map[string]string{},
	}
}

var (
	defaultMu sync.Mutex
	defaultRe = NewRegistry()
)

// Default returns the process-wide ownership registry. Kernel startup and
// app registration hooks (appregistry.EnsureDomainTables) record ownership
// here; the Guard and the architecture tests validate against it.
func Default() *Registry {
	defaultMu.Lock()
	defer defaultMu.Unlock()
	return defaultRe
}

// RegisterNamespaceOwner declares the owning domain of namespace ns.
// Reserved namespaces may only be claimed by their architecture owner
// (kernel → "kernel"; presales → app "presales"; commerce → app
// "commerce"; enterprise is claimed per promoted capability). Re-declaring
// the same owner is idempotent; a conflicting owner fails.
func (r *Registry) RegisterNamespaceOwner(ns, owner string) error {
	if !isIdent(ns) {
		return NewError(CodeInvalidDeclaration, "namespace %q must be a lowercase identifier", ns)
	}
	if owner == "" {
		return NewError(CodeInvalidDeclaration, "namespace %q: owner must not be empty", ns)
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if cur, ok := r.nsOwner[ns]; ok {
		if cur == owner {
			return nil // idempotent re-registration by the same owner
		}
		return NewError(CodeNamespaceOwned,
			"namespace %q is already owned by %q, cannot be claimed by %q", ns, cur, owner)
	}
	r.nsOwner[ns] = owner
	return nil
}

// NamespaceOwner returns the owning domain registered for ns.
func (r *Registry) NamespaceOwner(ns string) (string, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	o, ok := r.nsOwner[ns]
	return o, ok
}

// RegisterTable records that namespace ns owns table. New tables must carry
// the ns_ prefix (§7.1). Unique owner per table: re-registration by the
// same namespace is idempotent, by another namespace it fails.
func (r *Registry) RegisterTable(ns, table string) error {
	if !strings.HasPrefix(table, TablePrefix(ns)) {
		return NewError(CodeInvalidDeclaration,
			"table %q does not carry the %q prefix of its owning namespace", table, TablePrefix(ns))
	}
	return r.registerTable(ns, table)
}

// RegisterLegacyTable records ownership of a pre-existing table that
// predates the prefix rule (§7.1: 已有核心表通过所有权清单纳管，不为了形式
// 一致做高风险全表改名). Only the kernel ledger uses this.
func (r *Registry) RegisterLegacyTable(ns, table string) error {
	return r.registerTable(ns, table)
}

func (r *Registry) registerTable(ns, table string) error {
	if !isIdent(ns) {
		return NewError(CodeInvalidDeclaration, "namespace %q must be a lowercase identifier", ns)
	}
	if !isIdent(table) {
		return NewError(CodeInvalidDeclaration, "table %q must be a lowercase identifier", table)
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.nsOwner[ns]; !ok {
		return NewError(CodeUnknownNamespace,
			"namespace %q is not registered; register its owner before creating tables", ns)
	}
	if cur, dup := r.tableOwner[table]; dup {
		if cur == ns {
			return nil // idempotent
		}
		return NewError(CodeDuplicateOwner,
			"table %q is already owned by namespace %q, cannot be registered to %q", table, cur, ns)
	}
	r.tableOwner[table] = ns
	return nil
}

// TableOwner returns the owning namespace registered for table.
func (r *Registry) TableOwner(table string) (string, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	ns, ok := r.tableOwner[table]
	return ns, ok
}

// TablesOwnedBy lists the tables registered to namespace ns (sorted).
func (r *Registry) TablesOwnedBy(ns string) []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := []string{}
	for t, owner := range r.tableOwner {
		if owner == ns {
			out = append(out, t)
		}
	}
	sortStrings(out)
	return out
}

// RegisterWriteAPI records that namespace ns owns the command contract
// (write API), e.g. kernel owns "workcase.transition" and the presales app
// will own "presales.opportunity.create". Unique owner per contract.
func (r *Registry) RegisterWriteAPI(ns, contract string) error {
	if !isIdent(ns) {
		return NewError(CodeInvalidDeclaration, "namespace %q must be a lowercase identifier", ns)
	}
	if contract == "" {
		return NewError(CodeInvalidDeclaration, "write API contract must not be empty")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.nsOwner[ns]; !ok {
		return NewError(CodeUnknownNamespace, "namespace %q is not registered", ns)
	}
	if cur, dup := r.apiOwner[contract]; dup {
		if cur == ns {
			return nil
		}
		return NewError(CodeDuplicateOwner,
			"write API %q is already owned by namespace %q, cannot be registered to %q", contract, cur, ns)
	}
	r.apiOwner[contract] = ns
	return nil
}

// WriteAPIOwner returns the owning namespace registered for contract.
func (r *Registry) WriteAPIOwner(contract string) (string, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	ns, ok := r.apiOwner[contract]
	return ns, ok
}

// CheckWrite enforces owner-writes: callerNS may only mutate tables it
// owns. Every denial is recorded through the process audit sink (§13.3:
// 所有权测试能阻止违规; audits make denials reviewable).
func (r *Registry) CheckWrite(callerNS, table string) error {
	return r.checkAccess(callerNS, table, CodeCrossDomainWrite, "write")
}

// CheckRead enforces the §4.2 rule that cross-domain consumers do not read
// another domain's tables directly (they resolve a DomainRef through the
// owning domain's Query provider instead).
func (r *Registry) CheckRead(callerNS, table string) error {
	return r.checkAccess(callerNS, table, CodeCrossDomainRead, "read")
}

func (r *Registry) checkAccess(callerNS, table string, crossCode Code, verb string) error {
	if !isIdent(callerNS) {
		return NewError(CodeInvalidDeclaration, "caller namespace %q must be a lowercase identifier", callerNS)
	}
	owner, known := r.TableOwner(table)
	if !known {
		// Unregistered tables are outside the ownership contract (legacy
		// kernel paths not yet ledgered). The architecture gate — not the
		// runtime registry — polices those statically.
		return nil
	}
	if owner == callerNS {
		return nil
	}
	action := ActionTableRead
	if verb == "write" {
		action = ActionTableWrite
	}
	err := &Error{
		Code:            crossCode,
		Message:         fmt.Sprintf("namespace %q may not %s table %q owned by namespace %q; use the owner's Command/Query contract", callerNS, verb, table, owner),
		CallerNamespace: callerNS,
		TargetNamespace: owner,
		Target:          table,
	}
	RecordDenial(Denial{
		CallerNamespace: callerNS,
		Action:          action,
		TargetNamespace: owner,
		Target:          table,
		Code:            string(err.Code),
		Reason:          err.Message,
	})
	return err
}

// sortStrings is a tiny insertion sort to avoid importing sort for one call
// (keeps the package visibly stdlib-minimal). n is small (owned tables).
func sortStrings(s []string) {
	for i := 1; i < len(s); i++ {
		for j := i; j > 0 && s[j] < s[j-1]; j-- {
			s[j], s[j-1] = s[j-1], s[j]
		}
	}
}
