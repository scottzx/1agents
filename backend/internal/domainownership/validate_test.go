package domainownership

import (
	"testing"
)

func TestRegisterAppTablesOwnsNamespace(t *testing.T) {
	// Use the process-wide registry, exactly as appregistry.EnsureDomainTables
	// does at runtime. Unique app id keeps the test hermetic.
	appID := "validapp"
	ddls := []string{
		`CREATE TABLE IF NOT EXISTS validapp_items (id TEXT PRIMARY KEY)`,
		`CREATE TABLE IF NOT EXISTS validapp_notes (id TEXT PRIMARY KEY)`,
	}
	if err := RegisterAppTables(appID, ddls); err != nil {
		t.Fatalf("RegisterAppTables: %v", err)
	}
	reg := Default()
	if owner, ok := reg.NamespaceOwner(appID); !ok || owner != appID {
		t.Fatalf("namespace %q owner = %q,%v; want %q,true", appID, owner, ok, appID)
	}
	for _, table := range []string{"validapp_items", "validapp_notes"} {
		if owner, ok := reg.TableOwner(table); !ok || owner != appID {
			t.Fatalf("table %q owner = %q,%v; want %q,true", table, owner, ok, appID)
		}
	}

	// A second app cannot register tables under validapp_'s prefix.
	if err := RegisterAppTables("otherapp", []string{
		`CREATE TABLE IF NOT EXISTS validapp_items (id TEXT PRIMARY KEY)`,
	}); err == nil {
		t.Fatal("expected error when another app claims validapp_ tables")
	}
}

func TestRegisterAppTablesRejectsForeignPrefix(t *testing.T) {
	// The DDL table does not carry the app's own prefix.
	err := RegisterAppTables("foreignapp", []string{
		`CREATE TABLE IF NOT EXISTS someoneelse_items (id TEXT PRIMARY KEY)`,
	})
	if err == nil {
		t.Fatal("expected prefix mismatch error")
	}
}

func TestRegisterAppTablesRejectsReservedPrefix(t *testing.T) {
	// A random app cannot create tables in a reserved domain's prefix. The
	// kernel ledger pre-claims presales/commerce, and even without that the
	// prefix rule rejects any table that is not the app's own namespace.
	err := RegisterAppTables("intruderapp", []string{
		`CREATE TABLE IF NOT EXISTS presales_opportunities (id TEXT PRIMARY KEY)`,
	})
	if err == nil {
		t.Fatal("expected error: intruderapp may not create presales_ tables")
	}
	err = RegisterAppTables("intruderapp", []string{
		`CREATE TABLE IF NOT EXISTS commerce_products (id TEXT PRIMARY KEY)`,
	})
	if err == nil {
		t.Fatal("expected error: intruderapp may not create commerce_ tables")
	}
	// But the app can still own its own prefix.
	if err := RegisterAppTables("intruderapp", []string{
		`CREATE TABLE IF NOT EXISTS intruderapp_items (id TEXT PRIMARY KEY)`,
	}); err != nil {
		t.Fatalf("own prefix should succeed: %v", err)
	}
}

func TestReservedNamespaceCannotBeReclaimed(t *testing.T) {
	// StartupGate/RegisterKernelLedger pre-claim presales and commerce. A
	// different owner cannot take them over.
	reg := Default()
	if err := RegisterKernelLedger(reg); err != nil {
		t.Fatalf("ledger: %v", err)
	}
	if err := reg.RegisterNamespaceOwner(NamespacePresales, "intruderapp"); !IsCode(err, CodeNamespaceOwned) {
		t.Fatalf("want namespace_owned, got %v", err)
	}
}

func TestValidateAppTables(t *testing.T) {
	appID := "checkedapp"
	ddls := []string{`CREATE TABLE IF NOT EXISTS checkedapp_rows (id TEXT PRIMARY KEY)`}
	if err := RegisterAppTables(appID, ddls); err != nil {
		t.Fatalf("RegisterAppTables: %v", err)
	}
	// Declared tables that are registered and prefixed pass.
	if err := ValidateAppTables(appID, []string{"checkedapp_rows"}); err != nil {
		t.Fatalf("ValidateAppTables: %v", err)
	}
	// Empty declaration passes trivially.
	if err := ValidateAppTables(appID, nil); err != nil {
		t.Fatalf("empty declaration: %v", err)
	}
	// Wrong prefix is rejected.
	if err := ValidateAppTables(appID, []string{"other_rows"}); !IsCode(err, CodeInvalidDeclaration) {
		t.Fatalf("want invalid_declaration, got %v", err)
	}
	// Unregistered table is rejected.
	if err := ValidateAppTables(appID, []string{"checkedapp_ghost"}); !IsCode(err, CodeUnownedTable) {
		t.Fatalf("want unowned_table, got %v", err)
	}
	// Table owned by a different namespace is rejected.
	if err := RegisterAppTables("secondapp", []string{`CREATE TABLE IF NOT EXISTS secondapp_rows (id TEXT)`}); err != nil {
		t.Fatal(err)
	}
	if err := ValidateAppTables(appID, []string{"secondapp_rows"}); err == nil {
		t.Fatal("expected error declaring another app's table")
	}
}

func TestRegisterAppTablesEmptyDDLs(t *testing.T) {
	// Apps without domain tables impose no namespace rules.
	if err := RegisterAppTables("dash-app", nil); err != nil {
		t.Fatalf("empty ddls should be a no-op, got %v", err)
	}
}

func TestStartupGateIdempotent(t *testing.T) {
	// StartupGate may be called repeatedly (server restarts in-process in
	// tests) without erroring on the shared Default registry.
	if err := StartupGate(nil); err != nil {
		t.Fatalf("first StartupGate: %v", err)
	}
	if err := StartupGate(nil); err != nil {
		t.Fatalf("second StartupGate: %v", err)
	}
	reg := Default()
	if owner, ok := reg.NamespaceOwner(NamespaceKernel); !ok || owner != NamespaceKernel {
		t.Fatalf("kernel namespace owner = %q,%v", owner, ok)
	}
	// Reserved app namespaces are claimed by their architecture owner.
	for _, ns := range []string{NamespacePresales, NamespaceCommerce} {
		if owner, ok := reg.NamespaceOwner(ns); !ok || owner != ns {
			t.Fatalf("namespace %q owner = %q,%v; want %q", ns, owner, ok, ns)
		}
	}
	// kernel_access_denials is owned by the kernel.
	if owner, ok := reg.TableOwner("kernel_access_denials"); !ok || owner != NamespaceKernel {
		t.Fatalf("kernel_access_denials owner = %q,%v", owner, ok)
	}
}
