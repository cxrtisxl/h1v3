# Data Flows

## 1. External Message to Agent Response (Telegram/Slack)

A user sends a message on Telegram or Slack. It flows through the connector, session manager, registry, agent worker, LLM, and back out.

```
User (Telegram/Slack)
  |
  v
Connector.handleUpdate()
  |  telegram: core/internal/connector/telegram/telegram.go
  |  slack:    core/internal/connector/slack/slack.go
  v
InboundHandler (defined in core/cmd/h1v3d/main.go)
  |
  v
SessionManager.HandleInbound(chatID, content)       -- core/internal/agent/front.go
  |
  |-- Find existing session (chatID -> ticketID) or create new ticket
  |   via registry.CreateTicket()                    -- core/internal/registry/registry.go
  |
  v
registry.RouteMessage(msg{from:"_external", to:[frontAgentID]})
  |                                                  -- core/internal/registry/registry.go
  |-- Persist message to SQLite                      -- core/internal/ticket/sqlite.go
  |-- Push to frontAgent.Inbox channel
  |
  v
Worker.handleMessage()                               -- core/internal/agent/worker.go
  |
  |-- Load ticket from store
  |-- Load sub-tickets for context
  |-- BuildSystemPrompt(ticket, subTickets)           -- core/internal/agent/context.go
  |-- Convert ticket messages to LLM format
  |
  v
agent.RunWithHistory() -- ReAct Loop                 -- core/internal/agent/loop.go
  |
  |   +---> provider.Chat(request)                   -- core/internal/provider/*.go
  |   |       |
  |   |       v
  |   |     LLM API (OpenAI / Anthropic / OpenRouter)
  |   |       |
  |   |       v
  |   |     ChatResponse (text or tool_calls)
  |   |       |
  |   |  [if tool_calls]
  |   |       |
  |   |       v
  |   |     tool.Registry.Execute(name, params)      -- core/internal/tool/registry.go
  |   |       |
  |   |       v
  |   |     Tool result appended to history
  |   |       |
  |   +-------+  (repeat up to 20 iterations)
  |
  |  [when plain text response or respond_to_ticket called]
  |
  v
Worker routes response: RouteMessage(msg{from:agentID, to:["_external"]})
  |                                                  -- core/internal/registry/registry.go
  |-- Persist to SQLite
  |-- Lookup sink "_external"
  |
  v
ConnectorSink.Deliver(chatID, content)
  |  telegramSink: defined in core/cmd/h1v3d/main.go
  |  slackSink:    defined in core/cmd/h1v3d/main.go
  |
  v
Connector.Send() -> Telegram Bot API / Slack API
  |
  v
User sees response
```

**Key files in this flow:**

- [`core/internal/connector/telegram/telegram.go`](../core/internal/connector/telegram/telegram.go) -- inbound message handling
- [`core/internal/agent/front.go`](../core/internal/agent/front.go) -- session management
- [`core/internal/registry/registry.go`](../core/internal/registry/registry.go) -- message routing and persistence
- [`core/internal/agent/worker.go`](../core/internal/agent/worker.go) -- message-to-agent dispatch
- [`core/internal/agent/context.go`](../core/internal/agent/context.go) -- system prompt assembly
- [`core/internal/agent/loop.go`](../core/internal/agent/loop.go) -- ReAct loop
- [`core/internal/provider/openai.go`](../core/internal/provider/openai.go) / [`anthropic.go`](../core/internal/provider/anthropic.go) -- LLM API calls

---

## 2. Agent Delegation via Sub-Tickets

An agent (e.g. the front agent) delegates work to another agent by creating a sub-ticket. The child agent works on it, closes it, and the result flows back to the parent.

```
frontAgent ReAct loop calls create_ticket tool
  |                                                  -- core/internal/tool/tickets.go
  |-- CreateTicketTool.Execute():
  |     registry.CreateTicket(parentID=currentTicket) -- core/internal/registry/registry.go
  |     registry.RouteMessage(initial msg to coderAgent)
  |
  v
frontAgent calls wait tool                          -- core/internal/tool/tickets.go
  |-- Sets responded=true, loop exits
  |-- Worker does NOT send auto-response
  |
  v
coderAgent.Inbox <- message pushed
  |
  v
coderAgent Worker.handleMessage()                   -- core/internal/agent/worker.go
  |
  v
coderAgent ReAct loop                               -- core/internal/agent/loop.go
  |-- (does work: reads files, runs commands, etc.)
  |
  v
coderAgent calls respond_to_ticket + close_ticket
  |                                                  -- core/internal/tool/tickets.go
  |
  |-- respond_to_ticket: message deferred (same ticket)
  |-- close_ticket:
  |     registry.CloseTicket()                       -- core/internal/registry/registry.go
  |       |
  |       |-- Mark ticket closed in SQLite
  |       |-- relayToParent():
  |       |     Inject full child conversation into parent ticket
  |       |     Route "_system" message to frontAgent
  |       |
  |       v
  |     frontAgent.Inbox <- system message with sub-ticket results
  |
  v
frontAgent Worker.handleMessage() (woken by relay)
  |
  |-- Sees sub-ticket results in ticket messages
  |-- Runs ReAct loop
  |-- Calls respond_to_ticket on own ticket
  |
  v
Response delivered to "_external" -> Connector -> User
```

**Key files in this flow:**

- [`core/internal/tool/tickets.go`](../core/internal/tool/tickets.go) -- create_ticket, respond_to_ticket, close_ticket, wait
- [`core/internal/registry/registry.go`](../core/internal/registry/registry.go) -- `relayToParent()` is the critical piece that wires child results back to parents

**Design details:**

- `create_ticket` auto-sets `parentID` from the agent's current ticket context
- `wait` prevents the agent from sending an auto-response, letting it sleep until the sub-ticket resolves
- When a child ticket closes, `relayToParent` injects the **full child conversation** into the parent ticket, giving the parent agent complete visibility
- The parent agent is woken with a `_system` message so it can process the results

---

## 3. API Message Injection

External systems can inject messages via the REST API, triggering agent processing.

```
HTTP Client
  |
  v
POST /api/messages {from, ticket_id, content}
  |                                                  -- core/internal/api/server.go
  v
server.handlePostMessage()
  |-- Auth check (Bearer token)
  |
  v
hiveServiceAdapter.InjectMessage()                   -- core/cmd/h1v3d/main.go
  |
  |-- If no ticket_id: registry.CreateTicket()
  |-- registry.RouteMessage(msg{from, to:[frontAgentID]})
  |     |
  |     |-- Persist to SQLite
  |     |-- Push to frontAgent.Inbox
  |
  v
Returns {status:"accepted", ticket_id}

  [async] frontAgent processes message (same as flow 1)
```

**Key files:**

- [`core/internal/api/server.go`](../core/internal/api/server.go) -- REST endpoint
- [`core/cmd/h1v3d/main.go`](../core/cmd/h1v3d/main.go) -- `hiveServiceAdapter` bridges API to registry

---

## 4. Webhook Inbound

External services (e.g. GitHub) push events via webhooks, which are converted into messages.

```
External Service (e.g. GitHub)
  |
  v
POST /api/webhook/{name}
  |                                                  -- core/internal/connector/webhook/webhook.go
  v
webhook.Handler.ServeHTTP()
  |-- Verify auth (HMAC-SHA256 or Bearer token)
  |-- Parse WebhookPayload{sender_id, chat_id, content, metadata}
  |-- Append metadata as JSON to content
  |
  v
InboundHandler(ctx, InboundMessage)
  |
  v
SessionManager.HandleInbound()                       -- core/internal/agent/front.go
  |
  v
(same flow as #1 from here)
```

**Key files:**

- [`core/internal/connector/webhook/webhook.go`](../core/internal/connector/webhook/webhook.go) -- webhook auth and parsing

---

## 5. Telegram Voice Message

Voice messages take an extra transcription step before entering the standard flow.

```
User sends voice message on Telegram
  |
  v
telegram.Connector.handleUpdate()                    -- core/internal/connector/telegram/telegram.go
  |-- Detects voice message
  |
  v
voice.Transcribe(bot, voiceMsg)                      -- core/internal/connector/telegram/voice.go
  |-- Download audio file from Telegram servers
  |-- Save to temp file
  |-- POST multipart/form-data to Whisper API (Groq)
  |-- Return transcript
  |
  v
Content = "[Voice message]: {transcript}"
  |
  v
InboundHandler (same as text message flow #1)
```

---

## 6. Tool Execution During ReAct Loop

Within a single ReAct iteration, here is how tool calls are processed.

```
provider.Chat() returns ChatResponse with ToolCalls
  |                                                  -- core/internal/agent/loop.go
  v
For each ToolCall:
  |
  v
tool.Registry.Execute(name, params)                  -- core/internal/tool/registry.go
  |
  |-- Lookup tool by name
  |
  |-- [filesystem tools]                             -- core/internal/tool/filesystem.go
  |     Validate path against AllowedDir
  |     Read/Write/Edit/List file
  |
  |-- [shell tool]                                   -- core/internal/tool/shell.go
  |     Check against blocked patterns
  |     Execute via sh -c with timeout
  |     Cap output at 10KB
  |
  |-- [web tools]                                    -- core/internal/tool/web.go
  |     web_search: Brave Search API
  |     web_fetch: HTTP GET + readability extraction
  |
  |-- [memory tools]                                 -- core/internal/tool/memory.go
  |     Read/Write/List/Delete memory scopes
  |     Backed by markdown files on disk
  |
  |-- [ticket tools]                                 -- core/internal/tool/tickets.go
  |     create_ticket: Create + route (see flow #2)
  |     respond_to_ticket: Send message on ticket
  |     close_ticket: Close + relay to parent
  |     search_tickets / get_ticket: Query store
  |     wait: Signal no auto-response needed
  |
  |-- [mcp tools]                                    -- core/internal/tool/mcp.go
  |     Forward to MCP server via stdio or HTTP
  |
  v
Tool result string appended as tool_result message
  |
  v
Next iteration of ReAct loop
```

---

## 7. Configuration Loading

Three strategies for loading config at startup.

```
Strategy A: JSON file (--config flag)
  core/internal/config/config.go: Load(path)
  |-- Read file, unmarshal JSON

Strategy B: Environment variables
  core/internal/config/config.go: LoadFromEnv()
  |-- Read H1V3_* env vars
  |-- Parse JSON values for complex fields

Strategy C: Platform API (--platform-url flag)
  core/internal/config/platform.go: LoadFromPlatform(opts)
  |-- GET {platformURL}/api/hives/config
  |     Headers: Authorization: Bearer {key}, X-Hive-ID: {id}
  |-- Create agent workspace directories
  |-- Write SOUL.md identity files
  |-- Return Config
```

All three converge to the same `Config` struct, which drives the rest of daemon startup in [`core/cmd/h1v3d/main.go`](../core/cmd/h1v3d/main.go).
