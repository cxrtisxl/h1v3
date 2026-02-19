package config

import (
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

