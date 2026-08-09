// Package domainref implements the C0 cross-domain reference and read
// contracts frozen in docs/architecture/enterprise-foundation-v1.0.0.md
// (§4.2 Domain Object/DomainRef, §4.3 WorkCase, §5 Query, §7 ownership):
//
//   - DomainRef / CaseRef: versioned, immutable value objects referencing a
//     domain object or a WorkCase. They serialize stably and reject empty
//     namespace/type/id.
//   - A read-only Query contract: each domain registers one QueryProvider
//     under its namespace; cross-domain callers resolve authoritative state
//     (ObjectSummary) through the Registry instead of reading another
//     domain's tables directly.
//   - businessRef compatibility: historical project_items.business_ref
//     strings ("crm:lead:42", "sources:feishu:feishu_chat") convert
//     explicitly to DomainRef and back byte-identically.
//
// This package is L1 kernel infrastructure: it imports only the standard
// library. Domain applications import it; it never imports applications.
package domainref

import (
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// ── structured errors ──────────────────────────────────────────────────────

// Code is the machine-readable category of a structured contract error.
type Code string

const (
	CodeInvalidRef       Code = "invalid_ref"         // malformed or empty reference
	CodeUnknownProvider  Code = "unknown_provider"    // no provider registered for the namespace
	CodePermissionDenied Code = "permission_denied"   // actor may not read this object
	CodeVersionMismatch  Code = "version_unsupported" // contract version not supported
	CodeNotFound         Code = "not_found"           // referenced object does not exist
)

// Error is the structured error returned for every contract failure. It is
// JSON-serializable (for future HTTP envelopes) and inspectable via
// CodeOf/IsCode.
type Error struct {
	Code    Code   `json:"code"`
	Message string `json:"message"`
	// Ref is the canonical string of the offending reference, when applicable.
	Ref string `json:"ref,omitempty"`
}

func (e *Error) Error() string {
	if e.Ref != "" {
		return fmt.Sprintf("domainref: [%s] %s (ref=%s)", e.Code, e.Message, e.Ref)
	}
	return fmt.Sprintf("domainref: [%s] %s", e.Code, e.Message)
}

// NewError builds a structured contract error with the given code.
func NewError(code Code, format string, args ...any) *Error {
	return &Error{Code: code, Message: fmt.Sprintf(format, args...)}
}

// refError builds a structured contract error tied to a specific reference.
func refError(ref string, code Code, format string, args ...any) *Error {
	return &Error{Code: code, Message: fmt.Sprintf(format, args...), Ref: ref}
}

// CodeOf extracts the contract error code from err. ok=false when err is not
// a contract error.
func CodeOf(err error) (Code, bool) {
	var e *Error
	if !errors.As(err, &e) {
		return "", false
	}
	return e.Code, true
}

// IsCode reports whether err is a contract error carrying code.
func IsCode(err error, code Code) bool {
	c, ok := CodeOf(err)
	return ok && c == code
}

// ── component validation ───────────────────────────────────────────────────

// identPattern is the allowed shape for Namespace/Type and for CaseRef
// WorkspaceID/CaseID. Namespaces double as domain-table prefixes and
// registry keys, so they are lowercase identifiers; every historical
// business_ref namespace/type ("crm", "sources", "feishu", ...) conforms.
var identPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]*$`)

func checkIdent(kind, s string) error {
	if s == "" {
		return NewError(CodeInvalidRef, "%s must not be empty", kind)
	}
	if !identPattern.MatchString(s) {
		return NewError(CodeInvalidRef,
			"%s %q must match %s (lowercase letters, digits, '_' or '-')", kind, s, identPattern)
	}
	return nil
}

// checkID validates an object/case id: non-empty, no surrounding whitespace,
// no '@' (reserved for the version suffix). ':' is tolerated so historical
// colon-bearing ids survive compat parsing.
func checkID(kind, s string) error {
	if s == "" {
		return NewError(CodeInvalidRef, "%s must not be empty", kind)
	}
	if s != strings.TrimSpace(s) {
		return NewError(CodeInvalidRef, "%s %q must not have surrounding whitespace", kind, s)
	}
	if strings.Contains(s, "@") {
		return NewError(CodeInvalidRef, "%s %q must not contain '@'", kind, s)
	}
	return nil
}

func checkVersion(v int) error {
	if v < 0 {
		return NewError(CodeInvalidRef, "contract version must be >= 0, got %d", v)
	}
	return nil
}

// ── DomainRef ──────────────────────────────────────────────────────────────

// DomainRef is a versioned, immutable reference to a domain object owned by
// exactly one domain application (§4.2: {domain, objectType, objectId,
// contractVersion}). Namespace is the owning domain's identifier, Type the
// object type within that domain, ID the opaque object id.
//
// ContractVersion pins the read contract the consumer expects: 0 means
// "unversioned/legacy" (e.g. converted from a historical business_ref),
// >0 names a specific contract version.
//
// Cross-domain consumers must never derive table names or SQL from a
// DomainRef; they resolve it through the owning domain's QueryProvider.
type DomainRef struct {
	Namespace       string `json:"namespace"`
	Type            string `json:"type"`
	ID              string `json:"id"`
	ContractVersion int    `json:"contractVersion"`
}

// NewDomainRef returns a validated DomainRef.
func NewDomainRef(namespace, objectType, id string, contractVersion int) (DomainRef, error) {
	r := DomainRef{Namespace: namespace, Type: objectType, ID: id, ContractVersion: contractVersion}
	if err := r.Validate(); err != nil {
		return DomainRef{}, err
	}
	return r, nil
}

// Validate checks the structural invariants: non-empty namespace/type/id and
// a non-negative version, with namespaces/types as lowercase identifiers.
func (r DomainRef) Validate() error {
	if err := checkIdent("namespace", r.Namespace); err != nil {
		return err
	}
	if err := checkIdent("type", r.Type); err != nil {
		return err
	}
	if err := checkID("id", r.ID); err != nil {
		return err
	}
	return checkVersion(r.ContractVersion)
}

// String returns the canonical, stable serialization:
//
//	version 0: "namespace:type:id"     (byte-identical to legacy business_ref)
//	version N: "namespace:type:id@vN"
//
// ParseDomainRef(String(r)) == r for every valid ref.
func (r DomainRef) String() string {
	s := r.Namespace + ":" + r.Type + ":" + r.ID
	if r.ContractVersion > 0 {
		s += "@v" + strconv.Itoa(r.ContractVersion)
	}
	return s
}

// ParseDomainRef parses the canonical form produced by String. The id part is
// everything after the second ':' (it may itself contain ':'); a trailing
// "@vN" (N>0) on the id is stripped into ContractVersion.
func ParseDomainRef(s string) (DomainRef, error) {
	if s == "" {
		return DomainRef{}, NewError(CodeInvalidRef, "ref must not be empty")
	}
	parts := strings.SplitN(s, ":", 3)
	if len(parts) != 3 || parts[2] == "" {
		return DomainRef{}, refError(s, CodeInvalidRef, "ref must be namespace:type:id")
	}
	id, version := parts[2], 0
	if i := strings.LastIndex(id, "@v"); i >= 0 {
		if n, err := strconv.Atoi(id[i+2:]); err == nil && n > 0 {
			id, version = id[:i], n
		}
	}
	ref := DomainRef{Namespace: parts[0], Type: parts[1], ID: id, ContractVersion: version}
	if err := ref.Validate(); err != nil {
		if e, ok := err.(*Error); ok && e.Ref == "" {
			e.Ref = s
		}
		return DomainRef{}, err
	}
	return ref, nil
}

// ── CaseRef ────────────────────────────────────────────────────────────────

// CasePrefix is the reserved prefix of every CaseRef serialization. It keeps
// CaseRef strings distinguishable from DomainRef strings.
const CasePrefix = "case"

// CaseRef is a versioned, immutable reference to a WorkCase, the
// kernel-owned coordination object scoped to a workspace (§4.3).
// ContractVersion follows the same rules as DomainRef.
type CaseRef struct {
	WorkspaceID     string `json:"workspaceId"`
	CaseID          string `json:"caseId"`
	ContractVersion int    `json:"contractVersion"`
}

// NewCaseRef returns a validated CaseRef.
func NewCaseRef(workspaceID, caseID string, contractVersion int) (CaseRef, error) {
	c := CaseRef{WorkspaceID: workspaceID, CaseID: caseID, ContractVersion: contractVersion}
	if err := c.Validate(); err != nil {
		return CaseRef{}, err
	}
	return c, nil
}

// Validate checks the structural invariants: non-empty workspaceId/caseId
// (lowercase identifiers) and a non-negative version.
func (c CaseRef) Validate() error {
	if err := checkIdent("workspaceId", c.WorkspaceID); err != nil {
		return err
	}
	if err := checkIdent("caseId", c.CaseID); err != nil {
		return err
	}
	return checkVersion(c.ContractVersion)
}

// String returns the canonical, stable serialization:
//
//	version 0: "case:<workspaceId>:<caseId>"
//	version N: "case:<workspaceId>:<caseId>@vN"
func (c CaseRef) String() string {
	s := CasePrefix + ":" + c.WorkspaceID + ":" + c.CaseID
	if c.ContractVersion > 0 {
		s += "@v" + strconv.Itoa(c.ContractVersion)
	}
	return s
}

// ParseCaseRef parses the canonical form produced by String.
func ParseCaseRef(s string) (CaseRef, error) {
	if s == "" {
		return CaseRef{}, NewError(CodeInvalidRef, "case ref must not be empty")
	}
	parts := strings.Split(s, ":")
	if len(parts) != 3 || parts[0] != CasePrefix {
		return CaseRef{}, refError(s, CodeInvalidRef, "case ref must be %s:<workspaceId>:<caseId>", CasePrefix)
	}
	id, version := parts[2], 0
	if i := strings.LastIndex(id, "@v"); i >= 0 {
		if n, err := strconv.Atoi(id[i+2:]); err == nil && n > 0 {
			id, version = id[:i], n
		}
	}
	ref := CaseRef{WorkspaceID: parts[1], CaseID: id, ContractVersion: version}
	if err := ref.Validate(); err != nil {
		if e, ok := err.(*Error); ok && e.Ref == "" {
			e.Ref = s
		}
		return CaseRef{}, err
	}
	return ref, nil
}
