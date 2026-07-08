package ingest

import "testing"

// TestListTemplates verifies the embedded connector + governance templates are
// discoverable (go:embed wired), each with a kind and an id the install endpoint
// can resolve.
func TestListTemplates(t *testing.T) {
	list, err := listTemplates()
	if err != nil {
		t.Fatalf("listTemplates: %v", err)
	}
	if len(list) == 0 {
		t.Fatal("no embedded templates found — go:embed not wired?")
	}
	var haveConnector, haveGovernance bool
	for _, ti := range list {
		if ti.ID == "" || (ti.Kind != "connector" && ti.Kind != "governance") {
			t.Fatalf("malformed template: %+v", ti)
		}
		switch ti.Kind {
		case "connector":
			haveConnector = true
			if ti.Vendor == "" {
				t.Fatalf("connector template missing vendor: %+v", ti)
			}
		case "governance":
			haveGovernance = true
		}
	}
	if !haveConnector || !haveGovernance {
		t.Fatalf("expected both connector + governance templates, got %+v", list)
	}
}
