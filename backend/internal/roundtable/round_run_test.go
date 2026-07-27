package roundtable_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/scottzx/1Agents/backend/internal/roundtable"
)

func confirmedRunRoom(t *testing.T) (*roundtable.Service, *roundtable.StaticSeatPrompter, *roundtable.Room) {
	t.Helper()
	svc, prompter := testRig(t)
	prompter.Replies = nil
	prompter.FixedAcpID = "acp-round-run"
	prompter.ReplyFunc = func(req roundtable.SeatPromptRequest) (string, error) {
		if req.Role == roundtable.RoleReferee {
			return "Summary：五席已收敛。", nil
		}
		return fmt.Sprintf("%s 席观点", req.Role), nil
	}
	room, err := svc.CreateRoom(roundtable.CreateRoomRequest{Title: "RoundRun"})
	if err != nil {
		t.Fatal(err)
	}
	room, err = svc.ConfirmBrief(room.ID, roundtable.ConfirmBriefRequest{
		Title:           "可靠运行",
		Question:        "并发启动如何保持幂等？",
		Constraints:     "每席至多一次",
		SuccessCriteria: "可恢复事件完整",
	})
	if err != nil {
		t.Fatal(err)
	}
	return svc, prompter, room
}

func concurrentStart(
	t *testing.T,
	start func(string) (*roundtable.StartRoundResponse, error),
) [2]*roundtable.StartRoundResponse {
	t.Helper()
	var responses [2]*roundtable.StartRoundResponse
	var errs [2]error
	var wg sync.WaitGroup
	for i := range responses {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			responses[index], errs[index] = start(fmt.Sprintf("client-%d", index))
		}(i)
	}
	wg.Wait()
	for i, err := range errs {
		if err != nil {
			t.Fatalf("start[%d]: %v", i, err)
		}
	}
	if responses[0].RunID != responses[1].RunID {
		t.Fatalf("concurrent starts created different runs: %s != %s", responses[0].RunID, responses[1].RunID)
	}
	if responses[0].Reused == responses[1].Reused {
		t.Fatalf("want exactly one creator and one reuse: %+v %+v", responses[0], responses[1])
	}
	return responses
}

func waitTerminalRun(t *testing.T, svc *roundtable.Service, runID string) *roundtable.RoundRun {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	run, err := svc.WaitRoundRun(ctx, runID)
	if err != nil {
		t.Fatal(err)
	}
	return run
}

func assertOneCallPerRole(t *testing.T, calls []roundtable.SeatPromptRequest) {
	t.Helper()
	counts := map[roundtable.Role]int{}
	for _, call := range calls {
		counts[call.Role]++
	}
	for _, role := range roundtable.PanelistRoles {
		if counts[role] != 1 {
			t.Fatalf("role %s calls=%d, want 1; all=%v", role, counts[role], counts)
		}
	}
	if counts[roundtable.RoleReferee] != 1 {
		t.Fatalf("referee summary calls=%d, want 1; all=%v", counts[roundtable.RoleReferee], counts)
	}
}

func TestRoundRunConcurrentR2R3AtMostOnce(t *testing.T) {
	svc, prompter, room := confirmedRunRoom(t)

	gate := make(chan struct{})
	prompter.ReplyFunc = func(req roundtable.SeatPromptRequest) (string, error) {
		<-gate
		if req.Role == roundtable.RoleReferee {
			return "Summary₂：完成", nil
		}
		return "R2-" + string(req.Role), nil
	}
	r2 := concurrentStart(t, func(key string) (*roundtable.StartRoundResponse, error) {
		return svc.StartR2(room.ID, key)
	})

	deadline := time.Now().Add(3 * time.Second)
	for {
		current, err := svc.GetRoom(room.ID)
		if err != nil {
			t.Fatal(err)
		}
		if current.ActiveRun != nil &&
			current.ActiveRun.ID == r2[0].RunID &&
			len(current.Progress.ActiveRoles) > 0 {
			if current.Phase != "r2" || current.NextAction != "wait" {
				t.Fatalf("running projection: phase=%s next=%s", current.Phase, current.NextAction)
			}
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("room never exposed active role: %+v", current)
		}
		time.Sleep(5 * time.Millisecond)
	}
	close(gate)
	r2Run := waitTerminalRun(t, svc, r2[0].RunID)
	if r2Run.Status != roundtable.RunCompleted {
		t.Fatalf("r2 status=%s error=%s", r2Run.Status, r2Run.Error)
	}
	assertOneCallPerRole(t, prompter.Calls)

	prompter.Calls = nil
	prompter.ReplyFunc = func(req roundtable.SeatPromptRequest) (string, error) {
		if req.Role == roundtable.RoleReferee {
			return "Summary₃：完成", nil
		}
		return "R3-" + string(req.Role), nil
	}
	r3 := concurrentStart(t, func(key string) (*roundtable.StartRoundResponse, error) {
		return svc.StartR3(room.ID, key)
	})
	r3Run := waitTerminalRun(t, svc, r3[0].RunID)
	if r3Run.Status != roundtable.RunCompleted {
		t.Fatalf("r3 status=%s error=%s", r3Run.Status, r3Run.Error)
	}
	assertOneCallPerRole(t, prompter.Calls)

	finalRoom, err := svc.GetRoom(room.ID)
	if err != nil {
		t.Fatal(err)
	}
	if finalRoom.Phase != "done" ||
		finalRoom.PhaseStatus != string(roundtable.RunCompleted) ||
		finalRoom.NextAction != "none" ||
		finalRoom.Progress.Completed != 5 ||
		len(finalRoom.Progress.ActiveRoles) != 0 ||
		len(finalRoom.Progress.FailedRoles) != 0 {
		t.Fatalf("final room projection: %+v", finalRoom)
	}
}

func TestRoundRunHTTP202AndLegacyWaitCompatibility(t *testing.T) {
	svc, _, room := confirmedRunRoom(t)
	handler := roundtable.NewHandler(svc)

	startRecorder := httptest.NewRecorder()
	startRequest := httptest.NewRequest(
		http.MethodPost,
		"/api/roundtable/rooms/"+room.ID+"/r2",
		nil,
	)
	startRequest.Header.Set("Idempotency-Key", "browser-window-a")
	handler.HandleRoomsItem(startRecorder, startRequest)
	if startRecorder.Code != http.StatusAccepted {
		t.Fatalf("start status=%d body=%s", startRecorder.Code, startRecorder.Body.String())
	}
	var start roundtable.StartRoundResponse
	if err := json.NewDecoder(startRecorder.Body).Decode(&start); err != nil {
		t.Fatal(err)
	}
	if start.RunID == "" || start.Run == nil || start.Run.IdempotencyKey != "browser-window-a" {
		t.Fatalf("start response: %+v", start)
	}

	legacyRecorder := httptest.NewRecorder()
	legacyRequest := httptest.NewRequest(
		http.MethodPost,
		"/api/roundtable/rooms/"+room.ID+"/r2?wait=1",
		nil,
	)
	handler.HandleRoomsItem(legacyRecorder, legacyRequest)
	if legacyRecorder.Code != http.StatusOK {
		t.Fatalf("legacy status=%d body=%s", legacyRecorder.Code, legacyRecorder.Body.String())
	}
	var legacy roundtable.RunR2Response
	if err := json.NewDecoder(legacyRecorder.Body).Decode(&legacy); err != nil {
		t.Fatal(err)
	}
	if legacy.Room == nil || legacy.Room.State != roundtable.StateWaitingR3 ||
		len(legacy.SpeechTurns) != 5 || legacy.SummaryTurn.Kind != roundtable.TurnKindSummary {
		t.Fatalf("legacy response: %+v", legacy)
	}
}

func TestRoundRunEventsResumeAfterSequence(t *testing.T) {
	svc, _, room := confirmedRunRoom(t)
	start, err := svc.StartR2(room.ID, "events-r2")
	if err != nil {
		t.Fatal(err)
	}
	if run := waitTerminalRun(t, svc, start.RunID); run.Status != roundtable.RunCompleted {
		t.Fatalf("run status=%s error=%s", run.Status, run.Error)
	}

	all, err := svc.ListRoundEvents(room.ID, 0, 200)
	if err != nil {
		t.Fatal(err)
	}
	if len(all) < 14 {
		t.Fatalf("events=%d, want run + 5 seat pairs + summary + terminal", len(all))
	}
	kinds := map[string]bool{}
	for i, event := range all {
		kinds[event.Kind] = true
		if i > 0 && event.Seq <= all[i-1].Seq {
			t.Fatalf("event sequence not increasing: %d then %d", all[i-1].Seq, event.Seq)
		}
	}
	for _, kind := range []string{"run", "seat", "summary"} {
		if !kinds[kind] {
			t.Fatalf("missing %s event: %+v", kind, all)
		}
	}

	cursor := all[len(all)/2-1].Seq
	resumed, err := svc.ListRoundEvents(room.ID, cursor, 200)
	if err != nil {
		t.Fatal(err)
	}
	want := all[len(all)/2:]
	if len(resumed) != len(want) {
		t.Fatalf("resumed=%d want=%d cursor=%d", len(resumed), len(want), cursor)
	}
	for i := range resumed {
		if resumed[i].Seq != want[i].Seq || resumed[i].Seq <= cursor {
			t.Fatalf("resumed[%d]=%+v want=%+v", i, resumed[i], want[i])
		}
	}

	handler := roundtable.NewHandler(svc)
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(
		http.MethodGet,
		fmt.Sprintf("/api/roundtable/rooms/%s/events", room.ID),
		nil,
	)
	request.Header.Set("Last-Event-ID", fmt.Sprint(cursor))
	handler.HandleRoomsItem(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("events status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	var page struct {
		Events  []roundtable.RoundEvent `json:"events"`
		LastSeq int64                   `json:"last_seq"`
	}
	if err := json.NewDecoder(recorder.Body).Decode(&page); err != nil {
		t.Fatal(err)
	}
	if len(page.Events) != len(want) || page.LastSeq != all[len(all)-1].Seq {
		t.Fatalf("event page: %+v", page)
	}
}

func TestRoundRunPartialFailureProjectsFailedRoles(t *testing.T) {
	svc, prompter, room := confirmedRunRoom(t)
	prompter.FailRoles = map[roundtable.Role]error{
		roundtable.RoleOps: fmt.Errorf("ops unavailable"),
	}
	start, err := svc.StartR2(room.ID, "partial-r2")
	if err != nil {
		t.Fatal(err)
	}
	run := waitTerminalRun(t, svc, start.RunID)
	if run.Status != roundtable.RunPartialFailed {
		t.Fatalf("status=%s error=%s", run.Status, run.Error)
	}
	current, err := svc.GetRoom(room.ID)
	if err != nil {
		t.Fatal(err)
	}
	if current.Progress.Completed != 4 ||
		len(current.Progress.FailedRoles) != 1 ||
		current.Progress.FailedRoles[0] != string(roundtable.RoleOps) {
		t.Fatalf("progress: %+v", current.Progress)
	}
	if current.ActiveRun == nil ||
		current.ActiveRun.ErrorScope != roundtable.RunErrorSeat ||
		current.NextAction != "retry_failed_seats" {
		t.Fatalf("seat recovery projection: %+v", current)
	}
}

func TestRoundRunRetriesOnlyFailedSeatAndPreservesSuccessfulTurns(t *testing.T) {
	svc, prompter, room := confirmedRunRoom(t)
	prompter.FailRoles = map[roundtable.Role]error{
		roundtable.RoleOps: fmt.Errorf("ops unavailable"),
	}
	start, err := svc.StartR2(room.ID, "retry-one-seat")
	if err != nil {
		t.Fatal(err)
	}
	if run := waitTerminalRun(t, svc, start.RunID); run.Status != roundtable.RunPartialFailed {
		t.Fatalf("initial status=%s error=%s", run.Status, run.Error)
	}
	before, err := svc.ListTurns(room.ID)
	if err != nil {
		t.Fatal(err)
	}
	successIDs := map[string]string{}
	for _, turn := range before {
		if turn.Round == 2 && turn.Kind == roundtable.TurnKindSpeech &&
			!strings.HasPrefix(turn.ContentText, "[failed]") {
			successIDs[turn.SeatID] = turn.ID
		}
	}
	if len(successIDs) != 4 {
		t.Fatalf("successful turns before retry=%v", successIDs)
	}

	prompter.FailRoles = nil
	recovery, err := svc.RetryRoundSeat(room.ID, start.RunID, roundtable.RoleOps)
	if err != nil {
		t.Fatal(err)
	}
	if recovery.RunID != start.RunID {
		t.Fatalf("retry changed run id: %s != %s", recovery.RunID, start.RunID)
	}
	if run := waitTerminalRun(t, svc, start.RunID); run.Status != roundtable.RunCompleted {
		t.Fatalf("retry status=%s error=%s", run.Status, run.Error)
	}

	counts := map[roundtable.Role]int{}
	for _, call := range prompter.SnapshotCalls() {
		counts[call.Role]++
	}
	for _, role := range roundtable.PanelistRoles {
		want := 1
		if role == roundtable.RoleOps {
			want = 2
		}
		if counts[role] != want {
			t.Fatalf("role %s calls=%d want=%d; all=%v", role, counts[role], want, counts)
		}
	}
	if counts[roundtable.RoleReferee] != 1 {
		t.Fatalf("summary calls=%d want=1", counts[roundtable.RoleReferee])
	}
	after, err := svc.ListTurns(room.ID)
	if err != nil {
		t.Fatal(err)
	}
	for seatID, turnID := range successIDs {
		found := false
		for _, turn := range after {
			if turn.ID == turnID && turn.SeatID == seatID {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("successful turn %s for seat %s was not preserved", turnID, seatID)
		}
	}

	duplicate, err := svc.RetryRoundSeat(room.ID, start.RunID, roundtable.RoleOps)
	if err != nil {
		t.Fatal(err)
	}
	if !duplicate.Reused {
		t.Fatalf("duplicate retry should be idempotently reused: %+v", duplicate)
	}
	if len(prompter.SnapshotCalls()) != 7 {
		t.Fatalf("duplicate retry prompted again: calls=%d", len(prompter.SnapshotCalls()))
	}
}

func TestRoundRunSkipContinuesSummaryAndRecordsMissingRole(t *testing.T) {
	svc, prompter, room := confirmedRunRoom(t)
	prompter.FailRoles = map[roundtable.Role]error{
		roundtable.RoleOps: fmt.Errorf("ops unavailable"),
	}
	start, err := svc.StartR2(room.ID, "skip-seat")
	if err != nil {
		t.Fatal(err)
	}
	if run := waitTerminalRun(t, svc, start.RunID); run.Status != roundtable.RunPartialFailed {
		t.Fatalf("initial status=%s", run.Status)
	}
	recovery, err := svc.SkipFailedSeatsAndSummarize(room.ID, start.RunID)
	if err != nil {
		t.Fatal(err)
	}
	if recovery.RunID != start.RunID {
		t.Fatalf("skip changed run id: %s != %s", recovery.RunID, start.RunID)
	}
	run := waitTerminalRun(t, svc, start.RunID)
	if run.Status != roundtable.RunPartialFailed || run.ErrorScope != roundtable.RunErrorNone {
		t.Fatalf("skip terminal run=%+v", run)
	}
	current, err := svc.GetRoom(room.ID)
	if err != nil {
		t.Fatal(err)
	}
	if current.State != roundtable.StateWaitingR3 ||
		len(current.Progress.SkippedRoles) != 1 ||
		current.Progress.SkippedRoles[0] != string(roundtable.RoleOps) ||
		!strings.Contains(current.SummaryR2, "缺席角色（系统记录）") ||
		!strings.Contains(current.SummaryR2, "运营") {
		t.Fatalf("skip result did not persist absence: %+v", current)
	}
	counts := map[roundtable.Role]int{}
	for _, call := range prompter.SnapshotCalls() {
		counts[call.Role]++
	}
	for _, role := range roundtable.PanelistRoles {
		if counts[role] != 1 {
			t.Fatalf("skip reran panelist %s: counts=%v", role, counts)
		}
	}
	if counts[roundtable.RoleReferee] != 1 {
		t.Fatalf("summary calls=%d want=1", counts[roundtable.RoleReferee])
	}
}

func TestRoundRunSummaryRetryDoesNotRerunPanelists(t *testing.T) {
	svc, prompter, room := confirmedRunRoom(t)
	refereeCalls := 0
	prompter.ReplyFunc = func(req roundtable.SeatPromptRequest) (string, error) {
		if req.Role == roundtable.RoleReferee {
			refereeCalls++
			if refereeCalls == 1 {
				return "", fmt.Errorf("summary model unavailable")
			}
			return "Summary₂：重试成功", nil
		}
		return fmt.Sprintf("%s 席观点", req.Role), nil
	}
	start, err := svc.StartR2(room.ID, "summary-retry")
	if err != nil {
		t.Fatal(err)
	}
	failed := waitTerminalRun(t, svc, start.RunID)
	if failed.Status != roundtable.RunFailed || failed.ErrorScope != roundtable.RunErrorSummary {
		t.Fatalf("summary failure not classified: %+v", failed)
	}
	failedRoom, err := svc.GetRoom(room.ID)
	if err != nil {
		t.Fatal(err)
	}
	if failedRoom.NextAction != "retry_summary" {
		t.Fatalf("summary next action=%s", failedRoom.NextAction)
	}

	recovery, err := svc.RetryRoundSummary(room.ID, start.RunID)
	if err != nil {
		t.Fatal(err)
	}
	if recovery.RunID != start.RunID {
		t.Fatalf("summary retry changed run id")
	}
	if run := waitTerminalRun(t, svc, start.RunID); run.Status != roundtable.RunCompleted {
		t.Fatalf("summary retry status=%s error=%s", run.Status, run.Error)
	}
	counts := map[roundtable.Role]int{}
	for _, call := range prompter.SnapshotCalls() {
		counts[call.Role]++
	}
	for _, role := range roundtable.PanelistRoles {
		if counts[role] != 1 {
			t.Fatalf("summary retry reran %s: counts=%v", role, counts)
		}
	}
	if counts[roundtable.RoleReferee] != 2 {
		t.Fatalf("summary calls=%d want=2", counts[roundtable.RoleReferee])
	}
}

func TestRoundRunR3RetriesOnlyFailedSeatOnSameRun(t *testing.T) {
	svc, prompter, room := confirmedRunRoom(t)
	financeR3Calls := 0
	prompter.ReplyFunc = func(req roundtable.SeatPromptRequest) (string, error) {
		if req.Role == roundtable.RoleFinance && strings.Contains(req.Text, "当前阶段：R3") {
			financeR3Calls++
			if financeR3Calls == 1 {
				return "", fmt.Errorf("finance unavailable")
			}
		}
		if req.Role == roundtable.RoleReferee {
			return "Summary：完成", nil
		}
		return fmt.Sprintf("%s 席观点", req.Role), nil
	}
	r2Start, err := svc.StartR2(room.ID, "r3-retry-r2")
	if err != nil {
		t.Fatal(err)
	}
	if run := waitTerminalRun(t, svc, r2Start.RunID); run.Status != roundtable.RunCompleted {
		t.Fatalf("r2 status=%s error=%s", run.Status, run.Error)
	}
	r3Start, err := svc.StartR3(room.ID, "r3-retry")
	if err != nil {
		t.Fatal(err)
	}
	if run := waitTerminalRun(t, svc, r3Start.RunID); run.Status != roundtable.RunPartialFailed {
		t.Fatalf("initial r3 status=%s error=%s", run.Status, run.Error)
	}
	recovery, err := svc.RetryRoundSeat(room.ID, r3Start.RunID, roundtable.RoleFinance)
	if err != nil {
		t.Fatal(err)
	}
	if recovery.RunID != r3Start.RunID {
		t.Fatalf("r3 retry changed run id")
	}
	if run := waitTerminalRun(t, svc, r3Start.RunID); run.Status != roundtable.RunCompleted {
		t.Fatalf("r3 retry status=%s error=%s", run.Status, run.Error)
	}

	counts := map[roundtable.Role]int{}
	for _, call := range prompter.SnapshotCalls() {
		if strings.Contains(call.Text, "当前阶段：R3") {
			counts[call.Role]++
		}
	}
	for _, role := range roundtable.PanelistRoles {
		want := 1
		if role == roundtable.RoleFinance {
			want = 2
		}
		if counts[role] != want {
			t.Fatalf("r3 role %s calls=%d want=%d; all=%v", role, counts[role], want, counts)
		}
	}
	if counts[roundtable.RoleReferee] != 1 {
		t.Fatalf("r3 summary calls=%d want=1", counts[roundtable.RoleReferee])
	}
	current, err := svc.GetRoom(room.ID)
	if err != nil {
		t.Fatal(err)
	}
	if current.State != roundtable.StateDone || current.ActiveRun == nil ||
		current.ActiveRun.ID != r3Start.RunID {
		t.Fatalf("r3 recovery room=%+v", current)
	}
}

func TestRoundRunSeatRetryHTTPKeepsRunContext(t *testing.T) {
	svc, prompter, room := confirmedRunRoom(t)
	opsCalls := 0
	prompter.ReplyFunc = func(req roundtable.SeatPromptRequest) (string, error) {
		if req.Role == roundtable.RoleOps {
			opsCalls++
			if opsCalls == 1 {
				return "", fmt.Errorf("ops unavailable")
			}
		}
		if req.Role == roundtable.RoleReferee {
			return "Summary₂：完成", nil
		}
		return fmt.Sprintf("%s 席观点", req.Role), nil
	}
	start, err := svc.StartR2(room.ID, "http-seat-retry")
	if err != nil {
		t.Fatal(err)
	}
	if run := waitTerminalRun(t, svc, start.RunID); run.Status != roundtable.RunPartialFailed {
		t.Fatalf("initial status=%s", run.Status)
	}

	handler := roundtable.NewHandler(svc)
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(
		http.MethodPost,
		fmt.Sprintf(
			"/api/roundtable/rooms/%s/runs/%s/seats/%s/retry",
			room.ID,
			start.RunID,
			roundtable.RoleOps,
		),
		nil,
	)
	handler.HandleRoomsItem(recorder, request)
	if recorder.Code != http.StatusAccepted {
		t.Fatalf("retry status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	var response roundtable.RecoverRoundResponse
	if err := json.NewDecoder(recorder.Body).Decode(&response); err != nil {
		t.Fatal(err)
	}
	if response.RunID != start.RunID || response.Room == nil || response.Room.ID != room.ID {
		t.Fatalf("recovery response changed context: %+v", response)
	}
	if run := waitTerminalRun(t, svc, start.RunID); run.Status != roundtable.RunCompleted {
		t.Fatalf("retry status=%s error=%s", run.Status, run.Error)
	}
}
