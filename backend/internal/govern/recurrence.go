package govern

import (
	"encoding/json"
	"strconv"
	"strings"

	"github.com/scottzx/1Agents/backend/internal/meta"
)

// recurrence.go normalizes the two external repeat-rule dialects we ingest —
// Microsoft Graph's recurrence object (todos + calendar events) and RFC 5545
// RRULE (飞书 events) — into our canonical meta.Recurrence JSON. The silver
// governors store that JSON in a `recurrence_std` column alongside the raw
// value, so promotion and the scheduler consume one shape and never re-parse a
// vendor format. Empty in → empty out (no recurrence).

// weekdayNum maps a lowercase weekday name (MS Graph) to 0=Sunday…6=Saturday.
var weekdayNum = map[string]int{
	"sunday": 0, "monday": 1, "tuesday": 2, "wednesday": 3,
	"thursday": 4, "friday": 5, "saturday": 6,
}

// rruleWeekdayNum maps an RFC5545 BYDAY code to 0=Sunday…6=Saturday.
var rruleWeekdayNum = map[string]int{
	"SU": 0, "MO": 1, "TU": 2, "WE": 3, "TH": 4, "FR": 5, "SA": 6,
}

// graphIndex maps MS Graph's relativeMonthly/Yearly index to our WeekIndex
// (1..4, or -1 for last).
var graphIndex = map[string]int{
	"first": 1, "second": 2, "third": 3, "fourth": 4, "last": -1,
}

// NormalizeGraphRecurrence turns a raw MS Graph recurrence object into canonical
// meta.Recurrence JSON. Returns "" when raw is empty/unparseable or has no
// pattern type.
func NormalizeGraphRecurrence(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" || raw == "null" {
		return ""
	}
	var g struct {
		Pattern struct {
			Type       string   `json:"type"`
			Interval   int      `json:"interval"`
			DaysOfWeek []string `json:"daysOfWeek"`
			DayOfMonth int      `json:"dayOfMonth"`
			Index      string   `json:"index"`
			Month      int      `json:"month"`
		} `json:"pattern"`
		Range struct {
			Type                string `json:"type"`
			EndDate             string `json:"endDate"`
			NumberOfOccurrences int    `json:"numberOfOccurrences"`
		} `json:"range"`
	}
	if json.Unmarshal([]byte(raw), &g) != nil || g.Pattern.Type == "" {
		return ""
	}
	r := meta.Recurrence{Interval: g.Pattern.Interval}
	var days []int
	for _, d := range g.Pattern.DaysOfWeek {
		if n, ok := weekdayNum[strings.ToLower(strings.TrimSpace(d))]; ok {
			days = append(days, n)
		}
	}
	switch strings.ToLower(g.Pattern.Type) {
	case "daily":
		r.Freq = "daily"
	case "weekly":
		r.Freq = "weekly"
		r.DaysOfWeek = days
	case "absolutemonthly":
		r.Freq = "monthly"
		r.Monthday = g.Pattern.DayOfMonth
	case "relativemonthly":
		r.Freq = "monthly"
		r.WeekIndex = graphIndex[strings.ToLower(g.Pattern.Index)]
		r.DaysOfWeek = days
	case "absoluteyearly":
		r.Freq = "yearly"
		r.Month = g.Pattern.Month
		r.Monthday = g.Pattern.DayOfMonth
	case "relativeyearly":
		r.Freq = "yearly"
		r.Month = g.Pattern.Month
		r.WeekIndex = graphIndex[strings.ToLower(g.Pattern.Index)]
		r.DaysOfWeek = days
	default:
		return ""
	}
	switch strings.ToLower(g.Range.Type) {
	case "enddate":
		r.Until = g.Range.EndDate
	case "numbered":
		r.Count = g.Range.NumberOfOccurrences
	}
	return marshalRecurrence(r)
}

// NormalizeRRULE turns an RFC5545 RRULE string into canonical meta.Recurrence
// JSON. Accepts a bare "FREQ=…" or an "RRULE:FREQ=…" prefixed form. Returns ""
// when empty/unparseable or missing FREQ.
func NormalizeRRULE(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	// A source may hand us multi-line iCalendar; keep only the RRULE line.
	for _, line := range strings.Split(raw, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(strings.ToUpper(line), "RRULE") {
			raw = line
			break
		}
	}
	raw = strings.TrimPrefix(raw, "RRULE:")
	raw = strings.TrimPrefix(raw, "rrule:")

	parts := map[string]string{}
	for _, kv := range strings.Split(raw, ";") {
		if i := strings.IndexByte(kv, '='); i > 0 {
			parts[strings.ToUpper(strings.TrimSpace(kv[:i]))] = strings.TrimSpace(kv[i+1:])
		}
	}
	freq := strings.ToUpper(parts["FREQ"])
	if freq == "" {
		return ""
	}
	r := meta.Recurrence{}
	if v, err := strconv.Atoi(parts["INTERVAL"]); err == nil {
		r.Interval = v
	}
	var days []int
	var weekIndex int
	for _, code := range strings.Split(parts["BYDAY"], ",") {
		code = strings.ToUpper(strings.TrimSpace(code))
		if code == "" {
			continue
		}
		// BYDAY entries may carry an ordinal prefix, e.g. "1MO" / "-1FR".
		num := strings.TrimRight(code, "SUMOTUWEHFRA")
		name := code[len(num):]
		if n, ok := rruleWeekdayNum[name]; ok {
			days = append(days, n)
			if num != "" && num != "+" && num != "-" {
				if idx, err := strconv.Atoi(num); err == nil {
					weekIndex = idx
				}
			}
		}
	}
	switch freq {
	case "DAILY":
		r.Freq = "daily"
	case "WEEKLY":
		r.Freq = "weekly"
		r.DaysOfWeek = days
	case "MONTHLY":
		r.Freq = "monthly"
		if v, err := strconv.Atoi(parts["BYMONTHDAY"]); err == nil {
			r.Monthday = v
		}
		if weekIndex != 0 {
			r.WeekIndex = weekIndex
			r.DaysOfWeek = days
		}
	case "YEARLY":
		r.Freq = "yearly"
		if v, err := strconv.Atoi(parts["BYMONTH"]); err == nil {
			r.Month = v
		}
		if v, err := strconv.Atoi(parts["BYMONTHDAY"]); err == nil {
			r.Monthday = v
		}
		if weekIndex != 0 {
			r.WeekIndex = weekIndex
			r.DaysOfWeek = days
		}
	default:
		return ""
	}
	if v, err := strconv.Atoi(parts["COUNT"]); err == nil {
		r.Count = v
	}
	if until := parts["UNTIL"]; until != "" {
		r.Until = normalizeRRULEUntil(until)
	}
	return marshalRecurrence(r)
}

// normalizeRRULEUntil converts an RRULE UNTIL value (e.g. "20261231T000000Z" or
// "20261231") to our "2006-01-02" date form; returns the raw value if it doesn't
// match a known layout.
func normalizeRRULEUntil(s string) string {
	digits := s
	if i := strings.IndexAny(digits, "Tt"); i >= 0 {
		digits = digits[:i]
	}
	if len(digits) == 8 {
		return digits[:4] + "-" + digits[4:6] + "-" + digits[6:8]
	}
	return s
}

func marshalRecurrence(r meta.Recurrence) string {
	if r.Freq == "" {
		return ""
	}
	data, err := json.Marshal(r)
	if err != nil {
		return ""
	}
	return string(data)
}
