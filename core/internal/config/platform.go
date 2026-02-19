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
	if opts.DataDir == "" {
		opts.DataDir = "/data"
	}

	// 1. Fetch config from platform
	url := fmt.Sprintf("%s/api/hives/config", opts.PlatformURL)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("platform: create request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+opts.APIKey)
	req.Header.Set("X-Hive-ID", opts.HiveID)

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("platform: fetch config: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("platform: read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("platform: HTTP %d: %s", resp.StatusCode, string(body))
	}

	// 2. Parse into Config
	var cfg Config
	if err := json.Unmarshal(body, &cfg); err != nil {
		return nil, fmt.Errorf("platform: parse config: %w", err)
	}

	// Override data dir with local path
	cfg.Hive.DataDir = opts.DataDir

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
	return &cfg, nil
}
