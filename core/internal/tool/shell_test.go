package tool

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestExec_BasicCommand(t *testing.T) {
	tool := &ExecTool{}
	result, err := tool.Execute(context.Background(), map[string]any{
		"command": "echo hello world",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if strings.TrimSpace(result) != "hello world" {
		t.Errorf("expected 'hello world', got %q", result)
	}
}

func TestExec_WorkDir(t *testing.T) {
	dir := t.TempDir()
	tool := &ExecTool{WorkDir: dir}
	result, err := tool.Execute(context.Background(), map[string]any{
		"command": "pwd",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if strings.TrimSpace(result) != dir {
		t.Errorf("expected %q, got %q", dir, strings.TrimSpace(result))
	}
}

func TestExec_NonZeroExit(t *testing.T) {
	tool := &ExecTool{}
	result, err := tool.Execute(context.Background(), map[string]any{
		"command": "exit 42",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(result, "[exit code 42]") {
		t.Errorf("expected exit code 42 in output, got %q", result)
	}
}

func TestExec_Stderr(t *testing.T) {
	tool := &ExecTool{}
	result, err := tool.Execute(context.Background(), map[string]any{
		"command": "echo err >&2",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if strings.TrimSpace(result) != "err" {
		t.Errorf("expected stderr captured, got %q", result)
	}
}

func TestExec_BlockedCommand(t *testing.T) {
	tool := &ExecTool{}
	_, err := tool.Execute(context.Background(), map[string]any{
		"command": "rm -rf /",
	})
	if err == nil {
		t.Fatal("expected error for blocked command")
	}
}

func TestExec_Timeout(t *testing.T) {
	tool := &ExecTool{Timeout: 100 * time.Millisecond}
	_, err := tool.Execute(context.Background(), map[string]any{
		"command": "sleep 10",
	})
	if err == nil {
		t.Fatal("expected timeout error")
	}
	if !strings.Contains(err.Error(), "timed out") {
		t.Errorf("expected timeout message, got %q", err.Error())
	}
}

func TestExec_OutputTruncation(t *testing.T) {
	tool := &ExecTool{}
	// Generate output larger than maxOutputSize (10KB)
	result, err := tool.Execute(context.Background(), map[string]any{
		"command": "yes 'aaaaaaaaaa' | head -n 2000",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.HasSuffix(result, "... [truncated]") {
		t.Error("expected truncation marker in large output")
	}
}

func TestExec_EmptyCommand(t *testing.T) {
	tool := &ExecTool{}
	_, err := tool.Execute(context.Background(), map[string]any{
		"command": "",
	})
	if err == nil {
		t.Fatal("expected error for empty command")
	}
}
