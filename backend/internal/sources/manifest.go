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
	"regexp"
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
	Governance   []ManifestStep   `yaml:"governance"`
}

// ManifestStep is one declarative silver→gold SQL governance step: it reads N
// upstream tables (any join, all in data.db) and writes/upserts an output table.
// The generic form of the built-in Go gold governors, but pure config.
type ManifestStep struct {
	Name        string           `yaml:"name"`
	Upstreams   []string         `yaml:"upstreams"`
	Output      string           `yaml:"output"`
	Domain      string           `yaml:"domain"`
	Source      string           `yaml:"source"` // viewer source tag ("" ⇒ owning vendor / manifest name)
	CreateSQL   string           `yaml:"createSQL"`
	Body        string           `yaml:"body"`
	Incremental ManifestStepIncr `yaml:"incremental"`
	// Script fields — when Script is set the step runs an external Python transform
	// (InputSQL selects rows → script → upsert into Output on Conflict) instead of Body.
	Script      string   `yaml:"script"`      // path (relative to the connectors dir, or absolute)
	Interpreter string   `yaml:"interpreter"` // default "python3"
	InputSQL    string   `yaml:"inputSQL"`    // SELECT ... WHERE <column> > :since
	Conflict    []string `yaml:"conflict"`    // ON CONFLICT(...) columns for the upsert
}

// ManifestStepIncr names the driving upstream table + watermark column for a step's
// incremental cursor (body filters WHERE <column> > :since; cursor advances to MAX).
type ManifestStepIncr struct {
	Table  string `yaml:"table"`
	Column string `yaml:"column"`
}

// ManifestColl is one crawlable collection in a manifest.
type ManifestColl struct {
	Kind    string `yaml:"kind"`
	Domain  string `yaml:"domain"`
	Label   string `yaml:"label"`
	// Transport: "" | "rest" → HTTP; "cli" → shell out to Command with Args.
	Transport   string            `yaml:"transport"`
	Command     string            `yaml:"command"` // cli: binary (e.g. agently-cli)
	Args        []string          `yaml:"args"`    // cli: static args
	Method      string            `yaml:"method"`
	Endpoint    string            `yaml:"endpoint"`
	BaseParams  map[string]string `yaml:"baseParams"`
	Body        map[string]any    `yaml:"body"`
	Headers     map[string]string `yaml:"headers"`
	Auth        ManifestAuth      `yaml:"auth"`
	SuccessPath string            `yaml:"successPath"`
	ItemPath    string            `yaml:"itemPath"`
	UIDField    string            `yaml:"uidField"`
	Cursor      ManifestCursor    `yaml:"cursor"`
	Defaults    ManifestDefaults  `yaml:"defaults"`
	Silver      ManifestSilver    `yaml:"silver"`
}

// ManifestSilver declares the generic bronze→silver landing for a collection: the
// target table + viewer domain, and which payload JSON paths to promote to their
// own columns (the full payload is always stored). Optional — omit to skip silver.
type ManifestSilver struct {
	Table   string            `yaml:"table"`
	Domain  string            `yaml:"domain"`
	Promote map[string]string `yaml:"promote"`
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
	Arg                string `yaml:"arg"`       // cli timestamp: flag carrying the watermark (e.g. --after)
	TimeField          string `yaml:"timeField"` // cli timestamp: per-item watermark field (e.g. created_at)
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
		cliTool, isCLI := m.cliTool()
		authKind := m.AuthKind
		if authKind == "" {
			authKind = AuthBearer
			if isCLI {
				authKind = AuthCLI // a CLI transport manages its own credential
			}
		}
		appendVendor(VendorSpec{
			Vendor:       m.Vendor,
			Label:        orDefault(m.Label, m.Vendor),
			MultiAccount: m.MultiAccount,
			Regions:      manifestRegions(m.Region),
			AuthKind:     authKind,
			CliTool:      cliTool,
		})
	}
	for _, c := range m.Collections {
		RegisterRESTDescriptor(m.Vendor, m.BaseURL, c.descriptor())
	}
}

// cliTool returns the CLI binary a manifest's collections shell out to (first one
// found) and whether any collection uses the cli transport.
func (m Manifest) cliTool() (string, bool) {
	for _, c := range m.Collections {
		if c.Transport == "cli" {
			return c.Command, true
		}
	}
	return "", false
}

// descriptor maps a manifest collection onto a RESTDescriptor.
func (c ManifestColl) descriptor() RESTDescriptor {
	return RESTDescriptor{
		Kind:               c.Kind,
		Domain:             c.Domain,
		Label:              c.Label,
		Transport:          c.Transport,
		Command:            c.Command,
		Args:               c.Args,
		CursorArg:          c.Cursor.Arg,
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
		TimeItemField:      c.Cursor.TimeField,
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

// GovernanceManifest is a standalone governance declaration — decoupled from any
// connector. It expresses the 集成/治理解耦 principle at the config layer: a DAG of
// steps that read ANY data.db table (built-in silver/gold + connector silver) and
// write cross-source entity tables. Loaded from ~/.1agents/governance/*.yaml.
type GovernanceManifest struct {
	Name  string         `yaml:"name"`
	Steps []ManifestStep `yaml:"steps"`
}

// GovernanceDir is ~/.1agents/governance (honoring ONEAGENTS_HOME).
func GovernanceDir() string { return filepath.Join(filepath.Dir(sourcesHome()), "governance") }

// LoadGovernanceManifests reads every *.yaml in the governance dir. A missing dir
// yields none (not an error). A file with no name/steps is skipped.
func LoadGovernanceManifests() ([]GovernanceManifest, error) {
	dir := GovernanceDir()
	entries, err := os.ReadDir(dir)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var out []GovernanceManifest
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
		var gm GovernanceManifest
		if err := yaml.Unmarshal(b, &gm); err != nil {
			return nil, fmt.Errorf("governance manifest %s: %w", name, err)
		}
		if gm.Name == "" || len(gm.Steps) == 0 {
			continue
		}
		out = append(out, gm)
	}
	return out, nil
}

// vendorNameRe restricts a vendor name to a safe filename + bronze discriminator.
var vendorNameRe = regexp.MustCompile(`^[a-z0-9_-]{1,40}$`)

// ParseManifest unmarshals raw YAML into a Manifest.
func ParseManifest(b []byte) (Manifest, error) {
	var m Manifest
	if err := yaml.Unmarshal(b, &m); err != nil {
		return m, fmt.Errorf("parse manifest: %w", err)
	}
	return m, nil
}

// ValidateManifest checks a manifest is safe to persist + register: a filename-safe
// vendor, a base URL, and at least one collection with a kind + endpoint.
func ValidateManifest(m Manifest) error {
	if !vendorNameRe.MatchString(m.Vendor) {
		return fmt.Errorf("vendor must match %s (got %q)", vendorNameRe, m.Vendor)
	}
	if len(m.Collections) == 0 {
		return fmt.Errorf("at least one collection required")
	}
	for i, c := range m.Collections {
		if strings.TrimSpace(c.Kind) == "" {
			return fmt.Errorf("collection[%d]: kind required", i)
		}
		if c.Transport == "cli" {
			// CLI transport: needs a command; baseUrl/endpoint are irrelevant.
			if strings.TrimSpace(c.Command) == "" {
				return fmt.Errorf("collection[%d] %q: command required for transport=cli", i, c.Kind)
			}
			continue
		}
		// REST transport (default): needs a base URL + an endpoint.
		if strings.TrimSpace(m.BaseURL) == "" {
			return fmt.Errorf("baseUrl required for REST collections")
		}
		if strings.TrimSpace(c.Endpoint) == "" {
			return fmt.Errorf("collection[%d] %q: endpoint required", i, c.Kind)
		}
	}
	return nil
}

// SaveManifest writes the raw manifest YAML to ~/.1agents/connectors/<vendor>.yaml
// (mode 0644, dir 0755). vendor must already be validated (filename-safe).
func SaveManifest(vendor string, b []byte) error {
	if !vendorNameRe.MatchString(vendor) {
		return fmt.Errorf("unsafe vendor name %q", vendor)
	}
	dir := ConnectorsDir()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, vendor+".yaml"), b, 0o644)
}
