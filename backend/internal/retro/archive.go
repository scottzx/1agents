package retro

import (
	"fmt"

	"github.com/scottzx/1Agents/backend/internal/kwiki"
)

// Archive runs the复盘沉淀 for one archived project: Summarize the input into a
// retrospective and Ingest it into the kwiki wiki layer. It returns the
// resulting wiki page (with its slug). Re-running for the same project
// overwrites its retrospective page in place (kwiki.Ingest semantics).
func Archive(store *kwiki.Store, in Input) (kwiki.WikiPage, error) {
	if store == nil {
		return kwiki.WikiPage{}, fmt.Errorf("retro: nil kwiki store")
	}
	r := Summarize(in)
	page, err := store.Ingest(ToInboxItem(r))
	if err != nil {
		return kwiki.WikiPage{}, fmt.Errorf("retro: ingest retrospective for %q: %w", in.Project.Name, err)
	}
	return page, nil
}
