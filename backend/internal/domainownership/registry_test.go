package domainownership

import "testing"

func TestRegisterNamespaceOwnerUnique(t *testing.T) {
	reg := NewRegistry()
	if err := reg.RegisterNamespaceOwner("presales", "presales"); err != nil {
		t.Fatalf("first claim: %v", err)
	}
	// Idempotent re-claim by the same owner.
	if err := reg.RegisterNamespaceOwner("presales", "presales"); err != nil {
		t.Fatalf("idempotent re-claim: %v", err)
	}
	// A different owner cannot claim an owned namespace.
	if err := reg.RegisterNamespaceOwner("presales", "commerce"); !IsCode(err, CodeNamespaceOwned) {
		t.Fatalf("want namespace_owned, got %v", err)
	}
	if err := reg.RegisterNamespaceOwner("Bad-Ns", "x"); !IsCode(err, CodeInvalidDeclaration) {
		t.Fatalf("want invalid_declaration, got %v", err)
	}
}

func TestRegisterTableUniqueOwner(t *testing.T) {
	reg := NewRegistry()
	for _, ns := range []string{"presales", "commerce"} {
		if err := reg.RegisterNamespaceOwner(ns, ns); err != nil {
			t.Fatal(err)
		}
	}
	if err := reg.RegisterTable("presales", "presales_opportunities"); err != nil {
		t.Fatalf("register: %v", err)
	}
	if err := reg.RegisterTable("presales", "presales_opportunities"); err != nil {
		t.Fatalf("idempotent: %v", err)
	}
	// Commerce cannot even express a claim on a presales_ table: the prefix
	// rule rejects it before ownership is considered.
	if err := reg.RegisterTable("commerce", "presales_opportunities"); !IsCode(err, CodeInvalidDeclaration) {
		t.Fatalf("want invalid_declaration, got %v", err)
	}
	// Legacy (prefix-less) tables CAN collide across namespaces — the
	// registry must reject the second owner.
	if err := reg.RegisterLegacyTable("presales", "projects"); err != nil {
		t.Fatalf("legacy register: %v", err)
	}
	if err := reg.RegisterLegacyTable("commerce", "projects"); !IsCode(err, CodeDuplicateOwner) {
		t.Fatalf("want duplicate_owner, got %v", err)
	}
	// Unknown namespace cannot own tables.
	if err := reg.RegisterTable("ghost", "ghost_things"); !IsCode(err, CodeUnknownNamespace) {
		t.Fatalf("want unknown_namespace, got %v", err)
	}
}

func TestRegisterWriteAPIUniqueOwner(t *testing.T) {
	reg := NewRegistry()
	for _, ns := range []string{"kernel", "presales"} {
		if err := reg.RegisterNamespaceOwner(ns, ns); err != nil {
			t.Fatal(err)
		}
	}
	if err := reg.RegisterWriteAPI("presales", "presales.opportunity.create"); err != nil {
		t.Fatalf("register: %v", err)
	}
	if err := reg.RegisterWriteAPI("kernel", "presales.opportunity.create"); !IsCode(err, CodeDuplicateOwner) {
		t.Fatalf("want duplicate_owner, got %v", err)
	}
	if err := reg.RegisterWriteAPI("kernel", "workcase.create"); err != nil {
		t.Fatalf("kernel contract: %v", err)
	}
}

func TestCheckWriteAndRead(t *testing.T) {
	var captured []Denial
	SetDenialSink(func(d Denial) { captured = append(captured, d) })
	defer SetDenialSink(nil)

	reg := NewRegistry()
	for _, ns := range []string{"presales", "commerce"} {
		if err := reg.RegisterNamespaceOwner(ns, ns); err != nil {
			t.Fatal(err)
		}
	}
	if err := reg.RegisterTable("presales", "presales_leads"); err != nil {
		t.Fatal(err)
	}

	// Owner may write and read.
	if err := reg.CheckWrite("presales", "presales_leads"); err != nil {
		t.Fatalf("owner write: %v", err)
	}
	if err := reg.CheckRead("presales", "presales_leads"); err != nil {
		t.Fatalf("owner read: %v", err)
	}
	// Another domain may not write.
	err := reg.CheckWrite("commerce", "presales_leads")
	if !IsCode(err, CodeCrossDomainWrite) {
		t.Fatalf("want cross_domain_write, got %v", err)
	}
	// ...nor read directly (must use the owner's Query).
	err = reg.CheckRead("commerce", "presales_leads")
	if !IsCode(err, CodeCrossDomainRead) {
		t.Fatalf("want cross_domain_read, got %v", err)
	}
	// Both denials were audited.
	if len(captured) != 2 {
		t.Fatalf("captured %d denials, want 2", len(captured))
	}
	if captured[0].Action != ActionTableWrite || captured[1].Action != ActionTableRead {
		t.Fatalf("unexpected denial actions: %+v", captured)
	}
	// Unregistered tables are outside the runtime contract (legacy freedom).
	if err := reg.CheckWrite("commerce", "legacy_unregistered"); err != nil {
		t.Fatalf("unregistered table should pass runtime check: %v", err)
	}
}
