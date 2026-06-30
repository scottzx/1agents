package agent

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/scottzx/1Agents/backend/internal/workspace"
)

// discussionRig wires a handler whose task store, session store, and workspace
// registry all share one ONEAGENTS_HOME, and registers one workspace so the
// create handler's resolveWorkspacePath succeeds. Returns the handler, the
// workspace id, and its on-disk path.
func discussionRig(t *testing.T) (*Handler, string, string) {
	t.Helper()
	t.Setenv("ONEAGENTS_HOME", t.TempDir())

	wsPath := t.TempDir()
	wsID := "ws-disc"
	wh := workspace.NewHandler()
	if err := wh.SaveWorkspacesConfig(&workspace.WorkspacesConfig{
		Workspaces: []workspace.Workspace{{ID: wsID, Name: "Disc Proj", Path: wsPath, Status: "active"}},
	}); err != nil {
		t.Fatalf("SaveWorkspacesConfig: %v", err)
	}

	store, err := NewStore()
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	tasksStore, err := NewTasksStore()
	if err != nil {
		t.Fatalf("NewTasksStore: %v", err)
	}
	scheduler := NewScheduler(tasksStore, func() ([]WorkspaceRef, error) { return nil, nil })
	h := NewHandler(store, tasksStore, NewAcpxClient(38082), scheduler, NewCatalogStore(), "http://127.0.0.1:0")
	return h, wsID, wsPath
}

func postJSON(t *testing.T, h http.HandlerFunc, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
	h(rr, req)
	return rr
}

// 无卡 path: a boss-initiated idea thread with no card attached.
func TestDiscussionCreateCardless(t *testing.T) {
	h, wsID, _ := discussionRig(t)

	rr := postJSON(t, h.HandleDiscussionsRoot, "/api/agent/discussions",
		`{"workspace_id":"`+wsID+`","title":"要不要做插件市场","description":"边聊边细化"}`)
	if rr.Code != http.StatusOK {
		t.Fatalf("create status %d: %s", rr.Code, rr.Body.String())
	}
	var disc Task
	if err := json.NewDecoder(rr.Body).Decode(&disc); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if disc.Type != TaskTypeDiscussion {
		t.Fatalf("type %q, want discussion", disc.Type)
	}
	if disc.IssueState != IssueOpen || disc.ID == "" {
		t.Fatalf("unexpected discussion: %+v", disc)
	}
	if len(disc.Links) != 0 {
		t.Fatalf("cardless discussion should have no links, got %+v", disc.Links)
	}
}

// 有卡 path (task-row card): create a discussion from an existing agent-suggested
// card (#47), wiring a bidirectional relates link.
func TestDiscussionCreateWithSuggestionCard(t *testing.T) {
	h, wsID, wsPath := discussionRig(t)

	// Seed an agent-suggested card (#47).
	suggestion := Task{
		ID: "sugg-1", Title: "顺手发现的死代码", Type: TaskTypeRequirement,
		Source: TaskSourceAgent, Status: TaskStatusPending, IssueState: IssueOpen,
	}
	saveTasks(t, h.tasksStore, wsPath, []Task{suggestion})

	rr := postJSON(t, h.HandleDiscussionsRoot, "/api/agent/discussions",
		`{"workspace_id":"`+wsID+`","title":"讨论这条建议","sourceTaskId":"sugg-1"}`)
	if rr.Code != http.StatusOK {
		t.Fatalf("create status %d: %s", rr.Code, rr.Body.String())
	}
	var disc Task
	if err := json.NewDecoder(rr.Body).Decode(&disc); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !hasLink(disc.Links, "sugg-1", LinkRelates) {
		t.Fatalf("discussion missing card link: %+v", disc.Links)
	}
	// The card carries the reverse backlink.
	card, ok, _ := h.tasksStore.GetTask("sugg-1")
	if !ok || !hasLink(card.Links, disc.ID, LinkRelates) {
		t.Fatalf("card missing reverse backlink: %+v", card.Links)
	}
}

// 有卡 path (external inbox_item ref): create a discussion from an inbox item
// that has no task row yet (Inbox #60 unmerged) — recorded as a label.
func TestDiscussionCreateWithInboxRef(t *testing.T) {
	h, wsID, _ := discussionRig(t)

	rr := postJSON(t, h.HandleDiscussionsRoot, "/api/agent/discussions",
		`{"workspace_id":"`+wsID+`","title":"来自邮件的线索","sourceRef":"inbox-42"}`)
	if rr.Code != http.StatusOK {
		t.Fatalf("create status %d: %s", rr.Code, rr.Body.String())
	}
	var disc Task
	if err := json.NewDecoder(rr.Body).Decode(&disc); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !hasLabel(disc.Labels, discussionCardLabel+"inbox-42") {
		t.Fatalf("discussion missing inbox card label: %+v", disc.Labels)
	}
}

// Attach an agent-suggested card (#47) onto an existing discussion.
func TestDiscussionAttachCard(t *testing.T) {
	h, _, wsPath := discussionRig(t)

	disc := Task{ID: "disc-1", Title: "线程", Type: TaskTypeDiscussion, Status: TaskStatusPending, IssueState: IssueOpen}
	card := Task{ID: "card-1", Title: "建议", Type: TaskTypeRequirement, Source: TaskSourceAgent, Status: TaskStatusPending, IssueState: IssueOpen}
	saveTasks(t, h.tasksStore, wsPath, []Task{disc, card})

	rr := postJSON(t, h.HandleDiscussionItem, "/api/agent/discussions/disc-1/cards",
		`{"sourceTaskId":"card-1"}`)
	if rr.Code != http.StatusOK {
		t.Fatalf("attach status %d: %s", rr.Code, rr.Body.String())
	}
	var got Task
	if err := json.NewDecoder(rr.Body).Decode(&got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !hasLink(got.Links, "card-1", LinkRelates) {
		t.Fatalf("discussion missing card link after attach: %+v", got.Links)
	}

	// Attaching a non-discussion task is rejected.
	rr = postJSON(t, h.HandleDiscussionItem, "/api/agent/discussions/card-1/cards",
		`{"sourceTaskId":"disc-1"}`)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("attach to non-discussion status %d, want 400", rr.Code)
	}

	// Attaching an unknown source 404s.
	rr = postJSON(t, h.HandleDiscussionItem, "/api/agent/discussions/disc-1/cards",
		`{"sourceTaskId":"nope"}`)
	if rr.Code != http.StatusNotFound {
		t.Fatalf("attach unknown source status %d, want 404", rr.Code)
	}
}

// 拍板: conclude a discussion into a requirement with a backlink, closing the
// discussion.
func TestDiscussionConcludeToRequirement(t *testing.T) {
	h, _, wsPath := discussionRig(t)

	disc := Task{ID: "disc-2", Title: "讨论", Type: TaskTypeDiscussion, Status: TaskStatusPending, IssueState: IssueOpen}
	saveTasks(t, h.tasksStore, wsPath, []Task{disc})

	rr := postJSON(t, h.HandleDiscussionItem, "/api/agent/discussions/disc-2/conclude",
		`{"title":"做插件市场 v1","description":"结论","type":"requirement","sourceRef":"inbox-7","close":true}`)
	if rr.Code != http.StatusOK {
		t.Fatalf("conclude status %d: %s", rr.Code, rr.Body.String())
	}
	var spawned Task
	if err := json.NewDecoder(rr.Body).Decode(&spawned); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if spawned.Type != TaskTypeRequirement {
		t.Fatalf("spawned type %q, want requirement", spawned.Type)
	}
	if !hasLink(spawned.Links, "disc-2", LinkRelates) {
		t.Fatalf("spawned missing backlink to discussion: %+v", spawned.Links)
	}
	if !hasLabel(spawned.Labels, concludedFromLabel+"inbox-7") {
		t.Fatalf("spawned missing concluded-from label: %+v", spawned.Labels)
	}
	// The discussion is closed and relates to its outcome.
	got, ok, _ := h.tasksStore.GetTask("disc-2")
	if !ok || got.IssueState != IssueClosed {
		t.Fatalf("discussion not closed: %+v", got)
	}
	if !hasLink(got.Links, spawned.ID, LinkRelates) {
		t.Fatalf("discussion missing outcome link: %+v", got.Links)
	}
}

// 拍板 → temp Task (个人/健康域 降级为临时 Task), discussion stays open.
func TestDiscussionConcludeToTempTask(t *testing.T) {
	h, _, wsPath := discussionRig(t)

	disc := Task{ID: "disc-3", Title: "讨论", Type: TaskTypeDiscussion, Status: TaskStatusPending, IssueState: IssueOpen}
	saveTasks(t, h.tasksStore, wsPath, []Task{disc})

	rr := postJSON(t, h.HandleDiscussionItem, "/api/agent/discussions/disc-3/conclude",
		`{"title":"预约体检","type":"task","assignee":"user"}`)
	if rr.Code != http.StatusOK {
		t.Fatalf("conclude status %d: %s", rr.Code, rr.Body.String())
	}
	var spawned Task
	if err := json.NewDecoder(rr.Body).Decode(&spawned); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if spawned.Type != TaskTypeTask || spawned.Assignee != AssigneeUser {
		t.Fatalf("unexpected temp task: %+v", spawned)
	}
	got, _, _ := h.tasksStore.GetTask("disc-3")
	if got.IssueState != IssueOpen {
		t.Fatalf("discussion should stay open without close=true: %+v", got)
	}
}

// Concluding a non-discussion task is rejected; bad type is rejected.
func TestDiscussionConcludeValidation(t *testing.T) {
	h, _, wsPath := discussionRig(t)

	disc := Task{ID: "disc-4", Title: "讨论", Type: TaskTypeDiscussion, Status: TaskStatusPending, IssueState: IssueOpen}
	normal := Task{ID: "norm-1", Title: "普通任务", Type: TaskTypeTask, Status: TaskStatusPending, IssueState: IssueOpen}
	saveTasks(t, h.tasksStore, wsPath, []Task{disc, normal})

	rr := postJSON(t, h.HandleDiscussionItem, "/api/agent/discussions/norm-1/conclude", `{"title":"x"}`)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("conclude non-discussion status %d, want 400", rr.Code)
	}
	rr = postJSON(t, h.HandleDiscussionItem, "/api/agent/discussions/disc-4/conclude",
		`{"title":"x","type":"discussion"}`)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("conclude bad type status %d, want 400", rr.Code)
	}
	rr = postJSON(t, h.HandleDiscussionItem, "/api/agent/discussions/disc-4/conclude", `{"title":"  "}`)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("conclude empty title status %d, want 400", rr.Code)
	}
}

func hasLink(links []TaskLink, target string, rel LinkRel) bool {
	for _, l := range links {
		if l.Target == target && l.Rel == rel {
			return true
		}
	}
	return false
}

func hasLabel(labels []string, label string) bool {
	for _, l := range labels {
		if l == label {
			return true
		}
	}
	return false
}
