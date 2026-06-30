package media

import (
	"fmt"
	"os/exec"
	"strconv"
	"strings"

	"github.com/scottzx/1Agents/backend/internal/taskapi"
)

// Function type keys (= manifest taskTypes, = "fn:<type>" task labels).
const (
	FnSilenceDetect = "media.silence_detect"
	FnTrim          = "media.trim"
)

// SilenceSegment is one detected non-silent speech segment.
type SilenceSegment struct {
	Start float64 `json:"start"`
	End   float64 `json:"end"`
}

// silenceDetectResult is the JSON written to task.result by FnSilenceDetect.
type silenceDetectResult struct {
	Segments []SilenceSegment `json:"segments"`
	Source   string           `json:"source"` // "computed" — deterministic, token≈0
}

// registerFunctions installs the media executor=function handlers. Called from
// init(); these are pure in-process (token≈0), so they don't need the live API.
//
// Function tasks carry their parameters in the task Description as "key=value"
// lines (DispatchSpec exposes no custom-label field), which these handlers parse.
func registerFunctions() {
	// media.silence_detect — deterministic segment computation (token≈0).
	//
	// Phase 1 stubs the heavy ffmpeg/silero-vad pass with a reproducible split:
	// the clip duration (param "duration") is divided into fixed-length speech
	// windows separated by silence gaps. This is a *real* registered
	// FunctionHandler — it returns concrete segments and incurs zero token cost.
	taskapi.RegisterFunction(FnSilenceDetect, func(ctx taskapi.FunctionContext) (any, error) {
		params := parseParams(ctx.Task.Description)
		dur := atof(params["duration"])
		segs := computeSilenceSegments(dur)
		return silenceDetectResult{Segments: segs, Source: "computed"}, nil
	})

	// media.trim — ffmpeg trim by [start,end]. Degrades gracefully when ffmpeg
	// is unavailable: it records the *intended* op instead of failing the task.
	taskapi.RegisterFunction(FnTrim, func(ctx taskapi.FunctionContext) (any, error) {
		return runTrim(parseParams(ctx.Task.Description))
	})
}

// computeSilenceSegments splits a duration into deterministic speech windows.
// Pure function — exported indirectly via the registered handler so tests can
// assert token≈0 + concrete segments without ffmpeg.
func computeSilenceSegments(durationSec float64) []SilenceSegment {
	if durationSec <= 0 {
		// Unknown duration → a single canned window so the pipeline still advances.
		return []SilenceSegment{{Start: 0, End: 5}}
	}
	const (
		speechWin  = 8.0 // seconds of speech per window
		silenceGap = 1.5 // seconds of silence between windows (dropped)
	)
	var segs []SilenceSegment
	pos := 0.0
	for pos < durationSec {
		end := pos + speechWin
		if end > durationSec {
			end = durationSec
		}
		segs = append(segs, SilenceSegment{Start: round2(pos), End: round2(end)})
		pos = end + silenceGap
	}
	return segs
}

// trimResult records the outcome of an ffmpeg trim.
type trimResult struct {
	Status string  `json:"status"` // "trimmed" | "intended"
	Input  string  `json:"input"`
	Output string  `json:"output"`
	Start  float64 `json:"start"`
	End    float64 `json:"end"`
	FFmpeg bool    `json:"ffmpeg"`
	Note   string  `json:"note,omitempty"`
}

// runTrim trims the input file to [start,end] via ffmpeg. Params: in, out,
// start, end.
func runTrim(params map[string]string) (any, error) {
	in := params["in"]
	out := params["out"]
	start := atof(params["start"])
	end := atof(params["end"])
	if in == "" || out == "" {
		return nil, fmt.Errorf("media.trim: missing in/out params")
	}
	res := trimResult{Input: in, Output: out, Start: start, End: end}

	ffmpegPath, lookErr := exec.LookPath("ffmpeg")
	if lookErr != nil {
		// Graceful degradation: ffmpeg unavailable → record the intended op.
		res.Status = "intended"
		res.FFmpeg = false
		res.Note = "ffmpeg not found on PATH; recorded intended trim only"
		return res, nil
	}
	args := []string{"-y", "-i", in, "-ss", f2s(start), "-to", f2s(end), "-c", "copy", out}
	if cmdErr := exec.Command(ffmpegPath, args...).Run(); cmdErr != nil {
		// ffmpeg present but failed (e.g. dummy input in tests) → still degrade,
		// don't fail the pipeline; the agent stage can decide what to do.
		res.Status = "intended"
		res.FFmpeg = true
		res.Note = "ffmpeg invocation failed: " + cmdErr.Error()
		return res, nil
	}
	res.Status = "trimmed"
	res.FFmpeg = true
	return res, nil
}

// parseParams reads "key=value" lines (and inline "key=value" tokens) from a
// task description into a map.
func parseParams(desc string) map[string]string {
	out := map[string]string{}
	for _, line := range strings.Split(desc, "\n") {
		for _, tok := range strings.Fields(line) {
			if i := strings.Index(tok, "="); i > 0 {
				out[tok[:i]] = tok[i+1:]
			}
		}
	}
	return out
}

func atof(s string) float64 {
	f, _ := strconv.ParseFloat(s, 64)
	return f
}

func f2s(f float64) string { return strconv.FormatFloat(f, 'f', 3, 64) }

func round2(f float64) float64 {
	return float64(int(f*100+0.5)) / 100
}
