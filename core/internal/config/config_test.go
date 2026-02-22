package config

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/h1v3-io/h1v3/pkg/protocol"
)

const validJSON = `{
  "hive": {
    "id": "test-hive",
    "data_dir": "/tmp/h1v3-test",
    "front_agent_id": "front",
    "compact_threshold": 8000
  },
  "agents": [
    {
      "id": "coder",
      "role": "Software Engineer",
      "core_instructions": "Write clean code.",
      "directory": "/tmp/h1v3-test/agents/coder"
    }
  ],
  "providers": {
    "default": {
      "api_key": "sk-test-key",
      "model": "gpt-4o"
    }
  },
  "connectors": {
    "telegram": {
      "token": "123456:ABC",
      "allow_from": [100, 200]
    }
  },
  "tools": {
    "shell_timeout": 60,
    "brave_api_key": "brave-key"
  },
  "api": {
    "host": "0.0.0.0",
    "port": 8080,
    "api_key": "dashboard-key"
  }
}`

func TestLoad(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	os.WriteFile(path, []byte(validJSON), 0o644)

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if cfg.Hive.ID != "test-hive" {
		t.Errorf("hive.id = %q", cfg.Hive.ID)
	}
	if cfg.Hive.DataDir != "/tmp/h1v3-test" {
		t.Errorf("hive.data_dir = %q", cfg.Hive.DataDir)
	}
	if cfg.Hive.CompactThreshold != 8000 {
		t.Errorf("compact_threshold = %d", cfg.Hive.CompactThreshold)
	}
	if len(cfg.Agents) != 1 {
		t.Fatalf("agents count = %d", len(cfg.Agents))
	}
	if cfg.Agents[0].ID != "coder" {
		t.Errorf("agents[0].id = %q", cfg.Agents[0].ID)
	}
	if cfg.Providers["default"].APIKey != "sk-test-key" {
		t.Errorf("provider api_key = %q", cfg.Providers["default"].APIKey)
	}
	if cfg.Connectors.Telegram == nil {
		t.Fatal("telegram connector is nil")
	}
	if cfg.Connectors.Telegram.Token != "123456:ABC" {
		t.Errorf("telegram.token = %q", cfg.Connectors.Telegram.Token)
	}
	if len(cfg.Connectors.Telegram.AllowFrom) != 2 {
		t.Errorf("telegram.allow_from = %v", cfg.Connectors.Telegram.AllowFrom)
	}
	if cfg.API.Port != 8080 {
		t.Errorf("api.port = %d", cfg.API.Port)
	}
}

func TestLoad_FileNotFound(t *testing.T) {
	_, err := Load("/nonexistent/config.json")
	if err == nil {
		t.Fatal("expected error for missing file")
	}
}

func TestLoad_InvalidJSON(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "bad.json")
	os.WriteFile(path, []byte("not json"), 0o644)

	_, err := Load(path)
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}

func TestValidate_MissingHiveID(t *testing.T) {
	cfg := &Config{
		Hive:      HiveConfig{DataDir: "/data"},
		Providers: map[string]ProviderConfig{"default": {APIKey: "k", Model: "m"}},
	}
	err := cfg.Validate()
	if err == nil || !strings.Contains(err.Error(), "hive.id") {
		t.Errorf("expected hive.id error, got %v", err)
	}
}

func TestValidate_MissingProvider(t *testing.T) {
	cfg := &Config{
		Hive:      HiveConfig{ID: "h", DataDir: "/data"},
		Providers: map[string]ProviderConfig{},
	}
	err := cfg.Validate()
	if err == nil || !strings.Contains(err.Error(), "at least one provider") {
		t.Errorf("expected provider error, got %v", err)
	}
}

func TestValidate_MissingProviderAPIKey(t *testing.T) {
	cfg := &Config{
		Hive:      HiveConfig{ID: "h", DataDir: "/data"},
		Providers: map[string]ProviderConfig{"default": {Model: "m"}},
	}
	err := cfg.Validate()
	if err == nil || !strings.Contains(err.Error(), "api_key") {
		t.Errorf("expected api_key error, got %v", err)
	}
}

func TestValidate_MissingAgentID(t *testing.T) {
	cfg := &Config{
		Hive: HiveConfig{ID: "h", DataDir: "/data"},
		Providers: map[string]ProviderConfig{
			"default": {APIKey: "k", Model: "m"},
		},
		Agents: []protocol.AgentSpec{{Role: "dev"}},
	}
	err := cfg.Validate()
	if err == nil || !strings.Contains(err.Error(), "agents[0].id") {
		t.Errorf("expected agent id error, got %v", err)
	}
}

func TestValidate_TelegramNoToken(t *testing.T) {
	cfg := &Config{
		Hive: HiveConfig{ID: "h", DataDir: "/data"},
		Providers: map[string]ProviderConfig{
			"default": {APIKey: "k", Model: "m"},
		},
		Connectors: ConnectorConfig{Telegram: &TelegramConfig{}},
	}
	err := cfg.Validate()
	if err == nil || !strings.Contains(err.Error(), "telegram.token") {
		t.Errorf("expected telegram token error, got %v", err)
	}
}

func TestValidate_UnknownAgentProvider(t *testing.T) {
	cfg := &Config{
		Hive: HiveConfig{ID: "h", DataDir: "/data"},
		Providers: map[string]ProviderConfig{
			"default": {APIKey: "k", Model: "m"},
		},
		Agents: []protocol.AgentSpec{{ID: "a", Role: "r", Provider: "nonexistent"}},
	}
	err := cfg.Validate()
	if err == nil || !strings.Contains(err.Error(), "unknown provider") {
		t.Errorf("expected unknown provider error, got %v", err)
	}
}

func TestValidate_KnownAgentProvider(t *testing.T) {
	cfg := &Config{
		Hive: HiveConfig{ID: "h", DataDir: "/data"},
		Providers: map[string]ProviderConfig{
			"default": {APIKey: "k", Model: "m"},
			"smart":   {APIKey: "k2", Model: "m2"},
		},
		Agents: []protocol.AgentSpec{{ID: "a", Role: "r", Provider: "smart"}},
	}
	if err := cfg.Validate(); err != nil {
		t.Errorf("expected valid, got %v", err)
	}
}

func TestValidate_Valid(t *testing.T) {
	cfg := &Config{
		Hive: HiveConfig{ID: "h", DataDir: "/data"},
		Providers: map[string]ProviderConfig{
			"default": {APIKey: "k", Model: "m"},
		},
	}
	if err := cfg.Validate(); err != nil {
		t.Errorf("expected valid, got %v", err)
	}
}

func TestLoad_WithPresetFile(t *testing.T) {
	dir := t.TempDir()

	presetFile := `{
  "agents": [
    {
      "id": "dev-agent",
      "role": "Developer",
      "provider": "default",
      "core_instructions": "Write code.",
      "directory": "/tmp/test/agents/dev"
    }
  ]
}`
	os.WriteFile(filepath.Join(dir, "preset.json"), []byte(presetFile), 0o644)

	config := fmt.Sprintf(`{
  "hive": {
    "id": "test-hive",
    "data_dir": %q,
    "preset_file": "preset.json"
  },
  "providers": {
    "default": { "api_key": "k", "model": "m" }
  }
}`, dir)
	os.WriteFile(filepath.Join(dir, "config.json"), []byte(config), 0o644)

	cfg, err := Load(filepath.Join(dir, "config.json"))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Agents[0].ID != "dev-agent" {
		t.Errorf("expected id from preset file, got %q", cfg.Agents[0].ID)
	}
	if cfg.Agents[0].Role != "Developer" {
		t.Errorf("expected role from preset file, got %q", cfg.Agents[0].Role)
	}
	if cfg.Agents[0].CoreInstructions != "Write code." {
		t.Errorf("expected core_instructions from preset file, got %q", cfg.Agents[0].CoreInstructions)
	}
}

func TestLoad_ConfigAgentsOverridePresetFile(t *testing.T) {
	dir := t.TempDir()

	presetFile := `{
  "agents": [
    {
      "id": "preset-agent",
      "role": "Preset Role",
      "core_instructions": "From preset.",
      "directory": "/tmp/test/agents/dev"
    }
  ]
}`
	os.WriteFile(filepath.Join(dir, "preset.json"), []byte(presetFile), 0o644)

	config := fmt.Sprintf(`{
  "hive": {
    "id": "test-hive",
    "data_dir": %q,
    "preset_file": "preset.json"
  },
  "agents": [
    {
      "id": "inline-agent",
      "role": "Inline Role",
      "core_instructions": "From config.",
      "directory": "/tmp/test/agents/dev"
    }
  ],
  "providers": {
    "default": { "api_key": "k", "model": "m" }
  }
}`, dir)
	os.WriteFile(filepath.Join(dir, "config.json"), []byte(config), 0o644)

	cfg, err := Load(filepath.Join(dir, "config.json"))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Agents[0].ID != "inline-agent" {
		t.Errorf("expected config.json agents to win, got %q", cfg.Agents[0].ID)
	}
}

func TestLoad_NoPresetsBackwardCompat(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "config.json"), []byte(validJSON), 0o644)

	cfg, err := Load(filepath.Join(dir, "config.json"))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Agents[0].ID != "coder" {
		t.Errorf("expected inline agent to work without presets, got %q", cfg.Agents[0].ID)
	}
}

func TestLoadFromEnv(t *testing.T) {
	t.Setenv("H1V3_HIVE_ID", "env-hive")
	t.Setenv("H1V3_DATA_DIR", "/env/data")
	t.Setenv("H1V3_OPENAI_API_KEY", "sk-env")
	t.Setenv("H1V3_MODEL", "gpt-4o-mini")
	t.Setenv("H1V3_API_PORT", "9090")
	t.Setenv("H1V3_TELEGRAM_TOKEN", "tg-token")
	t.Setenv("H1V3_TELEGRAM_ALLOW_FROM", "100,200,300")

	cfg, err := LoadFromEnv()
	if err != nil {
		t.Fatalf("LoadFromEnv: %v", err)
	}

	if cfg.Hive.ID != "env-hive" {
		t.Errorf("hive.id = %q", cfg.Hive.ID)
	}
	if cfg.Hive.DataDir != "/env/data" {
		t.Errorf("data_dir = %q", cfg.Hive.DataDir)
	}
	if cfg.Providers["default"].APIKey != "sk-env" {
		t.Errorf("provider api_key = %q", cfg.Providers["default"].APIKey)
	}
	if cfg.Providers["default"].Model != "gpt-4o-mini" {
		t.Errorf("model = %q", cfg.Providers["default"].Model)
	}
	if cfg.API.Port != 9090 {
		t.Errorf("api.port = %d", cfg.API.Port)
	}
	if cfg.Connectors.Telegram == nil {
		t.Fatal("telegram is nil")
	}
	if len(cfg.Connectors.Telegram.AllowFrom) != 3 {
		t.Errorf("allow_from = %v", cfg.Connectors.Telegram.AllowFrom)
	}
}

func TestLoad_EnvVarRefs(t *testing.T) {
	t.Setenv("TEST_PROVIDER_KEY", "sk-resolved")
	t.Setenv("TEST_TG_TOKEN", "tg-resolved")
	t.Setenv("TEST_API_KEY", "api-resolved")

	dir := t.TempDir()
	config := `{
  "hive": { "id": "h", "data_dir": "/data" },
  "providers": {
    "default": { "api_key": "$TEST_PROVIDER_KEY", "model": "m" }
  },
  "connectors": {
    "telegram": { "token": "${TEST_TG_TOKEN}", "allow_from": [] }
  },
  "api": { "host": "0.0.0.0", "port": 8080, "api_key": "$TEST_API_KEY" }
}`
	os.WriteFile(filepath.Join(dir, "config.json"), []byte(config), 0o644)

	cfg, err := Load(filepath.Join(dir, "config.json"))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Providers["default"].APIKey != "sk-resolved" {
		t.Errorf("provider api_key = %q, want %q", cfg.Providers["default"].APIKey, "sk-resolved")
	}
	if cfg.Connectors.Telegram.Token != "tg-resolved" {
		t.Errorf("telegram token = %q, want %q", cfg.Connectors.Telegram.Token, "tg-resolved")
	}
	if cfg.API.Key != "api-resolved" {
		t.Errorf("api key = %q, want %q", cfg.API.Key, "api-resolved")
	}
}

func TestLoad_EnvVarRefs_LiteralFallthrough(t *testing.T) {
	dir := t.TempDir()
	config := `{
  "hive": { "id": "h", "data_dir": "/data" },
  "providers": {
    "default": { "api_key": "sk-literal-key", "model": "m" }
  },
  "api": { "host": "0.0.0.0", "port": 8080 }
}`
	os.WriteFile(filepath.Join(dir, "config.json"), []byte(config), 0o644)

	cfg, err := Load(filepath.Join(dir, "config.json"))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Providers["default"].APIKey != "sk-literal-key" {
		t.Errorf("literal api_key = %q, want %q", cfg.Providers["default"].APIKey, "sk-literal-key")
	}
}

func TestLoadFromPlatform_WithPreset(t *testing.T) {
	dataDir := t.TempDir()

	configResp := Config{
		Hive: HiveConfig{
			ID:               "test-hive",
			DataDir:          "/data",
			FrontAgentID:     "coder",
			CompactThreshold: 8000,
			PresetFile:       "preset.json",
		},
		Providers: map[string]ProviderConfig{
			"default": {APIKey: "sk-test", Model: "gpt-4o"},
		},
		API: APIConfig{Host: "0.0.0.0", Port: 8080, Key: "api-key"},
	}

	presetResp := PresetFile{
		Agents: []protocol.AgentSpec{
			{
				ID:               "coder",
				Role:             "Software Engineer",
				CoreInstructions: "Write clean code.",
			},
		},
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify auth headers
		if r.Header.Get("Authorization") != "Bearer test-api-key" {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		if r.Header.Get("X-Hive-ID") != "test-hive" {
			http.Error(w, "bad hive id", http.StatusForbidden)
			return
		}

		switch r.URL.Path {
		case "/api/hives/config":
			json.NewEncoder(w).Encode(configResp)
		case "/api/hives/preset":
			json.NewEncoder(w).Encode(presetResp)
		default:
			http.Error(w, "not found", http.StatusNotFound)
		}
	}))
	defer srv.Close()

	opts := PlatformOptions{
		PlatformURL: srv.URL,
		HiveID:      "test-hive",
		APIKey:      "test-api-key",
		DataDir:     dataDir,
	}

	cfg, err := loadFromPlatformWithClient(opts, srv.Client())
	if err != nil {
		t.Fatalf("LoadFromPlatform: %v", err)
	}

	// Config should have agents from preset
	if len(cfg.Agents) != 1 {
		t.Fatalf("expected 1 agent, got %d", len(cfg.Agents))
	}
	if cfg.Agents[0].ID != "coder" {
		t.Errorf("agent id = %q, want %q", cfg.Agents[0].ID, "coder")
	}
	if cfg.Agents[0].CoreInstructions != "Write clean code." {
		t.Errorf("core_instructions = %q", cfg.Agents[0].CoreInstructions)
	}

	// Agent workspace should be set up
	agentDir := filepath.Join(dataDir, "agents", "coder")
	if cfg.Agents[0].Directory != agentDir {
		t.Errorf("agent directory = %q, want %q", cfg.Agents[0].Directory, agentDir)
	}
	if _, err := os.Stat(agentDir); err != nil {
		t.Errorf("agent dir not created: %v", err)
	}

	// Preset file should be written to data dir
	presetPath := filepath.Join(dataDir, "preset.json")
	presetData, err := os.ReadFile(presetPath)
	if err != nil {
		t.Fatalf("preset file not written: %v", err)
	}
	var writtenPreset PresetFile
	if err := json.Unmarshal(presetData, &writtenPreset); err != nil {
		t.Fatalf("preset file invalid JSON: %v", err)
	}
	if len(writtenPreset.Agents) != 1 || writtenPreset.Agents[0].ID != "coder" {
		t.Errorf("preset file content unexpected: %+v", writtenPreset)
	}

	// SOUL.md should be written
	soulPath := filepath.Join(agentDir, "SOUL.md")
	if _, err := os.Stat(soulPath); err != nil {
		t.Errorf("SOUL.md not created: %v", err)
	}
}

func TestLoadFromPlatform_NoPreset(t *testing.T) {
	dataDir := t.TempDir()

	configResp := Config{
		Hive: HiveConfig{
			ID:               "test-hive",
			DataDir:          "/data",
			FrontAgentID:     "coder",
			CompactThreshold: 8000,
		},
		Agents: []protocol.AgentSpec{
			{
				ID:               "coder",
				Role:             "Software Engineer",
				CoreInstructions: "Write code.",
			},
		},
		Providers: map[string]ProviderConfig{
			"default": {APIKey: "sk-test", Model: "gpt-4o"},
		},
		API: APIConfig{Host: "0.0.0.0", Port: 8080, Key: "api-key"},
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/hives/config" {
			json.NewEncoder(w).Encode(configResp)
		} else {
			http.Error(w, "should not be called", http.StatusInternalServerError)
		}
	}))
	defer srv.Close()

	opts := PlatformOptions{
		PlatformURL: srv.URL,
		HiveID:      "test-hive",
		APIKey:      "test-api-key",
		DataDir:     dataDir,
	}

	cfg, err := loadFromPlatformWithClient(opts, srv.Client())
	if err != nil {
		t.Fatalf("LoadFromPlatform: %v", err)
	}

	if len(cfg.Agents) != 1 || cfg.Agents[0].ID != "coder" {
		t.Errorf("expected inline agent, got %+v", cfg.Agents)
	}

	// No preset file should be written
	presetPath := filepath.Join(dataDir, "preset.json")
	if _, err := os.Stat(presetPath); !os.IsNotExist(err) {
		t.Errorf("preset file should not exist when preset_file is empty")
	}
}

