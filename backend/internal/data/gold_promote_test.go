package data

import (
	"encoding/json"
	"testing"
)

func TestBuildPromotionBodySuperset(t *testing.T) {
	// A silver to-do row (raw column map) carrying every superset field.
	m := map[string]string{
		"title":           "交周报",
		"body":            "把本周进展写清楚",
		"importance":      "high",
		"categories":      `["工作","重要"]`,
		"due_at":          "1781740800000", // epoch ms
		"recurrence_std":  `{"freq":"weekly","daysOfWeek":[6,0]}`,
		"checklist_items": `[{"displayName":"起草","isChecked":true},{"displayName":"复核","isChecked":false}]`,
	}
	body := buildPromotionBody(m)

	if body["title"] != "交周报" || body["description"] != "把本周进展写清楚" {
		t.Fatalf("title/description not carried: %+v", body)
	}
	if body["priority"] != "high" {
		t.Fatalf("priority = %v, want high", body["priority"])
	}
	labels, _ := body["labels"].([]string)
	if len(labels) != 2 || labels[0] != "工作" {
		t.Fatalf("labels = %v, want [工作 重要]", body["labels"])
	}
	if body["scheduleType"] != "scheduled" || body["scheduledAt"] == "" {
		t.Fatalf("due not mapped to scheduled: %+v", body)
	}
	// recurrence carried as raw JSON of the canonical shape.
	rec, ok := body["recurrence"].(json.RawMessage)
	if !ok || string(rec) != `{"freq":"weekly","daysOfWeek":[6,0]}` {
		t.Fatalf("recurrence = %v, want canonical JSON", body["recurrence"])
	}
	// checklist mapped displayName→text, isChecked→done.
	cl, ok := body["checklist"].([]map[string]any)
	if !ok || len(cl) != 2 {
		t.Fatalf("checklist = %v, want 2 items", body["checklist"])
	}
	if cl[0]["text"] != "起草" || cl[0]["done"] != true {
		t.Fatalf("checklist[0] = %v, want {起草 true}", cl[0])
	}
	if cl[1]["done"] != false {
		t.Fatalf("checklist[1].done = %v, want false", cl[1]["done"])
	}
}

func TestBuildPromotionBodyMinimal(t *testing.T) {
	body := buildPromotionBody(map[string]string{"title": "买牛奶"})
	if body["title"] != "买牛奶" || body["priority"] != "medium" {
		t.Fatalf("minimal body wrong: %+v", body)
	}
	if _, ok := body["scheduledAt"]; ok {
		t.Fatalf("no due → no scheduledAt, got %+v", body)
	}
	if _, ok := body["recurrence"]; ok {
		t.Fatalf("no recurrence expected, got %+v", body)
	}
	if _, ok := body["checklist"]; ok {
		t.Fatalf("no checklist expected, got %+v", body)
	}
}
