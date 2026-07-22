package meta

import (
	"database/sql"
	"regexp"
	"strconv"
	"strings"
)

// Cross-reference + backlink knowledge graph (issue #136).
//
// GitHub auto-links any `#N` mention into a bidirectional reference: the source
// shows "references #N" and the target shows "referenced by". We reuse the
// existing TaskLink machinery — a forward edge is a TaskLink{Target, Rel} stored
// on Task.Links; the *backlink* is simply the reverse of that edge, computed by
// scanning who points at a task rather than stored a second time (no duplicate
// state to keep consistent). Mentions in a task's text are parsed into
// LinkRelates edges so the graph fills itself in as people/agents write `#N`.
//
// Reference grammar (mirrors the frontend permalink renderer in
// frontend/src/utils/markdown.ts):
//
//	#90            → same-project reference (the task's own project)
//	`项目名#90`     → cross-project reference (backtick-delimited; split on LAST #)
//
// `#bug` / `#todo` (non-digit) are never refs. The backend parser matches any
// `#<digits>` token, since text stored on a task is the authoritative body and
// over-matching a stray number is harmless — it only becomes a link if a task
// with that number actually exists.

var (
	// sameProjectRef matches `#<digits>` not immediately preceded by a word
	// char or backtick (so "abc#3" / "v1.2#3" / a cross-project token don't
	// false-match) and ending on a word boundary.
	sameProjectRef = regexp.MustCompile("(^|[^\\w`])#(\\d+)\\b")
	// crossProjectRef matches a backtick-delimited `<name>#<digits>` token,
	// splitting on the LAST '#' so names may contain anything but a backtick.
	crossProjectRef = regexp.MustCompile("`([^`\n]+?)#(\\d+)`")
)

// Ref is a parsed `#N` mention. Project is empty for a same-project reference
// and holds the display name/id for a cross-project one.
type Ref struct {
	Project string
	Number  int
}

// ParseRefs extracts the distinct task references mentioned in text. Cross-
// project refs are parsed first and their spans blanked so the same-project
// scanner doesn't also match the trailing `#N` inside them. Order is stable
// (first appearance) and duplicates are collapsed.
func ParseRefs(text string) []Ref {
	if text == "" {
		return nil
	}
	var refs []Ref
	seen := map[Ref]bool{}
	add := func(r Ref) {
		if r.Number <= 0 || seen[r] {
			return
		}
		seen[r] = true
		refs = append(refs, r)
	}

	// Cross-project first; blank out matched spans so the bare scanner skips them.
	masked := []byte(text)
	for _, m := range crossProjectRef.FindAllStringSubmatchIndex(text, -1) {
		name := strings.TrimSpace(text[m[2]:m[3]])
		n, _ := strconv.Atoi(text[m[4]:m[5]])
		add(Ref{Project: name, Number: n})
		for i := m[0]; i < m[1]; i++ {
			masked[i] = ' '
		}
	}
	for _, m := range sameProjectRef.FindAllStringSubmatch(string(masked), -1) {
		n, _ := strconv.Atoi(m[2])
		add(Ref{Number: n})
	}
	return refs
}

// resolveRefTargets turns the references mentioned in text into target task ids,
// resolved within the task's own project (same-project `#N`) or by project
// name/id (cross-project). selfID is excluded so a task can't link to itself.
// Unresolvable refs (unknown project or number) are dropped — a dangling `#N`
// is just not a link, matching the frontend's optimistic-but-validated rule.
func (s *TaskStore) resolveRefTargets(projectID, selfID, text string) ([]string, error) {
	refs := ParseRefs(text)
	if len(refs) == 0 {
		return nil, nil
	}
	var out []string
	seen := map[string]bool{}
	for _, ref := range refs {
		var (
			t   Task
			ok  bool
			err error
		)
		if ref.Project == "" {
			t, ok, err = s.GetTaskByNumber(projectID, ref.Number)
		} else {
			t, _, ok, err = s.ResolveByNumber(ref.Project, ref.Number)
		}
		if err != nil {
			return nil, err
		}
		if !ok || t.ID == selfID || seen[t.ID] {
			continue
		}
		seen[t.ID] = true
		out = append(out, t.ID)
	}
	return out, nil
}

// SyncRefLinks reconciles a task's auto cross-reference links against the `#N`
// mentions in its title + description, then persists the merged Links. It is
// additive and idempotent: a mention adds a LinkRelates edge if absent; existing
// links (including explicit "closes" edges and links the parser can no longer
// see) are preserved. The reverse direction is not stored — LinkGraphFor derives
// it. Call after a task's text changes (create / description update). Returns the
// links actually added.
func (s *TaskStore) SyncRefLinks(taskID string) ([]TaskLink, error) {
	var projectID, title, desc string
	err := s.db.sql.QueryRow(
		`SELECT project_id, title, description FROM project_items WHERE id = ?`, taskID,
	).Scan(&projectID, &title, &desc)
	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}

	targets, err := s.resolveRefTargets(projectID, taskID, title+"\n"+desc)
	if err != nil {
		return nil, err
	}
	if len(targets) == 0 {
		return nil, nil
	}

	var linksJSON string
	if err := s.db.sql.QueryRow(`SELECT links FROM project_items WHERE id = ?`, taskID).Scan(&linksJSON); err != nil {
		return nil, err
	}
	links := jsonToLinks(linksJSON)
	have := map[string]bool{}
	for _, l := range links {
		have[l.Target] = true
	}
	var added []TaskLink
	for _, tgt := range targets {
		if have[tgt] {
			continue
		}
		l := TaskLink{Target: tgt, Rel: LinkRelates}
		links = append(links, l)
		added = append(added, l)
		have[tgt] = true
	}
	if len(added) == 0 {
		return nil, nil
	}
	if _, err := s.db.sql.Exec(
		`UPDATE project_items SET links = ? WHERE id = ?`, linksToJSON(links), taskID); err != nil {
		return nil, err
	}
	return added, nil
}

// LinkEdge is one resolved edge of the cross-reference graph. Task carries the
// peer's identity for display/traversal; Rel is the relation kind on the stored
// forward edge (always the source→target direction, even for a backlink, so a
// "closes" backlink still reads as "closes").
type LinkEdge struct {
	Rel  LinkRel `json:"rel"`
	Task Task    `json:"task"`
}

// LinkGraph is the minimal knowledge graph around one task: the tasks it points
// at (Outgoing, from Task.Links) and the tasks that point at it (Incoming, the
// backlinks). Both are scoped to the task's own project — cross-project edges
// are out of scope for this first cut (the data model supports them, but the
// traversal here stays within a workspace, like the board itself).
type LinkGraph struct {
	Outgoing []LinkEdge `json:"outgoing"`
	Incoming []LinkEdge `json:"incoming"`
}

// LinkGraphFor returns the outgoing references and incoming backlinks for taskID.
// Peers are returned as lightweight tasks (no children hydration) sufficient for
// rendering a reference list and for an agent to walk "why does this task exist".
func (s *TaskStore) LinkGraphFor(taskID string) (LinkGraph, bool, error) {
	var projectID, linksJSON string
	err := s.db.sql.QueryRow(
		`SELECT project_id, links FROM project_items WHERE id = ?`, taskID).Scan(&projectID, &linksJSON)
	if err == sql.ErrNoRows {
		return LinkGraph{}, false, nil
	}
	if err != nil {
		return LinkGraph{}, false, err
	}

	g := LinkGraph{Outgoing: []LinkEdge{}, Incoming: []LinkEdge{}}

	// Outgoing: this task's own forward edges.
	for _, l := range jsonToLinks(linksJSON) {
		peer, ok, err := s.getTaskLite(l.Target)
		if err != nil {
			return LinkGraph{}, false, err
		}
		if ok {
			g.Outgoing = append(g.Outgoing, LinkEdge{Rel: l.Rel, Task: peer})
		}
	}

	// Incoming: every task in the same project whose Links target this one. The
	// links column is small JSON; a project scan is fine at task-board scale and
	// the LIKE prefilter avoids decoding rows that can't match.
	rows, err := s.db.sql.Query(
		`SELECT `+taskCols+` FROM project_items WHERE project_id = ? AND id <> ? AND links LIKE ?`,
		projectID, taskID, "%"+taskID+"%")
	if err != nil {
		return LinkGraph{}, false, err
	}
	defer rows.Close()
	for rows.Next() {
		t, err := scanTask(rows)
		if err != nil {
			return LinkGraph{}, false, err
		}
		for _, l := range t.Links {
			if l.Target == taskID {
				lite := t
				lite.Replies = nil
				lite.Sessions = nil
				lite.DependsOn = nil
				g.Incoming = append(g.Incoming, LinkEdge{Rel: l.Rel, Task: lite})
				break
			}
		}
	}
	return g, true, rows.Err()
}

// getTaskLite fetches a single task row without hydrating children — enough to
// describe a graph peer.
func (s *TaskStore) getTaskLite(taskID string) (Task, bool, error) {
	row := s.db.sql.QueryRow(`SELECT `+taskCols+` FROM project_items WHERE id = ?`, taskID)
	t, err := scanTask(row)
	if err == sql.ErrNoRows {
		return Task{}, false, nil
	}
	if err != nil {
		return Task{}, false, err
	}
	t.Replies = nil
	t.Sessions = nil
	t.DependsOn = nil
	return t, true, nil
}
