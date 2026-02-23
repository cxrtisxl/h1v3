package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/h1v3-io/h1v3/internal/agent"
	apiPkg "github.com/h1v3-io/h1v3/internal/api"
	"github.com/h1v3-io/h1v3/internal/config"
	"github.com/h1v3-io/h1v3/internal/connector"
	"github.com/h1v3-io/h1v3/internal/connector/telegram"
	"github.com/h1v3-io/h1v3/internal/logbuf"
	"github.com/h1v3-io/h1v3/internal/memory"
	"github.com/h1v3-io/h1v3/internal/provider"
	"github.com/h1v3-io/h1v3/internal/registry"
	"github.com/h1v3-io/h1v3/internal/ticket"
	"github.com/h1v3-io/h1v3/internal/tool"
	"github.com/h1v3-io/h1v3/pkg/protocol"
)

func main() {
	configPath := flag.String("config", "", "Path to config JSON file")
	platformURL := flag.String("platform-url", os.Getenv("H1V3_PLATFORM_URL"), "Platform dashboard URL")
	hiveID := flag.String("hive-id", os.Getenv("H1V3_HIVE_ID"), "Hive ID for platform mode")
	platformKey := flag.String("platform-key", os.Getenv("H1V3_PLATFORM_KEY"), "API key for platform auth")
	verbose := flag.Bool("v", false, "Verbose logging")
	flag.Parse()

	// Set up logging
	logLevel := slog.LevelInfo
	if *verbose {
		logLevel = slog.LevelDebug
	}
	logBuf := logbuf.New(2000)
	jsonHandler := slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: logLevel})
	logger := slog.New(logbuf.NewHandler(jsonHandler, logBuf))

	// Load config (3 modes: file, platform, env)
	var cfg *config.Config
	var err error
	if *configPath != "" {
		cfg, err = config.Load(*configPath)
	} else if *platformURL != "" {
		logger.Info("loading config from platform", "url", *platformURL, "hive_id", *hiveID)
		cfg, err = config.LoadFromPlatform(config.PlatformOptions{
			PlatformURL: *platformURL,
			HiveID:      *hiveID,
			APIKey:      *platformKey,
		})
	} else {
		cfg, err = config.LoadFromEnv()
	}
	if err != nil {
		logger.Error("failed to load config", "error", err)
		os.Exit(1)
	}

	logger.Info("h1v3d starting", "hive_id", cfg.Hive.ID)

	// 1. Initialize provider(s)
	providers := make(map[string]provider.Provider)
	for name, pcfg := range cfg.Providers {
		switch pcfg.Type {
		case "anthropic":
			var opts []provider.AnthropicOption
			if pcfg.BaseURL != "" {
				opts = append(opts, provider.WithAnthropicBaseURL(pcfg.BaseURL))
			}
			if pcfg.Model != "" {
				opts = append(opts, provider.WithAnthropicModel(pcfg.Model))
			}
			providers[name] = provider.NewAnthropic(pcfg.APIKey, opts...)
		default: // "openai" or empty
			var opts []provider.OpenAIOption
			if pcfg.BaseURL != "" {
				opts = append(opts, provider.WithBaseURL(pcfg.BaseURL))
			}
			if pcfg.Model != "" {
				opts = append(opts, provider.WithModel(pcfg.Model))
			}
			providers[name] = provider.NewOpenAI(pcfg.APIKey, opts...)
		}
		logger.Info("provider initialized", "name", name, "type", pcfg.Type, "model", pcfg.Model)
	}

	defaultProv, ok := providers["default"]
	if !ok {
		logger.Error("no 'default' provider configured")
		os.Exit(1)
	}

	// 2. Initialize ticket store + registry
	dbPath := cfg.Hive.DataDir + "/tickets.db"
	os.MkdirAll(cfg.Hive.DataDir, 0o755)
	store, err := ticket.NewSQLiteStore(dbPath)
	if err != nil {
		logger.Error("failed to open ticket store", "path", dbPath, "error", err)
		os.Exit(1)
	}
	// store will be cleaned up when the process exits

	reg := registry.New(store, logger)

	// Context for graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// 3. Register agents from config
	for _, spec := range cfg.Agents {
		// Create per-agent memory store
		mem := memory.NewStore(spec.Directory)

		// Create per-agent tool registry with whitelist/blacklist gating
		agentTools := tool.NewRegistry()
		register := func(t tool.Tool) {
			if spec.ToolAllowed(t.Name()) {
				agentTools.Register(t)
			}
		}
		register(&tool.ReadFileTool{AllowedDir: spec.Directory})
		register(&tool.WriteFileTool{AllowedDir: spec.Directory})
		register(&tool.EditFileTool{AllowedDir: spec.Directory})
		register(&tool.ListDirTool{AllowedDir: spec.Directory})
		register(&tool.ExecTool{WorkDir: spec.Directory})
		register(&tool.WebFetchTool{})
		if cfg.Tools.BraveAPIKey != "" {
			register(&tool.WebSearchTool{APIKey: cfg.Tools.BraveAPIKey})
		}
		// Memory tools bound to this agent's store
		register(&tool.ReadMemoryTool{Store: mem})
		register(&tool.WriteMemoryTool{Store: mem})
		register(&tool.ListMemoryTool{Store: mem})
		register(&tool.DeleteMemoryTool{Store: mem})
		// Hive discovery
		register(&tool.ListAgentsTool{Lister: &agentListerAdapter{reg: reg}})
		// Ticket tools — create, respond, close, search
		broker := &ticketBrokerAdapter{reg: reg}
		lister := &agentListerAdapter{reg: reg}
		register(&tool.CreateTicketTool{Broker: broker, AgentID: spec.ID, Agents: lister})
		register(&tool.RespondToTicketTool{Broker: broker, AgentID: spec.ID, Logger: logger.With("agent", spec.ID)})
		register(&tool.CloseTicketTool{Broker: broker, AgentID: spec.ID})
		register(&tool.SearchTicketsTool{Broker: broker, AgentID: spec.ID})
		register(&tool.GetTicketTool{Broker: broker})
		register(&tool.WaitTool{})

		// Select provider: per-agent override, then "default"
		prov := defaultProv
		if spec.Provider != "" {
			if p, ok := providers[spec.Provider]; ok {
				prov = p
			}
		}

		ag := agent.New(spec, prov, agentTools)
		ag.Memory = mem
		ag.Logger = logger.With("agent", spec.ID)

		if err := reg.RegisterAgent(spec, ag); err != nil {
			logger.Error("failed to register agent", "agent", spec.ID, "error", err)
			os.Exit(1)
		}

		// Start worker goroutine
		handle, _ := reg.GetAgent(spec.ID)
		worker := &agent.Worker{
			Agent:  ag,
			Inbox:  handle.Inbox,
			Router: reg,
		}
		go safeGo(logger, spec.ID, func() { worker.Start(ctx) })

		logger.Info("agent started", "agent", spec.ID, "role", spec.Role)
	}

	// 4. Start connectors
	if cfg.Connectors.Telegram != nil {
		// Determine which agent handles Telegram messages
		frontID := cfg.Connectors.Telegram.AgentID
		if frontID == "" && len(cfg.Agents) > 0 {
			frontID = cfg.Agents[0].ID
		}

		if _, ok := reg.GetAgent(frontID); !ok {
			logger.Warn("telegram agent not found, telegram connector will not start", "agent_id", frontID)
		} else {
			// Forward-declare tgConn so the handler/sink closures can reference it
			var tgConn *telegram.Connector

			// telegramSink delivers messages to Telegram when an agent
			// routes to "_external" via respond_to_ticket.
			sink := &telegramSink{
				ticketToChat: make(map[string]string),
				logger:       logger.With("component", "telegram-sink"),
			}
			sink.send = func(ctx context.Context, msg connector.OutboundMessage) error {
				return tgConn.Send(ctx, msg)
			}
			reg.RegisterSink("_external", sink)

			// SessionManager routes inbound messages to the front agent's inbox.
			sm := agent.NewSessionManager(frontID, reg, logger.With("component", "session-manager"))
			sm.OnSessionCreated = func(chatID, ticketID string) {
				sink.MapTicket(ticketID, chatID)
			}
			sm.OnSessionClosed = func(chatID string) {
				sink.UnmapChat(chatID)
			}

			tgHandler := func(ctx context.Context, msg connector.InboundMessage) error {
				cmd := msg.Content
				if cmd == "/new" || cmd == "/start" {
					sm.CloseSession(msg.ChatID)
					return tgConn.Send(ctx, connector.OutboundMessage{
						ChatID:  msg.ChatID,
						Content: "Starting a new conversation. Send me your message!",
					})
				}
				if cmd == "/parallel" || strings.HasPrefix(cmd, "/parallel ") {
					text := strings.TrimPrefix(cmd, "/parallel")
					text = strings.TrimSpace(text)
					if text == "" {
						text = "New parallel conversation"
					}
					ticketID, err := sm.StartParallelSession(msg.ChatID, text)
					if err != nil {
						return tgConn.Send(ctx, connector.OutboundMessage{
							ChatID:  msg.ChatID,
							Content: fmt.Sprintf("Failed to create parallel session: %v", err),
						})
					}
					_ = tgConn.Send(ctx, connector.OutboundMessage{
						ChatID:  msg.ChatID,
						Content: fmt.Sprintf("Parallel conversation started (ticket %s). Send your message!", ticketID),
					})
					if text != "New parallel conversation" {
						return sm.HandleInbound(msg.ChatID, text)
					}
					return nil
				}
				if strings.HasPrefix(cmd, "/close ") {
					ticketID := strings.TrimSpace(strings.TrimPrefix(cmd, "/close"))
					if ticketID == "" {
						return tgConn.Send(ctx, connector.OutboundMessage{
							ChatID:  msg.ChatID,
							Content: "Usage: /close <ticket_id>",
						})
					}
					if err := sm.CloseTicket(ticketID, "manually closed via /close"); err != nil {
						return tgConn.Send(ctx, connector.OutboundMessage{
							ChatID:  msg.ChatID,
							Content: fmt.Sprintf("Failed to close ticket: %v", err),
						})
					}
					return tgConn.Send(ctx, connector.OutboundMessage{
						ChatID:  msg.ChatID,
						Content: fmt.Sprintf("Ticket %s closed.", ticketID),
					})
				}
				return sm.HandleInbound(msg.ChatID, msg.Content)
			}

			var tgErr error
			tgConn, tgErr = telegram.New(
				telegram.Config{
					Token:     cfg.Connectors.Telegram.Token,
					AllowFrom: cfg.Connectors.Telegram.AllowFrom,
				},
				tgHandler,
				logger.With("connector", "telegram"),
			)
			if tgErr != nil {
				logger.Error("failed to init telegram connector", "error", tgErr)
				os.Exit(1)
			}

			go safeGo(logger, "telegram", func() { tgConn.Start(ctx) })
			logger.Info("telegram connector started")
		}
	}

	// 5. Start API server
	apiFrontID := cfg.Hive.FrontAgentID
	if apiFrontID == "" && len(cfg.Agents) > 0 {
		apiFrontID = cfg.Agents[0].ID
	}
	apiSvc := &hiveServiceAdapter{reg: reg, store: store, frontAgentID: apiFrontID}
	apiSrv := apiPkg.NewServer(apiSvc, apiPkg.Config{
		Host: cfg.API.Host,
		Port: cfg.API.Port,
		Key:  cfg.API.Key,
	}, logger.With("component", "api"), logBuf)

	go safeGo(logger, "api-server", func() { apiSrv.Start(ctx) })
	logger.Info("api server started", "port", cfg.API.Port)

	// 6. Graceful shutdown
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	sig := <-sigCh
	logger.Info("received signal, shutting down", "signal", sig)
	cancel()
	logger.Info("h1v3d stopped")
}

// safeGo runs fn with panic recovery.
func safeGo(logger *slog.Logger, name string, fn func()) {
	defer func() {
		if r := recover(); r != nil {
			logger.Error("goroutine panicked", "name", name, "panic", fmt.Sprintf("%v", r))
		}
	}()
	fn()
}

// hiveServiceAdapter implements api.HiveService using the registry.
type hiveServiceAdapter struct {
	reg          *registry.Registry
	store        ticket.Store
	frontAgentID string
}

func (h *hiveServiceAdapter) ListAgents() []apiPkg.AgentInfo {
	ids := h.reg.ListAgents()
	agents := make([]apiPkg.AgentInfo, len(ids))
	for i, id := range ids {
		handle, _ := h.reg.GetAgent(id)
		agents[i] = apiPkg.AgentInfo{
			ID:   id,
			Role: handle.Spec.Role,
		}
	}
	return agents
}

func (h *hiveServiceAdapter) GetAgent(id string) (*apiPkg.AgentInfo, bool) {
	handle, ok := h.reg.GetAgent(id)
	if !ok {
		return nil, false
	}
	return &apiPkg.AgentInfo{
		ID:   id,
		Role: handle.Spec.Role,
	}, true
}

func (h *hiveServiceAdapter) ListTickets(filter ticket.Filter) ([]*protocol.Ticket, error) {
	return h.reg.ListTickets(filter)
}

func (h *hiveServiceAdapter) GetTicket(id string) (*protocol.Ticket, error) {
	return h.reg.GetTicket(id)
}

func (h *hiveServiceAdapter) InjectMessage(from, ticketID, content string) (string, error) {
	if from == "" {
		from = "api"
	}

	// Auto-create a ticket if none provided
	if ticketID == "" {
		t, err := h.reg.CreateTicket(from, content, "", "", []string{h.frontAgentID}, nil)
		if err != nil {
			return "", fmt.Errorf("create ticket: %w", err)
		}
		ticketID = t.ID
	}

	msg := protocol.Message{
		From:      from,
		To:        []string{h.frontAgentID},
		Content:   content,
		TicketID:  ticketID,
		Timestamp: time.Now(),
	}
	return ticketID, h.reg.RouteMessage(msg)
}

// telegramSink implements registry.Sink — delivers messages to Telegram
// by looking up the chat ID for the message's ticket.
type telegramSink struct {
	mu           sync.Mutex
	ticketToChat map[string]string // ticketID → chatID
	send         func(ctx context.Context, msg connector.OutboundMessage) error
	logger       *slog.Logger
}

func (s *telegramSink) Deliver(msg protocol.Message) error {
	s.mu.Lock()
	chatID, ok := s.ticketToChat[msg.TicketID]
	s.mu.Unlock()
	if !ok {
		s.logger.Warn("no chat mapping for ticket", "ticket", msg.TicketID)
		return fmt.Errorf("telegram sink: no chat mapping for ticket %s", msg.TicketID)
	}
	return s.send(context.Background(), connector.OutboundMessage{
		ChatID:  chatID,
		Content: msg.Content,
	})
}

func (s *telegramSink) MapTicket(ticketID, chatID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.ticketToChat[ticketID] = chatID
}

func (s *telegramSink) UnmapChat(chatID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for tid, cid := range s.ticketToChat {
		if cid == chatID {
			delete(s.ticketToChat, tid)
		}
	}
}

// agentListerAdapter implements tool.AgentLister using the registry.
type agentListerAdapter struct {
	reg *registry.Registry
}

func (a *agentListerAdapter) ListAgentInfo() []tool.AgentInfo {
	ids := a.reg.ListAgents()
	agents := make([]tool.AgentInfo, 0, len(ids))
	for _, id := range ids {
		handle, ok := a.reg.GetAgent(id)
		if !ok {
			continue
		}
		agents = append(agents, tool.AgentInfo{
			ID:   id,
			Role: handle.Spec.Role,
		})
	}
	return agents
}

// ticketBrokerAdapter implements tool.TicketBroker using the registry.
type ticketBrokerAdapter struct {
	reg *registry.Registry
}

func (b *ticketBrokerAdapter) CreateTicket(from, title, goal, parentID string, to, tags []string) (*protocol.Ticket, error) {
	return b.reg.CreateTicket(from, title, goal, parentID, to, tags)
}

func (b *ticketBrokerAdapter) GetTicket(ticketID string) (*protocol.Ticket, error) {
	return b.reg.GetTicket(ticketID)
}

func (b *ticketBrokerAdapter) ListTickets(filter ticket.Filter) ([]*protocol.Ticket, error) {
	return b.reg.ListTickets(filter)
}

func (b *ticketBrokerAdapter) CountTickets(filter ticket.Filter) (int, error) {
	return b.reg.CountTickets(filter)
}

func (b *ticketBrokerAdapter) CloseTicket(ticketID, summary string) error {
	return b.reg.CloseTicket(ticketID, summary)
}

func (b *ticketBrokerAdapter) RouteMessage(msg protocol.Message) error {
	return b.reg.RouteMessage(msg)
}
