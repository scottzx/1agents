package meta

import "testing"

func TestCompanyStoreCRUDAndTenantMap(t *testing.T) {
	db := newTestDB(t)
	s := NewCompanyStore(db)

	c, err := s.CreateCompany("Acme 科技有限公司", "Acme", "91310000XXXX", "note")
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if c.ShortName != "Acme" || c.FullName != "Acme 科技有限公司" {
		t.Fatalf("company fields not round-tripped: %+v", c)
	}

	list, err := s.ListCompanies()
	if err != nil || len(list) != 1 {
		t.Fatalf("list: len=%d err=%v", len(list), err)
	}

	if err := s.MapTenant("tenant-a", c.ID); err != nil {
		t.Fatalf("map tenant: %v", err)
	}
	tm, err := s.TenantCompanyMap()
	if err != nil || len(tm) != 1 {
		t.Fatalf("tenant map: len=%d err=%v", len(tm), err)
	}
	if tm[0].TenantKey != "tenant-a" || tm[0].ShortName != "Acme" {
		t.Fatalf("tenant map row wrong: %+v", tm[0])
	}

	// Re-map (INSERT OR REPLACE) moves the tenant to another company.
	c2, _ := s.CreateCompany("Beta", "Beta", "", "")
	if err := s.MapTenant("tenant-a", c2.ID); err != nil {
		t.Fatalf("remap: %v", err)
	}
	tm, _ = s.TenantCompanyMap()
	if len(tm) != 1 || tm[0].ShortName != "Beta" {
		t.Fatalf("remap not applied: %+v", tm)
	}

	if err := s.UnmapTenant("tenant-a"); err != nil {
		t.Fatalf("unmap: %v", err)
	}
	tm, _ = s.TenantCompanyMap()
	if len(tm) != 0 {
		t.Fatalf("unmap left rows: %+v", tm)
	}
}

func TestSeedFeishuOfficialIdempotent(t *testing.T) {
	db := newTestDB(t)
	s := NewCompanyStore(db)

	if err := s.SeedFeishuOfficial(); err != nil {
		t.Fatalf("seed: %v", err)
	}
	tm, err := s.TenantCompanyMap()
	if err != nil || len(tm) != 1 {
		t.Fatalf("after seed: len=%d err=%v", len(tm), err)
	}
	if tm[0].TenantKey != feishuOfficialTenant || tm[0].ShortName != "飞书官方" {
		t.Fatalf("seed mapping wrong: %+v", tm[0])
	}

	// Second seed is a no-op: same single mapping, no duplicate company.
	if err := s.SeedFeishuOfficial(); err != nil {
		t.Fatalf("re-seed: %v", err)
	}
	tm, _ = s.TenantCompanyMap()
	if len(tm) != 1 {
		t.Fatalf("re-seed duplicated mapping: %+v", tm)
	}
	companies, _ := s.ListCompanies()
	if len(companies) != 1 {
		t.Fatalf("re-seed duplicated company: %+v", companies)
	}

	// Insert-only: if the tenant is re-pointed by the user, re-seed preserves it.
	other, _ := s.CreateCompany("Other", "Other", "", "")
	if err := s.MapTenant(feishuOfficialTenant, other.ID); err != nil {
		t.Fatalf("repoint: %v", err)
	}
	if err := s.SeedFeishuOfficial(); err != nil {
		t.Fatalf("seed after repoint: %v", err)
	}
	tm, _ = s.TenantCompanyMap()
	if len(tm) != 1 || tm[0].ShortName != "Other" {
		t.Fatalf("seed clobbered user re-point: %+v", tm)
	}
}
