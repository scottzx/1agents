package domainownership

import (
	"database/sql"
	"regexp"
	"strings"
)

// Guard is the controlled executor every new domain application must use
// for SQL access (§7.1: 受控事务 API). It validates each statement against
// the ownership registry before delegating to the underlying *sql.DB or
// *sql.Tx: writes and direct reads may only touch tables owned by the
// guard's namespace. Cross-domain statements are rejected and the denial is
// audited. Statements the parser does not recognize as table access pass
// through (PRAGMA, SAVEPOINT, ...), matching the C0 scope of policing
// table writes rather than every SQL feature.
type Guard struct {
	ns  string
	reg *Registry
	db  *sql.DB
	tx  *sql.Tx
}

// GuardDB wraps a *sql.DB for namespace ns.
func GuardDB(ns string, reg *Registry, db *sql.DB) *Guard {
	return &Guard{ns: ns, reg: reg, db: db}
}

// GuardTx wraps a *sql.Tx for namespace ns — the shape domain command
// handlers use inside their gateway transaction.
func GuardTx(ns string, reg *Registry, tx *sql.Tx) *Guard {
	return &Guard{ns: ns, reg: reg, tx: tx}
}

// Namespace returns the owning namespace this guard enforces.
func (g *Guard) Namespace() string { return g.ns }

// Check validates query without executing it: the first ownership violation
// found is returned as a structured *Error.
func (g *Guard) Check(query string) error {
	for _, stmt := range splitStatements(query) {
		tables, isWrite := classifyStatement(stmt)
		for _, table := range tables {
			if isWrite {
				if err := g.reg.CheckWrite(g.ns, table); err != nil {
					return err
				}
			} else {
				if err := g.reg.CheckRead(g.ns, table); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

// Exec validates then executes a write/DDL statement.
func (g *Guard) Exec(query string, args ...any) (sql.Result, error) {
	if err := g.Check(query); err != nil {
		return nil, err
	}
	if g.tx != nil {
		return g.tx.Exec(query, args...)
	}
	return g.db.Exec(query, args...)
}

// Query validates then runs a read statement. Guard intentionally exposes
// no QueryRow: *sql.Row cannot carry a pre-execution rejection, so domain
// code reads through Query and iterates rows.
func (g *Guard) Query(query string, args ...any) (*sql.Rows, error) {
	if err := g.Check(query); err != nil {
		return nil, err
	}
	if g.tx != nil {
		return g.tx.Query(query, args...)
	}
	return g.db.Query(query, args...)
}

// ── SQL statement analysis ─────────────────────────────────────────────────
//
// Deliberately conservative: statements are classified by their leading
// verb, and table names are extracted with narrow patterns matching the
// codebase's SQL style. Anything unrecognized is treated as passthrough so
// the guard never blocks a statement it cannot understand — ownership of
// unrecognized paths is policed by the architecture tests and review.

var (
	reInsertTable = regexp.MustCompile(`(?i)^\s*INSERT\s+(?:OR\s+[A-Z]+\s+)?INTO\s+` + tableIdent)
	reReplaceInto = regexp.MustCompile(`(?i)^\s*REPLACE\s+INTO\s+` + tableIdent)
	reUpdateTable = regexp.MustCompile(`(?i)^\s*UPDATE\s+` + tableIdent)
	reDeleteTable = regexp.MustCompile(`(?i)^\s*DELETE\s+FROM\s+` + tableIdent)
	reCreateTable = regexp.MustCompile(`(?i)^\s*CREATE\s+(?:TEMP\s+|TEMPORARY\s+)?TABLE\s+(?:IF\s+NOT\s+EXISTS\s+)?` + tableIdent)
	reCreateIndex = regexp.MustCompile(`(?i)^\s*CREATE\s+(?:UNIQUE\s+)?INDEX\s+(?:IF\s+NOT\s+EXISTS\s+)?\S+\s+ON\s+` + tableIdent)
	reDropTable   = regexp.MustCompile(`(?i)^\s*DROP\s+TABLE\s+(?:IF\s+EXISTS\s+)?` + tableIdent)
	reAlterTable  = regexp.MustCompile(`(?i)^\s*ALTER\s+TABLE\s+` + tableIdent)
	reFromTable   = regexp.MustCompile(`(?i)\bFROM\s+` + tableIdent)
	reJoinTable   = regexp.MustCompile(`(?i)\bJOIN\s+` + tableIdent)
)

// tableIdent matches one possibly-quoted SQL identifier and captures the
// bare name. SQLite quoting styles: "name", `name`, [name].
const tableIdent = `(?:"|` + "`" + `|\[)?([A-Za-z_][A-Za-z0-9_]*)(?:"|` + "`" + `|\])?`

// splitStatements splits a query on top-level semicolons. Quoted strings
// containing ';' are rare in this codebase; the naive split is acceptable
// because mis-splitting only weakens (never bypasses) classification of the
// first statement.
func splitStatements(query string) []string {
	parts := strings.Split(query, ";")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if strings.TrimSpace(p) != "" {
			out = append(out, p)
		}
	}
	return out
}

// classifyStatement returns the tables referenced by stmt and whether the
// statement mutates them. isWrite=false with zero tables means "not table
// access" (passthrough).
func classifyStatement(stmt string) (tables []string, isWrite bool) {
	trimmed := strings.TrimSpace(stmt)
	if trimmed == "" {
		return nil, false
	}
	switch {
	case reCreateTable.MatchString(trimmed):
		return reCreateTable.FindStringSubmatch(trimmed)[1:], true
	case reCreateIndex.MatchString(trimmed):
		return reCreateIndex.FindStringSubmatch(trimmed)[1:], true
	case reDropTable.MatchString(trimmed):
		return reDropTable.FindStringSubmatch(trimmed)[1:], true
	case reAlterTable.MatchString(trimmed):
		return reAlterTable.FindStringSubmatch(trimmed)[1:], true
	case reInsertTable.MatchString(trimmed):
		return reInsertTable.FindStringSubmatch(trimmed)[1:], true
	case reReplaceInto.MatchString(trimmed):
		return reReplaceInto.FindStringSubmatch(trimmed)[1:], true
	case reUpdateTable.MatchString(trimmed):
		return reUpdateTable.FindStringSubmatch(trimmed)[1:], true
	case reDeleteTable.MatchString(trimmed):
		return reDeleteTable.FindStringSubmatch(trimmed)[1:], true
	}
	// Read path: SELECT ... FROM / WITH ... SELECT. Collect FROM and JOIN
	// targets; subqueries simply contribute more matches.
	head := strings.ToUpper(trimmed)
	if strings.HasPrefix(head, "SELECT") || strings.HasPrefix(head, "WITH") {
		seen := map[string]bool{}
		for _, re := range []*regexp.Regexp{reFromTable, reJoinTable} {
			for _, m := range re.FindAllStringSubmatch(trimmed, -1) {
				name := m[1]
				// Skip SQL keywords that can follow FROM/JOIN in expressions.
				switch strings.ToUpper(name) {
				case "SELECT", "WHERE", "GROUP", "ORDER", "LIMIT", "VALUES":
					continue
				}
				if !seen[name] {
					seen[name] = true
					tables = append(tables, name)
				}
			}
		}
		return tables, false
	}
	return nil, false
}

// TableNameFromDDL extracts the created table name from a CREATE TABLE
// statement. Returns "" when the statement is not a CREATE TABLE.
func TableNameFromDDL(ddl string) string {
	if m := reCreateTable.FindStringSubmatch(strings.TrimSpace(ddl)); m != nil {
		return m[1]
	}
	return ""
}
