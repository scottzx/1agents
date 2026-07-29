// Package localtoken holds a process-scoped random bearer token used by
// loopback helper subprocesses (e.g. the `1agents project-items` MCP server that
// the AI Project Manager session spawns) to authenticate back to this
// backend's own HTTP API.
//
// The token never leaves the host: it is generated once at startup, injected
// into the helper subprocess via an environment variable, and accepted by the
// auth middleware only for requests that also originate from localhost. This
// keeps the internal call path working even when a public tunnel is active
// (which otherwise requires an ephemeral session token for every request).
package localtoken

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
)

// Token is the per-process internal bearer. Generated once at package init so
// every consumer in this process observes the same value.
var Token = generate()

// SessionToken binds a helper request to one host-created chat Session without
// exposing the process-wide internal bearer to the agent shell.
func SessionToken(sessionID string) string {
	mac := hmac.New(sha256.New, []byte(Token))
	mac.Write([]byte("1agents-session:"))
	mac.Write([]byte(sessionID))
	return hex.EncodeToString(mac.Sum(nil))
}

// ValidateSessionToken verifies the stable Session attribution injected by the
// host. A token for one Session cannot be replayed as another Session.
func ValidateSessionToken(sessionID, token string) bool {
	if sessionID == "" || token == "" {
		return false
	}
	return hmac.Equal([]byte(SessionToken(sessionID)), []byte(token))
}

func generate() string {
	var b [32]byte
	if _, err := rand.Read(b[:]); err != nil {
		// rand.Read never fails on supported platforms; fall back to a fixed
		// non-empty value so the bypass simply won't match anything useful.
		return "localtoken-unavailable"
	}
	return hex.EncodeToString(b[:])
}
