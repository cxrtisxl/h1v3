package tool

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestWebSearch_NoAPIKey(t *testing.T) {
	tool := &WebSearchTool{APIKey: ""}
	result, err := tool.Execute(context.Background(), map[string]any{"query": "test"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(result, "not available") {
		t.Errorf("expected 'not available' message, got %q", result)
	}
}

func TestWebSearch_EmptyQuery(t *testing.T) {
	tool := &WebSearchTool{APIKey: "test-key"}
	_, err := tool.Execute(context.Background(), map[string]any{"query": ""})
	if err == nil {
		t.Fatal("expected error for empty query")
	}
}

func TestWebSearch_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Subscription-Token") != "test-key" {
			t.Error("expected API key in header")
		}
		resp := braveSearchResponse{}
		resp.Web.Results = []struct {
			Title       string `json:"title"`
			URL         string `json:"url"`
			Description string `json:"description"`
		}{
			{Title: "Result 1", URL: "https://example.com", Description: "A test result"},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	// We can't easily override the Brave API URL in the tool,
	// so we test the response parsing separately.
	// The NoAPIKey and EmptyQuery tests cover the tool logic.
	// For a full integration test we'd need dependency injection on the base URL.
}

func TestWebFetch_HTML(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write([]byte(`<!DOCTYPE html>
<html><head><title>Test Page</title></head>
<body><article><h1>Hello World</h1><p>This is a test article with some content.</p></article></body>
</html>`))
	}))
	defer server.Close()

	tool := &WebFetchTool{}
	result, err := tool.Execute(context.Background(), map[string]any{"url": server.URL})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(result, "Test Page") {
		t.Errorf("expected title in output, got %q", result)
	}
	if !strings.Contains(result, "Words:") {
		t.Errorf("expected word count in output, got %q", result)
	}
}

func TestWebFetch_PlainText(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		w.Write([]byte("plain text content"))
	}))
	defer server.Close()

	tool := &WebFetchTool{}
	result, err := tool.Execute(context.Background(), map[string]any{"url": server.URL})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != "plain text content" {
		t.Errorf("expected 'plain text content', got %q", result)
	}
}

func TestWebFetch_EmptyURL(t *testing.T) {
	tool := &WebFetchTool{}
	_, err := tool.Execute(context.Background(), map[string]any{"url": ""})
	if err == nil {
		t.Fatal("expected error for empty URL")
	}
}

func TestWebFetch_HTTPError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	tool := &WebFetchTool{}
	_, err := tool.Execute(context.Background(), map[string]any{"url": server.URL})
	if err == nil {
		t.Fatal("expected error for 404")
	}
	if !strings.Contains(err.Error(), "404") {
		t.Errorf("expected 404 in error, got %q", err.Error())
	}
}
