package icloud

import "testing"

func TestParseVCards(t *testing.T) {
	data := "BEGIN:VCARD\r\n" +
		"VERSION:3.0\r\n" +
		"FN:Zhang San\r\n" +
		"ORG:Acme Inc.;Engineering\r\n" +
		"TITLE:Staff Engineer\r\n" +
		"item1.TEL;type=CELL;type=VOICE:+86 138 0013 8000\r\n" + // grouped + folded value
		"EMAIL;type=INTERNET:zhang@example.com\r\n" +
		"END:VCARD\r\n" +
		"BEGIN:VCARD\r\nVERSION:3.0\r\nFN:Li Si\r\nTEL:+1 (555) 010-2030\r\nEND:VCARD\r\n"

	cs := parseVCards(data)
	if len(cs) != 2 {
		t.Fatalf("want 2 contacts, got %d", len(cs))
	}
	z := cs[0]
	if z.Name != "Zhang San" || z.Org != "Acme Inc." || z.Title != "Staff Engineer" {
		t.Errorf("c0 fields wrong: %+v", z)
	}
	if len(z.Phones) != 1 || z.Phones[0] != "+86 138 0013 8000" {
		t.Errorf("c0 phone (group/strip) wrong: %+v", z.Phones)
	}
	if len(z.Emails) != 1 || z.Emails[0] != "zhang@example.com" {
		t.Errorf("c0 email wrong: %+v", z.Emails)
	}
	if cs[1].Name != "Li Si" || len(cs[1].Phones) != 1 {
		t.Errorf("c1 wrong: %+v", cs[1])
	}
}

func TestUnfoldAndEscape(t *testing.T) {
	// Folded note value: continuation line begins with a space.
	lines := unfold("FN:Long\r\n  Name\r\n")
	if len(lines) != 1 || lines[0] != "FN:Long Name" {
		t.Errorf("unfold wrong: %q", lines)
	}
	if got := unescape(`a\,b\;c\\d`); got != `a,b;c\d` {
		t.Errorf("unescape wrong: %q", got)
	}
}
