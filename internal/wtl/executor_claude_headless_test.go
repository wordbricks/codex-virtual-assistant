package wtl

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/siisee11/CodexVirtualAssistant/internal/assistant"
)

func TestClaudeHeadlessArgsSkipPermissions(t *testing.T) {
	t.Parallel()

	executor := NewClaudeHeadlessPhaseExecutor(ClaudeHeadlessPhaseExecutorConfig{
		BinaryPath: "claude",
		Model:      "claude-sonnet-4-5",
	}, time.Now)
	args, err := executor.args(CodexPhaseRequest{
		Role:   assistant.AttemptRoleGenerator,
		Prompt: "Return a result.",
	}, false)
	if err != nil {
		t.Fatalf("args() error = %v", err)
	}

	joined := strings.Join(args, "\n")
	for _, want := range []string{
		"--dangerously-skip-permissions",
		"--output-format\njson",
		"--model\nclaude-sonnet-4-5",
		"--json-schema",
	} {
		if !strings.Contains(joined, want) {
			t.Fatalf("args missing %q:\n%s", want, joined)
		}
	}
}

func TestClaudeHeadlessArgsUseWrapperFriendlyMode(t *testing.T) {
	t.Parallel()

	executor := NewClaudeHeadlessPhaseExecutor(ClaudeHeadlessPhaseExecutorConfig{
		BinaryPath:      "claude",
		UsePrintWrapper: true,
		Model:           "claude-sonnet-4-5",
	}, time.Now)
	args, err := executor.args(CodexPhaseRequest{
		Role:   assistant.AttemptRoleGenerator,
		Prompt: "Return a result.",
	}, true)
	if err != nil {
		t.Fatalf("args() error = %v", err)
	}

	joined := strings.Join(args, "\n")
	for _, want := range []string{
		"--dangerously-skip-permissions",
		"-p",
		"--model\nclaude-sonnet-4-5",
	} {
		if !strings.Contains(joined, want) {
			t.Fatalf("args missing %q:\n%s", want, joined)
		}
	}
	for _, forbidden := range []string{
		"--output-format",
		"--json-schema",
	} {
		if strings.Contains(joined, forbidden) {
			t.Fatalf("args unexpectedly contained %q:\n%s", forbidden, joined)
		}
	}
	if !strings.Contains(args[2], "Return exactly one JSON object") {
		t.Fatalf("wrapper prompt missing JSON contract:\n%s", args[2])
	}
}

func TestClaudeHeadlessRunPhaseParsesStructuredOutput(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	claudePath := filepath.Join(dir, "claude")
	script := `#!/usr/bin/env bash
set -euo pipefail
printf '%s\n' '{"result":"","structured_output":{"summary":"done","output":"Claude generated the result.","needs_user_input":false,"wait_kind":"","wait_title":"","wait_prompt":"","wait_risk_summary":""}}'
`
	if err := os.WriteFile(claudePath, []byte(script), 0o755); err != nil {
		t.Fatalf("write fake claude: %v", err)
	}

	executor := NewClaudeHeadlessPhaseExecutor(ClaudeHeadlessPhaseExecutorConfig{
		BinaryPath: claudePath,
		Cwd:        dir,
	}, func() time.Time { return time.Date(2026, 4, 16, 0, 0, 0, 0, time.UTC) })

	result, err := executor.RunPhase(context.Background(), CodexPhaseRequest{
		Role:       assistant.AttemptRoleGenerator,
		Prompt:     "Return a result.",
		WorkingDir: dir,
	})
	if err != nil {
		t.Fatalf("RunPhase() error = %v", err)
	}

	if result.Summary != "done" {
		t.Fatalf("Summary = %q, want done", result.Summary)
	}
	if result.Output != "Claude generated the result." {
		t.Fatalf("Output = %q", result.Output)
	}
	if len(result.Artifacts) != 1 {
		t.Fatalf("Artifacts len = %d, want 1", len(result.Artifacts))
	}
}

func TestClaudeFinalTextFallsBackToResult(t *testing.T) {
	t.Parallel()

	text, err := claudeFinalText([]byte(`{"result":"{\"summary\":\"ok\"}"}`))
	if err != nil {
		t.Fatalf("claudeFinalText() error = %v", err)
	}
	if text != `{"summary":"ok"}` {
		t.Fatalf("text = %q", text)
	}
}

func TestClaudeHeadlessRunPhaseWithWrapperParsesRawJSON(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	wrapperPath := filepath.Join(dir, "claude-pty-print")
	script := `#!/usr/bin/env bash
set -euo pipefail
printf '%s\n' '{"summary":"done","output":"Wrapper generated the result.","needs_user_input":false,"wait_kind":"","wait_title":"","wait_prompt":"","wait_risk_summary":""}'
`
	if err := os.WriteFile(wrapperPath, []byte(script), 0o755); err != nil {
		t.Fatalf("write fake wrapper: %v", err)
	}
	executor := NewClaudeHeadlessPhaseExecutor(ClaudeHeadlessPhaseExecutorConfig{
		BinaryPath:       "claude",
		UsePrintWrapper:  true,
		PrintWrapperPath: wrapperPath,
		Cwd:              dir,
	}, func() time.Time { return time.Date(2026, 4, 16, 0, 0, 0, 0, time.UTC) })

	result, err := executor.RunPhase(context.Background(), CodexPhaseRequest{
		Role:       assistant.AttemptRoleGenerator,
		Prompt:     "Return a result.",
		WorkingDir: dir,
	})
	if err != nil {
		t.Fatalf("RunPhase() error = %v", err)
	}

	if result.Summary != "done" {
		t.Fatalf("Summary = %q, want done", result.Summary)
	}
	if result.Output != "Wrapper generated the result." {
		t.Fatalf("Output = %q", result.Output)
	}
	if len(result.Artifacts) != 1 {
		t.Fatalf("Artifacts len = %d, want 1", len(result.Artifacts))
	}
}
