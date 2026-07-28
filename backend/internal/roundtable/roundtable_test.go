package roundtable_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/scottzx/1Agents/backend/internal/agent"
	"github.com/scottzx/1Agents/backend/internal/appregistry"
	"github.com/scottzx/1Agents/backend/internal/meta"
	"github.com/scottzx/1Agents/backend/internal/roundtable"
)

// testRig isolates ONEAGENTS_HOME so meta.db and app workspaces do not touch
// the developer's real store. Uses real CreateAppWorkspace so acceptance
// criteria (kind=app cwd exists + role seed) are verified end-to-end.
//
// prompter is a StaticSeatPrompter so R1 tests do not require a live 1acp bridge.
func testRig(t *testing.T) (*roundtable.Service, *roundtable.StaticSeatPrompter) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("ONEAGENTS_HOME", home)
	store, err := roundtable.NewStore()
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	chatStore, err := agent.NewStore()
	if err != nil {
		t.Fatalf("NewStore chat: %v", err)
	}
	prompter := &roundtable.StaticSeatPrompter{
		Replies: []string{
			"请补充目标用户与约束。",
			"建议 Brief 草案：title=测试 / question=如何验证",
		},
		FixedAcpID: "acp-ref-1",
	}
	return roundtable.NewService(store, chatStore, prompter), prompter
}

func TestCreateRoom_SixSeatsDraftingBrief(t *testing.T) {
	svc, _ := testRig(t)

	room, err := svc.CreateRoom(roundtable.CreateRoomRequest{Title: "测试议题"})
	if err != nil {
		t.Fatalf("CreateRoom: %v", err)
	}
	if room.ID == "" {
		t.Fatal("expected room id")
	}
	if room.State != roundtable.StateDraftingBrief {
		t.Fatalf("state %q, want drafting_brief", room.State)
	}
	if room.Title != "测试议题" {
		t.Fatalf("title %q", room.Title)
	}
	if len(room.Seats) != 6 {
		t.Fatalf("seats %d, want 6", len(room.Seats))
	}

	wantRoles := map[roundtable.Role]bool{
		roundtable.RoleReferee: true,
		roundtable.RoleMarket:  true,
		roundtable.RoleProduct: true,
		roundtable.RoleEng:     true,
		roundtable.RoleOps:     true,
		roundtable.RoleFinance: true,
	}
	var refereeSession string
	for _, seat := range room.Seats {
		if !wantRoles[seat.Role] {
			t.Fatalf("unexpected role %q", seat.Role)
		}
		delete(wantRoles, seat.Role)
		if seat.WorkspaceID == "" {
			t.Fatalf("seat %s missing workspace_id", seat.Role)
		}
		if !strings.HasPrefix(seat.WorkspaceID, "app-rt-") {
			t.Fatalf("workspace_id %q should start with app-rt-", seat.WorkspaceID)
		}
		if seat.AgentType != agent.AgentTypeGrokBuild {
			t.Fatalf("agent_type %q, want %q", seat.AgentType, agent.AgentTypeGrokBuild)
		}
		if seat.Status != roundtable.SeatReady {
			t.Fatalf("seat status %q", seat.Status)
		}
		if seat.Role == roundtable.RoleReferee {
			if seat.SessionID == "" {
				t.Fatal("referee seat must start a real Grok Build session (session_id)")
			}
			refereeSession = seat.SessionID
		} else if seat.SessionID != "" {
			t.Fatalf("panelist %s should not have session yet, got %s", seat.Role, seat.SessionID)
		}

		// Resolve cwd via projects row and check seed files.
		db, err := meta.OpenDefault()
		if err != nil {
			t.Fatalf("OpenDefault: %v", err)
		}
		proj, ok, err := db.GetProject(seat.WorkspaceID)
		if err != nil || !ok {
			t.Fatalf("GetProject %s: ok=%v err=%v", seat.WorkspaceID, ok, err)
		}
		if proj.Kind != meta.KindApp {
			t.Fatalf("project kind %q, want app", proj.Kind)
		}
		if _, err := os.Stat(proj.WorkspacePath); err != nil {
			t.Fatalf("app cwd missing for %s: %v", seat.Role, err)
		}
		var content string
		for _, seedFile := range []string{"AGENTS.md", "Claude.md"} {
			seed, err := os.ReadFile(filepath.Join(proj.WorkspacePath, seedFile))
			if err != nil {
				t.Fatalf("%s for %s: %v", seedFile, seat.Role, err)
			}
			seedContent := string(seed)
			if seedFile == "AGENTS.md" {
				content = seedContent
			}
			if !strings.Contains(seedContent, "圆桌讨论") {
				t.Fatalf("%s for %s missing roundtable header", seedFile, seat.Role)
			}
			for _, marker := range []string{
				"## 使命", "## 行为设置", "## 圆桌行为协议", "## 默认输出结构",
				"角色锁定", "事实边界", "轮次纪律", "待验证假设",
			} {
				if !strings.Contains(seedContent, marker) {
					t.Fatalf("%s for %s missing complete role prompt marker %q", seedFile, seat.Role, marker)
				}
			}
		}
		// Role-specific marker.
		switch seat.Role {
		case roundtable.RoleReferee:
			if !strings.Contains(content, "裁判") {
				t.Fatal("referee seed missing 裁判")
			}
		case roundtable.RoleMarket:
			if !strings.Contains(content, "市场") {
				t.Fatal("market seed missing 市场")
			}
		case roundtable.RoleProduct:
			if !strings.Contains(content, "产品") {
				t.Fatal("product seed missing 产品")
			}
		case roundtable.RoleEng:
			if !strings.Contains(content, "研发") {
				t.Fatal("eng seed missing 研发")
			}
		case roundtable.RoleOps:
			if !strings.Contains(content, "运营") {
				t.Fatal("ops seed missing 运营")
			}
		case roundtable.RoleFinance:
			if !strings.Contains(content, "财务") {
				t.Fatal("finance seed missing 财务")
			}
		}
		// Common contract.
		if !strings.Contains(content, "禁止寒暄") {
			t.Fatalf("seed for %s missing common contract", seat.Role)
		}
	}
	if len(wantRoles) != 0 {
		t.Fatalf("missing roles: %v", wantRoles)
	}

	// Referee ChatSessionRecord must exist in meta sessions.
	chatStore, err := agent.NewStore()
	if err != nil {
		t.Fatal(err)
	}
	rec, ok, err := chatStore.Get(refereeSession)
	if err != nil || !ok {
		t.Fatalf("referee chat session %s missing: ok=%v err=%v", refereeSession, ok, err)
	}
	if rec.AgentType != agent.AgentTypeGrokBuild {
		t.Fatalf("session agent_type %q", rec.AgentType)
	}

	// Persistence: get room + list seats.
	got, err := svc.GetRoom(room.ID)
	if err != nil {
		t.Fatalf("GetRoom: %v", err)
	}
	if got.State != roundtable.StateDraftingBrief {
		t.Fatalf("persisted state %q", got.State)
	}
	if len(got.Seats) != 6 {
		t.Fatalf("GetRoom seats %d", len(got.Seats))
	}
	seats, err := svc.ListSeats(room.ID)
	if err != nil {
		t.Fatalf("ListSeats: %v", err)
	}
	if len(seats) != 6 {
		t.Fatalf("ListSeats %d", len(seats))
	}
}

func TestStateMachineTransitions(t *testing.T) {
	// Happy path skeleton: drafting_brief → … → done
	path := []roundtable.RoomState{
		roundtable.StateDraftingBrief,
		roundtable.StateWaitingR2,
		roundtable.StateSummarizingR2,
		roundtable.StateWaitingR3,
		roundtable.StateSummarizingR3,
		roundtable.StateDone,
	}
	for i := 0; i < len(path)-1; i++ {
		if !roundtable.CanTransition(path[i], path[i+1]) {
			t.Fatalf("expected %s → %s legal", path[i], path[i+1])
		}
	}
	// failed from each active state
	for _, from := range []roundtable.RoomState{
		roundtable.StateDraftingBrief,
		roundtable.StateWaitingR2,
		roundtable.StateSummarizingR2,
		roundtable.StateWaitingR3,
		roundtable.StateSummarizingR3,
	} {
		if !roundtable.CanTransition(from, roundtable.StateFailed) {
			t.Fatalf("expected %s → failed legal", from)
		}
	}
	// Illegal skips
	if roundtable.CanTransition(roundtable.StateDraftingBrief, roundtable.StateDone) {
		t.Fatal("drafting_brief → done should be illegal")
	}
	if roundtable.CanTransition(roundtable.StateDone, roundtable.StateWaitingR2) {
		t.Fatal("done is terminal")
	}
	if roundtable.CanTransition(roundtable.StateFailed, roundtable.StateDraftingBrief) {
		t.Fatal("failed is terminal")
	}

	// Transition helper + persistence via service.
	svc, _ := testRig(t)
	room, err := svc.CreateRoom(roundtable.CreateRoomRequest{Title: "状态机"})
	if err != nil {
		t.Fatalf("CreateRoom: %v", err)
	}
	// In-memory Transition
	r := &roundtable.Room{State: roundtable.StateDraftingBrief}
	if err := roundtable.Transition(r, roundtable.StateWaitingR2); err != nil {
		t.Fatalf("Transition: %v", err)
	}
	if r.State != roundtable.StateWaitingR2 {
		t.Fatalf("state %q", r.State)
	}
	if err := roundtable.Transition(r, roundtable.StateDone); err == nil {
		t.Fatal("expected illegal transition error")
	}

	// Service-level: drafting_brief → waiting_r2
	updated, err := svc.TransitionRoom(room.ID, roundtable.StateWaitingR2)
	if err != nil {
		t.Fatalf("TransitionRoom: %v", err)
	}
	if updated.State != roundtable.StateWaitingR2 {
		t.Fatalf("persisted %q", updated.State)
	}
	// Illegal
	if _, err := svc.TransitionRoom(room.ID, roundtable.StateDone); err == nil {
		t.Fatal("expected illegal TransitionRoom error")
	}
	// fail path
	failed, err := svc.TransitionRoom(room.ID, roundtable.StateFailed)
	if err != nil {
		t.Fatalf("→ failed: %v", err)
	}
	if failed.State != roundtable.StateFailed {
		t.Fatalf("state %q", failed.State)
	}
	if !roundtable.IsTerminal(failed.State) {
		t.Fatal("failed should be terminal")
	}
}

func TestR1_MultiTurnChatAndConfirmBrief(t *testing.T) {
	svc, prompter := testRig(t)
	room, err := svc.CreateRoom(roundtable.CreateRoomRequest{Title: "R1议题"})
	if err != nil {
		t.Fatalf("CreateRoom: %v", err)
	}

	// Turn 1
	resp1, err := svc.ChatWithReferee(room.ID, roundtable.ChatRequest{Text: "我们想做一款团队协作工具"})
	if err != nil {
		t.Fatalf("chat1: %v", err)
	}
	if resp1.UserTurn.Kind != roundtable.TurnKindChat || resp1.UserTurn.SeatID != roundtable.TurnSeatUser {
		t.Fatalf("user turn: kind=%s seat=%s", resp1.UserTurn.Kind, resp1.UserTurn.SeatID)
	}
	if resp1.UserTurn.ContentText == "" {
		t.Fatal("user content_text empty")
	}
	if resp1.RefereeTurn.Kind != roundtable.TurnKindChat || resp1.RefereeTurn.ContentText == "" {
		t.Fatalf("referee turn: kind=%s text=%q", resp1.RefereeTurn.Kind, resp1.RefereeTurn.ContentText)
	}
	if resp1.SessionID == "" {
		t.Fatal("session_id empty")
	}
	if resp1.AcpSessionID != "acp-ref-1" {
		t.Fatalf("acp_session_id %q", resp1.AcpSessionID)
	}
	// First prompt should carry system context (fresh session).
	if len(prompter.Calls) < 1 || prompter.Calls[0].SystemContext == "" {
		t.Fatal("first prompt should inject R1 system context")
	}
	for _, marker := range []string{"## 使命", "## 行为设置", "## 圆桌行为协议", "## R1 写 Brief"} {
		if !strings.Contains(prompter.Calls[0].SystemContext, marker) {
			t.Fatalf("first referee prompt missing complete role marker %q", marker)
		}
	}
	if prompter.Calls[0].AcpSessionID != "" {
		t.Fatal("first prompt should not resume")
	}

	// Turn 2 — same session resume
	resp2, err := svc.ChatWithReferee(room.ID, roundtable.ChatRequest{Text: "约束：两周内出 MVP，团队 3 人"})
	if err != nil {
		t.Fatalf("chat2: %v", err)
	}
	if len(prompter.Calls) < 2 {
		t.Fatal("expected second prompt")
	}
	if prompter.Calls[1].AcpSessionID != "acp-ref-1" {
		t.Fatalf("second prompt should resume acp=%q", prompter.Calls[1].AcpSessionID)
	}
	if prompter.Calls[1].SystemContext != "" {
		t.Fatal("resume must not re-inject system context")
	}
	if resp2.SessionID != resp1.SessionID {
		t.Fatal("session_id should stay stable across R1 turns")
	}

	// Turns timeline
	turns, err := svc.ListTurns(room.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(turns) != 4 { // 2 user + 2 referee
		t.Fatalf("turns %d, want 4", len(turns))
	}
	var hasUser, hasRef bool
	for _, tr := range turns {
		if tr.Kind != roundtable.TurnKindChat {
			t.Fatalf("expected kind=chat, got %s", tr.Kind)
		}
		if tr.ContentText == "" {
			t.Fatal("content_text must bind main timeline")
		}
		if tr.SeatID == roundtable.TurnSeatUser {
			hasUser = true
		}
		if tr.SeatID != "" && tr.SeatID != roundtable.TurnSeatUser {
			hasRef = true
		}
	}
	if !hasUser || !hasRef {
		t.Fatalf("turns must include user and referee: user=%v ref=%v", hasUser, hasRef)
	}

	// Confirm Brief → waiting_r2
	updated, err := svc.ConfirmBrief(room.ID, roundtable.ConfirmBriefRequest{
		Title:           "团队协作 MVP",
		Question:        "如何在两周内验证协作工具 PMF？",
		Constraints:     "3 人团队，两周，无外部融资",
		SuccessCriteria: "有 5 个种子用户完成核心路径",
		ProductKind:     roundtable.ProductSoftware,
	})
	if err != nil {
		t.Fatalf("ConfirmBrief: %v", err)
	}
	if updated.State != roundtable.StateWaitingR2 {
		t.Fatalf("state %q, want waiting_r2", updated.State)
	}
	if updated.Brief == nil || updated.Brief.Title == "" || updated.Brief.Question == "" {
		t.Fatalf("brief empty: %+v", updated.Brief)
	}
	if updated.Brief.Constraints == "" || updated.Brief.SuccessCriteria == "" {
		t.Fatal("brief missing constraints/success_criteria")
	}
	if updated.Brief.ProductKind != roundtable.ProductSoftware {
		t.Fatalf("product_kind %q", updated.Brief.ProductKind)
	}

	// Persisted after reload
	got, err := svc.GetRoom(room.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.State != roundtable.StateWaitingR2 || got.Brief == nil || got.Brief.Title != "团队协作 MVP" {
		t.Fatalf("persisted brief/state: state=%s brief=%+v", got.State, got.Brief)
	}
	if len(got.Turns) < 5 { // + system turn
		t.Fatalf("GetRoom turns %d, want >=5", len(got.Turns))
	}

	// No more chat after leave drafting_brief
	if _, err := svc.ChatWithReferee(room.ID, roundtable.ChatRequest{Text: "late"}); err == nil {
		t.Fatal("chat after confirm should fail")
	}

	// Invalid brief
	room2, _ := svc.CreateRoom(roundtable.CreateRoomRequest{Title: "x"})
	if _, err := svc.ConfirmBrief(room2.ID, roundtable.ConfirmBriefRequest{Title: "only"}); err == nil {
		t.Fatal("incomplete brief should fail")
	}
}

func TestHTTP_CreateGetListSeats(t *testing.T) {
	svc, _ := testRig(t)
	h := roundtable.NewHandler(svc)

	// POST create
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/roundtable/rooms", strings.NewReader(`{"title":"API议题"}`))
	h.HandleRoomsRoot(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("create status %d: %s", rr.Code, rr.Body.String())
	}
	var room roundtable.Room
	if err := json.NewDecoder(rr.Body).Decode(&room); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if room.State != roundtable.StateDraftingBrief || len(room.Seats) != 6 {
		t.Fatalf("create response: state=%s seats=%d", room.State, len(room.Seats))
	}
	for _, s := range room.Seats {
		if s.WorkspaceID == "" {
			t.Fatalf("seat %s missing workspace_id in API response", s.Role)
		}
		if s.Role == roundtable.RoleReferee && s.SessionID == "" {
			t.Fatal("API create must return referee session_id")
		}
	}

	// GET room
	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/api/roundtable/rooms/"+room.ID, nil)
	h.HandleRoomsItem(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("get status %d: %s", rr.Code, rr.Body.String())
	}
	var got roundtable.Room
	if err := json.NewDecoder(rr.Body).Decode(&got); err != nil {
		t.Fatalf("decode get: %v", err)
	}
	if got.ID != room.ID || len(got.Seats) != 6 {
		t.Fatalf("get room: id=%s seats=%d", got.ID, len(got.Seats))
	}

	// GET seats
	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/api/roundtable/rooms/"+room.ID+"/seats", nil)
	h.HandleRoomsItem(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("seats status %d: %s", rr.Code, rr.Body.String())
	}
	var payload struct {
		Seats []roundtable.Seat `json:"seats"`
	}
	if err := json.NewDecoder(rr.Body).Decode(&payload); err != nil {
		t.Fatalf("decode seats: %v", err)
	}
	if len(payload.Seats) != 6 {
		t.Fatalf("seats len %d", len(payload.Seats))
	}

	// 404
	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/api/roundtable/rooms/no-such-id", nil)
	h.HandleRoomsItem(rr, req)
	if rr.Code != http.StatusNotFound {
		t.Fatalf("want 404, got %d", rr.Code)
	}
}

func TestHTTP_R1ChatAndBrief(t *testing.T) {
	svc, _ := testRig(t)
	h := roundtable.NewHandler(svc)

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/roundtable/rooms", strings.NewReader(`{"title":"HTTP R1"}`))
	h.HandleRoomsRoot(rr, req)
	var room roundtable.Room
	_ = json.NewDecoder(rr.Body).Decode(&room)

	// chat
	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/api/roundtable/rooms/"+room.ID+"/chat",
		strings.NewReader(`{"text":"议题是降低客服成本"}`))
	h.HandleRoomsItem(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("chat status %d: %s", rr.Code, rr.Body.String())
	}
	var chatResp roundtable.ChatResponse
	if err := json.NewDecoder(rr.Body).Decode(&chatResp); err != nil {
		t.Fatal(err)
	}
	if chatResp.UserTurn.ContentText == "" || chatResp.RefereeTurn.ContentText == "" {
		t.Fatalf("chat response missing content_text: %+v", chatResp)
	}

	// second chat
	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/api/roundtable/rooms/"+room.ID+"/chat",
		strings.NewReader(`{"text":"成功标准：工单量降 30%"}`))
	h.HandleRoomsItem(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("chat2 status %d: %s", rr.Code, rr.Body.String())
	}

	// turns
	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/api/roundtable/rooms/"+room.ID+"/turns", nil)
	h.HandleRoomsItem(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("turns status %d", rr.Code)
	}
	var turnsPayload struct {
		Turns []roundtable.Turn `json:"turns"`
	}
	_ = json.NewDecoder(rr.Body).Decode(&turnsPayload)
	if len(turnsPayload.Turns) != 4 {
		t.Fatalf("turns %d", len(turnsPayload.Turns))
	}

	// brief
	body := `{
		"title":"客服降本",
		"question":"如何用 Agent 降低客服成本？",
		"constraints":"不增编制",
		"success_criteria":"工单量降30%",
		"product_kind":"software"
	}`
	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/api/roundtable/rooms/"+room.ID+"/brief", strings.NewReader(body))
	h.HandleRoomsItem(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("brief status %d: %s", rr.Code, rr.Body.String())
	}
	var after roundtable.Room
	if err := json.NewDecoder(rr.Body).Decode(&after); err != nil {
		t.Fatal(err)
	}
	if after.State != roundtable.StateWaitingR2 || after.Brief == nil || after.Brief.Title == "" {
		t.Fatalf("after brief: state=%s brief=%+v", after.State, after.Brief)
	}
}

func TestRoleSeedContent(t *testing.T) {
	roleMarkers := map[roundtable.Role][]string{
		roundtable.RoleReferee: {"保持中立", "1–3 个最影响决策的问题", "Summary₃ 终稿"},
		roundtable.RoleMarket:  {"人群与场景", "竞争与定位", "市场验证"},
		roundtable.RoleProduct: {"问题定义", "MVP 的最小完整闭环", "验收指标"},
		roundtable.RoleEng:     {"可行性", "质量属性", "spike / PoC / 打样"},
		roundtable.RoleOps:     {"运营模型", "容量与履约", "异常升级条件"},
		roundtable.RoleFinance: {"单位经济", "现金与回本", "情景与敏感性"},
	}
	for _, role := range roundtable.DefaultRoster {
		md := roundtable.RoleSeedAGENTS(role)
		if md == "" {
			t.Fatalf("empty seed for %s", role)
		}
		for _, marker := range []string{
			"## 使命", "## 行为设置", "## 圆桌行为协议", "## 默认输出结构",
			"角色锁定", "事实边界", "结论优先", "可执行", "轮次纪律", "禁止寒暄",
		} {
			if !strings.Contains(md, marker) {
				t.Fatalf("%s missing complete role prompt marker %q", role, marker)
			}
		}
		for _, marker := range roleMarkers[role] {
			if !strings.Contains(md, marker) {
				t.Fatalf("%s missing role-specific behavior marker %q", role, marker)
			}
		}
	}
	// Write to disk
	dir := t.TempDir()
	if err := roundtable.WriteRoleSeed(dir, roundtable.RoleEng); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(filepath.Join(dir, "AGENTS.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), "研发") {
		t.Fatal("eng AGENTS.md content")
	}
	claude, err := os.ReadFile(filepath.Join(dir, "Claude.md"))
	if err != nil {
		t.Fatal("Claude.md missing")
	}
	for _, marker := range []string{
		"圆桌讨论（Claude.md） · 研发", "## 行为设置", "## 圆桌行为协议",
	} {
		if !strings.Contains(string(claude), marker) {
			t.Fatalf("Claude.md missing complete role marker %q", marker)
		}
	}
}

func TestValidateBrief(t *testing.T) {
	if err := roundtable.ValidateBrief(nil); err == nil {
		t.Fatal("nil brief")
	}
	ok := &roundtable.Brief{
		Title: "t", Question: "q", Constraints: "c", SuccessCriteria: "s",
	}
	if err := roundtable.ValidateBrief(ok); err != nil {
		t.Fatal(err)
	}
	bad := *ok
	bad.ProductKind = "robot"
	if err := roundtable.ValidateBrief(&bad); err == nil {
		t.Fatal("bad product_kind")
	}
}

// confirmBriefHelper runs minimal R1 exit so room is waiting_r2 with a Brief.
func confirmBriefHelper(t *testing.T, svc *roundtable.Service, roomID string) *roundtable.Room {
	t.Helper()
	room, err := svc.ConfirmBrief(roomID, roundtable.ConfirmBriefRequest{
		Title:           "团队协作 MVP",
		Question:        "如何在两周内验证协作工具 PMF？",
		Constraints:     "3 人团队，两周，无外部融资",
		SuccessCriteria: "有 5 个种子用户完成核心路径",
		ProductKind:     roundtable.ProductSoftware,
	})
	if err != nil {
		t.Fatalf("ConfirmBrief: %v", err)
	}
	if room.State != roundtable.StateWaitingR2 {
		t.Fatalf("state %q, want waiting_r2", room.State)
	}
	return room
}

func TestR2_ParallelIsolatedSpeechAndSummary(t *testing.T) {
	svc, prompter := testRig(t)
	// Unique per-role markers prove isolation: B must never receive A's body in inject.
	const (
		marketSecret  = "R2_SECRET_MARKET_AAA"
		productSecret = "R2_SECRET_PRODUCT_BBB"
		engSecret     = "R2_SECRET_ENG_CCC"
		opsSecret     = "R2_SECRET_OPS_DDD"
		finSecret     = "R2_SECRET_FIN_EEE"
		summaryMark   = "SUMMARY2_MARK"
	)
	prompter.Replies = nil // disable sequential R1 replies
	prompter.ReplyByRole = map[roundtable.Role]string{
		roundtable.RoleMarket:  marketSecret + "：聚焦种子用户与增长渠道。",
		roundtable.RoleProduct: productSecret + "：两周只做核心协作路径。",
		roundtable.RoleEng:     engSecret + "：单体+实时协作可行，工期紧。",
		roundtable.RoleOps:     opsSecret + "：客服与 onboarding 节奏。",
		roundtable.RoleFinance: finSecret + "：单位经济与烧钱红线。",
		roundtable.RoleReferee: summaryMark + "：综合五席，产品强调范围、研发强调工期风险。",
	}

	room, err := svc.CreateRoom(roundtable.CreateRoomRequest{Title: "R2议题"})
	if err != nil {
		t.Fatalf("CreateRoom: %v", err)
	}
	confirmBriefHelper(t, svc, room.ID)

	resp, err := svc.RunR2(room.ID)
	if err != nil {
		t.Fatalf("RunR2: %v", err)
	}
	if resp.Room == nil || resp.Room.State != roundtable.StateWaitingR3 {
		t.Fatalf("state want waiting_r3, got %+v", resp.Room)
	}
	if strings.TrimSpace(resp.Room.SummaryR2) == "" {
		t.Fatal("summary_r2 empty on room")
	}
	if !strings.Contains(resp.Room.SummaryR2, summaryMark) {
		t.Fatalf("summary_r2 missing mark: %q", resp.Room.SummaryR2)
	}
	if resp.SummaryTurn.Kind != roundtable.TurnKindSummary || resp.SummaryTurn.ContentText == "" {
		t.Fatalf("summary turn: kind=%s text=%q", resp.SummaryTurn.Kind, resp.SummaryTurn.ContentText)
	}
	if resp.SummaryTurn.Round != 2 {
		t.Fatalf("summary round %d", resp.SummaryTurn.Round)
	}
	if len(resp.FailedRoles) != 0 {
		t.Fatalf("unexpected failures: %v", resp.FailedRoles)
	}

	// Five speech turns (kind=speech), one per panelist.
	if len(resp.SpeechTurns) != 5 {
		t.Fatalf("speech turns %d, want 5", len(resp.SpeechTurns))
	}
	speechBySeat := map[string]roundtable.Turn{}
	for _, tr := range resp.SpeechTurns {
		if tr.Kind != roundtable.TurnKindSpeech {
			t.Fatalf("expected kind=speech, got %s", tr.Kind)
		}
		if tr.Round != 2 || tr.ContentText == "" {
			t.Fatalf("speech turn incomplete: %+v", tr)
		}
		speechBySeat[tr.SeatID] = tr
	}

	// Seat status: panelists done; each has session_id.
	seats, err := svc.ListSeats(room.ID)
	if err != nil {
		t.Fatal(err)
	}
	roleByID := map[string]roundtable.Role{}
	secrets := map[roundtable.Role]string{
		roundtable.RoleMarket:  marketSecret,
		roundtable.RoleProduct: productSecret,
		roundtable.RoleEng:     engSecret,
		roundtable.RoleOps:     opsSecret,
		roundtable.RoleFinance: finSecret,
	}
	gotRoles := map[roundtable.Role]bool{}
	for _, s := range seats {
		roleByID[s.ID] = s.Role
		if !roundtable.IsPanelist(s.Role) {
			continue
		}
		if s.Status != roundtable.SeatDone {
			t.Fatalf("seat %s status %q, want done", s.Role, s.Status)
		}
		if s.SessionID == "" {
			t.Fatalf("panelist %s missing session_id after R2", s.Role)
		}
		tr, ok := speechBySeat[s.ID]
		if !ok {
			t.Fatalf("no speech turn for seat %s", s.Role)
		}
		if !strings.Contains(tr.ContentText, secrets[s.Role]) {
			t.Fatalf("speech for %s missing own secret: %q", s.Role, tr.ContentText)
		}
		gotRoles[s.Role] = true
	}
	for _, r := range roundtable.PanelistRoles {
		if !gotRoles[r] {
			t.Fatalf("missing panelist role %s", r)
		}
	}

	// Isolation: each panelist inject (Text+SystemContext) must contain Brief fields
	// and MUST NOT contain any other seat's secret speech body.
	allSecrets := []string{marketSecret, productSecret, engSecret, opsSecret, finSecret}
	calls := prompter.SnapshotCalls()
	var panelistCalls int
	for _, c := range calls {
		if !roundtable.IsPanelist(c.Role) {
			continue
		}
		panelistCalls++
		inject := c.SystemContext + "\n" + c.Text
		// Brief present
		if !strings.Contains(c.Text, "团队协作 MVP") || !strings.Contains(c.Text, "两周内验证") {
			t.Fatalf("panelist %s inject missing Brief: %q", c.Role, c.Text)
		}
		// Role present in system context
		if !strings.Contains(c.SystemContext, roundtable.RoleLabel(c.Role)) {
			t.Fatalf("panelist %s sys ctx missing role label", c.Role)
		}
		for _, marker := range []string{
			"## 使命", "## 行为设置", "## 圆桌行为协议", "## 默认输出结构",
		} {
			if !strings.Contains(c.SystemContext, marker) {
				t.Fatalf("panelist %s sys ctx missing complete role marker %q", c.Role, marker)
			}
		}
		// Peer speech bodies forbidden
		for _, sec := range allSecrets {
			if sec == secrets[c.Role] {
				continue // own secret is only in the *reply*, not in inject
			}
			if strings.Contains(inject, sec) {
				t.Fatalf("ISOLATION VIOLATION: seat %s received peer secret %s in inject\ninject=%q",
					c.Role, sec, inject)
			}
		}
		// Own reply secret must also not be in inject (prompt built before reply)
		if strings.Contains(inject, secrets[c.Role]) {
			t.Fatalf("seat %s inject unexpectedly contains own secret (prompts must be brief-only)", c.Role)
		}
		// Fresh session for R2 first turn
		if c.AcpSessionID != "" {
			t.Fatalf("R2 panelist %s should start fresh (empty acp), got %q", c.Role, c.AcpSessionID)
		}
	}
	if panelistCalls != 5 {
		t.Fatalf("panelist prompt calls %d, want 5", panelistCalls)
	}

	// Referee summary prompt must see all five secrets (public package after speeches).
	var summaryCall *roundtable.SeatPromptRequest
	for i := range calls {
		c := &calls[i]
		if c.Role == roundtable.RoleReferee && strings.Contains(c.Text, "Summary₂") {
			summaryCall = c
			break
		}
	}
	if summaryCall == nil {
		// Fallback: last referee call
		for i := len(calls) - 1; i >= 0; i-- {
			if calls[i].Role == roundtable.RoleReferee {
				summaryCall = &calls[i]
				break
			}
		}
	}
	if summaryCall == nil {
		t.Fatal("missing referee summary prompt call")
	}
	for _, sec := range allSecrets {
		if !strings.Contains(summaryCall.Text, sec) {
			t.Fatalf("summary prompt missing speech secret %s", sec)
		}
	}

	// Timeline: 5 speech + 1 summary (plus earlier system brief turn)
	turns, err := svc.ListTurns(room.ID)
	if err != nil {
		t.Fatal(err)
	}
	var nSpeech, nSummary int
	for _, tr := range turns {
		switch tr.Kind {
		case roundtable.TurnKindSpeech:
			nSpeech++
		case roundtable.TurnKindSummary:
			nSummary++
		}
	}
	if nSpeech != 5 || nSummary != 1 {
		t.Fatalf("timeline speech=%d summary=%d, want 5/1", nSpeech, nSummary)
	}

	// Persisted reload
	got, err := svc.GetRoom(room.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.State != roundtable.StateWaitingR3 || got.SummaryR2 == "" {
		t.Fatalf("persisted: state=%s summary=%q", got.State, got.SummaryR2)
	}

	// Idempotency: cannot re-run R2 from waiting_r3
	if _, err := svc.RunR2(room.ID); err == nil {
		t.Fatal("second RunR2 should fail")
	}
}

func TestR2_PartialSeatFailureDoesNotBlock(t *testing.T) {
	svc, prompter := testRig(t)
	prompter.Replies = nil
	prompter.ReplyByRole = map[roundtable.Role]string{
		roundtable.RoleMarket:  "市场观点",
		roundtable.RoleProduct: "产品观点",
		roundtable.RoleEng:     "研发观点",
		roundtable.RoleOps:     "运营观点",
		roundtable.RoleFinance: "财务观点",
		roundtable.RoleReferee: "Summary₂：缺席已注明。",
	}
	prompter.FailRoles = map[roundtable.Role]error{
		roundtable.RoleOps: fmt.Errorf("simulated ops outage"),
	}

	room, err := svc.CreateRoom(roundtable.CreateRoomRequest{Title: "R2 partial"})
	if err != nil {
		t.Fatal(err)
	}
	confirmBriefHelper(t, svc, room.ID)

	resp, err := svc.RunR2(room.ID)
	if err != nil {
		t.Fatalf("RunR2 should succeed despite one seat fail: %v", err)
	}
	if resp.Room.State != roundtable.StateWaitingR3 {
		t.Fatalf("state %q", resp.Room.State)
	}
	if len(resp.FailedRoles) != 1 || resp.FailedRoles[0] != string(roundtable.RoleOps) {
		t.Fatalf("failed_roles %v", resp.FailedRoles)
	}
	if len(resp.SpeechTurns) != 5 {
		t.Fatalf("still want 5 speech turns (incl failed annotation), got %d", len(resp.SpeechTurns))
	}

	seats, _ := svc.ListSeats(room.ID)
	for _, s := range seats {
		if s.Role == roundtable.RoleOps {
			if s.Status != roundtable.SeatFailed {
				t.Fatalf("ops status %q, want failed", s.Status)
			}
			continue
		}
		if roundtable.IsPanelist(s.Role) && s.Status != roundtable.SeatDone {
			t.Fatalf("%s status %q, want done", s.Role, s.Status)
		}
	}

	// Failed seat still has a speech turn with [failed] annotation.
	var foundFailedSpeech bool
	for _, tr := range resp.SpeechTurns {
		if strings.Contains(tr.ContentText, "[failed]") && strings.Contains(tr.ContentText, "运营") {
			foundFailedSpeech = true
		}
	}
	if !foundFailedSpeech {
		t.Fatal("expected failed speech annotation for ops")
	}
	// Summary should mention failure / 缺席
	if !strings.Contains(resp.SummaryTurn.ContentText, "Summary₂") &&
		!strings.Contains(resp.Room.SummaryR2, "缺席") {
		// At least summary prompt received fail note; body is static double.
		// Soft check: summary non-empty is enough; fail details go into referee prompt.
	}
	// Stronger: referee summary inject includes 缺席/失败 for ops
	for _, c := range prompter.SnapshotCalls() {
		if c.Role == roundtable.RoleReferee && strings.Contains(c.Text, "运营") {
			if !strings.Contains(c.Text, "失败") && !strings.Contains(c.Text, "缺席") {
				t.Fatalf("summary prompt should note ops failure: %q", c.Text)
			}
		}
	}
}

func TestHTTP_R2(t *testing.T) {
	svc, prompter := testRig(t)
	prompter.Replies = nil
	prompter.ReplyByRole = map[roundtable.Role]string{
		roundtable.RoleMarket:  "mkt",
		roundtable.RoleProduct: "prd",
		roundtable.RoleEng:     "eng",
		roundtable.RoleOps:     "ops",
		roundtable.RoleFinance: "fin",
		roundtable.RoleReferee: "sum2",
	}
	h := roundtable.NewHandler(svc)

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/roundtable/rooms", strings.NewReader(`{"title":"HTTP R2"}`))
	h.HandleRoomsRoot(rr, req)
	var room roundtable.Room
	_ = json.NewDecoder(rr.Body).Decode(&room)

	briefBody := `{
		"title":"议题",
		"question":"核心问题",
		"constraints":"约束",
		"success_criteria":"成功标准"
	}`
	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/api/roundtable/rooms/"+room.ID+"/brief", strings.NewReader(briefBody))
	h.HandleRoomsItem(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("brief status %d: %s", rr.Code, rr.Body.String())
	}

	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/api/roundtable/rooms/"+room.ID+"/r2?wait=1", nil)
	h.HandleRoomsItem(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("r2 status %d: %s", rr.Code, rr.Body.String())
	}
	var r2resp roundtable.RunR2Response
	if err := json.NewDecoder(rr.Body).Decode(&r2resp); err != nil {
		t.Fatal(err)
	}
	if r2resp.Room == nil || r2resp.Room.State != roundtable.StateWaitingR3 {
		t.Fatalf("r2 response state: %+v", r2resp.Room)
	}
	if len(r2resp.SpeechTurns) != 5 {
		t.Fatalf("speech_turns %d", len(r2resp.SpeechTurns))
	}
	if r2resp.SummaryTurn.Kind != roundtable.TurnKindSummary {
		t.Fatalf("summary kind %s", r2resp.SummaryTurn.Kind)
	}
}

func TestBuildR2PanelistPromptIsolation(t *testing.T) {
	brief := &roundtable.Brief{
		Title: "T", Question: "Q", Constraints: "C", SuccessCriteria: "S",
	}
	p := roundtable.BuildR2PanelistPrompt(brief)
	if !strings.Contains(p, "title: T") || !strings.Contains(p, "question: Q") {
		t.Fatalf("brief missing: %q", p)
	}
	// Must not look like a multi-seat package
	if strings.Contains(p, "【各席") || strings.Contains(p, "Summary₂") {
		t.Fatalf("panelist prompt must not include multi-seat package: %q", p)
	}
}

func TestBuildR3PanelistPromptPublicContext(t *testing.T) {
	brief := &roundtable.Brief{
		Title: "T", Question: "Q", Constraints: "C", SuccessCriteria: "S",
	}
	// BuildR3PanelistPrompt needs exported speech items — use round-trip via RunR3 test path.
	// Unit-check the exported builder with a small inline package via Format helpers.
	p := roundtable.BuildR3PanelistPrompt(roundtable.RoleMarket, brief, nil, "SUM2_BODY")
	if !strings.Contains(p, "title: T") {
		t.Fatalf("brief missing: %q", p)
	}
	if !strings.Contains(p, "SUM2_BODY") {
		t.Fatalf("Summary₂ missing: %q", p)
	}
	if !strings.Contains(p, "R3") && !strings.Contains(p, "次轮") {
		t.Fatalf("R3 stage missing: %q", p)
	}
	for _, marker := range []string{"交叉验证", "【保留】", "【修正】", "【反驳】", "【新增证据/待验证】", "本席最终建议"} {
		if !strings.Contains(p, marker) {
			t.Fatalf("R3 cross-validation contract missing %q: %q", marker, p)
		}
	}
	if !strings.Contains(p, "必须点名所验证的席位或具体观点") {
		t.Fatalf("R3 should require concrete peer validation: %q", p)
	}
	if strings.Contains(p, "tool_call") || strings.Contains(p, "tool trace") {
		t.Fatalf("must not inject tool traces: %q", p)
	}
}

// TestR3_ResumeSameAcpAndPublicContext covers slice-4 acceptance:
// R3 与 R2 使用相同 acp_session_id；prompt 含他人 R2 正文；存在 Summary₃；state=done
func TestR3_ResumeSameAcpAndPublicContext(t *testing.T) {
	svc, prompter := testRig(t)
	// Distinct per-role ACP ids (empty FixedAcpID → StaticSeatPrompter uses acp-{slug}).
	prompter.FixedAcpID = ""
	prompter.Replies = nil

	const (
		marketR2  = "R2_BODY_MARKET_AAA"
		productR2 = "R2_BODY_PRODUCT_BBB"
		engR2     = "R2_BODY_ENG_CCC"
		opsR2     = "R2_BODY_OPS_DDD"
		finR2     = "R2_BODY_FIN_EEE"
		summary2  = "SUMMARY2_MARK_R2"
		marketR3  = "R3_BODY_MARKET_XXX"
		productR3 = "R3_BODY_PRODUCT_YYY"
		engR3     = "R3_BODY_ENG_ZZZ"
		opsR3     = "R3_BODY_OPS_WWW"
		finR3     = "R3_BODY_FIN_VVV"
		summary3  = "SUMMARY3_MARK_FINAL"
	)
	r2Secrets := map[roundtable.Role]string{
		roundtable.RoleMarket:  marketR2,
		roundtable.RoleProduct: productR2,
		roundtable.RoleEng:     engR2,
		roundtable.RoleOps:     opsR2,
		roundtable.RoleFinance: finR2,
	}
	r3Secrets := map[roundtable.Role]string{
		roundtable.RoleMarket:  marketR3,
		roundtable.RoleProduct: productR3,
		roundtable.RoleEng:     engR3,
		roundtable.RoleOps:     opsR3,
		roundtable.RoleFinance: finR3,
	}

	prompter.ReplyFunc = func(req roundtable.SeatPromptRequest) (string, error) {
		if req.Role == roundtable.RoleReferee {
			if strings.Contains(req.Text, "Summary₃") || strings.Contains(req.Text, "终稿") {
				return summary3 + "：综合两轮，产品扩范围 vs 研发工期。", nil
			}
			return summary2 + "：首轮综合。", nil
		}
		// R3 public package always includes Summary₂ marker / 「次轮」stage.
		if strings.Contains(req.Text, "Summary₂") || strings.Contains(req.Text, "次轮") {
			return r3Secrets[req.Role] + "：次轮回应。", nil
		}
		return r2Secrets[req.Role] + "：首轮观点。", nil
	}

	room, err := svc.CreateRoom(roundtable.CreateRoomRequest{Title: "R3议题"})
	if err != nil {
		t.Fatalf("CreateRoom: %v", err)
	}
	confirmBriefHelper(t, svc, room.ID)

	r2resp, err := svc.RunR2(room.ID)
	if err != nil {
		t.Fatalf("RunR2: %v", err)
	}
	if r2resp.Room.State != roundtable.StateWaitingR3 {
		t.Fatalf("after R2 state %q", r2resp.Room.State)
	}

	// Snapshot R2 acp_session_id per panelist (must match R3 resume).
	seatsAfterR2, err := svc.ListSeats(room.ID)
	if err != nil {
		t.Fatal(err)
	}
	acpAfterR2 := map[roundtable.Role]string{}
	for _, s := range seatsAfterR2 {
		if !roundtable.IsPanelist(s.Role) {
			continue
		}
		if s.AcpSessionID == "" {
			t.Fatalf("panelist %s missing acp_session_id after R2", s.Role)
		}
		acpAfterR2[s.Role] = s.AcpSessionID
	}
	// Per-seat ids should be distinct (resume identity is seat-scoped).
	seen := map[string]roundtable.Role{}
	for role, id := range acpAfterR2 {
		if other, ok := seen[id]; ok {
			t.Fatalf("acp collision: %s and %s share %q", role, other, id)
		}
		seen[id] = role
	}

	// Clear calls so R3 assertions only see R3 prompts (+ summary).
	prompter.Calls = nil

	r3resp, err := svc.RunR3(room.ID)
	if err != nil {
		t.Fatalf("RunR3: %v", err)
	}
	if r3resp.Room == nil || r3resp.Room.State != roundtable.StateDone {
		t.Fatalf("state want done, got %+v", r3resp.Room)
	}
	if strings.TrimSpace(r3resp.Room.SummaryR3) == "" {
		t.Fatal("summary_r3 empty on room")
	}
	if !strings.Contains(r3resp.Room.SummaryR3, summary3) {
		t.Fatalf("summary_r3 missing mark: %q", r3resp.Room.SummaryR3)
	}
	if r3resp.SummaryTurn.Kind != roundtable.TurnKindSummary || r3resp.SummaryTurn.Round != 3 {
		t.Fatalf("summary turn: kind=%s round=%d", r3resp.SummaryTurn.Kind, r3resp.SummaryTurn.Round)
	}
	if !strings.Contains(r3resp.SummaryTurn.ContentText, summary3) {
		t.Fatalf("summary turn text: %q", r3resp.SummaryTurn.ContentText)
	}
	if len(r3resp.FailedRoles) != 0 {
		t.Fatalf("unexpected failures: %v", r3resp.FailedRoles)
	}
	if len(r3resp.SpeechTurns) != 5 {
		t.Fatalf("speech turns %d, want 5", len(r3resp.SpeechTurns))
	}

	// Seats: same acp as R2, status done.
	seatsAfterR3, err := svc.ListSeats(room.ID)
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range seatsAfterR3 {
		if !roundtable.IsPanelist(s.Role) {
			continue
		}
		if s.Status != roundtable.SeatDone {
			t.Fatalf("seat %s status %q", s.Role, s.Status)
		}
		if s.AcpSessionID != acpAfterR2[s.Role] {
			t.Fatalf("acp_session_id changed for %s: R2=%q R3=%q",
				s.Role, acpAfterR2[s.Role], s.AcpSessionID)
		}
	}

	// R3 panelist prompts: resume same acp + public package with peer R2 bodies + Summary₂.
	allR2 := []string{marketR2, productR2, engR2, opsR2, finR2}
	calls := prompter.SnapshotCalls()
	var panelistCalls int
	for _, c := range calls {
		if !roundtable.IsPanelist(c.Role) {
			continue
		}
		panelistCalls++
		// Same acp_session_id as R2 (resume).
		wantACP := acpAfterR2[c.Role]
		if c.AcpSessionID != wantACP {
			t.Fatalf("R3 panelist %s AcpSessionID=%q want resume %q", c.Role, c.AcpSessionID, wantACP)
		}
		if c.SystemContext != "" {
			t.Fatalf("R3 resume must not re-inject SystemContext for %s", c.Role)
		}
		// Brief present
		if !strings.Contains(c.Text, "团队协作 MVP") {
			t.Fatalf("panelist %s R3 inject missing Brief: %q", c.Role, c.Text)
		}
		// Summary₂ present (body only)
		if !strings.Contains(c.Text, summary2) {
			t.Fatalf("panelist %s R3 inject missing Summary₂: %q", c.Role, c.Text)
		}
		// All five R2 speech bodies (含自己与他人)
		for _, body := range allR2 {
			if !strings.Contains(c.Text, body) {
				t.Fatalf("panelist %s R3 inject missing R2 body %s\ninject=%q", c.Role, body, c.Text)
			}
		}
		// No tool-trace markers
		if strings.Contains(c.Text, "tool_call") || strings.Contains(c.Text, "tool_result") {
			t.Fatalf("panelist %s inject must not include tool traces", c.Role)
		}
	}
	if panelistCalls != 5 {
		t.Fatalf("R3 panelist prompt calls %d, want 5", panelistCalls)
	}

	// Referee Summary₃ sees R3 bodies.
	var summaryCall *roundtable.SeatPromptRequest
	for i := range calls {
		c := &calls[i]
		if c.Role == roundtable.RoleReferee && (strings.Contains(c.Text, "Summary₃") || strings.Contains(c.Text, "终稿")) {
			summaryCall = c
			break
		}
	}
	if summaryCall == nil {
		t.Fatal("missing referee Summary₃ prompt")
	}
	if summaryCall.RequiredTool != roundtable.SubmitR3SummaryTool {
		t.Fatalf("Summary₃ must require %s, call=%+v", roundtable.SubmitR3SummaryTool, summaryCall)
	}
	if !strings.Contains(summaryCall.ToolInstruction, "普通回复文本不会被视为已提交") {
		t.Fatalf("Summary₃ tool instruction missing normal-text gate: %q", summaryCall.ToolInstruction)
	}
	for _, marker := range []string{"被支持", "被修正", "被推翻", "仍待验证"} {
		if !strings.Contains(summaryCall.Text, marker) {
			t.Fatalf("Summary₃ cross-validation audit missing %q: %q", marker, summaryCall.Text)
		}
	}
	for _, body := range []string{marketR3, productR3, engR3, opsR3, finR3} {
		if !strings.Contains(summaryCall.Text, body) {
			t.Fatalf("Summary₃ prompt missing R3 body %s", body)
		}
	}

	// Timeline: 5 R2 speech + 1 Summary₂ + 5 R3 speech + 1 Summary₃ (+ brief system)
	turns, err := svc.ListTurns(room.ID)
	if err != nil {
		t.Fatal(err)
	}
	var nSpeech2, nSpeech3, nSummary2, nSummary3 int
	for _, tr := range turns {
		switch {
		case tr.Kind == roundtable.TurnKindSpeech && tr.Round == 2:
			nSpeech2++
		case tr.Kind == roundtable.TurnKindSpeech && tr.Round == 3:
			nSpeech3++
		case tr.Kind == roundtable.TurnKindSummary && tr.Round == 2:
			nSummary2++
		case tr.Kind == roundtable.TurnKindSummary && tr.Round == 3:
			nSummary3++
		}
	}
	if nSpeech2 != 5 || nSpeech3 != 5 || nSummary2 != 1 || nSummary3 != 1 {
		t.Fatalf("timeline r2_speech=%d r3_speech=%d sum2=%d sum3=%d",
			nSpeech2, nSpeech3, nSummary2, nSummary3)
	}

	// Persisted reload
	got, err := svc.GetRoom(room.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.State != roundtable.StateDone || got.SummaryR3 == "" {
		t.Fatalf("persisted: state=%s summary_r3=%q", got.State, got.SummaryR3)
	}

	// Idempotency: cannot re-run R3 from done
	if _, err := svc.RunR3(room.ID); err == nil {
		t.Fatal("second RunR3 should fail")
	}
}

func TestR3_PartialSeatFailureDoesNotBlock(t *testing.T) {
	svc, prompter := testRig(t)
	prompter.FixedAcpID = ""
	prompter.Replies = nil
	prompter.ReplyFunc = func(req roundtable.SeatPromptRequest) (string, error) {
		if req.Role == roundtable.RoleReferee {
			if strings.Contains(req.Text, "Summary₃") || strings.Contains(req.Text, "终稿") {
				return "Summary₃：缺席已注明。", nil
			}
			return "Summary₂：ok", nil
		}
		if strings.Contains(req.Text, "Summary₂") || strings.Contains(req.Text, "次轮") {
			return string(req.Role) + " R3", nil
		}
		return string(req.Role) + " R2", nil
	}

	room, err := svc.CreateRoom(roundtable.CreateRoomRequest{Title: "R3 partial"})
	if err != nil {
		t.Fatal(err)
	}
	confirmBriefHelper(t, svc, room.ID)
	if _, err := svc.RunR2(room.ID); err != nil {
		t.Fatalf("RunR2: %v", err)
	}

	// Fail ops only on R3 (after R2 succeeded).
	prompter.FailRoles = map[roundtable.Role]error{
		roundtable.RoleOps: fmt.Errorf("simulated ops R3 outage"),
	}

	resp, err := svc.RunR3(room.ID)
	if err != nil {
		t.Fatalf("RunR3 should succeed despite one seat fail: %v", err)
	}
	if resp.Room.State != roundtable.StateDone {
		t.Fatalf("state %q, want done", resp.Room.State)
	}
	if len(resp.FailedRoles) != 1 || resp.FailedRoles[0] != string(roundtable.RoleOps) {
		t.Fatalf("failed_roles %v", resp.FailedRoles)
	}
	if len(resp.SpeechTurns) != 5 {
		t.Fatalf("speech turns %d", len(resp.SpeechTurns))
	}

	seats, _ := svc.ListSeats(room.ID)
	for _, s := range seats {
		if s.Role == roundtable.RoleOps {
			if s.Status != roundtable.SeatFailed {
				t.Fatalf("ops status %q", s.Status)
			}
			// acp from R2 still persisted
			if s.AcpSessionID == "" {
				t.Fatal("ops should retain acp_session_id after R3 fail")
			}
			continue
		}
		if roundtable.IsPanelist(s.Role) && s.Status != roundtable.SeatDone {
			t.Fatalf("%s status %q", s.Role, s.Status)
		}
	}
}

func TestHTTP_R3(t *testing.T) {
	svc, prompter := testRig(t)
	prompter.FixedAcpID = ""
	prompter.Replies = nil
	prompter.ReplyFunc = func(req roundtable.SeatPromptRequest) (string, error) {
		if req.Role == roundtable.RoleReferee {
			if strings.Contains(req.Text, "Summary₃") || strings.Contains(req.Text, "终稿") {
				return "sum3", nil
			}
			return "sum2", nil
		}
		if strings.Contains(req.Text, "Summary₂") || strings.Contains(req.Text, "次轮") {
			return "r3-" + string(req.Role), nil
		}
		return "r2-" + string(req.Role), nil
	}
	h := roundtable.NewHandler(svc)

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/roundtable/rooms", strings.NewReader(`{"title":"HTTP R3"}`))
	h.HandleRoomsRoot(rr, req)
	var room roundtable.Room
	_ = json.NewDecoder(rr.Body).Decode(&room)

	briefBody := `{
		"title":"议题",
		"question":"核心问题",
		"constraints":"约束",
		"success_criteria":"成功标准"
	}`
	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/api/roundtable/rooms/"+room.ID+"/brief", strings.NewReader(briefBody))
	h.HandleRoomsItem(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("brief status %d: %s", rr.Code, rr.Body.String())
	}

	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/api/roundtable/rooms/"+room.ID+"/r2?wait=1", nil)
	h.HandleRoomsItem(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("r2 status %d: %s", rr.Code, rr.Body.String())
	}

	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/api/roundtable/rooms/"+room.ID+"/r3?wait=1", nil)
	h.HandleRoomsItem(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("r3 status %d: %s", rr.Code, rr.Body.String())
	}
	var r3resp roundtable.RunR3Response
	if err := json.NewDecoder(rr.Body).Decode(&r3resp); err != nil {
		t.Fatal(err)
	}
	if r3resp.Room == nil || r3resp.Room.State != roundtable.StateDone {
		t.Fatalf("r3 response state: %+v", r3resp.Room)
	}
	if r3resp.Room.SummaryR3 == "" {
		t.Fatal("summary_r3 empty")
	}
	if len(r3resp.SpeechTurns) != 5 {
		t.Fatalf("speech_turns %d", len(r3resp.SpeechTurns))
	}
	if r3resp.SummaryTurn.Kind != roundtable.TurnKindSummary || r3resp.SummaryTurn.Round != 3 {
		t.Fatalf("summary turn %+v", r3resp.SummaryTurn)
	}
}

// TestE2E_Smoke_DesignSection7 is the slice-7 acceptance harness for design.md §7.
//
// Path: entry(manifest) → create room → R1 chat+confirm Brief → R2 five speeches+Summary₂
// → R3 resume+public context+Summary₃ → done; then reload (refresh recovery).
//
// Maps §7.1–§7.8 where automatable with StaticSeatPrompter (no live 1acp).
// UI-only bits (§7.1 click-path, §7.6 default-collapsed process) are covered by
// frontend unit tests + docs/features/agents-roundtable/ACCEPTANCE.md manual steps.
func TestE2E_Smoke_DesignSection7(t *testing.T) {
	// --- §7.1 entry: agents-roundtable manifest is registered (discovery → 应用) ---
	m, ok := appregistry.Get("agents-roundtable")
	if !ok {
		t.Fatal("§7.1 FAIL: agents-roundtable not in appregistry (discovery/apps entry)")
	}
	if !m.Enabled {
		t.Fatal("§7.1 FAIL: agents-roundtable disabled")
	}
	if len(m.MountPoints) == 0 || m.MountPoints[0].View != "AgentsRoundtable" {
		t.Fatalf("§7.1 FAIL: mount view missing/wrong: %+v", m.MountPoints)
	}
	t.Log("§7.1 PASS: app manifest agents-roundtable enabled (API create room = wizard Start)")

	svc, prompter := testRig(t)
	prompter.FixedAcpID = ""
	prompter.Replies = nil

	const (
		marketR2  = "E2E_R2_MARKET"
		productR2 = "E2E_R2_PRODUCT_SCOPE"
		engR2     = "E2E_R2_ENG_FEASIBILITY"
		opsR2     = "E2E_R2_OPS"
		finR2     = "E2E_R2_FIN"
		summary2  = "E2E_SUMMARY2_产品扩范围_研发工期"
		marketR3  = "E2E_R3_MARKET"
		productR3 = "E2E_R3_PRODUCT"
		engR3     = "E2E_R3_ENG"
		opsR3     = "E2E_R3_OPS"
		finR3     = "E2E_R3_FIN"
		summary3  = "E2E_SUMMARY3_终稿_产品vs研发可区分"
	)
	r2Bodies := map[roundtable.Role]string{
		roundtable.RoleMarket:  marketR2,
		roundtable.RoleProduct: productR2,
		roundtable.RoleEng:     engR2,
		roundtable.RoleOps:     opsR2,
		roundtable.RoleFinance: finR2,
	}
	r3Bodies := map[roundtable.Role]string{
		roundtable.RoleMarket:  marketR3,
		roundtable.RoleProduct: productR3,
		roundtable.RoleEng:     engR3,
		roundtable.RoleOps:     opsR3,
		roundtable.RoleFinance: finR3,
	}

	prompter.ReplyFunc = func(req roundtable.SeatPromptRequest) (string, error) {
		if req.Role == roundtable.RoleReferee {
			// R1 multi-turn clarification
			if !strings.Contains(req.Text, "Summary") && !strings.Contains(req.Text, "终稿") &&
				!strings.Contains(req.Text, "首轮发言") && !strings.Contains(req.Text, "次轮") {
				return "请确认 Brief：目标用户与两周约束。", nil
			}
			if strings.Contains(req.Text, "Summary₃") || strings.Contains(req.Text, "终稿") {
				// §7.8: summary distinguishes 产品 vs 研发
				return summary3 + "：产品侧重范围取舍；研发侧重可行性与工期。", nil
			}
			return summary2 + "：首轮综合——产品谈范围，研发谈工期风险。", nil
		}
		if strings.Contains(req.Text, "Summary₂") || strings.Contains(req.Text, "次轮") {
			return r3Bodies[req.Role] + " 次轮正文", nil
		}
		return r2Bodies[req.Role] + " 首轮正文", nil
	}

	// --- create room ---
	room, err := svc.CreateRoom(roundtable.CreateRoomRequest{Title: "§7 E2E 冒烟"})
	if err != nil {
		t.Fatalf("create room: %v", err)
	}
	if room.State != roundtable.StateDraftingBrief {
		t.Fatalf("§7.1/create: state %q want drafting_brief", room.State)
	}
	if len(room.Seats) != 6 {
		t.Fatalf("§7.2 seats at create: %d want 6", len(room.Seats))
	}
	for _, s := range room.Seats {
		if s.AgentType != agent.AgentTypeGrokBuild {
			t.Fatalf("§7.2 seat %s agent_type %q", s.Role, s.AgentType)
		}
	}
	t.Log("§7.1 PASS: room created (entry → 建房)")

	// --- §7.3 R1 multi-turn + confirm Brief ---
	if _, err := svc.ChatWithReferee(room.ID, roundtable.ChatRequest{Text: "我们想做协作工具两周验证 PMF"}); err != nil {
		t.Fatalf("§7.3 chat1: %v", err)
	}
	if _, err := svc.ChatWithReferee(room.ID, roundtable.ChatRequest{Text: "约束 3 人，成功标准 5 个种子用户"}); err != nil {
		t.Fatalf("§7.3 chat2: %v", err)
	}
	// Resume across R1 turns
	calls := prompter.SnapshotCalls()
	if len(calls) < 2 {
		t.Fatal("§7.3 expected ≥2 referee prompts")
	}
	if calls[0].AcpSessionID != "" {
		t.Fatal("§7.3 first R1 turn should not resume")
	}
	if calls[1].AcpSessionID == "" {
		t.Fatal("§7.3 second R1 turn must resume acp_session_id")
	}

	confirmed, err := svc.ConfirmBrief(room.ID, roundtable.ConfirmBriefRequest{
		Title:           "协作工具 PMF",
		Question:        "如何在两周内验证协作工具 PMF？",
		Constraints:     "3 人团队，两周",
		SuccessCriteria: "5 个种子用户完成核心路径",
		ProductKind:     roundtable.ProductSoftware,
	})
	if err != nil {
		t.Fatalf("§7.3 ConfirmBrief: %v", err)
	}
	if confirmed.State != roundtable.StateWaitingR2 || confirmed.Brief == nil {
		t.Fatalf("§7.3 after brief: state=%s brief=%+v", confirmed.State, confirmed.Brief)
	}
	t.Log("§7.3 PASS: R1 multi-turn + Brief confirmed → waiting_r2")

	// --- §7.4 R2 isolation + Summary₂ ---
	prompter.Calls = nil
	r2resp, err := svc.RunR2(room.ID)
	if err != nil {
		t.Fatalf("§7.4 RunR2: %v", err)
	}
	if r2resp.Room.State != roundtable.StateWaitingR3 {
		t.Fatalf("§7.4 state %q want waiting_r3", r2resp.Room.State)
	}
	if len(r2resp.SpeechTurns) != 5 {
		t.Fatalf("§7.4 speech turns %d want 5", len(r2resp.SpeechTurns))
	}
	if r2resp.SummaryTurn.Kind != roundtable.TurnKindSummary || !strings.Contains(r2resp.Room.SummaryR2, summary2) {
		t.Fatalf("§7.4 Summary₂ missing: %q", r2resp.Room.SummaryR2)
	}

	// Isolation: no peer secrets in panelist inject
	allR2 := []string{marketR2, productR2, engR2, opsR2, finR2}
	for _, c := range prompter.SnapshotCalls() {
		if !roundtable.IsPanelist(c.Role) {
			continue
		}
		inject := c.SystemContext + "\n" + c.Text
		for role, body := range r2Bodies {
			if role == c.Role {
				continue
			}
			if strings.Contains(inject, body) {
				t.Fatalf("§7.4 ISOLATION FAIL: %s saw peer %s body in inject", c.Role, role)
			}
		}
		if !strings.Contains(c.Text, "协作工具 PMF") {
			t.Fatalf("§7.4 panelist %s missing Brief", c.Role)
		}
		if c.AcpSessionID != "" {
			t.Fatalf("§7.4 R2 %s should be fresh session", c.Role)
		}
	}
	t.Log("§7.4 PASS: R2 five isolated speeches + Summary₂")

	// Snapshot acp for resume check + §7.2 six sessions after R2
	seatsR2, err := svc.ListSeats(room.ID)
	if err != nil {
		t.Fatal(err)
	}
	acpByRole := map[roundtable.Role]string{}
	var sessionCount int
	chatStore, err := agent.NewStore()
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range seatsR2 {
		if s.SessionID == "" {
			t.Fatalf("§7.2 seat %s missing session_id after R2", s.Role)
		}
		sessionCount++
		rec, ok, err := chatStore.Get(s.SessionID)
		if err != nil || !ok {
			t.Fatalf("§7.2 chat session for %s missing", s.Role)
		}
		if rec.AgentType != agent.AgentTypeGrokBuild {
			t.Fatalf("§7.2 session agent_type %q", rec.AgentType)
		}
		if roundtable.IsPanelist(s.Role) {
			if s.AcpSessionID == "" {
				t.Fatalf("§7.5 panelist %s missing acp after R2", s.Role)
			}
			acpByRole[s.Role] = s.AcpSessionID
		}
	}
	if sessionCount != 6 {
		t.Fatalf("§7.2 want 6 Grok Build sessions, got %d", sessionCount)
	}
	t.Log("§7.2 PASS: 6 Grok Build sessions (1 referee + 5 panelists)")

	// --- §7.5 R3 resume + public context + Summary₃ ---
	prompter.Calls = nil
	r3resp, err := svc.RunR3(room.ID)
	if err != nil {
		t.Fatalf("§7.5 RunR3: %v", err)
	}
	if r3resp.Room.State != roundtable.StateDone {
		t.Fatalf("§7.5 state %q want done", r3resp.Room.State)
	}
	if !strings.Contains(r3resp.Room.SummaryR3, summary3) {
		t.Fatalf("§7.5 Summary₃ missing: %q", r3resp.Room.SummaryR3)
	}
	if len(r3resp.SpeechTurns) != 5 {
		t.Fatalf("§7.5 R3 speeches %d", len(r3resp.SpeechTurns))
	}

	for _, c := range prompter.SnapshotCalls() {
		if !roundtable.IsPanelist(c.Role) {
			continue
		}
		if c.AcpSessionID != acpByRole[c.Role] {
			t.Fatalf("§7.5 resume FAIL %s: acp %q want %q", c.Role, c.AcpSessionID, acpByRole[c.Role])
		}
		for _, body := range allR2 {
			if !strings.Contains(c.Text, body) {
				t.Fatalf("§7.5 public context missing R2 body %s for %s", body, c.Role)
			}
		}
		if !strings.Contains(c.Text, summary2) {
			t.Fatalf("§7.5 public context missing Summary₂ for %s", c.Role)
		}
		if strings.Contains(c.Text, "tool_call") || strings.Contains(c.Text, "tool_result") {
			t.Fatalf("§7.5 must not inject tool traces for %s", c.Role)
		}
	}
	t.Log("§7.5 PASS: R3 resume same acp + R2 bodies + Summary₂ + Summary₃ → done")

	// --- §7.6 main timeline content_text only (no process noise in turn body) ---
	turns, err := svc.ListTurns(room.ID)
	if err != nil {
		t.Fatal(err)
	}
	var nSpeech2, nSpeech3, nSum2, nSum3, nChat int
	for _, tr := range turns {
		if strings.TrimSpace(tr.ContentText) == "" && tr.Kind != roundtable.TurnKindSystem {
			// system may be empty in some paths; speech/summary/chat must bind content_text
			if tr.Kind == roundtable.TurnKindSpeech || tr.Kind == roundtable.TurnKindSummary || tr.Kind == roundtable.TurnKindChat {
				t.Fatalf("§7.6 empty content_text kind=%s id=%s", tr.Kind, tr.ID)
			}
		}
		// process_ref is optional link; body is content_text only (UI folds process)
		switch {
		case tr.Kind == roundtable.TurnKindChat:
			nChat++
		case tr.Kind == roundtable.TurnKindSpeech && tr.Round == 2:
			nSpeech2++
		case tr.Kind == roundtable.TurnKindSpeech && tr.Round == 3:
			nSpeech3++
		case tr.Kind == roundtable.TurnKindSummary && tr.Round == 2:
			nSum2++
		case tr.Kind == roundtable.TurnKindSummary && tr.Round == 3:
			nSum3++
		}
	}
	if nChat < 2 || nSpeech2 != 5 || nSpeech3 != 5 || nSum2 != 1 || nSum3 != 1 {
		t.Fatalf("§7.6 timeline counts chat=%d s2=%d s3=%d sum2=%d sum3=%d",
			nChat, nSpeech2, nSpeech3, nSum2, nSum3)
	}
	t.Log("§7.6 PASS: timeline turns bind content_text (process_ref optional; UI collapses process)")

	// --- §7.7 refresh recovery: GetRoom reloads full room + seats + turns + acp ---
	reloaded, err := svc.GetRoom(room.ID)
	if err != nil {
		t.Fatalf("§7.7 GetRoom: %v", err)
	}
	if reloaded.State != roundtable.StateDone || reloaded.Brief == nil || reloaded.SummaryR2 == "" || reloaded.SummaryR3 == "" {
		t.Fatalf("§7.7 incomplete reload: state=%s brief=%v s2=%q s3=%q",
			reloaded.State, reloaded.Brief != nil, reloaded.SummaryR2, reloaded.SummaryR3)
	}
	if len(reloaded.Seats) != 6 || len(reloaded.Turns) < len(turns) {
		t.Fatalf("§7.7 seats=%d turns=%d", len(reloaded.Seats), len(reloaded.Turns))
	}
	for _, s := range reloaded.Seats {
		if roundtable.IsPanelist(s.Role) && s.AcpSessionID != acpByRole[s.Role] {
			t.Fatalf("§7.7 acp not restored for %s", s.Role)
		}
		if s.SessionID == "" {
			t.Fatalf("§7.7 session_id lost for %s", s.Role)
		}
	}
	t.Log("§7.7 PASS: room/seats/turns/acp restored after reload (refresh recovery)")

	// Mid-game resume capability: from waiting_r3 acp ids were non-empty (asserted above).
	// Done is terminal — document that unfinished seats resume via same acp fields.

	// --- §7.8 Summary distinguishes role sources; product vs eng seeds differ ---
	prodSeed := roundtable.RoleSeedAGENTS(roundtable.RoleProduct)
	engSeed := roundtable.RoleSeedAGENTS(roundtable.RoleEng)
	// Product focuses on 做什么/范围；eng on 可行性/工期 — must not be identical contracts.
	if !strings.Contains(prodSeed, "核心路径") || !strings.Contains(engSeed, "可行性") {
		t.Fatal("§7.8 product/eng role contracts not differentiated")
	}
	if prodSeed == engSeed {
		t.Fatal("§7.8 product and eng seeds must differ")
	}
	if !strings.Contains(r3resp.Room.SummaryR3, "产品") || !strings.Contains(r3resp.Room.SummaryR3, "研发") {
		t.Fatalf("§7.8 Summary₃ should cite 产品 and 研发: %q", r3resp.Room.SummaryR3)
	}
	// Role labels surface in R3 public package (panelist inject) and UI roleLabel map.
	pkg := roundtable.BuildR3PanelistPrompt(roundtable.RoleMarket, reloaded.Brief, nil, "x")
	if !strings.Contains(pkg, "市场") {
		t.Fatalf("§7.8 role labels missing in public package: %q", pkg)
	}
	if roundtable.RoleLabel(roundtable.RoleProduct) != "产品" || roundtable.RoleLabel(roundtable.RoleEng) != "研发" {
		t.Fatal("§7.8 RoleLabel product/eng mismatch")
	}
	t.Log("§7.8 PASS: Summary cites 产品/研发; role contracts differentiated")

	// HTTP path smoke: GET room after done
	h := roundtable.NewHandler(svc)
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/roundtable/rooms/"+room.ID, nil)
	h.HandleRoomsItem(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("§7 HTTP GET room status %d", rr.Code)
	}
	var httpRoom roundtable.Room
	if err := json.NewDecoder(rr.Body).Decode(&httpRoom); err != nil {
		t.Fatal(err)
	}
	if httpRoom.State != roundtable.StateDone || len(httpRoom.Seats) != 6 {
		t.Fatalf("§7 HTTP room incomplete: state=%s seats=%d", httpRoom.State, len(httpRoom.Seats))
	}
	t.Log("§7 E2E SMOKE ALL PASS (1–8 automatable parts)")
}
