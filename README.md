# h1v3

A ticket-based runtime where AI agents collaborate as a team — delegating, specializing, and solving problems together.

## How it works

A **hive** is a group of AI agents running in a single process. Each agent has a role (front desk, coder, reviewer, etc.), its own memory, and a set of tools. Agents communicate by creating **tickets** — structured work units that get routed to the right agent automatically.

When a user sends a message (via Telegram, Slack, or the API), it goes to the **front agent**, which can either handle it directly or delegate to specialists by creating sub-tickets. Agents reason through problems using a ReAct loop: think, use a tool, observe the result, repeat.

```
User ──► Front Agent ──► Coder Agent
              │               │
              │          (writes code,
              │           runs tests)
              │               │
              ◄───────────────┘
              │          (responds with result)
              │
         ◄────┘
     (replies to user)
```

## Repository structure

```
core/       Go daemon — agents, tools, tickets, connectors
monitor/    Next.js dashboard — inspect agents, tickets, and logs in real time
```

See each directory's README for setup instructions and details:

- **[core/](core/README.md)** — building, configuring, and running the daemon
- **[monitor/](monitor/README.md)** — running the monitor dashboard

## Quick start

```bash
# Build the daemon
cd core && make build

# Run with a config file
bin/h1v3d --config config.json -v

# Or run with Docker (cold start, wipes data)
make cold-start default.json
```
