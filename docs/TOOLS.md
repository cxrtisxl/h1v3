# Built-in Tools

Every agent in h1v3 has access to built-in tools. Tools can be restricted per-agent using whitelist/blacklist configuration in the agent spec.

## Filesystem

| Tool | Description | Key Parameters |
|------|-------------|----------------|
| `read_file` | Read the contents of a file | `path` |
| `write_file` | Write content to a file (creates parent directories) | `path`, `content` |
| `edit_file` | Replace old_text with new_text in a file (must be unique match) | `path`, `old_text`, `new_text` |
| `list_dir` | List directory contents with file sizes | `path` |

All filesystem tools validate paths against the agent's `directory` setting.

## Shell

| Tool | Description | Key Parameters |
|------|-------------|----------------|
| `exec` | Execute a shell command and return output | `command` |

Safety guards: blocked command patterns, 60s timeout, 10KB output cap.

## Web

| Tool | Description | Key Parameters |
|------|-------------|----------------|
| `web_search` | Search the web via Brave Search API | `query` |
| `web_fetch` | Fetch a URL and extract readable text content | `url` |

`web_search` requires `tools.brave_api_key` in the config. If not set, the tool is not registered.

## Memory

| Tool | Description | Key Parameters |
|------|-------------|----------------|
| `read_memory` | Read the content of a memory scope | `scope` |
| `write_memory` | Write content to a memory scope (replaces existing) | `scope`, `content` |
| `list_memory` | List all memory scopes with content lengths | _(none)_ |
| `delete_memory` | Delete a memory scope | `scope` |

Each agent has its own isolated memory store backed by markdown files at `{directory}/memory/{scope}.md`.

## Tickets

| Tool | Description | Key Parameters |
|------|-------------|----------------|
| `create_ticket` | Create a ticket to delegate work to other agents | `to`, `title`, `goal`, `message` (optional), `tags` (optional) |
| `respond_to_ticket` | Send a message on an existing ticket | `ticket_id`, `message` |
| `close_ticket` | Close a ticket with a summary | `ticket_id`, `summary` |
| `search_tickets` | Search tickets by query, status, or participant | `query`, `status`, `participant`, `limit` |
| `get_ticket` | Get full ticket details including messages | `ticket_id` |
| `wait` | Stop processing and wait for sub-ticket results or new messages | _(none)_ |

## Discovery

| Tool | Description | Key Parameters |
|------|-------------|----------------|
| `list_agents` | List all agents in the hive with IDs and roles | _(none)_ |

## MCP (Dynamic)

MCP tools are discovered dynamically from configured MCP servers and registered as `mcp_{server}_{tool}`. These are not affected by whitelist/blacklist since they use dynamic names.

---

## Tool Whitelist / Blacklist

Control which tools an agent can access using `tools_whitelist` and `tools_blacklist` in the agent spec.

### Rules

1. **No lists** — all tools are available (default)
2. **Whitelist only** — only the listed tools are registered
3. **Blacklist only** — all tools except the listed ones are registered
4. **Both set** — whitelist takes precedence; blacklist is ignored

### Preset Examples

Restrict a writer agent from creating sub-tickets or running shell commands:

```json
{
  "id": "writer",
  "role": "Writer",
  "core_instructions": "...",
  "directory": "/data/agents/writer",
  "tools_blacklist": ["create_ticket", "exec"]
}
```

Give an agent only filesystem access:

```json
{
  "id": "file-worker",
  "role": "File processor",
  "core_instructions": "...",
  "directory": "/data/agents/file-worker",
  "tools_whitelist": ["read_file", "write_file", "edit_file", "list_dir", "respond_to_ticket", "close_ticket"]
}
```

Note: agents almost always need `respond_to_ticket` and `close_ticket` to participate in the ticket protocol. Include them in any whitelist.
