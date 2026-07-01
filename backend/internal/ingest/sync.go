package ingest

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/scottzx/1Agents/backend/internal/feishu"
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

// FeishuSyncFunction is the registered function-handler key; a work-order task
// with executor=function and this fn: label runs one Feishu kind's sync.
const FeishuSyncFunction = "sources.feishu.sync"

// SystemWorkspacePath returns the on-disk host directory (~/.1agents/system/sources).
func SystemWorkspacePath() string {
	return filepath.Join(filepath.Dir(meta.DefaultPath()), "system", "sources")
}

// ProvisionSystemWorkspace creates the hidden host workspace (idempotent) and
// returns its path. It records an archived+builtin project so the scheduler
// schedules it but the sidebar hides it.
func (h *Handler) ProvisionSystemWorkspace() (string, error) {
	path := SystemWorkspacePath()
	if err := os.MkdirAll(path, 0o755); err != nil {
		return "", err
	}
	_, ok, err := h.db.GetProject(SystemWorkspaceID)
	if err != nil {
		return "", err
	}
	if !ok {
		if err := h.db.EnsureWorkspaceProject(meta.Project{
			ID: SystemWorkspaceID, Name: "数据源同步", WorkspacePath: path, Builtin: true,
		}); err != nil {
			return "", err
		}
		if err := h.db.ArchiveProject(SystemWorkspaceID, meta.ProjectStatusArchived,
			meta.ArchiveReasonCompleted, "系统数据源同步宿主（对侧栏隐藏）"); err != nil {
			return "", err
		}
	}
	h.systemWS = path
	return path, nil
}

// RegisterFunctions registers the source sync function handlers into the global
// work-order function registry. Call once at startup after NewHandlerDefault.
func (h *Handler) RegisterFunctions() {
	taskapi.RegisterFunction(FeishuSyncFunction, h.runFeishuSync)
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

	client := feishu.NewClient("", "default")
	puller := sources.NewFeishuPuller(client, []sources.FeishuSpec{spec})
	stats, err := h.bronze.Sync(puller, "default")
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
	return result, nil
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

// knownKinds lists the crawlable kinds for a source (used to aggregate history).
func knownKinds(source string) []string {
	if source != feishu.Source {
		return nil
	}
	cat := feishu.Catalog()
	ks := make([]string, 0, len(cat))
	for _, d := range cat {
		ks = append(ks, d.Kind)
	}
	return ks
}
