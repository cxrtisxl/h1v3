package tool

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os/exec"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// MCPTransport abstracts stdio vs HTTP communication.
type MCPTransport interface {
	Send(ctx context.Context, msg json.RawMessage) (json.RawMessage, error)
	Close() error
}

// MCPClient manages a connection to an MCP server.
type MCPClient struct {
	name      string
	transport MCPTransport
	tools     []*MCPToolWrapper
}

// MCPToolWrapper wraps a remote MCP tool as a local Tool.
type MCPToolWrapper struct {
	serverName  string
	toolName    string
	description string
	schema      map[string]any
	client      *MCPClient
}

func (w *MCPToolWrapper) Name() string        { return fmt.Sprintf("mcp_%s_%s", w.serverName, w.toolName) }
func (w *MCPToolWrapper) Description() string  { return w.description }
func (w *MCPToolWrapper) Parameters() map[string]any { return w.schema }

func (w *MCPToolWrapper) Execute(ctx context.Context, params map[string]any) (string, error) {
	return w.client.CallTool(ctx, w.toolName, params)
}

// --- JSON-RPC 2.0 types ---

type jsonRPCRequest struct {
	JSONRPC string `json:"jsonrpc"`
	ID      int64  `json:"id"`
	Method  string `json:"method"`
	Params  any    `json:"params,omitempty"`
}

type jsonRPCResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      *int64          `json:"id,omitempty"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *jsonRPCError   `json:"error,omitempty"`
}

type jsonRPCError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// --- MCP protocol types ---

type mcpToolDef struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	InputSchema map[string]any `json:"inputSchema"`
}

type mcpToolsListResult struct {
	Tools []mcpToolDef `json:"tools"`
}

type mcpCallToolParams struct {
	Name      string         `json:"name"`
	Arguments map[string]any `json:"arguments,omitempty"`
}

type mcpCallToolResult struct {
	Content []mcpContent `json:"content"`
	IsError bool         `json:"isError,omitempty"`
}

type mcpContent struct {
	Type string `json:"type"`
	Text string `json:"text,omitempty"`
}

// --- Stdio Transport ---

// StdioTransport communicates with an MCP server via stdin/stdout of a spawned process.
type StdioTransport struct {
	cmd    *exec.Cmd
	stdin  io.WriteCloser
	stdout *bufio.Reader
	mu     sync.Mutex
	nextID atomic.Int64
}

// NewStdioTransport spawns a process and returns a transport.
func NewStdioTransport(ctx context.Context, command string, args []string, env []string) (*StdioTransport, error) {
	cmd := exec.CommandContext(ctx, command, args...)
	if len(env) > 0 {
		cmd.Env = env
	}

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("mcp stdio: stdin pipe: %w", err)
	}

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		stdin.Close()
		return nil, fmt.Errorf("mcp stdio: stdout pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		stdin.Close()
		return nil, fmt.Errorf("mcp stdio: start %q: %w", command, err)
	}

	return &StdioTransport{
		cmd:    cmd,
		stdin:  stdin,
		stdout: bufio.NewReader(stdout),
	}, nil
}

func (t *StdioTransport) Send(ctx context.Context, msg json.RawMessage) (json.RawMessage, error) {
	t.mu.Lock()
	defer t.mu.Unlock()

	// Write message + newline
	data := append(msg, '\n')
	if _, err := t.stdin.Write(data); err != nil {
		return nil, fmt.Errorf("mcp stdio: write: %w", err)
	}

	// Read response line
	line, err := t.stdout.ReadBytes('\n')
	if err != nil {
		return nil, fmt.Errorf("mcp stdio: read: %w", err)
	}

	return json.RawMessage(bytes.TrimSpace(line)), nil
}

func (t *StdioTransport) Close() error {
	t.stdin.Close()
	return t.cmd.Wait()
}

// --- HTTP Transport ---

// HTTPTransport communicates with an MCP server via HTTP POST.
type HTTPTransport struct {
	url    string
	client *http.Client
	nextID atomic.Int64
}

// NewHTTPTransport creates a transport that POSTs JSON-RPC to the given URL.
func NewHTTPTransport(url string) *HTTPTransport {
	return &HTTPTransport{
		url:    url,
		client: &http.Client{Timeout: 60 * time.Second},
	}
}

func (t *HTTPTransport) Send(ctx context.Context, msg json.RawMessage) (json.RawMessage, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, t.url, bytes.NewReader(msg))
	if err != nil {
		return nil, fmt.Errorf("mcp http: create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := t.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("mcp http: request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("mcp http: read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("mcp http: status %d: %s", resp.StatusCode, string(body))
	}

	return json.RawMessage(body), nil
}

func (t *HTTPTransport) Close() error { return nil }

// --- MCPClient methods ---

// NewMCPClient creates an MCP client and performs the initialize handshake.
func NewMCPClient(ctx context.Context, name string, transport MCPTransport) (*MCPClient, error) {
	c := &MCPClient{
		name:      name,
		transport: transport,
	}

	// Initialize handshake
	if err := c.initialize(ctx); err != nil {
		return nil, err
	}

	// Discover tools
	if err := c.discoverTools(ctx); err != nil {
		return nil, err
	}

	return c, nil
}

func (c *MCPClient) call(ctx context.Context, method string, params any) (json.RawMessage, error) {
	req := jsonRPCRequest{
		JSONRPC: "2.0",
		ID:      time.Now().UnixNano(),
		Method:  method,
		Params:  params,
	}

	data, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("mcp: marshal request: %w", err)
	}

	respData, err := c.transport.Send(ctx, data)
	if err != nil {
		return nil, err
	}

	var resp jsonRPCResponse
	if err := json.Unmarshal(respData, &resp); err != nil {
		return nil, fmt.Errorf("mcp: unmarshal response: %w", err)
	}

	if resp.Error != nil {
		return nil, fmt.Errorf("mcp: rpc error %d: %s", resp.Error.Code, resp.Error.Message)
	}

	return resp.Result, nil
}

func (c *MCPClient) initialize(ctx context.Context) error {
	params := map[string]any{
		"protocolVersion": "2024-11-05",
		"capabilities":    map[string]any{},
		"clientInfo": map[string]string{
			"name":    "h1v3",
			"version": "0.1.0",
		},
	}

	_, err := c.call(ctx, "initialize", params)
	if err != nil {
		return fmt.Errorf("mcp: initialize %q: %w", c.name, err)
	}

	// Send initialized notification (no response expected for notifications,
	// but some servers accept it as a regular call)
	notif := jsonRPCRequest{
		JSONRPC: "2.0",
		Method:  "notifications/initialized",
	}
	data, _ := json.Marshal(notif)
	// Best-effort: some transports may not handle notifications well
	c.transport.Send(ctx, data)

	return nil
}

func (c *MCPClient) discoverTools(ctx context.Context) error {
	result, err := c.call(ctx, "tools/list", nil)
	if err != nil {
		return fmt.Errorf("mcp: tools/list %q: %w", c.name, err)
	}

	var toolsList mcpToolsListResult
	if err := json.Unmarshal(result, &toolsList); err != nil {
		return fmt.Errorf("mcp: parse tools list: %w", err)
	}

	for _, td := range toolsList.Tools {
		wrapper := &MCPToolWrapper{
			serverName:  c.name,
			toolName:    td.Name,
			description: td.Description,
			schema:      td.InputSchema,
			client:      c,
		}
		c.tools = append(c.tools, wrapper)
	}

	return nil
}

// CallTool invokes a tool on the remote MCP server.
func (c *MCPClient) CallTool(ctx context.Context, name string, arguments map[string]any) (string, error) {
	result, err := c.call(ctx, "tools/call", mcpCallToolParams{
		Name:      name,
		Arguments: arguments,
	})
	if err != nil {
		return "", err
	}

	var callResult mcpCallToolResult
	if err := json.Unmarshal(result, &callResult); err != nil {
		return "", fmt.Errorf("mcp: parse tool result: %w", err)
	}

	// Collect text content
	var parts []string
	for _, c := range callResult.Content {
		if c.Type == "text" && c.Text != "" {
			parts = append(parts, c.Text)
		}
	}
	output := strings.Join(parts, "\n")

	if callResult.IsError {
		return "", fmt.Errorf("mcp tool %q error: %s", name, output)
	}

	return output, nil
}

// Tools returns the discovered tool wrappers.
func (c *MCPClient) Tools() []*MCPToolWrapper {
	return c.tools
}

// Close shuts down the transport.
func (c *MCPClient) Close() error {
	return c.transport.Close()
}

// RegisterMCPTools connects to MCP servers and registers their tools in a registry.
func RegisterMCPTools(ctx context.Context, registry *Registry, servers []MCPServerConfig) ([]*MCPClient, error) {
	var clients []*MCPClient

	for _, srv := range servers {
		var transport MCPTransport
		var err error

		switch srv.Transport {
		case "stdio":
			transport, err = NewStdioTransport(ctx, srv.Command, srv.Args, srv.Env)
		case "http":
			transport = NewHTTPTransport(srv.URL)
		default:
			return nil, fmt.Errorf("mcp: unknown transport %q for server %q", srv.Transport, srv.Name)
		}
		if err != nil {
			// Close already-opened clients on error
			for _, c := range clients {
				c.Close()
			}
			return nil, err
		}

		client, err := NewMCPClient(ctx, srv.Name, transport)
		if err != nil {
			transport.Close()
			for _, c := range clients {
				c.Close()
			}
			return nil, err
		}

		// Register all tools from this server
		for _, t := range client.Tools() {
			registry.Register(t)
		}

		clients = append(clients, client)
	}

	return clients, nil
}

// MCPServerConfig holds configuration for connecting to an MCP server.
type MCPServerConfig struct {
	Name      string   `json:"name"`
	Transport string   `json:"transport"` // "stdio" or "http"
	Command   string   `json:"command,omitempty"`
	Args      []string `json:"args,omitempty"`
	Env       []string `json:"env,omitempty"`
	URL       string   `json:"url,omitempty"`
}
