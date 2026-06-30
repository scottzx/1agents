package crm

import (
	"log"

	"github.com/scottzx/1Agents/backend/internal/appkit"
	"github.com/scottzx/1Agents/backend/internal/appregistry"
	"github.com/scottzx/1Agents/backend/internal/meta"
	"github.com/scottzx/1Agents/backend/internal/taskapi"
)

// AppID is the CRM namespace — the single string threaded through business_ref,
// domain tables, taskTypes, and RegisterApp.
const AppID = "crm"

// Task type / function keys (manifest.taskTypes — all "crm." prefixed).
const (
	TaskTypeEnrich = "crm.enrich" // executor=function: deterministic external enrichment
	TaskTypeScore  = "crm.score"  // executor=agent: digest-style lead mining + scoring
)

// Manifest is the CRM app manifest (§2 canonical shape). CRM is a global app:
// one l1-page mount (the funnel + contact library) plus a project-scoped lens
// (关联线索) that overlays related leads onto a project.
func Manifest() appregistry.AppManifest {
	return appregistry.AppManifest{
		ID:      AppID,
		Name:    "CRM",
		Version: "1.0.0",
		Enabled: true,
		MountPoints: []appregistry.MountPoint{
			{Type: "l1-page", ID: "crm", Label: "CRM", View: "CrmPage", Icon: "👥"},
			{Type: "lens", ID: "leads", Label: "关联线索", View: "CrmLeadsLens", Scope: "project"},
		},
		TaskTypes:    []string{TaskTypeEnrich, TaskTypeScore},
		DomainTables: []string{"crm_contact", "crm_lead"},
	}
}

// domainDDLs are the idempotent CREATE TABLE statements for CRM domain tables.
// All prefixed crm_; never bump the global schemaVersion (R4).
func domainDDLs() []string {
	return []string{
		`CREATE TABLE IF NOT EXISTS crm_contact (
			id         TEXT PRIMARY KEY,
			name       TEXT NOT NULL,
			company    TEXT NOT NULL DEFAULT '',
			title      TEXT NOT NULL DEFAULT '',
			email      TEXT NOT NULL DEFAULT '',
			phone      TEXT NOT NULL DEFAULT '',
			source     TEXT NOT NULL DEFAULT '',
			created_at TEXT NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS crm_lead (
			id           TEXT PRIMARY KEY,
			contact_id   TEXT NOT NULL DEFAULT '',
			stage        TEXT NOT NULL DEFAULT 'new',
			score        INTEGER NOT NULL DEFAULT 0,
			owner        TEXT NOT NULL DEFAULT '',
			business_ref TEXT NOT NULL DEFAULT '',
			notes        TEXT NOT NULL DEFAULT '',
			created_at   TEXT NOT NULL,
			updated_at   TEXT NOT NULL
		)`,
	}
}

// pkgStore is the package-level store, opened once at init over the shared
// meta.db. HTTP handlers, the completion hook, and the function handler share it.
var pkgStore *Store

// init runs at package load: registers the manifest, ensures domain tables, and
// queues the runtime init (RegisterApp / RegisterFunction / RegisterCompletionHook).
// No API instance is needed here — appkit.OnInit gets the live API later.
func init() {
	appregistry.Register(Manifest())

	if err := appregistry.EnsureDomainTables(AppID, domainDDLs()); err != nil {
		log.Printf("[crm] ensure domain tables: %v", err)
	}

	if db, err := meta.OpenDefault(); err == nil {
		pkgStore = NewStore(db.SQL())
	} else {
		log.Printf("[crm] open meta db: %v", err)
	}

	appkit.OnInit(func(api *taskapi.API) {
		// 1. Declare permissions: CRM may dispatch crm.* tasks against crm: refs.
		api.RegisterApp(taskapi.AppPermissions{
			Namespace:    AppID,
			AllowedTypes: []string{TaskTypeEnrich, TaskTypeScore},
			AllowedRefs:  []string{"crm:"},
		})

		// 2. Register the deterministic enrichment function (#342, token≈0).
		taskapi.RegisterFunction(TaskTypeEnrich, enrichHandler)

		// 3. Register the completion writeback hook — claims crm: refs only and
		//    writes results back into crm_lead (#341 score, #342 enrich, #343 human).
		api.RegisterCompletionHook(completionHook)

		// 4. Keep a handle to the live API for dispatch from HTTP handlers.
		liveAPI = api
	})
}
