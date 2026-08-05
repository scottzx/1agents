package domainownership

import "strings"

// Reserved namespaces (§7.1 同库分域). SQLite has no real schema namespaces,
// so ownership is expressed through table-name prefixes plus this registry
// and the architecture gate tests.
const (
	// NamespaceKernel owns the runtime-kernel facts: Workspace, Identity,
	// WorkCase, Task, Session, Artifact, Agent, permission, audit and the
	// Command/Query/Event infrastructure (§3.1). New kernel tables use the
	// kernel_ prefix; pre-existing kernel tables stay unprefixed and are
	// governed through the ledger (RegisterKernelLedger).
	NamespaceKernel = "kernel"

	// NamespaceEnterprise is reserved for enterprise shared capabilities that
	// passed the promotion gate (§3.2, §6.1). It has no owner until a
	// capability is promoted; nobody may create enterprise_ tables beforehand.
	NamespaceEnterprise = "enterprise"

	// NamespacePresales belongs to the presales & delivery application (§3.3,
	// §9.1): presales_opportunities, presales_solution_versions, ...
	NamespacePresales = "presales"

	// NamespaceCommerce belongs to the commerce operations application
	// (§3.3, §9.2): commerce_products, commerce_listings, ...
	NamespaceCommerce = "commerce"
)

// reservedNamespaces are the namespaces fixed by the frozen v1.0.0
// architecture. Additional application namespaces may be registered at
// runtime (one per domain app, equal to the app id), but the reserved set
// cannot be re-assigned.
var reservedNamespaces = map[string]bool{
	NamespaceKernel:     true,
	NamespaceEnterprise: true,
	NamespacePresales:   true,
	NamespaceCommerce:   true,
}

// ReservedNamespaces lists the frozen architecture namespaces.
func ReservedNamespaces() []string {
	return []string{NamespaceKernel, NamespaceEnterprise, NamespacePresales, NamespaceCommerce}
}

// IsReservedNamespace reports whether ns is one of the four frozen
// architecture namespaces.
func IsReservedNamespace(ns string) bool { return reservedNamespaces[ns] }

// TablePrefix returns the table-name prefix owned by namespace ns.
func TablePrefix(ns string) string { return ns + "_" }

// NamespaceOfTable derives the owning namespace from a table name: the
// segment before the first '_' when it looks like a namespace identifier.
// Tables without an underscore (or whose prefix is not an identifier) have
// no derivable namespace and are governed through the ledger instead.
//
// Note: derivation only says what the prefix *claims*; whether the claimed
// namespace actually owns the table is decided by the registry.
func NamespaceOfTable(table string) string {
	i := strings.IndexByte(table, '_')
	if i <= 0 || i == len(table)-1 {
		return ""
	}
	prefix := table[:i]
	if !isIdent(prefix) {
		return ""
	}
	return prefix
}

// isIdent reports whether s is a lowercase identifier: starts with a letter
// or digit-free lowercase letter and contains only [a-z0-9_]. Matches the
// domainref ident convention so namespaces double as DomainRef namespaces.
func isIdent(s string) bool {
	if s == "" {
		return false
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c >= 'a' && c <= 'z':
		case c >= '0' && c <= '9':
		case c == '_':
		default:
			return false
		}
	}
	return true
}
