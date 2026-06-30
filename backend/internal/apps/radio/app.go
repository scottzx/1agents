package radio

import (
	"encoding/json"
	"log"

	"github.com/scottzx/1Agents/backend/internal/appkit"
	"github.com/scottzx/1Agents/backend/internal/appregistry"
	"github.com/scottzx/1Agents/backend/internal/meta"
	"github.com/scottzx/1Agents/backend/internal/taskapi"
)

// App is the radio backend module. It holds the North Task API handle and the
// domain store. It is constructed once at startup (via appkit.OnInit) and owns
// the function handler + completion-writeback hook.
type App struct {
	api   *taskapi.API
	store *Store
}

// singleton wired by Init; the HTTP handler (handler.go) also reads it.
var app *App

// Manifest is the app's canonical manifest (§2 of the SDK contract). Enabled is
// false — radio is OPT-IN per #344; the user toggles it on in app settings.
func Manifest() appregistry.AppManifest {
	return appregistry.AppManifest{
		ID:      AppID,
		Name:    "AI 电台",
		Version: "1.0.0",
		Enabled: false, // opt-in (#344)
		MountPoints: []appregistry.MountPoint{
			{Type: "l1-page", ID: "player", Label: "AI 电台", View: "RadioPage", Icon: "radio"},
		},
		TaskTypes:    []string{TTSFunctionType},
		DomainTables: []string{"radio_episode"},
	}
}

func init() {
	// Manifest + domain-table registration need no API instance → plain init().
	appregistry.Register(Manifest())

	if err := appregistry.EnsureDomainTables(AppID, schemaDDL); err != nil {
		log.Printf("[radio] ensure domain tables: %v", err)
	}

	// Runtime registration that needs the live API → appkit.OnInit.
	appkit.OnInit(func(api *taskapi.API) {
		Init(api)
	})
}

// Init wires the radio app against the live North Task API. Called once at
// startup by appkit.RunInits. It is exported so tests can construct an App
// against a test API/store without the package init() path.
func Init(api *taskapi.API) *App {
	db, err := meta.OpenDefault()
	if err != nil {
		log.Printf("[radio] open db: %v", err)
		return nil
	}
	a := NewApp(api, NewStore(db))
	app = a
	return a
}

// NewApp builds an App and registers its permissions, function handler and
// completion hook against api. Used both by Init and by unit tests.
func NewApp(api *taskapi.API, store *Store) *App {
	a := &App{api: api, store: store}

	// 6. Declare taskTypes permission (contract checklist step 6 / R3).
	api.RegisterApp(taskapi.AppPermissions{
		Namespace:    AppID,
		AllowedTypes: []string{TTSFunctionType},
		AllowedRefs:  []string{AppID + ":"},
	})

	// 4a. Register the function handler (contract step 4 / R5).
	taskapi.RegisterFunction(TTSFunctionType, a.ttsHandler)

	// 4b. Register the completion-writeback hook (contract step 4 / R5).
	api.RegisterCompletionHook(a.onTaskComplete)

	return a
}

// onTaskComplete is the writeback hook. It claims ONLY tasks whose business_ref
// is in the "radio:" namespace, then advances the episode status / persists the
// stage result into the radio_episode domain row. Results never auto-flow into
// domain tables — the kernel only notifies; this hook does the writeback.
func (a *App) onTaskComplete(ev taskapi.CompletionEvent) {
	t, ok, err := a.api.QueryTask(ev.TaskID)
	if err != nil || !ok {
		return
	}
	episodeID, mine := parseEpisodeRef(t.BusinessRef)
	if !mine {
		return // not a radio task — ignore (contract: claim by business_ref)
	}

	if ev.Status != meta.TaskStatusCompleted {
		// A failed/cancelled stage leaves the episode in its current stage so
		// the UI can show the stuck stage. Nothing to write back.
		return
	}

	switch t.Milestone {
	case StageSummarize:
		// The agent's result is its terminal output; persist as summary, then
		// advance status to the next stage.
		if s := extractAgentText(ev.Result); s != "" {
			_ = a.store.SetSummary(episodeID, s)
		}
		_ = a.store.SetStatus(episodeID, StatusTranscribing)

	case StageTranscript:
		if s := extractAgentText(ev.Result); s != "" {
			_ = a.store.SetTranscript(episodeID, s)
		}
		_ = a.store.SetStatus(episodeID, StatusSynthesizing)

	case StageSynthesize:
		// The function result carries the audio path + duration.
		var r struct {
			AudioPath string `json:"audioPath"`
			Duration  int    `json:"duration"`
		}
		if err := json.Unmarshal([]byte(ev.Result), &r); err == nil && r.AudioPath != "" {
			_ = a.store.SetAudio(episodeID, r.AudioPath, r.Duration)
		} else {
			_ = a.store.SetStatus(episodeID, StatusReady)
		}
	}
}

// extractAgentText pulls human-readable text out of an agent task result. Agent
// results may be plain text or a JSON envelope; we handle both leniently.
func extractAgentText(result string) string {
	if result == "" || result == "null" {
		return ""
	}
	// Try a few common JSON envelope shapes; fall back to the raw string.
	var env struct {
		Text    string `json:"text"`
		Summary string `json:"summary"`
		Output  string `json:"output"`
		Result  string `json:"result"`
	}
	if err := json.Unmarshal([]byte(result), &env); err == nil {
		for _, s := range []string{env.Text, env.Summary, env.Output, env.Result} {
			if s != "" {
				return s
			}
		}
		// Valid JSON but none of the known fields → not human text; keep raw.
	}
	return result
}
