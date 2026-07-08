package govern

// builtin_steps.go surfaces the built-in Go governors (the deep, stateful ones —
// silver source-cleaning + gold identity-resolution/fusion) as first-class
// governance DAG steps, alongside the declarative SQL/Python steps. Before this
// they ran monolithically (govern.Silver / govern.Gold) and were invisible to the
// 数据治理 view; now each governor is a named step with declared upstream→output
// tables, so it appears as a node in the dependency graph, gets an execution-log
// entry, and can be re-run individually (issue #409).
//
// This is display + orchestration metadata over the SAME sub-governors — the list
// here is the authoritative run order (silver before gold; iCloud address book
// before message resolves), and RunBuiltin drives it. govern.Silver / govern.Gold
// stay for their unit tests, but production runs through RunBuiltin.

import (
	"time"

	"github.com/scottzx/1Agents/backend/internal/data"
	"github.com/scottzx/1Agents/backend/internal/sources"
)

// Tier discriminates a step's medallion layer for the 数据治理 dependency view.
const (
	TierSilver = "silver" // bronze → silver (source-faithful cleaning)
	TierGold   = "gold"   // silver → gold (cross-source fusion / resolution)
)

// bronzeNode names the synthetic leaf node a silver governor reads from. Bronze
// lives in a separate DB (sync.db), so it is not a data.db table — it shows in the
// DAG as an origin node ("bronze:<vendor>") the silver step fans out of.
func bronzeNode(vendor string) string { return "bronze:" + vendor }

// BuiltinStep is one built-in Go governor exposed as a governance step. Run writes
// its output table and returns the row count; it is cursor-gated + idempotent, so
// running it any time only shapes what changed.
type BuiltinStep struct {
	Name      string
	Tier      string // TierSilver | TierGold
	Upstreams []string
	Output    string // the table it writes (real data.db table; "" ⇒ internal, not browsable)
	Domain    string // viewer domain for Output ("" ⇒ not browsable)
	Run       func(src *sources.Store, dst *data.Store) (int, error)
}

// goldRows collapses a GoldStats into a single "rows written" number for the log.
func goldRows(st GoldStats) int {
	return st.Threads + st.Messages + st.Contacts + st.Events + st.Todos
}

// gold adapts a gold governor (dst-only) to the BuiltinStep.Run signature.
func gold(fn func(*data.Store) (GoldStats, error)) func(*sources.Store, *data.Store) (int, error) {
	return func(_ *sources.Store, dst *data.Store) (int, error) {
		st, err := fn(dst)
		return goldRows(st), err
	}
}

// BuiltinSteps returns every built-in Go governor as a step, in authoritative run
// order: all silver source-cleaners first, then the gold governors (iCloud address
// book seeds degree-1 contacts before any message sender resolves). Output/domain
// mirror the real data.db tables so the 数据治理 view can drill into each.
func BuiltinSteps() []BuiltinStep {
	feishu, icloud, ms, agent := sources.VendorFeishu, sources.SourceICloud, sources.VendorMicrosoft, sources.VendorAgentMail
	return []BuiltinStep{
		// ---- silver (bronze → silver, source-faithful) ----
		{Name: "silver_icloud_contacts", Tier: TierSilver, Upstreams: []string{bronzeNode(icloud)}, Output: "silver_icloud_contacts", Domain: "contacts", Run: SilverIcloudContacts},
		{Name: "silver_feishu_users", Tier: TierSilver, Upstreams: []string{bronzeNode(feishu)}, Output: "silver_feishu_users", Domain: "contacts", Run: SilverFeishuUsers},
		{Name: "silver_feishu_messages", Tier: TierSilver, Upstreams: []string{bronzeNode(feishu)}, Output: "silver_feishu_messages", Domain: "messages", Run: SilverFeishuMessages},
		{Name: "silver_feishu_chats", Tier: TierSilver, Upstreams: []string{bronzeNode(feishu)}, Output: "silver_feishu_chats", Domain: "", Run: SilverFeishuChats},
		{Name: "silver_microsoft_mail", Tier: TierSilver, Upstreams: []string{bronzeNode(ms)}, Output: "silver_microsoft_mail", Domain: "messages", Run: SilverMicrosoftMail},
		{Name: "silver_agentmail_mail", Tier: TierSilver, Upstreams: []string{bronzeNode(agent)}, Output: "silver_agentmail_mail", Domain: "messages", Run: SilverAgentMail},
		{Name: "silver_feishu_events", Tier: TierSilver, Upstreams: []string{bronzeNode(feishu)}, Output: "silver_feishu_events", Domain: "events", Run: SilverFeishuEvents},
		{Name: "silver_microsoft_events", Tier: TierSilver, Upstreams: []string{bronzeNode(ms)}, Output: "silver_microsoft_events", Domain: "events", Run: SilverMicrosoftEvents},
		{Name: "silver_microsoft_todos", Tier: TierSilver, Upstreams: []string{bronzeNode(ms)}, Output: "silver_microsoft_todos", Domain: "todos", Run: SilverMicrosoftTodos},

		// ---- gold (silver → gold, fusion + transitive identity resolution) ----
		{Name: "gold_contacts_icloud", Tier: TierGold, Upstreams: []string{"silver_icloud_contacts"}, Output: "contacts", Domain: "contacts", Run: gold(GoldContactsIcloud)},
		{Name: "gold_feishu_messages", Tier: TierGold, Upstreams: []string{"silver_feishu_messages", "silver_feishu_users", "silver_feishu_chats"}, Output: "messages", Domain: "messages", Run: gold(GoldFeishu)},
		{Name: "gold_microsoft_mail", Tier: TierGold, Upstreams: []string{"silver_microsoft_mail"}, Output: "messages", Domain: "messages", Run: gold(GoldMicrosoftMail)},
		{Name: "gold_agentmail_mail", Tier: TierGold, Upstreams: []string{"silver_agentmail_mail"}, Output: "messages", Domain: "messages", Run: gold(GoldAgentMail)},
		{Name: "gold_feishu_events", Tier: TierGold, Upstreams: []string{"silver_feishu_events"}, Output: "calendar_events", Domain: "events", Run: gold(GoldFeishuEvents)},
		{Name: "gold_microsoft_events", Tier: TierGold, Upstreams: []string{"silver_microsoft_events"}, Output: "calendar_events", Domain: "events", Run: gold(GoldMicrosoftEvents)},
		{Name: "gold_microsoft_todos", Tier: TierGold, Upstreams: []string{"silver_microsoft_todos"}, Output: "todos", Domain: "todos", Run: gold(GoldMicrosoftTodos)},
	}
}

// RunBuiltin runs every built-in governor in order, recording one RunRecord per
// step. A step's error stops the batch (the caller logs it; the next run resumes
// from the unchanged cursors). This is the production run path for the built-in
// silver+gold layers.
func RunBuiltin(src *sources.Store, dst *data.Store, rec RunRecorder) error {
	for _, s := range BuiltinSteps() {
		start := time.Now()
		n, err := s.Run(src, dst)
		recordRun(rec, RunRecord{Step: s.Name, Output: s.Output, Lang: "go", Upstreams: s.Upstreams}, start, n, err)
		if err != nil {
			return err
		}
	}
	return nil
}

// RunBuiltinStep runs a single built-in governor by name (个别重跑), recording it.
// Returns (rows, found). found=false when no built-in step has that name.
func RunBuiltinStep(src *sources.Store, dst *data.Store, name string, rec RunRecorder) (int, bool, error) {
	for _, s := range BuiltinSteps() {
		if s.Name != name {
			continue
		}
		start := time.Now()
		n, err := s.Run(src, dst)
		recordRun(rec, RunRecord{Step: s.Name, Output: s.Output, Lang: "go", Upstreams: s.Upstreams}, start, n, err)
		return n, true, err
	}
	return 0, false, nil
}
