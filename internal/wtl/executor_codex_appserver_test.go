package wtl

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/siisee11/CodexVirtualAssistant/internal/assistant"
	"github.com/siisee11/CodexVirtualAssistant/internal/prompting"
)

func TestCollectReasoningSkipsEmptyStructuredContent(t *testing.T) {
	t.Parallel()

	session := newAppServerTurnSession(AppServerPhaseExecutorConfig{}, func() time.Time {
		return time.Date(2026, time.March, 29, 3, 0, 0, 0, time.UTC)
	})
	session.runID = "run_123"
	var emitted []assistant.RunEvent
	session.liveEmit = func(event assistant.RunEvent) {
		emitted = append(emitted, event)
	}

	session.collectReasoning(map[string]any{
		"id":      "reasoning_1",
		"type":    "reasoning",
		"content": []any{},
	})

	if len(emitted) != 0 {
		t.Fatalf("collectReasoning() emitted %#v, want no events for empty content", emitted)
	}
}

func TestCollectReasoningExtractsTextContent(t *testing.T) {
	t.Parallel()

	session := newAppServerTurnSession(AppServerPhaseExecutorConfig{}, func() time.Time {
		return time.Date(2026, time.March, 29, 3, 1, 0, 0, time.UTC)
	})
	session.runID = "run_123"
	session.attemptID = "attempt_123"
	session.attemptRole = assistant.AttemptRolePlanner

	var emitted []assistant.RunEvent
	session.liveEmit = func(event assistant.RunEvent) {
		emitted = append(emitted, event)
	}

	session.collectReasoning(map[string]any{
		"id":   "reasoning_2",
		"type": "reasoning",
		"content": []any{
			map[string]any{"text": "Plan the task carefully."},
		},
	})

	if len(emitted) != 1 {
		t.Fatalf("len(emitted) = %d, want 1", len(emitted))
	}
	if emitted[0].Type != assistant.EventTypeReasoning {
		t.Fatalf("event type = %q, want %q", emitted[0].Type, assistant.EventTypeReasoning)
	}
	if emitted[0].Summary != "Plan the task carefully." {
		t.Fatalf("summary = %q, want extracted reasoning text", emitted[0].Summary)
	}
}

func TestIsMeaningfulReasoningText(t *testing.T) {
	t.Parallel()

	cases := []struct {
		value string
		want  bool
	}{
		{value: "", want: false},
		{value: "   ", want: false},
		{value: "[]", want: false},
		{value: "{}", want: false},
		{value: "null", want: false},
		{value: `""`, want: false},
		{value: "Planner normalized the task.", want: true},
	}

	for _, tc := range cases {
		if got := isMeaningfulReasoningText(tc.value); got != tc.want {
			t.Fatalf("isMeaningfulReasoningText(%q) = %t, want %t", tc.value, got, tc.want)
		}
	}
}

func TestPhaseForAttemptRoleSupportsGateAnswerAndReport(t *testing.T) {
	t.Parallel()

	if got := phaseForAttemptRole(assistant.AttemptRoleGate); got != assistant.RunPhaseGating {
		t.Fatalf("phaseForAttemptRole(gate) = %q, want %q", got, assistant.RunPhaseGating)
	}
	if got := phaseForAttemptRole(assistant.AttemptRoleAnswer); got != assistant.RunPhaseAnswering {
		t.Fatalf("phaseForAttemptRole(answer) = %q, want %q", got, assistant.RunPhaseAnswering)
	}
	if got := phaseForAttemptRole(assistant.AttemptRoleReporter); got != assistant.RunPhaseReporting {
		t.Fatalf("phaseForAttemptRole(reporter) = %q, want %q", got, assistant.RunPhaseReporting)
	}
	if got := phaseForAttemptRole(assistant.AttemptRoleScheduler); got != assistant.RunPhaseScheduling {
		t.Fatalf("phaseForAttemptRole(scheduler) = %q, want %q", got, assistant.RunPhaseScheduling)
	}
}

func TestPhaseOutputSchemaSupportsGateAnswerAndReport(t *testing.T) {
	t.Parallel()

	gate := phaseOutputSchema(assistant.AttemptRoleGate)
	if gate == nil {
		t.Fatal("phaseOutputSchema(gate) = nil")
	}
	if _, ok := gate["properties"].(map[string]any)["route"]; !ok {
		t.Fatalf("gate schema properties = %#v, want route", gate["properties"])
	}

	answer := phaseOutputSchema(assistant.AttemptRoleAnswer)
	if answer == nil {
		t.Fatal("phaseOutputSchema(answer) = nil")
	}
	properties, ok := answer["properties"].(map[string]any)
	if !ok {
		t.Fatalf("answer schema properties type = %T, want map[string]any", answer["properties"])
	}
	if _, ok := properties["needs_user_input"]; !ok {
		t.Fatalf("answer schema properties = %#v, want needs_user_input", properties)
	}
	if _, ok := properties["wait_prompt"]; !ok {
		t.Fatalf("answer schema properties = %#v, want wait_prompt", properties)
	}

	report := phaseOutputSchema(assistant.AttemptRoleReporter)
	if report == nil {
		t.Fatal("phaseOutputSchema(reporter) = nil")
	}
	reportProperties, ok := report["properties"].(map[string]any)
	if !ok {
		t.Fatalf("report schema properties type = %T, want map[string]any", report["properties"])
	}
	if _, ok := reportProperties["delivery_status"]; !ok {
		t.Fatalf("report schema properties = %#v, want delivery_status", reportProperties)
	}
	if _, ok := reportProperties["report_payload"]; !ok {
		t.Fatalf("report schema properties = %#v, want report_payload", reportProperties)
	}

	scheduler := phaseOutputSchema(assistant.AttemptRoleScheduler)
	if scheduler == nil {
		t.Fatal("phaseOutputSchema(scheduler) = nil")
	}
	schedulerProperties, ok := scheduler["properties"].(map[string]any)
	if !ok {
		t.Fatalf("scheduler schema properties type = %T, want map[string]any", scheduler["properties"])
	}
	if _, ok := schedulerProperties["entries"]; !ok {
		t.Fatalf("scheduler schema properties = %#v, want entries", schedulerProperties)
	}
}

func TestPhaseOutputSchemasAreStrictStructuredOutputSchemas(t *testing.T) {
	t.Parallel()

	roles := []assistant.AttemptRole{
		assistant.AttemptRoleGate,
		assistant.AttemptRoleAnswer,
		assistant.AttemptRoleProjectSelector,
		assistant.AttemptRolePlanner,
		assistant.AttemptRoleContractor,
		assistant.AttemptRoleEvaluator,
		assistant.AttemptRoleScheduler,
		assistant.AttemptRoleReporter,
		assistant.AttemptRoleGenerator,
	}
	for _, role := range roles {
		role := role
		t.Run(string(role), func(t *testing.T) {
			t.Parallel()

			schema := phaseOutputSchema(role)
			if schema == nil {
				t.Fatal("phaseOutputSchema() = nil")
			}
			assertStrictStructuredOutputSchema(t, string(role), schema)
		})
	}
}

func assertStrictStructuredOutputSchema(t *testing.T, path string, schema any) {
	t.Helper()

	object, ok := schema.(map[string]any)
	if !ok {
		return
	}
	if properties, ok := object["properties"].(map[string]any); ok {
		if got, ok := object["additionalProperties"].(bool); !ok || got {
			t.Fatalf("%s additionalProperties = %#v, want false", path, object["additionalProperties"])
		}
		required, ok := object["required"].([]string)
		if !ok {
			t.Fatalf("%s required type = %T, want []string", path, object["required"])
		}
		if len(required) != len(properties) {
			t.Fatalf("%s required = %#v, want exactly %d properties", path, required, len(properties))
		}
		requiredSet := make(map[string]bool, len(required))
		for _, key := range required {
			requiredSet[key] = true
		}
		for key := range properties {
			if !requiredSet[key] {
				t.Fatalf("%s required = %#v, missing %q", path, required, key)
			}
		}
		for key, property := range properties {
			assertStrictStructuredOutputSchema(t, path+"."+key, property)
		}
	}
	if alternatives, ok := object["anyOf"].([]any); ok {
		for i, alternative := range alternatives {
			assertStrictStructuredOutputSchema(t, fmt.Sprintf("%s.anyOf[%d]", path, i), alternative)
		}
	}
	if items, ok := object["items"]; ok {
		assertStrictStructuredOutputSchema(t, path+".items", items)
	}
}

func TestPhasePromptForCodexIncludesProjectBrowserProfileGuidance(t *testing.T) {
	t.Parallel()

	prompt := phasePromptForCodex(CodexPhaseRequest{
		Role:   assistant.AttemptRoleGenerator,
		Prompt: "Collect evidence and return the phase result.",
		Project: assistant.ProjectContext{
			Slug:              "x-growth",
			Description:       "Grow the X presence over repeated tasks.",
			BrowserProfileDir: "/tmp/cva/projects/x-growth/.browser-profile",
			BrowserCDPPort:    9223,
		},
	})

	if !strings.Contains(prompt, "Project browser profile directory: /tmp/cva/projects/x-growth/.browser-profile") {
		t.Fatalf("prompt = %q, want browser profile directory guidance", prompt)
	}
	if !strings.Contains(prompt, "Project browser CDP endpoint: http://localhost:9223") {
		t.Fatalf("prompt = %q, want project browser CDP endpoint guidance", prompt)
	}
	if !strings.Contains(prompt, "curl -sS http://localhost:9223/json/version") {
		t.Fatalf("prompt = %q, want CDP health check guidance", prompt)
	}
	if !strings.Contains(prompt, "open -na \"Google Chrome\"") || !strings.Contains(prompt, "agent-browser --cdp 9223 open about:blank") {
		t.Fatalf("prompt = %q, want launch fallback and direct --cdp guidance", prompt)
	}
	if !strings.Contains(prompt, "do not launch a new Chrome window") {
		t.Fatalf("prompt = %q, want existing session reuse guidance", prompt)
	}
	if !strings.Contains(prompt, "If an agent-browser command that uses --cdp 9223 fails or times out") {
		t.Fatalf("prompt = %q, want stale session recovery guidance", prompt)
	}
	if strings.Contains(prompt, "agent-browser connect http://localhost:9223") {
		t.Fatalf("prompt = %q, should not encourage direct agent-browser connect for project reuse", prompt)
	}
	if !strings.Contains(prompt, "agent-browser close once") {
		t.Fatalf("prompt = %q, want close-once recovery guidance", prompt)
	}
	if !strings.Contains(prompt, "Reuse the same project browser profile across runs") || !strings.Contains(prompt, "--session-name") {
		t.Fatalf("prompt = %q, want project profile reuse guidance and session-name warning", prompt)
	}
	if !strings.Contains(prompt, "--headed") {
		t.Fatalf("prompt = %q, want headed browser guidance", prompt)
	}
	if !strings.Contains(prompt, "Persist auth with explicit state files instead.") {
		t.Fatalf("prompt = %q, want explicit state persistence guidance", prompt)
	}
	if !strings.Contains(prompt, "--auto-connect") {
		t.Fatalf("prompt = %q, want auto-connect fallback guidance", prompt)
	}
	if !strings.Contains(prompt, "immediately save a fresh auth state to a project-local path") {
		t.Fatalf("prompt = %q, want auto-connect save guidance", prompt)
	}
	if !strings.Contains(prompt, "do not keep relying on --auto-connect in the same task") {
		t.Fatalf("prompt = %q, want auto-connect handoff guidance", prompt)
	}
	if !strings.Contains(prompt, "agent-browser state load <path>") {
		t.Fatalf("prompt = %q, want explicit state load guidance", prompt)
	}
	if !strings.Contains(prompt, "When using --auto-connect") || !strings.Contains(prompt, "return a wait_request for approval") {
		t.Fatalf("prompt = %q, want Chrome remote debugging approval guidance", prompt)
	}
	if !strings.Contains(prompt, "notify the user of the result through the agent-message CLI") {
		t.Fatalf("prompt = %q, want agent-message notification guidance", prompt)
	}
}

func TestAppServerEnvForcesAgentBrowserHeaded(t *testing.T) {
	t.Parallel()

	session := newAppServerTurnSession(AppServerPhaseExecutorConfig{
		AgentBrowserHeaded: true,
	}, func() time.Time {
		return time.Date(2026, time.April, 5, 4, 0, 0, 0, time.UTC)
	})

	env := session.appServerEnv()
	joined := strings.Join(env, "\n")
	if !strings.Contains(joined, "AGENT_BROWSER_HEADED=true") {
		t.Fatalf("env = %q, want AGENT_BROWSER_HEADED=true", joined)
	}
}

func TestDetectAgentBrowserCLIPathPrefersAssistantManagedBinary(t *testing.T) {
	t.Setenv("ASSISTANT_AGENT_BROWSER_BIN", "/managed/agent-browser")
	t.Setenv("CVA_AGENT_BROWSER_BIN", "/compat/agent-browser")

	if got := detectAgentBrowserCLIPath(); got != "/managed/agent-browser" {
		t.Fatalf("detectAgentBrowserCLIPath() = %q, want ASSISTANT_AGENT_BROWSER_BIN", got)
	}
}

func TestDetectAgentBrowserCLIPathFallsBackToCVAManagedBinary(t *testing.T) {
	t.Setenv("ASSISTANT_AGENT_BROWSER_BIN", " ")
	t.Setenv("CVA_AGENT_BROWSER_BIN", "/compat/agent-browser")

	if got := detectAgentBrowserCLIPath(); got != "/compat/agent-browser" {
		t.Fatalf("detectAgentBrowserCLIPath() = %q, want CVA_AGENT_BROWSER_BIN", got)
	}
}

func TestDetectAgentBrowserCLIPathFallsBackToSiblingManagedBinary(t *testing.T) {
	t.Setenv("ASSISTANT_AGENT_BROWSER_BIN", " ")
	t.Setenv("CVA_AGENT_BROWSER_BIN", " ")

	tempDir := t.TempDir()
	executable := filepath.Join(tempDir, "cva")
	if err := os.WriteFile(executable, []byte("cva"), 0o755); err != nil {
		t.Fatalf("WriteFile(cva) error = %v", err)
	}
	agentBrowser := filepath.Join(tempDir, "agent-browser")
	if runtime.GOOS == "windows" {
		agentBrowser += ".exe"
	}
	if err := os.WriteFile(agentBrowser, []byte("agent-browser"), 0o755); err != nil {
		t.Fatalf("WriteFile(agent-browser) error = %v", err)
	}

	originalLookupSelfExecutablePath := lookupSelfExecutablePath
	lookupSelfExecutablePath = func() (string, error) {
		return executable, nil
	}
	t.Cleanup(func() {
		lookupSelfExecutablePath = originalLookupSelfExecutablePath
	})

	if got := detectAgentBrowserCLIPath(); got != agentBrowser {
		t.Fatalf("detectAgentBrowserCLIPath() = %q, want sibling managed binary %q", got, agentBrowser)
	}
}

func TestAppServerEnvUsesProjectBrowserSettings(t *testing.T) {
	originalLookup := lookupAgentBrowserExecutablePath
	originalCLILookup := lookupAgentBrowserCLIPath
	lookupAgentBrowserExecutablePath = func() string {
		return "/Applications/Google Chrome.app/Contents/MacOS/Google Chrome"
	}
	lookupAgentBrowserCLIPath = func() string {
		return "/usr/local/bin/agent-browser"
	}
	defer func() {
		lookupAgentBrowserExecutablePath = originalLookup
		lookupAgentBrowserCLIPath = originalCLILookup
	}()

	session := newAppServerTurnSession(AppServerPhaseExecutorConfig{
		AgentBrowserHeaded: true,
	}, func() time.Time {
		return time.Date(2026, time.April, 5, 4, 0, 0, 0, time.UTC)
	})
	session.runID = "run_123"
	session.attemptID = "attempt_456"
	session.project = assistant.ProjectContext{
		BrowserProfileDir: "/tmp/cva/projects/x-growth/.browser-profile",
	}

	env := session.appServerEnv()
	joined := strings.Join(env, "\n")
	if !strings.Contains(joined, "AGENT_BROWSER_SESSION=attempt_456") {
		t.Fatalf("env = %q, want AGENT_BROWSER_SESSION=attempt_456", joined)
	}
	if !strings.Contains(joined, "AGENT_BROWSER_PROFILE=/tmp/cva/projects/x-growth/.browser-profile") {
		t.Fatalf("env = %q, want AGENT_BROWSER_PROFILE for project browser", joined)
	}
	if !strings.Contains(joined, "AGENT_BROWSER_EXECUTABLE_PATH=/Applications/Google Chrome.app/Contents/MacOS/Google Chrome") {
		t.Fatalf("env = %q, want AGENT_BROWSER_EXECUTABLE_PATH for system Chrome", joined)
	}
	if !strings.Contains(joined, "PATH=") {
		t.Fatalf("env = %q, want PATH override for agent-browser wrapper", joined)
	}
	if strings.TrimSpace(session.agentBrowserWrapperDir) == "" {
		t.Fatal("agentBrowserWrapperDir = empty, want temporary wrapper directory")
	}
	scriptBytes, err := os.ReadFile(filepath.Join(session.agentBrowserWrapperDir, "agent-browser"))
	if err != nil {
		t.Fatalf("ReadFile(wrapper) error = %v", err)
	}
	script := string(scriptBytes)
	if !strings.Contains(script, `if [[ "${1:-}" == "connect" ]]`) {
		t.Fatalf("wrapper script = %q, want connect guard", script)
	}
	if !strings.Contains(script, "agent-browser connect is disabled for project CDP reuse") {
		t.Fatalf("wrapper script = %q, want helpful connect error", script)
	}
	if !strings.Contains(script, "/usr/local/bin/agent-browser") {
		t.Fatalf("wrapper script = %q, want exec to real agent-browser binary", script)
	}
	if err := session.Close(); err != nil {
		t.Fatalf("session.Close() error = %v", err)
	}
	if _, err := os.Stat(session.agentBrowserWrapperDir); !os.IsNotExist(err) {
		t.Fatalf("wrapper dir stat err = %v, want removed dir", err)
	}
}

func TestPrepareArtifactCaptureUsesProjectArtifactDirectory(t *testing.T) {
	t.Parallel()

	baseDir := t.TempDir()
	session := newAppServerTurnSession(AppServerPhaseExecutorConfig{
		ProjectsDir: filepath.Join(baseDir, "projects"),
		ArtifactDir: filepath.Join(baseDir, "artifacts"),
	}, func() time.Time {
		return time.Date(2026, time.April, 5, 4, 0, 0, 0, time.UTC)
	})

	session.prepareArtifactCapture(CodexPhaseRequest{
		ProjectSlug: "docs-bot",
		RunID:       "run_123",
		AttemptID:   "attempt_456",
	})

	wantDir := filepath.Join(baseDir, "projects", "docs-bot", "artifacts", "run_123", "attempt_456")
	if session.runArtifactDir != wantDir {
		t.Fatalf("runArtifactDir = %q, want %q", session.runArtifactDir, wantDir)
	}
	if session.runArtifactRelDir != filepath.Join("docs-bot", "run_123", "attempt_456") {
		t.Fatalf("runArtifactRelDir = %q", session.runArtifactRelDir)
	}
	if _, err := os.Stat(wantDir); err != nil {
		t.Fatalf("artifact dir stat err = %v", err)
	}
}

func TestBuildPhaseResultPreservesReporterEnvelope(t *testing.T) {
	t.Parallel()

	session := newAppServerTurnSession(AppServerPhaseExecutorConfig{}, func() time.Time {
		return time.Date(2026, time.April, 6, 6, 1, 52, 0, time.UTC)
	})
	session.finalText = `{
		"summary": "Delivered the final report through agent-message.",
		"delivery_status": "sent",
		"message_preview": "DevNam X login check delivered.",
		"report_payload": "{\"root\":\"main\",\"elements\":{\"main\":{\"type\":\"Text\",\"props\":{\"text\":\"Outcome: not logged in\"},\"children\":[]}}}",
		"needs_user_input": false,
		"wait_kind": "",
		"wait_title": "",
		"wait_prompt": "",
		"wait_risk_summary": ""
	}`

	result := session.buildPhaseResult(CodexPhaseRequest{
		Role: assistant.AttemptRoleReporter,
	})

	output, err := prompting.DecodeReportOutput([]byte(result.Output))
	if err != nil {
		t.Fatalf("DecodeReportOutput(result.Output) error = %v", err)
	}
	if output.DeliveryStatus != "sent" {
		t.Fatalf("delivery_status = %q, want sent", output.DeliveryStatus)
	}
	if len(result.Artifacts) != 1 {
		t.Fatalf("len(result.Artifacts) = %d, want 1", len(result.Artifacts))
	}
	if got := result.Artifacts[0].Content; got != output.ReportPayload {
		t.Fatalf("artifact content = %q, want report payload %q", got, output.ReportPayload)
	}
}

func TestBuildPhaseResultPromotesChromeRemoteDebugTimeoutToWaitRequest(t *testing.T) {
	t.Parallel()

	session := newAppServerTurnSession(AppServerPhaseExecutorConfig{}, func() time.Time {
		return time.Date(2026, time.April, 6, 6, 1, 52, 0, time.UTC)
	})
	session.turnErrMsg = "Operation timed out. The page may still be loading or the element may not exist."
	session.toolRuns = append(session.toolRuns, CodexToolRun{
		Name:          "agent-browser",
		InputSummary:  "agent-browser --auto-connect open https://x.com/DevNam125129",
		OutputSummary: "Google Chrome showed an Allow remote debugging dialog before attach completed.",
	})

	result := session.buildPhaseResult(CodexPhaseRequest{
		Role: assistant.AttemptRoleGenerator,
	})

	if result.WaitRequest == nil {
		t.Fatalf("WaitRequest = nil, want Chrome approval wait request")
	}
	if got := result.WaitRequest.Kind; got != assistant.WaitKindApproval {
		t.Fatalf("WaitRequest.Kind = %q, want %q", got, assistant.WaitKindApproval)
	}
	if !strings.Contains(result.WaitRequest.Prompt, "Allow") {
		t.Fatalf("WaitRequest.Prompt = %q, want Allow guidance", result.WaitRequest.Prompt)
	}
}
