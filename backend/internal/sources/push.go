package sources

// push.go is the inbound (push) mirror of the pull path. A "push" collection is
// never crawled: a local agent POSTs its already-processed records to
// /api/data/push/{source}/{kind} and they land verbatim in bronze (source_records)
// — the retention hook — exactly like a pulled page, so governance and the viewer
// treat push and pull sources identically. This file owns the neutral bits: is a
// source/kind push, validate an inbound item against its declared schema, and turn
// valid items into bronze RawRecords. The HTTP receiver (ingest.HandlePush) and
// credential check live in the ingest layer.

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
)

// PushField is one declared field of a push collection's schema (mirrors
// ManifestField, kept in this package so RESTDescriptor can carry it).
type PushField struct {
	Name     string `json:"name"`
	Type     string `json:"type"` // string|number|bool|object|array|any ("" ⇒ any)
	Required bool   `json:"required"`
}

// IsPushSource reports whether a source has any push-transport collection.
func IsPushSource(source string) bool {
	for _, d := range RESTKinds(source) {
		if d.Transport == "push" {
			return true
		}
	}
	return false
}

// IsPushKind reports whether a specific (source, kind) is inbound-push — used to
// exclude it from the pull scheduler (push has no cursor and no periodic sync).
func IsPushKind(source, kind string) bool {
	d, ok := RESTDescriptorFor(source, kind)
	return ok && d.Transport == "push"
}

// PushDescriptorFor returns the descriptor for a push (source, kind), or ok=false
// when the kind is unknown or is not a push collection.
func PushDescriptorFor(source, kind string) (RESTDescriptor, bool) {
	d, ok := RESTDescriptorFor(source, kind)
	if !ok || d.Transport != "push" {
		return RESTDescriptor{}, false
	}
	return d, true
}

// PushReject explains why one pushed item failed schema validation.
type PushReject struct {
	Index  int    `json:"index"`
	Reason string `json:"reason"`
}

// BuildPushRecords validates each raw JSON item against the descriptor's schema
// and turns the valid ones into bronze RawRecords. UID comes from the declared
// UIDField (falling back to a content hash); ETag is a content hash, so pushing an
// identical record twice is a no-op (its fetched_at stays put and governance won't
// reprocess it). All items are validated first — a non-empty rejects slice means
// the caller should commit nothing and report the rejects.
func BuildPushRecords(d RESTDescriptor, collection string, items []json.RawMessage) (recs []RawRecord, rejects []PushReject) {
	for i, raw := range items {
		var obj map[string]json.RawMessage
		if err := json.Unmarshal(raw, &obj); err != nil {
			rejects = append(rejects, PushReject{Index: i, Reason: "item must be a JSON object"})
			continue
		}
		if reason := validatePushObj(d.Schema, obj); reason != "" {
			rejects = append(rejects, PushReject{Index: i, Reason: reason})
			continue
		}
		recs = append(recs, RawRecord{
			Kind:        d.Kind,
			Collection:  collection,
			UID:         pushUID(d.UIDField, obj, raw),
			ETag:        contentHash(raw),
			ContentType: "application/json",
			Payload:     string(raw),
		})
	}
	return recs, rejects
}

// validatePushObj checks required fields are present and non-null and that present
// fields match their declared coarse JSON type. Returns "" when the item is valid.
func validatePushObj(schema []PushField, obj map[string]json.RawMessage) string {
	for _, f := range schema {
		raw, ok := obj[f.Name]
		if !ok || len(raw) == 0 || string(raw) == "null" {
			if f.Required {
				return fmt.Sprintf("missing required field %q", f.Name)
			}
			continue
		}
		if f.Type != "" && f.Type != "any" && !jsonTypeMatches(f.Type, raw) {
			return fmt.Sprintf("field %q must be %s", f.Name, f.Type)
		}
	}
	return ""
}

// jsonTypeMatches does a cheap first-byte check of a raw JSON value's type.
func jsonTypeMatches(t string, raw json.RawMessage) bool {
	s := strings.TrimSpace(string(raw))
	if s == "" {
		return false
	}
	switch t {
	case "string":
		return s[0] == '"'
	case "number":
		return s[0] == '-' || (s[0] >= '0' && s[0] <= '9')
	case "bool":
		return s == "true" || s == "false"
	case "object":
		return s[0] == '{'
	case "array":
		return s[0] == '['
	}
	return true
}

// pushUID resolves the stable id for a pushed record: the declared UIDField's
// value when present, else a content hash (so an id-less record still dedups on
// identical content).
func pushUID(field string, obj map[string]json.RawMessage, raw json.RawMessage) string {
	if field != "" {
		if v, ok := obj[field]; ok {
			var s string
			if json.Unmarshal(v, &s) == nil && s != "" {
				return s
			}
			if t := strings.Trim(string(v), `"`); t != "" && t != "null" {
				return t
			}
		}
	}
	return contentHash(raw)
}

// contentHash is a short hex digest of a record's bytes, used for ETag/UID.
func contentHash(raw []byte) string {
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:16])
}
