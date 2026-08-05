package commandbus

import (
	"context"
	"database/sql"
	"sort"
	"sync"
)

// HandlerFunc executes one registered command inside the gateway's
// transaction. The handler owns all domain validation and writes: it mutates
// the owner's tables through tx, appends the domain's audit event, and
// returns the result payload. Failures must be returned as *Error so the
// gateway can audit them under a stable code:
//
//	CodeInvalidPayload  — malformed payload fields
//	CodeVersionConflict — stale expectedVersion
//	CodeDomainRejected  — business-rule rejection (wrap the domain sentinel)
//	CodeInternal        — unexpected I/O failure
type HandlerFunc func(ctx context.Context, cmd Command, tx *sql.Tx) (Result, error)

// Descriptor registers one command contract with the gateway: the versions
// the handler speaks, the actor kinds allowed to execute it, an optional
// fine-grained policy and the handler itself.
type Descriptor struct {
	// Contract is the command name, e.g. "workcase.transition".
	Contract string
	// SchemaVersions lists the payload versions the handler accepts.
	SchemaVersions []int
	// AllowedKinds lists the actor kinds permitted to execute the command.
	// An empty list denies every actor.
	AllowedKinds []ActorKind
	// Authorize is an optional fine-grained policy run after the kind
	// check; it may inspect the payload (e.g. forbid agent-driven terminal
	// transitions). Return a *Error with CodePermissionDenied to reject.
	Authorize func(cmd Command) error
	// Handler executes the command.
	Handler HandlerFunc
}

func (d Descriptor) supportsVersion(v int) bool {
	for _, s := range d.SchemaVersions {
		if s == v {
			return true
		}
	}
	return false
}

func (d Descriptor) allows(kind ActorKind) bool {
	for _, k := range d.AllowedKinds {
		if k == kind {
			return true
		}
	}
	return false
}

// Registry maps command contracts to their descriptors. The zero value is
// not usable; construct with NewRegistry or use a Gateway.
type Registry struct {
	mu      sync.RWMutex
	byName  map[string]Descriptor
	version map[string]int // contract → max supported version (informational)
}

// NewRegistry returns an empty registry.
func NewRegistry() *Registry {
	return &Registry{byName: map[string]Descriptor{}, version: map[string]int{}}
}

// Register adds d. Malformed contracts and duplicate registrations are
// rejected: there is exactly one authoritative handler per command contract.
func (r *Registry) Register(d Descriptor) error {
	if !contractPattern.MatchString(d.Contract) {
		return NewError(CodeInvalidPayload,
			"contract %q must be a dotted lowercase identifier", d.Contract)
	}
	if len(d.SchemaVersions) == 0 {
		return NewError(CodeInvalidPayload, "contract %q: at least one schemaVersion is required", d.Contract)
	}
	if d.Handler == nil {
		return NewError(CodeInvalidPayload, "contract %q: handler is required", d.Contract)
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, dup := r.byName[d.Contract]; dup {
		return NewError(CodeInvalidPayload, "command contract %q already registered", d.Contract)
	}
	max := 0
	for _, v := range d.SchemaVersions {
		if v < 1 {
			return NewError(CodeInvalidPayload, "contract %q: schemaVersion must be >= 1", d.Contract)
		}
		if v > max {
			max = v
		}
	}
	r.byName[d.Contract] = d
	r.version[d.Contract] = max
	return nil
}

// Lookup returns the descriptor registered for contract.
func (r *Registry) Lookup(contract string) (Descriptor, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	d, ok := r.byName[contract]
	return d, ok
}

// Contracts lists the registered command names, sorted.
func (r *Registry) Contracts() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]string, 0, len(r.byName))
	for name := range r.byName {
		out = append(out, name)
	}
	sort.Strings(out)
	return out
}
