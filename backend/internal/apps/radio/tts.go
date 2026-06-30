package radio

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/scottzx/1Agents/backend/internal/domainstore"
	"github.com/scottzx/1Agents/backend/internal/taskapi"
)

// TTSConfig is the external-TTS seam. Phase 1 ships a deterministic STUB
// synthesizer (no network, token≈0). To wire a real provider (MiniMax, Azure,
// ElevenLabs, …), set Synthesize to a function that calls the provider and
// returns the raw audio bytes + a file extension. The function-handler path,
// artifact storage, status writeback and streaming endpoint stay unchanged.
type TTSConfig struct {
	// Synthesize turns transcript text into audio bytes. Returns the bytes,
	// the file extension (without dot, e.g. "mp3"/"wav"), an estimated duration
	// in seconds, and the token cost (0 for local stubs).
	Synthesize func(text string) (audio []byte, ext string, durationSec int, costTokens int64, err error)
}

// stubSynthesize is the deterministic Phase-1 stub: it writes a small text
// marker "audio" file derived from the transcript. No external call, token≈0.
// It proves the whole pipeline (dispatch → function → artifact → stream →
// player) end-to-end without depending on a paid TTS provider.
func stubSynthesize(text string) ([]byte, string, int, int64, error) {
	marker := "RADIO-TTS-STUB\n\n" + text
	// ~3 words/second of speech as a rough duration estimate.
	words := len(strings.Fields(text))
	dur := words / 3
	if dur < 1 {
		dur = 1
	}
	return []byte(marker), "txt", dur, 0, nil
}

// defaultTTS is the active configuration. Swap defaultTTS.Synthesize at startup
// (via SetTTS) to plug a real provider in.
var defaultTTS = TTSConfig{Synthesize: stubSynthesize}

// SetTTS overrides the synthesizer (real external TTS seam). Call before the
// pipeline runs. Pass nil to keep the deterministic stub.
func SetTTS(fn func(text string) ([]byte, string, int, int64, error)) {
	if fn == nil {
		return
	}
	defaultTTS.Synthesize = fn
}

// ttsHandler is the radio.tts_synthesize function handler. It is deterministic
// (token≈0 with the stub) and writes the audio artifact to the FILE face; the
// domain row only ever gets the path (via the completion hook).
//
// Result JSON shape: {"episodeId":N,"audioPath":"...","duration":S}.
func (a *App) ttsHandler(ctx taskapi.FunctionContext) (any, error) {
	episodeID, ok := parseEpisodeRef(ctx.Task.BusinessRef)
	if !ok {
		return nil, fmt.Errorf("radio: tts: bad business_ref %q", ctx.Task.BusinessRef)
	}

	ep, found, err := a.store.GetEpisode(episodeID)
	if err != nil {
		return nil, fmt.Errorf("radio: tts: load episode %d: %w", episodeID, err)
	}
	if !found {
		return nil, fmt.Errorf("radio: tts: episode %d not found", episodeID)
	}

	ws, _, err := a.store.GetWorkspace(episodeID)
	if err != nil {
		return nil, fmt.Errorf("radio: tts: load workspace: %w", err)
	}
	if ws == "" {
		return nil, fmt.Errorf("radio: tts: episode %d has no workspace", episodeID)
	}

	text := ep.Transcript
	if text == "" {
		// Pipeline ran without a real agent populating the transcript (e.g.
		// tests / stubbed agents). Fall back to summary or title so the stub
		// still produces a deterministic artifact.
		if ep.Summary != "" {
			text = ep.Summary
		} else {
			text = ep.Title
		}
	}

	audio, ext, dur, cost, err := defaultTTS.Synthesize(text)
	if err != nil {
		return nil, fmt.Errorf("radio: tts: synthesize: %w", err)
	}
	ctx.CostTokens = cost // token≈0 for the stub

	dir, err := domainstore.ArtifactDir(ws, AppID, "audio")
	if err != nil {
		return nil, fmt.Errorf("radio: tts: artifact dir: %w", err)
	}
	fileName := fmt.Sprintf("episode-%d.%s", episodeID, ext)
	abs := filepath.Join(dir, fileName)
	if err := os.WriteFile(abs, audio, 0o644); err != nil {
		return nil, fmt.Errorf("radio: tts: write audio: %w", err)
	}

	rel, err := domainstore.RelativePath(ws, abs)
	if err != nil {
		rel = abs // fall back to absolute; non-fatal
	}

	return map[string]any{
		"episodeId": episodeID,
		"audioPath": rel,
		"duration":  dur,
	}, nil
}
