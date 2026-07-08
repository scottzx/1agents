package sources

import "sync"

// vendors.go is the declarative capability table for data-source 厂家 (vendors).
// The 添加数据源 flow reads it to know, per vendor: which regions are offered
// (Apple 之前的国际/大陆问题 is fixed by making region an explicit choice here),
// whether multiple accounts are allowed (飞书 is single-account; Apple/微软/谷歌
// are multi), and how the account authenticates. Pure metadata — no I/O — so
// both the HTTP layer and (via the API) the frontend consume the same source.

// Region is a data-source partition. Fixed at account-add time and stored on
// the account (meta.SourceAccount.Region); pullers pick endpoints off it.
const (
	RegionIntl = "intl" // 国际 (icloud.com / graph.microsoft.com / larksuite.com)
	RegionCN   = "cn"   // 大陆 (icloud.com.cn / 世纪互联 21Vianet / feishu.cn)
)

// Auth kinds tell the UI how to collect credentials for a vendor.
const (
	AuthCredentials = "credentials" // Apple ID + app-specific password (Keychain)
	AuthCLI         = "cli"         // lark-cli manages the token
	AuthOAuth       = "oauth"       // OAuth authorization-code flow (骨架/占位 for now)
	AuthBearer      = "bearer"      // static Bearer token, stored per-account (manifest REST sources)
)

// VendorSpec is one vendor's add-time capability surface.
type VendorSpec struct {
	Vendor       string   `json:"vendor"`            // icloud|microsoft|google|feishu
	Label        string   `json:"label"`             // display name
	MultiAccount bool     `json:"multiAccount"`      // false = at most one account (Feishu)
	Regions      []string `json:"regions"`           // allowed regions, in display order
	AuthKind     string   `json:"authKind"`          // credentials|cli|oauth|bearer
	CliTool      string   `json:"cliTool,omitempty"` // cli authKind: the tool the 认证 zone probes (agently-cli)
}

// Vendors is the capability table. Region lists are limited by real vendor
// capability (谷歌 has no 大陆 endpoint, so intl only).
var Vendors = []VendorSpec{
	{Vendor: VendorICloud, Label: "Apple", MultiAccount: true, Regions: []string{RegionIntl, RegionCN}, AuthKind: AuthCredentials},
	{Vendor: VendorMicrosoft, Label: "Microsoft", MultiAccount: true, Regions: []string{RegionIntl, RegionCN}, AuthKind: AuthOAuth},
	{Vendor: VendorGoogle, Label: "Google", MultiAccount: true, Regions: []string{RegionIntl}, AuthKind: AuthOAuth},
	{Vendor: VendorFeishu, Label: "飞书 / Lark", MultiAccount: false, Regions: []string{RegionCN, RegionIntl}, AuthKind: AuthCLI},
	{Vendor: VendorAgentMail, Label: "Agent Mail", MultiAccount: true, Regions: []string{RegionCN}, AuthKind: AuthCLI},
}

// Vendor discriminators (must match meta.Vendor* and the bronze source column).
const (
	VendorICloud    = "icloud"
	VendorMicrosoft = "microsoft"
	VendorGoogle    = "google"
	VendorFeishu    = "feishu"
	VendorAgentMail = "agentmail" // 腾讯 Agent Mail (agently-cli manages the token)
)

// vendorsMu guards mutation of Vendors so a manifest hot-add (append) can race
// safely against readers (HandleVendors / VendorFor). Existing entries are never
// mutated in place, so a pointer from VendorFor stays valid across an append.
var vendorsMu sync.RWMutex

// appendVendor adds a vendor spec under the write lock (manifest registration).
func appendVendor(v VendorSpec) {
	vendorsMu.Lock()
	Vendors = append(Vendors, v)
	vendorsMu.Unlock()
}

// VendorsSnapshot returns a copy of the vendor table under the read lock — the
// safe way to serialize Vendors while hot-adds may be happening.
func VendorsSnapshot() []VendorSpec {
	vendorsMu.RLock()
	defer vendorsMu.RUnlock()
	return append([]VendorSpec(nil), Vendors...)
}

// VendorFor returns the spec for a vendor name, or nil when unknown.
func VendorFor(vendor string) *VendorSpec {
	vendorsMu.RLock()
	defer vendorsMu.RUnlock()
	for i := range Vendors {
		if Vendors[i].Vendor == vendor {
			return &Vendors[i]
		}
	}
	return nil
}

// RegionAllowed reports whether region is offered for vendor.
func RegionAllowed(vendor, region string) bool {
	v := VendorFor(vendor)
	if v == nil {
		return false
	}
	for _, r := range v.Regions {
		if r == region {
			return true
		}
	}
	return false
}
