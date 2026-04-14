package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/siisee11/CodexVirtualAssistant/internal/agentmessage"
	"github.com/siisee11/CodexVirtualAssistant/internal/assistant"
	"github.com/siisee11/CodexVirtualAssistant/internal/assistantapp"
	"github.com/siisee11/CodexVirtualAssistant/internal/config"
	"github.com/siisee11/CodexVirtualAssistant/internal/policy/gan"
	"github.com/siisee11/CodexVirtualAssistant/internal/project"
	"github.com/siisee11/CodexVirtualAssistant/internal/store"
	"github.com/siisee11/CodexVirtualAssistant/internal/wiki"
	"github.com/siisee11/CodexVirtualAssistant/internal/wtl"
)

func TestRunsAPICreateAndGetRun(t *testing.T) {
	t.Parallel()

	handler := newTestAPIHandler(t, &sequenceExecutor{
		steps: []executorStep{
			{role: assistant.AttemptRoleGate, result: gatePhaseResult("workflow", "This request requires full workflow execution.")},
			{role: assistant.AttemptRoleProjectSelector, result: projectSelectorPhaseResult("competitor-pricing", "Competitor Pricing", "Track repeat competitor pricing research.")},
			{role: assistant.AttemptRolePlanner, result: plannerPhaseResult("Compare competitor pricing", []string{"Pricing table", "Summary memo"})},
			{role: assistant.AttemptRoleContractor, result: contractPhaseResult("agreed", []string{"Pricing table", "Summary memo"})},
			{role: assistant.AttemptRoleGenerator, result: generatorPhaseResult("Prepared pricing comparison draft")},
			{role: assistant.AttemptRoleEvaluator, result: evaluatorPhaseResult(true, 92, "The result package is complete.", nil, "")},
			{role: assistant.AttemptRoleReporter, result: reportPhaseResult("Delivered final report.", "Pricing comparison delivered.")},
		},
	})

	response := doJSONRequest(t, handler, http.MethodPost, "/api/v1/runs", map[string]any{
		"user_request_raw":        "Compare competitor pricing and summarize it.",
		"max_generation_attempts": 2,
	})

	if response.Code != http.StatusAccepted {
		t.Fatalf("POST /runs status = %d, want %d", response.Code, http.StatusAccepted)
	}

	var created createRunResponse
	if err := json.Unmarshal(response.Body.Bytes(), &created); err != nil {
		t.Fatalf("decode create response: %v", err)
	}

	record := waitForRunStatus(t, handler, created.Run.ID, assistant.RunStatusCompleted)
	if record.Run.LatestEvaluation == nil || !record.Run.LatestEvaluation.Passed {
		t.Fatalf("LatestEvaluation = %#v, want passed evaluation", record.Run.LatestEvaluation)
	}
	if len(record.Artifacts) == 0 || len(record.Evidence) == 0 || len(record.ToolCalls) == 0 || len(record.WebSteps) == 0 {
		t.Fatalf("record = %#v, want stored artifacts/evidence/tool calls/web steps", record)
	}
}

func TestRunsAPICreateFollowUpRunWithParentRunID(t *testing.T) {
	t.Parallel()

	handler := newTestAPIHandler(t, &sequenceExecutor{
		steps: []executorStep{
			{role: assistant.AttemptRoleGate, result: gatePhaseResult("workflow", "This request requires execution work.")},
			{role: assistant.AttemptRoleProjectSelector, result: projectSelectorPhaseResult("competitor-pricing", "Competitor Pricing", "Track repeat competitor pricing research.")},
			{role: assistant.AttemptRolePlanner, result: plannerPhaseResult("Compare competitor pricing", []string{"Pricing table"})},
			{role: assistant.AttemptRoleContractor, result: contractPhaseResult("agreed", []string{"Pricing table"})},
			{role: assistant.AttemptRoleGenerator, result: generatorPhaseResult("Prepared pricing comparison draft")},
			{role: assistant.AttemptRoleEvaluator, result: evaluatorPhaseResult(true, 93, "Initial run completed.", nil, "")},
			{role: assistant.AttemptRoleReporter, result: reportPhaseResult("Delivered initial final report.", "Initial run delivered.")},
			{role: assistant.AttemptRoleGate, result: gatePhaseResult("answer", "This follow-up can be answered from prior evidence.")},
			{role: assistant.AttemptRoleAnswer, result: answerPhaseResult("Follow-up answer generated.", "Top three cheapest competitors were A, C, and E.")},
			{role: assistant.AttemptRoleReporter, result: reportPhaseResult("Delivered follow-up report.", "Top three cheapest competitors were A, C, and E.")},
		},
	})

	initialResponse := doJSONRequest(t, handler, http.MethodPost, "/api/v1/runs", map[string]any{
		"user_request_raw": "Compare competitor pricing and summarize it.",
	})
	if initialResponse.Code != http.StatusAccepted {
		t.Fatalf("initial POST /runs status = %d, want %d", initialResponse.Code, http.StatusAccepted)
	}

	var initial createRunResponse
	if err := json.Unmarshal(initialResponse.Body.Bytes(), &initial); err != nil {
		t.Fatalf("decode initial create response: %v", err)
	}
	waitForRunStatus(t, handler, initial.Run.ID, assistant.RunStatusCompleted)

	followUpResponse := doJSONRequest(t, handler, http.MethodPost, "/api/v1/runs", map[string]any{
		"user_request_raw": "What were the top three cheapest competitors from that run?",
		"parent_run_id":    initial.Run.ID,
	})
	if followUpResponse.Code != http.StatusAccepted {
		t.Fatalf("follow-up POST /runs status = %d, want %d", followUpResponse.Code, http.StatusAccepted)
	}

	var followUp createRunResponse
	if err := json.Unmarshal(followUpResponse.Body.Bytes(), &followUp); err != nil {
		t.Fatalf("decode follow-up create response: %v", err)
	}
	if followUp.Run.ParentRunID != initial.Run.ID {
		t.Fatalf("follow-up parent_run_id = %q, want %q", followUp.Run.ParentRunID, initial.Run.ID)
	}
	if followUp.Run.ChatID != initial.Run.ChatID {
		t.Fatalf("follow-up chat_id = %q, want %q", followUp.Run.ChatID, initial.Run.ChatID)
	}

	record := waitForRunStatus(t, handler, followUp.Run.ID, assistant.RunStatusCompleted)
	if record.Run.ParentRunID != initial.Run.ID {
		t.Fatalf("stored parent_run_id = %q, want %q", record.Run.ParentRunID, initial.Run.ID)
	}
	if record.Run.GateRoute != assistant.RunRouteAnswer {
		t.Fatalf("GateRoute = %q, want %q", record.Run.GateRoute, assistant.RunRouteAnswer)
	}

	chatResponse := doJSONRequest(t, handler, http.MethodGet, "/api/v1/chats/"+initial.Run.ChatID, nil)
	if chatResponse.Code != http.StatusOK {
		t.Fatalf("GET /chats/:id status = %d, want %d", chatResponse.Code, http.StatusOK)
	}
	var chatRecord store.ChatRecord
	if err := json.Unmarshal(chatResponse.Body.Bytes(), &chatRecord); err != nil {
		t.Fatalf("decode chat response: %v", err)
	}
	if len(chatRecord.Runs) != 2 {
		t.Fatalf("len(chatRecord.Runs) = %d, want 2", len(chatRecord.Runs))
	}
}

func TestRunsAPICreateFollowUpRequiresExistingParent(t *testing.T) {
	t.Parallel()

	handler := newTestAPIHandler(t, &sequenceExecutor{
		steps: []executorStep{},
	})

	response := doJSONRequest(t, handler, http.MethodPost, "/api/v1/runs", map[string]any{
		"user_request_raw": "Follow-up question.",
		"parent_run_id":    "run_missing_parent",
	})

	if response.Code != http.StatusNotFound {
		t.Fatalf("POST /runs with missing parent status = %d, want %d", response.Code, http.StatusNotFound)
	}
}

func TestRunsAPIRejectsInputOnCompletedRunAndSuggestsFollowUp(t *testing.T) {
	t.Parallel()

	handler := newTestAPIHandler(t, &sequenceExecutor{
		steps: []executorStep{
			{role: assistant.AttemptRoleGate, result: gatePhaseResult("answer", "Simple request can be answered directly.")},
			{role: assistant.AttemptRoleAnswer, result: answerPhaseResult("Answered directly.", "Here is the direct answer.")},
			{role: assistant.AttemptRoleReporter, result: reportPhaseResult("Delivered direct answer report.", "Here is the direct answer.")},
		},
	})

	createResponse := doJSONRequest(t, handler, http.MethodPost, "/api/v1/runs", map[string]any{
		"user_request_raw": "Quick question",
	})
	var created createRunResponse
	if err := json.Unmarshal(createResponse.Body.Bytes(), &created); err != nil {
		t.Fatalf("decode create response: %v", err)
	}
	waitForRunStatus(t, handler, created.Run.ID, assistant.RunStatusCompleted)

	inputResponse := doJSONRequest(t, handler, http.MethodPost, "/api/v1/runs/"+created.Run.ID+"/input", map[string]any{
		"input": map[string]string{"response": "another question"},
	})
	if inputResponse.Code != http.StatusConflict {
		t.Fatalf("POST /input on completed run status = %d, want %d", inputResponse.Code, http.StatusConflict)
	}
	if !strings.Contains(inputResponse.Body.String(), "parent_run_id") {
		t.Fatalf("response body = %q, want parent_run_id guidance", inputResponse.Body.String())
	}
}

func TestRunsAPIListsChats(t *testing.T) {
	t.Parallel()

	handler := newTestAPIHandler(t, &sequenceExecutor{
		steps: []executorStep{
			{role: assistant.AttemptRoleGate, result: gatePhaseResult("answer", "Simple request can be answered directly.")},
			{role: assistant.AttemptRoleAnswer, result: answerPhaseResult("Answered directly.", "Here is the direct answer.")},
			{role: assistant.AttemptRoleReporter, result: reportPhaseResult("Delivered direct answer report.", "Here is the direct answer.")},
		},
	})

	createResponse := doJSONRequest(t, handler, http.MethodPost, "/api/v1/runs", map[string]any{
		"user_request_raw": "Quick question",
	})
	var created createRunResponse
	if err := json.Unmarshal(createResponse.Body.Bytes(), &created); err != nil {
		t.Fatalf("decode create response: %v", err)
	}
	waitForRunStatus(t, handler, created.Run.ID, assistant.RunStatusCompleted)

	listResponse := doJSONRequest(t, handler, http.MethodGet, "/api/v1/chats", nil)
	if listResponse.Code != http.StatusOK {
		t.Fatalf("GET /chats status = %d, want %d", listResponse.Code, http.StatusOK)
	}
	var payload struct {
		Chats []assistant.Chat `json:"chats"`
	}
	if err := json.Unmarshal(listResponse.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode chats response: %v", err)
	}
	if len(payload.Chats) != 1 {
		t.Fatalf("len(payload.Chats) = %d, want 1", len(payload.Chats))
	}
	if payload.Chats[0].ID != created.Run.ChatID {
		t.Fatalf("chat ID = %q, want %q", payload.Chats[0].ID, created.Run.ChatID)
	}
}

func TestRunsAPIInputAndResumeFlow(t *testing.T) {
	t.Parallel()

	handler := newTestAPIHandler(t, &sequenceExecutor{
		steps: []executorStep{
			{role: assistant.AttemptRoleGate, result: gatePhaseResult("workflow", "This request needs planning and execution.")},
			{role: assistant.AttemptRoleProjectSelector, result: projectSelectorPhaseResult("competitor-pricing", "Competitor Pricing", "Track repeat competitor pricing research.")},
			{role: assistant.AttemptRolePlanner, result: wtl.CodexPhaseResult{
				Summary: "Need clarification before planning.",
				WaitRequest: &assistant.WaitRequest{
					Kind:   assistant.WaitKindClarification,
					Title:  "Need competitor scope",
					Prompt: "Which competitors should be included?",
				},
			}},
			{role: assistant.AttemptRolePlanner, expectResumeInput: map[string]string{"scope": "direct SaaS competitors"}, result: plannerPhaseResult("Compare direct SaaS competitors", []string{"Pricing table"})},
			{role: assistant.AttemptRoleContractor, result: contractPhaseResult("agreed", []string{"Pricing table"})},
			{role: assistant.AttemptRoleGenerator, result: generatorPhaseResult("Prepared revised comparison draft")},
			{role: assistant.AttemptRoleEvaluator, result: evaluatorPhaseResult(true, 90, "The resumed run is complete.", nil, "")},
			{role: assistant.AttemptRoleReporter, result: reportPhaseResult("Delivered resumed final report.", "Pricing table delivered.")},
		},
	})

	createResponse := doJSONRequest(t, handler, http.MethodPost, "/api/v1/runs", map[string]any{
		"user_request_raw": "Compare competitor pricing.",
	})

	var created createRunResponse
	if err := json.Unmarshal(createResponse.Body.Bytes(), &created); err != nil {
		t.Fatalf("decode create response: %v", err)
	}

	waiting := waitForRunStatus(t, handler, created.Run.ID, assistant.RunStatusWaiting)
	if waiting.Run.WaitingFor == nil {
		t.Fatal("WaitingFor is nil, want active wait request")
	}

	inputResponse := doJSONRequest(t, handler, http.MethodPost, "/api/v1/runs/"+created.Run.ID+"/input", map[string]any{
		"input": map[string]string{"scope": "direct SaaS competitors"},
	})

	if inputResponse.Code != http.StatusAccepted {
		t.Fatalf("POST /input status = %d, want %d", inputResponse.Code, http.StatusAccepted)
	}

	completed := waitForRunStatus(t, handler, created.Run.ID, assistant.RunStatusCompleted)
	if completed.Run.WaitingFor != nil {
		t.Fatalf("WaitingFor = %#v, want nil after resume", completed.Run.WaitingFor)
	}
}

func TestRunsAPICancelAndEventsStream(t *testing.T) {
	t.Parallel()

	block := make(chan struct{})
	handler := newTestAPIHandler(t, &sequenceExecutor{
		steps: []executorStep{
			{role: assistant.AttemptRoleGate, result: gatePhaseResult("workflow", "This request needs browser execution.")},
			{role: assistant.AttemptRoleProjectSelector, result: projectSelectorPhaseResult("dashboard-inspection", "Dashboard Inspection", "Inspect and summarize dashboard work.")},
			{role: assistant.AttemptRolePlanner, result: plannerPhaseResult("Inspect the dashboard", []string{"Dashboard summary"})},
			{role: assistant.AttemptRoleContractor, result: contractPhaseResult("agreed", []string{"Dashboard summary"})},
			{role: assistant.AttemptRoleGenerator, waitForRelease: block, result: generatorPhaseResult("Prepared dashboard draft")},
			{role: assistant.AttemptRoleEvaluator, result: evaluatorPhaseResult(true, 88, "Done.", nil, "")},
			{role: assistant.AttemptRoleReporter, result: reportPhaseResult("Delivered dashboard report.", "Dashboard summary delivered.")},
		},
	})

	createResponse := doJSONRequest(t, handler, http.MethodPost, "/api/v1/runs", map[string]any{
		"user_request_raw": "Inspect the dashboard and summarize it.",
	})

	var created createRunResponse
	if err := json.Unmarshal(createResponse.Body.Bytes(), &created); err != nil {
		t.Fatalf("decode create response: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	streamRequest := httptest.NewRequest(http.MethodGet, "/api/v1/runs/"+created.Run.ID+"/events", nil).WithContext(ctx)
	streamResponse := httptest.NewRecorder()
	done := make(chan struct{})
	go func() {
		handler.ServeHTTP(streamResponse, streamRequest)
		close(done)
	}()

	close(block)

	deadline := time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) {
		if strings.Contains(streamResponse.Body.String(), `"phase":"completed"`) {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if !strings.Contains(streamResponse.Body.String(), `"phase":"completed"`) {
		t.Fatal("did not observe completed event on SSE stream")
	}
	cancel()
	<-done

	waitHandler := newTestAPIHandler(t, &sequenceExecutor{
		steps: []executorStep{
			{role: assistant.AttemptRoleGate, result: gatePhaseResult("workflow", "This request needs external access approval.")},
			{role: assistant.AttemptRoleProjectSelector, result: projectSelectorPhaseResult("dashboard-inspection", "Dashboard Inspection", "Inspect and summarize dashboard work.")},
			{role: assistant.AttemptRolePlanner, result: wtl.CodexPhaseResult{
				Summary: "Need approval before continuing.",
				WaitRequest: &assistant.WaitRequest{
					Kind:   assistant.WaitKindApproval,
					Title:  "Approval required",
					Prompt: "Approve opening the external service?",
				},
			}},
		},
	})

	waitCreate := doJSONRequest(t, waitHandler, http.MethodPost, "/api/v1/runs", map[string]any{
		"user_request_raw": "Open the external dashboard.",
	})

	var waitingCreated createRunResponse
	if err := json.Unmarshal(waitCreate.Body.Bytes(), &waitingCreated); err != nil {
		t.Fatalf("decode waiting create response: %v", err)
	}

	waitForRunStatus(t, waitHandler, waitingCreated.Run.ID, assistant.RunStatusWaiting)

	cancelResponse := doJSONRequest(t, waitHandler, http.MethodPost, "/api/v1/runs/"+waitingCreated.Run.ID+"/cancel", map[string]any{})

	if cancelResponse.Code != http.StatusOK {
		t.Fatalf("POST /cancel status = %d, want %d", cancelResponse.Code, http.StatusOK)
	}

	var cancelled store.RunRecord
	if err := json.Unmarshal(cancelResponse.Body.Bytes(), &cancelled); err != nil {
		t.Fatalf("decode cancel response: %v", err)
	}
	if cancelled.Run.Status != assistant.RunStatusCancelled {
		t.Fatalf("Run.Status = %q, want %q", cancelled.Run.Status, assistant.RunStatusCancelled)
	}
}

func TestScheduledRunsAPIListShowAndCancel(t *testing.T) {
	t.Parallel()

	handler := newTestAPIHandler(t, &sequenceExecutor{
		steps: []executorStep{
			{role: assistant.AttemptRoleGate, result: gatePhaseResult("workflow", "This request requires full workflow execution.")},
			{role: assistant.AttemptRoleProjectSelector, result: projectSelectorPhaseResult("hospital-outreach", "Hospital Outreach", "Research and contact hospitals.")},
			{role: assistant.AttemptRolePlanner, result: plannerPhaseResultWithSchedule("Research hospitals", []string{"Hospital shortlist"}, []assistant.ScheduleEntry{
				{ScheduledFor: "13:00", Prompt: "Call the first shortlisted hospital."},
			})},
			{role: assistant.AttemptRoleContractor, result: contractPhaseResult("agreed", []string{"Hospital shortlist"})},
			{role: assistant.AttemptRoleGenerator, result: generatorPhaseResult("Prepared hospital shortlist")},
			{role: assistant.AttemptRoleEvaluator, result: evaluatorPhaseResult(true, 92, "The result package is complete.", nil, "")},
			{role: assistant.AttemptRoleScheduler, result: schedulerPhaseResult([]assistant.ScheduleEntry{
				{ScheduledFor: "2026-04-03T13:00:00Z", Prompt: "Call the first shortlisted hospital."},
			})},
			{role: assistant.AttemptRoleReporter, result: reportPhaseResult("Delivered final report.", "Hospital outreach scheduled.")},
		},
	})

	createResponse := doJSONRequest(t, handler, http.MethodPost, "/api/v1/runs", map[string]any{
		"user_request_raw": "Research hospitals and schedule a call later.",
	})
	if createResponse.Code != http.StatusAccepted {
		t.Fatalf("POST /runs status = %d, want %d", createResponse.Code, http.StatusAccepted)
	}

	var created createRunResponse
	if err := json.Unmarshal(createResponse.Body.Bytes(), &created); err != nil {
		t.Fatalf("decode create response: %v", err)
	}

	record := waitForRunStatus(t, handler, created.Run.ID, assistant.RunStatusCompleted)
	if len(record.ScheduledRuns) != 1 {
		t.Fatalf("len(record.ScheduledRuns) = %d, want 1 auto-created scheduled run", len(record.ScheduledRuns))
	}

	createScheduledResponse := doJSONRequest(t, handler, http.MethodPost, "/api/v1/runs/"+created.Run.ID+"/scheduled", map[string]any{
		"scheduled_for": "2026-04-03T13:00:00Z",
		"prompt":        "Call Saint Mary Hospital at +1-555-0100.",
	})
	if createScheduledResponse.Code != http.StatusCreated {
		t.Fatalf("POST /runs/:id/scheduled status = %d, want %d", createScheduledResponse.Code, http.StatusCreated)
	}
	var createdScheduled assistant.ScheduledRun
	if err := json.Unmarshal(createScheduledResponse.Body.Bytes(), &createdScheduled); err != nil {
		t.Fatalf("decode scheduled create response: %v", err)
	}
	scheduledRunID := createdScheduled.ID

	listResponse := doJSONRequest(t, handler, http.MethodGet, "/api/v1/scheduled?chat_id="+created.Run.ChatID+"&status=pending", nil)
	if listResponse.Code != http.StatusOK {
		t.Fatalf("GET /scheduled status = %d, want %d", listResponse.Code, http.StatusOK)
	}
	var listed struct {
		ScheduledRuns []assistant.ScheduledRun `json:"scheduled_runs"`
	}
	if err := json.Unmarshal(listResponse.Body.Bytes(), &listed); err != nil {
		t.Fatalf("decode list response: %v", err)
	}
	if len(listed.ScheduledRuns) != 2 {
		t.Fatalf("listed = %#v, want 2 scheduled runs after manual creation", listed)
	}

	showResponse := doJSONRequest(t, handler, http.MethodGet, "/api/v1/scheduled/"+scheduledRunID, nil)
	if showResponse.Code != http.StatusOK {
		t.Fatalf("GET /scheduled/:id status = %d, want %d", showResponse.Code, http.StatusOK)
	}

	updateResponse := doJSONRequest(t, handler, http.MethodPost, "/api/v1/scheduled/"+scheduledRunID+"/update", map[string]any{
		"scheduled_for": "2026-04-03T14:00:00Z",
		"prompt":        "Call General Hospital at +1-555-0101.",
	})
	if updateResponse.Code != http.StatusOK {
		t.Fatalf("POST /scheduled/:id/update status = %d, want %d", updateResponse.Code, http.StatusOK)
	}
	var updated assistant.ScheduledRun
	if err := json.Unmarshal(updateResponse.Body.Bytes(), &updated); err != nil {
		t.Fatalf("decode update response: %v", err)
	}
	if updated.UserRequestRaw != "Call General Hospital at +1-555-0101." {
		t.Fatalf("updated prompt = %q, want updated scheduled prompt", updated.UserRequestRaw)
	}

	cancelResponse := doJSONRequest(t, handler, http.MethodPost, "/api/v1/scheduled/"+scheduledRunID+"/cancel", map[string]any{})
	if cancelResponse.Code != http.StatusOK {
		t.Fatalf("POST /scheduled/:id/cancel status = %d, want %d", cancelResponse.Code, http.StatusOK)
	}

	byParentResponse := doJSONRequest(t, handler, http.MethodGet, "/api/v1/runs/"+created.Run.ID+"/scheduled", nil)
	if byParentResponse.Code != http.StatusOK {
		t.Fatalf("GET /runs/:id/scheduled status = %d, want %d", byParentResponse.Code, http.StatusOK)
	}
}

func TestProjectsAPIListsWikiAndSupportsIndexAndLint(t *testing.T) {
	t.Parallel()

	handler := newTestAPIHandler(t, &sequenceExecutor{
		steps: []executorStep{
			{role: assistant.AttemptRoleGate, result: gatePhaseResult("workflow", "This request requires workflow execution.")},
			{role: assistant.AttemptRoleProjectSelector, result: projectSelectorPhaseResult("docs-bot", "Docs Bot", "Maintain documentation workflows.")},
			{role: assistant.AttemptRolePlanner, result: plannerPhaseResult("Summarize docs work", []string{"Docs summary"})},
			{role: assistant.AttemptRoleContractor, result: contractPhaseResult("agreed", []string{"Docs summary"})},
			{role: assistant.AttemptRoleGenerator, result: generatorPhaseResult("Prepared docs summary")},
			{role: assistant.AttemptRoleEvaluator, result: evaluatorPhaseResult(true, 93, "The docs summary is complete.", nil, "")},
			{role: assistant.AttemptRoleReporter, result: reportPhaseResult("Delivered final report.", "Docs summary delivered.")},
		},
	})

	createResponse := doJSONRequest(t, handler, http.MethodPost, "/api/v1/runs", map[string]any{
		"user_request_raw": "Summarize the docs migration work.",
	})
	if createResponse.Code != http.StatusAccepted {
		t.Fatalf("POST /runs status = %d, want %d", createResponse.Code, http.StatusAccepted)
	}

	var created createRunResponse
	if err := json.Unmarshal(createResponse.Body.Bytes(), &created); err != nil {
		t.Fatalf("decode create response: %v", err)
	}
	waitForRunStatus(t, handler, created.Run.ID, assistant.RunStatusCompleted)

	listResponse := doJSONRequest(t, handler, http.MethodGet, "/api/v1/projects", nil)
	if listResponse.Code != http.StatusOK {
		t.Fatalf("GET /projects status = %d, want %d", listResponse.Code, http.StatusOK)
	}
	if !strings.Contains(listResponse.Body.String(), "docs-bot") {
		t.Fatalf("projects response = %q, want docs-bot", listResponse.Body.String())
	}

	indexResponse := doJSONRequest(t, handler, http.MethodGet, "/api/v1/projects/docs-bot/wiki/index", nil)
	if indexResponse.Code != http.StatusOK {
		t.Fatalf("GET /projects/:slug/wiki/index status = %d, want %d", indexResponse.Code, http.StatusOK)
	}
	if !strings.Contains(indexResponse.Body.String(), "Wiki Index") {
		t.Fatalf("index response = %q, want wiki index content", indexResponse.Body.String())
	}

	lintResponse := doJSONRequest(t, handler, http.MethodPost, "/api/v1/projects/docs-bot/wiki/lint", map[string]any{})
	if lintResponse.Code != http.StatusOK {
		t.Fatalf("POST /projects/:slug/wiki/lint status = %d, want %d", lintResponse.Code, http.StatusOK)
	}
	if !strings.Contains(lintResponse.Body.String(), "wiki-health-") {
		t.Fatalf("lint response = %q, want lint report path", lintResponse.Body.String())
	}
}

func TestRunAPIArtifactURL(t *testing.T) {
	t.Parallel()

	artifactDir := t.TempDir()
	projectsDir := filepath.Join(t.TempDir(), "projects")
	relativePath := filepath.Join("run_123", "attempt_123", "browser-replay.mp4")
	absolutePath := filepath.Join(artifactDir, relativePath)
	if err := os.MkdirAll(filepath.Dir(absolutePath), 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.WriteFile(absolutePath, []byte("video"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	projectRelativePath := filepath.Join("docs-bot", "run_456", "attempt_789", "browser-replay.mp4")
	projectAbsolutePath := filepath.Join(projectsDir, "docs-bot", "artifacts", "run_456", "attempt_789", "browser-replay.mp4")
	if err := os.MkdirAll(filepath.Dir(projectAbsolutePath), 0o755); err != nil {
		t.Fatalf("MkdirAll(project) error = %v", err)
	}
	if err := os.WriteFile(projectAbsolutePath, []byte("video"), 0o644); err != nil {
		t.Fatalf("WriteFile(project) error = %v", err)
	}

	api := &RunAPI{cfg: config.Config{ArtifactDir: artifactDir, ProjectsDir: projectsDir}}
	if got := api.artifactURL(relativePath); got != "/artifacts/run_123/attempt_123/browser-replay.mp4" {
		t.Fatalf("artifactURL(relative) = %q", got)
	}
	if got := api.artifactURL(absolutePath); got != "/artifacts/run_123/attempt_123/browser-replay.mp4" {
		t.Fatalf("artifactURL(absolute) = %q", got)
	}
	if got := api.artifactURL(projectRelativePath); got != "/artifacts/docs-bot/run_456/attempt_789/browser-replay.mp4" {
		t.Fatalf("artifactURL(project relative) = %q", got)
	}
	if got := api.artifactURL(projectAbsolutePath); got != "/artifacts/docs-bot/run_456/attempt_789/browser-replay.mp4" {
		t.Fatalf("artifactURL(project absolute) = %q", got)
	}
	if got := api.artifactURL(filepath.Join(t.TempDir(), "outside.mp4")); got != "" {
		t.Fatalf("artifactURL(outside) = %q, want empty", got)
	}
}

func TestRunAPIHandleArtifactServesProjectArtifact(t *testing.T) {
	t.Parallel()

	dataDir := t.TempDir()
	projectsDir := filepath.Join(dataDir, "projects")
	projectArtifact := filepath.Join(projectsDir, "docs-bot", "artifacts", "run_123", "attempt_456", "browser-replay.mp4")
	if err := os.MkdirAll(filepath.Dir(projectArtifact), 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.WriteFile(projectArtifact, []byte("video"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	api := &RunAPI{cfg: config.Config{
		ArtifactDir: filepath.Join(dataDir, "artifacts"),
		ProjectsDir: projectsDir,
	}}
	request := httptest.NewRequest(http.MethodGet, path.Join("/artifacts", "docs-bot", "run_123", "attempt_456", "browser-replay.mp4"), nil)
	recorder := httptest.NewRecorder()

	api.handleArtifact(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("handleArtifact status = %d, want %d", recorder.Code, http.StatusOK)
	}
	if body := recorder.Body.String(); body != "video" {
		t.Fatalf("handleArtifact body = %q, want %q", body, "video")
	}
}

func newTestAPIHandler(t *testing.T, executor *sequenceExecutor) http.Handler {
	t.Helper()

	dataDir := t.TempDir()
	cfg := config.Config{
		HTTPAddr:              "127.0.0.1:0",
		DataDir:               dataDir,
		DatabasePath:          filepath.Join(dataDir, "assistant.db"),
		ArtifactDir:           filepath.Join(dataDir, "artifacts"),
		DefaultModel:          config.FixedModel,
		MaxGenerationAttempts: 3,
	}

	repo, err := store.OpenSQLite(cfg)
	if err != nil {
		t.Fatalf("OpenSQLite() error = %v", err)
	}
	projectManager := project.NewManager(dataDir, filepath.Join(dataDir, "projects"))
	if err := projectManager.EnsureBaseScaffold(); err != nil {
		t.Fatalf("EnsureBaseScaffold() error = %v", err)
	}
	wikiService := wiki.NewService(cfg.EffectiveProjectsDir(), time.Now)
	policy := gan.New(gan.Config{MaxGenerationAttempts: cfg.MaxGenerationAttempts})
	events := NewEventBroker()
	events.SetSnapshotLoader(repo)
	runtime := wtl.NewCodexRuntime(executor, cfg.DefaultModel, time.Now)
	engine := wtl.NewRunEngine(repo, runtime, events, policy, projectManager, wikiService, apiTestMessenger(), time.Now)
	runs := assistantapp.NewRunService(context.Background(), repo, engine, policy, time.Now)
	handler, err := NewHandler(cfg, runs, events, wikiService)
	if err != nil {
		t.Fatalf("NewHandler() error = %v", err)
	}

	t.Cleanup(func() {
		_ = runtime.Close()
		_ = repo.Close()
	})
	return handler
}

func doJSONRequest(t *testing.T, handler http.Handler, method, path string, payload any) *httptest.ResponseRecorder {
	t.Helper()

	body, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}
	request := httptest.NewRequest(method, path, bytes.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	return response
}

func waitForRunStatus(t *testing.T, handler http.Handler, runID string, want assistant.RunStatus) store.RunRecord {
	t.Helper()

	deadline := time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) {
		request := httptest.NewRequest(http.MethodGet, "/api/v1/runs/"+runID, nil)
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		body := response.Body.Bytes()
		if response.Code == http.StatusOK {
			var record store.RunRecord
			if err := json.Unmarshal(body, &record); err != nil {
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

type sequenceExecutor struct {
	mu    sync.Mutex
	steps []executorStep
	index int
}

type executorStep struct {
	role              assistant.AttemptRole
	expectResumeInput map[string]string
	waitForRelease    chan struct{}
	result            wtl.CodexPhaseResult
}

func (e *sequenceExecutor) RunPhase(_ context.Context, request wtl.CodexPhaseRequest) (wtl.CodexPhaseResult, error) {
	e.mu.Lock()
	defer e.mu.Unlock()

	if e.index >= len(e.steps) {
		return wtl.CodexPhaseResult{}, fmt.Errorf("unexpected phase request for %s", request.Role)
	}
	step := e.steps[e.index]
	e.index++
	if step.role != request.Role {
		return wtl.CodexPhaseResult{}, fmt.Errorf("role = %s, want %s", request.Role, step.role)
	}
	for key, value := range step.expectResumeInput {
		if request.ResumeInput[key] != value {
			return wtl.CodexPhaseResult{}, fmt.Errorf("resume input %q = %q, want %q", key, request.ResumeInput[key], value)
		}
	}
	if step.waitForRelease != nil {
		<-step.waitForRelease
	}
	return step.result, nil
}

func (e *sequenceExecutor) Close() error {
	return nil
}

func plannerPhaseResult(goal string, deliverables []string) wtl.CodexPhaseResult {
	output, _ := json.Marshal(map[string]any{
		"goal":                    goal,
		"deliverables":            deliverables,
		"constraints":             []string{},
		"tools_allowed":           []string{"agent-browser"},
		"tools_required":          []string{"agent-browser"},
		"done_definition":         []string{"Produce the requested deliverables"},
		"evidence_required":       []string{"Capture browser evidence"},
		"risk_flags":              []string{},
		"max_generation_attempts": 2,
	})
	return wtl.CodexPhaseResult{
		Summary: "Planner normalized the task.",
		Output:  string(output),
	}
}

func plannerPhaseResultWithSchedule(goal string, deliverables []string, entries []assistant.ScheduleEntry) wtl.CodexPhaseResult {
	output, _ := json.Marshal(map[string]any{
		"goal":                    goal,
		"deliverables":            deliverables,
		"constraints":             []string{},
		"tools_allowed":           []string{"agent-browser"},
		"tools_required":          []string{"agent-browser"},
		"done_definition":         []string{"Produce the requested deliverables"},
		"evidence_required":       []string{"Capture browser evidence"},
		"risk_flags":              []string{},
		"max_generation_attempts": 2,
		"schedule_plan": map[string]any{
			"entries": entries,
		},
	})
	return wtl.CodexPhaseResult{
		Summary: "Planner normalized the task.",
		Output:  string(output),
	}
}

func gatePhaseResult(route, reason string) wtl.CodexPhaseResult {
	output, _ := json.Marshal(map[string]any{
		"route":   route,
		"reason":  reason,
		"summary": "Gate routing complete.",
	})
	return wtl.CodexPhaseResult{
		Summary: "Gate routing complete.",
		Output:  string(output),
	}
}

func contractPhaseResult(decision string, deliverables []string) wtl.CodexPhaseResult {
	output, _ := json.Marshal(map[string]any{
		"decision":            decision,
		"summary":             "Contract review completed.",
		"deliverables":        deliverables,
		"acceptance_criteria": []string{"Produce the requested deliverables"},
		"evidence_required":   []string{"Capture browser evidence"},
		"constraints":         []string{},
		"out_of_scope":        []string{},
		"revision_notes":      "",
	})
	return wtl.CodexPhaseResult{
		Summary: "Contract agreed.",
		Output:  string(output),
	}
}

func projectSelectorPhaseResult(slug, name, description string) wtl.CodexPhaseResult {
	output, _ := json.Marshal(map[string]any{
		"project_slug":        slug,
		"project_name":        name,
		"project_description": description,
		"summary":             "Project selected.",
	})
	return wtl.CodexPhaseResult{
		Summary: "Project selected.",
		Output:  string(output),
	}
}

func generatorPhaseResult(summary string) wtl.CodexPhaseResult {
	now := time.Now().UTC()
	return wtl.CodexPhaseResult{
		Summary:      summary,
		Output:       "Generator created a draft.",
		Observations: []string{"Observed a pricing card on the browser surface."},
		Artifacts: []assistant.Artifact{
			{
				Kind:      assistant.ArtifactKindReport,
				Title:     "Draft result",
				MIMEType:  "text/markdown",
				Content:   "# Draft",
				CreatedAt: now,
			},
		},
		ToolRuns: []wtl.CodexToolRun{
			{
				Name:          "agent-browser snapshot",
				InputSummary:  "Capture page state",
				OutputSummary: "Page state captured",
				StartedAt:     now,
				FinishedAt:    now.Add(time.Second),
			},
		},
		BrowserSteps: []wtl.AgentBrowserStep{
			{
				Title:   "Viewed page",
				URL:     "https://example.com/pricing",
				Summary: "The page shows a pricing card.",
				Action: wtl.AgentBrowserAction{
					Name:   "snapshot",
					Target: "pricing page",
				},
				ObservedText: []string{"Starter plan", "$49 per month"},
				OccurredAt:   now.Add(2 * time.Second),
			},
		},
	}
}

func evaluatorPhaseResult(passed bool, score int, summary string, missing []string, nextAction string) wtl.CodexPhaseResult {
	output, _ := json.Marshal(map[string]any{
		"passed":                    passed,
		"score":                     score,
		"summary":                   summary,
		"missing_requirements":      missing,
		"incorrect_claims":          []string{},
		"evidence_checked":          []string{"artifacts", "evidence"},
		"next_action_for_generator": nextAction,
	})
	return wtl.CodexPhaseResult{
		Summary: "Evaluator finished.",
		Output:  string(output),
	}
}

func answerPhaseResult(summary, output string) wtl.CodexPhaseResult {
	return wtl.CodexPhaseResult{
		Summary: summary,
		Output:  output,
	}
}

func reportPhaseResult(summary, preview string) wtl.CodexPhaseResult {
	payload := `{"root":"screen","elements":{"screen":{"type":"Text","props":{"text":"Delivered final report."},"children":[]}}}`
	output, _ := json.Marshal(map[string]any{
		"summary":           summary,
		"delivery_status":   "sent",
		"message_preview":   preview,
		"report_payload":    payload,
		"needs_user_input":  false,
		"wait_kind":         "",
		"wait_title":        "",
		"wait_prompt":       "",
		"wait_risk_summary": "",
	})
	return wtl.CodexPhaseResult{
		Summary: summary,
		Output:  string(output),
	}
}

func schedulerPhaseResult(entries []assistant.ScheduleEntry) wtl.CodexPhaseResult {
	output, _ := json.Marshal(map[string]any{
		"entries": entries,
	})
	return wtl.CodexPhaseResult{
		Summary: "Scheduler finalized the scheduled prompts.",
		Output:  string(output),
	}
}

type apiMessenger struct{}

func apiTestMessenger() *apiMessenger {
	return &apiMessenger{}
}

func (*apiMessenger) WithChatAccount(_ context.Context, chatID string, fn func(agentmessage.ChatAccount) error) error {
	return fn(agentmessage.ChatAccount{
		ChatID: chatID,
		Name:   "cva-chat-api",
		Master: "supervisor",
	})
}

func (*apiMessenger) CatalogPrompt(context.Context, string) (string, error) {
	return "catalog prompt", nil
}

func (*apiMessenger) SendJSONRender(context.Context, string, string) error {
	return nil
}

func (*apiMessenger) ReadReplies(context.Context, string) ([]agentmessage.IncomingMessage, error) {
	return nil, nil
}

func (*apiMessenger) ReactToMessage(context.Context, string, string, string) error {
	return nil
}
