package git

import "testing"

func TestParseSubmoduleStatus(t *testing.T) {
	out := " 244a18e8944cf01cb81ea3f5178ae10f4e881233 modules/1acp (v0.12.0-52-g244a18e)\n" +
		"+bc1a6703399f93b1bc3fd4936f38b805ab21338d modules/cc-switch-cli (v5.9.1)\n" +
		"-80ea6ea18b8937db0d453e3f5440fdfbc4778a32 modules/FunClip\n" +
		"U979ba4be341534731445e4b11a84f0c0aa4b40a8 modules/cc-connect (v1)\n"
	got := parseSubmoduleStatus(out)
	if len(got) != 4 {
		t.Fatalf("len=%d want 4: %+v", len(got), got)
	}
	if got[0].Flag != "" || got[0].Path != "modules/1acp" || got[0].Short != "244a18e" {
		t.Fatalf("entry0: %+v", got[0])
	}
	if got[1].Flag != "+" || got[1].Path != "modules/cc-switch-cli" {
		t.Fatalf("entry1: %+v", got[1])
	}
	if got[2].Flag != "-" || got[2].Desc != "" {
		t.Fatalf("entry2: %+v", got[2])
	}
	if got[3].Flag != "U" {
		t.Fatalf("entry3: %+v", got[3])
	}
}
