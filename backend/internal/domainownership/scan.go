package domainownership

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// Rule identifies one architecture gate check. The ids are stable: CI
// output and review comments reference them.
type Rule string

const (
	// RuleAppImportsApp: a domain application imports another application's
	// implementation (§3.4: 禁止应用依赖应用实现).
	RuleAppImportsApp Rule = "app_imports_app"
	// RuleAppImportsNonSDK: an application imports an internal package that
	// is not part of the kernel SDK surface (apps depend only on SDK
	// interfaces).
	RuleAppImportsNonSDK Rule = "app_imports_non_sdk"
	// RuleForeignRepoImport: a package outside an application imports that
	// application's repository package (direct access to another domain's
	// Repository), or any app implementation below an app root.
	RuleForeignRepoImport Rule = "foreign_repository_import"
	// RuleAppImportedByKernel: a non-aggregator kernel/platform package
	// imports an app's root package (dependency direction §3.4: the kernel
	// never depends on L3).
	RuleAppImportedByKernel Rule = "app_imported_by_kernel"
	// RuleCrossDomainSQLWrite: a SQL statement writes a table owned by
	// another namespace (§3.4: 禁止跨域 SQL).
	RuleCrossDomainSQLWrite Rule = "cross_domain_sql_write"
	// RuleCrossDomainSQLRead: an application directly reads another
	// domain's tables instead of resolving a DomainRef via Query (§4.2).
	RuleCrossDomainSQLRead Rule = "cross_domain_sql_read"
	// RuleParseError: a Go file failed to parse; it cannot be gated.
	RuleParseError Rule = "parse_error"
)

// Violation is one architecture gate finding.
type Violation struct {
	File    string `json:"file"` // path relative to the scanned root
	Line    int    `json:"line"`
	Rule    Rule   `json:"rule"`
	Message string `json:"message"`
}

func (v Violation) String() string {
	return fmt.Sprintf("%s:%d [%s] %s", v.File, v.Line, v.Rule, v.Message)
}

// modulePath is the Go module prefix of every internal import path.
const modulePath = "github.com/scottzx/1Agents/backend/"

// sdkPackages are the kernel interfaces domain applications may import
// (the "SDK surface"). Everything else under internal/ is off limits to
// apps; widening this list is an explicit architecture decision.
var sdkPackages = map[string]bool{
	"internal/appkit":           true, // runtime init seam
	"internal/appregistry":      true, // manifest + capability contract
	"internal/taskapi":          true, // north Task API
	"internal/domainref":        true, // cross-domain Query contract
	"internal/commandbus":       true, // unified Command gateway
	"internal/domainownership":  true, // this gate
	"internal/domainstore":      true, // app-owned tables & artifacts
	"internal/templateregistry": true, // project templates
}

// repoDirNames mark an app sub-package as a repository implementation:
// importing one from outside the app is RuleForeignRepoImport.
var repoDirNames = map[string]bool{"repository": true, "store": true}

// ScanBackend enforces the architecture gate over a backend source tree.
// root is the backend module root (the directory containing internal/ and
// cmd/). Test files (*_test.go) and testdata/ trees are excluded: tests may
// set up cross-domain fixtures, production code may not.
func ScanBackend(root string) ([]Violation, error) {
	apps := discoverApps(root)
	var out []Violation

	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, relErr := filepath.Rel(root, path)
		if relErr != nil {
			rel = path
		}
		rel = filepath.ToSlash(rel)
		if d.IsDir() {
			name := d.Name()
			if rel != "." && (name == "testdata" || name == "vendor" ||
				name == "node_modules" || name == "dist" || strings.HasPrefix(name, ".")) {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		out = append(out, scanFile(rel, path, apps)...)
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

// discoverApps lists application ids: the immediate subdirectories of
// internal/apps. Each app id doubles as its table namespace.
func discoverApps(root string) []string {
	entries, err := os.ReadDir(filepath.Join(root, "internal", "apps"))
	if err != nil {
		return nil
	}
	var apps []string
	for _, e := range entries {
		if e.IsDir() && isIdent(e.Name()) {
			apps = append(apps, e.Name())
		}
	}
	return apps
}

// appNamespaceOf returns the application namespace of a slash-separated
// path relative to the backend root: internal/apps/<ns>/... → (ns, true).
// Kernel/platform paths (including the internal/apps aggregator package
// itself) return (NamespaceKernel, false).
func appNamespaceOf(rel string) (string, bool) {
	const appsPrefix = "internal/apps/"
	if !strings.HasPrefix(rel, appsPrefix) {
		return NamespaceKernel, false
	}
	rest := rel[len(appsPrefix):]
	i := strings.IndexByte(rest, '/')
	if i <= 0 {
		return NamespaceKernel, false
	}
	id := rest[:i]
	if !isIdent(id) {
		return NamespaceKernel, false
	}
	return id, true
}

func scanFile(rel, abs string, apps []string) []Violation {
	var out []Violation
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, abs, nil, parser.SkipObjectResolution)
	if err != nil {
		out = append(out, Violation{File: rel, Rule: RuleParseError,
			Message: fmt.Sprintf("cannot parse for architecture gating: %v", err)})
		return out
	}

	callerNS, callerIsApp := appNamespaceOf(rel)
	// The aggregator lives directly in internal/apps (depth 2 rel segments);
	// it (and cmd/) may blank-import app root packages for registration.
	callerIsAggregator := countSlash(rel) == 2 && strings.HasPrefix(rel, "internal/apps/")
	callerIsCmd := strings.HasPrefix(rel, "cmd/")

	// ── import rules ────────────────────────────────────────────────────────
	for _, imp := range f.Imports {
		raw, err := strconv.Unquote(imp.Path.Value)
		if err != nil || !strings.HasPrefix(raw, modulePath) {
			continue // stdlib or external module: unrestricted
		}
		sub := raw[len(modulePath):] // e.g. internal/apps/presales/repository
		line := fset.Position(imp.Path.Pos()).Line

		targetAppID, depth := appTarget(sub)
		switch {
		case targetAppID == "" && callerIsApp && strings.HasPrefix(sub, "internal/"):
			if !sdkPackages[sub] {
				out = append(out, Violation{File: rel, Line: line, Rule: RuleAppImportsNonSDK,
					Message: fmt.Sprintf("app %q imports non-SDK internal package %q; apps depend only on SDK interfaces", callerNS, raw)})
			}
		case targetAppID != "" && callerIsApp && targetAppID != callerNS:
			if repoDirNames[firstSegment(sub, targetAppID)] {
				out = append(out, Violation{File: rel, Line: line, Rule: RuleForeignRepoImport,
					Message: fmt.Sprintf("app %q imports repository %q of app %q; use the owner's Command/Query contracts", callerNS, raw, targetAppID)})
			} else {
				out = append(out, Violation{File: rel, Line: line, Rule: RuleAppImportsApp,
					Message: fmt.Sprintf("app %q imports implementation %q of app %q", callerNS, raw, targetAppID)})
			}
		case targetAppID != "" && !callerIsApp:
			if depth > 1 {
				out = append(out, Violation{File: rel, Line: line, Rule: RuleForeignRepoImport,
					Message: fmt.Sprintf("kernel/platform imports app implementation %q (app %q); app internals are never imported from outside", raw, targetAppID)})
			} else if !(callerIsAggregator || callerIsCmd) {
				out = append(out, Violation{File: rel, Line: line, Rule: RuleAppImportedByKernel,
					Message: fmt.Sprintf("kernel/platform imports app package %q; only the apps aggregator or cmd/ may", raw)})
			}
		}
	}

	// ── SQL ownership rules ─────────────────────────────────────────────────
	out = append(out, scanSQL(rel, fset, f, callerNS, callerIsApp, apps)...)
	return out
}

// appTarget classifies a module-relative import path: when it addresses an
// application package it returns the app id and the import depth below the
// app root (1 = root package, >1 = implementation sub-package).
func appTarget(sub string) (appID string, depth int) {
	const p = "internal/apps/"
	if !strings.HasPrefix(sub, p) {
		return "", 0
	}
	parts := strings.Split(strings.Trim(sub[len(p):], "/"), "/")
	if parts[0] == "" || !isIdent(parts[0]) {
		return "", 0
	}
	return parts[0], len(parts)
}

// firstSegment returns the first path segment below an app root, e.g.
// "repository" for internal/apps/alpha/repository.
func firstSegment(sub, appID string) string {
	rest := strings.TrimPrefix(sub, "internal/apps/"+appID)
	rest = strings.Trim(rest, "/")
	if i := strings.IndexByte(rest, '/'); i >= 0 {
		return rest[:i]
	}
	return rest
}

func countSlash(s string) int { return strings.Count(s, "/") }

func scanSQL(rel string, fset *token.FileSet, f *ast.File, callerNS string, callerIsApp bool, apps []string) []Violation {
	var out []Violation
	ast.Inspect(f, func(n ast.Node) bool {
		lit, ok := n.(*ast.BasicLit)
		if !ok || lit.Kind != token.STRING {
			return true
		}
		s, err := strconv.Unquote(lit.Value)
		if err != nil || !looksLikeSQL(s) {
			return true
		}
		line := fset.Position(lit.Pos()).Line
		for _, stmt := range splitStatements(s) {
			tables, isWrite := classifyStatement(stmt)
			for _, table := range tables {
				if v := checkSQLOwnership(rel, line, callerNS, callerIsApp, apps, table, isWrite); v != nil {
					out = append(out, *v)
				}
			}
		}
		return true
	})
	return out
}

func checkSQLOwnership(rel string, line int, callerNS string, callerIsApp bool, apps []string, table string, isWrite bool) *Violation {
	claimed := NamespaceOfTable(table)
	if callerIsApp {
		if isWrite {
			if !strings.HasPrefix(table, TablePrefix(callerNS)) {
				return &Violation{File: rel, Line: line, Rule: RuleCrossDomainSQLWrite,
					Message: fmt.Sprintf("app %q writes table %q; apps may only write their own %s* tables", callerNS, table, TablePrefix(callerNS))}
			}
			return nil
		}
		// App reads: own tables fine; any other claimed domain namespace is
		// a direct cross-domain read (§4.2 requires a Query instead).
		if claimed != "" && claimed != callerNS && (IsReservedNamespace(claimed) || isAppID(apps, claimed)) {
			return &Violation{File: rel, Line: line, Rule: RuleCrossDomainSQLRead,
				Message: fmt.Sprintf("app %q reads table %q of namespace %q; resolve a DomainRef through the owner's Query instead", callerNS, table, claimed)}
		}
		return nil
	}
	// Kernel/platform writer: may touch legacy kernel tables freely, but
	// never tables of a reserved or application domain namespace.
	if isWrite && claimed != "" && claimed != NamespaceKernel &&
		(IsReservedNamespace(claimed) || isAppID(apps, claimed)) {
		return &Violation{File: rel, Line: line, Rule: RuleCrossDomainSQLWrite,
			Message: fmt.Sprintf("kernel/platform writes table %q owned by domain namespace %q; only the owner may write its tables", table, claimed)}
	}
	return nil
}

func isAppID(apps []string, id string) bool {
	for _, a := range apps {
		if a == id {
			return true
		}
	}
	return false
}

var sqlVerbs = []string{
	"INSERT", "UPDATE", "DELETE", "REPLACE", "CREATE", "DROP", "ALTER",
	"SELECT", "WITH",
}

func looksLikeSQL(s string) bool {
	head := strings.ToUpper(strings.TrimSpace(s))
	for _, v := range sqlVerbs {
		if strings.HasPrefix(head, v) {
			return true
		}
	}
	return false
}
