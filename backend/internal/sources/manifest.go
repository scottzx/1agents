package sources

// manifest.go loads declarative connector manifests from ~/.1agents/connectors/*.yaml
// and registers each as a first-class REST source at startup — a thin REST source
// (静态 Bearer + JSON endpoint) is added by dropping a file, no Go recompile. A
// manifest registers three things: a VendorSpec (so 添加数据源 lists it), the REST
// descriptors (so the generic restPuller can crawl it), and — via the caller —
// default per-collection crawl config. Parsing stays free of meta.db; seeding the
// SourceCollectionConfig defaults is done by the ingest layer (which owns meta.db)
// off the returned manifests.

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// Manifest is one connector file. Governance steps (PR4) are a separate section
// parsed by the govern layer; yaml.v3 ignores unknown keys, so a manifest may
// carry a `governance:` block without breaking this loader.
type Manifest struct {
	Vendor       string           `yaml:"vendor"`
	Label        string           `yaml:"label"`
	Region       string           `yaml:"region"`
	MultiAccount bool             `yaml:"multiAccount"`
	AuthKind     string           `yaml:"authKind"`
	BaseURL      string           `yaml:"baseUrl"`
	Collections  []ManifestColl   `yaml:"collections"`
}

// ManifestColl is one crawlable collection in a manifest.
type ManifestColl struct {
	Kind       string            `yaml:"kind"`
	Domain     string            `yaml:"domain"`
	Label      string            `yaml:"label"`
	Method     string            `yaml:"method"`
	Endpoint   string            `yaml:"endpoint"`
	BaseParams map[string]string `yaml:"baseParams"`
	Body       map[string]any    `yaml:"body"`
	Headers    map[string]string `yaml:"headers"`
	Auth       ManifestAuth      `yaml:"auth"`
	SuccessPath string           `yaml:"successPath"`
	ItemPath   string            `yaml:"itemPath"`
	UIDField   string            `yaml:"uidField"`
	Cursor     ManifestCursor    `yaml:"cursor"`
	Defaults   ManifestDefaults  `yaml:"defaults"`
}

type ManifestAuth struct {
	Scheme     string `yaml:"scheme"`
	HeaderName string `yaml:"headerName"`
	Prefix     string `yaml:"prefix"`
}

type ManifestCursor struct {
	Flavor             string `yaml:"flavor"`
	DateParam          string `yaml:"dateParam"`
	DateLayout         string `yaml:"dateLayout"`
	LookbackDays       int    `yaml:"lookbackDays"`
	MinIntervalSeconds int    `yaml:"minIntervalSeconds"`
	TooFrequentPath    string `yaml:"tooFrequentPath"`
}

type ManifestDefaults struct {
	Enabled             bool `yaml:"enabled"`
	IncrementalMinutes  int  `yaml:"incrementalMinutes"`
	PageSize            int  `yaml:"pageSize"`
	InitialLookbackDays int  `yaml:"initialLookbackDays"`
}

// ConnectorsDir is ~/.1agents/connectors (honoring ONEAGENTS_HOME).
func ConnectorsDir() string { return filepath.Join(filepath.Dir(sourcesHome()), "connectors") }

// LoadManifests reads and parses every *.yaml in the connectors dir. A missing
// dir yields no manifests (not an error) — connectors are opt-in.
func LoadManifests() ([]Manifest, error) {
	dir := ConnectorsDir()
	entries, err := os.ReadDir(dir)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var out []Manifest
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if !strings.HasSuffix(name, ".yaml") && !strings.HasSuffix(name, ".yml") {
			continue
		}
		b, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			return nil, err
		}
		var m Manifest
		if err := yaml.Unmarshal(b, &m); err != nil {
			return nil, fmt.Errorf("connector manifest %s: %w", name, err)
		}
		if m.Vendor == "" {
			continue // not a connector manifest
		}
		out = append(out, m)
	}
	return out, nil
}

// RegisterManifest appends the manifest's vendor to Vendors (if new) and registers
// its REST descriptors. Call once per manifest at startup, before RegisterFunctions.
func RegisterManifest(m Manifest) {
	if VendorFor(m.Vendor) == nil {
		Vendors = append(Vendors, VendorSpec{
			Vendor:       m.Vendor,
			Label:        orDefault(m.Label, m.Vendor),
			MultiAccount: m.MultiAccount,
			Regions:      manifestRegions(m.Region),
			AuthKind:     orDefault(m.AuthKind, AuthBearer),
		})
	}
	for _, c := range m.Collections {
		RegisterRESTDescriptor(m.Vendor, m.BaseURL, c.descriptor())
	}
}

// descriptor maps a manifest collection onto a RESTDescriptor.
func (c ManifestColl) descriptor() RESTDescriptor {
	return RESTDescriptor{
		Kind:               c.Kind,
		Domain:             c.Domain,
		Label:              c.Label,
		Method:             c.Method,
		Endpoint:           c.Endpoint,
		BaseParams:         c.BaseParams,
		Body:               c.Body,
		Headers:            c.Headers,
		AuthScheme:         c.Auth.Scheme,
		AuthHeaderName:     c.Auth.HeaderName,
		AuthPrefix:         c.Auth.Prefix,
		SuccessPath:        c.SuccessPath,
		ItemPath:           c.ItemPath,
		UIDField:           c.UIDField,
		CursorFlavor:       c.Cursor.Flavor,
		DateParam:          c.Cursor.DateParam,
		DateLayout:         c.Cursor.DateLayout,
		LookbackDays:       c.Cursor.LookbackDays,
		MinIntervalSeconds: c.Cursor.MinIntervalSeconds,
		TooFrequentPath:    c.Cursor.TooFrequentPath,
	}
}

func manifestRegions(region string) []string {
	if region == "" {
		return []string{RegionIntl}
	}
	return []string{region}
}
