package agent

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/scottzx/1Agents/backend/internal/meta"
)

func milestoneRequest(t *testing.T, h *Handler, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	rr := httptest.NewRecorder()
	if strings.HasPrefix(path, "/api/agent/milestones?") || path == "/api/agent/milestones" {
		h.HandleMilestonesRoot(rr, req)
	} else {
		h.HandleMilestonesItem(rr, req)
	}
	return rr
}

func TestMilestoneAPICreatesByBumpAndRejectsLegacyCreateShape(t *testing.T) {
	h, _, wsID, _, _ := mutationAttributionRig(t)

	rr := milestoneRequest(t, h, http.MethodPost, "/api/agent/milestones",
		`{"workspace_id":"`+wsID+`","bump":"minor","description":"first"}`)
	if rr.Code != http.StatusOK {
		t.Fatalf("bump create status %d: %s", rr.Code, rr.Body.String())
	}
	var created meta.Milestone
	if err := json.NewDecoder(rr.Body).Decode(&created); err != nil {
		t.Fatalf("decode created milestone: %v", err)
	}
	if created.Version != "0.1.0" || created.Name != created.Version || created.IsLegacy {
		t.Fatalf("created milestone = %+v", created)
	}

	for _, body := range []string{
		`{"workspace_id":"` + wsID + `","name":"legacy-name"}`,
		`{"workspace_id":"` + wsID + `","bump":"minor","name":"custom"}`,
		`{"workspace_id":"` + wsID + `","bump":"minor","predecessorId":"manual"}`,
		`{"workspace_id":"` + wsID + `","bump":"breaking"}`,
	} {
		rr = milestoneRequest(t, h, http.MethodPost, "/api/agent/milestones", body)
		if rr.Code != http.StatusBadRequest {
			t.Errorf("legacy/invalid create %s status = %d, want 400: %s", body, rr.Code, rr.Body.String())
		}
	}

	rr = milestoneRequest(t, h, http.MethodGet,
		"/api/agent/milestones?workspace_id="+wsID, "")
	if rr.Code != http.StatusOK {
		t.Fatalf("list status %d: %s", rr.Code, rr.Body.String())
	}
	var list []meta.Milestone
	if err := json.NewDecoder(rr.Body).Decode(&list); err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 || list[0].Version != "0.1.0" {
		t.Fatalf("rejected create mutated milestones: %+v", list)
	}
}

func TestMilestoneAPIPatchProtectsVersionChain(t *testing.T) {
	h, _, wsID, _, _ := mutationAttributionRig(t)
	create := func(bump string) meta.Milestone {
		t.Helper()
		rr := milestoneRequest(t, h, http.MethodPost, "/api/agent/milestones",
			`{"workspace_id":"`+wsID+`","bump":"`+bump+`"}`)
		if rr.Code != http.StatusOK {
			t.Fatalf("create %s status %d: %s", bump, rr.Code, rr.Body.String())
		}
		var got meta.Milestone
		if err := json.NewDecoder(rr.Body).Decode(&got); err != nil {
			t.Fatal(err)
		}
		return got
	}
	first := create("minor")
	second := create("minor")
	path := "/api/agent/milestones/" + second.ID

	for _, body := range []string{
		`{"workspace_id":"` + wsID + `","version":"9.9.9"}`,
		`{"workspace_id":"` + wsID + `","name":"9.9.9"}`,
		`{"workspace_id":"` + wsID + `","predecessorId":""}`,
	} {
		rr := milestoneRequest(t, h, http.MethodPatch, path, body)
		if rr.Code != http.StatusBadRequest {
			t.Errorf("protected patch %s status = %d, want 400: %s", body, rr.Code, rr.Body.String())
		}
	}

	rr := milestoneRequest(t, h, http.MethodPatch, path,
		`{"workspace_id":"`+wsID+`","description":"editable"}`)
	if rr.Code != http.StatusOK {
		t.Fatalf("allowed patch status %d: %s", rr.Code, rr.Body.String())
	}
	var updated meta.Milestone
	if err := json.NewDecoder(rr.Body).Decode(&updated); err != nil {
		t.Fatal(err)
	}
	if updated.Description != "editable" || updated.Version != "0.2.0" ||
		updated.Name != "0.2.0" || updated.PredecessorID != first.ID {
		t.Fatalf("allowed patch changed protected fields: %+v", updated)
	}
}
