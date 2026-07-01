package icloud

import (
	"fmt"
	"net/url"
	"strings"
)

// This file adds the incremental CardDAV surface (RFC 6578 sync-collection +
// addressbook-multiget) on top of the full-pull path in carddav.go. It lets the
// ingestion pipeline discover address books with their CTag (collection version
// gate), ask for just what changed since a sync-token, and fetch only those
// resources' bodies — instead of re-downloading every vCard on every sync, which
// is what triggers iCloud's "ck throttling".

// AddressBook is a discovered CardDAV collection: its resource URL, the CTag
// (collection version — unchanged CTag ⇒ nothing changed) and a display name.
type AddressBook struct {
	Href        string
	CTag        string
	DisplayName string
}

// SyncChange is one entry from a sync-collection report: a resource that
// changed (Deleted=false, carrying its new ETag) or was removed (Deleted=true).
type SyncChange struct {
	Href    string
	ETag    string
	Deleted bool
}

// VCardResource is one resource body fetched by multiget.
type VCardResource struct {
	Href string
	ETag string
	Data string
}

// AddressBooks discovers the account's address books with their CTags. Like
// FetchContacts it retries discovery against the .cn root on a DNS failure
// (China-region partition hosts live on icloud.com.cn); on success it pins the
// client to the resolved base so subsequent Sync/Multiget calls stay on-region.
func (c *Client) AddressBooks() ([]AddressBook, error) {
	books, err := c.listBooksFrom(c.base)
	if err != nil && isDNSFailure(err) && !strings.Contains(c.base, "icloud.com.cn") {
		c.base = strings.Replace(c.base, "icloud.com", "icloud.com.cn", 1)
		return c.listBooksFrom(c.base)
	}
	return books, err
}

func (c *Client) listBooksFrom(base string) ([]AddressBook, error) {
	principal, err := c.findPrincipal(base)
	if err != nil {
		return nil, err
	}
	home, err := c.findHomeSet(principal)
	if err != nil {
		return nil, err
	}
	const body = `<d:propfind xmlns:d="DAV:" xmlns:cs="http://calendarserver.org/ns/">` +
		`<d:prop><d:resourcetype/><d:displayname/><cs:getctag/></d:prop></d:propfind>`
	ms, respBase, err := c.do("PROPFIND", home, "1", body)
	if err != nil {
		return nil, err
	}
	var out []AddressBook
	for _, r := range ms.Responses {
		for _, ps := range r.Propstat {
			if ps.Prop.ResourceType.Addressbook != nil {
				out = append(out, AddressBook{
					Href:        resolve(respBase, r.Href),
					CTag:        ps.Prop.GetCTag,
					DisplayName: ps.Prop.DisplayName,
				})
			}
		}
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("icloud: no address books found under home set")
	}
	return out, nil
}

// SyncCollection runs a sync-collection REPORT against a book. An empty token
// requests the full set (initial seed); a stored token requests only what
// changed since. Returns the change list (with tombstones) and the new
// sync-token to persist. If a non-empty token is rejected as stale (409/412 /
// invalid-sync-token) it self-heals by re-seeding with a full sync, so callers
// never have to reason about token invalidation.
func (c *Client) SyncCollection(book, token string) (changes []SyncChange, newToken string, err error) {
	changes, newToken, err = c.syncOnce(book, token)
	if err != nil && token != "" && isInvalidSyncToken(err) {
		return c.syncOnce(book, "")
	}
	return changes, newToken, err
}

func (c *Client) syncOnce(book, token string) (changes []SyncChange, newToken string, err error) {
	body := `<d:sync-collection xmlns:d="DAV:">` +
		`<d:sync-token>` + xmlEscape(token) + `</d:sync-token>` +
		`<d:sync-level>1</d:sync-level>` +
		`<d:prop><d:getetag/></d:prop></d:sync-collection>`
	// RFC 6578: a sync-collection REPORT MUST use Depth: 0 (the traversal depth is
	// carried by <sync-level>, not the Depth header). iCloud 400s on Depth: 1.
	ms, _, err := c.do("REPORT", book, "0", body)
	if err != nil {
		return nil, "", err
	}
	// iCloud returns the collection itself as the first response entry (its own
	// getetag). It's not a member resource; naming it in a subsequent multiget
	// makes iCloud 400 the whole request, so drop the self-entry here.
	collPath := book
	if u, e := url.Parse(book); e == nil {
		collPath = u.Path
	}
	collPath = strings.TrimSuffix(collPath, "/")
	for _, r := range ms.Responses {
		href := strings.TrimSpace(r.Href)
		if href == "" || strings.TrimSuffix(href, "/") == collPath {
			continue
		}
		if isRemoved(r) {
			changes = append(changes, SyncChange{Href: href, Deleted: true})
			continue
		}
		etag := ""
		for _, ps := range r.Propstat {
			if ps.Prop.GetETag != "" {
				etag = ps.Prop.GetETag
			}
		}
		changes = append(changes, SyncChange{Href: href, ETag: etag})
	}
	return changes, ms.SyncToken, nil
}

// Multiget fetches the getetag + address-data for a set of resource hrefs in one
// addressbook-multiget REPORT. Order is not guaranteed; callers key by Href.
func (c *Client) Multiget(book string, hrefs []string) ([]VCardResource, error) {
	if len(hrefs) == 0 {
		return nil, nil
	}
	var b strings.Builder
	b.WriteString(`<c:addressbook-multiget xmlns:d="DAV:" xmlns:c="urn:ietf:params:xml:ns:carddav">`)
	b.WriteString(`<d:prop><d:getetag/><c:address-data/></d:prop>`)
	for _, h := range hrefs {
		b.WriteString(`<d:href>`)
		b.WriteString(xmlEscape(h))
		b.WriteString(`</d:href>`)
	}
	b.WriteString(`</c:addressbook-multiget>`)
	// addressbook-multiget names its targets explicitly, so Depth: 0.
	ms, _, err := c.do("REPORT", book, "0", b.String())
	if err != nil {
		return nil, err
	}
	var out []VCardResource
	for _, r := range ms.Responses {
		res := VCardResource{Href: strings.TrimSpace(r.Href)}
		for _, ps := range r.Propstat {
			if ps.Prop.GetETag != "" {
				res.ETag = ps.Prop.GetETag
			}
			if ps.Prop.AddressData != "" {
				res.Data = ps.Prop.AddressData
			}
		}
		if res.Data != "" {
			out = append(out, res)
		}
	}
	return out, nil
}

// isRemoved reports whether a sync-collection response is a tombstone (the
// resource-level status is 404, i.e. the resource no longer exists).
func isRemoved(r davResponse) bool {
	if strings.Contains(r.Status, "404") {
		return true
	}
	for _, ps := range r.Propstat {
		if strings.Contains(ps.Status, "404") {
			return true
		}
	}
	return false
}

// isInvalidSyncToken reports whether err is iCloud rejecting a stored sync-token
// as stale (HTTP 409/412, or a DAV:valid-sync-token precondition). The caller
// clears the token and re-seeds with a full sync.
func isInvalidSyncToken(err error) bool {
	if err == nil {
		return false
	}
	s := err.Error()
	return strings.Contains(s, "→ 409") || strings.Contains(s, "→ 412") ||
		strings.Contains(s, "valid-sync-token")
}

// xmlEscape escapes a value for inclusion in the request XML we build by hand.
func xmlEscape(s string) string {
	r := strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;")
	return r.Replace(s)
}
