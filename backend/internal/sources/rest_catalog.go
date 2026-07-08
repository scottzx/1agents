package sources

// rest_catalog.go is the third descriptor path (beside feishu.CollectionDescriptor
// — deeply bound to lark-cli — and the minimal CatalogItem). RESTDescriptor fully
// describes how one collection of a "thin REST source" (static Bearer token + a
// JSON endpoint, e.g. 训记) is crawled, so such a source can be added by dropping
// a manifest into ~/.1agents/connectors — zero Go code, no recompile. The generic
// restPuller (rest_puller.go) is driven entirely by this struct.
//
// Descriptors are registered at startup by the manifest loader (manifest.go); the
// registry is a process-local map, read per Pull. CatalogFor surfaces REST kinds
// as Implemented CatalogItems so the config UI / putCollection guard /
// EnsureRecurringForEnabled all work unchanged (they only know CatalogItemFor).

import "sync"

// RESTDescriptor describes how one collection of a REST source is fetched.
type RESTDescriptor struct {
	Kind   string // source_records.kind, unique within the source (e.g. "xunji_record")
	Domain string // viewer domain grouping (contacts|messages|calendar|todo|fitness|...)
	Label  string // UI display name

	Method     string            // GET | POST (default GET)
	Endpoint   string            // absolute URL, or path joined onto the source BaseURL
	BaseParams map[string]string // constant query params (GET)
	Body       map[string]any    // constant JSON body fields (POST) — typed, so bools/ints survive
	Headers    map[string]string // static request headers (non-auth)

	// Auth injection (paired with AuthScheme="bearer"; token from bearer_credentials).
	AuthScheme     string // "" | "bearer"
	AuthHeaderName string // default "Authorization"
	AuthPrefix     string // default "Bearer "

	// Response parsing.
	SuccessPath string // dotted path whose value must be true, else the page is an error ("" skips)
	ItemPath    string // dotted path to the items array (reuses extractItems, default "data.items")
	UIDField    string // per-item stable id field; "" falls back to a content hash

	// Cursor strategy.
	CursorFlavor string // "date-window" | "" (single full pull, ETag dedups)
	// date-window (训记): walk datestr from today-LookbackDays to today, one request/day.
	DateParam    string // key carrying the date (query for GET, body for POST)
	DateLayout   string // date format, default "2006-01-02"
	LookbackDays int    // days to backfill on a first/empty cursor

	// Rate limiting.
	MinIntervalSeconds int    // minimum gap between requests for this kind (guards "too frequent")
	TooFrequentPath    string // dotted path; present in the response ⇒ back off (don't advance cursor)
}

// restRegistry holds the manifest-loaded REST descriptors and per-source base URLs.
var restRegistry = struct {
	mu    sync.RWMutex
	descs map[string]map[string]RESTDescriptor // source -> kind -> desc
	bases map[string]string                    // source -> baseURL
}{
	descs: map[string]map[string]RESTDescriptor{},
	bases: map[string]string{},
}

// RegisterRESTDescriptor records one collection descriptor (and its source's base
// URL) into the process-local registry. Idempotent per (source, kind).
func RegisterRESTDescriptor(source, baseURL string, d RESTDescriptor) {
	restRegistry.mu.Lock()
	defer restRegistry.mu.Unlock()
	if restRegistry.descs[source] == nil {
		restRegistry.descs[source] = map[string]RESTDescriptor{}
	}
	restRegistry.descs[source][d.Kind] = d
	if baseURL != "" {
		restRegistry.bases[source] = baseURL
	}
}

// RESTDescriptorFor returns the descriptor for a (source, kind), or ok=false.
func RESTDescriptorFor(source, kind string) (RESTDescriptor, bool) {
	restRegistry.mu.RLock()
	defer restRegistry.mu.RUnlock()
	d, ok := restRegistry.descs[source][kind]
	return d, ok
}

// RESTKinds returns all descriptors registered for a source (nil when none).
func RESTKinds(source string) []RESTDescriptor {
	restRegistry.mu.RLock()
	defer restRegistry.mu.RUnlock()
	m := restRegistry.descs[source]
	if len(m) == 0 {
		return nil
	}
	out := make([]RESTDescriptor, 0, len(m))
	for _, d := range m {
		out = append(out, d)
	}
	return out
}

// RESTBaseURL returns the registered base URL for a REST source, or ok=false.
func RESTBaseURL(source string) (string, bool) {
	restRegistry.mu.RLock()
	defer restRegistry.mu.RUnlock()
	b, ok := restRegistry.bases[source]
	return b, ok
}

// RESTSources returns the sources that have at least one registered descriptor.
func RESTSources() []string {
	restRegistry.mu.RLock()
	defer restRegistry.mu.RUnlock()
	out := make([]string, 0, len(restRegistry.descs))
	for s := range restRegistry.descs {
		out = append(out, s)
	}
	return out
}

// restCatalogItems maps a source's REST descriptors to CatalogItems (all
// Implemented=true — a registered descriptor is by definition crawlable).
func restCatalogItems(source string) []CatalogItem {
	ds := RESTKinds(source)
	if len(ds) == 0 {
		return nil
	}
	out := make([]CatalogItem, 0, len(ds))
	for _, d := range ds {
		out = append(out, CatalogItem{Kind: d.Kind, Domain: d.Domain, Label: d.Label, Implemented: true})
	}
	return out
}
