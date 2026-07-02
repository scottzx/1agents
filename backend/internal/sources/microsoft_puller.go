package sources

// microsoft_puller.go is the framework skeleton for the Microsoft (Graph) data
// source. It plugs into the existing bronze pipeline (Store.Sync → Discover →
// Pull → CommitPage) so the account/region model, config UI, and work-order
// scheduling are exercised end-to-end today. The real Graph pull (OAuth token +
// /me/contacts, /me/events, /me/messages delta) is deferred: Pull is a no-op
// that returns an empty page, so a sync run reports 0 changes honestly instead
// of failing. See catalog.go for the roadmap kinds.

// Microsoft Graph base endpoints per region. The 大陆 (21Vianet / 世纪互联)
// tenant is a physically separate cloud with its own host — pinned here so the
// real pull picks the right endpoint off the account's region.
const (
	graphBaseIntl = "https://graph.microsoft.com/v1.0"
	graphBaseCN   = "https://microsoftgraph.chinacloudapi.cn/v1.0"
)

// microsoftPuller pulls Microsoft Graph collections for one account. base is
// resolved from the account region at construction.
type microsoftPuller struct {
	base  string
	kinds []string // enabled kinds to surface as collections
}

// NewMicrosoftPuller builds a Graph puller for the given region and enabled
// kinds. region selects the Graph base (国际 vs 世纪互联).
func NewMicrosoftPuller(region string, kinds []string) Puller {
	base := graphBaseIntl
	if region == RegionCN {
		base = graphBaseCN
	}
	return &microsoftPuller{base: base, kinds: kinds}
}

func (p *microsoftPuller) Source() string { return VendorMicrosoft }

func (p *microsoftPuller) Discover(accountID string) ([]Collection, error) {
	out := make([]Collection, 0, len(p.kinds))
	for _, k := range p.kinds {
		// One collection per kind; no CTag gate yet (Graph uses delta tokens).
		out = append(out, Collection{Kind: k, ID: k})
	}
	return out, nil
}

func (p *microsoftPuller) Pull(accountID string, c Collection, cur Cursor) ([]RawRecord, Cursor, bool, error) {
	// TODO: real Graph delta — GET {base}/me/{contacts|events|messages}/delta with
	// the OAuth token, page through @odata.nextLink, persist @odata.deltaLink as a
	// "delta_link" cursor. Framework skeleton: empty page, done.
	return nil, cur, true, nil
}
