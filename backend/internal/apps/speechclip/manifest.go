// Package speechclip is the 口播剪辑 (voiceover-clip) business app: a project-scoped
// pipeline over agentsOS — 录屏 → 转录(FunClip) → 提金句/纠错(1acp agent) → 混剪 → 成片.
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
		Name:    "口播剪辑",
		Version: "0.1.0",
		Enabled: true,
		MountPoints: []appregistry.MountPoint{{
			Type:  "project-tab",
			ID:    "pipeline",
			Label: "口播剪辑",
			View:  "SpeechClipTab",
		}},
		TaskTypes:    []string{"speech_clip.transcribe", "speech_clip.extract_highlights"},
		DomainTables: nil, // 文件系统(jsonl/json), 无域表
	})

	templateregistry.Register(templateregistry.ProjectTemplate{
		ID:        "speech_clip.project",
		Name:      "口播剪辑项目",
		AppID:     AppID,
		Subdirs:   []string{"assets", "transcripts", "output"},
		DomainDDL: nil,
		PresetConfig: templateregistry.ProjectConfig{
			Instructions: "口播视频剪辑项目：多素材录屏 → FunClip 转录 → 1acp 提金句/纠错 → 跨素材混剪 → 成片。",
		},
	})
}
