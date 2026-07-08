// Package ingest is the composition root for data-source ingestion: it wires the
// per-collection crawl config (meta.db), the bronze store + Feishu puller
// (sync.db / internal/sources), the CLI-lifecycle probe (internal/sourcecli),
// and the work-order task API (internal/taskapi) into one HTTP surface and one
// scheduler-driven sync path.
//
// Design intent (user directive): every pull — 立刻执行 or 定时增量 — is a
// work-order function task, so the work-order scheduler owns timing and every
// run is automatically counted. This package registers the "sources.feishu.sync"
// function handler and dispatches tasks; it never grows its own ticker.
package ingest

import (
	"context"

	"github.com/scottzx/1Agents/backend/internal/data"
	"github.com/scottzx/1Agents/backend/internal/feishu"
	"github.com/scottzx/1Agents/backend/internal/govern"
	"github.com/scottzx/1Agents/backend/internal/meta"
	"github.com/scottzx/1Agents/backend/internal/sourcecli"
	"github.com/scottzx/1Agents/backend/internal/sources"
)

// Handler serves the data-source management API and owns the sync composition.
type Handler struct {
	db       *meta.DB
	cfg      *meta.SourceCollectionStore
	accounts *meta.SourceAccountStore
	chats    *meta.FeishuChatStore
	bronze   *sources.Store
	silver   *data.Store     // data.db silver/gold — bronze→silver runs after each sync
	msAuth   *sources.MSAuth // Microsoft Graph OAuth (PKCE) — connect flow + token store
	cli      *sourcecli.Handler
	systemWS string     // host workspace path (set by ProvisionSystemWorkspace)
	disp     Dispatcher // set via SetDispatcher once the task API is available
	// messageSync drives the existing (proven) Feishu message → unified_messages
	// sync + 二度联系人 ingestion. Injected from server.go (digest.SyncTracked) so
	// the feishu_message work-order task also keeps the message/digest UI fresh
	// while bronze holds the raw archive. nil until wired.
	messageSync func(context.Context) error
	// manifestSilver holds the generic bronze→silver specs derived from connector
	// manifests (set by RegisterManifestGovernance), run after each sync.
	manifestSilver []govern.ManifestSilverSpec
}

// Dispatcher is the narrow slice of the work-order task API this package needs:
// dispatch a function task and query prior runs for the history/statistics view.
// It is an interface so ingest doesn't import the whole taskapi/agent graph at
// construction (server.go injects the concrete implementation).
type Dispatcher interface {
	// SyncNow dispatches an immediate one-off sync task for (source, kind).
	// collection is optional (a specific chat id for message kinds). Returns the
	// new task id.
	SyncNow(source, kind, collection string) (string, error)
	// EnsureRecurring makes sure a periodic (interval) sync task exists for
	// (source, kind) at everyMinutes cadence, creating it if absent.
	EnsureRecurring(source, kind string, everyMinutes int) error
	// History returns prior sync runs for a source (newest first), read from the
	// work-order tasks by business_ref prefix.
	History(source string) ([]SyncRun, error)
	// Schedules returns the live periodic-sync trigger state for a source, one row
	// per kind that has any work-order task, read from the tasks table.
	Schedules(source string) ([]ScheduleRow, error)
}

// SyncRun is one work-order sync task surfaced to the history view.
type SyncRun struct {
	TaskID      string `json:"taskId"`
	Kind        string `json:"kind"`
	Collection  string `json:"collection,omitempty"`
	Status      string `json:"status"`
	Result      string `json:"result,omitempty"`
	CreatedAt   string `json:"createdAt"`
	CompletedAt string `json:"completedAt,omitempty"`
}

// ScheduleRow is one source kind's live periodic-sync trigger state, surfaced to
// the 定时任务 view: whether an interval task is armed, its current status and
// next trigger, and the most recent completed run. It is the "task system" slice
// of the config UI — the collection's enabled/cadence policy comes from
// /collections; this adds the runtime trigger status on top.
type ScheduleRow struct {
	Kind       string `json:"kind"`
	Recurring  bool   `json:"recurring"`            // a live interval task exists
	Status     string `json:"status,omitempty"`     // that task's status ("" = not armed)
	NextRunAt  string `json:"nextRunAt,omitempty"`  // its scheduled trigger time
	LastRunAt  string `json:"lastRunAt,omitempty"`  // most recent terminal run
	LastStatus string `json:"lastStatus,omitempty"` // that run's terminal status
}

// NewHandlerDefault wires a Handler over the default meta.db + sync.db bronze
// store plus a CLI probe.
func NewHandlerDefault() (*Handler, error) {
	db, err := meta.OpenDefault()
	if err != nil {
		return nil, err
	}
	bronze, err := sources.OpenDefault()
	if err != nil {
		return nil, err
	}
	// A missing microsoft_oauth.json is not fatal — the connect flow reports
	// "not configured" until the config is dropped in.
	msAuth, err := sources.NewMSAuth()
	if err != nil {
		return nil, err
	}
	silver, err := data.OpenDefault()
	if err != nil {
		return nil, err
	}
	return &Handler{
		db:       db,
		cfg:      meta.NewSourceCollectionStore(db),
		accounts: meta.NewSourceAccountStore(db),
		chats:    meta.NewFeishuChatStore(db),
		bronze:   bronze,
		silver:   silver,
		msAuth:   msAuth,
		cli:      sourcecli.NewHandler(),
	}, nil
}

// SetDispatcher injects the work-order dispatcher (built in server.go from the
// task API + scheduler). Until set, sync-now / history degrade gracefully.
func (h *Handler) SetDispatcher(d Dispatcher) { h.disp = d }

// SetMessageSync injects the Feishu message → unified_messages sync (digest's
// SyncTracked). The feishu_message work-order task calls it after the bronze
// pull so the existing message/digest UI stays fresh. See the messageSync field.
func (h *Handler) SetMessageSync(fn func(context.Context) error) { h.messageSync = fn }

// MigrateLegacyMessageSync carries the old digest auto-sync setting
// (feishu_sync_config) into the new per-collection config, once, so existing
// users' message sync keeps running through the work-order path after the
// periodic ticker is retired. No-op when feishu_message is already configured
// (respects an explicit user choice) or legacy sync was off.
func (h *Handler) MigrateLegacyMessageSync() error {
	if _, ok, err := h.cfg.Get(feishu.Source, "feishu_message"); err != nil {
		return err
	} else if ok {
		return nil
	}
	legacy, err := h.chats.GetSyncConfig()
	if err != nil {
		return err
	}
	if !legacy.Enabled {
		return nil
	}
	return h.cfg.Upsert(meta.SourceCollectionConfig{
		Source: feishu.Source, Kind: "feishu_message", Enabled: true,
		InitialLookbackDays: 7, IncrementalMinutes: legacy.IntervalMinutes, PageSize: 50,
	})
}

// EnsureRecurringForEnabled re-arms a periodic work-order task for every enabled
// Feishu collection. Called once at startup so cadences configured in a prior
// run resume without the user re-toggling them — this is what replaces the
// retired digest ticker. Idempotent (EnsureRecurring skips live tasks).
func (h *Handler) EnsureRecurringForEnabled() error {
	if h.disp == nil {
		return nil
	}
	// Feishu (its own catalog/descriptor).
	enabled, err := h.cfg.ListEnabled(feishu.Source)
	if err != nil {
		return err
	}
	for _, c := range enabled {
		if d := feishu.DescriptorFor(c.Kind); d == nil || !d.Implemented {
			continue
		}
		if err := h.disp.EnsureRecurring(feishu.Source, c.Kind, c.IncrementalMinutes); err != nil {
			return err
		}
	}
	// Microsoft / Google (this package's catalog). Without this, their periodic
	// sync tasks are only armed on toggle and are NOT re-created at startup — so a
	// recurring task that ended in a terminal (e.g. failed) state would never come
	// back after a restart. EnsureRecurring is idempotent (skips a live one).
	// Built-in multi-account sources + manifest-declared REST sources (both use
	// CatalogItemFor, which now surfaces REST kinds too).
	vendors := append([]string{meta.VendorMicrosoft, meta.VendorGoogle, meta.VendorAgentMail}, sources.RESTSources()...)
	for _, source := range vendors {
		list, err := h.cfg.ListEnabled(source)
		if err != nil {
			return err
		}
		for _, c := range list {
			if d := sources.CatalogItemFor(source, c.Kind); d == nil || !d.Implemented {
				continue
			}
			if err := h.disp.EnsureRecurring(source, c.Kind, c.IncrementalMinutes); err != nil {
				return err
			}
		}
	}
	return nil
}

// CLIHandler exposes the CLI-lifecycle handler for route registration.
func (h *Handler) CLIHandler() *sourcecli.Handler { return h.cli }

// EnabledFeishuSpecs reads the enabled Feishu collections from config and turns
// them into puller specs. chatIDs supplies the tracked-chat set for PerChat
// kinds (feishu_message); the caller sources it from feishu_tracked_chats.
func (h *Handler) EnabledFeishuSpecs(chatIDs []string) ([]sources.FeishuSpec, error) {
	enabled, err := h.cfg.ListEnabled(feishu.Source)
	if err != nil {
		return nil, err
	}
	specs := make([]sources.FeishuSpec, 0, len(enabled))
	for _, c := range enabled {
		d := feishu.DescriptorFor(c.Kind)
		if d == nil || !d.Implemented {
			continue
		}
		spec := sources.FeishuSpec{
			Kind:         c.Kind,
			PageSize:     c.PageSize,
			LookbackDays: c.InitialLookbackDays,
		}
		if d.PerChat {
			spec.ChatIDs = chatIDs
		}
		specs = append(specs, spec)
	}
	return specs, nil
}
