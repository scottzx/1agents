// Package appkit is the thin registration seam between installable apps (业务层 ①)
// and the task kernel (③④). Apps register a runtime init in their package init();
// the server runs all inits once after constructing the North Task API.
//
// Dependency direction: appkit → taskapi only. App packages import appkit (never the
// reverse), so the central apps aggregator can blank-import every app package without
// creating an import cycle.
package appkit

import "github.com/scottzx/1Agents/backend/internal/taskapi"

// Init is a runtime registration hook. It receives the live *taskapi.API so an app can
// RegisterFunction (executor=function handlers), RegisterApp (permission allowlist) and
// RegisterCompletionHook (domain-table writeback). Manifest / domain-table / template
// registration that needs no API instance should run in the app's plain init() instead.
type Init func(api *taskapi.API)

var inits []Init

// OnInit queues a runtime init. Call it from an app package's init().
func OnInit(f Init) {
	inits = append(inits, f)
}

// RunInits runs every queued init against the live API. The server calls this once at
// startup, after taskapi.New and after all app packages have been imported.
func RunInits(api *taskapi.API) {
	for _, f := range inits {
		f(api)
	}
}
