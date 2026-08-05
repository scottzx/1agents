package domainownership

import "testing"

func TestRepositoryBoundary(t *testing.T) {
	var captured []Denial
	SetDenialSink(func(d Denial) { captured = append(captured, d) })
	defer SetDenialSink(nil)

	reg := NewRegistry()
	for _, ns := range []string{"presales", "commerce"} {
		if err := reg.RegisterNamespaceOwner(ns, ns); err != nil {
			t.Fatal(err)
		}
	}
	if err := reg.RegisterRepository("presales", "opportunityrepo"); err != nil {
		t.Fatalf("register repository: %v", err)
	}
	if err := reg.RegisterRepository("presales", "opportunityrepo"); err != nil {
		t.Fatalf("idempotent register: %v", err)
	}
	if err := reg.RegisterRepository("ghost", "repo"); !IsCode(err, CodeUnknownNamespace) {
		t.Fatalf("want unknown_namespace, got %v", err)
	}

	// The owning domain may use its own repository.
	if err := reg.CheckRepositoryAccess("presales", "presales", "opportunityrepo"); err != nil {
		t.Fatalf("owner access: %v", err)
	}
	// Another domain is denied and the denial is audited.
	err := reg.CheckRepositoryAccess("commerce", "presales", "opportunityrepo")
	if !IsCode(err, CodeRepositoryAccess) {
		t.Fatalf("want repository_access, got %v", err)
	}
	if len(captured) != 1 {
		t.Fatalf("captured %d denials, want 1", len(captured))
	}
	d := captured[0]
	if d.Action != ActionRepository || d.CallerNamespace != "commerce" ||
		d.TargetNamespace != "presales" || d.Target != "opportunityrepo" {
		t.Fatalf("unexpected denial: %+v", d)
	}
}
