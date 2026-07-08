package sources

// cli_puller.go is the generic, manifest-driven Puller for sources whose data is
// fetched by shelling out to a local CLI (agently-cli, lark-cli, …) rather than an
// HTTP call. It is the exec twin of rest_puller.go: the response parsing is
// identical (SuccessPath / ItemPath / UIDField, shared helpers), only the fetch
// differs. Auth is external — the CLI holds the credential — so no token is stored
// here; the user runs the CLI's own login once on the host.
//
// The one cursor flavor CLI sources need is "timestamp": the watermark is the max
// TimeItemField (e.g. created_at, RFC3339) seen, passed back next run via CursorArg
// (e.g. --after) so the CLI returns only newer rows. Bronze ETags drop the
// inclusive-overlap row, so nothing is missed or duplicated.

import (
	"context"
	"fmt"
	"os/exec"
	"time"
)

// cliPullTimeout bounds a single CLI invocation.
const cliPullTimeout = 90 * time.Second

type cliPuller struct {
	source string
	kinds  []RESTDescriptor
	// run is the exec seam (production shells out; tests feed a canned envelope).
	run func(ctx context.Context, d RESTDescriptor, cursor string) ([]byte, error)
}

// NewCLIPuller builds a generic CLI puller over the given descriptors.
func NewCLIPuller(source string, kinds []RESTDescriptor) Puller {
	p := &cliPuller{source: source, kinds: kinds}
	p.run = p.exec
	return p
}

func (p *cliPuller) Source() string { return p.source }

// Discover yields one collection per descriptor (CLI sources don't fan out).
func (p *cliPuller) Discover(accountID string) ([]Collection, error) {
	out := make([]Collection, 0, len(p.kinds))
	for _, d := range p.kinds {
		out = append(out, Collection{Kind: d.Kind, ID: d.Kind})
	}
	return out, nil
}

func (p *cliPuller) Pull(accountID string, c Collection, cur Cursor) ([]RawRecord, Cursor, bool, error) {
	d, ok := descriptorFor(p.source, c.Kind, p.kinds)
	if !ok {
		return nil, cur, true, fmt.Errorf("cli: no descriptor for %s/%s", p.source, c.Kind)
	}
	out, err := p.run(context.Background(), d, cur.Value)
	if err != nil {
		return nil, cur, true, err
	}
	if d.SuccessPath != "" && !pathIsTrue(out, d.SuccessPath) {
		return nil, cur, true, fmt.Errorf("cli: %s %s: %s!=true", p.source, d.Kind, d.SuccessPath)
	}
	items, _ := extractItems(out, d.ItemPath)
	recs := make([]RawRecord, 0, len(items))
	watermark := cur.Value
	for _, it := range items {
		uid := fieldString(it, d.UIDField)
		if uid == "" {
			uid = hashHex(it)
		}
		recs = append(recs, RawRecord{
			Kind: d.Kind, Collection: d.Kind, UID: uid,
			ETag: hashHex(it), ContentType: "application/json", Payload: string(it),
		})
		if d.TimeItemField != "" {
			if ts := fieldString(it, d.TimeItemField); ts > watermark {
				watermark = ts // RFC3339 sorts lexically == chronologically
			}
		}
	}
	next := cur
	if d.CursorFlavor == "timestamp" {
		next = Cursor{Kind: "timestamp", Value: watermark}
	}
	return recs, next, true, nil
}

// exec runs the descriptor's command, appending the cursor arg when set.
func (p *cliPuller) exec(ctx context.Context, d RESTDescriptor, cursor string) ([]byte, error) {
	if d.Command == "" {
		return nil, fmt.Errorf("cli: %s %s: no command", p.source, d.Kind)
	}
	ctx, cancel := context.WithTimeout(ctx, cliPullTimeout)
	defer cancel()
	args := append([]string(nil), d.Args...)
	if d.CursorArg != "" && cursor != "" {
		args = append(args, d.CursorArg, cursor)
	}
	out, err := exec.CommandContext(ctx, d.Command, args...).Output()
	if err != nil {
		return nil, fmt.Errorf("cli: %s %v: %w", d.Command, args, err)
	}
	return out, nil
}

// descriptorFor finds a kind's descriptor in the registry, falling back to the
// puller's own list (tests construct without a registry).
func descriptorFor(source, kind string, list []RESTDescriptor) (RESTDescriptor, bool) {
	if d, ok := RESTDescriptorFor(source, kind); ok {
		return d, true
	}
	for _, k := range list {
		if k.Kind == kind {
			return k, true
		}
	}
	return RESTDescriptor{}, false
}
