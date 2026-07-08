package sources

// catalog.go holds the declarative roadmap of crawlable kinds for the sources
// that live in this package (microsoft / google). It mirrors the shape of
// feishu/catalog.go but stays minimal: the config UI needs Domain/Label/
// Implemented to render the roadmap with per-kind toggles, and the puller needs
// the kind list. Real crawl mechanics (endpoint, cursor flavor) are added when
// the Graph/Google pulls are implemented — this framework pass ships the kinds
// as documentation (Implemented=false), same convention as飞书's planned kinds.

// CatalogItem is one crawlable kind for a source's roadmap.
type CatalogItem struct {
	Kind        string `json:"kind"`
	Domain      string `json:"domain"`
	Label       string `json:"label"`
	Implemented bool   `json:"implemented"`
}

// microsoftCatalog / googleCatalog are the first-wave kind tables. Kinds are
// namespaced per vendor so bronze rows never collide across sources.
var microsoftCatalog = []CatalogItem{
	{Kind: "ms_contact", Domain: "contacts", Label: "联系人", Implemented: true},
	{Kind: "ms_event", Domain: "calendar", Label: "日历事件", Implemented: true},
	{Kind: "ms_mail", Domain: "mail", Label: "邮件", Implemented: true},
	{Kind: "ms_todo", Domain: "todo", Label: "待办", Implemented: true},
}

var googleCatalog = []CatalogItem{
	{Kind: "google_contact", Domain: "contacts", Label: "联系人", Implemented: false},
	{Kind: "google_event", Domain: "calendar", Label: "日历事件", Implemented: false},
	{Kind: "google_mail", Domain: "mail", Label: "邮件", Implemented: false},
}

// agentmailCatalog is the 腾讯 Agent Mail roadmap. Inbound mail only for now
// (the agently-cli inbox); sending/replying is out of scope for the fetch layer.
var agentmailCatalog = []CatalogItem{
	{Kind: "agentmail_mail", Domain: "mail", Label: "邮件", Implemented: true},
}

// CatalogFor returns the roadmap for a source owned by this package, or nil for
// sources with no local catalog (e.g. feishu, whose catalog lives in its own
// package, or icloud, which has none).
func CatalogFor(source string) []CatalogItem {
	switch source {
	case VendorMicrosoft:
		return append([]CatalogItem(nil), microsoftCatalog...)
	case VendorGoogle:
		return append([]CatalogItem(nil), googleCatalog...)
	case VendorAgentMail:
		return append([]CatalogItem(nil), agentmailCatalog...)
	default:
		// Manifest-loaded REST sources (e.g. 训记) surface their descriptors here,
		// so the config UI / putCollection guard / EnsureRecurringForEnabled all
		// treat them like any built-in source.
		return restCatalogItems(source)
	}
}

// CatalogItemFor returns the item for a (source, kind), or nil when unknown.
func CatalogItemFor(source, kind string) *CatalogItem {
	for _, it := range CatalogFor(source) {
		if it.Kind == kind {
			c := it
			return &c
		}
	}
	return nil
}
