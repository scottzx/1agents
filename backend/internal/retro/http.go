package retro

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/scottzx/1Agents/backend/internal/kwiki"
)

// Item is one retrospective surfaced to the frontend: the wiki page's metadata
// plus its Markdown body. It mirrors the fields the RetroPane renders.
type Item struct {
	Slug    string   `json:"slug"`
	Title   string   `json:"title"`
	Summary string   `json:"summary,omitempty"`
	Tags    []string `json:"tags,omitempty"`
	Body    string   `json:"body"`
	Created string   `json:"created,omitempty"`
	Updated string   `json:"updated,omitempty"`
}

// isRetro reports whether a wiki page is a #144 retrospective: source "retro"
// or carrying the "retrospective" tag.
func isRetro(p kwiki.WikiPage) bool {
	if p.Source == "retro" {
		return true
	}
	for _, t := range p.Tags {
		if t == "retrospective" {
			return true
		}
	}
	return false
}

func toItem(p kwiki.WikiPage) Item {
	it := Item{
		Slug:    p.Slug,
		Title:   p.Title,
		Summary: p.Summary,
		Tags:    p.Tags,
		Body:    p.Body,
	}
	if !p.Created.IsZero() {
		it.Created = p.Created.UTC().Format("2006-01-02T15:04:05Z07:00")
	}
	if !p.Updated.IsZero() {
		it.Updated = p.Updated.UTC().Format("2006-01-02T15:04:05Z07:00")
	}
	return it
}

// Handler serves the #271 retrospective read API over the shared kwiki store:
//
//	GET /api/retrospectives        → {items: [...]} list of all retrospective pages
//	GET /api/retrospectives/{slug} → one retrospective page (404 if not a retro)
//
// It is read-only; retrospectives are written by the project-archive hook (#144).
func Handler(store *kwiki.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		slug := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/retrospectives"), "/")
		pages, err := store.Pages()
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		if slug == "" {
			items := []Item{}
			for _, p := range pages {
				if isRetro(p) {
					items = append(items, toItem(p))
				}
			}
			writeJSON(w, map[string]any{"items": items})
			return
		}

		for _, p := range pages {
			if p.Slug == slug && isRetro(p) {
				writeJSON(w, toItem(p))
				return
			}
		}
		http.Error(w, "retrospective not found", http.StatusNotFound)
	}
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}
