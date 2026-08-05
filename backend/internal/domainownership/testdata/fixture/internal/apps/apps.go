// Package apps is the central aggregator: it blank-imports every domain
// application root so their init() registration runs. It is the ONLY
// internal package allowed to import app root packages.
package apps

import (
	// Aggregator blank imports: legal by design.
	_ "github.com/scottzx/1Agents/backend/internal/apps/alpha"
	_ "github.com/scottzx/1Agents/backend/internal/apps/beta"
)
