// Package domainownership implements the C0 "domain ownership and
// application dependency gate" slice frozen by
// docs/architecture/enterprise-foundation-v1.0.0.md (§3.4 boundary rules,
// §6.3 C0, §7 same-database domain partition, release gate §13.3):
//
//   - Namespaces: every table and write API belongs to exactly one owning
//     domain. Reserved namespaces are kernel, enterprise, presales and
//     commerce; new tables carry the owning namespace as prefix
//     (e.g. presales_opportunities). Pre-existing kernel tables are governed
//     through a ledger instead of being renamed (§7.1).
//   - Ownership registry: unique-owner registration for namespaces, tables
//     and write APIs (command contracts). Cross-domain writes through the
//     controlled executor (Guard) are rejected and audited.
//   - Repository exposure boundary: domain repositories are private to their
//     owning domain; cross-domain reads go through domainref.Query providers
//     and cross-domain mutations go through commandbus commands (§4.2).
//   - Architecture gate: a source scanner enforces that domain applications
//     (backend/internal/apps/<id>/) import only kernel SDK interfaces, never
//     another app's implementation or repository, and never write tables
//     outside their own namespace. The gate runs as ordinary Go tests
//     (make archgate) so CI can execute it.
//
// This package is L1 kernel infrastructure: it imports only the standard
// library. Domain applications and kernel platform packages import it; it
// never imports applications.
package domainownership
