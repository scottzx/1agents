package domainownership

import (
	"context"
	"database/sql"
	"testing"

	"github.com/scottzx/1Agents/backend/internal/commandbus"
	"github.com/scottzx/1Agents/backend/internal/domainref"
	_ "modernc.org/sqlite"
)

// TestLegitCrossDomainPathsPass is the positive half of the gate: the legal
// Command → owner-write → Outbox Event → Query chain of a domain application
// runs end to end through the ownership infrastructure, while every denial on
// the way is audited (design §5, §7, §13.3).
func TestLegitCrossDomainPathsPass(t *testing.T) {
	db, reg := newOwnershipDB(t) // presales_leads + commerce_products + audit table
	SetDenialSink(DBSink(db))
	WireQueryDenialAudit()
	defer SetDenialSink(nil)
	defer domainref.SetDenialHook(nil)

	ctx := context.Background()

	// The presales app owns its write API contract.
	if err := reg.RegisterWriteAPI(NamespacePresales, "presales.lead.create"); err != nil {
		t.Fatalf("register write API: %v", err)
	}

	gw, err := commandbus.New(db)
	if err != nil {
		t.Fatalf("commandbus.New: %v", err)
	}
	err = gw.Register(commandbus.Descriptor{
		Contract:       "presales.lead.create",
		SchemaVersions: []int{1},
		AllowedKinds:   []commandbus.ActorKind{commandbus.ActorUser, commandbus.ActorHuman},
		Handler: func(ctx context.Context, cmd commandbus.Command, tx *sql.Tx) (commandbus.Result, error) {
			var p struct {
				LeadID string `json:"leadId"`
				Name   string `json:"name"`
			}
			if err := cmd.PayloadObject(&p); err != nil {
				return commandbus.Result{}, err
			}
			// The handler writes ONLY its own domain's tables, through the
			// guarded executor, inside the gateway transaction.
			g := GuardTx(NamespacePresales, reg, tx)
			if _, err := g.Exec(`INSERT INTO presales_leads (id, name) VALUES (?, ?)`, p.LeadID, p.Name); err != nil {
				return commandbus.Result{}, err
			}
			// The fact notification is appended by the gateway in the SAME
			// transaction when the result carries an event envelope.
			return commandbus.Result{
				Version:            1,
				TargetID:           p.LeadID,
				EventID:            "evt-" + p.LeadID,
				EventType:          "presales.lead_created",
				EventSchemaVersion: 1,
				SubjectRef:         "presales:lead:" + p.LeadID,
			}, nil
		},
	})
	if err != nil {
		t.Fatalf("register command: %v", err)
	}

	// ── legitimate Command path ─────────────────────────────────────────────
	res, err := gw.Dispatch(ctx, commandbus.Command{
		Contract:       "presales.lead.create",
		SchemaVersion:  1,
		WorkspaceID:    "ws-1",
		Actor:          commandbus.Actor{Kind: commandbus.ActorUser, Name: "alice"},
		IdempotencyKey: "lead-1-create",
		Payload:        []byte(`{"leadId":"lead-1","name":"Acme"}`),
	})
	if err != nil {
		t.Fatalf("legit command dispatch: %v", err)
	}
	if res.Status != commandbus.ResultSucceeded || res.TargetID != "lead-1" {
		t.Fatalf("unexpected result: %+v", res)
	}
	var leads int
	if err := db.QueryRow(`SELECT COUNT(1) FROM presales_leads`).Scan(&leads); err != nil || leads != 1 {
		t.Fatalf("presales_leads rows = %d (%v), want 1", leads, err)
	}

	// ── legitimate Event path: the outbox row committed atomically ─────────
	var eventType, subjectRef, causationID string
	if err := db.QueryRow(`
		SELECT event_type, subject_ref, causation_id FROM outbox_events WHERE event_id = ?`,
		"evt-lead-1").Scan(&eventType, &subjectRef, &causationID); err != nil {
		t.Fatalf("outbox row missing after legit command: %v", err)
	}
	if eventType != "presales.lead_created" || subjectRef != "presales:lead:lead-1" || causationID == "" {
		t.Fatalf("unexpected outbox row: type=%q subject=%q causation=%q", eventType, subjectRef, causationID)
	}

	// Replay with the same idempotency key: no second effect.
	res2, err := gw.Dispatch(ctx, commandbus.Command{
		Contract:       "presales.lead.create",
		SchemaVersion:  1,
		WorkspaceID:    "ws-1",
		Actor:          commandbus.Actor{Kind: commandbus.ActorUser, Name: "alice"},
		IdempotencyKey: "lead-1-create",
		Payload:        []byte(`{"leadId":"lead-1","name":"Acme"}`),
	})
	if err != nil || res2.Status != commandbus.ResultReplayed {
		t.Fatalf("replay: status=%q err=%v, want replayed", res2.Status, err)
	}
	if err := db.QueryRow(`SELECT COUNT(1) FROM presales_leads`).Scan(&leads); err != nil || leads != 1 {
		t.Fatalf("replay produced extra rows: %d", leads)
	}

	// ── Command permission denial is audited ────────────────────────────────
	// Agents may not create leads (AllowedKinds: user/human only).
	_, err = gw.Dispatch(ctx, commandbus.Command{
		Contract:       "presales.lead.create",
		SchemaVersion:  1,
		WorkspaceID:    "ws-1",
		Actor:          commandbus.Actor{Kind: commandbus.ActorAgent, Name: "bot"},
		IdempotencyKey: "lead-agent-attempt",
		Payload:        []byte(`{"leadId":"lead-x","name":"X"}`),
	})
	if !commandbus.IsCode(err, commandbus.CodePermissionDenied) {
		t.Fatalf("agent dispatch: want permission_denied, got %v", err)
	}
	var rejectedAudits int
	if err := db.QueryRow(`
		SELECT COUNT(1) FROM command_executions
		WHERE contract = ? AND status = 'rejected' AND error_code = 'permission_denied'`,
		"presales.lead.create").Scan(&rejectedAudits); err != nil || rejectedAudits != 1 {
		t.Fatalf("command_executions rejected audit rows = %d (%v), want 1", rejectedAudits, err)
	}

	// ── legitimate Query path ───────────────────────────────────────────────
	qreg := domainref.NewRegistry()
	if err := qreg.Register(&leadProvider{db: db}); err != nil {
		t.Fatalf("register query provider: %v", err)
	}
	ref, err := domainref.NewDomainRef("presales", "lead", "lead-1", 1)
	if err != nil {
		t.Fatal(err)
	}
	summary, err := qreg.Resolve(ctx, domainref.QueryRequest{Ref: ref, Actor: "owner", WorkspaceID: "ws-1"})
	if err != nil {
		t.Fatalf("legit query resolve: %v", err)
	}
	if summary.Title != "Acme" {
		t.Fatalf("summary title = %q, want Acme", summary.Title)
	}

	// ── Query permission denial is audited via the hook ────────────────────
	_, err = qreg.Resolve(ctx, domainref.QueryRequest{Ref: ref, Actor: "intruder", WorkspaceID: "ws-1"})
	if !domainref.IsCode(err, domainref.CodePermissionDenied) {
		t.Fatalf("intruder query: want permission_denied, got %v", err)
	}
	var queryDenials int
	if err := db.QueryRow(`
		SELECT COUNT(1) FROM kernel_access_denials WHERE action = ?`,
		string(ActionQueryPermission)).Scan(&queryDenials); err != nil || queryDenials != 1 {
		t.Fatalf("query denial audit rows = %d (%v), want 1", queryDenials, err)
	}
}

// leadProvider is the presales domain's read seam: it answers DomainRef
// queries for presales:lead objects and enforces object-level permission.
type leadProvider struct{ db *sql.DB }

func (p *leadProvider) Namespace() string { return "presales" }
func (p *leadProvider) Versions() []int   { return []int{1} }

func (p *leadProvider) Query(ctx context.Context, req domainref.QueryRequest) (domainref.ObjectSummary, error) {
	if req.Actor != "owner" {
		return domainref.ObjectSummary{}, domainref.NewError(domainref.CodePermissionDenied,
			"actor %q may not read lead %s", req.Actor, req.Ref.ID)
	}
	var name string
	err := p.db.QueryRow(`SELECT name FROM presales_leads WHERE id = ?`, req.Ref.ID).Scan(&name)
	if err == sql.ErrNoRows {
		return domainref.ObjectSummary{}, domainref.NewError(domainref.CodeNotFound,
			"lead %s does not exist", req.Ref.ID)
	}
	if err != nil {
		return domainref.ObjectSummary{}, err
	}
	return domainref.ObjectSummary{Ref: req.Ref, Title: name, Status: "open"}, nil
}
