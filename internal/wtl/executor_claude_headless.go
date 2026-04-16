package wtl

import (
	"bytes"
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/siisee11/CodexVirtualAssistant/internal/assistant"
)

//go:embed claude_pty_print.sh
var bundledClaudePrintWrapper string

type ClaudeHeadlessPhaseExecutorConfig struct {
	BinaryPath      string
	UsePrintWrapper bool
	PrintWrapperPath string
	Cwd             string
	Model           string
	ProjectsDir     string
	ArtifactDir     string
}

type ClaudeHeadlessPhaseExecutor struct {
	config ClaudeHeadlessPhaseExecutorConfig
	now    func() time.Time
}

func NewClaudeHeadlessPhaseExecutor(config ClaudeHeadlessPhaseExecutorConfig, now func() time.Time) *ClaudeHeadlessPhaseExecutor {
	if now == nil {
		now = time.Now
	}
	if strings.TrimSpace(config.BinaryPath) == "" {
		config.BinaryPath = "claude"
	}
	if strings.TrimSpace(config.Cwd) == "" {
		config.Cwd = "."
	}
	return &ClaudeHeadlessPhaseExecutor{
		config: config,
		now:    now,
	}
}

func (e *ClaudeHeadlessPhaseExecutor) RunPhase(ctx context.Context, request CodexPhaseRequest) (CodexPhaseResult, error) {
	if _, err := exec.LookPath(e.config.BinaryPath); err != nil {
		return CodexPhaseResult{}, fmt.Errorf("find claude binary %q: %w", e.config.BinaryPath, err)
	}

	cwd := strings.TrimSpace(firstNonEmpty(request.WorkingDir, e.config.Cwd))
	if cwd == "" {
		cwd = "."
	}

	args, err := e.args(request, e.shouldUseWrapper())
	if err != nil {
		return CodexPhaseResult{}, err
	}

	if request.LiveEmit != nil {
		request.LiveEmit(assistant.RunEvent{
			Type:    assistant.EventTypeReasoning,
			Phase:   phaseForAttemptRole(request.Role),
			Summary: "Claude headless phase started.",
		})
	}

	result, runErr := e.runPhaseCommand(ctx, request, cwd, args, e.shouldUseWrapper())
	if runErr != nil {
		return CodexPhaseResult{}, runErr
	}

	raw, err := claudeFinalText(result)
	if err != nil {
		return CodexPhaseResult{}, err
	}
	session := newAppServerTurnSession(AppServerPhaseExecutorConfig{
		ProjectsDir: e.config.ProjectsDir,
		ArtifactDir: e.config.ArtifactDir,
	}, e.now)
	session.runID = request.RunID
	session.attemptID = request.AttemptID
	session.attemptRole = request.Role
	session.project = request.Project
	session.finalText = raw
	return session.buildPhaseResult(request), nil
}

func (e *ClaudeHeadlessPhaseExecutor) runPhaseCommand(ctx context.Context, request CodexPhaseRequest, cwd string, args []string, useWrapper bool) ([]byte, error) {
	binaryPath, cleanupBinary, err := e.binaryPathForRun(useWrapper)
	if err != nil {
		return nil, err
	}
	defer cleanupBinary()

	cmd := exec.CommandContext(ctx, binaryPath, args...)
	cmd.Dir = cwd
	env, cleanupEnv := e.env(request, useWrapper)
	defer cleanupEnv()
	cmd.Env = env

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		message := strings.TrimSpace(stderr.String())
		if message == "" {
			message = strings.TrimSpace(stdout.String())
		}
		if message == "" {
			message = err.Error()
		}
		return nil, fmt.Errorf("run claude headless: %w: %s", err, message)
	}
	return stdout.Bytes(), nil
}

func (e *ClaudeHeadlessPhaseExecutor) binaryPathForRun(useWrapper bool) (string, func(), error) {
	if !useWrapper {
		return e.config.BinaryPath, func() {}, nil
	}
	if path := strings.TrimSpace(e.config.PrintWrapperPath); path != "" {
		return path, func() {}, nil
	}
	dir, err := os.MkdirTemp("", "cva-claude-print-wrapper-")
	if err != nil {
		return "", nil, fmt.Errorf("create bundled claude print wrapper dir: %w", err)
	}
	path := filepath.Join(dir, "claude-pty-print")
	if err := os.WriteFile(path, []byte(bundledClaudePrintWrapper), 0o755); err != nil {
		_ = os.RemoveAll(dir)
		return "", nil, fmt.Errorf("write bundled claude print wrapper: %w", err)
	}
	return path, func() { _ = os.RemoveAll(dir) }, nil
}

func (e *ClaudeHeadlessPhaseExecutor) args(request CodexPhaseRequest, useWrapper bool) ([]string, error) {
	prompt, err := e.prompt(request, useWrapper)
	if err != nil {
		return nil, err
	}
	args := []string{
		"--dangerously-skip-permissions",
		"-p", prompt,
	}
	if model := strings.TrimSpace(e.config.Model); model != "" {
		args = append(args, "--model", model)
	}
	if useWrapper {
		return args, nil
	}
	args = append(args, "--output-format", "json")
	if schema := phaseOutputSchema(request.Role); schema != nil {
		schemaJSON, err := json.Marshal(schema)
		if err != nil {
			return nil, fmt.Errorf("marshal claude output schema: %w", err)
		}
		args = append(args, "--json-schema", string(schemaJSON))
	}
	return args, nil
}

func (e *ClaudeHeadlessPhaseExecutor) prompt(request CodexPhaseRequest, useWrapper bool) (string, error) {
	prompt := phasePromptForCodex(request)
	if !useWrapper {
		return prompt, nil
	}
	schema := phaseOutputSchema(request.Role)
	if schema == nil {
		return prompt, nil
	}
	schemaJSON, err := json.MarshalIndent(schema, "", "  ")
	if err != nil {
		return "", fmt.Errorf("marshal claude wrapper output schema: %w", err)
	}
	return strings.TrimSpace(prompt + "\n\n" +
		"Final response contract for wrapper mode:\n" +
		"- Return exactly one JSON object that conforms to the schema below.\n" +
		"- Do not wrap the JSON in markdown fences.\n" +
		"- Do not prepend or append any prose, explanation, or status text.\n" +
		"- The entire final answer must be valid JSON and nothing else.\n\n" +
		string(schemaJSON)), nil
}

func (e *ClaudeHeadlessPhaseExecutor) shouldUseWrapper() bool {
	return e.config.UsePrintWrapper
}

func (e *ClaudeHeadlessPhaseExecutor) env(request CodexPhaseRequest, useWrapper bool) ([]string, func()) {
	session := newAppServerTurnSession(AppServerPhaseExecutorConfig{AgentBrowserHeaded: true}, e.now)
	session.runID = request.RunID
	session.attemptID = request.AttemptID
	session.project = request.Project
	env := session.appServerEnv()
	if useWrapper {
		env = upsertEnv(env, "CLAUDE_REAL_BIN", e.config.BinaryPath)
	}
	cleanup := func() {}
	if wrapperDir := strings.TrimSpace(session.agentBrowserWrapperDir); wrapperDir != "" {
		cleanup = func() {
			_ = os.RemoveAll(wrapperDir)
		}
	}
	return env, cleanup
}

func (e *ClaudeHeadlessPhaseExecutor) Close() error {
	return nil
}

func claudeFinalText(stdout []byte) (string, error) {
	raw := strings.TrimSpace(string(stdout))
	if raw == "" {
		return "", errors.New("claude returned empty output")
	}

	var payload struct {
		Result           string          `json:"result"`
		StructuredOutput json.RawMessage `json:"structured_output"`
	}
	if err := json.Unmarshal([]byte(raw), &payload); err == nil {
		if len(payload.StructuredOutput) > 0 && string(payload.StructuredOutput) != "null" {
			return strings.TrimSpace(string(payload.StructuredOutput)), nil
		}
		if strings.TrimSpace(payload.Result) != "" {
			return strings.TrimSpace(payload.Result), nil
		}
	}
	return raw, nil
}
