package cli

import "testing"

func TestValidateAutomationCreate(t *testing.T) {
	if err := validateAutomationCreate(automationCreateFlags{}); err == nil {
		t.Fatal("empty flags should fail")
	}
	ok := automationCreateFlags{Title: "每日摘要", Instructions: "总结未读邮件"}
	if err := validateAutomationCreate(ok); err != nil {
		t.Fatal(err)
	}
	both := ok
	both.EveryMinutes = 60
	both.At = "2026-08-13T09:00:00+08:00"
	if err := validateAutomationCreate(both); err == nil {
		t.Fatal("at + every-minutes should fail")
	}
	badAt := ok
	badAt.At = "tomorrow"
	if err := validateAutomationCreate(badAt); err == nil {
		t.Fatal("non-RFC3339 --at should fail")
	}
}
