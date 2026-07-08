package sources

// rest_puller.go is the generic, fully data-driven Puller for thin REST sources.
// It reads nothing but the RESTDescriptor registry (rest_catalog.go): the same
// binary serves any manifest-declared source. Two cursor strategies:
//
//   - date-window (训记): one Pull processes a single datestr and returns
//     done=false, so the Store.Sync driver loops it day-by-day from the stored
//     watermark up to today; the cursor is the last completed datestr.
//   - "" (default): a single full pull each run; bronze ETags dedup unchanged rows.
//
// It writes nothing itself — Store.Sync persists every page — matching the Puller
// contract. Auth is a static Bearer token supplied by the caller (bearer_credentials).

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

// restPullTimeout bounds a single day's request.
const restPullTimeout = 60 * time.Second

type restPuller struct {
	source  string
	baseURL string
	kinds   []RESTDescriptor
	http    *http.Client
	token   func() (string, bool) // bearer token provider; nil ⇒ no auth header

	mu      sync.Mutex
	lastReq map[string]time.Time // per-kind last request time (throttle state)
}

// NewRESTPuller builds a generic REST puller. token may be nil for unauthenticated
// sources; it is called per request so a rotated token is picked up without a rebuild.
func NewRESTPuller(source, baseURL string, kinds []RESTDescriptor, token func() (string, bool)) Puller {
	return &restPuller{
		source:  source,
		baseURL: baseURL,
		kinds:   kinds,
		http:    &http.Client{Timeout: restPullTimeout},
		token:   token,
		lastReq: map[string]time.Time{},
	}
}

func (p *restPuller) Source() string { return p.source }

// Discover yields one collection per descriptor — a thin source does not fan out.
// Gate stays "" so the driver always pulls (incremental is cursor-based).
func (p *restPuller) Discover(accountID string) ([]Collection, error) {
	out := make([]Collection, 0, len(p.kinds))
	for _, d := range p.kinds {
		out = append(out, Collection{Kind: d.Kind, ID: d.Kind})
	}
	return out, nil
}

func (p *restPuller) Pull(accountID string, c Collection, cur Cursor) ([]RawRecord, Cursor, bool, error) {
	d, ok := RESTDescriptorFor(p.source, c.Kind)
	if !ok {
		// Fall back to the puller's own list (tests construct without a registry).
		for _, k := range p.kinds {
			if k.Kind == c.Kind {
				d, ok = k, true
				break
			}
		}
	}
	if !ok {
		return nil, cur, true, fmt.Errorf("rest: no descriptor for %s/%s", p.source, c.Kind)
	}
	switch d.CursorFlavor {
	case "date-window":
		return p.pullDateWindow(d, cur)
	default:
		return p.pullSingle(d)
	}
}

// pullDateWindow processes exactly one calendar day and asks the driver to loop
// (done=false) until it catches up to today.
func (p *restPuller) pullDateWindow(d RESTDescriptor, cur Cursor) ([]RawRecord, Cursor, bool, error) {
	layout := orDefault(d.DateLayout, "2006-01-02")
	now := time.Now()
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	start := windowStart(cur, d, layout, today)
	if start.After(today) {
		return nil, cur, true, nil // caught up: nothing new to pull
	}
	p.throttle(d)
	datestr := start.Format(layout)
	raw, err := p.do(d, datestr)
	if err != nil {
		return nil, cur, true, err // stop this run; cursor unchanged ⇒ next run retries this day
	}
	if d.SuccessPath != "" && !pathIsTrue(raw, d.SuccessPath) {
		if d.TooFrequentPath != "" && pathHas(raw, d.TooFrequentPath) {
			return nil, cur, true, nil // throttled by the server: back off, retry next run
		}
		return nil, cur, true, fmt.Errorf("rest: %s %s: success!=true", p.source, datestr)
	}
	recs := p.records(d, raw)
	next := Cursor{Kind: "date-window", Value: datestr}
	done := !start.Before(today) // start == today ⇒ this was the last day
	return recs, next, done, nil
}

// pullSingle does one full request; ETags dedup unchanged rows across runs.
func (p *restPuller) pullSingle(d RESTDescriptor) ([]RawRecord, Cursor, bool, error) {
	p.throttle(d)
	raw, err := p.do(d, "")
	if err != nil {
		return nil, Cursor{}, true, err
	}
	if d.SuccessPath != "" && !pathIsTrue(raw, d.SuccessPath) {
		return nil, Cursor{}, true, fmt.Errorf("rest: %s %s: success!=true", p.source, d.Kind)
	}
	return p.records(d, raw), Cursor{Kind: d.CursorFlavor}, true, nil
}

// records extracts the item array and maps each item to a bronze RawRecord.
func (p *restPuller) records(d RESTDescriptor, raw []byte) []RawRecord {
	items, _ := extractItems(raw, d.ItemPath)
	recs := make([]RawRecord, 0, len(items))
	for _, it := range items {
		uid := fieldString(it, d.UIDField)
		if uid == "" {
			uid = hashHex(it)
		}
		recs = append(recs, RawRecord{
			Kind:        d.Kind,
			Collection:  d.Kind,
			UID:         uid,
			ETag:        hashHex(it),
			ContentType: "application/json",
			Payload:     string(it),
		})
	}
	return recs
}

// do issues one request (GET → query params, POST → JSON body) with the date
// injected per method, and the Bearer header when configured.
func (p *restPuller) do(d RESTDescriptor, datestr string) ([]byte, error) {
	method := orDefault(d.Method, http.MethodGet)
	target := d.Endpoint
	if !strings.HasPrefix(target, "http") {
		target = strings.TrimRight(p.baseURL, "/") + "/" + strings.TrimLeft(d.Endpoint, "/")
	}

	var body io.Reader
	if method == http.MethodPost {
		payload := map[string]any{}
		for k, v := range d.Body {
			payload[k] = v
		}
		if d.DateParam != "" && datestr != "" {
			payload[d.DateParam] = datestr
		}
		buf, err := json.Marshal(payload)
		if err != nil {
			return nil, err
		}
		body = bytes.NewReader(buf)
	} else {
		q := url.Values{}
		for k, v := range d.BaseParams {
			q.Set(k, v)
		}
		if d.DateParam != "" && datestr != "" {
			q.Set(d.DateParam, datestr)
		}
		if enc := q.Encode(); enc != "" {
			target += "?" + enc
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), restPullTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, method, target, body)
	if err != nil {
		return nil, err
	}
	for k, v := range d.Headers {
		req.Header.Set(k, v)
	}
	if method == http.MethodPost && req.Header.Get("Content-Type") == "" {
		req.Header.Set("Content-Type", "application/json")
	}
	if d.AuthScheme == "bearer" && p.token != nil {
		if tok, ok := p.token(); ok {
			req.Header.Set(orDefault(d.AuthHeaderName, "Authorization"), orDefault(d.AuthPrefix, "Bearer ")+tok)
		}
	}

	resp, err := p.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("rest: %s %s: %d %s", method, target, resp.StatusCode, snippet(raw))
	}
	return raw, nil
}

// throttle sleeps to keep at least MinIntervalSeconds between requests of a kind.
func (p *restPuller) throttle(d RESTDescriptor) {
	if d.MinIntervalSeconds <= 0 {
		return
	}
	gap := time.Duration(d.MinIntervalSeconds) * time.Second
	p.mu.Lock()
	last, ok := p.lastReq[d.Kind]
	now := time.Now()
	var wait time.Duration
	if ok {
		if elapsed := now.Sub(last); elapsed < gap {
			wait = gap - elapsed
		}
	}
	p.lastReq[d.Kind] = now.Add(wait)
	p.mu.Unlock()
	if wait > 0 {
		time.Sleep(wait)
	}
}

// windowStart returns the first datestr to pull: the day after the stored
// watermark, or today-LookbackDays on a first/empty cursor.
func windowStart(cur Cursor, d RESTDescriptor, layout string, today time.Time) time.Time {
	if cur.Value != "" {
		if t, err := time.ParseInLocation(layout, cur.Value, today.Location()); err == nil {
			return t.AddDate(0, 0, 1)
		}
	}
	lb := d.LookbackDays
	if lb < 0 {
		lb = 0
	}
	return today.AddDate(0, 0, -lb)
}

// ── small JSON/util helpers ─────────────────────────────────────────────────

func orDefault(s, def string) string {
	if s == "" {
		return def
	}
	return s
}

// jsonPath navigates a dotted path through nested objects, returning the raw
// value and whether the path resolved.
func jsonPath(raw []byte, path string) (json.RawMessage, bool) {
	cur := json.RawMessage(raw)
	for _, part := range strings.Split(path, ".") {
		var m map[string]json.RawMessage
		if json.Unmarshal(cur, &m) != nil {
			return nil, false
		}
		next, ok := m[part]
		if !ok {
			return nil, false
		}
		cur = next
	}
	return cur, true
}

// pathIsTrue reports whether the value at path is truthy (bool true, "true", or a
// non-zero number).
func pathIsTrue(raw []byte, path string) bool {
	v, ok := jsonPath(raw, path)
	if !ok {
		return false
	}
	var b bool
	if json.Unmarshal(v, &b) == nil {
		return b
	}
	var s string
	if json.Unmarshal(v, &s) == nil {
		return strings.EqualFold(s, "true")
	}
	var n float64
	if json.Unmarshal(v, &n) == nil {
		return n != 0
	}
	return false
}

// pathHas reports whether path resolves to a present, non-null value.
func pathHas(raw []byte, path string) bool {
	v, ok := jsonPath(raw, path)
	if !ok {
		return false
	}
	return string(v) != "null"
}
