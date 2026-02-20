# h1v3 Documentation

h1v3 is a ticket-based multi-agent runtime. It runs a "hive" of AI agents that communicate by creating and routing structured work units called **tickets**. Agents use a ReAct (Reason + Act) loop, calling tools iteratively until they produce a final response.

## Components

| Component | Description |
|-----------|-------------|
| **[core/](../core/)** | Go daemon (`h1v3d`) and CLI (`h1v3ctl`) |
| **[monitor/](../monitor/)** | Next.js dashboard for real-time inspection |

## Documentation

- **[Modules](modules.md)** -- every package/module and what it does
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
