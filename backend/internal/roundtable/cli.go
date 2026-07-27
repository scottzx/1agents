package roundtable

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/scottzx/1Agents/backend/internal/meta"
)

// EnvRoundtableRoomID is injected (or set by seat harness) so CLI works across cwd.
const EnvRoundtableRoomID = "ONEAGENTS_ROUNDTABLE_ROOM_ID"

// EnvCLI is the absolute path to the 1agents binary (same as project-items / PM skill).
// Dev environments often lack `1agents` on PATH; set this or use the path from help.
const EnvCLI = "ONEAGENTS_CLI"

// SeatSidecarFile is written into each seat cwd at CreateRoom so agents can resolve room_id.
const SeatSidecarFile = ".1agents-roundtable.json"

// SeatSidecar is the on-disk context for a seat workspace (design: 跨 cwd 同步).
type SeatSidecar struct {
	RoomID string `json:"room_id"`
	Role   string `json:"role"`
	SeatID string `json:"seat_id,omitempty"`
	// CLIBin is the absolute 1agents binary path at seed time (dev: often not on PATH).
	CLIBin string `json:"cli_bin,omitempty"`
}

// ResolveCLIBinary returns the 1agents binary path agents should invoke.
// Priority matches project-items / PM skill: ONEAGENTS_CLI → os.Executable() → "1agents".
func ResolveCLIBinary() string {
	if cli := strings.TrimSpace(os.Getenv(EnvCLI)); cli != "" {
		return cli
	}
	exe, err := os.Executable()
	if err != nil || exe == "" {
		return "1agents"
	}
	if resolved, err := filepath.EvalSymlinks(exe); err == nil {
		exe = resolved
	}
	return exe
}

// RoundtableCLI returns a shell-safe prefix like `/abs/path/1agents roundtable`
// (or `1agents roundtable` when the binary cannot be resolved).
// Same idea as agent.projectItemsCLI / workspace.rewriteCLIBinaryPath.
func RoundtableCLI() string {
	return shellQuote(ResolveCLIBinary()) + " roundtable"
}

// formatUsage builds help text with the resolved binary (not a bare `1agents`
// that may be missing from PATH in local/dev builds).
func formatUsage() string {
	rt := RoundtableCLI()
	bin := ResolveCLIBinary()
	return strings.TrimSpace(fmt.Sprintf(`
usage:
  %s help
  %s get [--room ID] [--json]
  %s propose-brief [--room ID] [--expected-version N]
      --title T --question Q --constraints C --success-criteria S
      [--product-kind software|hardware|hybrid] [--source-turn ID] [--json]
  %s propose-brief [--room ID] --from-json '{...}' [--json]

compatibility / administration only (deprecated for agents):
  %s set-brief [--room ID]
      --title T --question Q --constraints C --success-criteria S
      [--product-kind software|hardware|hybrid] [--json]
  %s set-brief [--room ID] --from-json '{...}' [--json]
  set-brief preserves the pre-versioning one-shot set+confirm behavior. Agents
  must migrate to propose-brief; only a user may call the confirm API.

binary: %s
  Dev tip: local builds often have no "1agents" on PATH. Prefer the absolute
  path above (this binary), or export %s=/path/to/1agents (same as project-items).
  Seat sidecars also store cli_bin when rooms are created by a running daemon.

room_id resolution (first match wins):
  1. --room <id>
  2. env %s
  3. %s in cwd (or ancestors)
  4. reverse-lookup seat by workspace path matching cwd

Writes go directly to ~/.1agents/meta.db (daemon not required).
`, rt, rt, rt, rt, rt, rt, bin, EnvCLI, EnvRoundtableRoomID, SeatSidecarFile))
}

// RunCLI dispatches `1agents roundtable …` verbs. Returns process exit code.
func RunCLI(args []string) int {
	if len(args) == 0 {
		fmt.Println(formatUsage())
		return 1
	}
	switch args[0] {
	case "help", "-h", "--help":
		fmt.Println(formatUsage())
		return 0
	case "get":
		return cliGet(args[1:])
	case "propose-brief":
		return cliWriteBrief(args[1:], true)
	case "set-brief":
		return cliWriteBrief(args[1:], false)
	default:
		fmt.Fprintf(os.Stderr, "error: unknown roundtable verb %q\n\n%s\n", args[0], formatUsage())
		return 1
	}
}

func cliGet(args []string) int {
	fs := flag.NewFlagSet("roundtable get", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	roomFlag := fs.String("room", "", "room id (optional if env/sidecar/cwd resolve)")
	asJSON := fs.Bool("json", false, "machine-readable output")
	if err := fs.Parse(args); err != nil {
		return 1
	}

	store, err := NewStore()
	if err != nil {
		return cliFail("open store: %v", err)
	}
	roomID, err := ResolveRoomID(store, *roomFlag, ".")
	if err != nil {
		return cliFail("%v", err)
	}
	room, err := store.GetRoom(roomID)
	if err != nil {
		if err == meta.ErrNotFound {
			return cliFail("room %s not found", roomID)
		}
		return cliFail("get room: %v", err)
	}
	seats, err := store.ListSeats(roomID)
	if err != nil {
		return cliFail("list seats: %v", err)
	}
	room.Seats = seats

	if *asJSON {
		return cliPrintJSON(room)
	}
	fmt.Printf("id:    %s\n", room.ID)
	fmt.Printf("title: %s\n", room.Title)
	fmt.Printf("state: %s\n", room.State)
	if room.Brief != nil {
		fmt.Printf("brief:\n")
		fmt.Printf("  title:            %s\n", room.Brief.Title)
		fmt.Printf("  question:         %s\n", room.Brief.Question)
		fmt.Printf("  constraints:      %s\n", room.Brief.Constraints)
		fmt.Printf("  success_criteria: %s\n", room.Brief.SuccessCriteria)
		if room.Brief.ProductKind != "" {
			fmt.Printf("  product_kind:     %s\n", room.Brief.ProductKind)
		}
	} else {
		fmt.Printf("brief: (none)\n")
	}
	return 0
}

func cliWriteBrief(args []string, propose bool) int {
	verb := "set-brief"
	if propose {
		verb = "propose-brief"
	}
	fs := flag.NewFlagSet("roundtable "+verb, flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	roomFlag := fs.String("room", "", "room id (optional if env/sidecar/cwd resolve)")
	title := fs.String("title", "", "brief title")
	question := fs.String("question", "", "brief question")
	constraints := fs.String("constraints", "", "brief constraints")
	success := fs.String("success-criteria", "", "brief success_criteria")
	productKind := fs.String("product-kind", "", "optional software|hardware|hybrid")
	fromJSON := fs.String("from-json", "", "JSON object with title/question/constraints/success_criteria[/product_kind]")
	expectedVersion := fs.Int("expected-version", -1, "current version this write is based on (defaults to a fresh read)")
	sourceTurnID := fs.String("source-turn", "", "optional referee source turn id")
	asJSON := fs.Bool("json", false, "print updated room as JSON")
	if err := fs.Parse(args); err != nil {
		return 1
	}

	req := ConfirmBriefRequest{
		Title:           *title,
		Question:        *question,
		Constraints:     *constraints,
		SuccessCriteria: *success,
		ProductKind:     ProductKind(*productKind),
	}
	if strings.TrimSpace(*fromJSON) != "" {
		var body struct {
			Title           string      `json:"title"`
			Question        string      `json:"question"`
			Constraints     string      `json:"constraints"`
			SuccessCriteria string      `json:"success_criteria"`
			ProductKind     ProductKind `json:"product_kind"`
			ExpectedVersion *int        `json:"expected_version"`
			SourceTurnID    string      `json:"source_turn_id"`
		}
		if err := json.Unmarshal([]byte(*fromJSON), &body); err != nil {
			return cliFail("parse --from-json: %v", err)
		}
		// Explicit flags override JSON fields when non-empty.
		if req.Title == "" {
			req.Title = body.Title
		}
		if req.Question == "" {
			req.Question = body.Question
		}
		if req.Constraints == "" {
			req.Constraints = body.Constraints
		}
		if req.SuccessCriteria == "" {
			req.SuccessCriteria = body.SuccessCriteria
		}
		if req.ProductKind == "" {
			req.ProductKind = body.ProductKind
		}
		if *expectedVersion < 0 && body.ExpectedVersion != nil {
			*expectedVersion = *body.ExpectedVersion
		}
		if *sourceTurnID == "" {
			*sourceTurnID = body.SourceTurnID
		}
	}
	if req.Title == "" || req.Question == "" || req.Constraints == "" || req.SuccessCriteria == "" {
		return cliFail("requires --title --question --constraints --success-criteria (or --from-json with those fields)\n%s", formatUsage())
	}

	store, err := NewStore()
	if err != nil {
		return cliFail("open store: %v", err)
	}
	roomID, err := ResolveRoomID(store, *roomFlag, ".")
	if err != nil {
		return cliFail("%v", err)
	}

	svc := NewService(store, nil, &StaticSeatPrompter{})
	var room *Room
	if propose {
		if *expectedVersion < 0 {
			current, getErr := store.GetRoom(roomID)
			if getErr != nil {
				return cliFail("get room: %v", getErr)
			}
			*expectedVersion = current.CurrentBriefVersion
		}
		room, err = svc.ProposeBrief(roomID, ProposeBriefRequest{
			ConfirmBriefRequest: req,
			ExpectedVersion:     *expectedVersion,
			SourceTurnID:        *sourceTurnID,
		})
	} else {
		room, err = svc.ConfirmBrief(roomID, req)
	}
	if err != nil {
		return cliFail("%v", err)
	}
	if *asJSON {
		return cliPrintJSON(room)
	}
	if propose {
		fmt.Printf(
			"brief proposed: room=%s version=%d status=%s title=%q\n",
			room.ID,
			room.CurrentBriefVersion,
			room.CurrentBrief.Status,
			room.CurrentBrief.Content.Title,
		)
	} else {
		fmt.Printf(
			"brief set by compatibility/admin path: room=%s version=%d state=%s title=%q\n",
			room.ID,
			room.ConfirmedBriefVersion,
			room.State,
			room.Brief.Title,
		)
	}
	return 0
}

// ResolveRoomID picks the room id using the priority documented in roundtableUsage.
// cwdHint is typically "." (process cwd); tests may pass an absolute path.
func ResolveRoomID(store *Store, explicitRoom, cwdHint string) (string, error) {
	if id := strings.TrimSpace(explicitRoom); id != "" {
		return id, nil
	}
	if id := strings.TrimSpace(os.Getenv(EnvRoundtableRoomID)); id != "" {
		return id, nil
	}
	cwd := strings.TrimSpace(cwdHint)
	if cwd == "" {
		cwd = "."
	}
	abs, err := filepath.Abs(cwd)
	if err != nil {
		return "", fmt.Errorf("resolve cwd: %w", err)
	}
	if id, ok := readSidecarRoomID(abs); ok {
		return id, nil
	}
	if store == nil {
		return "", fmt.Errorf("no room id: pass --room, set %s, or write %s in seat cwd", EnvRoundtableRoomID, SeatSidecarFile)
	}
	if id, err := store.FindRoomIDByWorkspacePath(abs); err == nil && id != "" {
		return id, nil
	}
	return "", fmt.Errorf("no room id: pass --room, set %s, or write %s in seat cwd (cwd=%s)", EnvRoundtableRoomID, SeatSidecarFile, abs)
}

// readSidecarRoomID walks cwd and parents looking for SeatSidecarFile.
func readSidecarRoomID(start string) (string, bool) {
	dir := start
	for i := 0; i < 8; i++ {
		path := filepath.Join(dir, SeatSidecarFile)
		b, err := os.ReadFile(path)
		if err == nil {
			var sc SeatSidecar
			if json.Unmarshal(b, &sc) == nil {
				if id := strings.TrimSpace(sc.RoomID); id != "" {
					return id, true
				}
			}
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return "", false
}

// WriteSeatSidecar writes room/role context into a seat cwd for CLI resolution.
func WriteSeatSidecar(cwd, roomID, seatID string, role Role) error {
	if strings.TrimSpace(cwd) == "" {
		return fmt.Errorf("roundtable: empty cwd for seat sidecar")
	}
	if strings.TrimSpace(roomID) == "" {
		return fmt.Errorf("roundtable: empty room_id for seat sidecar")
	}
	sc := SeatSidecar{
		RoomID: roomID,
		Role:   string(role),
		SeatID: seatID,
		CLIBin: ResolveCLIBinary(),
	}
	b, err := json.MarshalIndent(sc, "", "  ")
	if err != nil {
		return err
	}
	path := filepath.Join(cwd, SeatSidecarFile)
	if err := os.WriteFile(path, append(b, '\n'), 0o644); err != nil {
		return fmt.Errorf("write %s: %w", SeatSidecarFile, err)
	}
	return nil
}

// rewriteRoundtableCLIInSeed replaces bare `1agents roundtable` with the
// resolved absolute binary (same pattern as workspace.rewriteCLIBinaryPath for
// project-items). No-op when already rewritten or resolution falls back to "1agents".
func rewriteRoundtableCLIInSeed(content string) string {
	rt := RoundtableCLI()
	if rt == "1agents roundtable" {
		return content
	}
	return strings.ReplaceAll(content, "1agents roundtable", rt)
}

func shellQuote(s string) string {
	if s == "" {
		return "''"
	}
	safe := true
	for _, r := range s {
		if !(r == '/' || r == '.' || r == '-' || r == '_' ||
			(r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r > 127) {
			safe = false
			break
		}
	}
	if safe {
		return s
	}
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

func cliFail(format string, a ...any) int {
	fmt.Fprintf(os.Stderr, "error: "+format+"\n", a...)
	return 1
}

func cliPrintJSON(v any) int {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	if err := enc.Encode(v); err != nil {
		return cliFail("encode json: %v", err)
	}
	return 0
}
