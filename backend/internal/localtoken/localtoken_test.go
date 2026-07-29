package localtoken

import "testing"

func TestSessionTokenIsBoundToSession(t *testing.T) {
	token := SessionToken("session-1")
	if token == "" || !ValidateSessionToken("session-1", token) {
		t.Fatal("valid Session token was rejected")
	}
	if ValidateSessionToken("session-2", token) {
		t.Fatal("Session token was replayable across Sessions")
	}
	if ValidateSessionToken("session-1", "forged") {
		t.Fatal("forged Session token was accepted")
	}
}
