package domainownership

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// TestKernelLedgerCoversAllDDL keeps the ownership ledger honest: every table
// created by a kernel package must either be in the kernel ledger or carry
// the kernel_ prefix. Without this the ledger silently drifts from the schema.
func TestKernelLedgerCoversAllDDL(t *testing.T) {
	reg := NewRegistry()
	if err := RegisterKernelLedger(reg); err != nil {
		t.Fatalf("RegisterKernelLedger: %v", err)
	}

	root := backendRoot(t)
	kernelDirs := []string{
		filepath.Join(root, "internal", "meta"),
		filepath.Join(root, "internal", "commandbus"),
		filepath.Join(root, "internal", "appregistry"),
		filepath.Join(root, "internal", "domainownership"),
		filepath.Join(root, "internal", "outbox"),
	}
	for _, dir := range kernelDirs {
		entries, err := os.ReadDir(dir)
		if os.IsNotExist(err) {
			continue // kernel package not present in this tree (yet)
		}
		if err != nil {
			t.Fatalf("read %s: %v", dir, err)
		}
		for _, e := range entries {
			if e.IsDir() || !strings.HasSuffix(e.Name(), ".go") || strings.HasSuffix(e.Name(), "_test.go") {
				continue
			}
			path := filepath.Join(dir, e.Name())
			for _, table := range ddlTablesInFile(t, path) {
				if strings.HasPrefix(table, TablePrefix(NamespaceKernel)) {
					continue // new-style kernel tables are prefix-governed
				}
				if owner, ok := reg.TableOwner(table); !ok {
					t.Errorf("kernel table %q (created in %s) is missing from the ownership ledger", table, e.Name())
				} else if owner != NamespaceKernel {
					t.Errorf("kernel table %q owned by %q, want kernel", table, owner)
				}
			}
		}
	}
}

func ddlTablesInFile(t *testing.T, path string) []string {
	t.Helper()
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, path, nil, parser.SkipObjectResolution)
	if err != nil {
		t.Fatalf("parse %s: %v", path, err)
	}
	var tables []string
	ast.Inspect(f, func(n ast.Node) bool {
		lit, ok := n.(*ast.BasicLit)
		if !ok || lit.Kind != token.STRING {
			return true
		}
		s, err := strconv.Unquote(lit.Value)
		if err != nil {
			return true
		}
		for _, stmt := range splitStatements(s) {
			if name := TableNameFromDDL(stmt); name != "" {
				tables = append(tables, name)
			}
		}
		return true
	})
	return tables
}

func TestKernelLedgerWriteAPIs(t *testing.T) {
	reg := NewRegistry()
	if err := RegisterKernelLedger(reg); err != nil {
		t.Fatalf("RegisterKernelLedger: %v", err)
	}
	for _, c := range []string{
		"workcase.create", "workcase.update", "workcase.transition",
		"workcase.delete", "workcase.link", "workcase.unlink", "workcase.set_phase",
	} {
		owner, ok := reg.WriteAPIOwner(c)
		if !ok {
			t.Errorf("write API %q not ledgered", c)
		} else if owner != NamespaceKernel {
			t.Errorf("write API %q owned by %q, want kernel", c, owner)
		}
	}
}

func TestEnterpriseNamespaceUnowned(t *testing.T) {
	reg := NewRegistry()
	if err := RegisterKernelLedger(reg); err != nil {
		t.Fatalf("RegisterKernelLedger: %v", err)
	}
	// enterprise has no owner until a capability passes the promotion gate,
	// so enterprise_ tables cannot be registered.
	if err := reg.RegisterTable(NamespaceEnterprise, "enterprise_customers"); err == nil {
		t.Fatal("expected error registering enterprise_ table before promotion")
	}
}
