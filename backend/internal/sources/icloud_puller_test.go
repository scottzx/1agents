package sources

import (
	"database/sql"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	_ "modernc.org/sqlite"
)

// fakeDAV is a minimal in-memory CardDAV server: it answers the discovery
// PROPFINDs, a sync-collection REPORT (full when the request token is empty,
// delta otherwise), and an addressbook-multiget REPORT. The test mutates its
// fields between sync runs to drive full/skip/incremental/delete scenarios and
// counts REPORTs to prove the CTag gate skips untouched collections.
type fakeDAV struct {
	ctag  string
	token string // sync-token to hand back
	full  []davRes
	delta []davRes // returned when the request carries a non-empty token

	syncReports int
	multigets   int
}

type davRes struct {
	href    string
	etag    string
	vcard   string
	deleted bool
}

var syncTokenRe = regexp.MustCompile(`sync-token>([^<]*)<`)

func (f *fakeDAV) handler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		buf := make([]byte, r.ContentLength)
		_, _ = r.Body.Read(buf)
		body := string(buf)
		w.Header().Set("Content-Type", "application/xml; charset=utf-8")
		switch r.Method {
		case "PROPFIND":
			switch {
			case strings.Contains(body, "current-user-principal"):
				_, _ = w.Write([]byte(`<multistatus xmlns="DAV:"><response><href>/principal/</href>` +
					`<propstat><prop><current-user-principal><href>/principal/</href></current-user-principal></prop>` +
					`<status>HTTP/1.1 200 OK</status></propstat></response></multistatus>`))
			case strings.Contains(body, "addressbook-home-set"):
				_, _ = w.Write([]byte(`<multistatus xmlns="DAV:" xmlns:c="urn:ietf:params:xml:ns:carddav">` +
					`<response><href>/principal/</href><propstat><prop>` +
					`<c:addressbook-home-set><href>/home/</href></c:addressbook-home-set>` +
					`</prop></propstat></response></multistatus>`))
			default: // resourcetype + getctag: list address books
				_, _ = w.Write([]byte(`<multistatus xmlns="DAV:" xmlns:cs="http://calendarserver.org/ns/">` +
					`<response><href>/home/book1/</href><propstat><prop>` +
					`<resourcetype><collection/><addressbook xmlns="urn:ietf:params:xml:ns:carddav"/></resourcetype>` +
					`<displayname>Contacts</displayname><cs:getctag>` + f.ctag + `</cs:getctag>` +
					`</prop></propstat></response></multistatus>`))
			}
		case "REPORT":
			if strings.Contains(body, "sync-collection") {
				f.syncReports++
				reqToken := ""
				if m := syncTokenRe.FindStringSubmatch(body); m != nil {
					reqToken = strings.TrimSpace(m[1])
				}
				set := f.full
				if reqToken != "" {
					set = f.delta
				}
				var b strings.Builder
				b.WriteString(`<multistatus xmlns="DAV:">`)
				for _, res := range set {
					b.WriteString(`<response><href>` + res.href + `</href>`)
					if res.deleted {
						b.WriteString(`<status>HTTP/1.1 404 Not Found</status>`)
					} else {
						b.WriteString(`<propstat><prop><getetag>` + res.etag + `</getetag></prop>` +
							`<status>HTTP/1.1 200 OK</status></propstat>`)
					}
					b.WriteString(`</response>`)
				}
				b.WriteString(`<sync-token>` + f.token + `</sync-token></multistatus>`)
				_, _ = w.Write([]byte(b.String()))
				return
			}
			// addressbook-multiget: return bodies for the requested resource hrefs.
			f.multigets++
			all := map[string]davRes{}
			for _, res := range append(append([]davRes{}, f.full...), f.delta...) {
				if !res.deleted {
					all[res.href] = res
				}
			}
			var b strings.Builder
			b.WriteString(`<multistatus xmlns="DAV:" xmlns:c="urn:ietf:params:xml:ns:carddav">`)
			for href, res := range all {
				if !strings.Contains(body, href) {
					continue
				}
				b.WriteString(`<response><href>` + href + `</href><propstat><prop>` +
					`<getetag>` + res.etag + `</getetag>` +
					`<c:address-data>` + res.vcard + `</c:address-data>` +
					`</prop><status>HTTP/1.1 200 OK</status></propstat></response>`)
			}
			b.WriteString(`</multistatus>`)
			_, _ = w.Write([]byte(b.String()))
		default:
			http.Error(w, "unexpected method "+r.Method, http.StatusMethodNotAllowed)
		}
	}
}

func vcard(uid, fn, tel string) string {
	return "BEGIN:VCARD\nVERSION:3.0\nUID:" + uid + "\nFN:" + fn + "\nTEL:" + tel + "\nEND:VCARD"
}

func openSourcesStore(t *testing.T) *Store {
	t.Helper()
	path := filepath.Join(t.TempDir(), "sync.db")
	dsn := "file:" + url.PathEscape(path) + "?_pragma=busy_timeout(5000)"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		t.Fatalf("open sync.db: %v", err)
	}
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { db.Close() })
	st, err := NewStore(db)
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	return st
}

const book = "/home/book1/"

// TestICloudDriverBronze exercises the pull half of Phase 1 against a fake DAV
// server: full seed → unchanged (CTag skip) → incremental change → deletion.
// It asserts bronze/cursor/gate state and — crucially — that an unchanged
// collection costs zero REPORTs and a change re-fetches only the changed vCard.
// The governance-window semantics (RecordsSince) are checked here too so the
// gold layer's "process only what changed" property is proven at the store.
func TestICloudDriverBronze(t *testing.T) {
	dav := &fakeDAV{
		ctag:  "ctag-1",
		token: "token-1",
		full: []davRes{
			{href: book + "a.vcf", etag: `"ea1"`, vcard: vcard("A", "Alice", "13800000001")},
			{href: book + "b.vcf", etag: `"eb1"`, vcard: vcard("B", "Bob", "13800000002")},
		},
	}
	srv := httptest.NewServer(dav.handler())
	defer srv.Close()

	st := openSourcesStore(t)
	puller := newICloudPullerWithBase(srv.URL, "apple@id", "pw")

	// ── 1) Full seed ──
	stats, err := st.Sync(puller, "default")
	if err != nil {
		t.Fatalf("sync 1: %v", err)
	}
	if stats.Changed != 2 || dav.syncReports != 1 || dav.multigets != 1 {
		t.Fatalf("seed: changed=%d syncReports=%d multigets=%d", stats.Changed, dav.syncReports, dav.multigets)
	}
	// Discovery resolves the book href to an absolute URL, so that's the stored
	// collection id (the relative href is only the per-resource UID).
	collID := srv.URL + book
	cur, gate, ok, err := st.LoadCursor(SourceICloud, "default", KindContact, collID)
	if err != nil || !ok || cur.Value != "token-1" || gate != "ctag-1" {
		t.Fatalf("cursor after seed: cur=%+v gate=%q ok=%v err=%v", cur, gate, ok, err)
	}
	seed, seedMax, err := st.RecordsSince(SourceICloud, KindContact, 0)
	if err != nil || len(seed) != 2 {
		t.Fatalf("bronze after seed: n=%d err=%v", len(seed), err)
	}

	// ── 2) Unchanged: same CTag → collection skipped, no REPORT ──
	stats, err = st.Sync(puller, "default")
	if err != nil {
		t.Fatalf("sync 2: %v", err)
	}
	if stats.Skipped != 1 || dav.syncReports != 1 || dav.multigets != 1 {
		t.Fatalf("skip: skipped=%d syncReports=%d multigets=%d", stats.Skipped, dav.syncReports, dav.multigets)
	}
	if fresh, _, _ := st.RecordsSince(SourceICloud, KindContact, seedMax); len(fresh) != 0 {
		t.Fatalf("unchanged sync produced %d fresh bronze rows, want 0", len(fresh))
	}

	// ── 3) Incremental: CTag bumps, one vCard changed (new etag) ──
	time.Sleep(2 * time.Millisecond) // ensure fetched_at strictly advances past seedMax
	dav.ctag = "ctag-2"
	dav.token = "token-2"
	dav.delta = []davRes{{href: book + "a.vcf", etag: `"ea2"`, vcard: vcard("A", "Alice Smith", "13800000001")}}
	stats, err = st.Sync(puller, "default")
	if err != nil {
		t.Fatalf("sync 3: %v", err)
	}
	if stats.Changed != 1 || dav.syncReports != 2 || dav.multigets != 2 {
		t.Fatalf("incremental: changed=%d syncReports=%d multigets=%d", stats.Changed, dav.syncReports, dav.multigets)
	}
	fresh, _, err := st.RecordsSince(SourceICloud, KindContact, seedMax)
	if err != nil || len(fresh) != 1 || fresh[0].UID != book+"a.vcf" || !strings.Contains(fresh[0].Payload, "Alice Smith") {
		t.Fatalf("incremental bronze window: %+v err=%v", fresh, err)
	}

	// ── 4) Deletion tombstone lands in bronze ──
	dav.ctag = "ctag-3"
	dav.token = "token-3"
	dav.delta = []davRes{{href: book + "b.vcf", deleted: true}}
	if _, err = st.Sync(puller, "default"); err != nil {
		t.Fatalf("sync 4: %v", err)
	}
	recs, _, err := st.RecordsSince(SourceICloud, KindContact, 0)
	if err != nil {
		t.Fatalf("records: %v", err)
	}
	tomb := false
	for _, r := range recs {
		if r.UID == book+"b.vcf" && r.Deleted {
			tomb = true
		}
	}
	if !tomb {
		t.Fatalf("expected tombstone for b.vcf in bronze; got %+v", recs)
	}
}
