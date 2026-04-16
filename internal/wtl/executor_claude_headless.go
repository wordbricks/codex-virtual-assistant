package wtl

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/siisee11/CodexVirtualAssistant/internal/assistant"
)

type ClaudeHeadlessPhaseExecutorConfig struct {
	BinaryPath  string
	Cwd         string
	Model       string
	ProjectsDir string
	ArtifactDir string
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

	args, err := e.args(request)
	if err != nil {
		return CodexPhaseResult{}, err
	}

	cmd := exec.CommandContext(ctx, e.config.BinaryPath, args...)
	cmd.Dir = cwd
	env, cleanupEnv := e.env(request)
	defer cleanupEnv()
	cmd.Env = env

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if request.LiveEmit != nil {
		request.LiveEmit(assistant.RunEvent{
			Type:    assistant.EventTypeReasoning,
			Phase:   phaseForAttemptRole(request.Role),
			Summary: "Claude headless phase started.",
		})
	}

	if err := cmd.Run(); err != nil {
		message := strings.TrimSpace(stderr.String())
		if message == "" {
			message = strings.TrimSpace(stdout.String())
		}
		if message == "" {
			message = err.Error()
		}
		return CodexPhaseResult{}, fmt.Errorf("run claude headless: %w: %s", err, message)
	}

	raw, err := claudeFinalText(stdout.Bytes())
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

func (e *ClaudeHeadlessPhaseExecutor) args(request CodexPhaseRequest) ([]string, error) {
	prompt := phasePromptForCodex(request)
	args := []string{
		"--dangerously-skip-permissions",
		"-p", prompt,
		"--output-format", "json",
	}
	if model := strings.TrimSpace(e.config.Model); model != "" {
		args = append(args, "--model", model)
	}
	if schema := phaseOutputSchema(request.Role); schema != nil {
		schemaJSON, err := json.Marshal(schema)
		if err != nil {
			return nil, fmt.Errorf("marshal claude output schema: %w", err)
		}
		args = append(args, "--json-schema", string(schemaJSON))
	}
	return args, nil
}

func (e *ClaudeHeadlessPhaseExecutor) env(request CodexPhaseRequest) ([]string, func()) {
	session := newAppServerTurnSession(AppServerPhaseExecutorConfig{AgentBrowserHeaded: true}, e.now)
	session.runID = request.RunID
	session.attemptID = request.AttemptID
	session.project = request.Project
	env := session.appServerEnv()
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
