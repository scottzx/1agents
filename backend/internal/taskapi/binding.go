package taskapi

import (
	"github.com/scottzx/1Agents/backend/internal/meta"
)

// IssueTasksFromBusiness is the forward binding seam: creates one or more tasks
// derived from a business object/stage and returns their IDs. This is the
// canonical entry point for Wave 3 apps to produce tasks from domain events.
//
// ref is the business object identifier (e.g. "crm:lead:42").
// stage is an optional sub-stage label appended to each spec's Milestone.
// specs are the tasks to create; their BusinessRef is set to ref automatically.
//
// Example (Wave 3 CRM app):
//
//	ids, err := api.IssueTasksFromBusiness("crm:lead:42", "enrich", []DispatchSpec{
//	    {Title: "富集线索 42", Executor: meta.TaskExecutorAgent, WorkspacePath: ws},
//	})
func (a *API) IssueTasksFromBusiness(namespace, ref, stage string, specs []DispatchSpec) ([]string, error) {
	ids := make([]string, 0, len(specs))
	for _, s := range specs {
		s.BusinessRef = ref
		if stage != "" && s.Milestone == "" {
			s.Milestone = stage
		}
		id, err := a.DispatchTask(namespace, s)
		if err != nil {
			return ids, err
		}
		ids = append(ids, id)
	}
	return ids, nil
}

// ListTasksForBusiness is the reverse binding seam: returns all tasks whose
// business_ref == ref, for business UI to show inline execution state. The
// caller can filter further (by status, executor, etc.) from the returned slice.
func (a *API) ListTasksForBusiness(ref string) ([]meta.Task, error) {
	return a.store.ListTasksByBusinessRef(ref)
}
