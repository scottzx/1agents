package sources

import "github.com/scottzx/1Agents/backend/internal/icloud"

// SourceICloud is the source discriminator for Apple iCloud data.
const SourceICloud = "icloud"

// KindContact is the record kind for address-book entries (vCard).
const KindContact = "contact"

// icloudPuller pulls iCloud address books incrementally over CardDAV. It maps
// each address book to a Collection (Gate=CTag) and each sync-collection change
// to a bronze RawRecord keyed by the resource href (the source's stable id, and
// the same key sync-collection reports for deletions). The vCard body is stored
// verbatim so governance can extract any field later without re-fetching.
type icloudPuller struct {
	client *icloud.Client
}

// NewICloudPuller builds an iCloud CardDAV puller from an Apple ID +
// app-specific password (国际 discovery root; .cn fallback still applies).
func NewICloudPuller(appleID, password string) Puller {
	return &icloudPuller{client: icloud.NewClient(appleID, password)}
}

// NewICloudPullerRegion builds an iCloud puller whose discovery root is pinned
// by the account's region ("cn" → 大陆 root). Used by the multi-account data-
// source sync path where region is an explicit account property.
func NewICloudPullerRegion(region, appleID, password string) Puller {
	return &icloudPuller{client: icloud.NewClientRegion(region, appleID, password)}
}

// newICloudPullerWithBase points the puller at an explicit DAV base (tests).
func newICloudPullerWithBase(base, appleID, password string) Puller {
	return &icloudPuller{client: icloud.NewClientWithBase(base, appleID, password)}
}

func (p *icloudPuller) Source() string { return SourceICloud }

func (p *icloudPuller) Discover(accountID string) ([]Collection, error) {
	books, err := p.client.AddressBooks()
	if err != nil {
		return nil, err
	}
	out := make([]Collection, 0, len(books))
	for _, b := range books {
		out = append(out, Collection{Kind: KindContact, ID: b.Href, Gate: b.CTag})
	}
	return out, nil
}

func (p *icloudPuller) Pull(accountID string, c Collection, cur Cursor) ([]RawRecord, Cursor, bool, error) {
	changes, newToken, err := p.client.SyncCollection(c.ID, cur.Value)
	if err != nil {
		return nil, cur, true, err
	}
	var hrefs []string
	var recs []RawRecord
	for _, ch := range changes {
		if ch.Deleted {
			recs = append(recs, RawRecord{Kind: KindContact, Collection: c.ID, UID: ch.Href, Deleted: true})
			continue
		}
		hrefs = append(hrefs, ch.Href)
	}
	// Fetch bodies in bounded batches: a multiget naming ~1200 hrefs in one
	// request body is rejected by iCloud (400). Chunking keeps each REPORT small.
	for _, batch := range chunk(hrefs, multigetBatch) {
		res, err := p.client.Multiget(c.ID, batch)
		if err != nil {
			return nil, cur, true, err
		}
		for _, r := range res {
			recs = append(recs, RawRecord{
				Kind: KindContact, Collection: c.ID, UID: r.Href, ETag: r.ETag,
				ContentType: "text/vcard", Payload: r.Data,
			})
		}
	}
	// iCloud returns the whole delta in one sync-collection response, so a book
	// is a single page; done=true after this call.
	return recs, Cursor{Kind: "sync_token", Value: newToken}, true, nil
}

// multigetBatch bounds how many resource hrefs go into a single
// addressbook-multiget REPORT (iCloud rejects an overly large request body).
const multigetBatch = 100

func chunk(s []string, n int) [][]string {
	var out [][]string
	for i := 0; i < len(s); i += n {
		end := i + n
		if end > len(s) {
			end = len(s)
		}
		out = append(out, s[i:end])
	}
	return out
}
