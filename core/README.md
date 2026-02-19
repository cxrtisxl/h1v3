# h1v3

Ticket-based multi-agent runtime in Go. Agents communicate through tickets (structured work units), use tools via a ReAct loop, and connect to external platforms through connectors.

## Architecture

```
                         +-----------+
  Telegram / Slack /     |  h1v3d    |     REST API
  Webhook  ───────────►  |  daemon   |  ◄──── h1v3ctl
                         +-----+-----+
                               |
            +------------------+------------------+
            |                  |                  |
      +-----+-----+     +-----+-----+     +-----+-----+
      |   front    |     |   coder   |     |  reviewer  |
      |   agent    |     |   agent   |     |   agent    |
      +-----+-----+     +-----+-----+     +-----+-----+
            |                  |                  |
            +----------+------+------+------------+
                       |             |
                 +-----+-----+ +----+------+
                 |  Ticket    | |  Memory   |
                 |  Store     | |  Store    |
                 |  (SQLite)  | |  (.md)    |
                 +-----------+ +-----------+
```

**Key concepts:**

- **Agents** run as goroutines, each with their own tool registry, memory store, and inbox
- **Front Agent** handles external messages synchronously — runs the ReAct loop inline, returns the response directly to the connector. Maintains per-chat sessions (`chatID → ticketID`) so conversation context is preserved across messages. `/new` and `/start` reset the session.
- **Tickets** are structured conversations routed between agents (stored in SQLite)
- **Tools** follow a ReAct loop: LLM calls tools, gets results, reasons, repeats
- **Connectors** bridge external platforms (Telegram, Slack, webhooks) to the front agent
- **Providers** abstract LLM backends (OpenAI-compatible, native Anthropic)

## Quick Start

### Prerequisites

- Go 1.24+
- An LLM API key (OpenAI, Anthropic, OpenRouter, or any OpenAI-compatible endpoint)

### Build

```bash
cd packages/h1v3
make build
```

This produces `bin/h1v3d` (daemon) and `bin/h1v3ctl` (CLI).

### Run the CLI (single agent, no daemon needed)

The fastest way to test. Runs one agent in an interactive REPL with filesystem, shell, and web tools:

```bash
# OpenAI (default provider)
OPENAI_API_KEY=sk-... bin/h1v3ctl run

# Anthropic (native API)
ANTHROPIC_API_KEY=sk-ant-... bin/h1v3ctl run --provider anthropic

# Anthropic via OpenRouter (uses OpenAI-compatible endpoint)
OPENAI_API_KEY=sk-or-... bin/h1v3ctl run \
  --model anthropic/claude-sonnet-4-20250514 \
  --base-url https://openrouter.ai/api/v1

# One-shot mode
OPENAI_API_KEY=sk-... bin/h1v3ctl run --prompt "list files in the current directory"

# With verbose logging and a specific working directory
OPENAI_API_KEY=sk-... bin/h1v3ctl run -v --work-dir /path/to/project
```

### Run the Daemon (multi-agent hive)

#### 1. Create a config file

Create `config.json`:

```json
{
  "hive": {
    "id": "my-hive",
    "data_dir": "/data",
    "front_agent_id": "front",
    "compact_threshold": 8000
  },
  "agents": [
    {
      "id": "front",
      "role": "Front desk agent",
      "core_instructions": "You are a helpful assistant. Greet users and answer their questions. Use your tools to read/write files and run commands.",
      "directory": "/data/agents/front"
    },
    {
      "id": "coder",
      "role": "Software engineer",
      "core_instructions": "You are a software engineer. Write clean, working code. Use filesystem and shell tools.",
      "directory": "/data/agents/coder"
    }
  ],
  "providers": {
    "default": {
      "type": "anthropic",
      "api_key": "sk-ant-...",
      "model": "claude-sonnet-4-20250514"
    }
  },
  "api": {
    "host": "0.0.0.0",
    "port": 8080,
    "api_key": "my-secret-key"
  }
}
```

#### 2. Run directly

```bash
bin/h1v3d --config config.json -v
```

#### 3. Or run with Docker

```bash
# Build the image
docker build -t h1v3 .

# Run with config mounted
docker run -it --rm \
  -p 8080:8080 \
  -v $(pwd)/config.json:/config.json \
  -v h1v3-data:/data \
  h1v3 h1v3d --config /config.json -v
```

### Interact with the Daemon

From another terminal:

```bash
export H1V3_API_URL=http://localhost:8080
export H1V3_API_KEY=my-secret-key

# Health check
bin/h1v3ctl health

# List agents
bin/h1v3ctl agents list

# Send a message (creates a ticket routed to the front agent)
curl -X POST http://localhost:8080/api/messages \
  -H "Authorization: Bearer my-secret-key" \
  -H "Content-Type: application/json" \
  -d '{"from": "user", "content": "Hello, what can you do?"}'

# List tickets to see the result
bin/h1v3ctl tickets list

# View a specific ticket with full conversation
bin/h1v3ctl tickets show <ticket-id>
```

## Configuration

### Config File (JSON)

See the example above. All fields:

| Field | Description |
|-------|-------------|
| `hive.id` | Unique hive identifier |
| `hive.data_dir` | Data directory for SQLite, agent workspaces, memory |
| `hive.front_agent_id` | Agent that receives external messages (default: `front`) |
| `hive.compact_threshold` | Token threshold for ticket compaction (default: 8000) |
| `agents[].id` | Unique agent ID |
| `agents[].role` | Human-readable role description |
| `agents[].core_instructions` | System prompt for the agent |
| `agents[].directory` | Agent's workspace directory |
| `agents[].wake_schedule` | Cron expression for periodic wake-ups (e.g., `@every 5m`) |
| `providers.<name>.type` | Provider type: `openai` (default) or `anthropic` |
| `providers.<name>.api_key` | LLM API key |
| `providers.<name>.model` | Model name |
| `providers.<name>.base_url` | Custom API base URL (for OpenRouter, local models, etc.) |
| `connectors.telegram.token` | Telegram bot token |
| `connectors.telegram.allow_from` | Array of allowed Telegram user IDs |
| `connectors.telegram.voice.whisper_api_key` | Whisper API key for voice transcription |
| `tools.brave_api_key` | Brave Search API key for web search |
| `api.host` | API listen host (default: `0.0.0.0`) |
| `api.port` | API listen port (default: `8080`) |
| `api.api_key` | Bearer token for API authentication |

### Environment Variables

When no config file is provided, the daemon reads from environment variables:

| Variable | Description |
|----------|-------------|
| `H1V3_HIVE_ID` | Hive ID (default: `default`) |
| `H1V3_DATA_DIR` | Data directory (default: `/data`) |
| `H1V3_ANTHROPIC_API_KEY` | Anthropic API key (takes precedence over OpenAI) |
| `H1V3_OPENAI_API_KEY` | OpenAI-compatible API key |
| `H1V3_OPENAI_BASE_URL` | Custom base URL (OpenAI provider only) |
| `H1V3_MODEL` | Model name (defaults depend on provider) |
| `H1V3_API_HOST` | API listen host (default: `0.0.0.0`) |
| `H1V3_API_PORT` | API listen port (default: `8080`) |
| `H1V3_API_KEY` | API auth key |
| `H1V3_TELEGRAM_TOKEN` | Telegram bot token |
| `H1V3_TELEGRAM_ALLOW_FROM` | Comma-separated Telegram user IDs |
| `H1V3_BRAVE_API_KEY` | Brave Search API key |
| `H1V3_FRONT_AGENT_ID` | Front agent ID (default: `front`) |
| `H1V3_COMPACT_THRESHOLD` | Compaction threshold (default: `8000`) |

## REST API

All endpoints except `/api/health` require `Authorization: Bearer <api_key>`.

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/api/health` | Health check |
| `GET` | `/api/agents` | List all agents |
| `GET` | `/api/agents/{id}` | Get agent details |
| `GET` | `/api/tickets` | List tickets (`?status=open&agent=front&limit=50`) |
| `GET` | `/api/tickets/{id}` | Get ticket with messages |
| `POST` | `/api/messages` | Send a message `{"from", "ticket_id", "content"}` |

## Telegram

To connect a Telegram bot, set the token in `config.json`:

```json
{
  "connectors": {
    "telegram": {
      "token": "123456:ABC-DEF...",
      "allow_from": [123456789]
    }
  }
}
```

Or via environment variable: `H1V3_TELEGRAM_TOKEN`. Optionally restrict access with `H1V3_TELEGRAM_ALLOW_FROM` (comma-separated user IDs).

Get a bot token from [@BotFather](https://t.me/BotFather) on Telegram.

When a Telegram connector is configured, the bot maintains a per-chat session. Each chat gets its own ticket, and messages within the same chat accumulate as conversation history so the agent has full context.

**Commands:**

| Command | Description |
|---------|-------------|
| `/start` | Reset the session and start a new conversation |
| `/new` | Same as `/start` — closes the current ticket, starts fresh |
| `/help` | Show available commands |

Session lifecycle:
1. First message from a chat creates a ticket and a session mapping
2. Subsequent messages reuse the same ticket (conversation context preserved)
3. `/new` or `/start` closes the current ticket and clears the session
4. The next message after reset creates a new ticket

## Tools

Every agent gets these tools by default:

| Tool | Description |
|------|-------------|
| `read_file` | Read file contents |
| `write_file` | Write/create a file |
| `edit_file` | Search-and-replace edit |
| `list_dir` | List directory contents |
| `exec` | Execute shell commands |
| `web_fetch` | Fetch a URL and extract content |
| `web_search` | Search the web (requires Brave API key) |
| `read_memory` | Read agent's scoped memory |
| `write_memory` | Write to agent's scoped memory |
| `list_memory` | List memory scopes |
| `delete_memory` | Delete a memory scope |

MCP tools from external servers are registered with the `mcp_{server}_{tool}` prefix.

## Project Structure

```
cmd/
  h1v3d/          Daemon entry point
  h1v3ctl/        CLI entry point
internal/
  agent/          Agent core, ReAct loop, skills, sub-agents
  api/            REST API server
  config/         Configuration loading (file, env, platform)
  connector/
    telegram/     Telegram bot (long-poll)
    slack/        Slack bot (Socket Mode)
    webhook/      Generic HTTP webhook ingress
  memory/         Scoped memory store + consolidation
  provider/       LLM providers (OpenAI, Anthropic)
  registry/       Agent registry, ticket routing, compaction
  scheduler/      Cron-based agent wake-up scheduling
  ticket/         Ticket store (SQLite)
  tool/           Tool interface, built-in tools, MCP client
pkg/
  protocol/       Shared types (messages, tickets, agents, LLM)
```

## Tests

```bash
make test
```

Runs all tests across all packages. No external services required (everything uses mocks/httptest).
