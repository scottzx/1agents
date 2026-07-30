package agent

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/scottzx/1Agents/backend/internal/meta"
)

func featureRequest(method, path, body string) *http.Request {
	var reader *strings.Reader
	if body != "" {
		reader = strings.NewReader(body)
	} else {
		reader = strings.NewReader("")
	}
	return httptest.NewRequest(method, path, reader)
}

func TestFeatureCatalogHandlerRoundTripAndFailures(t *testing.T) {
	h, db, workspaceID, workspacePath, _ := mutationAttributionRig(t)

	postNode := func(body string) meta.FeatureNode {
		t.Helper()
		rr := httptest.NewRecorder()
		h.HandleFeatureCatalogRoot(rr, featureRequest(http.MethodPost, "/api/agent/feature-catalog", body))
		if rr.Code != http.StatusOK {
			t.Fatalf("create feature node status=%d body=%s", rr.Code, rr.Body.String())
		}
		var node meta.FeatureNode
		if err := json.NewDecoder(rr.Body).Decode(&node); err != nil {
			t.Fatal(err)
		}
		if node.ID == "" {
			t.Fatal("create returned empty node id")
		}
		return node
	}

	root := postNode(`{"workspace_id":"` + workspaceID + `","kind":"module","title":"Root"}`)
	feature := postNode(`{"workspace_id":"` + workspaceID + `","parentId":"` + root.ID + `","kind":"feature","title":"Feature"}`)

	rr := httptest.NewRecorder()
	h.HandleFeatureCatalogRoot(
		rr,
		featureRequest(http.MethodGet, "/api/agent/feature-catalog?workspace_id="+workspaceID, ""),
	)
	if rr.Code != http.StatusOK {
		t.Fatalf("list status=%d body=%s", rr.Code, rr.Body.String())
	}
	var catalog meta.FeatureCatalog
	if err := json.NewDecoder(rr.Body).Decode(&catalog); err != nil {
		t.Fatal(err)
	}
	if len(catalog.Nodes) != 2 || len(catalog.Links) != 0 {
		t.Fatalf("catalog = %+v", catalog)
	}

	rr = httptest.NewRecorder()
	h.HandleFeatureCatalogItem(
		rr,
		featureRequest(
			http.MethodDelete,
			"/api/agent/feature-catalog/"+root.ID+"?workspace_id="+workspaceID,
			"",
		),
	)
	if rr.Code != http.StatusConflict || !strings.Contains(rr.Body.String(), "has_children") {
		t.Fatalf("delete parent status=%d body=%s", rr.Code, rr.Body.String())
	}

	rr = httptest.NewRecorder()
	h.HandleFeatureCatalogItem(
		rr,
		featureRequest(
			http.MethodPatch,
			"/api/agent/feature-catalog/"+root.ID,
			`{"workspace_id":"`+workspaceID+`","parentId":"`+feature.ID+`"}`,
		),
	)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("cycle patch status=%d body=%s", rr.Code, rr.Body.String())
	}

	now := time.Now().UTC()
	if err := meta.NewTaskStore(db).Mutate(workspacePath, func(cfg *meta.TasksConfig) bool {
		cfg.Tasks = append(cfg.Tasks, meta.ProjectItem{
			ID: "delivery", Title: "Delivery", Type: meta.ItemTypeTask,
			Status: meta.TaskStatusPending, CreatedAt: now, UpdatedAt: now,
		})
		return true
	}); err != nil {
		t.Fatal(err)
	}

	linkBody := `{"workspace_id":"` + workspaceID + `","itemId":"delivery","relation":"delivery"}`
	for i := 0; i < 2; i++ {
		rr = httptest.NewRecorder()
		h.HandleFeatureCatalogItem(
			rr,
			featureRequest(http.MethodPost, "/api/agent/feature-catalog/"+feature.ID+"/items", linkBody),
		)
		if rr.Code != http.StatusOK {
			t.Fatalf("link attempt %d status=%d body=%s", i+1, rr.Code, rr.Body.String())
		}
	}

	rr = httptest.NewRecorder()
	h.HandleFeatureCatalogItem(
		rr,
		featureRequest(
			http.MethodPost,
			"/api/agent/feature-catalog/"+feature.ID+"/items",
			`{"workspace_id":"`+workspaceID+`","itemId":"delivery","relation":"source"}`,
		),
	)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("wrong link type status=%d body=%s", rr.Code, rr.Body.String())
	}

	rr = httptest.NewRecorder()
	h.HandleFeatureCatalogItem(
		rr,
		featureRequest(
			http.MethodDelete,
			"/api/agent/feature-catalog/"+feature.ID+"/items/delivery?workspace_id="+workspaceID+"&relation=delivery",
			"",
		),
	)
	if rr.Code != http.StatusOK {
		t.Fatalf("unlink status=%d body=%s", rr.Code, rr.Body.String())
	}

	rr = httptest.NewRecorder()
	h.HandleFeatureCatalogItem(
		rr,
		featureRequest(
			http.MethodDelete,
			"/api/agent/feature-catalog/"+feature.ID+"?workspace_id="+workspaceID,
			"",
		),
	)
	if rr.Code != http.StatusOK {
		t.Fatalf("delete feature status=%d body=%s", rr.Code, rr.Body.String())
	}
	if _, ok, err := meta.NewTaskStore(db).GetTask("delivery"); err != nil || !ok {
		t.Fatalf("deleting feature removed delivery task: ok=%v err=%v", ok, err)
	}
}

func TestFeatureCatalogVersionHandlerRoutesBeforeFeatureNodeIDs(t *testing.T) {
	h, _, workspaceID, _, _ := mutationAttributionRig(t)

	rr := httptest.NewRecorder()
	h.HandleFeatureCatalogItem(
		rr,
		featureRequest(
			http.MethodPost,
			"/api/agent/feature-catalog/versions",
			`{"workspace_id":"`+workspaceID+`","alias":" baseline "}`,
		),
	)
	if rr.Code != http.StatusOK {
		t.Fatalf("create version status=%d body=%s", rr.Code, rr.Body.String())
	}
	var version meta.FeatureCatalogVersion
	if err := json.NewDecoder(rr.Body).Decode(&version); err != nil {
		t.Fatal(err)
	}
	if version.ID == "" || version.Alias != "baseline" {
		t.Fatalf("created version = %+v", version)
	}

	rr = httptest.NewRecorder()
	h.HandleFeatureCatalogItem(
		rr,
		featureRequest(
			http.MethodGet,
			"/api/agent/feature-catalog/versions?workspace_id="+workspaceID,
			"",
		),
	)
	if rr.Code != http.StatusOK {
		t.Fatalf("list versions status=%d body=%s", rr.Code, rr.Body.String())
	}
	var page meta.FeatureCatalogVersionPage
	if err := json.NewDecoder(rr.Body).Decode(&page); err != nil {
		t.Fatal(err)
	}
	if len(page.Items) != 1 || page.Items[0].ID != version.ID {
		t.Fatalf("version page = %+v", page)
	}

	rr = httptest.NewRecorder()
	h.HandleFeatureCatalogItem(
		rr,
		featureRequest(
			http.MethodPatch,
			"/api/agent/feature-catalog/versions/"+version.ID,
			`{"workspace_id":"`+workspaceID+`","alias":"renamed"}`,
		),
	)
	if rr.Code != http.StatusOK {
		t.Fatalf("rename version status=%d body=%s", rr.Code, rr.Body.String())
	}

	rr = httptest.NewRecorder()
	h.HandleFeatureCatalogItem(
		rr,
		featureRequest(
			http.MethodPost,
			"/api/agent/feature-catalog/versions/"+version.ID+"/restore",
			`{"workspace_id":"`+workspaceID+`","requestId":"http-restore"}`,
		),
	)
	if rr.Code != http.StatusOK {
		t.Fatalf("restore version status=%d body=%s", rr.Code, rr.Body.String())
	}
	var result meta.FeatureCatalogRestoreResult
	if err := json.NewDecoder(rr.Body).Decode(&result); err != nil {
		t.Fatal(err)
	}
	if result.RequestID != "http-restore" || result.SafetyVersion.ID == "" {
		t.Fatalf("restore result = %+v", result)
	}

	rr = httptest.NewRecorder()
	h.HandleFeatureCatalogItem(
		rr,
		featureRequest(
			http.MethodDelete,
			"/api/agent/feature-catalog/versions/"+version.ID+"?workspace_id="+workspaceID,
			"",
		),
	)
	if rr.Code != http.StatusOK {
		t.Fatalf("delete version status=%d body=%s", rr.Code, rr.Body.String())
	}
}

func TestFeatureTargetMilestoneDiffSyncAndInheritedTaskCreation(t *testing.T) {
	h, db, workspaceID, workspacePath, _ := mutationAttributionRig(t)
	taskStore := meta.NewTaskStore(db)
	target, err := taskStore.CreateVersionMilestone(
		workspacePath, meta.MilestoneBumpMinor, "", nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	legacy, err := taskStore.CreateMilestone(workspacePath, "legacy", "", nil, "")
	if err != nil {
		t.Fatal(err)
	}
	otherPath := t.TempDir()
	if err := db.EnsureProject("project-2", "Project 2", otherPath); err != nil {
		t.Fatal(err)
	}
	otherTarget, err := taskStore.CreateVersionMilestone(
		otherPath, meta.MilestoneBumpMajor, "", nil,
	)
	if err != nil {
		t.Fatal(err)
	}

	postNode := func(body string) meta.FeatureNode {
		t.Helper()
		rr := httptest.NewRecorder()
		h.HandleFeatureCatalogRoot(rr, featureRequest(http.MethodPost, "/api/agent/feature-catalog", body))
		if rr.Code != http.StatusOK {
			t.Fatalf("create feature node status=%d body=%s", rr.Code, rr.Body.String())
		}
		var node meta.FeatureNode
		if err := json.NewDecoder(rr.Body).Decode(&node); err != nil {
			t.Fatal(err)
		}
		return node
	}
	root := postNode(`{"workspace_id":"` + workspaceID + `","kind":"module","title":"Root"}`)
	feature := postNode(
		`{"workspace_id":"` + workspaceID + `","parentId":"` + root.ID +
			`","kind":"feature","title":"Feature","targetMilestoneId":"` + target.ID + `"}`,
	)

	rr := httptest.NewRecorder()
	h.HandleFeatureCatalogItem(
		rr,
		featureRequest(
			http.MethodPatch,
			"/api/agent/feature-catalog/"+feature.ID,
			`{"workspace_id":"`+workspaceID+`","targetMilestoneId":"`+legacy.ID+`"}`,
		),
	)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("legacy target status=%d body=%s", rr.Code, rr.Body.String())
	}
	rr = httptest.NewRecorder()
	h.HandleFeatureCatalogItem(
		rr,
		featureRequest(
			http.MethodPatch,
			"/api/agent/feature-catalog/"+feature.ID,
			`{"workspace_id":"`+workspaceID+`","targetMilestoneId":"`+otherTarget.ID+`"}`,
		),
	)
	if rr.Code != http.StatusForbidden {
		t.Fatalf("cross-project target status=%d body=%s", rr.Code, rr.Body.String())
	}

	now := time.Now().UTC()
	if err := taskStore.Mutate(workspacePath, func(cfg *meta.TasksConfig) bool {
		cfg.Tasks = append(cfg.Tasks, meta.ProjectItem{
			ID: "existing", Title: "Existing", Type: meta.ItemTypeTask,
			Status: meta.TaskStatusPending, Milestone: "",
			CreatedAt: now, UpdatedAt: now,
		})
		return true
	}); err != nil {
		t.Fatal(err)
	}
	if _, _, err := h.featureStore.LinkItem(
		workspaceID,
		feature.ID,
		"existing",
		meta.FeatureItemDelivery,
		meta.ProjectEvent{
			ActorKind: "user",
			ActorName: "user",
			Origin:    "http",
			Status:    meta.ProjectEventSucceeded,
		},
	); err != nil {
		t.Fatal(err)
	}

	rr = httptest.NewRecorder()
	h.HandleFeatureCatalogItem(
		rr,
		featureRequest(
			http.MethodGet,
			"/api/agent/feature-catalog/"+feature.ID+"/milestone-diff?workspace_id="+workspaceID,
			"",
		),
	)
	if rr.Code != http.StatusOK {
		t.Fatalf("diff status=%d body=%s", rr.Code, rr.Body.String())
	}
	var preview meta.FeatureMilestoneSyncPreview
	if err := json.NewDecoder(rr.Body).Decode(&preview); err != nil {
		t.Fatal(err)
	}
	if preview.TargetVersion != target.Version || len(preview.Tasks) != 1 ||
		preview.Tasks[0].ID != "existing" {
		t.Fatalf("diff preview = %+v", preview)
	}
	existing, _, _ := taskStore.GetTask("existing")
	if existing.Milestone != "" {
		t.Fatalf("diff silently updated existing task to %q", existing.Milestone)
	}

	rr = httptest.NewRecorder()
	h.HandleFeatureCatalogItem(
		rr,
		featureRequest(
			http.MethodPost,
			"/api/agent/feature-catalog/"+feature.ID+"/sync-milestone",
			`{"workspace_id":"`+workspaceID+`"}`,
		),
	)
	if rr.Code != http.StatusOK {
		t.Fatalf("sync status=%d body=%s", rr.Code, rr.Body.String())
	}
	existing, _, _ = taskStore.GetTask("existing")
	if existing.Milestone != target.Name {
		t.Fatalf("synced milestone = %q, want %q", existing.Milestone, target.Name)
	}

	rr = httptest.NewRecorder()
	h.HandleTasksRoot(
		rr,
		featureRequest(
			http.MethodPost,
			"/api/agent/project-items",
			`{"workspace_id":"`+workspaceID+`","title":"Inherited","type":"task","featureId":"`+feature.ID+`"}`,
		),
	)
	if rr.Code != http.StatusOK {
		t.Fatalf("create from feature status=%d body=%s", rr.Code, rr.Body.String())
	}
	var created meta.ProjectItem
	if err := json.NewDecoder(rr.Body).Decode(&created); err != nil {
		t.Fatal(err)
	}
	if created.Milestone != target.Name {
		t.Fatalf("created milestone = %q, want %q", created.Milestone, target.Name)
	}
	catalog, err := h.featureStore.List(workspaceID)
	if err != nil {
		t.Fatal(err)
	}
	var linked bool
	for _, link := range catalog.Links {
		if link.FeatureID == feature.ID && link.ItemID == created.ID &&
			link.Relation == meta.FeatureItemDelivery {
			linked = true
		}
	}
	if !linked {
		t.Fatal("created task did not receive delivery link")
	}
}

func TestFeatureCatalogBatchAPIResolvesReferencesAndRollsBack(t *testing.T) {
	h, _, workspaceID, _, _ := mutationAttributionRig(t)
	callBatch := func(body string) *httptest.ResponseRecorder {
		t.Helper()
		rr := httptest.NewRecorder()
		h.HandleFeatureCatalogItem(
			rr,
			featureRequest(http.MethodPost, "/api/agent/feature-catalog/batch", body),
		)
		return rr
	}

	rr := callBatch(`{
		"workspace_id":"` + workspaceID + `",
		"operations":[
			{"op":"create","clientRef":"root","kind":"module","title":"Root"},
			{"op":"create","clientRef":"child","parentRef":"root","kind":"module","title":"Child"},
			{"op":"create","clientRef":"point","parentRef":"child","kind":"feature","title":"Point"}
		]
	}`)
	if rr.Code != http.StatusOK {
		t.Fatalf("batch success status=%d body=%s", rr.Code, rr.Body.String())
	}
	var response struct {
		Results []meta.FeatureCatalogBatchResult `json:"results"`
	}
	if err := json.NewDecoder(rr.Body).Decode(&response); err != nil {
		t.Fatal(err)
	}
	if len(response.Results) != 3 || response.Results[0].Node == nil ||
		response.Results[1].Node == nil || response.Results[2].Node == nil {
		t.Fatalf("batch response = %+v", response)
	}
	if response.Results[1].Node.ParentID != response.Results[0].Node.ID ||
		response.Results[2].Node.ParentID != response.Results[1].Node.ID {
		t.Fatalf("batch parent references = %+v", response.Results)
	}

	before, err := h.featureStore.List(workspaceID)
	if err != nil {
		t.Fatal(err)
	}
	rr = callBatch(`{
		"workspace_id":"` + workspaceID + `",
		"operations":[
			{"op":"create","clientRef":"rollback","kind":"module","title":"Must Roll Back"},
			{"op":"create","parentRef":"missing","kind":"feature","title":"Invalid"}
		]
	}`)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("batch failure status=%d body=%s", rr.Code, rr.Body.String())
	}
	after, err := h.featureStore.List(workspaceID)
	if err != nil {
		t.Fatal(err)
	}
	if len(after.Nodes) != len(before.Nodes) || len(after.Links) != len(before.Links) {
		t.Fatalf("failed REST batch was not atomic: before=%+v after=%+v", before, after)
	}
}
