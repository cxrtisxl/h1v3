package config

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

const platformConfigJSON = `{
  "hive": {
    "id": "test-hive",
    "data_dir": "/ignored",
    "front_agent_id": "front"
  },
  "agents": [
    {
      "id": "coder",
      "role": "Software Engineer",
      "core_instructions": "Write clean code."
    },
    {
      "id": "front",
      "role": "Front Agent",
      "core_instructions": "Route messages."
    }
  ],
  "providers": {
    "default": {
      "api_key": "sk-test",
      "model": "gpt-4o"
    }
  },
  "api": {
    "host": "0.0.0.0",
    "port": 8080
  }
}`

func TestLoadFromPlatform(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/hives/config" {
			http.NotFound(w, r)
			return
		}
		if r.Header.Get("Authorization") != "Bearer test-key" {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		if r.Header.Get("X-Hive-ID") != "hive-123" {
			http.Error(w, "missing hive id", http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(platformConfigJSON))
	}))
	defer srv.Close()

	dataDir := t.TempDir()
	cfg, err := LoadFromPlatform(PlatformOptions{
		PlatformURL: srv.URL,
		HiveID:      "hive-123",
		APIKey:      "test-key",
		DataDir:     dataDir,
	})
	if err != nil {
		t.Fatalf("LoadFromPlatform: %v", err)
	}

	if cfg.Hive.ID != "test-hive" {
		t.Errorf("hive.id = %q", cfg.Hive.ID)
	}
	if cfg.Hive.DataDir != dataDir {
		t.Errorf("data_dir should be overridden to %q, got %q", dataDir, cfg.Hive.DataDir)
	}

	// Agent directories should be created
	for _, ag := range cfg.Agents {
		if ag.Directory == "" {
			t.Errorf("agent %q has no directory", ag.ID)
			continue
		}
		if _, err := os.Stat(ag.Directory); err != nil {
			t.Errorf("agent dir %q not created: %v", ag.Directory, err)
		}
		// SOUL.md should be written
		soulPath := filepath.Join(ag.Directory, "SOUL.md")
		if _, err := os.Stat(soulPath); err != nil {
			t.Errorf("SOUL.md not created for %q: %v", ag.ID, err)
		}
	}
}

func TestLoadFromPlatform_Unauthorized(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
	}))
	defer srv.Close()

	_, err := LoadFromPlatform(PlatformOptions{
		PlatformURL: srv.URL,
		HiveID:      "x",
		APIKey:      "wrong",
		DataDir:     t.TempDir(),
	})
	if err == nil {
		t.Fatal("expected error for unauthorized")
	}
}

func TestLoadFromPlatform_InvalidJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("not json"))
	}))
	defer srv.Close()

	_, err := LoadFromPlatform(PlatformOptions{
		PlatformURL: srv.URL,
		HiveID:      "x",
		APIKey:      "k",
		DataDir:     t.TempDir(),
	})
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}

func TestLoadFromPlatform_SOULNotOverwritten(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(platformConfigJSON))
	}))
	defer srv.Close()

	dataDir := t.TempDir()

	// Pre-create a SOUL.md
	agentDir := filepath.Join(dataDir, "agents", "coder")
	os.MkdirAll(agentDir, 0o755)
	os.WriteFile(filepath.Join(agentDir, "SOUL.md"), []byte("custom soul"), 0o644)

	cfg, err := LoadFromPlatform(PlatformOptions{
		PlatformURL: srv.URL,
		HiveID:      "x",
		APIKey:      "k",
		DataDir:     dataDir,
	})
	if err != nil {
		t.Fatalf("LoadFromPlatform: %v", err)
	}

	// SOUL.md should NOT be overwritten
	data, _ := os.ReadFile(filepath.Join(cfg.Agents[0].Directory, "SOUL.md"))
	if string(data) != "custom soul" {
		t.Errorf("SOUL.md was overwritten, got %q", string(data))
	}
}
