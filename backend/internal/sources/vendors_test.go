package sources

import "testing"

func TestVendorCapabilities(t *testing.T) {
	// Feishu is single-account; the rest are multi-account.
	if v := VendorFor(VendorFeishu); v == nil || v.MultiAccount {
		t.Fatalf("feishu should be single-account: %+v", v)
	}
	for _, name := range []string{VendorICloud, VendorMicrosoft, VendorGoogle} {
		if v := VendorFor(name); v == nil || !v.MultiAccount {
			t.Fatalf("%s should be multi-account: %+v", name, v)
		}
	}
	// Google is intl-only; Apple offers both regions.
	if RegionAllowed(VendorGoogle, RegionCN) {
		t.Fatalf("google should not allow 大陆 region")
	}
	if !RegionAllowed(VendorICloud, RegionCN) || !RegionAllowed(VendorICloud, RegionIntl) {
		t.Fatalf("apple should allow both regions")
	}
	if RegionAllowed("nope", RegionIntl) {
		t.Fatalf("unknown vendor should allow no region")
	}
}

func TestSkeletonPullersDiscoverAndPull(t *testing.T) {
	cases := []struct {
		name   string
		puller Puller
		source string
		kind   string
	}{
		// nil token provider ⇒ Pull is a no-op empty page (auth not connected).
		{"microsoft", NewMicrosoftPuller(RegionIntl, []string{"ms_contact", "ms_event"}, nil), VendorMicrosoft, "ms_contact"},
		{"microsoft-cn", NewMicrosoftPuller(RegionCN, []string{"ms_mail"}, nil), VendorMicrosoft, "ms_mail"},
		{"google", NewGooglePuller([]string{"google_contact"}), VendorGoogle, "google_contact"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if c.puller.Source() != c.source {
				t.Fatalf("Source() = %q; want %q", c.puller.Source(), c.source)
			}
			colls, err := c.puller.Discover("acct-1")
			if err != nil {
				t.Fatalf("Discover: %v", err)
			}
			if len(colls) == 0 {
				t.Fatalf("Discover returned no collections")
			}
			// Skeleton Pull is a no-op empty page that reports done.
			recs, _, done, err := c.puller.Pull("acct-1", colls[0], Cursor{})
			if err != nil {
				t.Fatalf("Pull: %v", err)
			}
			if len(recs) != 0 {
				t.Fatalf("skeleton Pull should return no records, got %d", len(recs))
			}
			if !done {
				t.Fatalf("skeleton Pull should report done")
			}
		})
	}
}

func TestCatalogFor(t *testing.T) {
	if CatalogFor(VendorMicrosoft) == nil || CatalogFor(VendorGoogle) == nil {
		t.Fatalf("microsoft/google should have catalogs")
	}
	if CatalogFor(VendorFeishu) != nil || CatalogFor(VendorICloud) != nil {
		t.Fatalf("feishu/icloud have no local catalog in this package")
	}
	if it := CatalogItemFor(VendorMicrosoft, "ms_contact"); it == nil || !it.Implemented {
		t.Fatalf("ms_contact should exist and be implemented: %+v", it)
	}
	if it := CatalogItemFor(VendorMicrosoft, "ms_event"); it == nil || it.Implemented {
		t.Fatalf("ms_event should exist and be not-yet-implemented: %+v", it)
	}
}
