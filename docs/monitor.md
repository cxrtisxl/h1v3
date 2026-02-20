# Monitor

A Next.js 14 (App Router) dashboard styled with Tailwind CSS and shadcn/ui. Lives in [`monitor/`](../monitor/).

The monitor is a standalone web app with **no special integration** into the daemon. It uses the same [REST API](README.md#rest-api) that `h1v3ctl` or any HTTP client can use. Authentication is a Bearer token stored in `localStorage` -- there is no server-side session.

## How It Connects

```
Browser
  |
  |-- localStorage: { apiUrl, apiKey }               -- monitor/lib/auth.ts
  |
  |-- fetch() with Authorization: Bearer {apiKey}    -- monitor/lib/api.ts
  |     GET /api/agents
  |     GET /api/tickets
  |     GET /api/tickets/{id}
  |     GET /api/logs
  |     POST /api/messages
  |
  v
daemon REST API                                      -- core/internal/api/server.go
```

There is no WebSocket or persistent connection. The monitor uses polling:

- **Ticket detail page**: auto-refreshes every 3s while the ticket is open
- **Logs page**: polls every 2s with incremental `since` parameter

## Prompt Context

LLM prompt context (the full input sent to the LLM for each agent response) is captured as structured log entries with message `"prompt_context"`. These flow through the normal log system:

1. Agent worker or `respond_to_ticket` tool logs a `"prompt_context"` entry with the full LLM message array as a JSON attribute, keyed by `msg_id`
2. Entry lands in the in-memory ring buffer ([`core/internal/logbuf/logbuf.go`](../core/internal/logbuf/logbuf.go))
3. Monitor fetches via `GET /api/logs`
4. Monitor matches log entries to messages by `msg_id` and shows the prompt context dialog

## Lib

| File | Description |
|------|-------------|
| [`lib/auth.ts`](../monitor/lib/auth.ts) | Stores API URL and key in `localStorage`. `isAuthenticated()` checks presence; `clearAuth()` removes |
| [`lib/api.ts`](../monitor/lib/api.ts) | Typed fetch wrappers: `fetchAgents`, `fetchTickets`, `fetchTicket`, `fetchLogs`, `postMessage`. Auto-redirects to `/login` on 401 |

## Pages

| File | Description |
|------|-------------|
| [`app/login/page.tsx`](../monitor/app/login/page.tsx) | Login form (API URL + API Key). Validates via `/api/health` |
| [`app/(app)/layout.tsx`](../monitor/app/(app)/layout.tsx) | Authenticated layout: checks `isAuthenticated()`, renders sidebar + main |
| [`app/(app)/page.tsx`](../monitor/app/(app)/page.tsx) | Overview: agent count, ticket counts, agent grid, recent tickets table |
| [`app/(app)/tickets/page.tsx`](../monitor/app/(app)/tickets/page.tsx) | Ticket list with All/Open/Closed filters, up to 100 tickets |
| [`app/(app)/tickets/[id]/page.tsx`](../monitor/app/(app)/tickets/[id]/page.tsx) | Ticket detail: metadata, parent/sub-ticket links, message thread + logs side-by-side. Auto-refreshes every 3s for open tickets. Prompt-context dialog |
| [`app/(app)/logs/page.tsx`](../monitor/app/(app)/logs/page.tsx) | Live log stream: polls every 2s, level filter, pause/resume, keeps last 2000 entries |

## Components

| File | Description |
|------|-------------|
| [`components/sidebar.tsx`](../monitor/components/sidebar.tsx) | Left nav: Overview / Tickets / Logs + Disconnect button |
| [`components/ticket-table.tsx`](../monitor/components/ticket-table.tsx) | Table: ID, Title + goal, Status badge, Created By, Waiting On, Created timestamp |
| [`components/message-thread.tsx`](../monitor/components/message-thread.tsx) | Chat bubble layout: external messages right-aligned, agent messages left-aligned. Sender, timestamp, content |
| [`components/log-table.tsx`](../monitor/components/log-table.tsx) | Tabular log view with multi-select copy-to-clipboard and compact mode |
| [`components/context-dialog.tsx`](../monitor/components/context-dialog.tsx) | Modal showing full LLM prompt context for a specific response |
