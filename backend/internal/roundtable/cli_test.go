package roundtable_test

import (
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/scottzx/1Agents/backend/internal/meta"
	"github.com/scottzx/1Agents/backend/internal/roundtable"
)

func TestFormatUsage_UsesResolvedBinary(t *testing.T) {
	// Simulate dev: ONEAGENTS_CLI points at a concrete path (no bare 1agents on PATH).
	fake := "/tmp/dev-build/1agents"
	t.Setenv(roundtable.EnvCLI, fake)
	out := captureRoundtableHelp(t)
	if !strings.Contains(out, fake+" roundtable help") {
		t.Fatalf("help should use resolved binary, got:\n%s", out)
	}
	if !strings.Contains(out, "ONEAGENTS_CLI") {
		t.Fatalf("help should mention ONEAGENTS_CLI tip:\n%s", out)
	}
	if strings.Contains(out, "\n  1agents roundtable help\n") {
		t.Fatalf("help should not lead with bare 1agents when ONEAGENTS_CLI is set:\n%s", out)
	}
}

func captureRoundtableHelp(t *testing.T) string {
	t.Helper()
	oldOut := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w
	code := roundtable.RunCLI([]string{"help"})
	_ = w.Close()
	os.Stdout = oldOut
	b, _ := io.ReadAll(r)
	_ = r.Close()
	if code != 0 {
		t.Fatalf("help exit=%d", code)
	}
	return string(b)
}

func TestResolveRoomID_Priority(t *testing.T) {
	home := t.TempDir()
	t.Setenv("ONEAGENTS_HOME", home)
	t.Setenv(roundtable.EnvRoundtableRoomID, "")
	store, err := roundtable.NewStore()
	if err != nil {
		t.Fatal(err)
	}

	// Explicit wins over everything.
	id, err := roundtable.ResolveRoomID(store, "explicit-room", home)
	if err != nil || id != "explicit-room" {
		t.Fatalf("explicit: id=%q err=%v", id, err)
	}

	// Env next.
	t.Setenv(roundtable.EnvRoundtableRoomID, "env-room")
	id, err = roundtable.ResolveRoomID(store, "", home)
	if err != nil || id != "env-room" {
		t.Fatalf("env: id=%q err=%v", id, err)
	}
	t.Setenv(roundtable.EnvRoundtableRoomID, "")

	// Sidecar in cwd.
	seatCwd := filepath.Join(home, "seat-cwd")
	if err := os.MkdirAll(seatCwd, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := roundtable.WriteSeatSidecar(seatCwd, "sidecar-room", "seat-1", roundtable.RoleReferee); err != nil {
		t.Fatal(err)
	}
	id, err = roundtable.ResolveRoomID(store, "", seatCwd)
	if err != nil || id != "sidecar-room" {
		t.Fatalf("sidecar: id=%q err=%v", id, err)
	}

	// Missing → clear error.
	empty := t.TempDir()
	_, err = roundtable.ResolveRoomID(store, "", empty)
	if err == nil {
		t.Fatal("expected error when no room id source")
	}
	if !strings.Contains(err.Error(), "no room id") {
		t.Fatalf("error message: %v", err)
	}
}

func TestValidateBrief_RejectsPlaceholder(t *testing.T) {
	ok := &roundtable.Brief{
		Title: "T", Question: "Q", Constraints: "C", SuccessCriteria: "S",
	}
	if err := roundtable.ValidateBrief(ok); err != nil {
		t.Fatalf("ok brief: %v", err)
	}
	bad := &roundtable.Brief{
		Title: "T", Question: "Q", Constraints: "—", SuccessCriteria: "S",
	}
	if err := roundtable.ValidateBrief(bad); err == nil {
		t.Fatal("expected reject placeholder constraints")
	}
	empty := &roundtable.Brief{Title: "T", Question: "Q", Constraints: "C", SuccessCriteria: ""}
	if err := roundtable.ValidateBrief(empty); err == nil {
		t.Fatal("expected reject empty success_criteria")
	}
}

func TestCreateRoom_WritesSidecarAndRefereeCLISeed(t *testing.T) {
	svc, _ := testRig(t)
	room, err := svc.CreateRoom(roundtable.CreateRoomRequest{Title: "CLI种子"})
	if err != nil {
		t.Fatalf("CreateRoom: %v", err)
	}
	var ref *roundtable.Seat
	for i := range room.Seats {
		if room.Seats[i].Role == roundtable.RoleReferee {
			ref = &room.Seats[i]
			break
		}
	}
	if ref == nil {
		t.Fatal("no referee seat")
	}
	db, err := meta.OpenDefault()
	if err != nil {
		t.Fatal(err)
	}
	proj, ok, err := db.GetProject(ref.WorkspaceID)
	if err != nil || !ok {
		t.Fatalf("GetProject: ok=%v err=%v", ok, err)
	}
	scPath := filepath.Join(proj.WorkspacePath, roundtable.SeatSidecarFile)
	b, err := os.ReadFile(scPath)
	if err != nil {
		t.Fatalf("sidecar missing: %v", err)
	}
	var sc roundtable.SeatSidecar
	if err := json.Unmarshal(b, &sc); err != nil {
		t.Fatal(err)
	}
	if sc.RoomID != room.ID || sc.Role != string(roundtable.RoleReferee) {
		t.Fatalf("sidecar=%+v room=%s", sc, room.ID)
	}
	agents, err := os.ReadFile(filepath.Join(proj.WorkspacePath, "AGENTS.md"))
	if err != nil {
		t.Fatal(err)
	}
	body := string(agents)
	if !strings.Contains(body, "roundtable set-brief") {
		t.Fatal("referee AGENTS.md missing set-brief usage")
	}
	if !strings.Contains(body, ".1agents-roundtable.json") {
		t.Fatal("referee AGENTS.md missing sidecar mention")
	}
	// Seed should rewrite bare 1agents → absolute path when executable resolves
	// (go test binary path is fine; just ensure rewrite ran when not falling back).
	cliBin := roundtable.ResolveCLIBinary()
	if cliBin != "1agents" && !strings.Contains(body, cliBin) {
		t.Fatalf("referee AGENTS.md should embed absolute CLI path %q", cliBin)
	}
	if sc.CLIBin == "" {
		t.Fatal("sidecar should store cli_bin")
	}
}

func TestCLI_GetAndSetBrief_FromSeatCwd(t *testing.T) {
	svc, _ := testRig(t)
	room, err := svc.CreateRoom(roundtable.CreateRoomRequest{Title: "跨cwd"})
	if err != nil {
		t.Fatalf("CreateRoom: %v", err)
	}
	var ref *roundtable.Seat
	for i := range room.Seats {
		if room.Seats[i].Role == roundtable.RoleReferee {
			ref = &room.Seats[i]
			break
		}
	}
	db, err := meta.OpenDefault()
	if err != nil {
		t.Fatal(err)
	}
	proj, ok, err := db.GetProject(ref.WorkspaceID)
	if err != nil || !ok {
		t.Fatal(err)
	}
	seatCwd := proj.WorkspacePath

	// get without --room from seat cwd (sidecar)
	code, stdout, stderr := runRoundtableCLI(t, seatCwd, nil, "get", "--json")
	if code != 0 {
		t.Fatalf("get exit=%d stderr=%s", code, stderr)
	}
	var got map[string]any
	if err := json.Unmarshal([]byte(stdout), &got); err != nil {
		t.Fatalf("parse get json: %v\n%s", err, stdout)
	}
	if got["id"] != room.ID {
		t.Fatalf("get id=%v want %s", got["id"], room.ID)
	}
	if got["state"] != string(roundtable.StateDraftingBrief) {
		t.Fatalf("state=%v", got["state"])
	}

	// set-brief without --room
	code, stdout, stderr = runRoundtableCLI(t, seatCwd, nil,
		"set-brief",
		"--title", "真实标题",
		"--question", "核心问题是什么？",
		"--constraints", "两周三个人",
		"--success-criteria", "5 个种子用户",
		"--product-kind", "software",
		"--json",
	)
	if code != 0 {
		t.Fatalf("set-brief exit=%d stderr=%s stdout=%s", code, stderr, stdout)
	}
	var after roundtable.Room
	if err := json.Unmarshal([]byte(stdout), &after); err != nil {
		t.Fatalf("parse set-brief json: %v\n%s", err, stdout)
	}
	if after.State != roundtable.StateWaitingR2 {
		t.Fatalf("state %q want waiting_r2", after.State)
	}
	if after.Brief == nil || after.Brief.Title != "真实标题" || after.Brief.Constraints == "—" {
		t.Fatalf("brief=%+v", after.Brief)
	}

	// Persisted via service GetRoom
	reloaded, err := svc.GetRoom(room.ID)
	if err != nil {
		t.Fatal(err)
	}
	if reloaded.State != roundtable.StateWaitingR2 || reloaded.Brief == nil || reloaded.Brief.Question != "核心问题是什么？" {
		t.Fatalf("persisted: state=%s brief=%+v", reloaded.State, reloaded.Brief)
	}

	// Reject placeholder
	code, _, stderr = runRoundtableCLI(t, seatCwd, nil,
		"set-brief",
		"--title", "x",
		"--question", "y",
		"--constraints", "—",
		"--success-criteria", "z",
	)
	// room already waiting_r2 — wrong state OR placeholder; either is non-zero
	if code == 0 {
		t.Fatal("expected non-zero for invalid/wrong-state set-brief")
	}
	_ = stderr

	// Unknown room
	code, _, stderr = runRoundtableCLI(t, seatCwd, nil, "get", "--room", "does-not-exist-zzzz", "--json")
	if code == 0 {
		t.Fatal("expected fail for unknown room")
	}
	if !strings.Contains(stderr, "not found") && !strings.Contains(stderr, "error") {
		t.Fatalf("stderr should explain: %s", stderr)
	}
}

func TestCLI_SetBrief_RejectsPlaceholderOnDrafting(t *testing.T) {
	svc, _ := testRig(t)
	room, err := svc.CreateRoom(roundtable.CreateRoomRequest{Title: "占位拒"})
	if err != nil {
		t.Fatal(err)
	}
	code, _, stderr := runRoundtableCLI(t, ".", nil,
		"set-brief",
		"--room", room.ID,
		"--title", "T",
		"--question", "Q",
		"--constraints", "—",
		"--success-criteria", "S",
	)
	if code == 0 {
		t.Fatal("expected reject")
	}
	if !strings.Contains(stderr, "constraints") && !strings.Contains(stderr, "placeholder") && !strings.Contains(stderr, "required") {
		t.Fatalf("stderr=%s", stderr)
	}
}

// runRoundtableCLI invokes RunCLI with optional chdir and env overrides.
func runRoundtableCLI(t *testing.T, cwd string, env map[string]string, args ...string) (code int, stdout, stderr string) {
	t.Helper()
	oldWd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if cwd != "" && cwd != "." {
		if err := os.Chdir(cwd); err != nil {
			t.Fatal(err)
		}
		defer func() { _ = os.Chdir(oldWd) }()
	}
	for k, v := range env {
		t.Setenv(k, v)
	}
	oldOut, oldErr := os.Stdout, os.Stderr
	rOut, wOut, _ := os.Pipe()
	rErr, wErr, _ := os.Pipe()
	os.Stdout, os.Stderr = wOut, wErr
	code = roundtable.RunCLI(args)
	_ = wOut.Close()
	_ = wErr.Close()
	os.Stdout, os.Stderr = oldOut, oldErr
	outB, _ := io.ReadAll(rOut)
	errB, _ := io.ReadAll(rErr)
	_ = rOut.Close()
	_ = rErr.Close()
	return code, string(outB), string(errB)
}
