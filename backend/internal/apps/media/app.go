// Package media is the 自媒体 (self-media) installable app — the first of three
// Phase 1 vertical-slice apps proving apps are ADDITIVE (zero changes to core
// tables or the task main flow). It owns the "media_*" domain tables, registers
// a project template, dispatches a mixed-executor processing pipeline through the
// North Task API, and exposes HTTP handlers + frontend project-tab views.
//
// Contract: this package imports the platform packages (taskapi, appkit,
// appregistry, domainstore, templateregistry, meta) and is imported back only by
// the central apps aggregator (blank import). It never touches core tables.
package media

import (
	"log"

	"github.com/scottzx/1Agents/backend/internal/appkit"
	"github.com/scottzx/1Agents/backend/internal/appregistry"
	"github.com/scottzx/1Agents/backend/internal/meta"
	"github.com/scottzx/1Agents/backend/internal/taskapi"
	"github.com/scottzx/1Agents/backend/internal/templateregistry"
)

// Manifest fields, surfaced at GET /api/apps.
const (
	appName    = "自媒体"
	appVersion = "1.0.0"
	templateID = "media.content_project"
)

func init() {
	// 1. App manifest (compile-time registry). project-tab mount points only —
	//    a media project = one project instance, surfaced as tabs in ProjectShell.
	appregistry.Register(appregistry.AppManifest{
		ID:      AppID,
		Name:    appName,
		Version: appVersion,
		Enabled: true,
		MountPoints: []appregistry.MountPoint{
			{Type: "project-tab", ID: "material", Label: "素材", View: "MediaMaterialTab"},
			{Type: "project-tab", ID: "pipeline", Label: "阶段追踪", View: "MediaPipelineTab"},
		},
		TaskTypes:    []string{FnSilenceDetect, FnTrim},
		DomainTables: []string{"media_content_project", "media_material", "media_segment"},
	})

	// 2. Domain tables (idempotent; never bumps global schemaVersion).
	if err := EnsureTables(); err != nil {
		log.Printf("[media] ensure domain tables: %v", err)
	}

	// 3. Function handlers — pure in-process, no live API needed.
	registerFunctions()

	// 4. Project template — scaffolds 素材库/剪辑/发布 dirs + 媒体助手 preset config.
	templateregistry.Register(templateregistry.ProjectTemplate{
		ID:        templateID,
		Name:      "自媒体内容项目",
		AppID:     AppID,
		Subdirs:   []string{"素材库", "剪辑", "发布"},
		DomainDDL: domainDDLs,
		PresetConfig: templateregistry.ProjectConfig{
			Instructions: "你是媒体内容助手。围绕选题→素材→剪辑脚本→包装→分发→回流推进。" +
				"素材处理走静音检测(function)→智能剪辑(agent)→段落终审(human)三态管线。",
			Connectors: []string{},
			Experts:    []string{},
			Skills:     []string{},
			Automation: []string{},
		},
	})

	// 5. Runtime registration that needs the live API (RegisterApp permission
	//    allowlist + completion writeback hook + store handle for the human gate).
	appkit.OnInit(func(api *taskapi.API) {
		store := mediaStore()
		setRuntime(api, store)

		api.RegisterApp(taskapi.AppPermissions{
			Namespace:    AppID,
			AllowedTypes: []string{FnSilenceDetect, FnTrim},
			AllowedRefs:  []string{AppID + ":"},
		})
		api.RegisterCompletionHook(onTaskCompletion)
	})
}

// mediaStore builds a TaskStore over the default meta DB for the human-gate
// completion path (the API exposes no public status setter).
func mediaStore() *meta.TaskStore {
	db, err := meta.OpenDefault()
	if err != nil {
		log.Printf("[media] open db for task store: %v", err)
		return nil
	}
	return meta.NewTaskStore(db)
}
