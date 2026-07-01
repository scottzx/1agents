package sources

// A Puller knows how to enumerate one source's collections and pull a
// collection's changes since a cursor. It writes nothing itself — the driver
// (Store.Sync) persists everything to bronze — so a puller is a thin, testable
// adapter over a source's API. New sources (飞书, MS Graph, Google) implement
// this same interface; only the cursor flavor and the discovery differ.
type Puller interface {
	// Source is the stable discriminator written into source_records.source.
	Source() string
	// Discover lists the collections to sync, each carrying a version Gate
	// (CardDAV CTag / "" when the source has none). An unchanged Gate lets the
	// driver skip the whole collection without a single content request.
	Discover(accountID string) ([]Collection, error)
	// Pull returns one page of changes for a collection since cur. done=false
	// asks the driver to call again with the returned cursor (pagination);
	// done=true ends the collection. A tombstone is a RawRecord with Deleted set.
	Pull(accountID string, c Collection, cur Cursor) (recs []RawRecord, next Cursor, done bool, err error)
}

// Collection is one syncable unit within a source (an address book, a calendar,
// a chat). Gate is the current collection version (CTag); "" means the source
// exposes no version, so the driver always pulls.
type Collection struct {
	Kind string
	ID   string
	Gate string
}

// SyncStats summarizes a driver run for logging/verification.
type SyncStats struct {
	Collections int // discovered
	Skipped     int // unchanged CTag, no network
	Changed     int // bronze rows written (inserts + real updates + tombstones)
}

// Sync drives a puller into bronze: discover collections, skip the ones whose
// version gate is unchanged, else page through their changes and commit each
// page (records + cursor) in one transaction, finally recording the new gate.
// Fetch-only: it never touches gold. Governance is a separate, re-runnable step.
func (st *Store) Sync(p Puller, accountID string) (SyncStats, error) {
	var stats SyncStats
	source := p.Source()
	colls, err := p.Discover(accountID)
	if err != nil {
		return stats, err
	}
	for _, c := range colls {
		stats.Collections++
		cur, gate, ok, err := st.LoadCursor(source, accountID, c.Kind, c.ID)
		if err != nil {
			return stats, err
		}
		if ok && c.Gate != "" && gate == c.Gate {
			stats.Skipped++ // collection unchanged since last sync — zero network
			continue
		}
		if !ok {
			cur = Cursor{} // first sync: empty cursor = full seed
		}
		for {
			recs, next, done, err := p.Pull(accountID, c, cur)
			if err != nil {
				return stats, err
			}
			changed, err := st.CommitPage(source, accountID, recs, next)
			if err != nil {
				return stats, err
			}
			stats.Changed += changed
			// An empty page still advances the cursor (the token moved even though
			// nothing changed); CommitPage only persists it when records exist.
			if len(recs) == 0 && next.Kind != "" {
				if err := st.SaveCollectionCursor(source, accountID, c.Kind, c.ID, next); err != nil {
					return stats, err
				}
			}
			cur = next
			if done {
				break
			}
		}
		if c.Gate != "" {
			if err := st.SaveGate(source, accountID, c.Kind, c.ID, c.Gate); err != nil {
				return stats, err
			}
		}
	}
	return stats, nil
}
