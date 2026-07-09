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
	"crypto/rand"
	"encoding/hex"
)

// Token is the per-process internal bearer. Generated once at package init so
// every consumer in this process observes the same value.
var Token = generate()

func generate() string {
	var b [32]byte
	if _, err := rand.Read(b[:]); err != nil {
		// rand.Read never fails on supported platforms; fall back to a fixed
		// non-empty value so the bypass simply won't match anything useful.
		return "localtoken-unavailable"
	}
	return hex.EncodeToString(b[:])
}
