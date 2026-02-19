package agent

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/h1v3-io/h1v3/pkg/protocol"
)

// BuildSystemPrompt assembles the system prompt from layered context.
// The ticket parameter is optional — pass nil for non-ticket interactions.
func (a *Agent) BuildSystemPrompt(ticket *protocol.Ticket) string {
	var b strings.Builder

	// 1. Agent identity
	fmt.Fprintf(&b, "# Agent: %s\n", a.Spec.ID)
	if a.Spec.Role != "" {
		fmt.Fprintf(&b, "Role: %s\n", a.Spec.Role)
	}
	b.WriteString("\n")
	b.WriteString(a.Spec.CoreInstructions)
	b.WriteString("\n\n")

	// 2. Current time
	now := time.Now()
	fmt.Fprintf(&b, "# Current Time\n%s\n\n", now.Format("2006-01-02 15:04:05 MST"))

	// 3. Scoped contexts (memory, config, etc.)
	if len(a.Spec.ScopedContexts) > 0 {
		b.WriteString("# Context\n")
		for scope, content := range a.Spec.ScopedContexts {
			fmt.Fprintf(&b, "## %s\n%s\n\n", scope, content)
		}
	}

	// 3b. Dynamic memory (from memory store)
	if a.Memory != nil {
		scopes := a.Memory.List()
		if len(scopes) > 0 {
			b.WriteString("# Memory\n")
			keys := make([]string, 0, len(scopes))
			for k := range scopes {
				keys = append(keys, k)
			}
			sort.Strings(keys)
			for _, scope := range keys {
				fmt.Fprintf(&b, "## %s\n%s\n\n", scope, scopes[scope])
			}
		}
	}

	// 4. Ticket context
	if ticket != nil {
		b.WriteString("# Current Ticket\n")
		fmt.Fprintf(&b, "ID: %s\n", ticket.ID)
		fmt.Fprintf(&b, "Title: %s\n", ticket.Title)
		if ticket.Goal != "" {
			fmt.Fprintf(&b, "Goal: %s\n", ticket.Goal)
		}
		fmt.Fprintf(&b, "Status: %s\n", ticket.Status)
		fmt.Fprintf(&b, "You are: %s\n", func() string {
			if ticket.CreatedBy == a.Spec.ID {
				return "creator"
			}
			return "responder"
		}())
		if len(ticket.Messages) > 0 {
			fmt.Fprintf(&b, "Messages: %d\n", len(ticket.Messages))
		}
		b.WriteString("\n")
	}

	// 5. Available tools
	toolNames := a.Tools.List()
	if len(toolNames) > 0 {
		b.WriteString("# Available Tools\n")
		defs := a.Tools.Definitions()
		for _, d := range defs {
			fmt.Fprintf(&b, "- **%s**: %s\n", d.Function.Name, d.Function.Description)
		}
		b.WriteString("\n")
	}

	// 6. Platform rules
	b.WriteString("# Rules\n")
	b.WriteString("- Only use tools when necessary to accomplish the task.\n")
	b.WriteString("- Stay focused on the current task or ticket.\n")
	b.WriteString("- Do not access resources outside your assigned directory.\n")
	b.WriteString("- Be concise in responses.\n")
	b.WriteString("- Use write_memory to persist important information you learn or decide (your name, user preferences, key facts). Memory survives across sessions — anything not written to memory will be forgotten.\n")
	b.WriteString("\n# Ticket Lifecycle\n")
	b.WriteString("- Always respond to tickets using respond_to_ticket — whether from a user or another agent.\n")
	b.WriteString("- To delegate work to another agent, use create_ticket with a clear title and a concrete goal (the specific condition that would satisfy the ticket).\n")
	b.WriteString("- Sub-tickets are linked automatically: when you create a ticket while working on another ticket, the new one becomes a child. When a child ticket is closed, its summary is automatically relayed back to the parent ticket — you do NOT need to manually forward results.\n")
	b.WriteString("- Only the ticket creator can close it.\n")
	b.WriteString("\n## As a RESPONDER (you are assigned to the ticket):\n")
	b.WriteString("- Answer the question or complete the task directly and concisely.\n")
	b.WriteString("- Do NOT ask follow-up questions unless the goal is genuinely unclear.\n")
	b.WriteString("- Do NOT make small talk or discuss the task beyond what was asked.\n")
	b.WriteString("- One response is usually enough. Provide the answer and stop.\n")
	b.WriteString("\n## As the CREATOR (you opened the ticket):\n")
	b.WriteString("- After receiving a response, evaluate whether the ticket's goal has been met.\n")
	b.WriteString("- If the goal is satisfied, close the ticket IMMEDIATELY with close_ticket. Do not thank, acknowledge, or continue the conversation.\n")
	b.WriteString("- If the goal is NOT satisfied, send ONE specific follow-up explaining what is still missing.\n")
	b.WriteString("- Never leave a ticket open once its goal is met.\n")

	return b.String()
}
