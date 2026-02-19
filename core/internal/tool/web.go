package tool

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	readability "codeberg.org/readeck/go-readability/v2"
)

const (
	maxFetchSize    = 50 * 1024 // 50KB text output
	fetchTimeout    = 30 * time.Second
	defaultNumResults = 5
)

// --- WebSearch ---

// WebSearchTool searches the web using Brave Search API.
type WebSearchTool struct {
	APIKey string // Brave Search API key
}

func (t *WebSearchTool) Name() string        { return "web_search" }
func (t *WebSearchTool) Description() string  { return "Search the web and return top results" }
func (t *WebSearchTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"query": map[string]any{"type": "string", "description": "Search query"},
		},
		"required": []string{"query"},
	}
}

func (t *WebSearchTool) Execute(ctx context.Context, params map[string]any) (string, error) {
	query := getString(params, "query")
	if query == "" {
		return "", fmt.Errorf("web_search: query is required")
	}
	if t.APIKey == "" {
		return "web search is not available (no API key configured)", nil
	}

	reqURL := fmt.Sprintf("https://api.search.brave.com/res/v1/web/search?q=%s&count=%d",
		url.QueryEscape(query), defaultNumResults)

	req, err := http.NewRequestWithContext(ctx, "GET", reqURL, nil)
	if err != nil {
		return "", fmt.Errorf("web_search: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("X-Subscription-Token", t.APIKey)

	client := &http.Client{Timeout: fetchTimeout}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("web_search: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return "", fmt.Errorf("web_search: API returned %d: %s", resp.StatusCode, string(body))
	}

	var result braveSearchResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("web_search: parse response: %w", err)
	}

	var b strings.Builder
	for i, r := range result.Web.Results {
		fmt.Fprintf(&b, "%d. %s\n   %s\n   %s\n\n", i+1, r.Title, r.URL, r.Description)
	}
	if b.Len() == 0 {
		return "No results found.", nil
	}
	return b.String(), nil
}

type braveSearchResponse struct {
	Web struct {
		Results []struct {
			Title       string `json:"title"`
			URL         string `json:"url"`
			Description string `json:"description"`
		} `json:"results"`
	} `json:"web"`
}

// --- WebFetch ---

// WebFetchTool fetches a URL and extracts readable content.
type WebFetchTool struct{}

func (t *WebFetchTool) Name() string        { return "web_fetch" }
func (t *WebFetchTool) Description() string  { return "Fetch a URL and extract readable text content" }
func (t *WebFetchTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"url": map[string]any{"type": "string", "description": "URL to fetch"},
		},
		"required": []string{"url"},
	}
}

func (t *WebFetchTool) Execute(ctx context.Context, params map[string]any) (string, error) {
	rawURL := getString(params, "url")
	if rawURL == "" {
		return "", fmt.Errorf("web_fetch: url is required")
	}

	parsedURL, err := url.Parse(rawURL)
	if err != nil {
		return "", fmt.Errorf("web_fetch: invalid URL: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "GET", rawURL, nil)
	if err != nil {
		return "", fmt.Errorf("web_fetch: %w", err)
	}
	req.Header.Set("User-Agent", "h1v3-agent/1.0")

	client := &http.Client{Timeout: fetchTimeout}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("web_fetch: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("web_fetch: HTTP %d", resp.StatusCode)
	}

	contentType := resp.Header.Get("Content-Type")

	// For non-HTML content, return raw text (truncated)
	if !strings.Contains(contentType, "text/html") {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, int64(maxFetchSize)))
		return string(body), nil
	}

	// Parse with readability
	article, err := readability.FromReader(resp.Body, parsedURL)
	if err != nil {
		return "", fmt.Errorf("web_fetch: parse: %w", err)
	}

	var textBuf bytes.Buffer
	if err := article.RenderText(&textBuf); err != nil {
		return "", fmt.Errorf("web_fetch: render: %w", err)
	}

	text := textBuf.String()
	wordCount := len(strings.Fields(text))

	if len(text) > maxFetchSize {
		text = text[:maxFetchSize] + "\n... [truncated]"
	}

	result := fmt.Sprintf("Title: %s\nURL: %s\nWords: %d\n\n%s", article.Title(), rawURL, wordCount, text)
	return result, nil
}
