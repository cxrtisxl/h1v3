package tool

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"
)

const (
	defaultTimeout   = 60 * time.Second
	maxOutputSize    = 10 * 1024 // 10KB
)

// blockedPatterns are shell commands that should never be executed.
var blockedPatterns = []string{
	"rm -rf /",
	"rm -rf /*",
	"mkfs.",
	"dd if=",
	":(){ :|:& };:",
	"> /dev/sd",
	"chmod -R 777 /",
}

// ExecTool runs shell commands with safety guards.
type ExecTool struct {
	WorkDir string
	Timeout time.Duration
}

func (t *ExecTool) Name() string        { return "exec" }
func (t *ExecTool) Description() string  { return "Execute a shell command and return its output" }
func (t *ExecTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"command": map[string]any{"type": "string", "description": "Shell command to execute"},
		},
		"required": []string{"command"},
	}
}

func (t *ExecTool) Execute(ctx context.Context, params map[string]any) (string, error) {
	command := getString(params, "command")
	if command == "" {
		return "", fmt.Errorf("exec: command is required")
	}

	// Check blocked patterns
	lower := strings.ToLower(command)
	for _, pat := range blockedPatterns {
		if strings.Contains(lower, pat) {
			return "", fmt.Errorf("exec: blocked command pattern %q", pat)
		}
	}

	timeout := t.Timeout
	if timeout == 0 {
		timeout = defaultTimeout
	}

	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "/bin/sh", "-c", command)
	if t.WorkDir != "" {
		os.MkdirAll(t.WorkDir, 0o755)
		cmd.Dir = t.WorkDir
		cmd.Env = append(os.Environ(), "HOME="+t.WorkDir)
	}

	var buf bytes.Buffer
	cmd.Stdout = &buf
	cmd.Stderr = &buf

	err := cmd.Run()

	output := buf.String()
	if len(output) > maxOutputSize {
		output = output[:maxOutputSize] + "\n... [truncated]"
	}

	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return output, fmt.Errorf("exec: command timed out after %s", timeout)
		}
		// Return output + exit code for non-zero exits (not a hard error)
		if exitErr, ok := err.(*exec.ExitError); ok {
			return fmt.Sprintf("%s\n[exit code %d]", output, exitErr.ExitCode()), nil
		}
		return "", fmt.Errorf("exec: %w", err)
	}

	return output, nil
}
