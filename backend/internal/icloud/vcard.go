package icloud

import "strings"

// Contact is one address-book entry parsed from a vCard.
type Contact struct {
	Name   string
	Phones []string
	Emails []string
	Org    string
	Title  string
}

// ParseVCards is the exported entry point for governance (internal/govern),
// which parses bronze vCard payloads without knowing CardDAV internals.
func ParseVCards(data string) []Contact { return parseVCards(data) }

// VCardProps returns a vCard's properties as ordered (NAME, value) pairs using
// native property names (Apple "itemN." group prefix + params stripped, value
// unescaped). It skips only the structural BEGIN/END/VERSION lines. Repeated
// properties (e.g. two TELs) appear multiple times, preserving source order.
// Used by the data-source viewer to show raw records natively — no fixed schema
// is imposed, so contacts/todos/calendars all render by whatever they carry.
func VCardProps(data string) [][2]string {
	var out [][2]string
	for _, line := range unfold(data) {
		name, raw := splitProp(line)
		switch name {
		case "", "BEGIN", "END", "VERSION":
			continue
		}
		out = append(out, [2]string{name, unescape(raw)})
	}
	return out
}

// parseVCards parses a payload that may contain one or more concatenated vCards
// (as returned by an addressbook-query REPORT) into Contacts. It is tolerant:
// unknown properties are ignored, line folding is unwrapped, Apple's
// "itemN.PROP" grouping is stripped, and standard vCard escaping is undone.
func parseVCards(data string) []Contact {
	var out []Contact
	var cur *Contact
	for _, line := range unfold(data) {
		name, rawValue := splitProp(line)
		switch name {
		case "BEGIN":
			if strings.EqualFold(rawValue, "VCARD") {
				cur = &Contact{}
			}
		case "END":
			if cur != nil && strings.EqualFold(rawValue, "VCARD") {
				out = append(out, *cur)
				cur = nil
			}
		case "FN":
			if cur != nil {
				cur.Name = unescape(rawValue)
			}
		case "TEL":
			if cur != nil {
				if v := strings.TrimSpace(unescape(rawValue)); v != "" {
					cur.Phones = append(cur.Phones, v)
				}
			}
		case "EMAIL":
			if cur != nil {
				if v := strings.TrimSpace(unescape(rawValue)); v != "" {
					cur.Emails = append(cur.Emails, v)
				}
			}
		case "ORG":
			if cur != nil {
				// ORG is structured (Company;Unit;…) on unescaped ';' — the first
				// component is the company name.
				cur.Org = unescape(strings.SplitN(rawValue, ";", 2)[0])
			}
		case "TITLE":
			if cur != nil {
				cur.Title = unescape(rawValue)
			}
		}
	}
	return out
}

// unfold splits the payload into logical lines, unwrapping RFC 6350 folding
// (a continuation line begins with a space or tab).
func unfold(data string) []string {
	raw := strings.Split(strings.ReplaceAll(data, "\r\n", "\n"), "\n")
	var lines []string
	for _, l := range raw {
		if l == "" {
			continue
		}
		if (l[0] == ' ' || l[0] == '\t') && len(lines) > 0 {
			lines[len(lines)-1] += l[1:]
			continue
		}
		lines = append(lines, l)
	}
	return lines
}

// splitProp splits "GROUP.NAME;params:value" into the uppercased property name
// (group prefix and params removed) and the raw value (params stripped, escaping
// intact).
func splitProp(line string) (name, value string) {
	colon := strings.IndexByte(line, ':')
	if colon < 0 {
		return "", ""
	}
	left, value := line[:colon], line[colon+1:]
	nameTok := left
	if semi := strings.IndexByte(nameTok, ';'); semi >= 0 {
		nameTok = nameTok[:semi]
	}
	if dot := strings.LastIndexByte(nameTok, '.'); dot >= 0 { // strip Apple itemN. group
		nameTok = nameTok[dot+1:]
	}
	return strings.ToUpper(strings.TrimSpace(nameTok)), value
}

// unescape undoes vCard value escaping (\n \, \; \\).
func unescape(s string) string {
	r := strings.NewReplacer(`\n`, "\n", `\N`, "\n", `\,`, ",", `\;`, ";", `\\`, `\`)
	return r.Replace(s)
}
