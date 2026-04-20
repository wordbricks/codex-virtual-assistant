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

	pagesResponse := doJSONRequest(t, handler, http.MethodGet, "/api/v1/projects/docs-bot/wiki/pages", nil)
	if pagesResponse.Code != http.StatusOK {
		t.Fatalf("GET /projects/:slug/wiki/pages status = %d, want %d", pagesResponse.Code, http.StatusOK)
	}
	if !strings.Contains(pagesResponse.Body.String(), "\"pages\"") || !strings.Contains(pagesResponse.Body.String(), "overview.md") {
		t.Fatalf("pages response = %q, want flat pages payload", pagesResponse.Body.String())
	}

	lintResponse := doJSONRequest(t, handler, http.MethodPost, "/api/v1/projects/docs-bot/wiki/lint", map[string]any{})
	if lintResponse.Code != http.StatusOK {
		t.Fatalf("POST /projects/:slug/wiki/lint status = %d, want %d", lintResponse.Code, http.StatusOK)
	}
	if !strings.Contains(lintResponse.Body.String(), "wiki-health-") {
		t.Fatalf("lint response = %q, want lint report path", lintResponse.Body.String())
	}
}

func TestProjectsAPIProjectDetailAndRunsEndpoints(t *testing.T) {
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

			{role: assistant.AttemptRoleGate, result: gatePhaseResult("workflow", "This request requires workflow execution.")},
			{role: assistant.AttemptRoleProjectSelector, result: projectSelectorPhaseResult("docs-bot", "Docs Bot", "Maintain documentation workflows.")},
			{role: assistant.AttemptRolePlanner, result: wtl.CodexPhaseResult{
				Summary: "Need approval before continuing.",
				WaitRequest: &assistant.WaitRequest{
					Kind:   assistant.WaitKindApproval,
					Title:  "Approval required",
					Prompt: "Approve opening the external service?",
				},
			}},

			{role: assistant.AttemptRoleGate, result: gatePhaseResult("workflow", "This request requires workflow execution.")},
			{role: assistant.AttemptRoleProjectSelector, result: projectSelectorPhaseResult("ops-bot", "Ops Bot", "Maintain operations workflows.")},
			{role: assistant.AttemptRolePlanner, result: plannerPhaseResult("Summarize ops work", []string{"Ops summary"})},
			{role: assistant.AttemptRoleContractor, result: contractPhaseResult("agreed", []string{"Ops summary"})},
			{role: assistant.AttemptRoleGenerator, result: generatorPhaseResult("Prepared ops summary")},
			{role: assistant.AttemptRoleEvaluator, result: evaluatorPhaseResult(true, 91, "The ops summary is complete.", nil, "")},
			{role: assistant.AttemptRoleReporter, result: reportPhaseResult("Delivered ops report.", "Ops summary delivered.")},
		},
	})

	completedResponse := doJSONRequest(t, handler, http.MethodPost, "/api/v1/runs", map[string]any{
		"user_request_raw": "Summarize docs migration work.",
	})
	if completedResponse.Code != http.StatusAccepted {
		t.Fatalf("POST /runs docs complete status = %d, want %d", completedResponse.Code, http.StatusAccepted)
	}
	var completed createRunResponse
	if err := json.Unmarshal(completedResponse.Body.Bytes(), &completed); err != nil {
		t.Fatalf("decode completed response: %v", err)
	}
	waitForRunStatus(t, handler, completed.Run.ID, assistant.RunStatusCompleted)

	scheduledResponse := doJSONRequest(t, handler, http.MethodPost, "/api/v1/runs/"+completed.Run.ID+"/scheduled", map[string]any{
		"scheduled_for":           "2026-04-18T12:00:00Z",
		"prompt":                  "Run a docs follow-up tomorrow.",
		"max_generation_attempts": 2,
	})
	if scheduledResponse.Code != http.StatusCreated {
		t.Fatalf("POST /runs/:id/scheduled status = %d, want %d", scheduledResponse.Code, http.StatusCreated)
	}

	waitingResponse := doJSONRequest(t, handler, http.MethodPost, "/api/v1/runs", map[string]any{
		"user_request_raw": "Open the external docs dashboard.",
	})
	if waitingResponse.Code != http.StatusAccepted {
		t.Fatalf("POST /runs docs waiting status = %d, want %d", waitingResponse.Code, http.StatusAccepted)
	}
	var waiting createRunResponse
	if err := json.Unmarshal(waitingResponse.Body.Bytes(), &waiting); err != nil {
		t.Fatalf("decode waiting response: %v", err)
	}
	waitForRunStatus(t, handler, waiting.Run.ID, assistant.RunStatusWaiting)

	otherProjectResponse := doJSONRequest(t, handler, http.MethodPost, "/api/v1/runs", map[string]any{
		"user_request_raw": "Summarize ops incident triage.",
	})
	if otherProjectResponse.Code != http.StatusAccepted {
		t.Fatalf("POST /runs ops status = %d, want %d", otherProjectResponse.Code, http.StatusAccepted)
	}
	var otherProject createRunResponse
	if err := json.Unmarshal(otherProjectResponse.Body.Bytes(), &otherProject); err != nil {
		t.Fatalf("decode ops response: %v", err)
	}
	waitForRunStatus(t, handler, otherProject.Run.ID, assistant.RunStatusCompleted)

	detailResponse := doJSONRequest(t, handler, http.MethodGet, "/api/v1/projects/docs-bot", nil)
	if detailResponse.Code != http.StatusOK {
		t.Fatalf("GET /projects/:slug status = %d, want %d", detailResponse.Code, http.StatusOK)
	}
	var detail projectDetailResponse
	if err := json.Unmarshal(detailResponse.Body.Bytes(), &detail); err != nil {
		t.Fatalf("decode project detail response: %v", err)
	}
	if detail.Project.Slug != "docs-bot" {
		t.Fatalf("detail.Project.Slug = %q, want docs-bot", detail.Project.Slug)
	}
	if detail.Stats.WaitingRuns != 1 {
		t.Fatalf("detail.Stats.WaitingRuns = %d, want 1", detail.Stats.WaitingRuns)
	}
	if detail.Stats.CompletedRuns != 1 {
		t.Fatalf("detail.Stats.CompletedRuns = %d, want 1", detail.Stats.CompletedRuns)
	}
	if detail.Stats.ScheduledRuns != 1 {
		t.Fatalf("detail.Stats.ScheduledRuns = %d, want 1", detail.Stats.ScheduledRuns)
	}
	if len(detail.RecentRuns) != 2 {
		t.Fatalf("len(detail.RecentRuns) = %d, want 2", len(detail.RecentRuns))
	}

	runsResponse := doJSONRequest(t, handler, http.MethodGet, "/api/v1/projects/docs-bot/runs?status=waiting&page=1&page_size=10", nil)
	if runsResponse.Code != http.StatusOK {
		t.Fatalf("GET /projects/:slug/runs status = %d, want %d", runsResponse.Code, http.StatusOK)
	}
	var runsPayload projectRunsResponse
	if err := json.Unmarshal(runsResponse.Body.Bytes(), &runsPayload); err != nil {
		t.Fatalf("decode project runs response: %v", err)
	}
	if len(runsPayload.Runs) != 1 || runsPayload.Runs[0].Status != assistant.RunStatusWaiting {
		t.Fatalf("runs payload = %#v, want one waiting run", runsPayload)
	}
	if runsPayload.Pagination.Total != 1 {
		t.Fatalf("runs pagination total = %d, want 1", runsPayload.Pagination.Total)
	}

	detailedRunsResponse := doJSONRequest(t, handler, http.MethodGet, "/api/v1/projects/docs-bot/runs?page=1&page_size=1&include_details=true", nil)
	if detailedRunsResponse.Code != http.StatusOK {
		t.Fatalf("GET /projects/:slug/runs include_details status = %d, want %d", detailedRunsResponse.Code, http.StatusOK)
	}
	var detailedRuns projectRunsResponse
	if err := json.Unmarshal(detailedRunsResponse.Body.Bytes(), &detailedRuns); err != nil {
		t.Fatalf("decode detailed runs response: %v", err)
	}
	if len(detailedRuns.RunRecords) != 1 {
		t.Fatalf("len(detailedRuns.RunRecords) = %d, want 1", len(detailedRuns.RunRecords))
	}

	invalidStatusResponse := doJSONRequest(t, handler, http.MethodGet, "/api/v1/projects/docs-bot/runs?status=unknown_status", nil)
	if invalidStatusResponse.Code != http.StatusBadRequest {
		t.Fatalf("GET /projects/:slug/runs invalid status = %d, want %d", invalidStatusResponse.Code, http.StatusBadRequest)
	}

	missingProjectResponse := doJSONRequest(t, handler, http.MethodGet, "/api/v1/projects/missing-project", nil)
	if missingProjectResponse.Code != http.StatusNotFound {
		t.Fatalf("GET /projects/missing status = %d, want %d", missingProjectResponse.Code, http.StatusNotFound)
	}
	missingProjectRunsResponse := doJSONRequest(t, handler, http.MethodGet, "/api/v1/projects/missing-project/runs", nil)
	if missingProjectRunsResponse.Code != http.StatusNotFound {
		t.Fatalf("GET /projects/missing/runs status = %d, want %d", missingProjectRunsResponse.Code, http.StatusNotFound)
	}
}

func TestRunsAPICreateRunWithProjectSlugSkipsProjectSelector(t *testing.T) {
	t.Parallel()

	handler := newTestAPIHandler(t, &sequenceExecutor{
		steps: []executorStep{
			{role: assistant.AttemptRoleGate, result: gatePhaseResult("workflow", "This request requires workflow execution.")},
			{role: assistant.AttemptRoleProjectSelector, result: projectSelectorPhaseResult("docs-bot", "Docs Bot", "Maintain documentation workflows.")},
			{role: assistant.AttemptRolePlanner, result: plannerPhaseResult("Seed docs-bot", []string{"Seed docs"})},
			{role: assistant.AttemptRoleContractor, result: contractPhaseResult("agreed", []string{"Seed docs"})},
			{role: assistant.AttemptRoleGenerator, result: generatorPhaseResult("Prepared seed docs output")},
			{role: assistant.AttemptRoleEvaluator, result: evaluatorPhaseResult(true, 92, "Seed docs output complete.", nil, "")},
			{role: assistant.AttemptRoleReporter, result: reportPhaseResult("Delivered seed report.", "Seed docs delivered.")},

			// Explicit project_slug run intentionally omits AttemptRoleProjectSelector.
			{role: assistant.AttemptRoleGate, result: gatePhaseResult("workflow", "This request requires workflow execution.")},
			{role: assistant.AttemptRolePlanner, result: plannerPhaseResult("Bound docs-bot", []string{"Bound docs"})},
			{role: assistant.AttemptRoleContractor, result: contractPhaseResult("agreed", []string{"Bound docs"})},
			{role: assistant.AttemptRoleGenerator, result: generatorPhaseResult("Prepared bound docs output")},
			{role: assistant.AttemptRoleEvaluator, result: evaluatorPhaseResult(true, 94, "Bound docs output complete.", nil, "")},
			{role: assistant.AttemptRoleReporter, result: reportPhaseResult("Delivered bound report.", "Bound docs delivered.")},
		},
	})

	seedResponse := doJSONRequest(t, handler, http.MethodPost, "/api/v1/runs", map[string]any{
		"user_request_raw": "Create docs-bot project context.",
	})
	if seedResponse.Code != http.StatusAccepted {
		t.Fatalf("seed POST /runs status = %d, want %d", seedResponse.Code, http.StatusAccepted)
	}
	var seedCreated createRunResponse
	if err := json.Unmarshal(seedResponse.Body.Bytes(), &seedCreated); err != nil {
		t.Fatalf("decode seed create response: %v", err)
	}
	waitForRunStatus(t, handler, seedCreated.Run.ID, assistant.RunStatusCompleted)

	boundResponse := doJSONRequest(t, handler, http.MethodPost, "/api/v1/runs", map[string]any{
		"user_request_raw": "Continue docs-bot work with explicit project binding.",
		"project_slug":     "docs-bot",
	})
	if boundResponse.Code != http.StatusAccepted {
		t.Fatalf("bound POST /runs status = %d, want %d", boundResponse.Code, http.StatusAccepted)
	}
	var boundCreated createRunResponse
	if err := json.Unmarshal(boundResponse.Body.Bytes(), &boundCreated); err != nil {
		t.Fatalf("decode bound create response: %v", err)
	}
	record := waitForRunStatus(t, handler, boundCreated.Run.ID, assistant.RunStatusCompleted)
	if record.Run.Project.Slug != "docs-bot" {
		t.Fatalf("record.Run.Project.Slug = %q, want docs-bot", record.Run.Project.Slug)
	}

	missingProjectResponse := doJSONRequest(t, handler, http.MethodPost, "/api/v1/runs", map[string]any{
		"user_request_raw": "This should fail because project is missing.",
		"project_slug":     "missing-project",
	})
	if missingProjectResponse.Code != http.StatusNotFound {
		t.Fatalf("POST /runs with missing project status = %d, want %d", missingProjectResponse.Code, http.StatusNotFound)
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

func TestRunAPIServesIndexForSPARoutes(t *testing.T) {
	t.Parallel()

	handler := newTestAPIHandler(t, &sequenceExecutor{})

	response := doJSONRequest(t, handler, http.MethodGet, "/projects/docs-bot/wiki", nil)
	if response.Code != http.StatusOK {
		t.Fatalf("GET SPA route status = %d, want %d", response.Code, http.StatusOK)
	}
	if contentType := response.Header().Get("Content-Type"); !strings.Contains(contentType, "text/html") {
		t.Fatalf("Content-Type = %q, want text/html", contentType)
	}
	if !strings.Contains(response.Body.String(), `id="root"`) {
		t.Fatalf("SPA route body = %q, want app index", response.Body.String())
	}

	headResponse := doJSONRequest(t, handler, http.MethodHead, "/projects/docs-bot/wiki", nil)
	if headResponse.Code != http.StatusOK {
		t.Fatalf("HEAD SPA route status = %d, want %d", headResponse.Code, http.StatusOK)
	}
	if headResponse.Body.Len() != 0 {
		t.Fatalf("HEAD SPA route body length = %d, want 0", headResponse.Body.Len())
	}

	apiResponse := doJSONRequest(t, handler, http.MethodGet, "/api/v1/not-found", nil)
	if apiResponse.Code != http.StatusNotFound {
		t.Fatalf("GET missing API route status = %d, want %d", apiResponse.Code, http.StatusNotFound)
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
	trackedEngine := &trackedEngine{inner: engine}
	runs := assistantapp.NewRunService(context.Background(), repo, trackedEngine, policy, time.Now)
	handler, err := NewHandler(cfg, runs, events, wikiService)
	if err != nil {
		t.Fatalf("NewHandler() error = %v", err)
	}

	t.Cleanup(func() {
		trackedEngine.Wait()
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

type trackedEngine struct {
	inner wtl.Engine
	wg    sync.WaitGroup
}

func (e *trackedEngine) Start(ctx context.Context, run assistant.Run) error {
	e.wg.Add(1)
	defer e.wg.Done()
	return e.inner.Start(ctx, run)
}

func (e *trackedEngine) Resume(ctx context.Context, runID string, input map[string]string) error {
	e.wg.Add(1)
	defer e.wg.Done()
	return e.inner.Resume(ctx, runID, input)
}

func (e *trackedEngine) Cancel(ctx context.Context, runID string) error {
	return e.inner.Cancel(ctx, runID)
}

func (e *trackedEngine) Wait() {
	e.wg.Wait()
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
