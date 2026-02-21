package config

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

// PlatformOptions holds parameters for fetching config from the h1v3 dashboard.
type PlatformOptions struct {
	PlatformURL string // e.g. https://dashboard.h1v3.com
	HiveID      string
	APIKey      string
	DataDir     string // local data directory, default /data
}

// LoadFromPlatform fetches the hive configuration from the dashboard API,
// sets up agent workspaces, and returns the parsed Config.
func LoadFromPlatform(opts PlatformOptions) (*Config, error) {
	return loadFromPlatformWithClient(opts, &http.Client{Timeout: 30 * time.Second})
}

func loadFromPlatformWithClient(opts PlatformOptions, client *http.Client) (*Config, error) {
	if opts.DataDir == "" {
		opts.DataDir = "/data"
	}

	// 1. Fetch config from platform
	cfg, err := fetchPlatformJSON[Config](client, opts, "/api/hives/config")
	if err != nil {
		return nil, err
	}

	// Override data dir with local path
	cfg.Hive.DataDir = opts.DataDir

	// 2. If preset_file is set, fetch preset from platform and write to data dir
	if cfg.Hive.PresetFile != "" {
		preset, err := fetchPlatformJSON[PresetFile](client, opts, "/api/hives/preset")
		if err != nil {
			return nil, fmt.Errorf("platform: fetch preset: %w", err)
		}

		presetData, err := json.MarshalIndent(preset, "", "  ")
		if err != nil {
			return nil, fmt.Errorf("platform: marshal preset: %w", err)
		}

		presetPath := filepath.Join(opts.DataDir, cfg.Hive.PresetFile)
		if err := os.MkdirAll(filepath.Dir(presetPath), 0o755); err != nil {
			return nil, fmt.Errorf("platform: create preset dir: %w", err)
		}
		if err := os.WriteFile(presetPath, presetData, 0o644); err != nil {
			return nil, fmt.Errorf("platform: write preset file: %w", err)
		}

		applyPresetFile(cfg, preset)
	}

	// 3. Set up agent workspaces
	for i, spec := range cfg.Agents {
		agentDir := filepath.Join(opts.DataDir, "agents", spec.ID)
		if err := os.MkdirAll(agentDir, 0o755); err != nil {
			return nil, fmt.Errorf("platform: create agent dir %q: %w", agentDir, err)
		}
		cfg.Agents[i].Directory = agentDir

		// 4. Write identity files if not present
		if spec.CoreInstructions != "" {
			soulPath := filepath.Join(agentDir, "SOUL.md")
			if _, err := os.Stat(soulPath); os.IsNotExist(err) {
				content := fmt.Sprintf("# %s\n\nRole: %s\n\n%s\n", spec.ID, spec.Role, spec.CoreInstructions)
				os.WriteFile(soulPath, []byte(content), 0o644)
			}
		}
	}

	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("platform: %w", err)
	}
	return cfg, nil
}

// fetchPlatformJSON fetches and parses a JSON endpoint from the platform.
func fetchPlatformJSON[T any](client *http.Client, opts PlatformOptions, path string) (*T, error) {
	url := fmt.Sprintf("%s%s", opts.PlatformURL, path)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("platform: create request for %s: %w", path, err)
	}
	req.Header.Set("Authorization", "Bearer "+opts.APIKey)
	req.Header.Set("X-Hive-ID", opts.HiveID)

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("platform: fetch %s: %w", path, err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("platform: read %s: %w", path, err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("platform: %s HTTP %d: %s", path, resp.StatusCode, string(body))
	}

	var result T
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("platform: parse %s: %w", path, err)
	}
	return &result, nil
}
