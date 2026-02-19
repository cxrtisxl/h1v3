package tool

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

// mockTransport simulates an MCP server for testing.
type mockTransport struct {
	handler func(method string, params json.RawMessage) (json.RawMessage, error)
}

func (t *mockTransport) Send(_ context.Context, msg json.RawMessage) (json.RawMessage, error) {
	var req jsonRPCRequest
	if err := json.Unmarshal(msg, &req); err != nil {
		return nil, err
	}

	// Notifications (no ID field with value) — return empty for mock
	if req.Method == "notifications/initialized" {
		resp := jsonRPCResponse{JSONRPC: "2.0"}
		return json.Marshal(resp)
	}

	paramsBytes, _ := json.Marshal(req.Params)

	result, err := t.handler(req.Method, paramsBytes)
	if err != nil {
		errResp := jsonRPCResponse{
			JSONRPC: "2.0",
			ID:      &req.ID,
			Error:   &jsonRPCError{Code: -32000, Message: err.Error()},
		}
		return json.Marshal(errResp)
	}

	resp := jsonRPCResponse{
		JSONRPC: "2.0",
		ID:      &req.ID,
		Result:  result,
	}
	return json.Marshal(resp)
}

func (t *mockTransport) Close() error { return nil }

func newMockMCPTransport(tools []mcpToolDef) *mockTransport {
	return &mockTransport{
		handler: func(method string, params json.RawMessage) (json.RawMessage, error) {
			switch method {
			case "initialize":
				return json.Marshal(map[string]any{
					"protocolVersion": "2024-11-05",
					"capabilities":    map[string]any{},
					"serverInfo": map[string]string{
						"name":    "test-server",
						"version": "1.0.0",
					},
				})
			case "tools/list":
				return json.Marshal(mcpToolsListResult{Tools: tools})
			case "tools/call":
				var p mcpCallToolParams
				json.Unmarshal(params, &p)
				return json.Marshal(mcpCallToolResult{
					Content: []mcpContent{{Type: "text", Text: fmt.Sprintf("called %s", p.Name)}},
				})
			default:
				return nil, fmt.Errorf("unknown method: %s", method)
			}
		},
	}
}

func TestMCPClient_DiscoverTools(t *testing.T) {
	transport := newMockMCPTransport([]mcpToolDef{
		{
			Name:        "create_issue",
			Description: "Create a new issue",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"title": map[string]any{"type": "string"},
				},
			},
		},
		{
			Name:        "list_issues",
			Description: "List all issues",
			InputSchema: map[string]any{"type": "object"},
		},
	})

	client, err := NewMCPClient(context.Background(), "linear", transport)
	if err != nil {
		t.Fatalf("NewMCPClient: %v", err)
	}
	defer client.Close()

	tools := client.Tools()
	if len(tools) != 2 {
		t.Fatalf("expected 2 tools, got %d", len(tools))
	}

	if tools[0].Name() != "mcp_linear_create_issue" {
		t.Errorf("tool[0] name = %q", tools[0].Name())
	}
	if tools[1].Name() != "mcp_linear_list_issues" {
		t.Errorf("tool[1] name = %q", tools[1].Name())
	}
	if tools[0].Description() != "Create a new issue" {
		t.Errorf("tool[0] description = %q", tools[0].Description())
	}
}

func TestMCPToolWrapper_Execute(t *testing.T) {
	transport := newMockMCPTransport([]mcpToolDef{
		{Name: "greet", Description: "Say hello", InputSchema: map[string]any{"type": "object"}},
	})

	client, err := NewMCPClient(context.Background(), "test", transport)
	if err != nil {
		t.Fatalf("NewMCPClient: %v", err)
	}
	defer client.Close()

	tool := client.Tools()[0]
	result, err := tool.Execute(context.Background(), map[string]any{"name": "world"})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if result != "called greet" {
		t.Errorf("result = %q", result)
	}
}

func TestMCPToolWrapper_Parameters(t *testing.T) {
	schema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"path": map[string]any{"type": "string"},
		},
		"required": []any{"path"},
	}
	transport := newMockMCPTransport([]mcpToolDef{
		{Name: "read_file", Description: "Read a file", InputSchema: schema},
	})

	client, err := NewMCPClient(context.Background(), "fs", transport)
	if err != nil {
		t.Fatalf("NewMCPClient: %v", err)
	}

	tool := client.Tools()[0]
	params := tool.Parameters()
	if params["type"] != "object" {
		t.Errorf("schema type = %v", params["type"])
	}
}

func TestMCPClient_CallToolError(t *testing.T) {
	transport := &mockTransport{
		handler: func(method string, params json.RawMessage) (json.RawMessage, error) {
			switch method {
			case "initialize":
				return json.Marshal(map[string]any{"protocolVersion": "2024-11-05", "capabilities": map[string]any{}})
			case "tools/list":
				return json.Marshal(mcpToolsListResult{Tools: []mcpToolDef{
					{Name: "fail_tool", Description: "Always fails", InputSchema: map[string]any{"type": "object"}},
				}})
			case "tools/call":
				return json.Marshal(mcpCallToolResult{
					Content: []mcpContent{{Type: "text", Text: "something went wrong"}},
					IsError: true,
				})
			default:
				return nil, fmt.Errorf("unknown method")
			}
		},
	}

	client, err := NewMCPClient(context.Background(), "bad", transport)
	if err != nil {
		t.Fatalf("NewMCPClient: %v", err)
	}

	_, err = client.CallTool(context.Background(), "fail_tool", nil)
	if err == nil {
		t.Fatal("expected error from tool with isError=true")
	}
}

func TestMCPClient_InitializeError(t *testing.T) {
	transport := &mockTransport{
		handler: func(method string, params json.RawMessage) (json.RawMessage, error) {
			return nil, fmt.Errorf("server not ready")
		},
	}

	_, err := NewMCPClient(context.Background(), "bad", transport)
	if err == nil {
		t.Fatal("expected error from failed initialize")
	}
}

func TestRegisterMCPTools(t *testing.T) {
	// Set up a mock HTTP MCP server
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req jsonRPCRequest
		json.NewDecoder(r.Body).Decode(&req)

		var result any
		switch req.Method {
		case "initialize":
			result = map[string]any{"protocolVersion": "2024-11-05", "capabilities": map[string]any{}}
		case "notifications/initialized":
			result = nil
		case "tools/list":
			result = mcpToolsListResult{Tools: []mcpToolDef{
				{Name: "search", Description: "Search things", InputSchema: map[string]any{"type": "object"}},
			}}
		case "tools/call":
			result = mcpCallToolResult{Content: []mcpContent{{Type: "text", Text: "found it"}}}
		}

		resp := jsonRPCResponse{JSONRPC: "2.0", ID: &req.ID}
		if result != nil {
			resp.Result, _ = json.Marshal(result)
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	registry := NewRegistry()
	clients, err := RegisterMCPTools(context.Background(), registry, []MCPServerConfig{
		{Name: "brave", Transport: "http", URL: srv.URL},
	})
	if err != nil {
		t.Fatalf("RegisterMCPTools: %v", err)
	}
	defer func() {
		for _, c := range clients {
			c.Close()
		}
	}()

	if registry.Len() != 1 {
		t.Fatalf("expected 1 tool in registry, got %d", registry.Len())
	}
	if !registry.Has("mcp_brave_search") {
		t.Errorf("expected tool mcp_brave_search, got %v", registry.List())
	}

	// Execute the registered tool
	result, err := registry.Execute(context.Background(), "mcp_brave_search", nil)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if result != "found it" {
		t.Errorf("result = %q", result)
	}
}

func TestRegisterMCPTools_UnknownTransport(t *testing.T) {
	registry := NewRegistry()
	_, err := RegisterMCPTools(context.Background(), registry, []MCPServerConfig{
		{Name: "bad", Transport: "websocket"},
	})
	if err == nil {
		t.Fatal("expected error for unknown transport")
	}
}

func TestHTTPTransport_ServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("internal error"))
	}))
	defer srv.Close()

	transport := NewHTTPTransport(srv.URL)
	_, err := transport.Send(context.Background(), json.RawMessage(`{}`))
	if err == nil {
		t.Fatal("expected error for 500 status")
	}
}

func TestMCPClient_EmptyToolsList(t *testing.T) {
	transport := newMockMCPTransport(nil) // no tools

	client, err := NewMCPClient(context.Background(), "empty", transport)
	if err != nil {
		t.Fatalf("NewMCPClient: %v", err)
	}

	if len(client.Tools()) != 0 {
		t.Errorf("expected 0 tools, got %d", len(client.Tools()))
	}
}
