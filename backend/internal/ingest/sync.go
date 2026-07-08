package ingest

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/scottzx/1Agents/backend/internal/feishu"
	"github.com/scottzx/1Agents/backend/internal/icloud"
	"github.com/scottzx/1Agents/backend/internal/meta"
	"github.com/scottzx/1Agents/backend/internal/sources"
	"github.com/scottzx/1Agents/backend/internal/taskapi"
)

// SystemWorkspaceID is the fixed id of the hidden system workspace that hosts
// data-source sync tasks. It lives outside the sidebar (archived) but IS scanned
// by the scheduler (LoadWorkspacesConfig returns archived projects), so its
// function tasks run while never cluttering the user's project list. Its own
// workspace lock also isolates long syncs from real project work.
const SystemWorkspaceID = "__sources_sync__"

// Sync function-handler keys; a work-order task with executor=function and one
// of these fn: labels runs one (vendor, kind) sync. Feishu is single-account;
// microsoft/google fan out over the vendor's accounts.
const (
	FeishuSyncFunction    = "sources.feishu.sync"
	MicrosoftSyncFunction = "sources.microsoft.sync"
	GoogleSyncFunction    = "sources.google.sync"
	AgentMailSyncFunction = "sources.agentmail.sync"
)

// SystemWorkspacePath returns the on-disk host directory (~/.1agents/system/sources).
func SystemWorkspacePath() string {
	return filepath.Join(filepath.Dir(meta.DefaultPath()), "system", "sources")
}

// ProvisionSystemWorkspace creates the sync-task host workspace (idempotent) and
// returns its path. The project carries ProjectStatusSystem: sidebar hides it
// (only "active" appears), scheduler still schedules it (LoadWorkspacesConfig
// returns every status), and agenda/dashboard filters explicitly opt system in
// so periodic system work stays visible without cluttering the user's list.
//
// Self-heal: older provisionings recorded the project as archived + builtin
// (that was the first hack to hide it from the sidebar). Detect that shape and
// migrate to the new status; existing bronze/tasks data is preserved.
func (h *Handler) ProvisionSystemWorkspace() (string, error) {
	path := SystemWorkspacePath()
	if err := os.MkdirAll(path, 0o755); err != nil {
		return "", err
	}
	existing, ok, err := h.db.GetProject(SystemWorkspaceID)
	if err != nil {
		return "", err
	}
	if !ok {
		if err := h.db.EnsureWorkspaceProject(meta.Project{
			ID: SystemWorkspaceID, Name: "数据源同步", WorkspacePath: path,
		}); err != nil {
			return "", err
		}
		if err := h.db.SetProjectStatus(SystemWorkspaceID, meta.ProjectStatusSystem); err != nil {
			return "", err
		}
	} else if existing.Status != meta.ProjectStatusSystem || existing.Builtin {
		// Migrate an older archived+builtin record into the new system status.
		// Re-upsert clears the builtin flag; SetProjectStatus writes "system".
		if err := h.db.EnsureWorkspaceProject(meta.Project{
			ID: SystemWorkspaceID, Name: "数据源同步", WorkspacePath: path,
		}); err != nil {
			return "", err
		}
		if err := h.db.SetProjectStatus(SystemWorkspaceID, meta.ProjectStatusSystem); err != nil {
			return "", err
		}
	}
	h.systemWS = path
	return path, nil
}

// RegisterFunctions registers the source sync function handlers into the global
// work-order function registry. Call once at startup after NewHandlerDefault and
// after manifests are registered (so REST sources get their handler too).
func (h *Handler) RegisterFunctions() {
	taskapi.RegisterFunction(FeishuSyncFunction, h.runFeishuSync)
	taskapi.RegisterFunction(MicrosoftSyncFunction, h.runMicrosoftSync)
	taskapi.RegisterFunction(GoogleSyncFunction, h.runGoogleSync)
	taskapi.RegisterFunction(AgentMailSyncFunction, h.runAgentMailSync)
	// Manifest-declared REST sources share one generic handler, keyed per vendor
	// so the dispatcher's "sources.<vendor>.sync" FunctionType resolves.
	for _, source := range sources.RESTSources() {
		h.RegisterManifestSyncFn(source)
	}
}

// RegisterManifestSyncFn registers the generic sync handler for one manifest
// vendor. Called at startup for pre-loaded manifests and at runtime for a
// hot-added connector (taskapi.RegisterFunction is mutex-guarded).
func (h *Handler) RegisterManifestSyncFn(vendor string) {
	taskapi.RegisterFunction("sources."+vendor+".sync", h.runManifestSync)
}

// SeedManifestAccounts auto-registers one account for each manifest source that
// has none yet, so its card appears on the 数据接入 home without the user manually
// running 添加数据源 (a manifest connector is self-describing — vendor + region +
// label all come from the file). Idempotent: skips a vendor that already has an
// account. The account id then keys the bronze rows + Bearer token consistently.
func (h *Handler) SeedManifestAccounts(ms []sources.Manifest) error {
	if h.accounts == nil {
		return nil
	}
	for _, m := range ms {
		n, err := h.accounts.CountByVendor(m.Vendor)
		if err != nil {
			return err
		}
		if n > 0 {
			continue
		}
		region := m.Region
		if region == "" {
			region = sources.RegionIntl
		}
		label := m.Label
		if label == "" {
			label = m.Vendor
		}
		if _, err := h.accounts.Create(meta.SourceAccount{Vendor: m.Vendor, Region: region, Label: label}, false); err != nil {
			return err
		}
	}
	return nil
}

// SeedManifestConfigs writes a default SourceCollectionConfig for each manifest
// collection that was never configured, so a freshly dropped-in connector appears
// in the config UI (enabled per its manifest default) and is picked up by
// EnsureRecurringForEnabled. Idempotent — never overwrites an existing row.
func (h *Handler) SeedManifestConfigs(ms []sources.Manifest) error {
	for _, m := range ms {
		for _, c := range m.Collections {
			if _, ok, err := h.cfg.Get(m.Vendor, c.Kind); err != nil {
				return err
			} else if ok {
				continue
			}
			if err := h.cfg.Upsert(meta.SourceCollectionConfig{
				Source:              m.Vendor,
				Kind:                c.Kind,
				Enabled:             c.Defaults.Enabled,
				InitialLookbackDays: c.Defaults.InitialLookbackDays,
				IncrementalMinutes:  c.Defaults.IncrementalMinutes, // Upsert clamps <1 → default
				PageSize:            c.Defaults.PageSize,            // Upsert clamps <1 → default
			}); err != nil {
				return err
			}
		}
	}
	return nil
}

// SeedLegacyAccounts migrates the pre-account-model singletons into the
// source_accounts registry so existing installs surface as managed sources.
// Best-effort and idempotent (guarded by CountByVendor==0): an iCloud account
// is seeded from the stored Keychain credential; a Feishu account is seeded when
// legacy Feishu crawl config exists. Fresh installs seed nothing — the user adds
// sources through 添加数据源. Existing "default"-keyed bronze rows are left as-is
// (they re-sync under the new account id on the next run — a dev-acceptable
// re-crawl rather than a risky in-place re-key).
func (h *Handler) SeedLegacyAccounts() error {
	if h.accounts == nil {
		return nil
	}
	// iCloud: a configured Keychain credential ⇒ one intl account (labeled by
	// Apple ID). Region defaults to intl; the user can add a 大陆 account too.
	if n, err := h.accounts.CountByVendor(meta.VendorICloud); err == nil && n == 0 {
		if appleID, ok := icloud.Status(); ok {
			if a, e := h.accounts.Create(meta.SourceAccount{
				Vendor: meta.VendorICloud, Region: sources.RegionIntl, Label: appleID,
			}, false); e == nil {
				_ = h.bronze.ReassignAccount(meta.VendorICloud, "default", a.ID)
			}
		}
	}
	// Feishu: single-account. Seed when legacy crawl config exists.
	if n, err := h.accounts.CountByVendor(meta.VendorFeishu); err == nil && n == 0 {
		if cfgs, cerr := h.cfg.List(feishu.Source); cerr == nil && len(cfgs) > 0 {
			if a, e := h.accounts.Create(meta.SourceAccount{
				Vendor: meta.VendorFeishu, Region: sources.RegionCN, Label: "飞书",
			}, true); e == nil {
				_ = h.bronze.ReassignAccount(feishu.Source, "default", a.ID)
			}
		}
	}
	return nil
}

// feishuAccountID returns the single Feishu account's id (源为中心: bronze is keyed
// by it so the per-account data view lines up), falling back to "default" when no
// account is registered yet — pre-seed / fresh install.
func (h *Handler) feishuAccountID() string {
	if h.accounts == nil {
		return "default"
	}
	accts, err := h.accounts.ListByVendor(meta.VendorFeishu)
	if err != nil || len(accts) == 0 {
		return "default"
	}
	return accts[0].ID
}

// runFeishuSync is the function-executor body for one Feishu kind. The task's
// business_ref ("sources:feishu:<kind>") selects the kind; config supplies the
// crawl policy; the tracked-chat set fans out message kinds. It runs Store.Sync
// (bronze only) and returns SyncStats as the task result — which is exactly the
// per-run statistic the work-order system counts.
func (h *Handler) runFeishuSync(ctx taskapi.FunctionContext) (any, error) {
	kind := kindFromRef(ctx.Task.BusinessRef)
	if kind == "" {
		return nil, fmt.Errorf("ingest: bad business_ref %q", ctx.Task.BusinessRef)
	}
	cfg, _, err := h.cfg.Get(feishu.Source, kind)
	if err != nil {
		return nil, err
	}
	if !cfg.Enabled {
		return map[string]any{"kind": kind, "skipped": "disabled"}, nil
	}
	d := feishu.DescriptorFor(kind)
	if d == nil || !d.Implemented {
		return nil, fmt.Errorf("ingest: kind %q not crawlable", kind)
	}
	spec := sources.FeishuSpec{Kind: kind, PageSize: cfg.PageSize, LookbackDays: cfg.InitialLookbackDays}
	if d.PerChat {
		spec.ChatIDs = h.trackedChatIDs()
		if len(spec.ChatIDs) == 0 {
			return map[string]any{"kind": kind, "skipped": "no tracked chats"}, nil
		}
	}
	if d.PerCalendar {
		spec.CalendarIDs = h.calendarIDs()
		if len(spec.CalendarIDs) == 0 {
			return map[string]any{"kind": kind, "skipped": "no calendars — sync 日历 first"}, nil
		}
	}

	// Feishu is single-account; bronze is keyed by its registry account id so the
	// per-account 数据 view lines up (SeedLegacyAccounts re-keyed legacy "default"
	// rows onto it). Digest/chats read bronze account-agnostically, so this does
	// not disturb the message/digest flow.
	accountID := h.feishuAccountID()
	client := feishu.NewClient("", "default")
	puller := sources.NewFeishuPuller(client, []sources.FeishuSpec{spec})
	stats, err := h.bronze.Sync(puller, accountID)
	if err != nil {
		return nil, err
	}
	result := map[string]any{
		"kind":        kind,
		"collections": stats.Collections,
		"changed":     stats.Changed,
		"skipped":     stats.Skipped,
	}
	// Message kind: after archiving raw messages into bronze, also drive the
	// proven message → unified_messages sync (+ 二度联系人) so the existing
	// message/digest UI stays fresh. This is what makes the work-order task the
	// single message-sync driver (the periodic ticker is retired). Best-effort:
	// a failure here doesn't fail the bronze pull.
	if d.PerChat && h.messageSync != nil {
		mctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
		defer cancel()
		if e := h.messageSync(mctx); e != nil {
			result["messageSyncError"] = e.Error()
		} else {
			result["messageSync"] = "ok"
		}
	}
	// First-time roster capture: after the message sync, pull each tracked chat's
	// group members ONCE (SyncChatMembers skips chats already captured) so 飞书联系人
	// gets the full roster. The roster is not on the recurring schedule — it rarely
	// changes; manual per-group refresh handles updates.
	if kind == "feishu_message" {
		if n, e := h.SyncChatMembers(spec.ChatIDs, false); e != nil {
			result["memberSyncError"] = e.Error()
		} else if n > 0 {
			result["memberChanged"] = n
		}
	}
	h.afterSyncSilver(result) // shape the just-synced bronze into silver
	return result, nil
}

// SyncChatMembers pulls the group roster (feishu_chat_member) for the given chats
// into bronze. force=false pulls only chats without an existing snapshot (the
// first-time capture); force=true re-pulls regardless (manual per-group refresh).
// The roster is NOT on the recurring schedule — it changes rarely — so this is
// its sole driver: once after the first message sync, and on demand. The caller
// runs silver governance afterwards (runFeishuSync via afterSyncSilver; the
// manual endpoint explicitly).
func (h *Handler) SyncChatMembers(chatIDs []string, force bool) (int, error) {
	accountID := h.feishuAccountID()
	client := feishu.NewClient("", "default")
	var changed int
	for _, chatID := range chatIDs {
		if chatID == "" {
			continue
		}
		if !force {
			if _, _, ok, _ := h.bronze.LoadCursor(feishu.Source, accountID, "feishu_chat_member", chatID); ok {
				continue // already captured once
			}
		}
		spec := sources.FeishuSpec{Kind: "feishu_chat_member", PageSize: 100, ChatIDs: []string{chatID}}
		puller := sources.NewFeishuPuller(client, []sources.FeishuSpec{spec})
		stats, err := h.bronze.Sync(puller, accountID)
		if err != nil {
			return changed, err
		}
		changed += stats.Changed
	}
	return changed, nil
}

// runMicrosoftSync / runGoogleSync are the function-executor bodies for the
// Microsoft (Graph) and Google sources. They fan out over the vendor's accounts
// (each with its own region-pinned puller) and run Store.Sync per account. The
// pullers are framework skeletons (Pull is a no-op empty page), so a run wires
// the whole account→config→scheduler→bronze path end-to-end while honestly
// reporting 0 changes until the real API pulls land.
func (h *Handler) runMicrosoftSync(ctx taskapi.FunctionContext) (any, error) {
	return h.runVendorSync(meta.VendorMicrosoft, ctx.Task.BusinessRef, func(a meta.SourceAccount, kinds []string) sources.Puller {
		return sources.NewMicrosoftPuller(a.Region, kinds, h.msAuth)
	})
}

func (h *Handler) runGoogleSync(ctx taskapi.FunctionContext) (any, error) {
	return h.runVendorSync(meta.VendorGoogle, ctx.Task.BusinessRef, func(a meta.SourceAccount, kinds []string) sources.Puller {
		return sources.NewGooglePuller(kinds)
	})
}

// runAgentMailSync is the function-executor body for 腾讯 Agent Mail. Like the
// Microsoft/Google skeletons it fans out over the vendor's accounts (single in
// practice — agently-cli holds one mailbox), but its puller is a real pull:
// agently-cli must be on PATH and authorized on the host.
func (h *Handler) runAgentMailSync(ctx taskapi.FunctionContext) (any, error) {
	return h.runVendorSync(meta.VendorAgentMail, ctx.Task.BusinessRef, func(a meta.SourceAccount, kinds []string) sources.Puller {
		return sources.NewAgentMailPuller()
	})
}

// runVendorSync is the shared body for multi-account skeleton sources. It reads
// the kind from business_ref, checks the (source, kind) crawl config is enabled
// and the catalog kind is implemented (gracefully skipping otherwise), then runs
// one Store.Sync per registered account. build() constructs the account's puller.
func (h *Handler) runVendorSync(source, ref string, build func(meta.SourceAccount, []string) sources.Puller) (any, error) {
	kind := kindFromRef(ref)
	if kind == "" {
		return nil, fmt.Errorf("ingest: bad business_ref %q", ref)
	}
	cfg, _, err := h.cfg.Get(source, kind)
	if err != nil {
		return nil, err
	}
	if !cfg.Enabled {
		return map[string]any{"kind": kind, "skipped": "disabled"}, nil
	}
	if d := sources.CatalogItemFor(source, kind); d == nil || !d.Implemented {
		return map[string]any{"kind": kind, "skipped": "not implemented yet"}, nil
	}
	accts, err := h.accounts.ListByVendor(source)
	if err != nil {
		return nil, err
	}
	if len(accts) == 0 {
		return map[string]any{"kind": kind, "skipped": "no accounts"}, nil
	}
	total := sources.SyncStats{}
	for _, a := range accts {
		stats, serr := h.bronze.Sync(build(a, []string{kind}), a.ID)
		if serr != nil {
			return nil, fmt.Errorf("ingest: %s account %s: %w", source, a.ID, serr)
		}
		total.Collections += stats.Collections
		total.Changed += stats.Changed
		total.Skipped += stats.Skipped
	}
	result := map[string]any{
		"kind":        kind,
		"accounts":    len(accts),
		"collections": total.Collections,
		"changed":     total.Changed,
		"skipped":     total.Skipped,
	}
	h.afterSyncSilver(result) // shape the just-synced bronze into silver
	return result, nil
}

// runManifestSync is the function-executor body for a manifest-declared REST
// source (e.g. 训记). vendor + kind come from business_ref; the RESTDescriptor and
// base URL from the manifest registry; the Bearer token from the per-account store.
// It runs Store.Sync (bronze only) then governance — the same shape as the built-in
// vendor syncs, but driven entirely by manifest data (zero per-source Go).
func (h *Handler) runManifestSync(ctx taskapi.FunctionContext) (any, error) {
	ref := ctx.Task.BusinessRef
	source, kind := sourceFromRef(ref), kindFromRef(ref)
	if source == "" || kind == "" {
		return nil, fmt.Errorf("ingest: bad business_ref %q", ref)
	}
	cfg, _, err := h.cfg.Get(source, kind)
	if err != nil {
		return nil, err
	}
	if !cfg.Enabled {
		return map[string]any{"kind": kind, "skipped": "disabled"}, nil
	}
	desc, ok := sources.RESTDescriptorFor(source, kind)
	if !ok {
		return map[string]any{"kind": kind, "skipped": "no descriptor"}, nil
	}
	baseURL, _ := sources.RESTBaseURL(source)
	accountID := h.manifestAccountID(source)
	// Never run a tokenless pull for a bearer source: an unauthenticated response
	// (e.g. HTTP 200 {"success":false,"res":"apikey missing"}) looks like an empty
	// page to the generic puller and would burn a date-window cursor past real data.
	// Skip until the token is entered — the auto-armed recurring task fires before
	// the user sets it, so this guard is what keeps the first real sync intact.
	if v := sources.VendorFor(source); v != nil && v.AuthKind == sources.AuthBearer && !sources.BearerConfigured(source, accountID) {
		return map[string]any{"kind": kind, "skipped": "no token"}, nil
	}
	token := func() (string, bool) {
		tok, ok, _ := sources.LoadBearerToken(source, accountID)
		return tok, ok
	}
	puller := sources.NewRESTPuller(source, baseURL, []sources.RESTDescriptor{desc}, token)
	stats, err := h.bronze.Sync(puller, accountID)
	if err != nil {
		return nil, err
	}
	result := map[string]any{
		"kind":        kind,
		"collections": stats.Collections,
		"changed":     stats.Changed,
		"skipped":     stats.Skipped,
	}
	h.afterSyncSilver(result)
	return result, nil
}

// manifestAccountID returns the first registered account for a manifest source,
// or "default" pre-seed. Manifest sources may be single- or multi-account; the
// Bearer token store is keyed by this id.
func (h *Handler) manifestAccountID(source string) string {
	if h.accounts != nil {
		if accts, err := h.accounts.ListByVendor(source); err == nil && len(accts) > 0 {
			return accts[0].ID
		}
	}
	return "default"
}

// trackedChatIDs returns the ids of chats flagged for auto-sync — the collection
// set for message-family kinds.
func (h *Handler) trackedChatIDs() []string {
	if h.chats == nil {
		return nil
	}
	tracked, err := h.chats.ListTrackedChats(true)
	if err != nil {
		return nil
	}
	ids := make([]string, 0, len(tracked))
	for _, c := range tracked {
		ids = append(ids, c.ChatID)
	}
	return ids
}

// calendarIDs returns the feishu calendar_ids to crawl events from, read from the
// feishu_calendar bronze list (so 日历 must be synced first). Only owner/writer
// calendars are included — read-only (show_only_free_busy) calendars return no
// event detail, so crawling them is wasted work.
func (h *Handler) calendarIDs() []string {
	recs, err := h.bronze.ListRecords(feishu.Source, "", "feishu_calendar", 0)
	if err != nil {
		return nil
	}
	var ids []string
	for _, r := range recs {
		if r.Deleted {
			continue
		}
		var cal struct {
			CalendarID string `json:"calendar_id"`
			Role       string `json:"role"`
		}
		if json.Unmarshal([]byte(r.Payload), &cal) != nil || cal.CalendarID == "" {
			continue
		}
		if cal.Role == "owner" || cal.Role == "writer" {
			ids = append(ids, cal.CalendarID)
		}
	}
	return ids
}

// businessRef is the opaque work-order binding for a source sync task.
func businessRef(source, kind string) string { return "sources:" + source + ":" + kind }

// kindFromRef pulls <kind> out of "sources:<source>:<kind>".
func kindFromRef(ref string) string {
	parts := strings.SplitN(ref, ":", 3)
	if len(parts) == 3 && parts[0] == "sources" {
		return parts[2]
	}
	return ""
}

// sourceFromRef pulls <source> out of "sources:<source>:<kind>".
func sourceFromRef(ref string) string {
	parts := strings.SplitN(ref, ":", 3)
	if len(parts) == 3 && parts[0] == "sources" {
		return parts[1]
	}
	return ""
}

// knownKinds lists the crawlable kinds for a source (used to aggregate history).
func knownKinds(source string) []string {
	if source == feishu.Source {
		cat := feishu.Catalog()
		ks := make([]string, 0, len(cat))
		for _, d := range cat {
			ks = append(ks, d.Kind)
		}
		return ks
	}
	cat := sources.CatalogFor(source)
	if cat == nil {
		return nil
	}
	ks := make([]string, 0, len(cat))
	for _, it := range cat {
		ks = append(ks, it.Kind)
	}
	return ks
}
