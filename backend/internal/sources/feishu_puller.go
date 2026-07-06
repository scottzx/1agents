package sources

import (
	"context"
	"encoding/json"
	"fmt"
	"hash/fnv"
	"strconv"
	"strings"
	"time"

	"github.com/scottzx/1Agents/backend/internal/feishu"
)

// feishuPuller drives the Feishu ingestion catalog (feishu.Catalog) into bronze.
// Unlike the iCloud puller — which discovers collections from the source — the
// Feishu puller is handed the exact set of collections to crawl (FeishuSpec),
// computed by the caller from source_collection_config + the tracked-chat set,
// so the puller itself stays free of the meta.db config dependency. Every item
// is stored verbatim as application/json; governance shapes it later.
type feishuPuller struct {
	client *feishu.Client
	specs  []FeishuSpec
}

// FeishuSpec is one enabled kind the caller wants crawled. PageSize and
// LookbackDays come from source_collection_config; ChatIDs is the tracked-chat
// set for PerChat kinds (feishu_message / feishu_chat_member); CalendarIDs is the
// calendar set for PerCalendar kinds (feishu_calendar_event). Both are ignored by
// kinds that don't fan out.
type FeishuSpec struct {
	Kind         string
	PageSize     int
	LookbackDays int
	ChatIDs      []string
	CalendarIDs  []string
}

// NewFeishuPuller builds a Feishu puller over client for the given specs.
func NewFeishuPuller(client *feishu.Client, specs []FeishuSpec) Puller {
	return &feishuPuller{client: client, specs: specs}
}

func (p *feishuPuller) Source() string { return feishu.Source }

// Discover turns each spec into one or more collections. PerChat kinds fan out
// across their ChatIDs (one collection per chat); other kinds are a single
// collection keyed by the kind itself. Gate stays "" — Feishu exposes no
// collection version, so the driver always pulls (incremental is cursor-based).
func (p *feishuPuller) Discover(accountID string) ([]Collection, error) {
	var out []Collection
	for _, s := range p.specs {
		d := feishu.DescriptorFor(s.Kind)
		if d == nil || !d.Implemented {
			continue
		}
		if d.PerChat {
			for _, chatID := range s.ChatIDs {
				out = append(out, Collection{Kind: s.Kind, ID: chatID})
			}
			continue
		}
		if d.PerCalendar {
			for _, calID := range s.CalendarIDs {
				out = append(out, Collection{Kind: s.Kind, ID: calID})
			}
			continue
		}
		out = append(out, Collection{Kind: s.Kind, ID: s.Kind})
	}
	return out, nil
}

// pullTimeout bounds a single collection crawl (lark-cli --page-all can loop
// many pages on a large chat).
const pullTimeout = 2 * time.Minute

func (p *feishuPuller) Pull(accountID string, c Collection, cur Cursor) ([]RawRecord, Cursor, bool, error) {
	d := feishu.DescriptorFor(c.Kind)
	if d == nil {
		return nil, cur, true, fmt.Errorf("feishu: no descriptor for kind %q", c.Kind)
	}
	spec := p.specFor(c.Kind)

	params := map[string]string{}
	for k, v := range d.BaseParams {
		params[k] = v
	}
	if spec.PageSize > 0 {
		params["page_size"] = strconv.Itoa(spec.PageSize)
	}
	// Fan-out kinds carry their parent id either as a container_id param (messages)
	// or embedded in the path (group members: /im/v1/chats/{chat_id}/members;
	// calendar events: /calendar/v4/calendars/{calendar_id}/events). c.ID is the
	// fanned-out chat/calendar id in all cases.
	endpoint := d.Endpoint
	if d.PerChat || d.PerCalendar {
		switch {
		case strings.Contains(endpoint, "{chat_id}"):
			endpoint = strings.ReplaceAll(endpoint, "{chat_id}", c.ID)
		case strings.Contains(endpoint, "{calendar_id}"):
			endpoint = strings.ReplaceAll(endpoint, "{calendar_id}", c.ID)
		default:
			params["container_id"] = c.ID
		}
	}

	// timestamp flavor: bound the lower edge by the stored watermark (seconds),
	// or now-lookback on a first crawl.
	if d.CursorFlavor == "timestamp" && d.TimeParam != "" {
		startSec := p.watermarkStart(cur, spec)
		if startSec > 0 {
			params[d.TimeParam] = strconv.FormatInt(startSec, 10)
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), pullTimeout)
	defer cancel()
	out, err := p.client.RawAPI(ctx, d.Method, endpoint, params, true)
	if err != nil {
		return nil, cur, true, err
	}
	if err := checkAPICode(out); err != nil {
		return nil, cur, true, fmt.Errorf("feishu: %s: %w", c.Kind, err)
	}

	items, err := extractItems(out, d.ItemPath)
	if err != nil {
		return nil, cur, true, fmt.Errorf("feishu: %s: parse items: %w", c.Kind, err)
	}

	recs := make([]RawRecord, 0, len(items))
	var maxTS int64
	for _, item := range items {
		uid := fieldString(item, d.UIDField)
		if uid == "" {
			uid = hashHex(item) // UID-less kinds fall back to a content hash
		}
		recs = append(recs, RawRecord{
			Kind:        c.Kind,
			Collection:  c.ID,
			UID:         uid,
			ETag:        hashHex(item), // content hash → CommitPage skips unchanged rows
			ContentType: "application/json",
			Payload:     string(item),
		})
		if d.CursorFlavor == "timestamp" && d.TimeItemField != "" {
			if ts := itemEpochSec(item, d.TimeItemField, d.TimeMs); ts > maxTS {
				maxTS = ts
			}
		}
	}

	// RawAPI aggregates every page, so a collection is a single page: done=true.
	next := cur
	switch d.CursorFlavor {
	case "timestamp":
		wm := cur.Value
		if maxTS > 0 {
			// Advance to the newest seen; inclusive boundary, dedup handles overlap.
			if prev, _ := strconv.ParseInt(cur.Value, 10, 64); maxTS >= prev {
				wm = strconv.FormatInt(maxTS, 10)
			}
		}
		next = Cursor{Kind: "timestamp", Value: wm}
	default: // page_token / none: full re-crawl each run; ETag dedups. No cursor.
		next = Cursor{Kind: d.CursorFlavor}
	}
	return recs, next, true, nil
}

func (p *feishuPuller) specFor(kind string) FeishuSpec {
	for _, s := range p.specs {
		if s.Kind == kind {
			return s
		}
	}
	return FeishuSpec{Kind: kind}
}

// watermarkStart returns the lower-bound epoch-seconds for a timestamp crawl:
// the stored watermark, or now-lookback on a first crawl.
func (p *feishuPuller) watermarkStart(cur Cursor, spec FeishuSpec) int64 {
	if cur.Value != "" {
		if v, err := strconv.ParseInt(cur.Value, 10, 64); err == nil {
			return v
		}
	}
	days := spec.LookbackDays
	if days <= 0 {
		return 0 // 0 → no lower bound (full history)
	}
	return time.Now().Add(-time.Duration(days) * 24 * time.Hour).Unix()
}

// ── generic JSON helpers ────────────────────────────────────────────────────

// checkAPICode fails when the lark-cli envelope reports a non-zero API code.
func checkAPICode(raw []byte) error {
	var env struct {
		Code int    `json:"code"`
		Msg  string `json:"msg"`
	}
	if err := json.Unmarshal(raw, &env); err != nil {
		return nil // not the standard envelope; leave item parsing to report issues
	}
	if env.Code != 0 {
		return fmt.Errorf("api code=%d msg=%q", env.Code, env.Msg)
	}
	return nil
}

// extractItems navigates a dotted path (e.g. "data.items") to the items array.
// A missing path yields no items (not an error): some responses legitimately
// omit the array when empty.
func extractItems(raw []byte, path string) ([]json.RawMessage, error) {
	if path == "" {
		path = "data.items"
	}
	cur := json.RawMessage(raw)
	for _, part := range strings.Split(path, ".") {
		var m map[string]json.RawMessage
		if err := json.Unmarshal(cur, &m); err != nil {
			return nil, err
		}
		next, ok := m[part]
		if !ok {
			return nil, nil
		}
		cur = next
	}
	var items []json.RawMessage
	if err := json.Unmarshal(cur, &items); err != nil {
		return nil, err
	}
	return items, nil
}

// fieldString reads a top-level string field from an item, tolerating numeric
// JSON by returning its literal text.
func fieldString(item json.RawMessage, field string) string {
	if field == "" {
		return ""
	}
	var m map[string]json.RawMessage
	if err := json.Unmarshal(item, &m); err != nil {
		return ""
	}
	v, ok := m[field]
	if !ok {
		return ""
	}
	var s string
	if json.Unmarshal(v, &s) == nil {
		return s
	}
	return strings.Trim(string(v), `"`)
}

// itemEpochSec reads TimeItemField from item and returns it as epoch seconds,
// converting from ms when ms is true.
func itemEpochSec(item json.RawMessage, field string, ms bool) int64 {
	raw := fieldString(item, field)
	if raw == "" {
		return 0
	}
	v, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		return 0
	}
	if ms {
		return v / 1000
	}
	return v
}

// hashHex is a stable FNV-1a hex digest of b, used as an ETag / fallback UID.
func hashHex(b []byte) string {
	h := fnv.New64a()
	_, _ = h.Write(b)
	return strconv.FormatUint(h.Sum64(), 16)
}
