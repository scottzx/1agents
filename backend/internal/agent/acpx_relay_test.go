package agent

import (
	"encoding/json"
	"testing"
)

// The relay forwards raw bytes verbatim; the only rewrite point is the
// first-prompt system-context merge. It must go through map[string]any so
// fields the WsMessage peek struct does not declare survive the round trip.
func TestMergeSystemContextPreservesUnknownFields(t *testing.T) {
	raw := []byte(`{"action":"prompt","sessionId":"s1","text":"hello","attachments":[{"mediaType":"image/png","data":"aGk="}],"futureField":{"nested":true}}`)

	out := mergeSystemContextIntoPrompt(raw, "ROLE CONTEXT")

	var got map[string]any
	if err := json.Unmarshal(out, &got); err != nil {
		t.Fatalf("rewritten prompt is not valid JSON: %v", err)
	}
	if got["text"] != "ROLE CONTEXT\n\nhello" {
		t.Errorf("text = %q, want merged preamble", got["text"])
	}
	if _, ok := got["attachments"]; !ok {
		t.Errorf("attachments field dropped by rewrite")
	}
	if _, ok := got["futureField"]; !ok {
		t.Errorf("unknown field dropped by rewrite — relay is no longer lossless")
	}
}

// Malformed input must pass through unchanged rather than being swallowed.
func TestMergeSystemContextMalformedPassthrough(t *testing.T) {
	raw := []byte(`not-json`)
	if out := mergeSystemContextIntoPrompt(raw, "X"); string(out) != "not-json" {
		t.Errorf("malformed input rewritten to %q, want verbatim passthrough", out)
	}
}
