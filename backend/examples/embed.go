// Package examples embeds the built-in connector + governance templates into the
// binary, so a user can install a working example (训记 REST connector, 跨源联系人
// 并集 governance DAG) from the UI with one click — no file to author, no path to
// know. Installing copies the template (manifest + any referenced scripts) into
// ~/.1agents/{connectors,governance}/ and hot-registers it, exactly as a
// hand-dropped file would be loaded at startup.
package examples

import "embed"

//go:embed connectors governance
var FS embed.FS
