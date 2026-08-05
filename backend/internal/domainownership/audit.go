package domainownership

import (
	"database/sql"
	"fmt"
	"sync"
	"time"
)

// Action labels the kind of access an audit record describes.
type Action string

const (
	ActionTableWrite      Action = "table_write"       // Guard / registry blocked a cross-domain write
	ActionTableRead       Action = "table_read"        // Guard / registry blocked a cross-domain read
	ActionRepository      Action = "repository_access" // blocked direct access to another domain's repository
	ActionQueryPermission Action = "query_permission"  // domainref Query provider denied the actor
)

// Denial is one audited access rejection. Times are stamped by the sink.
type Denial struct {
	Actor           string `json:"actor,omitempty"`
	CallerNamespace string `json:"callerNamespace"`
	Action          Action `json:"action"`
	TargetNamespace string `json:"targetNamespace,omitempty"`
	Target          string `json:"target,omitempty"` // table / repository / ref
	Code            string `json:"code"`
	Reason          string `json:"reason"`
	CorrelationID   string `json:"correlationId,omitempty"`
	// At is set by RecordDenial when zero.
	At time.Time `json:"at"`
}

// DenialSink receives every recorded denial. Install with SetDenialSink.
type DenialSink func(Denial)

var (
	sinkMu sync.RWMutex
	sink   DenialSink
)

// SetDenialSink installs the process-wide receiver for audited denials.
// Passing nil detaches any sink (denials are then dropped after the in-test
// capture helper, if any, runs). Startup wires the persistent DB sink.
func SetDenialSink(s DenialSink) {
	sinkMu.Lock()
	sink = s
	sinkMu.Unlock()
}

// RecordDenial stamps and forwards one denial to the installed sink. It
// never panics and never returns an error: auditing an access denial must
// not mask the denial itself.
func RecordDenial(d Denial) {
	if d.At.IsZero() {
		d.At = time.Now().UTC()
	}
	sinkMu.RLock()
	s := sink
	sinkMu.RUnlock()
	if s != nil {
		s(d)
	}
}

// kernel_access_denials is the persistent audit trail for rejected
// cross-domain access. It is itself a NEW kernel table, so per the prefix
// rule it carries the kernel_ prefix.
const denialTableDDL = `CREATE TABLE IF NOT EXISTS kernel_access_denials (
	id               INTEGER PRIMARY KEY AUTOINCREMENT,
	at               TEXT NOT NULL,
	actor            TEXT NOT NULL DEFAULT '',
	caller_namespace TEXT NOT NULL DEFAULT '',
	action           TEXT NOT NULL DEFAULT '',
	target_namespace TEXT NOT NULL DEFAULT '',
	target           TEXT NOT NULL DEFAULT '',
	code             TEXT NOT NULL DEFAULT '',
	reason           TEXT NOT NULL DEFAULT '',
	correlation_id   TEXT NOT NULL DEFAULT ''
)`

// EnsureAuditSchema creates the kernel_access_denials table when missing.
// Idempotent; safe to call at every startup.
func EnsureAuditSchema(db *sql.DB) error {
	if db == nil {
		return fmt.Errorf("domainownership: nil database handle")
	}
	_, err := db.Exec(denialTableDDL)
	if err != nil {
		return fmt.Errorf("domainownership: ensure kernel_access_denials: %w", err)
	}
	return nil
}

// DBSink returns a DenialSink that persists denials to db best-effort. The
// write is fire-and-forget so a slow or locked audit table never blocks the
// request path.
func DBSink(db *sql.DB) DenialSink {
	return func(d Denial) {
		if db == nil {
			return
		}
		_, _ = db.Exec(`
			INSERT INTO kernel_access_denials
				(at, actor, caller_namespace, action, target_namespace, target, code, reason, correlation_id)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			d.At.UTC().Format(time.RFC3339Nano), d.Actor, d.CallerNamespace, string(d.Action),
			d.TargetNamespace, d.Target, d.Code, d.Reason, d.CorrelationID)
	}
}

// Denials reads back audited denials, newest first. Primarily for tests and
// future admin surfaces.
func Denials(db *sql.DB, limit int) ([]Denial, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	rows, err := db.Query(`
		SELECT at, actor, caller_namespace, action, target_namespace, target, code, reason, correlation_id
		FROM kernel_access_denials ORDER BY id DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Denial{}
	for rows.Next() {
		var d Denial
		var at, action string
		if err := rows.Scan(&at, &d.Actor, &d.CallerNamespace, &action,
			&d.TargetNamespace, &d.Target, &d.Code, &d.Reason, &d.CorrelationID); err != nil {
			return nil, err
		}
		d.Action = Action(action)
		if t, err := time.Parse(time.RFC3339Nano, at); err == nil {
			d.At = t
		}
		out = append(out, d)
	}
	return out, rows.Err()
}
