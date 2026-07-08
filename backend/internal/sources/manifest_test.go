package sources

import (
	"os"
	"path/filepath"
	"testing"
)

// xunjiYAML mirrors the real 训记 Open API shape (POST /api_trains_for_llm_v2,
// Bearer auth, body {schema_version, datestr, include_full_data}, items at
// res.trains keyed by localid) — the canonical thin-REST connector.
const xunjiYAML = `
vendor: xunji
label: 训记
region: cn
multiAccount: false
authKind: bearer
baseUrl: https://trains.xunjiapp.cn
collections:
  - kind: xunji_train
    domain: fitness
    label: 训练记录
    method: POST
    endpoint: /api_trains_for_llm_v2
    body:
      schema_version: train_open_api_v2
      include_full_data: false
    auth:
      scheme: bearer
      headerName: Authorization
      prefix: "Bearer "
    successPath: success
    itemPath: res.trains
    uidField: localid
    cursor:
      flavor: date-window
      dateParam: datestr
      dateLayout: "2006-01-02"
      lookbackDays: 30
      minIntervalSeconds: 2
    defaults:
      enabled: true
      incrementalMinutes: 720
      pageSize: 0
      initialLookbackDays: 30
`

func TestLoadAndRegisterManifest_Xunji(t *testing.T) {
	home := t.TempDir()
	t.Setenv("ONEAGENTS_HOME", home)
	dir := ConnectorsDir()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "xunji.yaml"), []byte(xunjiYAML), 0o644); err != nil {
		t.Fatal(err)
	}

	ms, err := LoadManifests()
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if len(ms) != 1 || ms[0].Vendor != "xunji" {
		t.Fatalf("manifests = %+v", ms)
	}
	m := ms[0]
	if m.AuthKind != "bearer" || m.BaseURL != "https://trains.xunjiapp.cn" {
		t.Fatalf("manifest header parse: %+v", m)
	}
	if len(m.Collections) != 1 {
		t.Fatalf("collections = %d", len(m.Collections))
	}
	c := m.Collections[0]
	// Typed body value must decode as a bool, not the string "false".
	if v, ok := c.Body["include_full_data"].(bool); !ok || v {
		t.Fatalf("include_full_data = %#v", c.Body["include_full_data"])
	}
	if c.Cursor.Flavor != "date-window" || c.Cursor.DateParam != "datestr" || c.Cursor.LookbackDays != 30 {
		t.Fatalf("cursor parse: %+v", c.Cursor)
	}
	if !c.Defaults.Enabled || c.Defaults.IncrementalMinutes != 720 {
		t.Fatalf("defaults parse: %+v", c.Defaults)
	}

	RegisterManifest(m)
	if VendorFor("xunji") == nil {
		t.Fatal("vendor not registered")
	}
	d, ok := RESTDescriptorFor("xunji", "xunji_train")
	if !ok || d.Method != "POST" || d.ItemPath != "res.trains" || d.UIDField != "localid" {
		t.Fatalf("descriptor = %+v ok=%v", d, ok)
	}
	if b, ok := RESTBaseURL("xunji"); !ok || b != "https://trains.xunjiapp.cn" {
		t.Fatalf("base url = %q", b)
	}
	// Surfaces through CatalogFor like a built-in source.
	if CatalogItemFor("xunji", "xunji_train") == nil {
		t.Fatal("CatalogItemFor should resolve manifest kind")
	}
}
