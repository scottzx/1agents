package provider

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"

	"github.com/BurntSushi/toml"
	_ "modernc.org/sqlite"
)

const ccSwitchMigration = "cc-switch-sqlite-v1"

var errUnsupportedCCSwitchType = errors.New("unsupported cc-switch provider type")

type ccSwitchProviderRow struct {
	ID, AppType, Name, Settings string
	IsCurrent                   bool
}

// importLegacyCCSwitch is intentionally automatic only for the real default
// store. Tests and alternate stores must opt in through ImportCCSwitch.
func (s *Store) importLegacyCCSwitch(pd *ProviderData) (bool, error) {
	if filepath.Clean(s.filePath) != filepath.Clean(DefaultPath()) {
		return false, nil
	}
	if pd.Migrations != nil && pd.Migrations[ccSwitchMigration] != 0 {
		return false, nil
	}
	path := findCCSwitchDatabase()
	if path == "" {
		return false, nil
	}
	if err := importCCSwitchFile(pd, path); err != nil {
		return false, fmt.Errorf("import cc-switch providers: %w", err)
	}
	if pd.Migrations == nil {
		pd.Migrations = map[string]int64{}
	}
	pd.Migrations[ccSwitchMigration] = time.Now().Unix()
	return true, nil
}

// ImportCCSwitch imports a user-selected legacy database. It is idempotent by
// provider id + agent endpoint and does not delete existing 1agents records.
func (s *Store) ImportCCSwitch(path string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	pd, err := s.loadUnlocked()
	if err != nil {
		return err
	}
	if err := importCCSwitchFile(pd, path); err != nil {
		return err
	}
	if pd.Migrations == nil {
		pd.Migrations = map[string]int64{}
	}
	pd.Migrations[ccSwitchMigration] = time.Now().Unix()
	return s.saveUnlocked(pd)
}

func findCCSwitchDatabase() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	candidates := []string{filepath.Join(home, ".cc-switch", "cc-switch.db")}
	switch runtime.GOOS {
	case "darwin":
		candidates = append(candidates, filepath.Join(home, "Library", "Application Support", "cc-switch", "cc-switch.db"))
	case "linux":
		dataHome := os.Getenv("XDG_DATA_HOME")
		if dataHome == "" {
			dataHome = filepath.Join(home, ".local", "share")
		}
		candidates = append(candidates, filepath.Join(dataHome, "cc-switch", "cc-switch.db"))
	}
	for _, path := range candidates {
		if info, err := os.Stat(path); err == nil && !info.IsDir() {
			return path
		}
	}
	return ""
}

func importCCSwitchFile(pd *ProviderData, path string) error {
	if strings.TrimSpace(path) == "" {
		return fmt.Errorf("database path is required")
	}
	db, err := sql.Open("sqlite", path+"?mode=ro")
	if err != nil {
		return err
	}
	defer db.Close()
	rows, err := db.Query("SELECT id, app_type, name, settings_config, is_current FROM providers")
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var row ccSwitchProviderRow
		if err := rows.Scan(&row.ID, &row.AppType, &row.Name, &row.Settings, &row.IsCurrent); err != nil {
			return err
		}
		if err := mergeCCSwitchRow(pd, row); err != nil {
			if errors.Is(err, errUnsupportedCCSwitchType) {
				log.Printf("[provider] skip unsupported cc-switch provider type %q", row.AppType)
				continue
			}
			return fmt.Errorf("provider %s/%s: %w", row.ID, row.AppType, err)
		}
	}
	return rows.Err()
}

func mergeCCSwitchRow(pd *ProviderData, row ccSwitchProviderRow) error {
	var settings map[string]any
	if err := json.Unmarshal([]byte(row.Settings), &settings); err != nil {
		return err
	}
	endpoint, models, mapping, err := ccSwitchEndpoint(row.AppType, settings)
	if err != nil {
		return err
	}
	id := strings.TrimSpace(row.ID)
	if id == "" {
		id = sanitizeID(row.Name)
	}
	if id == "" {
		return fmt.Errorf("missing id")
	}
	p := providerByID(pd.Providers, id)
	if p == nil {
		pd.Providers = append(pd.Providers, Provider{ID: id, Name: row.Name, CreatedAt: time.Now().Unix()})
		p = &pd.Providers[len(pd.Providers)-1]
	}
	if p.Name == "" {
		p.Name = row.Name
	}
	p.Apps = compactStrings(append(p.Apps, strings.ToLower(row.AppType))...)
	p.UpdatedAt = time.Now().Unix()
	model := ""
	if len(models) > 0 {
		model = models[0]
	}
	useEndpoint := true
	if (strings.EqualFold(row.AppType, "openclaw") || strings.EqualFold(row.AppType, "opencode")) && hasEndpoint(p.Endpoints, endpoint.AgentID) {
		useEndpoint = false
	}
	if useEndpoint {
		upsertEndpoint(&p.Endpoints, endpoint)
	}
	if p.APIKey == "" {
		p.APIKey = endpoint.APIKey
	}
	if p.BaseURL == "" {
		p.BaseURL = endpoint.BaseURL
	}
	if p.Model == "" {
		p.Model = model
	}
	if useEndpoint {
		switch endpoint.AgentID {
		case AgentClaude:
			p.AnthropicBaseURL = endpoint.BaseURL
			p.Protocol = "anthropic"
			p.HaikuModel = mapping["haiku"]
			p.SonnetModel = mapping["sonnet"]
			p.OpusModel = mapping["opus"]
		case AgentCodex:
			p.OpenAIBaseURL = endpoint.BaseURL
			if p.Protocol == "anthropic" {
				p.Protocol = "dual"
			} else {
				p.Protocol = "openai"
			}
		}
	}
	for _, modelID := range append(models, mappingValues(mapping)...) {
		if modelID != "" {
			upsertModel(&pd.Models, ProviderModel{ProviderID: id, ModelID: modelID, Source: "cc-switch", Available: true, DiscoveredAt: time.Now().Unix(), LastSeenAt: time.Now().Unix()})
		}
	}
	if row.IsCurrent && (strings.EqualFold(row.AppType, "claude") || strings.EqualFold(row.AppType, "codex")) {
		upsertBinding(&pd.Bindings, AgentBinding{AgentID: endpoint.AgentID, ProviderID: id, ModelID: model, ModelMapping: compactMapping(mapping), UpdatedAt: time.Now().Unix()})
	}
	return nil
}

func ccSwitchEndpoint(appType string, settings map[string]any) (ProviderEndpoint, []string, map[string]string, error) {
	mapping := map[string]string{}
	switch strings.ToLower(appType) {
	case "claude":
		env, _ := settings["env"].(map[string]any)
		if env == nil {
			return ProviderEndpoint{}, nil, nil, fmt.Errorf("missing env")
		}
		key := stringValue(env, "ANTHROPIC_AUTH_TOKEN")
		if key == "" {
			key = stringValue(env, "ANTHROPIC_API_KEY")
		}
		model := stringValue(env, "ANTHROPIC_MODEL")
		mapping["haiku"] = stringValue(env, "ANTHROPIC_DEFAULT_HAIKU_MODEL")
		mapping["sonnet"] = stringValue(env, "ANTHROPIC_DEFAULT_SONNET_MODEL")
		mapping["opus"] = stringValue(env, "ANTHROPIC_DEFAULT_OPUS_MODEL")
		return ProviderEndpoint{AgentID: AgentClaude, Protocol: "anthropic", BaseURL: stringValue(env, "ANTHROPIC_BASE_URL"), APIKey: key}, compactStrings(model), mapping, nil
	case "codex":
		auth, _ := settings["auth"].(map[string]any)
		key := stringValue(auth, "OPENAI_API_KEY")
		configText, _ := settings["config"].(string)
		config := map[string]any{}
		if configText != "" {
			if _, err := toml.Decode(configText, &config); err != nil {
				return ProviderEndpoint{}, nil, nil, err
			}
		}
		baseURL, _ := config["base_url"].(string)
		model, _ := config["model"].(string)
		protocol := "openai_responses"
		if wire, _ := config["wire_api"].(string); wire == "chat" {
			protocol = "openai_chat"
		}
		return ProviderEndpoint{AgentID: AgentCodex, Protocol: protocol, BaseURL: baseURL, APIKey: key}, compactStrings(model), mapping, nil
	case "openclaw":
		models := make([]string, 0)
		if values, ok := settings["models"].([]any); ok {
			for _, value := range values {
				if model, ok := value.(map[string]any); ok {
					models = append(models, stringValue(model, "id"))
				}
			}
		}
		protocol := strings.ToLower(stringValue(settings, "api"))
		if strings.Contains(protocol, "anthropic") {
			return ProviderEndpoint{AgentID: AgentClaude, Protocol: "anthropic", BaseURL: stringValue(settings, "baseUrl"), APIKey: stringValue(settings, "apiKey")}, compactStrings(models...), mapping, nil
		}
		return ProviderEndpoint{AgentID: AgentCodex, Protocol: "openai", BaseURL: stringValue(settings, "baseUrl"), APIKey: stringValue(settings, "apiKey")}, compactStrings(models...), mapping, nil
	case "opencode":
		options, _ := settings["options"].(map[string]any)
		models := make([]string, 0)
		if values, ok := settings["models"].(map[string]any); ok {
			for modelID := range values {
				models = append(models, modelID)
			}
			sort.Strings(models)
		}
		if strings.Contains(strings.ToLower(stringValue(settings, "npm")), "anthropic") {
			return ProviderEndpoint{AgentID: AgentClaude, Protocol: "anthropic", BaseURL: stringValue(options, "baseURL"), APIKey: stringValue(options, "apiKey")}, compactStrings(models...), mapping, nil
		}
		return ProviderEndpoint{AgentID: AgentCodex, Protocol: "openai", BaseURL: stringValue(options, "baseURL"), APIKey: stringValue(options, "apiKey")}, compactStrings(models...), mapping, nil
	default:
		return ProviderEndpoint{}, nil, nil, fmt.Errorf("%w %q", errUnsupportedCCSwitchType, appType)
	}
}

func compactStrings(values ...string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" && !seen[value] {
			seen[value] = true
			out = append(out, value)
		}
	}
	return out
}

func mappingValues(mapping map[string]string) []string {
	return compactStrings(mapping["haiku"], mapping["sonnet"], mapping["opus"])
}

func stringValue(values map[string]any, key string) string {
	value, _ := values[key].(string)
	return strings.TrimSpace(value)
}
func upsertEndpoint(values *[]ProviderEndpoint, endpoint ProviderEndpoint) {
	for i := range *values {
		if (*values)[i].AgentID == endpoint.AgentID {
			(*values)[i] = endpoint
			return
		}
	}
	*values = append(*values, endpoint)
}
func upsertModel(values *[]ProviderModel, model ProviderModel) {
	for i := range *values {
		if (*values)[i].ProviderID == model.ProviderID && (*values)[i].ModelID == model.ModelID {
			(*values)[i].Available = true
			return
		}
	}
	*values = append(*values, model)
}
func upsertBinding(values *[]AgentBinding, binding AgentBinding) {
	for i := range *values {
		if (*values)[i].AgentID == binding.AgentID {
			(*values)[i] = binding
			return
		}
	}
	*values = append(*values, binding)
}
