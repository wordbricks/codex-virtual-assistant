package wtl

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/siisee11/CodexVirtualAssistant/internal/assistant"
)

var urlPattern = regexp.MustCompile(`https?://[^\s"'<>]+`)

var lookupAgentBrowserExecutablePath = detectAgentBrowserExecutablePath
var lookupAgentBrowserCLIPath = detectAgentBrowserCLIPath

type AppServerPhaseExecutorConfig struct {
	BinaryPath         string
	Cwd                string
	ArtifactDir        string
	Model              string
	ApprovalPolicy     string
	SandboxMode        string
	NetworkAccess      bool
	AgentBrowserHeaded bool
}

type AppServerPhaseExecutor struct {
	config AppServerPhaseExecutorConfig
	now    func() time.Time
}

func NewAppServerPhaseExecutor(config AppServerPhaseExecutorConfig, now func() time.Time) *AppServerPhaseExecutor {
	if now == nil {
		now = time.Now
	}
	if strings.TrimSpace(config.BinaryPath) == "" {
		config.BinaryPath = "codex"
	}
	if strings.TrimSpace(config.ApprovalPolicy) == "" {
		config.ApprovalPolicy = "never"
	}
	if strings.TrimSpace(config.SandboxMode) == "" {
		config.SandboxMode = "workspace-write"
	}
	if strings.TrimSpace(config.Model) == "" {
		config.Model = "gpt-5.4"
	}
	if strings.TrimSpace(config.Cwd) == "" {
		config.Cwd = "."
	}
	if !config.AgentBrowserHeaded {
		config.AgentBrowserHeaded = true
	}
	return &AppServerPhaseExecutor{
		config: config,
		now:    now,
	}
}

func (e *AppServerPhaseExecutor) RunPhase(ctx context.Context, request CodexPhaseRequest) (CodexPhaseResult, error) {
	session := newAppServerTurnSession(e.config, e.now)
	defer session.Close()
	return session.run(ctx, request)
}

func (e *AppServerPhaseExecutor) Close() error {
	return nil
}

type appServerTurnSession struct {
	config AppServerPhaseExecutorConfig
	now    func() time.Time
	cwd    string

	cmd   *exec.Cmd
	stdin io.WriteCloser

	nextID atomic.Int64

	writeMu sync.Mutex

	mu        sync.Mutex
	pending   map[string]chan appServerRPCResponse
	readErr   error
	closed    chan struct{}
	closeOnce sync.Once

	threadID    string
	turnDone    chan struct{}
	runID       string
	attemptID   string
	attemptRole assistant.AttemptRole
	project     assistant.ProjectContext
	liveEmit    func(assistant.RunEvent)

	stderr bytes.Buffer

	textBuilder       strings.Builder
	finalText         string
	turnStatus        string
	turnErrMsg        string
	itemOutputBuffers map[string]*strings.Builder
	itemStartedAt     map[string]time.Time
	reasoningBuffers  map[string]*strings.Builder

	toolRuns     []CodexToolRun
	browserSteps []AgentBrowserStep
	artifacts    []assistant.Artifact
	observations []string
	waitRequest  *assistant.WaitRequest

	runArtifactDir    string
	runArtifactRelDir string
	browserFramePaths []string
	browserFrameRel   []string

	agentBrowserWrapperDir string
}

type appServerRPCEnvelope struct {
	ID     json.RawMessage    `json:"id"`
	Method string             `json:"method"`
	Params json.RawMessage    `json:"params"`
	Result json.RawMessage    `json:"result"`
	Error  *appServerRPCError `json:"error"`
}

type appServerRPCError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type appServerRPCResponse struct {
	Result json.RawMessage
	Error  *appServerRPCError
}

func newAppServerTurnSession(config AppServerPhaseExecutorConfig, now func() time.Time) *appServerTurnSession {
	session := &appServerTurnSession{
		config:            config,
		now:               now,
		pending:           make(map[string]chan appServerRPCResponse),
		closed:            make(chan struct{}),
		turnDone:          make(chan struct{}),
		itemOutputBuffers: make(map[string]*strings.Builder),
		itemStartedAt:     make(map[string]time.Time),
		reasoningBuffers:  make(map[string]*strings.Builder),
	}
	session.nextID.Store(1)
	return session
}

func (s *appServerTurnSession) start(ctx context.Context, cwd string) error {
	if _, err := exec.LookPath(s.config.BinaryPath); err != nil {
		return fmt.Errorf("find codex binary %q: %w", s.config.BinaryPath, err)
	}
	s.cwd = strings.TrimSpace(firstNonEmpty(cwd, s.config.Cwd))
	if s.cwd == "" {
		s.cwd = "."
	}

	cmd := exec.CommandContext(ctx, s.config.BinaryPath, "app-server")
	cmd.Dir = s.cwd
	cmd.Env = s.appServerEnv()

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return err
	}

	if err := cmd.Start(); err != nil {
		return err
	}

	s.cmd = cmd
	s.stdin = stdin

	go s.readLoop(stdout)
	go s.captureStderr(stderr)
	go func() {
		s.shutdown(cmd.Wait())
	}()

	if _, err := s.sendRequest(ctx, "initialize", map[string]any{
		"clientInfo": map[string]any{
			"name":    "codex_virtual_assistant",
			"title":   "Codex Virtual Assistant",
			"version": "0.1.1",
		},
	}); err != nil {
		return err
	}
	if err := s.sendNotification("initialized", map[string]any{}); err != nil {
		return err
	}

	raw, err := s.sendRequest(ctx, "thread/start", map[string]any{
		"model":          s.config.Model,
		"cwd":            s.cwd,
		"approvalPolicy": s.config.ApprovalPolicy,
		"sandbox":        s.threadSandboxMode(),
		"personality":    "pragmatic",
		"serviceName":    "codex_virtual_assistant",
		"ephemeral":      true,
	})
	if err != nil {
		return err
	}

	var payload struct {
		Thread struct {
			ID string `json:"id"`
		} `json:"thread"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return fmt.Errorf("decode thread/start response: %w", err)
	}
	if strings.TrimSpace(payload.Thread.ID) == "" {
		return errors.New("codex app server returned an empty thread id")
	}
	s.threadID = payload.Thread.ID
	return nil
}

func (s *appServerTurnSession) run(ctx context.Context, request CodexPhaseRequest) (CodexPhaseResult, error) {
	s.runID = request.RunID
	s.attemptID = request.AttemptID
	s.attemptRole = request.Role
	s.project = request.Project
	s.liveEmit = request.LiveEmit

	if err := s.start(ctx, request.WorkingDir); err != nil {
		return CodexPhaseResult{}, err
	}
	s.prepareArtifactCapture(request)

	params := map[string]any{
		"threadId":       s.threadID,
		"input":          []map[string]any{{"type": "text", "text": phasePromptForCodex(request)}},
		"cwd":            s.cwd,
		"approvalPolicy": s.config.ApprovalPolicy,
		"sandboxPolicy":  s.turnSandboxPolicy(),
		"model":          s.config.Model,
		"personality":    "pragmatic",
		"summary":        "concise",
	}
	if schema := phaseOutputSchema(request.Role); schema != nil {
		params["outputSchema"] = schema
	}

	if _, err := s.sendRequest(ctx, "turn/start", params); err != nil {
		return CodexPhaseResult{}, err
	}

	select {
	case <-ctx.Done():
		return CodexPhaseResult{}, ctx.Err()
	case <-s.closed:
		return CodexPhaseResult{}, s.closedError("codex app server closed during phase execution")
	case <-s.turnDone:
	}

	response := s.buildPhaseResult(request)
	if response.WaitRequest != nil {
		return response, nil
	}
	if s.turnStatus != "" && s.turnStatus != "completed" {
		message := firstNonEmpty(s.turnErrMsg, fmt.Sprintf("turn finished with status %s", s.turnStatus))
		return CodexPhaseResult{}, errors.New(message)
	}
	return response, nil
}

func (s *appServerTurnSession) appServerEnv() []string {
	env := append([]string{}, os.Environ()...)
	if s.config.AgentBrowserHeaded {
		env = upsertEnv(env, "AGENT_BROWSER_HEADED", "true")
	}
	if sessionName := strings.TrimSpace(firstNonEmpty(s.attemptID, s.runID)); sessionName != "" {
		env = upsertEnv(env, "AGENT_BROWSER_SESSION", sessionName)
	}
	if profileDir := strings.TrimSpace(s.project.BrowserProfileDir); profileDir != "" {
		env = upsertEnv(env, "AGENT_BROWSER_PROFILE", profileDir)
		if executablePath := strings.TrimSpace(lookupAgentBrowserExecutablePath()); executablePath != "" {
			env = upsertEnv(env, "AGENT_BROWSER_EXECUTABLE_PATH", executablePath)
		}
		if wrapperDir := strings.TrimSpace(s.ensureAgentBrowserWrapperDir()); wrapperDir != "" {
			env = upsertEnv(env, "PATH", wrapperDir+string(os.PathListSeparator)+os.Getenv("PATH"))
		}
	}
	return env
}

func detectAgentBrowserCLIPath() string {
	path, err := exec.LookPath("agent-browser")
	if err != nil {
		return ""
	}
	return path
}

func (s *appServerTurnSession) ensureAgentBrowserWrapperDir() string {
	if strings.TrimSpace(s.agentBrowserWrapperDir) != "" {
		return s.agentBrowserWrapperDir
	}
	realBinary := strings.TrimSpace(lookupAgentBrowserCLIPath())
	if realBinary == "" {
		return ""
	}
	port := s.project.BrowserCDPPort
	if port <= 0 {
		port = 9223
	}
	dir, err := os.MkdirTemp("", "cva-agent-browser-wrapper-")
	if err != nil {
		return ""
	}
	scriptPath := filepath.Join(dir, "agent-browser")
	script := fmt.Sprintf(`#!/usr/bin/env bash
set -euo pipefail

if [[ "${1:-}" == "connect" ]]; then
  echo "agent-browser connect is disabled for project CDP reuse. Use agent-browser --cdp %d <command> instead." >&2
  exit 64
fi

exec %q "$@"
`, port, realBinary)
	if err := os.WriteFile(scriptPath, []byte(script), 0o755); err != nil {
		_ = os.RemoveAll(dir)
		return ""
	}
	s.agentBrowserWrapperDir = dir
	return dir
}

func detectAgentBrowserExecutablePath() string {
	candidates := []string{}
	switch runtime.GOOS {
	case "darwin":
		candidates = append(candidates,
			"/Applications/Google Chrome.app/Contents/MacOS/Google Chrome",
			"/Applications/Chromium.app/Contents/MacOS/Chromium",
		)
	case "linux":
		candidates = append(candidates,
			"/usr/bin/google-chrome",
			"/usr/bin/google-chrome-stable",
			"/usr/bin/chromium",
			"/usr/bin/chromium-browser",
		)
	case "windows":
		candidates = append(candidates,
			`C:\Program Files\Google\Chrome\Application\chrome.exe`,
			`C:\Program Files (x86)\Google\Chrome\Application\chrome.exe`,
		)
	}

	for _, candidate := range candidates {
		if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
			return candidate
		}
	}

	commands := []string{"google-chrome", "google-chrome-stable", "chromium", "chromium-browser", "chrome"}
	for _, command := range commands {
		if path, err := exec.LookPath(command); err == nil {
			return path
		}
	}
	return ""
}

func upsertEnv(env []string, key string, value string) []string {
	prefix := key + "="
	for idx, entry := range env {
		if strings.HasPrefix(entry, prefix) {
			env[idx] = prefix + value
			return env
		}
	}
	return append(env, prefix+value)
}

func (s *appServerTurnSession) buildPhaseResult(request CodexPhaseRequest) CodexPhaseResult {
	response := CodexPhaseResult{
		ToolRuns:     append([]CodexToolRun{}, s.toolRuns...),
		BrowserSteps: append([]AgentBrowserStep{}, s.browserSteps...),
		Artifacts:    append([]assistant.Artifact{}, s.artifacts...),
		Observations: append([]string{}, s.observations...),
		WaitRequest:  s.waitRequest,
	}
	if recording := s.finalizeBrowserRecording(); recording != nil {
		response.Artifacts = append(response.Artifacts, *recording)
	}

	raw := strings.TrimSpace(firstNonEmpty(s.finalText, s.textBuilder.String()))
	if raw == "" {
		if response.WaitRequest == nil {
			response.WaitRequest = s.remoteDebugApprovalWaitRequest()
		}
		if response.WaitRequest != nil {
			response.Summary = firstNonEmpty(response.WaitRequest.Title, "Input required")
			return response
		}
		response.Summary = "Codex completed the phase without returning final text."
		return response
	}

	type waitEnvelope struct {
		WaitRequest *waitRequestPayload `json:"wait_request"`
	}
	var waitOnly waitEnvelope
	if err := json.Unmarshal([]byte(raw), &waitOnly); err == nil && waitOnly.WaitRequest != nil && response.WaitRequest == nil {
		response.WaitRequest = waitOnly.WaitRequest.toAssistantWaitRequest()
	}

	switch request.Role {
	case assistant.AttemptRoleGenerator, assistant.AttemptRoleAnswer, assistant.AttemptRoleReporter:
		var payload struct {
			Summary         string `json:"summary"`
			Output          string `json:"output"`
			DeliveryStatus  string `json:"delivery_status"`
			MessagePreview  string `json:"message_preview"`
			ReportPayload   string `json:"report_payload"`
			NeedsUserInput  bool   `json:"needs_user_input"`
			WaitKind        string `json:"wait_kind"`
			WaitTitle       string `json:"wait_title"`
			WaitPrompt      string `json:"wait_prompt"`
			WaitRiskSummary string `json:"wait_risk_summary"`
		}
		if err := json.Unmarshal([]byte(raw), &payload); err == nil {
			response.Summary = strings.TrimSpace(firstNonEmpty(payload.Summary, summarizeOutput(payload.Output), payload.MessagePreview))
			switch request.Role {
			case assistant.AttemptRoleReporter:
				// Preserve the full structured reporter JSON so the engine can decode
				// delivery_status/message_preview/report_payload without losing fields.
				response.Output = raw
			default:
				response.Output = strings.TrimSpace(firstNonEmpty(payload.Output, payload.ReportPayload))
			}
			if response.WaitRequest == nil && payload.NeedsUserInput {
				response.WaitRequest = (&waitRequestPayload{
					Kind:        payload.WaitKind,
					Title:       payload.WaitTitle,
					Prompt:      payload.WaitPrompt,
					RiskSummary: payload.WaitRiskSummary,
				}).toAssistantWaitRequest()
			}
		} else {
			response.Summary = summarizeOutput(raw)
			response.Output = raw
		}
		if response.WaitRequest == nil && strings.TrimSpace(response.Output) != "" {
			title := "Assistant draft result"
			mimeType := "text/markdown"
			artifactContent := response.Output
			if request.Role == assistant.AttemptRoleAnswer {
				title = "Assistant answer"
			}
			if request.Role == assistant.AttemptRoleReporter {
				title = "Delivered report payload"
				mimeType = "application/json"
				if err := json.Unmarshal([]byte(raw), &payload); err == nil && strings.TrimSpace(payload.ReportPayload) != "" {
					artifactContent = strings.TrimSpace(payload.ReportPayload)
				}
			}
			response.Artifacts = append(response.Artifacts, assistant.Artifact{
				Kind:     assistant.ArtifactKindReport,
				Title:    title,
				MIMEType: mimeType,
				Content:  artifactContent,
			})
		}
	default:
		response.Summary = summarizeOutput(raw)
		response.Output = raw
	}

	if response.WaitRequest != nil {
		response.Output = ""
		response.Summary = firstNonEmpty(response.WaitRequest.Title, response.Summary)
	} else if wait := s.remoteDebugApprovalWaitRequest(); wait != nil {
		response.WaitRequest = wait
		response.Output = ""
		response.Summary = firstNonEmpty(wait.Title, response.Summary)
	}
	return response
}

func (s *appServerTurnSession) Close() error {
	s.shutdown(nil)
	if strings.TrimSpace(s.agentBrowserWrapperDir) != "" {
		_ = os.RemoveAll(s.agentBrowserWrapperDir)
	}
	if s.stdin != nil {
		_ = s.stdin.Close()
	}
	if s.cmd != nil && s.cmd.Process != nil {
		_ = s.cmd.Process.Kill()
	}
	return nil
}

func (s *appServerTurnSession) readLoop(stdout io.Reader) {
	scanner := bufio.NewScanner(stdout)
	scanner.Buffer(make([]byte, 0, 1024*1024), 10*1024*1024)
	for scanner.Scan() {
		var message appServerRPCEnvelope
		if err := json.Unmarshal(scanner.Bytes(), &message); err != nil {
			continue
		}
		switch {
		case message.Method != "" && len(message.ID) > 0:
			s.handleServerRequest(message)
		case message.Method != "":
			s.handleNotification(message)
		case len(message.ID) > 0:
			s.handleResponse(message)
		}
	}

	if err := scanner.Err(); err != nil {
		s.shutdown(err)
		return
	}
	s.shutdown(io.EOF)
}

func (s *appServerTurnSession) captureStderr(stderr io.Reader) {
	_, _ = io.Copy(&s.stderr, stderr)
}

func (s *appServerTurnSession) handleResponse(message appServerRPCEnvelope) {
	key := rpcIDKey(message.ID)
	s.mu.Lock()
	ch := s.pending[key]
	delete(s.pending, key)
	s.mu.Unlock()
	if ch == nil {
		return
	}
	ch <- appServerRPCResponse{Result: message.Result, Error: message.Error}
}

func (s *appServerTurnSession) handleNotification(message appServerRPCEnvelope) {
	switch message.Method {
	case "item/agentMessage/delta":
		var params struct {
			Delta string `json:"delta"`
		}
		if err := json.Unmarshal(message.Params, &params); err != nil {
			return
		}
		s.textBuilder.WriteString(params.Delta)
	case "item/commandExecution/outputDelta", "item/fileChange/outputDelta":
		var params struct {
			ItemID string `json:"itemId"`
			Delta  string `json:"delta"`
		}
		if err := json.Unmarshal(message.Params, &params); err != nil {
			return
		}
		if strings.TrimSpace(params.ItemID) == "" || params.Delta == "" {
			return
		}
		builder := s.outputBuffer(params.ItemID)
		builder.WriteString(params.Delta)
	case "item/reasoning/summaryTextDelta":
		var params struct {
			ItemID string `json:"itemId"`
			Delta  string `json:"delta"`
		}
		if err := json.Unmarshal(message.Params, &params); err != nil {
			return
		}
		s.appendReasoningDelta(params.ItemID, params.Delta)
	case "item/started":
		var params struct {
			Item map[string]any `json:"item"`
		}
		if err := json.Unmarshal(message.Params, &params); err != nil {
			return
		}
		itemID := stringValue(params.Item["id"])
		if itemID != "" {
			s.itemStartedAt[itemID] = s.now().UTC()
		}
		s.handleStartedItem(params.Item)
	case "item/completed":
		var params struct {
			Item map[string]any `json:"item"`
		}
		if err := json.Unmarshal(message.Params, &params); err != nil {
			return
		}
		s.handleCompletedItem(params.Item)
	case "turn/completed":
		var params struct {
			Turn struct {
				Status string `json:"status"`
				Error  *struct {
					Message string `json:"message"`
				} `json:"error"`
			} `json:"turn"`
		}
		if err := json.Unmarshal(message.Params, &params); err != nil {
			return
		}
		s.turnStatus = params.Turn.Status
		if params.Turn.Error != nil {
			s.turnErrMsg = params.Turn.Error.Message
		}
		select {
		case <-s.turnDone:
		default:
			close(s.turnDone)
		}
	}
}

func (s *appServerTurnSession) handleCompletedItem(item map[string]any) {
	switch stringValue(item["type"]) {
	case "agentMessage":
		s.finalText = strings.TrimSpace(stringValue(item["text"]))
	case "reasoning":
		s.collectReasoning(item)
	case "commandExecution":
		s.collectCommandExecution(item)
	case "mcpToolCall":
		s.collectMCPToolCall(item)
	case "dynamicToolCall":
		s.collectDynamicToolCall(item)
	case "webSearch":
		s.collectWebSearch(item)
	case "imageView":
		s.collectImageView(item)
	case "fileChange":
		s.collectFileChange(item)
	}
}

func (s *appServerTurnSession) handleStartedItem(item map[string]any) {
	switch stringValue(item["type"]) {
	case "commandExecution", "mcpToolCall", "dynamicToolCall", "webSearch":
		s.emitToolStarted(item)
	}
}

func (s *appServerTurnSession) handleServerRequest(message appServerRPCEnvelope) {
	switch message.Method {
	case "item/commandExecution/requestApproval":
		var params struct {
			Reason                 string         `json:"reason"`
			Command                any            `json:"command"`
			Cwd                    string         `json:"cwd"`
			NetworkApprovalContext map[string]any `json:"networkApprovalContext"`
		}
		if err := json.Unmarshal(message.Params, &params); err != nil {
			_ = s.sendResult(message.ID, map[string]any{"decision": "decline"})
			return
		}
		commandText := commandSummary(params.Command)
		riskSummary := strings.TrimSpace(commandText)
		if len(params.NetworkApprovalContext) > 0 {
			riskSummary = strings.TrimSpace(firstNonEmpty(riskSummary, stringifyJSON(params.NetworkApprovalContext)))
		}
		s.setWaitRequest(&assistant.WaitRequest{
			Kind:        assistant.WaitKindApproval,
			Title:       "Approval required",
			Prompt:      firstNonEmpty(strings.TrimSpace(params.Reason), "Codex needs approval before continuing this action."),
			RiskSummary: firstNonEmpty(riskSummary, params.Cwd),
		})
		_ = s.sendResult(message.ID, map[string]any{"decision": "decline"})
	case "item/fileChange/requestApproval":
		var params struct {
			Reason    string `json:"reason"`
			GrantRoot string `json:"grantRoot"`
		}
		if err := json.Unmarshal(message.Params, &params); err != nil {
			_ = s.sendResult(message.ID, map[string]any{"decision": "decline"})
			return
		}
		s.setWaitRequest(&assistant.WaitRequest{
			Kind:        assistant.WaitKindApproval,
			Title:       "File change approval required",
			Prompt:      firstNonEmpty(strings.TrimSpace(params.Reason), "Codex needs approval before writing files."),
			RiskSummary: strings.TrimSpace(params.GrantRoot),
		})
		_ = s.sendResult(message.ID, map[string]any{"decision": "decline"})
	case "item/tool/requestUserInput":
		var params struct {
			Questions []struct {
				Header   string `json:"header"`
				ID       string `json:"id"`
				Question string `json:"question"`
			} `json:"questions"`
		}
		if err := json.Unmarshal(message.Params, &params); err != nil {
			_ = s.sendResult(message.ID, map[string]any{"answers": map[string]any{}})
			return
		}
		s.setWaitRequest(waitRequestFromQuestions(params.Questions))
		_ = s.sendResult(message.ID, map[string]any{"answers": map[string]any{}})
	default:
		_ = s.sendError(message.ID, -32601, "unsupported server request")
	}
}

func (s *appServerTurnSession) collectCommandExecution(item map[string]any) {
	itemID := stringValue(item["id"])
	command := commandSummary(item["command"])
	output := strings.TrimSpace(firstNonEmpty(stringValue(item["aggregatedOutput"]), s.outputBufferString(itemID)))
	finishedAt := s.now().UTC()
	tool := CodexToolRun{
		ID:            itemID,
		Name:          firstNonEmpty(commandName(command), "command"),
		InputSummary:  command,
		OutputSummary: summarizeOutput(output),
		StartedAt:     normalizeTime(s.itemStartedAt[itemID], s.now().UTC()),
		FinishedAt:    finishedAt,
	}
	s.toolRuns = append(s.toolRuns, tool)
	s.emitToolCompleted(item, tool.Name, command, output, tool.StartedAt, finishedAt)

	if isBrowserCommand(command) {
		step := AgentBrowserStep{
			Title:   firstNonEmpty(browserTitle(command), "Browser step"),
			URL:     extractFirstURL(command + "\n" + output),
			Summary: firstNonEmpty(strings.TrimSpace(output), "Codex executed a browser-oriented command."),
			Action: AgentBrowserAction{
				Name:   browserAction(command),
				Target: browserTarget(command),
			},
			ObservedText: observedText(output),
			OccurredAt:   finishedAt,
		}
		if screenshotPath, screenshotRel := s.captureBrowserFrame(command, len(s.browserSteps)+1); screenshotPath != "" {
			step.ScreenshotPath = screenshotRel
			step.ScreenshotNote = step.Title
			s.browserFramePaths = append(s.browserFramePaths, screenshotPath)
			s.browserFrameRel = append(s.browserFrameRel, screenshotRel)
		}
		s.browserSteps = append(s.browserSteps, step)
	}
}

func (s *appServerTurnSession) collectMCPToolCall(item map[string]any) {
	itemID := stringValue(item["id"])
	startedAt := normalizeTime(s.itemStartedAt[stringValue(item["id"])], s.now().UTC())
	finishedAt := s.now().UTC()
	result := stringifyJSON(item["result"])
	if result == "" {
		result = stringifyJSON(item["error"])
	}
	name := strings.TrimSpace(firstNonEmpty(stringValue(item["server"])+"/"+stringValue(item["tool"]), stringValue(item["tool"])))
	input := stringifyJSON(item["arguments"])
	s.toolRuns = append(s.toolRuns, CodexToolRun{
		ID:            itemID,
		Name:          name,
		InputSummary:  input,
		OutputSummary: summarizeOutput(result),
		StartedAt:     startedAt,
		FinishedAt:    finishedAt,
	})
	s.emitToolCompleted(item, name, input, result, startedAt, finishedAt)
}

func (s *appServerTurnSession) collectDynamicToolCall(item map[string]any) {
	itemID := stringValue(item["id"])
	startedAt := normalizeTime(s.itemStartedAt[stringValue(item["id"])], s.now().UTC())
	finishedAt := s.now().UTC()
	name := stringValue(item["tool"])
	input := stringifyJSON(item["arguments"])
	result := firstNonEmpty(stringifyJSON(item["contentItems"]), stringifyJSON(item["success"]))
	s.toolRuns = append(s.toolRuns, CodexToolRun{
		ID:            itemID,
		Name:          name,
		InputSummary:  input,
		OutputSummary: summarizeOutput(result),
		StartedAt:     startedAt,
		FinishedAt:    finishedAt,
	})
	s.emitToolCompleted(item, name, input, result, startedAt, finishedAt)
}

func (s *appServerTurnSession) collectWebSearch(item map[string]any) {
	itemID := stringValue(item["id"])
	now := s.now().UTC()
	query := strings.TrimSpace(firstNonEmpty(stringValue(item["query"]), stringifyJSON(item["action"])))
	s.toolRuns = append(s.toolRuns, CodexToolRun{
		ID:            itemID,
		Name:          "webSearch",
		InputSummary:  query,
		OutputSummary: summarizeOutput(query),
		StartedAt:     now,
		FinishedAt:    now,
	})
	s.emitToolCompleted(item, "webSearch", query, query, now, now)
}

func (s *appServerTurnSession) collectImageView(item map[string]any) {
	path := strings.TrimSpace(stringValue(item["path"]))
	if path == "" {
		return
	}
	s.artifacts = append(s.artifacts, assistant.Artifact{
		Kind:     assistant.ArtifactKindScreenshot,
		Title:    "Viewed image",
		MIMEType: "image/png",
		Path:     path,
	})
}

func (s *appServerTurnSession) collectFileChange(item map[string]any) {
	changes, ok := item["changes"].([]any)
	if !ok || len(changes) == 0 {
		return
	}
	paths := make([]string, 0, len(changes))
	for _, change := range changes {
		changeMap, ok := change.(map[string]any)
		if !ok {
			continue
		}
		path := strings.TrimSpace(stringValue(changeMap["path"]))
		if path != "" {
			paths = append(paths, path)
		}
	}
	if len(paths) == 0 {
		return
	}
	s.observations = append(s.observations, fmt.Sprintf("Codex updated files: %s", strings.Join(paths, ", ")))
}

func (s *appServerTurnSession) outputBuffer(itemID string) *strings.Builder {
	if s.itemOutputBuffers[itemID] == nil {
		s.itemOutputBuffers[itemID] = &strings.Builder{}
	}
	return s.itemOutputBuffers[itemID]
}

func (s *appServerTurnSession) reasoningBuffer(itemID string) *strings.Builder {
	if s.reasoningBuffers[itemID] == nil {
		s.reasoningBuffers[itemID] = &strings.Builder{}
	}
	return s.reasoningBuffers[itemID]
}

func (s *appServerTurnSession) reasoningBufferString(itemID string) string {
	if s.reasoningBuffers[itemID] == nil {
		return ""
	}
	return s.reasoningBuffers[itemID].String()
}

func (s *appServerTurnSession) appendReasoningDelta(itemID, delta string) {
	if strings.TrimSpace(itemID) == "" || delta == "" {
		return
	}
	builder := s.reasoningBuffer(itemID)
	builder.WriteString(delta)
	s.emitLiveEvent(assistant.EventTypeReasoning, summarizeOutput(builder.String()), map[string]any{
		"item_id":      itemID,
		"attempt_id":   s.attemptID,
		"attempt_role": string(s.attemptRole),
		"text":         strings.TrimSpace(builder.String()),
	})
}

func (s *appServerTurnSession) collectReasoning(item map[string]any) {
	itemID := stringValue(item["id"])
	text := strings.TrimSpace(firstNonEmpty(
		reasoningSummary(item["summary"]),
		s.reasoningBufferString(itemID),
		reasoningContent(item["content"]),
	))
	if !isMeaningfulReasoningText(text) {
		return
	}
	s.emitLiveEvent(assistant.EventTypeReasoning, summarizeOutput(text), map[string]any{
		"item_id":      itemID,
		"attempt_id":   s.attemptID,
		"attempt_role": string(s.attemptRole),
		"text":         text,
	})
}

func (s *appServerTurnSession) emitToolStarted(item map[string]any) {
	itemID := strings.TrimSpace(stringValue(item["id"]))
	if itemID == "" {
		return
	}
	name, input := liveToolMetadata(item)
	s.emitLiveEvent(assistant.EventTypeToolCallStart, firstNonEmpty(name, "Tool started"), map[string]any{
		"item_id":       itemID,
		"tool_call_id":  itemID,
		"attempt_id":    s.attemptID,
		"attempt_role":  string(s.attemptRole),
		"tool_name":     name,
		"input_summary": input,
		"status":        stringValue(item["status"]),
	})
}

func (s *appServerTurnSession) emitToolCompleted(item map[string]any, name, input, output string, startedAt, finishedAt time.Time) {
	itemID := strings.TrimSpace(stringValue(item["id"]))
	if itemID == "" {
		return
	}
	s.emitLiveEvent(assistant.EventTypeToolCallEnd, summarizeOutput(firstNonEmpty(output, name)), map[string]any{
		"item_id":        itemID,
		"tool_call_id":   itemID,
		"attempt_id":     s.attemptID,
		"attempt_role":   string(s.attemptRole),
		"tool_name":      strings.TrimSpace(name),
		"input_summary":  strings.TrimSpace(input),
		"output_summary": strings.TrimSpace(output),
		"status":         stringValue(item["status"]),
		"started_at":     normalizeTime(startedAt, s.now()),
		"finished_at":    normalizeTime(finishedAt, s.now()),
	})
}

func (s *appServerTurnSession) emitLiveEvent(eventType assistant.EventType, summary string, data map[string]any) {
	if s.liveEmit == nil || strings.TrimSpace(s.runID) == "" {
		return
	}
	s.liveEmit(assistant.RunEvent{
		ID:        assistant.NewID("event", s.now().UTC()),
		RunID:     s.runID,
		Type:      eventType,
		Phase:     phaseForAttemptRole(s.attemptRole),
		Summary:   firstNonEmpty(strings.TrimSpace(summary), string(eventType)),
		Data:      data,
		CreatedAt: s.now().UTC(),
	})
}

func liveToolMetadata(item map[string]any) (name string, input string) {
	switch stringValue(item["type"]) {
	case "commandExecution":
		input = commandSummary(item["command"])
		name = firstNonEmpty(commandName(input), "command")
	case "mcpToolCall":
		name = strings.TrimSpace(firstNonEmpty(stringValue(item["server"])+"/"+stringValue(item["tool"]), stringValue(item["tool"])))
		input = stringifyJSON(item["arguments"])
	case "dynamicToolCall":
		name = stringValue(item["tool"])
		input = stringifyJSON(item["arguments"])
	case "webSearch":
		name = "webSearch"
		input = strings.TrimSpace(firstNonEmpty(stringValue(item["query"]), stringifyJSON(item["action"])))
	}
	return strings.TrimSpace(name), strings.TrimSpace(input)
}

func reasoningSummary(value any) string {
	switch typed := value.(type) {
	case string:
		text := strings.TrimSpace(typed)
		if !isMeaningfulReasoningText(text) {
			return ""
		}
		return text
	case []any:
		parts := make([]string, 0, len(typed))
		for _, entry := range typed {
			switch summary := entry.(type) {
			case string:
				text := strings.TrimSpace(summary)
				if isMeaningfulReasoningText(text) {
					parts = append(parts, text)
				}
			case map[string]any:
				text := strings.TrimSpace(firstNonEmpty(stringValue(summary["text"]), stringValue(summary["summary"])))
				if isMeaningfulReasoningText(text) {
					parts = append(parts, text)
				}
			}
		}
		return strings.TrimSpace(strings.Join(parts, "\n\n"))
	default:
		return ""
	}
}

func reasoningContent(value any) string {
	switch typed := value.(type) {
	case nil:
		return ""
	case string:
		text := strings.TrimSpace(typed)
		if !isMeaningfulReasoningText(text) {
			return ""
		}
		return text
	case []any:
		parts := make([]string, 0, len(typed))
		for _, entry := range typed {
			text := reasoningContent(entry)
			if text != "" {
				parts = append(parts, text)
			}
		}
		return strings.TrimSpace(strings.Join(parts, "\n\n"))
	case map[string]any:
		text := strings.TrimSpace(firstNonEmpty(
			stringValue(typed["text"]),
			stringValue(typed["summary"]),
			reasoningContent(typed["content"]),
			reasoningContent(typed["items"]),
			reasoningContent(typed["parts"]),
		))
		if isMeaningfulReasoningText(text) {
			return text
		}
		fallback := strings.TrimSpace(stringifyJSON(typed))
		if !isMeaningfulReasoningText(fallback) {
			return ""
		}
		return fallback
	default:
		text := strings.TrimSpace(stringifyJSON(value))
		if !isMeaningfulReasoningText(text) {
			return ""
		}
		return text
	}
}

func isMeaningfulReasoningText(value string) bool {
	switch strings.TrimSpace(value) {
	case "", "[]", "{}", "null", `""`:
		return false
	default:
		return true
	}
}

func phaseForAttemptRole(role assistant.AttemptRole) assistant.RunPhase {
	switch role {
	case assistant.AttemptRoleGate:
		return assistant.RunPhaseGating
	case assistant.AttemptRoleAnswer:
		return assistant.RunPhaseAnswering
	case assistant.AttemptRoleProjectSelector:
		return assistant.RunPhaseSelectingProject
	case assistant.AttemptRolePlanner:
		return assistant.RunPhasePlanning
	case assistant.AttemptRoleContractor:
		return assistant.RunPhaseContracting
	case assistant.AttemptRoleEvaluator:
		return assistant.RunPhaseEvaluating
	case assistant.AttemptRoleScheduler:
		return assistant.RunPhaseScheduling
	case assistant.AttemptRoleReporter:
		return assistant.RunPhaseReporting
	default:
		return assistant.RunPhaseGenerating
	}
}

func (s *appServerTurnSession) outputBufferString(itemID string) string {
	if s.itemOutputBuffers[itemID] == nil {
		return ""
	}
	return s.itemOutputBuffers[itemID].String()
}

func (s *appServerTurnSession) setWaitRequest(wait *assistant.WaitRequest) {
	if wait == nil || s.waitRequest != nil {
		return
	}
	s.waitRequest = wait
}

func (s *appServerTurnSession) sendRequest(ctx context.Context, method string, params any) (json.RawMessage, error) {
	id := s.nextID.Add(1)
	key := strconv.FormatInt(id, 10)
	ch := make(chan appServerRPCResponse, 1)

	s.mu.Lock()
	s.pending[key] = ch
	s.mu.Unlock()

	if err := s.sendMessage(map[string]any{
		"id":     id,
		"method": method,
		"params": params,
	}); err != nil {
		s.mu.Lock()
		delete(s.pending, key)
		s.mu.Unlock()
		return nil, err
	}

	select {
	case <-ctx.Done():
		s.mu.Lock()
		delete(s.pending, key)
		s.mu.Unlock()
		return nil, ctx.Err()
	case <-s.closed:
		s.mu.Lock()
		delete(s.pending, key)
		s.mu.Unlock()
		return nil, s.closedError("codex app server closed")
	case response := <-ch:
		if response.Error != nil {
			return nil, fmt.Errorf("codex app server: %s", response.Error.Message)
		}
		return response.Result, nil
	}
}

func (s *appServerTurnSession) sendNotification(method string, params any) error {
	return s.sendMessage(map[string]any{
		"method": method,
		"params": params,
	})
}

func (s *appServerTurnSession) sendResult(id json.RawMessage, result any) error {
	var idValue any
	if err := json.Unmarshal(id, &idValue); err != nil {
		idValue = string(id)
	}
	return s.sendMessage(map[string]any{
		"id":     idValue,
		"result": result,
	})
}

func (s *appServerTurnSession) sendError(id json.RawMessage, code int, message string) error {
	var idValue any
	if err := json.Unmarshal(id, &idValue); err != nil {
		idValue = string(id)
	}
	return s.sendMessage(map[string]any{
		"id": idValue,
		"error": map[string]any{
			"code":    code,
			"message": message,
		},
	})
}

func (s *appServerTurnSession) sendMessage(payload any) error {
	line, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	if s.stdin == nil {
		return errors.New("codex app server stdin is closed")
	}
	_, err = s.stdin.Write(append(line, '\n'))
	return err
}

func (s *appServerTurnSession) threadSandboxMode() string {
	return strings.TrimSpace(firstNonEmpty(s.config.SandboxMode, "workspace-write"))
}

func (s *appServerTurnSession) turnSandboxPolicy() map[string]any {
	switch s.config.SandboxMode {
	case "danger-full-access":
		return map[string]any{"type": "dangerFullAccess"}
	case "read-only":
		return map[string]any{
			"type": "readOnly",
			"access": map[string]any{
				"type": "fullAccess",
			},
		}
	default:
		return map[string]any{
			"type":          "workspaceWrite",
			"writableRoots": []string{s.cwd},
			"networkAccess": s.config.NetworkAccess,
		}
	}
}

func (s *appServerTurnSession) prepareArtifactCapture(request CodexPhaseRequest) {
	s.browserFramePaths = nil
	s.browserFrameRel = nil
	s.runArtifactDir = ""
	s.runArtifactRelDir = ""
	if strings.TrimSpace(s.config.ArtifactDir) == "" || strings.TrimSpace(request.RunID) == "" || strings.TrimSpace(request.AttemptID) == "" {
		return
	}

	relDir := filepath.Join(firstNonEmpty(strings.TrimSpace(request.ProjectSlug), "no_project"), request.RunID, request.AttemptID)
	absDir := filepath.Join(s.config.ArtifactDir, relDir)
	if err := os.MkdirAll(absDir, 0o755); err != nil {
		return
	}
	s.runArtifactDir = absDir
	s.runArtifactRelDir = filepath.ToSlash(relDir)
}

func (s *appServerTurnSession) captureBrowserFrame(command string, sequence int) (string, string) {
	if s.runArtifactDir == "" {
		return "", ""
	}

	absPath := filepath.Join(s.runArtifactDir, fmt.Sprintf("browser-step-%03d.png", sequence))
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	args := browserScreenshotCommand(command, absPath)
	if len(args) == 0 {
		return "", ""
	}
	cmd := exec.CommandContext(ctx, args[0], args[1:]...)
	cmd.Dir = s.cwd
	if _, err := cmd.CombinedOutput(); err != nil {
		return "", ""
	}
	if _, err := os.Stat(absPath); err != nil {
		return "", ""
	}
	return absPath, filepath.ToSlash(filepath.Join(s.runArtifactRelDir, filepath.Base(absPath)))
}

func (s *appServerTurnSession) finalizeBrowserRecording() *assistant.Artifact {
	if s.runArtifactDir == "" || len(s.browserFramePaths) == 0 {
		return nil
	}
	ffmpegPath, err := exec.LookPath("ffmpeg")
	if err != nil {
		return nil
	}

	outputPath := filepath.Join(s.runArtifactDir, "browser-replay.mp4")
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	args := []string{
		"-y",
		"-framerate", "1/2",
		"-i", filepath.Join(s.runArtifactDir, "browser-step-%03d.png"),
		"-vf", "fps=30,format=yuv420p,pad=ceil(iw/2)*2:ceil(ih/2)*2",
		"-c:v", "libx264",
		"-movflags", "+faststart",
		outputPath,
	}
	cmd := exec.CommandContext(ctx, ffmpegPath, args...)
	cmd.Dir = s.cwd
	if _, err := cmd.CombinedOutput(); err != nil {
		return nil
	}
	if _, err := os.Stat(outputPath); err != nil {
		return nil
	}

	return &assistant.Artifact{
		Kind:     assistant.ArtifactKindEvidence,
		Title:    "Browser session replay",
		MIMEType: "video/mp4",
		Path:     filepath.ToSlash(filepath.Join(s.runArtifactRelDir, filepath.Base(outputPath))),
	}
}

func (s *appServerTurnSession) shutdown(err error) {
	s.closeOnce.Do(func() {
		s.mu.Lock()
		defer s.mu.Unlock()
		if err != nil && !errors.Is(err, io.EOF) {
			s.readErr = err
		}
		for key, ch := range s.pending {
			delete(s.pending, key)
			close(ch)
		}
		close(s.closed)
	})
}

func (s *appServerTurnSession) closedError(prefix string) error {
	message := prefix
	if s.readErr != nil {
		message = fmt.Sprintf("%s: %v", prefix, s.readErr)
	}
	if stderr := strings.TrimSpace(s.stderr.String()); stderr != "" {
		message = fmt.Sprintf("%s: %s", message, stderr)
	}
	return errors.New(message)
}

type waitRequestPayload struct {
	Kind        string `json:"kind"`
	Title       string `json:"title"`
	Prompt      string `json:"prompt"`
	RiskSummary string `json:"risk_summary"`
}

func (w *waitRequestPayload) toAssistantWaitRequest() *assistant.WaitRequest {
	if w == nil {
		return nil
	}
	kind := assistant.WaitKindClarification
	switch strings.TrimSpace(w.Kind) {
	case string(assistant.WaitKindApproval):
		kind = assistant.WaitKindApproval
	case string(assistant.WaitKindAuthentication):
		kind = assistant.WaitKindAuthentication
	case string(assistant.WaitKindClarification):
		kind = assistant.WaitKindClarification
	}
	return &assistant.WaitRequest{
		Kind:        kind,
		Title:       strings.TrimSpace(w.Title),
		Prompt:      strings.TrimSpace(w.Prompt),
		RiskSummary: strings.TrimSpace(w.RiskSummary),
	}
}

func waitRequestFromQuestions(questions []struct {
	Header   string `json:"header"`
	ID       string `json:"id"`
	Question string `json:"question"`
}) *assistant.WaitRequest {
	if len(questions) == 0 {
		return &assistant.WaitRequest{
			Kind:   assistant.WaitKindClarification,
			Title:  "Input required",
			Prompt: "Codex asked for additional user input before it could continue.",
		}
	}
	lines := make([]string, 0, len(questions))
	kind := assistant.WaitKindClarification
	for _, question := range questions {
		text := strings.TrimSpace(firstNonEmpty(question.Question, question.Header, question.ID))
		if text == "" {
			continue
		}
		lines = append(lines, "- "+text)
		lower := strings.ToLower(text)
		if strings.Contains(lower, "login") || strings.Contains(lower, "password") || strings.Contains(lower, "token") || strings.Contains(lower, "credential") || strings.Contains(lower, "otp") {
			kind = assistant.WaitKindAuthentication
		}
	}
	return &assistant.WaitRequest{
		Kind:   kind,
		Title:  "Input required",
		Prompt: strings.Join(lines, "\n"),
	}
}

func (s *appServerTurnSession) remoteDebugApprovalWaitRequest() *assistant.WaitRequest {
	combined := strings.ToLower(strings.Join([]string{
		s.turnErrMsg,
		s.stderr.String(),
		s.finalText,
		s.textBuilder.String(),
	}, "\n"))
	if !strings.Contains(combined, "timed out") {
		return nil
	}

	foundRemoteDebugSignal := strings.Contains(combined, "remote debugging") ||
		strings.Contains(combined, "allow remote debugging")

	for _, tool := range s.toolRuns {
		commandText := strings.ToLower(strings.TrimSpace(tool.InputSummary + "\n" + tool.OutputSummary))
		if strings.Contains(commandText, "agent-browser") &&
			strings.Contains(commandText, "--auto-connect") {
			foundRemoteDebugSignal = true
			break
		}
	}

	if !foundRemoteDebugSignal {
		return nil
	}

	return &assistant.WaitRequest{
		Kind:        assistant.WaitKindApproval,
		Title:       "Chrome approval required",
		Prompt:      "Google Chrome may be asking whether to allow remote debugging. Please click 'Allow' in Chrome, then resume the run.",
		RiskSummary: "Real Chrome CDP attach requires user approval before automation can continue.",
	}
}

func phasePromptForCodex(request CodexPhaseRequest) string {
	parts := []string{
		"You are operating inside Codex Virtual Assistant.",
		"Use available tools directly when needed. Do not describe hypothetical steps when you can execute them.",
		"Carry out the request, then notify the user of the result through the agent-message CLI before you finish the phase response.",
		"If you need login, approval, or missing business context, stop and return the required wait_request field instead of calling request_user_input.",
		"If the task needs deferred follow-up work, create scheduled runs directly instead of expecting a separate scheduler phase.",
		"Return only the schema-conforming final response for this phase.",
	}
	if strings.TrimSpace(request.RunID) != "" {
		parts = append(parts,
			fmt.Sprintf("Current run id: %s", request.RunID),
			fmt.Sprintf("You can create a scheduled run with: cva schedule create --run %s --at <scheduled_for> \"<prompt>\"", request.RunID),
			fmt.Sprintf("Equivalent API endpoint: POST /api/v1/runs/%s/scheduled with JSON {\"scheduled_for\":\"...\",\"prompt\":\"...\"}.", request.RunID),
		)
	}
	if request.Role != assistant.AttemptRoleProjectSelector && strings.TrimSpace(request.Project.Slug) != "" {
		parts = append(parts,
			fmt.Sprintf("Current project slug: %s", request.Project.Slug),
			fmt.Sprintf("Current project purpose: %s", request.Project.Description),
			"Review PROJECT.md and any existing project files when they are relevant to the task.",
		)
		if profileDir := strings.TrimSpace(request.Project.BrowserProfileDir); profileDir != "" {
			port := request.Project.BrowserCDPPort
			if port <= 0 {
				port = 9223
			}
			parts = append(parts,
				fmt.Sprintf("Project browser profile directory: %s", profileDir),
				fmt.Sprintf("Project browser CDP endpoint: http://localhost:%d", port),
				fmt.Sprintf("Before opening any new Chrome window for this project, first health-check the project CDP endpoint with curl -sS http://localhost:%d/json/version.", port),
				fmt.Sprintf("If that health check succeeds, do not launch a new Chrome window, do not call agent-browser connect, and do not call agent-browser close first; instead reuse the existing project Chrome session by passing --cdp %d directly on every agent-browser command.", port),
				fmt.Sprintf("Preferred reuse pattern: agent-browser --cdp %d open about:blank && agent-browser --cdp %d snapshot -i --json", port, port),
				fmt.Sprintf("Only if the health check fails should you launch Chrome for this project with a dedicated profile, for example on macOS: open -na \"Google Chrome\" --args --user-data-dir=%q --remote-debugging-port=%d --no-first-run --no-default-browser-check --new-window about:blank", profileDir, port),
				fmt.Sprintf("If an agent-browser command that uses --cdp %d fails or times out after the CDP health check succeeded, treat that as a stale agent-browser session problem: then try agent-browser close once, retry the same --cdp %d command, and avoid launching another Chrome window unless the CDP health check stops responding.", port, port),
				"Reuse the same project browser profile across runs so site login state persists in the profile directory.",
				"Persist auth with explicit state files instead.",
				"Use explicit auth state files only as a secondary export/import mechanism, and do not rely on --session-name for auth persistence.",
				"Use --auto-connect only when the task must attach to the user's already-running Chrome session and the project-specific browser profile cannot be used.",
				"If --auto-connect succeeds, immediately save a fresh auth state to a project-local path.",
				"After a successful state save, do not keep relying on --auto-connect in the same task unless the saved state fails and login must be recovered again.",
				"When reusing saved state, prefer opening a blank page, running agent-browser state load <path>, and only then opening the target URL instead of relying on --state during the initial open command.",
				"When using --auto-connect to attach to a real Google Chrome session, Chrome may show an 'Allow remote debugging?' dialog. If an attach attempt or the first browser command times out during that flow, do not assume the attempt failed. Ask the user to click Allow in Chrome, return a wait_request for approval, and retry after the user confirms approval.",
				"For agent-browser work, prefer commands like agent-browser open <url> --headed, then agent-browser snapshot -i --json before interacting.",
				"Keep agent-browser in foreground/headed mode unless the task explicitly requires otherwise.",
			)
		}
	}
	if len(request.Tools) > 0 {
		parts = append(parts, "Tools expected in this phase: "+strings.Join(request.Tools, ", "))
	}
	parts = append(parts, strings.TrimSpace(request.Prompt))
	return strings.TrimSpace(strings.Join(parts, "\n\n"))
}

func phaseOutputSchema(role assistant.AttemptRole) map[string]any {
	switch role {
	case assistant.AttemptRoleGate:
		return map[string]any{
			"type": "object",
			"properties": map[string]any{
				"route":   map[string]any{"type": "string", "enum": []string{"answer", "workflow"}},
				"reason":  map[string]any{"type": "string"},
				"summary": map[string]any{"type": "string"},
			},
			"required": []string{
				"route",
				"reason",
				"summary",
			},
			"additionalProperties": false,
		}
	case assistant.AttemptRoleAnswer:
		return map[string]any{
			"type": "object",
			"properties": map[string]any{
				"summary":           map[string]any{"type": "string"},
				"output":            map[string]any{"type": "string"},
				"needs_user_input":  map[string]any{"type": "boolean"},
				"wait_kind":         map[string]any{"type": "string", "enum": []string{"", "approval", "clarification", "authentication"}},
				"wait_title":        map[string]any{"type": "string"},
				"wait_prompt":       map[string]any{"type": "string"},
				"wait_risk_summary": map[string]any{"type": "string"},
			},
			"required": []string{
				"summary",
				"output",
				"needs_user_input",
				"wait_kind",
				"wait_title",
				"wait_prompt",
				"wait_risk_summary",
			},
			"additionalProperties": false,
		}
	case assistant.AttemptRoleProjectSelector:
		return map[string]any{
			"type": "object",
			"properties": map[string]any{
				"project_slug":        map[string]any{"type": "string"},
				"project_name":        map[string]any{"type": "string"},
				"project_description": map[string]any{"type": "string"},
				"summary":             map[string]any{"type": "string"},
			},
			"required": []string{
				"project_slug",
				"project_name",
				"project_description",
				"summary",
			},
			"additionalProperties": false,
		}
	case assistant.AttemptRolePlanner:
		return map[string]any{
			"type": "object",
			"properties": map[string]any{
				"goal":                    map[string]any{"type": "string"},
				"deliverables":            stringArraySchema(),
				"constraints":             stringArraySchema(),
				"tools_allowed":           stringArraySchema(),
				"tools_required":          stringArraySchema(),
				"done_definition":         stringArraySchema(),
				"evidence_required":       stringArraySchema(),
				"risk_flags":              stringArraySchema(),
				"max_generation_attempts": map[string]any{"type": "integer"},
				"schedule_plan": map[string]any{
					"anyOf": []any{
						map[string]any{"type": "null"},
						map[string]any{
							"type": "object",
							"properties": map[string]any{
								"entries": map[string]any{
									"type": "array",
									"items": map[string]any{
										"type": "object",
										"properties": map[string]any{
											"scheduled_for": map[string]any{"type": "string"},
											"prompt":        map[string]any{"type": "string"},
										},
										"required":             []string{"scheduled_for", "prompt"},
										"additionalProperties": false,
									},
								},
							},
							"required":             []string{"entries"},
							"additionalProperties": false,
						},
					},
				},
			},
			"required": []string{
				"goal",
				"deliverables",
				"constraints",
				"tools_allowed",
				"tools_required",
				"done_definition",
				"evidence_required",
				"risk_flags",
				"max_generation_attempts",
				"schedule_plan",
			},
			"additionalProperties": false,
		}
	case assistant.AttemptRoleContractor:
		return map[string]any{
			"type": "object",
			"properties": map[string]any{
				"decision":            map[string]any{"type": "string", "enum": []string{"revise", "agreed", "fail"}},
				"summary":             map[string]any{"type": "string"},
				"deliverables":        stringArraySchema(),
				"acceptance_criteria": stringArraySchema(),
				"evidence_required":   stringArraySchema(),
				"constraints":         stringArraySchema(),
				"out_of_scope":        stringArraySchema(),
				"revision_notes":      map[string]any{"type": "string"},
			},
			"required": []string{
				"decision",
				"summary",
				"deliverables",
				"acceptance_criteria",
				"evidence_required",
				"constraints",
				"out_of_scope",
				"revision_notes",
			},
			"additionalProperties": false,
		}
	case assistant.AttemptRoleEvaluator:
		return map[string]any{
			"type": "object",
			"properties": map[string]any{
				"passed":                    map[string]any{"type": "boolean"},
				"score":                     map[string]any{"type": "integer"},
				"summary":                   map[string]any{"type": "string"},
				"missing_requirements":      stringArraySchema(),
				"incorrect_claims":          stringArraySchema(),
				"evidence_checked":          stringArraySchema(),
				"next_action_for_generator": map[string]any{"type": "string"},
			},
			"required": []string{
				"passed",
				"score",
				"summary",
				"missing_requirements",
				"incorrect_claims",
				"evidence_checked",
				"next_action_for_generator",
			},
			"additionalProperties": false,
		}
	case assistant.AttemptRoleScheduler:
		return map[string]any{
			"type": "object",
			"properties": map[string]any{
				"entries": map[string]any{
					"type": "array",
					"items": map[string]any{
						"type": "object",
						"properties": map[string]any{
							"scheduled_for": map[string]any{"type": "string"},
							"prompt":        map[string]any{"type": "string"},
						},
						"required":             []string{"scheduled_for", "prompt"},
						"additionalProperties": false,
					},
				},
			},
			"required":             []string{"entries"},
			"additionalProperties": false,
		}
	case assistant.AttemptRoleReporter:
		return map[string]any{
			"type": "object",
			"properties": map[string]any{
				"summary":           map[string]any{"type": "string"},
				"delivery_status":   map[string]any{"type": "string", "enum": []string{"sent", "wait"}},
				"message_preview":   map[string]any{"type": "string"},
				"report_payload":    map[string]any{"type": "string"},
				"needs_user_input":  map[string]any{"type": "boolean"},
				"wait_kind":         map[string]any{"type": "string", "enum": []string{"", "approval", "clarification", "authentication"}},
				"wait_title":        map[string]any{"type": "string"},
				"wait_prompt":       map[string]any{"type": "string"},
				"wait_risk_summary": map[string]any{"type": "string"},
			},
			"required": []string{
				"summary",
				"delivery_status",
				"message_preview",
				"report_payload",
				"needs_user_input",
				"wait_kind",
				"wait_title",
				"wait_prompt",
				"wait_risk_summary",
			},
			"additionalProperties": false,
		}
	case assistant.AttemptRoleGenerator:
		return map[string]any{
			"type": "object",
			"properties": map[string]any{
				"summary":           map[string]any{"type": "string"},
				"output":            map[string]any{"type": "string"},
				"needs_user_input":  map[string]any{"type": "boolean"},
				"wait_kind":         map[string]any{"type": "string", "enum": []string{"", "approval", "clarification", "authentication"}},
				"wait_title":        map[string]any{"type": "string"},
				"wait_prompt":       map[string]any{"type": "string"},
				"wait_risk_summary": map[string]any{"type": "string"},
			},
			"required": []string{
				"summary",
				"output",
				"needs_user_input",
				"wait_kind",
				"wait_title",
				"wait_prompt",
				"wait_risk_summary",
			},
			"additionalProperties": false,
		}
	default:
		return nil
	}
}

func stringArraySchema() map[string]any {
	return map[string]any{
		"type":  "array",
		"items": map[string]any{"type": "string"},
	}
}

func rpcIDKey(id json.RawMessage) string {
	var stringID string
	if err := json.Unmarshal(id, &stringID); err == nil {
		return stringID
	}
	var intID int64
	if err := json.Unmarshal(id, &intID); err == nil {
		return strconv.FormatInt(intID, 10)
	}
	return string(id)
}

func stringifyJSON(value any) string {
	if value == nil {
		return ""
	}
	switch typed := value.(type) {
	case string:
		return strings.TrimSpace(typed)
	case json.RawMessage:
		return strings.TrimSpace(string(typed))
	default:
		payload, err := json.Marshal(typed)
		if err != nil {
			return ""
		}
		return strings.TrimSpace(string(payload))
	}
}

func stringValue(value any) string {
	switch typed := value.(type) {
	case string:
		return typed
	default:
		return ""
	}
}

func commandSummary(value any) string {
	switch typed := value.(type) {
	case string:
		return strings.TrimSpace(typed)
	case []any:
		parts := make([]string, 0, len(typed))
		for _, part := range typed {
			text := strings.TrimSpace(stringValue(part))
			if text != "" {
				parts = append(parts, text)
			}
		}
		return strings.Join(parts, " ")
	default:
		return stringifyJSON(value)
	}
}

func commandName(command string) string {
	fields := strings.Fields(command)
	if len(fields) == 0 {
		return ""
	}
	return fields[0]
}

func browserScreenshotCommand(command, outputPath string) []string {
	lower := strings.ToLower(command)
	switch {
	case strings.Contains(lower, "pnpm exec agent-browser"):
		return []string{"pnpm", "exec", "agent-browser", "screenshot", outputPath}
	case strings.Contains(lower, "bunx agent-browser"):
		return []string{"bunx", "agent-browser", "screenshot", outputPath}
	case strings.Contains(lower, "npx agent-browser"):
		return []string{"npx", "agent-browser", "screenshot", outputPath}
	case strings.Contains(lower, "agent-browser"):
		return []string{"agent-browser", "screenshot", outputPath}
	default:
		return nil
	}
}

func isBrowserCommand(command string) bool {
	lower := strings.ToLower(command)
	return strings.Contains(lower, "agent-browser") || strings.Contains(lower, "playwright") || strings.Contains(lower, "browser")
}

func browserTitle(command string) string {
	fields := strings.Fields(command)
	if len(fields) >= 2 {
		return strings.Join(fields[:2], " ")
	}
	return command
}

func browserAction(command string) string {
	fields := strings.Fields(command)
	if len(fields) >= 2 {
		return fields[1]
	}
	return "command"
}

func browserTarget(command string) string {
	fields := strings.Fields(command)
	if len(fields) <= 2 {
		return ""
	}
	return strings.Join(fields[2:], " ")
}

func extractFirstURL(value string) string {
	return urlPattern.FindString(value)
}

func observedText(output string) []string {
	output = strings.TrimSpace(output)
	if output == "" {
		return nil
	}
	lines := strings.Split(output, "\n")
	observed := make([]string, 0, 3)
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		observed = append(observed, line)
		if len(observed) == 3 {
			break
		}
	}
	return observed
}
