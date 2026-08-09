package domainownership

import (
	"database/sql"
	"fmt"
	"regexp"
	"strings"
)

// StartupGate prepares the process-wide ownership gate at server startup:
// it registers the kernel namespace and ledger (including the reserved
// presales/commerce owners), ensures the denial-audit table exists and
// installs the persistent denial sink. Idempotent.
func StartupGate(db *sql.DB) error {
	reg := Default()
	if err := RegisterKernelLedger(reg); err != nil {
		return fmt.Errorf("domainownership: register kernel ledger: %w", err)
	}
	if db != nil {
		if err := EnsureAuditSchema(db); err != nil {
			return err
		}
		SetDenialSink(DBSink(db))
	}
	return nil
}

// ddlTablePattern is re-declared here (guard.go keeps its own for DDL
// classification) to keep RegisterAppTables independent of the guard's
// statement splitter.
var ddlTablePattern = regexp.MustCompile(`(?i)^\s*CREATE\s+TABLE\s+(?:IF\s+NOT\s+EXISTS\s+)?` + tableIdent)

// RegisterAppTables records namespace ownership for every table an app
// creates via its DDL statements, so each new table has exactly one owner
// before the first row is written. The app's namespace is its app id; an
// app id that collides with a reserved namespace must BE that namespace's
// architecture owner (e.g. only the app "presales" may create presales_*
// tables). Called by appregistry.EnsureDomainTables after the DDL ran.
func RegisterAppTables(appID string, ddls []string) error {
	if len(ddls) == 0 {
		return nil // apps without domain tables impose no namespace rules
	}
	if !isIdent(appID) {
		return NewError(CodeInvalidDeclaration,
			"app id %q is not a valid namespace identifier (lowercase letters, digits, '_'); domain apps with tables must use a namespace-shaped id", appID)
	}
	reg := Default()
	if err := reg.RegisterNamespaceOwner(appID, appID); err != nil {
		return err
	}
	for _, ddl := range ddls {
		m := ddlTablePattern.FindStringSubmatch(strings.TrimSpace(ddl))
		if m == nil {
			continue // non-CREATE-TABLE DDL (indexes etc.) carry no new owner
		}
		table := m[1]
		if err := reg.RegisterTable(appID, table); err != nil {
			return err
		}
	}
	return nil
}

// ValidateAppTables checks an app manifest's declared domain tables against
// the ownership rules: every declared table must carry the app's namespace
// prefix and be registered to it, so manifests cannot claim tables they do
// not own. Apps with an id that is not a namespace identifier may not
// declare domain tables at all.
func ValidateAppTables(appID string, tables []string) error {
	if len(tables) == 0 {
		return nil
	}
	if !isIdent(appID) {
		return NewError(CodeInvalidDeclaration,
			"app %q declares domain tables but its id is not a namespace identifier", appID)
	}
	reg := Default()
	for _, t := range tables {
		if !strings.HasPrefix(t, TablePrefix(appID)) {
			return NewError(CodeInvalidDeclaration,
				"app %q declares table %q without its %q prefix", appID, t, TablePrefix(appID))
		}
		owner, known := reg.TableOwner(t)
		if !known {
			return NewError(CodeUnownedTable,
				"table %q declared by app %q is not registered in the ownership ledger", t, appID)
		}
		if owner != appID {
			return NewError(CodeDuplicateOwner,
				"table %q declared by app %q is owned by namespace %q", t, appID, owner)
		}
	}
	return nil
}
