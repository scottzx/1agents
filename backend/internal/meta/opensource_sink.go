package meta

import "github.com/scottzx/1Agents/backend/internal/opensource"

// InboxProposalSink lands opensource 吸收提案 (#138) into the Inbox 收口层 (#60)
// as captured items, so they flow through the same review/archive trail as every
// other intake. It implements opensource.ProposalSink. Kept in meta (not
// opensource) so the opensource package never imports the store — the dependency
// points inward, matching the project's decoupling rule.
type InboxProposalSink struct {
	store *InboxStore
}

// NewInboxProposalSink wraps an InboxStore as a proposal sink.
func NewInboxProposalSink(store *InboxStore) *InboxProposalSink {
	return &InboxProposalSink{store: store}
}

// Submit delivers the proposal into the default Workspace Inbox (#202 P3):
// source=misc (opensource absorb channel), workspace_id always set (never bare
// global). Dedup is by URL — re-running the pipeline won't double-capture.
func (s *InboxProposalSink) Submit(p opensource.Proposal) error {
	if s.alreadyCaptured(p.Candidate.URL) {
		return nil
	}
	_, err := s.store.Deliver(InboxItem{
		WorkspaceID: DefaultInboxWorkspaceID,
		Source:      InboxSourceMisc,
		FromRef:     "opensource:" + p.Candidate.FullName,
		Title:       p.Title(),
		Content:     p.Body(),
		URL:         p.Candidate.URL,
		Summary:     p.Summary(),
		Tags:        []string{"opensource-absorb", string(p.Decision)},
		Status:      InboxStatusUnread,
	})
	return err
}

// alreadyCaptured reports whether an open-source-absorb item with this URL
// already exists (any status), so re-runs are idempotent.
func (s *InboxProposalSink) alreadyCaptured(url string) bool {
	if url == "" {
		return false
	}
	var n int
	if err := s.store.db.sql.QueryRow(
		`SELECT COUNT(1) FROM inbox_items WHERE url = ? AND source = ?`,
		url, InboxSourceMisc,
	).Scan(&n); err != nil {
		return false
	}
	return n > 0
}
