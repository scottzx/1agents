package govern

import (
	"regexp"
	"strings"
	"time"

	"github.com/scottzx/1Agents/backend/internal/data"
	"github.com/scottzx/1Agents/backend/internal/sources"
)

// silver.go is the SHARED bronze→silver scaffolding; each source's governors and
// parsers live in their own silver_<source>.go file (issue #399).
//
// The bronze→silver stage is per-SOURCE, source-faithful cleaning. Each source's
// governor knows its own raw shape and writes its own native silver table
// (internal/data), preserving everything valuable — Apple
// birthday/nickname/note, 飞书 @mentions + reply chain, MS todo
// recurrence/checklist. It is re-runnable: a per-(source,kind) StageSilver cursor
// over bronze.fetched_at tracks progress, and every upsert is idempotent, so
// resetting the cursor re-shapes everything without a network round-trip.
// Cross-source unification is gold (step 3), not here.

// SilverStats reports how many silver rows each viewer domain wrote in a run
// (contacts = icloud + 飞书二级用户; messages = 飞书 + MS + AgentMail).
type SilverStats struct {
	Contacts, Messages, Events, Todos int
}

// Silver runs every source's bronze→silver transform. Adding a source means
// dropping in a silver_<source>.go with its governor(s) and listing it here.
func Silver(src *sources.Store, dst *data.Store) (SilverStats, error) {
	var st SilverStats
	steps := []struct {
		add *int
		run func(*sources.Store, *data.Store) (int, error)
	}{
		{&st.Contacts, SilverIcloudContacts},
		{&st.Contacts, SilverFeishuUsers},
		{&st.Messages, SilverFeishuMessages},
		{&st.Messages, SilverMicrosoftMail},
		{&st.Messages, SilverAgentMail},
		{&st.Events, SilverMicrosoftEvents},
		{&st.Events, SilverFeishuEvents},
		{&st.Todos, SilverMicrosoftTodos},
		{nil, SilverFeishuChats}, // group metadata; supports gold threads, not a viewer domain
	}
	for _, s := range steps {
		n, err := s.run(src, dst)
		if err != nil {
			return st, err
		}
		if s.add != nil {
			*s.add += n
		}
	}
	return st, nil
}

// runSilver drives one (source, kind) feed: read the bronze slice since the
// StageSilver cursor, parse each record into zero-or-more silver rows, upsert
// them, and advance the cursor to the max fetched_at seen. Shared by every 1:1
// governor.
func runSilver[T any](src *sources.Store, dst *data.Store, source, kind string,
	parse func(sources.StoredRecord) []T, upsert func([]T) (int, error)) (int, error) {
	since, err := dst.GovernCursor(data.StageSilver, source, kind)
	if err != nil {
		return 0, err
	}
	recs, maxFetched, err := src.RecordsSince(source, kind, since)
	if err != nil {
		return 0, err
	}
	var rows []T
	for _, r := range recs {
		rows = append(rows, parse(r)...)
	}
	n, err := upsert(rows)
	if err != nil {
		return 0, err
	}
	if err := dst.SaveGovernCursor(data.StageSilver, source, kind, maxFetched); err != nil {
		return n, err
	}
	return n, nil
}

// ---- shared time / text helpers (used by more than one source's parser) ----

var isoLayouts = []string{
	time.RFC3339Nano,
	time.RFC3339,
	"2006-01-02T15:04:05.9999999Z07:00",
	"2006-01-02T15:04:05.9999999", // Graph often omits the zone (implicit UTC)
	"2006-01-02T15:04:05",
}

func parseISOTime(s string) int64 {
	if s == "" {
		return 0
	}
	for _, l := range isoLayouts {
		if t, err := time.Parse(l, s); err == nil {
			return t.UnixMilli()
		}
	}
	return 0
}

var tagRe = regexp.MustCompile(`<[^>]+>`)

func stripTags(s string) string { return strings.TrimSpace(tagRe.ReplaceAllString(s, "")) }
