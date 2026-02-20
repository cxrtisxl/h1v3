# Core Modules

The Go backend lives in [`core/`](../core/). It produces two binaries: the daemon (`h1v3d`) and the CLI (`h1v3ctl`).

## Entry Points

### `cmd/h1v3d` -- Daemon

[`core/cmd/h1v3d/main.go`](../core/cmd/h1v3d/main.go)

Startup sequence:

1. Parse flags: `--config`, `--platform-url`, `--hive-id`, `--platform-key`, `-v`
2. Load config (file, platform API, or env vars)
3. Initialize LLM providers (one or more named providers)
4. Open SQLite ticket store at `{data_dir}/tickets.db`
5. Create the registry
6. For each agent spec: create memory store, tool registry (all built-in + ticket tools), agent, register in registry, start worker goroutine
7. Start Telegram/Slack connectors if configured
8. Start REST API server
9. Block on SIGINT/SIGTERM, then gracefully shut down

### `cmd/h1v3ctl` -- CLI

[`core/cmd/h1v3ctl/main.go`](../core/cmd/h1v3ctl/main.go)

Two modes:

- **`run`**: Single-agent interactive REPL or one-shot mode. Creates a standalone agent with filesystem/shell/web tools and runs it directly (no daemon, no tickets).
- **API client commands**: `health`, `agents list/show`, `tickets list/show`, `config validate` -- all call the daemon's REST API using `H1V3_API_URL` and `H1V3_API_KEY`.

---

## Configuration (`internal/config`)

| File | Description |
|------|-------------|
| [`config.go`](../core/internal/config/config.go) | Full config schema and three loading strategies: JSON file (`Load`), env vars with `H1V3_` prefix (`LoadFromEnv`), or remote platform (`LoadFromPlatform`) |
| [`platform.go`](../core/internal/config/platform.go) | Fetches config from a remote platform dashboard (`GET /api/hives/config`). Sets up agent workspace directories and writes `SOUL.md` identity files |

Config struct hierarchy:

```
Config
+-- HiveConfig           id, data_dir, front_agent_id, compact_threshold
+-- []AgentSpec          id, role, provider, core_instructions, directory, wake_schedule, scoped_contexts, tools, skills
+-- map[name]ProviderConfig   type (openai|anthropic), api_key, model, base_url
+-- ConnectorConfig      telegram{token, allow_from}, slack{bot_token, app_token, allow_from}
+-- ToolsConfig          brave_api_key, shell_timeout, blocked_commands
+-- APIConfig            host, port, api_key
```

See [`core/config.example.json`](../core/config.example.json) for a working example with multiple providers.

---

## Public Types (`pkg/protocol`)

Shared types used across all packages.

| File | Types | Description |
|------|-------|-------------|
| [`agent.go`](../core/pkg/protocol/agent.go) | `AgentSpec` | Configuration/identity of a persistent agent |
| [`ticket.go`](../core/pkg/protocol/ticket.go) | `Ticket` | Core data structure: ID, title, goal, status, creator, assignees, messages, tags, parent_id, summary, timestamps |
| [`message.go`](../core/pkg/protocol/message.go) | `Message` | Unit of communication: from, to (array), content, ticket_id, timestamp |
| [`llm.go`](../core/pkg/protocol/llm.go) | `ChatMessage`, `ChatRequest`, `ChatResponse`, `ToolCall`, `Usage` | Provider-agnostic normalized LLM message format |
| [`tool.go`](../core/pkg/protocol/tool.go) | `ToolDefinition`, `ToolFunctionSchema` | OpenAI function-calling format for describing tools to LLMs |

---

## Agent Package (`internal/agent`)

| File | Description |
|------|-------------|
| [`agent.go`](../core/internal/agent/agent.go) | `Agent` struct: holds spec, provider, tool registry, memory store. `MaxIterations` defaults to 20 |
| [`loop.go`](../core/internal/agent/loop.go) | The ReAct loop. `Run()` and `RunWithHistory()` send messages to the provider, execute tool calls, append results, and repeat. Exits early if `respond_to_ticket` was called |
| [`worker.go`](../core/internal/agent/worker.go) | `Worker` wraps an Agent with an inbox channel. Reads messages, loads the ticket from the store, builds system prompt, runs `RunWithHistory`, flushes deferred messages, routes auto-response. Retries up to 3 times on error |
| [`context.go`](../core/internal/agent/context.go) | `BuildSystemPrompt` -- assembles layered system prompt from: agent identity, timestamp, scoped contexts, dynamic memory, current ticket details, sub-ticket summaries, available tools, and platform rules (ticket lifecycle protocol) |
| [`front.go`](../core/internal/agent/front.go) | `SessionManager` -- tracks chatID-to-ticketID sessions for external platforms. Creates or finds sessions and routes messages to the front agent |
| [`skills.go`](../core/internal/agent/skills.go) | `SkillsLoader` -- reads skill definitions from `{agentDir}/skills/` subdirectories. Each skill has `SKILL.md` + optional `config.json`. Supports `always_load` skills |
| [`subagent.go`](../core/internal/agent/subagent.go) | `SubAgent` -- ephemeral one-shot worker spawned from a parent agent. Gets only "safe" tools (no ticket/spawn tools). Max 15 iterations. Infrastructure for future use |

---

## Tool Package (`internal/tool`)

All tools implement the `Tool` interface: `Name()`, `Description()`, `Parameters()` (JSON Schema), `Execute(ctx, params)`.

| File | Tools | Description |
|------|-------|-------------|
| [`tool.go`](../core/internal/tool/tool.go) | `Tool` interface | Core tool abstraction |
| [`registry.go`](../core/internal/tool/registry.go) | `Registry` | Thread-safe map of tool name to Tool. Register/Get/List/Execute |
| [`filesystem.go`](../core/internal/tool/filesystem.go) | `read_file`, `write_file`, `edit_file`, `list_dir` | File operations. All validate paths against `AllowedDir` |
| [`shell.go`](../core/internal/tool/shell.go) | `exec` | Runs shell commands via `sh -c`. Blocked patterns list, 60s timeout, 10KB output cap |
| [`web.go`](../core/internal/tool/web.go) | `web_search`, `web_fetch` | Brave Search API for search; URL fetch with `go-readability` for HTML extraction |
| [`memory.go`](../core/internal/tool/memory.go) | `read_memory`, `write_memory`, `list_memory`, `delete_memory` | CRUD over the agent's `memory.Store` |
| [`tickets.go`](../core/internal/tool/tickets.go) | `create_ticket`, `respond_to_ticket`, `close_ticket`, `search_tickets`, `get_ticket`, `wait` | The primary inter-agent communication mechanism. See [Data Flows](data-flows.md) for details |
| [`list_agents.go`](../core/internal/tool/list_agents.go) | `list_agents` | Returns all agents with IDs and roles |
| [`mcp.go`](../core/internal/tool/mcp.go) | MCP tools (`mcp_{server}_{tool}`) | Full MCP (Model Context Protocol) client. Supports stdio and HTTP transports. Discovers tools via `tools/list` and wraps each as a `Tool` |

Key design in `tickets.go`:

- **Deferred messages**: When `respond_to_ticket` targets the current ticket, the message is buffered. If `close_ticket` is called in the same turn, the buffered message is suppressed (ticket is closed).
- **Context-carried state**: Current ticket ID, responded flag, deferred messages, and input messages are carried via `context.Context` values.
- **`wait` tool**: Marks `responded=true` and returns immediately, telling the Worker not to send an auto-response. The agent is woken again when a sub-ticket resolves or a new message arrives.

---

## Registry Package (`internal/registry`)

| File | Description |
|------|-------------|
| [`registry.go`](../core/internal/registry/registry.go) | Central message broker. `RegisterAgent`/`DeregisterAgent` manages agents and their inbox channels (buffered, size 64). `RouteMessage` persists to SQLite then delivers to inboxes or sinks. `CloseTicket` marks closed; if a child ticket, calls `relayToParent` to inject the full child conversation into the parent ticket and wake the parent's creator agent |
| [`agent_tools.go`](../core/internal/registry/agent_tools.go) | `CreateAgentTool` and `DestroyAgentTool` for dynamic agent lifecycle. Only the creator can destroy an agent |
| [`compact.go`](../core/internal/registry/compact.go) | `Compactor` -- reduces ticket token count by summarizing old messages via LLM. Keeps last 4 messages, replaces the rest with a summary. Defined but not yet wired into startup |
| [`id.go`](../core/internal/registry/id.go) | `generateID()` -- 8 random bytes as hex |

---

## Ticket Package (`internal/ticket`)

| File | Description |
|------|-------------|
| [`store.go`](../core/internal/ticket/store.go) | `Store` interface: `Save`, `Get`, `List(Filter)`, `Count(Filter)`, `AppendMessage`, `UpdateStatus`, `Close`. `Filter` supports status, agentID, tags, text query, parentID, limit |
| [`sqlite.go`](../core/internal/ticket/sqlite.go) | SQLite implementation using `modernc.org/sqlite` (pure Go, no CGO). Two tables: `tickets` and `ticket_messages`. WAL mode for concurrent reads. Idempotent schema migrations |

---

## Provider Package (`internal/provider`)

| File | Description |
|------|-------------|
| [`provider.go`](../core/internal/provider/provider.go) | `Provider` interface: `Chat(ctx, ChatRequest) (*ChatResponse, error)`, `Name() string` |
| [`openai.go`](../core/internal/provider/openai.go) | `OpenAIProvider` -- HTTP client for any OpenAI-compatible API (OpenAI, OpenRouter, DeepSeek, Groq, local models). Default model `gpt-4o` |
| [`anthropic.go`](../core/internal/provider/anthropic.go) | `AnthropicProvider` -- native Anthropic Messages API. Default model `claude-sonnet-4-20250514`. Handles content block format and extracts system messages into top-level `system` field |

---

## Memory Package (`internal/memory`)

| File | Description |
|------|-------------|
| [`store.go`](../core/internal/memory/store.go) | `Store` -- scoped persistent memory backed by markdown files at `{agentDir}/memory/{scope}.md`. In-memory cache loaded at startup. Thread-safe |
| [`consolidate.go`](../core/internal/memory/consolidate.go) | `Consolidator` -- extracts learnings from closed tickets into agent memory via LLM. Standard scopes: `project`, `preferences`, `team`. Defined but not currently called |

---

## Connector Package (`internal/connector`)

| File | Description |
|------|-------------|
| [`connector.go`](../core/internal/connector/connector.go) | `Connector` interface: `Name()`, `Start(ctx)`, `Stop()`, `Send(ctx, OutboundMessage)`. `InboundHandler` function type |

### Telegram (`internal/connector/telegram`)

| File | Description |
|------|-------------|
| [`telegram.go`](../core/internal/connector/telegram/telegram.go) | Long-polling Telegram bot. Handles text, captions, voice messages. Access control via `AllowFrom` user ID list. Commands: `/help` (local), `/start` and `/new` (forwarded to session manager) |
| [`format.go`](../core/internal/connector/telegram/format.go) | `MarkdownToTelegramHTML` and `StripMarkdown` for Telegram-compatible formatting |
| [`voice.go`](../core/internal/connector/telegram/voice.go) | Voice transcription via Whisper API (default Groq endpoint). Downloads audio, POSTs to Whisper, returns transcript |

### Slack (`internal/connector/slack`)

| File | Description |
|------|-------------|
| [`slack.go`](../core/internal/connector/slack/slack.go) | Slack Socket Mode connector. Handles `MessageEvent`, `AppMentionEvent`, and slash commands. Thread-aware: uses `channel:thread_ts` as chatID. Converts Markdown to Slack mrkdwn |

### Webhook (`internal/connector/webhook`)

| File | Description |
|------|-------------|
| [`webhook.go`](../core/internal/connector/webhook/webhook.go) | Generic HTTP webhook at `/api/webhook/{name}`. HMAC-SHA256 or Bearer token auth. Parses `WebhookPayload{sender_id, chat_id, content, metadata}` |

---

## API Package (`internal/api`)

[`server.go`](../core/internal/api/server.go)

REST API server with CORS middleware and Bearer auth. See [README](README.md#rest-api) for the endpoint table.

---

## Utility Packages

| Package | File | Description |
|---------|------|-------------|
| `internal/logbuf` | [`logbuf.go`](../core/internal/logbuf/logbuf.go) | Thread-safe ring buffer (2000 entries) for log storage |
| `internal/logbuf` | [`handler.go`](../core/internal/logbuf/handler.go) | `slog.Handler` that writes to both the ring buffer and stdout JSON |
| `internal/scheduler` | [`scheduler.go`](../core/internal/scheduler/scheduler.go) | Cron-based agent wake-up using `robfig/cron/v3`. Defined but not currently started |

---

## Infrastructure

| File | Description |
|------|-------------|
| [`core/Dockerfile`](../core/Dockerfile) | Multi-stage: Go 1.24 Alpine builder, Alpine 3.21 runtime. VOLUME `/data`, EXPOSE 8080. Healthcheck via `/api/health` |
| [`core/Makefile`](../core/Makefile) | `build`, `test`, `lint`, `cold-start` targets |
| [`core/go.mod`](../core/go.mod) | Module `github.com/h1v3-io/h1v3`. Key deps: `modernc.org/sqlite`, `go-readability`, `telegram-bot-api/v5`, `slack-go/slack`, `robfig/cron/v3` |
| [`.github/workflows/build.yml`](../.github/workflows/build.yml) | CI: run tests, then build+push Docker image to `ghcr.io` tagged with git SHA and `latest` |
