// Package govern is the transform layer between the raw bronze store
// (internal/sources) and the curated gold model (meta.db). It reads source
// records and shapes them into domain entities, never touching the network — so
// changing a transform (extracting a new field, fixing a mapping) re-runs
// against existing bronze instead of re-fetching, which matters when the source
// throttles repeated pulls (iCloud). It is re-runnable: a governance cursor
// tracks what's already been shaped, and every gold write is idempotent, so a
// full re-govern (cursor reset) is always safe.
package govern

import (
	"github.com/scottzx/1Agents/backend/internal/icloud"
	"github.com/scottzx/1Agents/backend/internal/meta"
	"github.com/scottzx/1Agents/backend/internal/sources"
)

// ICloudContacts shapes bronze iCloud vCard records into gold contacts, keyed on
// normalized phone (the cross-source "same person" merge point in
// ContactStore.IngestContacts). It processes only records changed since the last
// governance run, then advances the cursor — so an unchanged sync governs zero
// records (created=0/updated=0). Passing a reset cursor re-governs everything
// safely, reproducing gold from bronze without a network round-trip.
//
// Tombstones (deleted vCards) are recorded in bronze but not yet removed from
// gold — IngestContacts is upsert-only. Gold-side deletion is a Phase 1
// limitation tracked in Epic #359.
func ICloudContacts(st *sources.Store, cs *meta.ContactStore) (created, updated int, err error) {
	since, err := st.GovernCursor(sources.SourceICloud, sources.KindContact)
	if err != nil {
		return 0, 0, err
	}
	recs, maxFetched, err := st.RecordsSince(sources.SourceICloud, sources.KindContact, since)
	if err != nil {
		return 0, 0, err
	}
	var imported []meta.ImportedContact
	for _, r := range recs {
		if r.Deleted {
			continue
		}
		for _, person := range icloud.ParseVCards(r.Payload) {
			phone := ""
			if len(person.Phones) > 0 {
				phone = person.Phones[0]
			}
			imported = append(imported, meta.ImportedContact{
				Phone:   phone,
				Name:    person.Name,
				Company: person.Org,
				Title:   person.Title,
			})
		}
	}
	created, updated, err = cs.IngestContacts(imported)
	if err != nil {
		return created, updated, err
	}
	if err := st.SaveGovernCursor(sources.SourceICloud, sources.KindContact, maxFetched); err != nil {
		return created, updated, err
	}
	return created, updated, nil
}
