package domainref_test

import (
	"encoding/json"
	"testing"

	"github.com/scottzx/1Agents/backend/internal/domainref"
)

// codeOf extracts the structured contract code from err, failing the test
// when err is not a contract error at all.
func codeOf(t *testing.T, err error) domainref.Code {
	t.Helper()
	c, ok := domainref.CodeOf(err)
	if !ok {
		t.Fatalf("expected structured contract error, got %v", err)
	}
	return c
}

// ── DomainRef: valid construction & stable serialization ───────────────────

func TestNewDomainRefValid(t *testing.T) {
	r, err := domainref.NewDomainRef("crm", "lead", "42", 0)
	if err != nil {
		t.Fatalf("NewDomainRef: %v", err)
	}
	if r.Namespace != "crm" || r.Type != "lead" || r.ID != "42" || r.ContractVersion != 0 {
		t.Errorf("unexpected ref: %+v", r)
	}
}

func TestDomainRefStringStable(t *testing.T) {
	r, _ := domainref.NewDomainRef("crm", "lead", "42", 0)
	first := r.String()
	for i := 0; i < 3; i++ {
		if got := r.String(); got != first {
			t.Fatalf("unstable serialization: %q vs %q", got, first)
		}
	}
	if first != "crm:lead:42" {
		t.Errorf("v0 string = %q, want crm:lead:42", first)
	}

	v2, _ := domainref.NewDomainRef("crm", "lead", "42", 2)
	if got := v2.String(); got != "crm:lead:42@v2" {
		t.Errorf("v2 string = %q, want crm:lead:42@v2", got)
	}
}

func TestDomainRefParseRoundTrip(t *testing.T) {
	for _, s := range []string{
		"crm:lead:42",
		"sources:feishu:feishu_chat",
		"media:clip:7",
		"crm:lead:42@v3",
		"a:b:c:d", // ids may contain ':' (SplitN keeps the tail intact)
	} {
		r, err := domainref.ParseDomainRef(s)
		if err != nil {
			t.Fatalf("Parse(%q): %v", s, err)
		}
		if got := r.String(); got != s {
			t.Errorf("round trip %q → %q", s, got)
		}
	}

	r, _ := domainref.ParseDomainRef("crm:lead:42@v3")
	if r.ContractVersion != 3 {
		t.Errorf("version = %d, want 3", r.ContractVersion)
	}
	if r.ID != "42" {
		t.Errorf("id = %q, want 42", r.ID)
	}
}

func TestDomainRefJSONStable(t *testing.T) {
	r, _ := domainref.NewDomainRef("crm", "lead", "42", 1)
	b1, err := json.Marshal(r)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	b2, _ := json.Marshal(r)
	if string(b1) != string(b2) {
		t.Fatalf("unstable JSON: %s vs %s", b1, b2)
	}
	var back domainref.DomainRef
	if err := json.Unmarshal(b1, &back); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if back != r {
		t.Errorf("JSON round trip: %+v vs %+v", back, r)
	}
	// Every core field must always be present (no omitempty on identity).
	var raw map[string]any
	_ = json.Unmarshal(b1, &raw)
	for _, k := range []string{"namespace", "type", "id", "contractVersion"} {
		if _, ok := raw[k]; !ok {
			t.Errorf("JSON missing field %q: %s", k, b1)
		}
	}
}

// ── DomainRef: invalid inputs are rejected with structured errors ──────────

func TestNewDomainRefRejectsInvalid(t *testing.T) {
	cases := []struct {
		name            string
		ns, typ, id     string
		version         int
		wantMsgContains string
	}{
		{"empty namespace", "", "lead", "42", 0, "namespace"},
		{"empty type", "crm", "", "42", 0, "type"},
		{"empty id", "crm", "lead", "", 0, "id"},
		{"whitespace namespace", " ", "lead", "42", 0, "namespace"},
		{"whitespace id", "crm", "lead", " 42 ", 0, "id"},
		{"uppercase namespace", "CRM", "lead", "42", 0, "namespace"},
		{"colon in namespace", "cr:m", "lead", "42", 0, "namespace"},
		{"at in type", "crm", "le@ad", "42", 0, "type"},
		{"at in id", "crm", "lead", "4@2", 0, "id"},
		{"negative version", "crm", "lead", "42", -1, "version"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := domainref.NewDomainRef(tc.ns, tc.typ, tc.id, tc.version)
			if err == nil {
				t.Fatalf("expected error for %+v", tc)
			}
			if !domainref.IsCode(err, domainref.CodeInvalidRef) {
				t.Errorf("code = %v, want invalid_ref (err=%v)", codeOf(t, err), err)
			}
		})
	}
}

func TestParseDomainRefRejectsInvalid(t *testing.T) {
	for _, s := range []string{
		"",
		"crm",
		"crm:lead",
		"crm:lead:",      // empty id
		"crm:lead:@v2",   // empty id with version
		"crm:lead:42@v0", // v0 suffix is not a version marker → '@' stays in id → invalid
		"crm:lead:42@vX", // malformed version suffix → '@' stays in id → invalid
		"CRM:lead:42",    // uppercase namespace
	} {
		t.Run(s, func(t *testing.T) {
			_, err := domainref.ParseDomainRef(s)
			if err == nil {
				t.Fatalf("Parse(%q): expected error", s)
			}
			if !domainref.IsCode(err, domainref.CodeInvalidRef) {
				t.Errorf("Parse(%q): code = %v, want invalid_ref", s, codeOf(t, err))
			}
		})
	}
}

// ── CaseRef ────────────────────────────────────────────────────────────────

func TestCaseRefRoundTrip(t *testing.T) {
	c, err := domainref.NewCaseRef("1agentsapp", "wc123", 0)
	if err != nil {
		t.Fatalf("NewCaseRef: %v", err)
	}
	if got := c.String(); got != "case:1agentsapp:wc123" {
		t.Errorf("string = %q", got)
	}
	back, err := domainref.ParseCaseRef(c.String())
	if err != nil || back != c {
		t.Fatalf("round trip: %+v %v", back, err)
	}

	v2, _ := domainref.NewCaseRef("ws", "c1", 2)
	if got := v2.String(); got != "case:ws:c1@v2" {
		t.Errorf("versioned string = %q", got)
	}
	parsed, err := domainref.ParseCaseRef("case:ws:c1@v2")
	if err != nil || parsed != v2 {
		t.Fatalf("versioned round trip: %+v %v", parsed, err)
	}
}

func TestCaseRefRejectsInvalid(t *testing.T) {
	if _, err := domainref.NewCaseRef("", "c1", 0); !domainref.IsCode(err, domainref.CodeInvalidRef) {
		t.Errorf("empty workspace: %v", err)
	}
	if _, err := domainref.NewCaseRef("ws", "", 0); !domainref.IsCode(err, domainref.CodeInvalidRef) {
		t.Errorf("empty case id: %v", err)
	}
	if _, err := domainref.NewCaseRef("ws", "c1", -2); !domainref.IsCode(err, domainref.CodeInvalidRef) {
		t.Errorf("negative version: %v", err)
	}

	for _, s := range []string{
		"",
		"1agentsapp:wc123", // missing case: prefix
		"casez:ws:c1",      // wrong prefix
		"case:ws",          // too few parts
		"case:ws:c1:extra", // too many parts
		"case:WS:c1",       // uppercase workspace
	} {
		if _, err := domainref.ParseCaseRef(s); err == nil {
			t.Errorf("ParseCaseRef(%q): expected error", s)
		} else if !domainref.IsCode(err, domainref.CodeInvalidRef) {
			t.Errorf("ParseCaseRef(%q): code = %v, want invalid_ref", s, codeOf(t, err))
		}
	}
}

func TestCaseRefJSON(t *testing.T) {
	c, _ := domainref.NewCaseRef("ws", "c1", 1)
	b, err := json.Marshal(c)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var back domainref.CaseRef
	if err := json.Unmarshal(b, &back); err != nil || back != c {
		t.Fatalf("JSON round trip: %+v %v", back, err)
	}
}

// ── businessRef compatibility ──────────────────────────────────────────────

func TestBusinessRefExplicitConversion(t *testing.T) {
	// Every historical business_ref shape in this repo must convert cleanly
	// and round-trip byte-identically (so ListTasksByBusinessRef etc. keep
	// matching existing rows).
	for _, legacy := range []string{
		"crm:lead:42",                // binding.go doc example
		"media:clip:7",               // types.go doc example
		"sources:feishu:feishu_chat", // ingest/sync.go actual work order
		"sources:microsoft:calendar",
	} {
		ref, err := domainref.DomainRefFromBusinessRef(legacy)
		if err != nil {
			t.Fatalf("convert %q: %v", legacy, err)
		}
		if ref.ContractVersion != 0 {
			t.Errorf("%q: version = %d, want 0 (legacy)", legacy, ref.ContractVersion)
		}
		if got := domainref.DomainRefToBusinessRef(ref); got != legacy {
			t.Errorf("round trip %q → %q (must be byte-identical)", legacy, got)
		}
	}
}

func TestBusinessRefRejectsMalformed(t *testing.T) {
	for _, legacy := range []string{"", "legacy", "only-two:parts", "UPPER:case:x"} {
		_, err := domainref.DomainRefFromBusinessRef(legacy)
		if err == nil {
			t.Errorf("convert %q: expected structured error", legacy)
		} else if !domainref.IsCode(err, domainref.CodeInvalidRef) {
			t.Errorf("convert %q: code = %v, want invalid_ref", legacy, codeOf(t, err))
		}
	}
}
