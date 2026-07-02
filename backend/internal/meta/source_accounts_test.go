package meta

import (
	"path/filepath"
	"testing"
)

func openTestAccounts(t *testing.T) *SourceAccountStore {
	t.Helper()
	db, err := Open(filepath.Join(t.TempDir(), "meta.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return NewSourceAccountStore(db)
}

func TestSourceAccountsCRUD(t *testing.T) {
	s := openTestAccounts(t)

	a, err := s.Create(SourceAccount{Vendor: VendorGoogle, Region: "intl", Label: "alice@gmail.com"}, false)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if a.ID == "" || a.CreatedAt == "" {
		t.Fatalf("expected generated id + timestamp, got %+v", a)
	}

	// A second Google account is a distinct source (multi-account vendor).
	b, err := s.Create(SourceAccount{Vendor: VendorGoogle, Region: "intl", Label: "bob@gmail.com"}, false)
	if err != nil {
		t.Fatalf("create second google: %v", err)
	}
	if a.ID == b.ID {
		t.Fatalf("expected distinct ids, both %q", a.ID)
	}

	got, err := s.ListByVendor(VendorGoogle)
	if err != nil || len(got) != 2 {
		t.Fatalf("ListByVendor google = %d,%v; want 2", len(got), err)
	}

	one, ok, err := s.Get(a.ID)
	if err != nil || !ok || one.Label != "alice@gmail.com" {
		t.Fatalf("Get(%s) = %+v,%v,%v", a.ID, one, ok, err)
	}

	if err := s.Delete(a.ID); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if _, ok, _ := s.Get(a.ID); ok {
		t.Fatalf("expected %s deleted", a.ID)
	}
	if n, _ := s.CountByVendor(VendorGoogle); n != 1 {
		t.Fatalf("CountByVendor google = %d; want 1", n)
	}
}

func TestSourceAccountsSingleAccountVendor(t *testing.T) {
	s := openTestAccounts(t)

	if _, err := s.Create(SourceAccount{Vendor: VendorFeishu, Region: "cn", Label: "飞书"}, true); err != nil {
		t.Fatalf("first feishu: %v", err)
	}
	// singleAccount=true must reject a second Feishu account.
	if _, err := s.Create(SourceAccount{Vendor: VendorFeishu, Region: "cn", Label: "飞书2"}, true); err == nil {
		t.Fatalf("expected second feishu account to be rejected")
	}
}

func TestSourceAccountsDefaultRegion(t *testing.T) {
	s := openTestAccounts(t)
	a, err := s.Create(SourceAccount{Vendor: VendorICloud, Label: "me@icloud.com"}, false)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if a.Region != "intl" {
		t.Fatalf("blank region should default to intl, got %q", a.Region)
	}
	if a.Status != "active" {
		t.Fatalf("blank status should default to active, got %q", a.Status)
	}
}
