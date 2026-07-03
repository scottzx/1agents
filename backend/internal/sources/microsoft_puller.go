package sources

// microsoft_puller.go pulls Microsoft Graph collections for one account into the
// bronze layer via the shared Store.Sync driver. Authorization is the real
// OAuth flow (microsoft_oauth.go): the puller holds an MSTokenProvider and asks
// it for a fresh access token per collection (refreshing transparently). The
// 大陆 (21Vianet) endpoint is picked off the account region at construction.
//
// Change detection uses Graph delta: each collection's cursor is the
// @odata.deltaLink returned at the end of a page walk; the next sync resumes
// from it and receives only adds/updates/tombstones. Kinds without a delta
// endpoint wired (events/todo) return an empty done page — honest no-op — so the
// account/config/scheduler path still exercises end-to-end.

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// MSTokenProvider yields a fresh (refreshed if needed) Graph access token for an
// account. *MSAuth implements it.
type MSTokenProvider interface {
	AccessToken(accountID string) (string, error)
}

// microsoftPuller pulls Microsoft Graph collections for one account.
type microsoftPuller struct {
	base   string // region Graph base (…/v1.0)
	region string
	kinds  []string
	tok    MSTokenProvider
	http   *http.Client
}

// NewMicrosoftPuller builds a Graph puller for the given region and enabled
// kinds. tok supplies the OAuth access token (nil ⇒ every Pull is a no-op, used
// by tests that only exercise Discover/Source).
func NewMicrosoftPuller(region string, kinds []string, tok MSTokenProvider) Puller {
	base := graphBaseIntl
	if region == RegionCN {
		base = graphBaseCN
	}
	return &microsoftPuller{
		base:   base,
		region: region,
		kinds:  kinds,
		tok:    tok,
		http:   &http.Client{Timeout: 60 * time.Second},
	}
}

func (p *microsoftPuller) Source() string { return VendorMicrosoft }

func (p *microsoftPuller) Discover(accountID string) ([]Collection, error) {
	out := make([]Collection, 0, len(p.kinds))
	for _, k := range p.kinds {
		// To Do is two-level: enumerate the account's task lists, one collection
		// (and thus one delta cursor) per list.
		if k == "ms_todo" {
			lists, err := p.fetchTodoLists(accountID)
			if err != nil {
				return nil, err
			}
			for _, id := range lists {
				out = append(out, Collection{Kind: k, ID: id})
			}
			continue
		}
		// One collection per kind; Graph tracks change via delta tokens, so no
		// CTag gate — the driver always calls Pull, which returns 0 changes cheaply
		// once the delta link is caught up.
		out = append(out, Collection{Kind: k, ID: k})
	}
	return out, nil
}

// firstPageEndpoint builds the Graph delta path (relative to base) for a
// collection's first page. "" means the kind has no wired pull. ms_event and
// ms_todo depend on runtime state (a date window / the list id), so this is a
// method rather than a pure table.
func (p *microsoftPuller) firstPageEndpoint(c Collection) string {
	switch c.Kind {
	case "ms_contact":
		// Default contacts folder delta. (/me/contactFolders/contacts/delta is
		// invalid — "contacts" is not a folder id; the delta segment only hangs
		// off /me/contacts or /me/contactFolders/{realId}/contacts.)
		return "/me/contacts/delta?$select=displayName,givenName,surname,emailAddresses,mobilePhone,businessPhones,companyName,jobTitle"
	case "ms_mail":
		return "/me/mailFolders/inbox/messages/delta?$select=subject,from,toRecipients,receivedDateTime,bodyPreview,webLink,isRead"
	case "ms_event":
		// Calendar delta is a calendarView (bounded window) with track-changes;
		// the deltaLink encodes the window so later syncs need no dates. Generous
		// initial window (3 years back … 1 year ahead) so a personal-data
		// aggregator captures history; delta keeps subsequent syncs cheap.
		now := time.Now().UTC()
		start := now.AddDate(-3, 0, 0).Format("2006-01-02T15:04:05Z")
		end := now.AddDate(1, 0, 0).Format("2006-01-02T15:04:05Z")
		return "/me/calendarView/delta?startDateTime=" + start + "&endDateTime=" + end +
			"&$select=subject,start,end,location,organizer,isAllDay,showAs,webLink,bodyPreview"
	case "ms_todo":
		// c.ID is the task-list id discovered in Discover. No $select: Graph
		// rejects $select on todo tasks(/delta) with 400 RequestBroker--ParseUri;
		// the full task JSON goes to bronze verbatim anyway.
		return "/me/todo/lists/" + c.ID + "/tasks/delta"
	default:
		return ""
	}
}

// fetchTodoLists returns the account's To Do list ids (one bronze collection
// each). Returns nil when there is no token (test provider).
func (p *microsoftPuller) fetchTodoLists(accountID string) ([]string, error) {
	if p.tok == nil {
		return nil, nil
	}
	token, err := p.tok.AccessToken(accountID)
	if err != nil {
		return nil, err
	}
	page, err := p.get(token, p.base+"/me/todo/lists", "")
	if err != nil {
		return nil, err
	}
	ids := make([]string, 0, len(page.Value))
	for _, raw := range page.Value {
		var l struct {
			ID string `json:"id"`
		}
		if json.Unmarshal(raw, &l) == nil && l.ID != "" {
			ids = append(ids, l.ID)
		}
	}
	return ids, nil
}

func (p *microsoftPuller) Pull(accountID string, c Collection, cur Cursor) ([]RawRecord, Cursor, bool, error) {
	ep := p.firstPageEndpoint(c)
	if ep == "" || p.tok == nil {
		// Not-yet-wired kind, or a test provider: nothing to pull, cleanly done.
		return nil, cur, true, nil
	}
	token, err := p.tok.AccessToken(accountID)
	if err != nil {
		return nil, cur, false, err
	}
	// First page → the delta endpoint; subsequent pages/syncs → the opaque
	// nextLink/deltaLink URL stored as the cursor.
	target := cur.Value
	prefer := ""
	if target == "" {
		target = p.base + ep
		if c.Kind == "ms_event" {
			// calendarView delta must be started with this Prefer header; the
			// returned nextLink/deltaLink carry the tracking so follow-ups omit it.
			prefer = "odata.track-changes"
		}
	}
	page, err := p.get(token, target, prefer)
	if err != nil {
		return nil, cur, false, err
	}
	recs := make([]RawRecord, 0, len(page.Value))
	for _, raw := range page.Value {
		rec, ok := mapGraphRecord(c.Kind, c.ID, raw)
		if ok {
			recs = append(recs, rec)
		}
	}
	// nextLink ⇒ more pages (not done); else deltaLink ⇒ collection caught up.
	if page.NextLink != "" {
		return recs, Cursor{Kind: "delta_link", Value: page.NextLink}, false, nil
	}
	return recs, Cursor{Kind: "delta_link", Value: page.DeltaLink}, true, nil
}

// graphDeltaPage is the envelope of a Graph delta response.
type graphDeltaPage struct {
	Value     []json.RawMessage `json:"value"`
	NextLink  string            `json:"@odata.nextLink"`
	DeltaLink string            `json:"@odata.deltaLink"`
}

func (p *microsoftPuller) get(token, target, prefer string) (graphDeltaPage, error) {
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, target, nil)
	if err != nil {
		return graphDeltaPage{}, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/json")
	if prefer != "" {
		req.Header.Set("Prefer", prefer)
	}
	resp, err := p.http.Do(req)
	if err != nil {
		return graphDeltaPage{}, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if resp.StatusCode >= 400 {
		return graphDeltaPage{}, fmt.Errorf("microsoft: graph %s: %d %s", target, resp.StatusCode, snippet(body))
	}
	var page graphDeltaPage
	if err := json.Unmarshal(body, &page); err != nil {
		return graphDeltaPage{}, fmt.Errorf("microsoft: decode graph page: %w", err)
	}
	return page, nil
}

// mapGraphRecord turns one Graph resource into a bronze RawRecord. A delta
// tombstone carries an "@removed" annotation and only the id.
func mapGraphRecord(kind, collection string, raw json.RawMessage) (RawRecord, bool) {
	var env struct {
		ID      string          `json:"id"`
		ETag    string          `json:"@odata.etag"`
		Removed json.RawMessage `json:"@removed"`
	}
	if err := json.Unmarshal(raw, &env); err != nil || env.ID == "" {
		return RawRecord{}, false
	}
	return RawRecord{
		Kind:        kind,
		Collection:  collection,
		UID:         env.ID,
		ETag:        env.ETag,
		ContentType: "application/json",
		Payload:     string(raw),
		Deleted:     len(env.Removed) > 0,
	}, true
}

func snippet(b []byte) string {
	const max = 300
	if len(b) > max {
		return string(b[:max]) + "…"
	}
	return string(b)
}
