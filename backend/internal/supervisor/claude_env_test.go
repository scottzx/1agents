package supervisor

import "testing"

func TestMergeEnvironmentOverridesOnlyConfiguredValues(t *testing.T) {
	base := []string{
		"PATH=/bin",
		"ANTHROPIC_AUTH_TOKEN=old-token",
		"KEEP=unchanged",
	}

	got := mergeEnvironment(base, map[string]string{
		"ANTHROPIC_AUTH_TOKEN": "new-token",
		"ANTHROPIC_BASE_URL":   "https://example.test",
	})

	values := make(map[string]string, len(got))
	for _, entry := range got {
		for i := 0; i < len(entry); i++ {
			if entry[i] == '=' {
				values[entry[:i]] = entry[i+1:]
				break
			}
		}
	}

	if values["ANTHROPIC_AUTH_TOKEN"] != "new-token" {
		t.Fatalf("AUTH_TOKEN = %q, want new-token", values["ANTHROPIC_AUTH_TOKEN"])
	}
	if values["ANTHROPIC_BASE_URL"] != "https://example.test" {
		t.Fatalf("BASE_URL = %q, want configured URL", values["ANTHROPIC_BASE_URL"])
	}
	if values["KEEP"] != "unchanged" {
		t.Fatalf("KEEP = %q, want unchanged", values["KEEP"])
	}
}

func TestMergeEnvironmentWithoutOverridesPreservesBase(t *testing.T) {
	base := []string{"PATH=/bin", "KEEP=unchanged"}
	got := mergeEnvironment(base, nil)

	if len(got) != len(base) {
		t.Fatalf("got %d entries, want %d", len(got), len(base))
	}
	for i := range base {
		if got[i] != base[i] {
			t.Fatalf("entry %d = %q, want %q", i, got[i], base[i])
		}
	}
}
