package crm

import (
	"encoding/json"
	"path/filepath"
	"testing"
	"time"

	"github.com/scottzx/1Agents/backend/internal/domainstore"
	"github.com/scottzx/1Agents/backend/internal/meta"
	"github.com/scottzx/1Agents/backend/internal/taskapi"
)

// openTestDB gives each test an isolated meta.db with crm domain tables ensured.
func openTestDB(t *testing.T) *meta.DB {
	t.Helper()
	db, err := meta.Open(filepath.Join(t.TempDir(), "meta.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := domainstore.EnsureTables(db.SQL(), AppID, domainDDLs()); err != nil {
		t.Fatalf("ensure tables: %v", err)
	}
	return db
}

func TestEnsureTablesIdempotent(t *testing.T) {
	db := openTestDB(t)
	// Run a second and third time — must not error and must not lose data.
	store := NewStore(db.SQL())
	if _, err := store.UpsertContact(Contact{Name: "Ada"}); err != nil {
		t.Fatalf("upsert: %v", err)
	}
	for i := 0; i < 2; i++ {
		if err := domainstore.EnsureTables(db.SQL(), AppID, domainDDLs()); err != nil {
			t.Fatalf("re-ensure %d: %v", i, err)
		}
	}
	cs, err := store.ListContacts()
	if err != nil || len(cs) != 1 {
		t.Fatalf("expected 1 contact after re-ensure, got %d (err=%v)", len(cs), err)
	}
}

func TestBusinessRefRoundTrip(t *testing.T) {
	id := "abc123"
	ref := LeadRef(id)
	if ref != "crm:lead:abc123" {
		t.Fatalf("LeadRef = %q", ref)
	}
	got, ok := LeadIDFromRef(ref)
	if !ok || got != id {
		t.Fatalf("LeadIDFromRef(%q) = %q,%v", ref, got, ok)
	}
	if _, ok := LeadIDFromRef("media:content:1"); ok {
		t.Fatalf("non-crm ref should not parse")
	}
}

func TestParseBusinessCard(t *testing.T) {
	card := "张三\nAcme 科技有限公司\n职位: CTO\nzhang@acme.com\n+86 138 0000 1111"
	c, ok := ParseBusinessCard(card)
	if !ok {
		t.Fatal("expected parse ok")
	}
	if c.Name != "张三" {
		t.Errorf("name = %q", c.Name)
	}
	if c.Email != "zhang@acme.com" {
		t.Errorf("email = %q", c.Email)
	}
	if c.Phone == "" {
		t.Errorf("phone empty")
	}
	if c.Title != "CTO" {
		t.Errorf("title = %q", c.Title)
	}
	if c.Source != "card" {
		t.Errorf("source = %q", c.Source)
	}

	if _, ok := ParseBusinessCard("just a note with no contact info"); ok {
		t.Error("weak text should not parse as contact")
	}
}

func TestEnrichHandler(t *testing.T) {
	db := openTestDB(t)
	pkgStore = NewStore(db.SQL())
	t.Cleanup(func() { pkgStore = nil })

	contact, _ := pkgStore.UpsertContact(Contact{Name: "Bob", Company: "TechCorp"})
	lead, _ := pkgStore.CreateLead(Lead{ContactID: contact.ID})

	res, err := enrichHandler(taskapi.FunctionContext{
		Task: meta.Task{ID: "t1", BusinessRef: LeadRef(lead.ID)},
	})
	if err != nil {
		t.Fatalf("enrichHandler: %v", err)
	}
	r := res.(enrichResult)
	if r.Kind != TaskTypeEnrich || r.LeadID != lead.ID {
		t.Fatalf("unexpected result: %+v", r)
	}
	if r.Industry != "科技" {
		t.Errorf("industry = %q, want 科技 (from TechCorp)", r.Industry)
	}
	if r.ScoreBump <= 0 {
		t.Errorf("scoreBump = %d", r.ScoreBump)
	}

	// No business_ref → error.
	if _, err := enrichHandler(taskapi.FunctionContext{Task: meta.Task{ID: "x"}}); err == nil {
		t.Error("expected error for missing business_ref")
	}
}

func TestCompletionHook_EnrichWriteback(t *testing.T) {
	db := openTestDB(t)
	pkgStore = NewStore(db.SQL())
	api := taskapi.New(meta.NewTaskStore(db))
	liveAPI = api
	t.Cleanup(func() { pkgStore = nil; liveAPI = nil })

	ws := t.TempDir()
	contact, _ := pkgStore.UpsertContact(Contact{Name: "Carol", Company: "AI Labs"})
	lead, _ := pkgStore.CreateLead(Lead{ContactID: contact.ID, Score: 5})

	// Dispatch an enrich (function) task so it lands in the store with the ref + fn label.
	taskID, err := DispatchEnrich(ws, lead.ID)
	if err != nil {
		t.Fatalf("DispatchEnrich: %v", err)
	}

	payload, _ := json.Marshal(enrichResult{Kind: TaskTypeEnrich, LeadID: lead.ID, ScoreBump: 10, Note: "enriched"})
	completionHook(taskapi.CompletionEvent{
		TaskID: taskID, Status: meta.TaskStatusCompleted, Result: string(payload), CompletedAt: time.Now(),
	})

	got, _, _ := pkgStore.GetLead(lead.ID)
	if got.Score != 15 {
		t.Fatalf("score = %d, want 15 (5 + 10 bump)", got.Score)
	}
	if got.Notes != "enriched" {
		t.Errorf("notes = %q", got.Notes)
	}
}

func TestCompletionHook_HumanStageTransition(t *testing.T) {
	db := openTestDB(t)
	pkgStore = NewStore(db.SQL())
	api := taskapi.New(meta.NewTaskStore(db))
	liveAPI = api
	t.Cleanup(func() { pkgStore = nil; liveAPI = nil })

	ws := t.TempDir()
	contact, _ := pkgStore.UpsertContact(Contact{Name: "Dora"})
	lead, _ := pkgStore.CreateLead(Lead{ContactID: contact.ID})
	if lead.Stage != StageNew {
		t.Fatalf("initial stage = %q", lead.Stage)
	}

	// Follow decision (human) → on completion, stage should advance to contacted.
	taskID, err := DispatchDecision(ws, lead.ID, StageContacted)
	if err != nil {
		t.Fatalf("DispatchDecision: %v", err)
	}
	completionHook(taskapi.CompletionEvent{
		TaskID: taskID, Status: meta.TaskStatusCompleted, CompletedAt: time.Now(),
	})
	got, _, _ := pkgStore.GetLead(lead.ID)
	if got.Stage != StageContacted {
		t.Fatalf("stage = %q, want %q", got.Stage, StageContacted)
	}

	// Drop decision → dropped.
	dropID, _ := DispatchDecision(ws, lead.ID, StageDropped)
	completionHook(taskapi.CompletionEvent{TaskID: dropID, Status: meta.TaskStatusCompleted, CompletedAt: time.Now()})
	got, _, _ = pkgStore.GetLead(lead.ID)
	if got.Stage != StageDropped {
		t.Fatalf("stage after drop = %q, want %q", got.Stage, StageDropped)
	}
}

func TestCompletionHook_IgnoresForeignRefs(t *testing.T) {
	db := openTestDB(t)
	pkgStore = NewStore(db.SQL())
	api := taskapi.New(meta.NewTaskStore(db))
	liveAPI = api
	t.Cleanup(func() { pkgStore = nil; liveAPI = nil })

	ws := t.TempDir()
	// A non-crm task: dispatch directly with a media ref via kernel namespace.
	taskID, err := api.DispatchTask("", taskapi.DispatchSpec{
		Title: "foreign", BusinessRef: "media:content:9", WorkspacePath: ws,
		Executor: meta.TaskExecutorHuman, Milestone: "decide:contacted",
	})
	if err != nil {
		t.Fatalf("dispatch foreign: %v", err)
	}
	// Hook must not panic or touch crm tables.
	completionHook(taskapi.CompletionEvent{TaskID: taskID, Status: meta.TaskStatusCompleted, CompletedAt: time.Now()})
	leads, _ := pkgStore.ListLeads()
	if len(leads) != 0 {
		t.Fatalf("foreign event should not create leads, got %d", len(leads))
	}
}

func TestIngestFromInbox(t *testing.T) {
	db := openTestDB(t)
	store := NewStore(db.SQL())
	inbox := meta.NewInboxStore(db)

	// An IM item carrying a business card → becomes a contact.
	if _, err := inbox.Capture(meta.InboxItem{
		Source:  meta.InboxSourceIM,
		Title:   "名片",
		Content: "李四\n职位: 销售总监\nli@foo.com\n+86 139 2222 3333",
	}); err != nil {
		t.Fatalf("capture card: %v", err)
	}
	// A plain note with no contact signal → skipped.
	if _, err := inbox.Capture(meta.InboxItem{Source: meta.InboxSourceManual, Title: "随手记", Content: "记得开会"}); err != nil {
		t.Fatalf("capture note: %v", err)
	}

	n, err := store.IngestFromInbox(inbox)
	if err != nil {
		t.Fatalf("IngestFromInbox: %v", err)
	}
	if n != 1 {
		t.Fatalf("ingested = %d, want 1", n)
	}
	cs, _ := store.ListContacts()
	if len(cs) != 1 || cs[0].Email != "li@foo.com" {
		t.Fatalf("contacts = %+v", cs)
	}
	if cs[0].Source != "im" {
		t.Errorf("source = %q, want im", cs[0].Source)
	}
}

func TestScoreWriteback(t *testing.T) {
	db := openTestDB(t)
	pkgStore = NewStore(db.SQL())
	api := taskapi.New(meta.NewTaskStore(db))
	liveAPI = api
	t.Cleanup(func() { pkgStore = nil; liveAPI = nil })

	ws := t.TempDir()
	contact, _ := pkgStore.UpsertContact(Contact{Name: "Eve"})
	lead, _ := pkgStore.CreateLead(Lead{ContactID: contact.ID})

	taskID, err := DispatchScore(ws, lead.ID, "客户对企业版很感兴趣")
	if err != nil {
		t.Fatalf("DispatchScore: %v", err)
	}
	payload, _ := json.Marshal(scoreResult{Kind: TaskTypeScore, Score: 72, Reason: "高意向", NextStep: "安排 demo"})
	completionHook(taskapi.CompletionEvent{
		TaskID: taskID, Status: meta.TaskStatusCompleted, Result: string(payload), CompletedAt: time.Now(),
	})
	got, _, _ := pkgStore.GetLead(lead.ID)
	if got.Score != 72 {
		t.Fatalf("score = %d, want 72", got.Score)
	}
}
