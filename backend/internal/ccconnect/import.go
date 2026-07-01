package ccconnect

import (
	"encoding/json"
	"fmt"
	"hash/fnv"
	"log"
	"net/http"
	"path/filepath"
	"strings"

	"github.com/chenhg5/cc-connect/config"
	"github.com/chenhg5/cc-connect/core"
	"github.com/scottzx/1Agents/backend/internal/meta"
)

// import.go implements incremental import from an EXTERNAL cc-connect config
// (e.g. a globally-installed cc-connect's ~/.cc-connect/config.toml) into
// 1agents' own decoupled stores. Matching is by work_dir PATH — the path decides
// the workspace/folder; the source project NAME is never a match key. Only the
// delta is added: existing workspaces (meta.db) and channels (im_channels
// config) are never removed or overwritten.

// ImportResult summarizes one incremental import run.
type ImportResult struct {
	Source          string   `json:"source"`
	ProjectsAdded   int      `json:"projects_added"`
	ChannelsAdded   int      `json:"channels_added"`
	ChannelsSkipped int      `json:"channels_skipped"`
	Notes           []string `json:"notes,omitempty"`
}

// channelIdentity returns a stable dedup key for a channel within a project: its
// type plus the first present id-like option (app_id, bot_token, …). This is the
// "渠道 ID" used for incremental matching so re-importing is idempotent and two
// distinct bots of the same type never collapse. Falls back to the rendered
// options when no known id field is present.
func channelIdentity(pc config.PlatformConfig) string {
	for _, k := range []string{
		"app_id", "bot_token", "token", "app_key", "corp_id",
		"agent_id", "webhook", "bot_id", "access_token", "appid",
	} {
		if v, ok := pc.Options[k].(string); ok && strings.TrimSpace(v) != "" {
			return pc.Type + "|" + k + "=" + strings.TrimSpace(v)
		}
	}
	return pc.Type + "|" + fmt.Sprintf("%v", pc.Options)
}

func shortHash(s string) string {
	h := fnv.New32a()
	_, _ = h.Write([]byte(s))
	return fmt.Sprintf("%08x", h.Sum32())
}

func uniqueProjectID(existing map[string]bool, base string) string {
	if base == "" {
		base = "ws"
	}
	id := base
	for i := 1; existing[id]; i++ {
		id = fmt.Sprintf("%s-%d", base, i)
	}
	return id
}

// ImportFromCCConfig incrementally merges projects + channels from the source
// cc-connect config at sourcePath (default: the legacy shared
// ~/.cc-connect/config.toml) into 1agents. For each source project with a
// work_dir it ensures a meta.db workspace at that path (created if missing, name
// = path basename) and appends any of the project's channels that aren't already
// present (by channelIdentity) to the matching 1agents (im_channels) config
// project. Triggers a cc-connect hot reload when anything changed.
func ImportFromCCConfig(sourcePath string) (ImportResult, error) {
	res := ImportResult{}
	if strings.TrimSpace(sourcePath) == "" {
		sourcePath = legacyCCConfigPath()
	}
	res.Source = sourcePath

	src, err := loadConfigForChannels(sourcePath)
	if err != nil {
		return res, fmt.Errorf("load source config %s: %w", sourcePath, err)
	}
	targetPath := configFilePath()
	tgt, err := loadConfigForChannels(targetPath)
	if err != nil {
		return res, fmt.Errorf("load target config %s: %w", targetPath, err)
	}

	db, err := meta.OpenDefault()
	if err != nil {
		return res, err
	}

	// Index EXISTING target projects by normalized work_dir path (stable pointers
	// — we only mutate these in place; brand-new projects are collected
	// separately and appended after the loop to avoid slice-realloc dangling).
	tgtByPath := make(map[string]*config.ProjectConfig, len(tgt.Projects))
	for i := range tgt.Projects {
		if wd, _ := tgt.Projects[i].Agent.Options["work_dir"].(string); wd != "" {
			tgtByPath[normalizePath(wd)] = &tgt.Projects[i]
		}
	}

	// Snapshot existing workspace paths + ids from meta.db.
	wsProjects, err := db.ListWorkspaceProjects()
	if err != nil {
		return res, err
	}
	wsByPath := make(map[string]bool, len(wsProjects))
	idsInUse := make(map[string]bool, len(wsProjects))
	for _, p := range wsProjects {
		wsByPath[normalizePath(p.WorkspacePath)] = true
		idsInUse[p.ID] = true
	}

	var newProjects []config.ProjectConfig
	changed := false

	for _, sp := range src.Projects {
		wd, _ := sp.Agent.Options["work_dir"].(string)
		if strings.TrimSpace(wd) == "" {
			continue // platform-only/placeholder source projects aren't workspaces
		}
		key := normalizePath(wd)

		// 1) Ensure a meta.db workspace at this path (path decides the name).
		if !wsByPath[key] {
			base := sanitizeID(filepath.Base(wd))
			if base == "" {
				base = "ws-" + shortHash(key)
			}
			id := uniqueProjectID(idsInUse, base)
			if err := db.EnsureWorkspaceProject(meta.Project{
				ID:            id,
				Name:          filepath.Base(wd),
				WorkspacePath: wd,
				DefaultAgent:  sp.Agent.Type,
			}); err != nil {
				res.Notes = append(res.Notes, fmt.Sprintf("skip %s: create workspace: %v", wd, err))
				continue
			}
			wsByPath[key] = true
			idsInUse[id] = true
			res.ProjectsAdded++
		}

		// 2) Merge channels into the matching target config project (by path).
		tp := tgtByPath[key]
		isNew := false
		if tp == nil {
			// Mirror the source project's agent + work_dir; start with no
			// channels and fill from source below. Name is a display slug only.
			slug := CCProjectSlug(filepath.Base(wd))
			np := config.ProjectConfig{
				Name:  slug,
				Agent: sp.Agent,
			}
			if np.Agent.Options == nil {
				np.Agent.Options = map[string]any{}
			}
			np.Agent.Options["work_dir"] = wd
			tp = &np
			isNew = true
		}

		have := make(map[string]bool, len(tp.Platforms))
		for _, pc := range tp.Platforms {
			have[channelIdentity(pc)] = true
		}
		for _, pc := range sp.Platforms {
			cid := channelIdentity(pc)
			if have[cid] {
				res.ChannelsSkipped++
				continue
			}
			tp.Platforms = append(tp.Platforms, pc)
			have[cid] = true
			res.ChannelsAdded++
			changed = true
		}

		if isNew {
			// Guarantee a bridge channel so the project is valid + reachable.
			hasBridge := false
			for _, pc := range tp.Platforms {
				if pc.Type == "bridge" {
					hasBridge = true
					break
				}
			}
			if !hasBridge {
				tp.Platforms = append(tp.Platforms, config.PlatformConfig{Type: "bridge"})
			}
			newProjects = append(newProjects, *tp)
			changed = true
		}
	}

	if changed {
		tgt.Projects = append(tgt.Projects, newProjects...)
		if err := saveConfig(tgt, targetPath); err != nil {
			return res, fmt.Errorf("save target config: %w", err)
		}
		select {
		case core.RestartCh <- core.RestartRequest{}:
		default:
		}
		log.Printf("[ccconnect] import from %s: +%d projects, +%d channels (%d skipped)",
			sourcePath, res.ProjectsAdded, res.ChannelsAdded, res.ChannelsSkipped)
	}
	return res, nil
}

// ImportHandler serves POST /api/cc-connect/import. Body: {"source_path": "..."}
// (optional; defaults to the shared ~/.cc-connect/config.toml). Returns the
// ImportResult. GET returns the default source path so the UI can preview it.
func ImportHandler(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		writeJSONImport(w, map[string]any{"default_source": legacyCCConfigPath()})
	case http.MethodPost:
		var body struct {
			SourcePath string `json:"source_path"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		res, err := ImportFromCCConfig(body.SourcePath)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSONImport(w, res)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func writeJSONImport(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}
