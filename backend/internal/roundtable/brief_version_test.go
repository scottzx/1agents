package roundtable_test

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/scottzx/1Agents/backend/internal/meta"
	"github.com/scottzx/1Agents/backend/internal/roundtable"
)

func validBrief(title string) roundtable.ConfirmBriefRequest {
	return roundtable.ConfirmBriefRequest{
		Title:           title,
		Question:        "应该解决什么核心问题？",
		Constraints:     "两周、三人团队",
		SuccessCriteria: "五名用户完成核心路径",
		ProductKind:     roundtable.ProductSoftware,
	}
}

func TestBriefVersionProposalConfirmAndStaleWrite(t *testing.T) {
	svc, _ := testRig(t)
	room, err := svc.CreateRoom(roundtable.CreateRoomRequest{Title: "版本契约"})
	if err != nil {
		t.Fatal(err)
	}

	proposed, err := svc.ProposeBrief(room.ID, roundtable.ProposeBriefRequest{
		ConfirmBriefRequest: validBrief("裁判提案 v1"),
		ExpectedVersion:     0,
		SourceTurnID:        "turn-referee-1",
	})
	if err != nil {
		t.Fatalf("ProposeBrief: %v", err)
	}
	if proposed.State != roundtable.StateDraftingBrief ||
		proposed.CurrentBriefVersion != 1 ||
		proposed.ConfirmedBriefVersion != 0 ||
		proposed.CurrentBrief == nil ||
		proposed.CurrentBrief.Status != roundtable.BriefStatusProposed ||
		proposed.CurrentBrief.ProposedBy != roundtable.BriefProposerReferee ||
		proposed.CurrentBrief.SourceTurnID != "turn-referee-1" {
		t.Fatalf("proposal room=%+v current=%+v", proposed, proposed.CurrentBrief)
	}

	_, err = svc.SaveBriefDraft(room.ID, roundtable.SaveBriefDraftRequest{
		ConfirmBriefRequest: validBrief("stale overwrite"),
		ExpectedVersion:     0,
	})
	if !errors.Is(err, roundtable.ErrBriefVersionConflict) {
		t.Fatalf("stale draft err=%v, want ErrBriefVersionConflict", err)
	}
	unchanged, err := svc.GetRoom(room.ID)
	if err != nil {
		t.Fatal(err)
	}
	if unchanged.CurrentBriefVersion != 1 || unchanged.CurrentBrief.Content.Title != "裁判提案 v1" {
		t.Fatalf("stale write overwrote current: %+v", unchanged.CurrentBrief)
	}

	confirmed, err := svc.ConfirmBriefVersion(room.ID, roundtable.ConfirmBriefVersionRequest{
		Version:         1,
		ExpectedVersion: 1,
	})
	if err != nil {
		t.Fatalf("ConfirmBriefVersion: %v", err)
	}
	if confirmed.State != roundtable.StateWaitingR2 ||
		confirmed.ConfirmedBriefVersion != 1 ||
		confirmed.ConfirmedBrief == nil ||
		confirmed.ConfirmedBrief.Status != roundtable.BriefStatusConfirmed ||
		confirmed.ConfirmedBrief.ConfirmedAt == nil {
		t.Fatalf("confirmed room=%+v confirmed=%+v", confirmed, confirmed.ConfirmedBrief)
	}

	draft, err := svc.SaveBriefDraft(room.ID, roundtable.SaveBriefDraftRequest{
		ConfirmBriefRequest: validBrief("用户草案 v2"),
		ExpectedVersion:     1,
	})
	if err != nil {
		t.Fatalf("SaveBriefDraft: %v", err)
	}
	if draft.State != roundtable.StateDraftingBrief ||
		draft.CurrentBriefVersion != 2 ||
		draft.ConfirmedBriefVersion != 1 ||
		draft.CurrentBrief.Status != roundtable.BriefStatusDraft ||
		draft.CurrentBrief.ProposedBy != roundtable.BriefProposerUser {
		t.Fatalf("draft room=%+v current=%+v", draft, draft.CurrentBrief)
	}
	reconfirmed, err := svc.ConfirmBriefVersion(room.ID, roundtable.ConfirmBriefVersionRequest{
		Version:         2,
		ExpectedVersion: 2,
	})
	if err != nil {
		t.Fatalf("confirm v2: %v", err)
	}
	store, err := roundtable.NewStore()
	if err != nil {
		t.Fatal(err)
	}
	versions, err := store.ListBriefVersions(room.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(versions) != 2 ||
		versions[0].Status != roundtable.BriefStatusSuperseded ||
		versions[1].Status != roundtable.BriefStatusConfirmed ||
		reconfirmed.ConfirmedBriefVersion != 2 {
		t.Fatalf("version lifecycle=%+v room=%+v", versions, reconfirmed)
	}
}

func TestBriefVersionConcurrentProposalsUseAtomicCAS(t *testing.T) {
	svc, _ := testRig(t)
	room, err := svc.CreateRoom(roundtable.CreateRoomRequest{Title: "并发提案"})
	if err != nil {
		t.Fatal(err)
	}
	store, err := roundtable.NewStore()
	if err != nil {
		t.Fatal(err)
	}
	content := roundtable.Brief{
		Title:           "并发",
		Question:        "哪个写入成功？",
		Constraints:     "同一 expected version",
		SuccessCriteria: "只产生一个 v1",
	}
	start := make(chan struct{})
	errs := make(chan error, 2)
	var wg sync.WaitGroup
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			_, err := store.CreateBriefVersion(
				room.ID,
				0,
				roundtable.BriefStatusProposed,
				content,
				roundtable.BriefProposerReferee,
				"",
			)
			errs <- err
		}()
	}
	close(start)
	wg.Wait()
	close(errs)

	var succeeded, conflicted int
	for err := range errs {
		switch {
		case err == nil:
			succeeded++
		case errors.Is(err, roundtable.ErrBriefVersionConflict):
			conflicted++
		default:
			t.Fatalf("concurrent proposal returned non-conflict error: %v", err)
		}
	}
	if succeeded != 1 || conflicted != 1 {
		t.Fatalf("succeeded=%d conflicted=%d", succeeded, conflicted)
	}
	versions, err := store.ListBriefVersions(room.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(versions) != 1 || versions[0].Version != 1 {
		t.Fatalf("versions=%+v", versions)
	}
}

func TestBriefHTTPProposalCannotConfirmAndConflictIs409(t *testing.T) {
	svc, _ := testRig(t)
	room, err := svc.CreateRoom(roundtable.CreateRoomRequest{Title: "HTTP 版本"})
	if err != nil {
		t.Fatal(err)
	}
	h := roundtable.NewHandler(svc)
	body := `{
		"title":"Agent proposal",
		"question":"问题？",
		"constraints":"约束",
		"success_criteria":"标准",
		"expected_version":0,
		"status":"confirmed"
	}`
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(
		http.MethodPost,
		"/api/roundtable/rooms/"+room.ID+"/brief/propose",
		strings.NewReader(body),
	)
	h.HandleRoomsItem(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("propose status=%d body=%s", rr.Code, rr.Body.String())
	}
	var proposed roundtable.Room
	if err := json.NewDecoder(rr.Body).Decode(&proposed); err != nil {
		t.Fatal(err)
	}
	if proposed.State != roundtable.StateDraftingBrief ||
		proposed.ConfirmedBriefVersion != 0 ||
		proposed.CurrentBrief == nil ||
		proposed.CurrentBrief.Status != roundtable.BriefStatusProposed {
		t.Fatalf("agent proposal confirmed directly: %+v", proposed)
	}

	rr = httptest.NewRecorder()
	req = httptest.NewRequest(
		http.MethodPost,
		"/api/roundtable/rooms/"+room.ID+"/brief/propose",
		strings.NewReader(strings.Replace(body, `"Agent proposal"`, `"stale proposal"`, 1)),
	)
	h.HandleRoomsItem(rr, req)
	if rr.Code != http.StatusConflict {
		t.Fatalf("stale status=%d want 409 body=%s", rr.Code, rr.Body.String())
	}
	var conflict struct {
		Code           string `json:"code"`
		CurrentVersion int    `json:"current_version"`
	}
	if err := json.NewDecoder(rr.Body).Decode(&conflict); err != nil {
		t.Fatal(err)
	}
	if conflict.Code != "brief_version_conflict" || conflict.CurrentVersion != 1 {
		t.Fatalf("conflict payload=%+v", conflict)
	}
}

func TestR2PinsConfirmedBriefSnapshotAcrossLaterVersions(t *testing.T) {
	svc, prompter := testRig(t)
	room, err := svc.CreateRoom(roundtable.CreateRoomRequest{Title: "R2 快照"})
	if err != nil {
		t.Fatal(err)
	}
	confirmed, err := svc.ConfirmBrief(room.ID, validBrief("R2 使用的 v1"))
	if err != nil {
		t.Fatal(err)
	}
	if confirmed.ConfirmedBriefVersion != 1 {
		t.Fatalf("confirmed version=%d", confirmed.ConfirmedBriefVersion)
	}
	if _, err := svc.RunR2(room.ID); err != nil {
		t.Fatalf("RunR2: %v", err)
	}

	afterR2, err := svc.GetRoom(room.ID)
	if err != nil {
		t.Fatal(err)
	}
	if afterR2.R2BriefVersion != 1 ||
		afterR2.R2Brief == nil ||
		afterR2.R2Brief.Content.Title != "R2 使用的 v1" {
		t.Fatalf("r2 snapshot=%+v pointer=%d", afterR2.R2Brief, afterR2.R2BriefVersion)
	}
	for _, call := range prompter.SnapshotCalls() {
		if call.Role == roundtable.RoleReferee && !strings.Contains(call.Text, "【Brief】") {
			continue // R1 calls, if any
		}
		if strings.Contains(call.Text, "【Brief】") && !strings.Contains(call.Text, "R2 使用的 v1") {
			t.Fatalf("R2 prompt did not use v1 snapshot: %q", call.Text)
		}
	}

	reopened, err := svc.ProposeBrief(room.ID, roundtable.ProposeBriefRequest{
		ConfirmBriefRequest: validBrief("R2 后的新 v2"),
		ExpectedVersion:     1,
	})
	if err != nil {
		t.Fatalf("proposal after R2: %v", err)
	}
	if reopened.State != roundtable.StateDraftingBrief ||
		reopened.CurrentBriefVersion != 2 ||
		reopened.R2BriefVersion != 1 ||
		reopened.R2Brief.Content.Title != "R2 使用的 v1" {
		t.Fatalf("post-R2 re-discussion contract: %+v", reopened)
	}
}

func TestLegacyRoomBriefMigratesToConfirmedV1(t *testing.T) {
	home := t.TempDir()
	t.Setenv("ONEAGENTS_HOME", home)
	db, err := meta.OpenDefault()
	if err != nil {
		t.Fatal(err)
	}
	sqlDB := db.SQL()
	_, err = sqlDB.Exec(`CREATE TABLE agents_roundtable_rooms (
		id TEXT PRIMARY KEY,
		title TEXT NOT NULL DEFAULT '',
		state TEXT NOT NULL,
		brief_json TEXT,
		summary_r2 TEXT NOT NULL DEFAULT '',
		summary_r3 TEXT NOT NULL DEFAULT '',
		created_at TEXT NOT NULL,
		updated_at TEXT NOT NULL
	)`)
	if err != nil {
		t.Fatal(err)
	}
	legacyReq := validBrief("legacy")
	content, _ := json.Marshal(roundtable.Brief{
		Title:           legacyReq.Title,
		Question:        legacyReq.Question,
		Constraints:     legacyReq.Constraints,
		SuccessCriteria: legacyReq.SuccessCriteria,
		ProductKind:     legacyReq.ProductKind,
	})
	now := time.Now().UTC().Format(time.RFC3339Nano)
	_, err = sqlDB.Exec(
		`INSERT INTO agents_roundtable_rooms
			(id, title, state, brief_json, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		"legacy-room", "旧房间", string(roundtable.StateWaitingR2), string(content), now, now,
	)
	if err != nil {
		t.Fatal(err)
	}

	store, err := roundtable.NewStore()
	if err != nil {
		t.Fatalf("NewStore migration: %v", err)
	}
	room, err := store.GetRoom("legacy-room")
	if err != nil {
		t.Fatal(err)
	}
	if room.CurrentBriefVersion != 1 ||
		room.ConfirmedBriefVersion != 1 ||
		room.CurrentBrief == nil ||
		room.CurrentBrief.Status != roundtable.BriefStatusConfirmed ||
		room.CurrentBrief.Content.Title != "legacy" ||
		room.Brief == nil ||
		room.Brief.Title != "legacy" {
		t.Fatalf("legacy migration room=%+v current=%+v", room, room.CurrentBrief)
	}
}
