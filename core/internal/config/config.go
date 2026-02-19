package config

import (
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/h1v3-io/h1v3/pkg/protocol"
)

// Config is the top-level h1v3 configuration.
type Config struct {
	Hive       HiveConfig                `json:"hive"`
	Agents     []protocol.AgentSpec      `json:"agents"`
	Providers  map[string]ProviderConfig `json:"providers"`
	Connectors ConnectorConfig           `json:"connectors"`
	Tools      ToolsConfig               `json:"tools"`
	API        APIConfig                 `json:"api"`
}

// HiveConfig holds hive-level settings.
type HiveConfig struct {
	ID               string `json:"id"`
	DataDir          string `json:"data_dir"`
	FrontAgentID     string `json:"front_agent_id"`
	CompactThreshold int    `json:"compact_threshold"`
}

// ProviderConfig holds LLM provider settings.
type ProviderConfig struct {
	Type    string `json:"type,omitempty"` // "openai" (default) or "anthropic"
	APIKey  string `json:"api_key"`
	BaseURL string `json:"base_url,omitempty"`
	Model   string `json:"model"`
}

// ConnectorConfig holds settings for external platform connectors.
type ConnectorConfig struct {
	Telegram *TelegramConfig `json:"telegram,omitempty"`
}

// TelegramConfig holds Telegram bot settings.
type TelegramConfig struct {
	Token     string  `json:"token"`
	AllowFrom []int64 `json:"allow_from,omitempty"`
}

// ToolsConfig holds tool-level settings.
type ToolsConfig struct {
	ShellTimeout   int      `json:"shell_timeout,omitempty"`    // seconds, default 30
	BlockedCommands []string `json:"blocked_commands,omitempty"`
	BraveAPIKey    string   `json:"brave_api_key,omitempty"`
}

// APIConfig holds REST API server settings.
type APIConfig struct {
	Host string `json:"host"`
	Port int    `json:"port"`
	Key  string `json:"api_key"`
}

// Load reads configuration from a JSON file.
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("config: read %s: %w", path, err)
	}

	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("config: parse %s: %w", path, err)
	}

	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	return &cfg, nil
}

// LoadFromEnv builds a minimal config from environment variables with H1V3_ prefix.
func LoadFromEnv() (*Config, error) {
	cfg := &Config{
		Hive: HiveConfig{
			ID:      getenv("H1V3_HIVE_ID", "default"),
			DataDir: getenv("H1V3_DATA_DIR", "/data"),
		},
		Providers: make(map[string]ProviderConfig),
		API: APIConfig{
			Host: getenv("H1V3_API_HOST", "0.0.0.0"),
			Port: getenvInt("H1V3_API_PORT", 8080),
			Key:  os.Getenv("H1V3_API_KEY"),
		},
	}

	// Default provider from env
	if apiKey := os.Getenv("H1V3_ANTHROPIC_API_KEY"); apiKey != "" {
		cfg.Providers["default"] = ProviderConfig{
			Type:   "anthropic",
			APIKey: apiKey,
			Model:  getenv("H1V3_MODEL", "claude-sonnet-4-20250514"),
		}
	} else if apiKey := os.Getenv("H1V3_OPENAI_API_KEY"); apiKey != "" {
		cfg.Providers["default"] = ProviderConfig{
			Type:    "openai",
			APIKey:  apiKey,
			BaseURL: os.Getenv("H1V3_OPENAI_BASE_URL"),
			Model:   getenv("H1V3_MODEL", "gpt-4o"),
		}
	}

	// Telegram connector from env
	if token := os.Getenv("H1V3_TELEGRAM_TOKEN"); token != "" {
		cfg.Connectors.Telegram = &TelegramConfig{
			Token: token,
		}
		if ids := os.Getenv("H1V3_TELEGRAM_ALLOW_FROM"); ids != "" {
			parsed, err := parseInt64List(ids)
			if err != nil {
				return nil, fmt.Errorf("config: H1V3_TELEGRAM_ALLOW_FROM: %w", err)
			}
			cfg.Connectors.Telegram.AllowFrom = parsed
		}
	}

	cfg.Hive.FrontAgentID = getenv("H1V3_FRONT_AGENT_ID", "front")
	cfg.Hive.CompactThreshold = getenvInt("H1V3_COMPACT_THRESHOLD", 8000)
	cfg.Tools.BraveAPIKey = os.Getenv("H1V3_BRAVE_API_KEY")

	return cfg, nil
}

// Validate checks for required fields.
func (c *Config) Validate() error {
	var errs []string

	if c.Hive.ID == "" {
		errs = append(errs, "hive.id is required")
	}
	if c.Hive.DataDir == "" {
		errs = append(errs, "hive.data_dir is required")
	}

	if len(c.Providers) == 0 {
		errs = append(errs, "at least one provider is required")
	}
	for name, p := range c.Providers {
		if p.APIKey == "" {
			errs = append(errs, fmt.Sprintf("providers.%s.api_key is required", name))
		}
		if p.Model == "" {
			errs = append(errs, fmt.Sprintf("providers.%s.model is required", name))
		}
	}

	for i, a := range c.Agents {
		if a.ID == "" {
			errs = append(errs, fmt.Sprintf("agents[%d].id is required", i))
		}
		if a.Role == "" {
			errs = append(errs, fmt.Sprintf("agents[%d].role is required", i))
		}
	}

	if c.Connectors.Telegram != nil && c.Connectors.Telegram.Token == "" {
		errs = append(errs, "connectors.telegram.token is required")
	}

	if len(errs) > 0 {
		return fmt.Errorf("config validation failed:\n  - %s", strings.Join(errs, "\n  - "))
	}
	return nil
}

func getenv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func getenvInt(key string, fallback int) int {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return fallback
}

func parseFloat64List(s string) ([]float64, error) {
	parts := strings.Split(s, ",")
	result := make([]float64, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		f, err := strconv.ParseFloat(p, 64)
		if err != nil {
			return nil, fmt.Errorf("invalid number %q", p)
		}
		result = append(result, f)
	}
	return result, nil
}

func parseInt64List(s string) ([]int64, error) {
	parts := strings.Split(s, ",")
	result := make([]int64, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		n, err := strconv.ParseInt(p, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("invalid integer %q", p)
		}
		result = append(result, n)
	}
	return result, nil
}
