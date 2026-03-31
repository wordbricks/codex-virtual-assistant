package app

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/siisee11/CodexVirtualAssistant/internal/api"
	"github.com/siisee11/CodexVirtualAssistant/internal/assistant"
	"github.com/siisee11/CodexVirtualAssistant/internal/config"
	"github.com/siisee11/CodexVirtualAssistant/internal/store"
	"github.com/siisee11/CodexVirtualAssistant/internal/wtl"
)

func TestNewBootstrapsHTTPSurface(t *testing.T) {
	t.Parallel()

	dataDir := t.TempDir()
	cfg := config.Config{
		HTTPAddr:              "127.0.0.1:0",
		DataDir:               dataDir,
		DatabasePath:          filepath.Join(dataDir, "assistant.db"),
		ArtifactDir:           filepath.Join(dataDir, "artifacts"),
		DefaultModel:          config.FixedModel,
		MaxGenerationAttempts: 3,
		CodexBin:              "codex",
		CodexCwd:              dataDir,
		CodexApprovalPolicy:   "never",
		CodexSandboxMode:      "workspace-write",
		CodexNetworkAccess:    true,
	}

	app, err := NewWithExecutor(cfg, wtl.NewHeuristicPhaseExecutor(time.Now))
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/bootstrap", nil)
	res := httptest.NewRecorder()
	app.Handler().ServeHTTP(res, req)

	if res.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", res.Code, http.StatusOK)
	}

	var payload api.BootstrapResponse
	if err := json.Unmarshal(res.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode bootstrap response: %v", err)
	}

	if payload.DefaultModel != config.FixedModel {
		t.Fatalf("DefaultModel = %q, want %q", payload.DefaultModel, config.FixedModel)
	}
	if len(payload.RunStatuses) == 0 {
		t.Fatal("RunStatuses is empty, want scaffolded lifecycle states")
	}
}

func TestNewServesOperatorWorkspaceShell(t *testing.T) {
	t.Parallel()

	dataDir := t.TempDir()
	cfg := config.Config{
		HTTPAddr:              "127.0.0.1:0",
		DataDir:               dataDir,
		DatabasePath:          filepath.Join(dataDir, "assistant.db"),
		ArtifactDir:           filepath.Join(dataDir, "artifacts"),
		DefaultModel:          config.FixedModel,
		MaxGenerationAttempts: 3,
		CodexBin:              "codex",
		CodexCwd:              dataDir,
		CodexApprovalPolicy:   "never",
		CodexSandboxMode:      "workspace-write",
		CodexNetworkAccess:    true,
	}

	app, err := NewWithExecutor(cfg, wtl.NewHeuristicPhaseExecutor(time.Now))
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	res := httptest.NewRecorder()
	app.Handler().ServeHTTP(res, req)

	if res.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", res.Code, http.StatusOK)
	}

	body := res.Body.String()
	for _, needle := range []string{
		`<div id="root"></div>`,
		`/assets/index-`,
		`Codex Virtual Assistant`,
	} {
		if !strings.Contains(body, needle) {
			t.Fatalf("response body missing %q", needle)
		}
	}
}

func TestNewAppCompletesRunEndToEnd(t *testing.T) {
	t.Parallel()

	app := newTestApp(t)

	create := doJSONRequest(t, app.Handler(), http.MethodPost, "/api/v1/runs", map[string]any{
		"user_request_raw": "Research five competitor pricing pages and draft a comparison summary.",
	})
	if create.Code != http.StatusAccepted {
		t.Fatalf("POST /api/v1/runs status = %d, want %d", create.Code, http.StatusAccepted)
	}

	var response struct {
		Run assistant.Run `json:"run"`
	}
	if err := json.Unmarshal(create.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode create response: %v", err)
	}

	record := waitForRunStatus(t, app.Handler(), response.Run.ID, assistant.RunStatusCompleted)
	if record.Run.LatestEvaluation == nil || !record.Run.LatestEvaluation.Passed {
		t.Fatalf("LatestEvaluation = %#v, want passed evaluation", record.Run.LatestEvaluation)
	}
	if len(record.Artifacts) == 0 {
		t.Fatal("Artifacts is empty, want generated output")
	}
	if len(record.Evidence) == 0 || len(record.ToolCalls) == 0 || len(record.WebSteps) == 0 {
		t.Fatalf("record = %#v, want evidence, tool calls, and web steps", record)
	}
	if record.Run.WaitingFor != nil {
		t.Fatalf("WaitingFor = %#v, want nil after completed run", record.Run.WaitingFor)
	}
}

func TestNewAppDispatchesRunCompletedHook(t *testing.T) {
	t.Parallel()

	app := newTestApp(t)
	hooks := make(chan api.HookPayload, 1)
	unregister := app.RegisterHook(api.HookOnRunCompleted, func(_ context.Context, payload api.HookPayload) error {
		hooks <- payload
		return nil
	})
	defer unregister()

	create := doJSONRequest(t, app.Handler(), http.MethodPost, "/api/v1/runs", map[string]any{
		"user_request_raw": "Research five competitor pricing pages and draft a comparison summary.",
	})
	if create.Code != http.StatusAccepted {
		t.Fatalf("POST /api/v1/runs status = %d, want %d", create.Code, http.StatusAccepted)
	}

	var response struct {
		Run assistant.Run `json:"run"`
	}
	if err := json.Unmarshal(create.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode create response: %v", err)
	}

	record := waitForRunStatus(t, app.Handler(), response.Run.ID, assistant.RunStatusCompleted)

	select {
	case payload := <-hooks:
		if payload.Event.RunID != response.Run.ID {
			t.Fatalf("payload.Event.RunID = %q, want %q", payload.Event.RunID, response.Run.ID)
		}
		if payload.Record == nil || payload.Record.Run.Status != assistant.RunStatusCompleted {
			t.Fatalf("payload.Record = %#v, want completed snapshot", payload.Record)
		}
		if payload.Record.Run.CompletedAt == nil {
			t.Fatalf("payload.Record.Run.CompletedAt = nil, want terminal timestamp")
		}
	case <-time.After(2 * time.Second):
		t.Fatalf("timed out waiting for completed hook for run %s; final record = %#v", response.Run.ID, record.Run)
	}
}

func TestNewAppWaitsAndResumesEndToEnd(t *testing.T) {
	t.Parallel()

	app := newTestApp(t)

	create := doJSONRequest(t, app.Handler(), http.MethodPost, "/api/v1/runs", map[string]any{
		"user_request_raw": "Log in to HubSpot and update the lead status, then send a short summary.",
	})
	if create.Code != http.StatusAccepted {
		t.Fatalf("POST /api/v1/runs status = %d, want %d", create.Code, http.StatusAccepted)
	}

	var response struct {
		Run assistant.Run `json:"run"`
	}
	if err := json.Unmarshal(create.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode create response: %v", err)
	}

	waiting := waitForRunStatus(t, app.Handler(), response.Run.ID, assistant.RunStatusWaiting)
	if waiting.Run.WaitingFor == nil {
		t.Fatal("WaitingFor is nil, want active wait request")
	}
	if waiting.Run.WaitingFor.Kind != assistant.WaitKindAuthentication {
		t.Fatalf("WaitingFor.Kind = %q, want %q", waiting.Run.WaitingFor.Kind, assistant.WaitKindAuthentication)
	}

	resume := doJSONRequest(t, app.Handler(), http.MethodPost, "/api/v1/runs/"+response.Run.ID+"/resume", map[string]any{
		"input": map[string]string{
			"approval": "approved",
			"response": "Use the sales-ops workspace and continue.",
		},
	})
	if resume.Code != http.StatusAccepted {
		t.Fatalf("POST /api/v1/runs/:id/resume status = %d, want %d", resume.Code, http.StatusAccepted)
	}

	completed := waitForRunStatus(t, app.Handler(), response.Run.ID, assistant.RunStatusCompleted)
	if completed.Run.WaitingFor != nil {
		t.Fatalf("WaitingFor = %#v, want nil after resume", completed.Run.WaitingFor)
	}
	if len(completed.WaitRequests) == 0 {
		t.Fatal("WaitRequests is empty, want recorded wait history")
	}
	if len(completed.Artifacts) == 0 || !strings.Contains(completed.Artifacts[0].Content, "Use the sales-ops workspace and continue.") {
		t.Fatalf("Artifacts = %#v, want resumed input reflected in heuristic artifact", completed.Artifacts)
	}
}

func newTestApp(t *testing.T) *App {
	t.Helper()

	dataDir := t.TempDir()
	cfg := config.Config{
		HTTPAddr:              "127.0.0.1:0",
		DataDir:               dataDir,
		DatabasePath:          filepath.Join(dataDir, "assistant.db"),
		ArtifactDir:           filepath.Join(dataDir, "artifacts"),
		DefaultModel:          config.FixedModel,
		MaxGenerationAttempts: 3,
		CodexBin:              "codex",
		CodexCwd:              dataDir,
		CodexApprovalPolicy:   "never",
		CodexSandboxMode:      "workspace-write",
		CodexNetworkAccess:    true,
	}

	app, err := NewWithExecutor(cfg, wtl.NewHeuristicPhaseExecutor(time.Now))
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	t.Cleanup(func() {
		_ = app.store.Close()
		_ = app.runtime.Close()
	})
	return app
}

func doJSONRequest(t *testing.T, handler http.Handler, method, path string, payload any) *httptest.ResponseRecorder {
	t.Helper()

	body, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}
	req := httptest.NewRequest(method, path, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	res := httptest.NewRecorder()
	handler.ServeHTTP(res, req)
	return res
}

func waitForRunStatus(t *testing.T, handler http.Handler, runID string, want assistant.RunStatus) store.RunRecord {
	t.Helper()

	deadline := time.Now().Add(4 * time.Second)
	for time.Now().Before(deadline) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/runs/"+runID, nil)
		res := httptest.NewRecorder()
		handler.ServeHTTP(res, req)
		if res.Code == http.StatusOK {
			var record store.RunRecord
			if err := json.Unmarshal(res.Body.Bytes(), &record); err != nil {
				t.Fatalf("decode run record: %v", err)
			}
			if record.Run.Status == want {
				return record
			}
		}
		time.Sleep(20 * time.Millisecond)
	}

	t.Fatalf("run %s did not reach status %q", runID, want)
	return store.RunRecord{}
}
