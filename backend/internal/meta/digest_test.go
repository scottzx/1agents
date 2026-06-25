package meta

import "testing"

func TestDigestTemplatesCRUDAndResolution(t *testing.T) {
	db := newTestDB(t)
	s := NewDigestStore(db)

	// Seed a global default + two non-default presets.
	if err := s.UpsertTemplate(DigestTemplate{ID: "tpl-general", Name: "通用社群", Scope: "global", BodyMD: "general", Builtin: true, IsDefault: true}); err != nil {
		t.Fatalf("seed default: %v", err)
	}
	invest, err := s.CreateTemplate("投资群", "global", "investment standard", false)
	if err != nil {
		t.Fatalf("create invest: %v", err)
	}
	product, err := s.CreateTemplate("产品创业群", "global", "product standard", false)
	if err != nil {
		t.Fatalf("create product: %v", err)
	}

	all, err := s.ListTemplates()
	if err != nil || len(all) != 3 {
		t.Fatalf("list: len=%d err=%v", len(all), err)
	}

	// No binding → global default set.
	got, err := s.TemplatesForSession("oc_x")
	if err != nil || len(got) != 1 || got[0].ID != "tpl-general" {
		t.Fatalf("fallback resolution: %+v err=%v", got, err)
	}

	// Bind both invest + product → composed, default no longer applies.
	if err := s.BindTemplate("oc_x", invest.ID); err != nil {
		t.Fatalf("bind invest: %v", err)
	}
	if err := s.BindTemplate("oc_x", product.ID); err != nil {
		t.Fatalf("bind product: %v", err)
	}
	if err := s.BindTemplate("oc_x", invest.ID); err != nil { // idempotent
		t.Fatalf("bind invest again: %v", err)
	}
	got, err = s.TemplatesForSession("oc_x")
	if err != nil || len(got) != 2 {
		t.Fatalf("composed resolution: %+v err=%v", got, err)
	}
	if got[0].Name != "产品创业群" || got[1].Name != "投资群" { // ordered by name
		t.Fatalf("expected name order, got %q,%q", got[0].Name, got[1].Name)
	}

	// Hot edit takes effect immediately.
	if err := s.UpdateTemplateBody(invest.ID, "v2 investment standard"); err != nil {
		t.Fatalf("update body: %v", err)
	}
	if v, _, _ := s.GetTemplate(invest.ID); v.BodyMD != "v2 investment standard" {
		t.Fatalf("body not updated: %q", v.BodyMD)
	}

	// Unbind one → only the other remains.
	if err := s.UnbindTemplate("oc_x", product.ID); err != nil {
		t.Fatalf("unbind: %v", err)
	}
	got, _ = s.TemplatesForSession("oc_x")
	if len(got) != 1 || got[0].ID != invest.ID {
		t.Fatalf("after unbind: %+v", got)
	}

	// Delete a template clears its bindings too → back to default fallback.
	if err := s.DeleteTemplate(invest.ID); err != nil {
		t.Fatalf("delete: %v", err)
	}
	got, _ = s.TemplatesForSession("oc_x")
	if len(got) != 1 || got[0].ID != "tpl-general" {
		t.Fatalf("after delete, expected default fallback: %+v", got)
	}
	if _, ok, _ := s.GetTemplate(invest.ID); ok {
		t.Fatalf("deleted template still present")
	}
}
