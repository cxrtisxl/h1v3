# h1v3 Documentation

h1v3 is a ticket-based multi-agent runtime. It runs a "hive" of AI agents that communicate by creating and routing structured work units called **tickets**. Agents use a ReAct (Reason + Act) loop, calling tools iteratively until they produce a final response.

## Components

| Component | Description |
|-----------|-------------|
| **[core/](../core/)** | Go daemon (`h1v3d`) and CLI (`h1v3ctl`) |
| **[monitor/](../monitor/)** | Next.js dashboard for real-time inspection |

The monitor is a standalone web app. It connects to the daemon exclusively through the [REST API](#rest-api) -- it is not a Connector and has no special integration. Any HTTP client (the monitor, `h1v3ctl`, `curl`) can use the same API.

## Documentation

- **[Core Modules](core.md)** -- every Go package in `core/`
- **[Monitor](monitor.md)** -- the Next.js dashboard
- **[Data Flows](data-flows.md)** -- how data moves between components

## Core Concepts

### Tickets

A ticket is the atomic unit of work. Every interaction -- whether from a user via Telegram or from one agent delegating to another -- lives inside a ticket. Tickets have a title, goal, status (open/closed), creator, assignees (`waiting_on`), tags, optional parent (for sub-tickets), and an ordered list of messages.

### Agents

An agent is a long-lived goroutine with an LLM provider, a tool registry, and a persistent memory store. It reads from an inbox channel and responds by running the ReAct loop. Agents can create sub-tickets that are automatically parented to whatever ticket they are currently processing.

### ReAct Loop

The loop runs up to 20 iterations:

1. Send messages (system prompt + history) to the LLM provider
2. If the LLM returns tool calls, execute each one and append results to history
3. Repeat until the LLM returns a plain text response (or `respond_to_ticket` is called)
4. Return the final response

### Registry

The central message broker. Holds all agents, their inbox channels, and "sinks" (e.g. Telegram). Routes messages by looking up target agent IDs and pushing to their inbox channels. Manages the ticket lifecycle.

### Connectors

External platform integrations (Telegram, Slack, webhooks) that bridge inbound user messages into the ticket system and deliver agent responses back out.

### REST API

The daemon exposes a REST API ([`core/internal/api/server.go`](../core/internal/api/server.go)) for inspecting and interacting with the hive. Authenticated via Bearer token.

| Method | Path | Description |
|--------|------|-------------|
| GET | `/api/health` | Health check (no auth) |
| GET | `/api/agents` | List all agents |
| GET | `/api/agents/{id}` | Get single agent |
| GET | `/api/tickets` | List tickets (query: status, agent, parent_id, limit) |
| GET | `/api/tickets/{id}` | Get ticket with messages |
| POST | `/api/messages` | Inject message (auto-creates ticket if none specified) |
| GET | `/api/logs` | Buffered log entries (query: limit, level, since) |

LLM prompt context is captured via structured log entries (message `"prompt_context"` with the full LLM input as a JSON attribute). These are stored in the in-memory log buffer and served through `GET /api/logs` like any other log entry. The monitor matches them to messages by `msg_id` to display the prompt context dialog.
