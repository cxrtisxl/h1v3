package main

import (
	"bufio"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/h1v3-io/h1v3/internal/agent"
	"github.com/h1v3-io/h1v3/internal/config"
	"github.com/h1v3-io/h1v3/internal/provider"
	"github.com/h1v3-io/h1v3/internal/tool"
	"github.com/h1v3-io/h1v3/pkg/protocol"
)

func main() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(0)
	}

	switch os.Args[1] {
	case "run":
		cmdRun(os.Args[2:])
	case "health":
		cmdHealth()
	case "agents":
		if len(os.Args) < 3 {
			fmt.Fprintln(os.Stderr, "usage: h1v3ctl agents <list|show>")
			os.Exit(1)
		}
		switch os.Args[2] {
		case "list":
			cmdAgentsList()
		case "show":
			if len(os.Args) < 4 {
				fmt.Fprintln(os.Stderr, "usage: h1v3ctl agents show <id>")
				os.Exit(1)
			}
			cmdAgentsShow(os.Args[3])
		default:
			fmt.Fprintf(os.Stderr, "unknown agents subcommand: %s\n", os.Args[2])
			os.Exit(1)
		}
	case "tickets":
		if len(os.Args) < 3 {
			fmt.Fprintln(os.Stderr, "usage: h1v3ctl tickets <list|show>")
			os.Exit(1)
		}
		switch os.Args[2] {
		case "list":
			cmdTicketsList(os.Args[3:])
		case "show":
			if len(os.Args) < 4 {
				fmt.Fprintln(os.Stderr, "usage: h1v3ctl tickets show <id>")
				os.Exit(1)
			}
			cmdTicketsShow(os.Args[3])
		default:
			fmt.Fprintf(os.Stderr, "unknown tickets subcommand: %s\n", os.Args[2])
			os.Exit(1)
		}
	case "config":
		if len(os.Args) < 3 || os.Args[2] != "validate" {
			fmt.Fprintln(os.Stderr, "usage: h1v3ctl config validate <path>")
			os.Exit(1)
		}
		if len(os.Args) < 4 {
			fmt.Fprintln(os.Stderr, "usage: h1v3ctl config validate <path>")
			os.Exit(1)
		}
		cmdConfigValidate(os.Args[3])
	default:
		fmt.Fprintf(os.Stderr, "unknown command: %s\n", os.Args[1])
		printUsage()
		os.Exit(1)
	}
}

// --- run command (from Phase 1) ---

func cmdRun(args []string) {
	fs := flag.NewFlagSet("run", flag.ExitOnError)
	provType := fs.String("provider", envOr("H1V3_PROVIDER", "openai"), "Provider type: openai or anthropic")
	model := fs.String("model", envOr("H1V3_MODEL", ""), "LLM model name")
	apiKey := fs.String("api-key", "", "API key (or set OPENAI_API_KEY / ANTHROPIC_API_KEY)")
	baseURL := fs.String("base-url", envOr("H1V3_BASE_URL", ""), "Override API base URL")
	prompt := fs.String("prompt", "", "Single prompt (omit for interactive)")
	workDir := fs.String("work-dir", ".", "Working directory")
	verbose := fs.Bool("v", false, "Verbose logging")
	fs.Parse(args)

	// Resolve API key from env if not passed as flag
	if *apiKey == "" {
		switch *provType {
		case "anthropic":
			*apiKey = os.Getenv("ANTHROPIC_API_KEY")
		default:
			*apiKey = os.Getenv("OPENAI_API_KEY")
		}
	}
	if *apiKey == "" {
		fmt.Fprintln(os.Stderr, "error: API key required (--api-key, OPENAI_API_KEY, or ANTHROPIC_API_KEY)")
		os.Exit(1)
	}

	logLevel := slog.LevelWarn
	if *verbose {
		logLevel = slog.LevelDebug
	}
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: logLevel}))

	var prov provider.Provider
	switch *provType {
	case "anthropic":
		if *model == "" {
			*model = "claude-sonnet-4-20250514"
		}
		var opts []provider.AnthropicOption
		opts = append(opts, provider.WithAnthropicModel(*model))
		if *baseURL != "" {
			opts = append(opts, provider.WithAnthropicBaseURL(*baseURL))
		}
		prov = provider.NewAnthropic(*apiKey, opts...)
	default:
		if *model == "" {
			*model = "gpt-4o"
		}
		var opts []provider.OpenAIOption
		opts = append(opts, provider.WithModel(*model))
		if *baseURL != "" {
			opts = append(opts, provider.WithBaseURL(*baseURL))
		}
		prov = provider.NewOpenAI(*apiKey, opts...)
	}

	absDir, _ := os.Getwd()
	if *workDir != "." {
		absDir = *workDir
	}

	reg := tool.NewRegistry()
	reg.Register(&tool.ReadFileTool{AllowedDir: absDir})
	reg.Register(&tool.WriteFileTool{AllowedDir: absDir})
	reg.Register(&tool.EditFileTool{AllowedDir: absDir})
	reg.Register(&tool.ListDirTool{AllowedDir: absDir})
	reg.Register(&tool.ExecTool{WorkDir: absDir})
	reg.Register(&tool.WebFetchTool{})
	if braveKey := os.Getenv("BRAVE_API_KEY"); braveKey != "" {
		reg.Register(&tool.WebSearchTool{APIKey: braveKey})
	}

	a := agent.New(
		protocol.AgentSpec{
			ID:               "h1v3ctl",
			Role:             "General-purpose assistant",
			CoreInstructions: "You are a helpful assistant with access to filesystem, shell, and web tools.",
			Directory:        absDir,
		},
		prov, reg,
	)
	a.Logger = logger
	ctx := context.Background()

	if *prompt != "" {
		result, err := a.Run(ctx, *prompt)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}
		fmt.Println(result)
	} else {
		fmt.Println("h1v3ctl interactive mode (type 'quit' to exit)")
		fmt.Printf("Model: %s | Tools: %s\n\n", *model, strings.Join(reg.List(), ", "))
		scanner := bufio.NewScanner(os.Stdin)
		for {
			fmt.Print("> ")
			if !scanner.Scan() {
				break
			}
			line := strings.TrimSpace(scanner.Text())
			if line == "" {
				continue
			}
			if line == "quit" || line == "exit" {
				break
			}
			result, err := a.Run(ctx, line)
			if err != nil {
				fmt.Fprintf(os.Stderr, "error: %v\n", err)
				continue
			}
			fmt.Println(result)
			fmt.Println()
		}
	}
}

// --- API client commands ---

func cmdHealth() {
	body, err := apiGet("/api/health")
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
	fmt.Println(string(body))
}

func cmdAgentsList() {
	body, err := apiGet("/api/agents")
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
	var agents []map[string]any
	json.Unmarshal(body, &agents)
	for _, a := range agents {
		fmt.Printf("%-20s %s\n", a["id"], a["role"])
	}
}

func cmdAgentsShow(id string) {
	body, err := apiGet("/api/agents/" + id)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
	fmt.Println(prettyJSON(body))
}

func cmdTicketsList(args []string) {
	fs := flag.NewFlagSet("tickets list", flag.ExitOnError)
	status := fs.String("status", "", "Filter by status (open|awaiting_close|closed)")
	agentID := fs.String("agent", "", "Filter by agent")
	limit := fs.Int("limit", 50, "Max results")
	fs.Parse(args)

	query := fmt.Sprintf("?limit=%d", *limit)
	if *status != "" {
		query += "&status=" + *status
	}
	if *agentID != "" {
		query += "&agent=" + *agentID
	}

	body, err := apiGet("/api/tickets" + query)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
	var tickets []map[string]any
	json.Unmarshal(body, &tickets)
	for _, t := range tickets {
		fmt.Printf("%-12s %-8s %s\n", t["id"], t["status"], t["title"])
	}
}

func cmdTicketsShow(id string) {
	body, err := apiGet("/api/tickets/" + id)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
	fmt.Println(prettyJSON(body))
}

func cmdConfigValidate(path string) {
	_, err := config.Load(path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "invalid: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("config is valid")
}

// --- Helpers ---

func apiGet(path string) ([]byte, error) {
	base := envOr("H1V3_API_URL", "http://localhost:8080")
	url := base + path

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}
	if key := os.Getenv("H1V3_API_KEY"); key != "" {
		req.Header.Set("Authorization", "Bearer "+key)
	}

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(body))
	}
	return body, nil
}

func prettyJSON(data []byte) string {
	var v any
	if err := json.Unmarshal(data, &v); err != nil {
		return string(data)
	}
	out, _ := json.MarshalIndent(v, "", "  ")
	return string(out)
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func printUsage() {
	fmt.Println("h1v3ctl — hive management CLI")
	fmt.Println()
	fmt.Println("Commands:")
	fmt.Println("  run                  Run agent with prompt or interactive REPL")
	fmt.Println("  health               Check daemon health")
	fmt.Println("  agents list          List all agents")
	fmt.Println("  agents show <id>     Show agent details")
	fmt.Println("  tickets list         List tickets (--status, --agent, --limit)")
	fmt.Println("  tickets show <id>    Show ticket details")
	fmt.Println("  config validate <p>  Validate config file")
	fmt.Println()
	fmt.Println("Environment:")
	fmt.Println("  H1V3_API_URL       Daemon URL (default: http://localhost:8080)")
	fmt.Println("  H1V3_API_KEY       API key for authentication")
	fmt.Println("  H1V3_PROVIDER      Provider type: openai (default) or anthropic")
	fmt.Println("  OPENAI_API_KEY     API key for OpenAI provider")
	fmt.Println("  ANTHROPIC_API_KEY  API key for Anthropic provider")
}
