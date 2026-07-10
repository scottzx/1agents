// Package speechclip is the Vlog & Clip content-studio app: a project-scoped
// pipeline over agentsOS — 录屏/导入 → 转录(FunClip) → 提金句/纠错(1acp agent) → 混剪 → 成片.
//
// Domain data is filesystem-only (jsonl/json under <workspace>/.artifacts/speech_clip/),
// so the manifest declares no domain tables. Both heavy steps (transcribe, highlight)
// run through the task kernel so status + results are visible in the task list.
//
// Registration split (per appkit's contract):
//   - manifest + project template register in plain init() (no API instance needed)
//   - executor=function handler registers in init() too (RegisterFunction is global)
//   - completion-hook pipeline glue registers via appkit.OnInit (needs live *taskapi.API)
package speechclip

import (
	"github.com/scottzx/1Agents/backend/internal/appregistry"
	"github.com/scottzx/1Agents/backend/internal/templateregistry"
)

// AppID is the manifest id (kept snake_case to match the frontend view + mount).
const AppID = "speech_clip"

func init() {
	appregistry.Register(appregistry.AppManifest{
		ID:      AppID,
		Name:    "Vlog & Clip",
		Version: "0.1.0",
		Enabled: true,
		MountPoints: []appregistry.MountPoint{{
			Type:  "project-tab",
			ID:    "pipeline",
			Label: "内容工作室",
			View:  "ContentStudioTab",
		}},
		TaskTypes:    []string{"speech_clip.transcribe", "speech_clip.extract_highlights"},
		DomainTables: nil, // 文件系统(jsonl/json), 无域表
	})

	templateregistry.Register(templateregistry.ProjectTemplate{
		ID:        "speech_clip.project",
		Name:      "Vlog & Clip 项目",
		AppID:     AppID,
		Subdirs:   []string{"assets", "transcripts", "clips", "output", "timelines"},
		DomainDDL: nil,
		PresetConfig: templateregistry.ProjectConfig{
			Instructions: "Vlog 与短视频剪辑项目：录屏/导入 → FunClip 转录 → 1acp 提金句/纠错 → 跨素材混剪 → 成片。",
		},
	})
}
