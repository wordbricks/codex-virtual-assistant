package wtl

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/siisee11/CodexVirtualAssistant/internal/agentmessage"
	"github.com/siisee11/CodexVirtualAssistant/internal/assistant"
	"github.com/siisee11/CodexVirtualAssistant/internal/config"
	"github.com/siisee11/CodexVirtualAssistant/internal/policy/gan"
	"github.com/siisee11/CodexVirtualAssistant/internal/project"
	"github.com/siisee11/CodexVirtualAssistant/internal/store"
	"github.com/siisee11/CodexVirtualAssistant/internal/wiki"
)

func TestRunEngineRetriesGeneratorUntilEvaluationPasses(t *testing.T) {
	t.Parallel()

	repo, dataDir := openEngineTestRepository(t)
	now := time.Date(2026, time.March, 27, 14, 0, 0, 0, time.UTC)
	runtime := &scriptedRuntime{
		steps: map[assistant.AttemptRole][]runtimeStep{
			assistant.AttemptRoleGate: {
				{response: PhaseResponse{Summary: "Gate routed to workflow.", Output: gateJSON("workflow", "This request needs full execution work.")}},
			},
			assistant.AttemptRoleProjectSelector: {
				{response: PhaseResponse{Summary: "Selected the competitor research project.", Output: selectorJSON("competitor-pricing", "Competitor Pricing", "Track repeat competitor pricing research.")}},
			},
			assistant.AttemptRolePlanner: {
				{response: PhaseResponse{Summary: "Planner normalized the request.", Output: plannerJSON("Compare competitor pricing", []string{"Pricing table", "Summary memo"})}},
			},
			assistant.AttemptRoleContractor: {
				{response: PhaseResponse{Summary: "Contract agreed.", Output: contractJSON("agreed", []string{"Pricing table", "Summary memo"}, []string{"Each competitor row includes a direct source URL"}, "")}},
			},
			assistant.AttemptRoleGenerator: {
				{response: PhaseResponse{Summary: "Drafted the first comparison table.", Artifacts: []assistant.Artifact{{Kind: assistant.ArtifactKindTable, Title: "Draft comparison", MIMEType: "text/markdown", Content: "| Vendor | Price |\n| --- | --- |\n| A | $49 |"}}}},
				{expectCritique: "Add direct source URLs for each competitor.", response: PhaseResponse{Summary: "Updated the table with source links.", Artifacts: []assistant.Artifact{{Kind: assistant.ArtifactKindTable, Title: "Updated comparison", MIMEType: "text/markdown", Content: "| Vendor | Price | Source |\n| --- | --- | --- |\n| A | $49 | example.com |"}}}},
			},
			assistant.AttemptRoleEvaluator: {
				{response: PhaseResponse{Summary: "First evaluation complete.", Output: evaluatorJSON(false, 72, "The table is missing source URLs.", []string{"Direct source URLs for each competitor"}, "Add direct source URLs for each competitor.")}},
				{response: PhaseResponse{Summary: "Second evaluation complete.", Output: evaluatorJSON(true, 94, "The comparison now meets the done definition.", nil, "")}},
			},
			assistant.AttemptRoleReporter: {
				{response: PhaseResponse{Summary: "Delivered final report.", Output: reportJSON("Delivered final report.", "Pricing comparison delivered.")}},
			},
		},
	}
	observer := &capturingObserver{}
	projectManager := newEngineTestProjectManager(t, dataDir)
	engine := NewRunEngine(repo, runtime, observer, gan.New(gan.Config{MaxGenerationAttempts: 2}), projectManager, newEngineTestWikiService(dataDir), fakeMessenger(), fixedClock(now))

	run := assistant.NewRun("Compare competitor pricing and summarize it.", now, 2)
	if err := engine.Start(context.Background(), run); err != nil {
		t.Fatalf("Start() error = %v", err)
	}

	record, err := repo.GetRunRecord(context.Background(), run.ID)
	if err != nil {
		t.Fatalf("GetRunRecord() error = %v", err)
	}

	if record.Run.Status != assistant.RunStatusCompleted {
		t.Fatalf("Run.Status = %q, want %q", record.Run.Status, assistant.RunStatusCompleted)
	}
	if len(record.Attempts) != 9 {
		t.Fatalf("len(Attempts) = %d, want 9", len(record.Attempts))
	}
	if len(record.Evaluations) != 2 {
		t.Fatalf("len(Evaluations) = %d, want 2", len(record.Evaluations))
	}
	if record.Run.LatestEvaluation == nil || !record.Run.LatestEvaluation.Passed {
		t.Fatalf("LatestEvaluation = %#v, want passed evaluation", record.Run.LatestEvaluation)
	}
	if !record.Run.TaskSpec.HasAcceptedContract() {
		t.Fatalf("AcceptanceContract = %#v, want agreed contract", record.Run.TaskSpec.AcceptanceContract)
	}
	if len(record.Artifacts) != 3 {
		t.Fatalf("len(Artifacts) = %d, want 3", len(record.Artifacts))
	}
	if record.Attempts[0].Role != assistant.AttemptRoleGate {
		t.Fatalf("Attempts[0].Role = %q, want %q", record.Attempts[0].Role, assistant.AttemptRoleGate)
	}
	if len(observer.events) == 0 {
		t.Fatal("observer events are empty")
	}
}

func TestRunEngineWaitsAndResumes(t *testing.T) {
	t.Parallel()

	repo, dataDir := openEngineTestRepository(t)
	now := time.Date(2026, time.March, 27, 15, 0, 0, 0, time.UTC)
	runtime := &scriptedRuntime{
		steps: map[assistant.AttemptRole][]runtimeStep{
			assistant.AttemptRoleGate: {
				{response: PhaseResponse{Summary: "Gate routed to workflow.", Output: gateJSON("workflow", "Planner and execution are required.")}},
			},
			assistant.AttemptRoleProjectSelector: {
				{response: PhaseResponse{Summary: "Selected the competitor research project.", Output: selectorJSON("competitor-pricing", "Competitor Pricing", "Track repeat competitor pricing research.")}},
			},
			assistant.AttemptRolePlanner: {
				{response: PhaseResponse{Summary: "Planner needs clarification.", WaitRequest: &assistant.WaitRequest{Kind: assistant.WaitKindClarification, Title: "Need competitor scope", Prompt: "Which competitors should be included?"}}},
				{expectResumeInput: map[string]string{"competitors": "Only direct SaaS competitors"}, response: PhaseResponse{Summary: "Planner resumed.", Output: plannerJSON("Compare direct SaaS competitors", []string{"Pricing table"})}},
			},
			assistant.AttemptRoleContractor: {
				{response: PhaseResponse{Summary: "Contract agreed.", Output: contractJSON("agreed", []string{"Pricing table"}, []string{"The table covers direct SaaS competitors only"}, "")}},
			},
			assistant.AttemptRoleGenerator: {
				{response: PhaseResponse{Summary: "Generated the requested table."}},
			},
			assistant.AttemptRoleEvaluator: {
				{response: PhaseResponse{Summary: "Evaluation complete.", Output: evaluatorJSON(true, 90, "The run is complete.", nil, "")}},
			},
			assistant.AttemptRoleReporter: {
				{response: PhaseResponse{Summary: "Delivered final report.", Output: reportJSON("Delivered final report.", "Pricing table delivered.")}},
			},
		},
	}
	engine := NewRunEngine(repo, runtime, &capturingObserver{}, gan.New(gan.Config{MaxGenerationAttempts: 2}), newEngineTestProjectManager(t, dataDir), newEngineTestWikiService(dataDir), fakeMessenger(), fixedClock(now))

	run := assistant.NewRun("Compare competitor pricing.", now, 2)
	if err := engine.Start(context.Background(), run); err != nil {
		t.Fatalf("Start() error = %v", err)
	}

	waitingRecord, err := repo.GetRunRecord(context.Background(), run.ID)
	if err != nil {
		t.Fatalf("GetRunRecord() waiting error = %v", err)
	}
	if waitingRecord.Run.Status != assistant.RunStatusWaiting {
		t.Fatalf("Run.Status = %q, want %q", waitingRecord.Run.Status, assistant.RunStatusWaiting)
	}
	if waitingRecord.Run.WaitingFor == nil {
		t.Fatal("WaitingFor is nil, want active wait request")
	}

	if err := engine.Resume(context.Background(), run.ID, map[string]string{"competitors": "Only direct SaaS competitors"}); err != nil {
		t.Fatalf("Resume() error = %v", err)
	}

	record, err := repo.GetRunRecord(context.Background(), run.ID)
	if err != nil {
		t.Fatalf("GetRunRecord() error = %v", err)
	}
	if record.Run.Status != assistant.RunStatusCompleted {
		t.Fatalf("Run.Status = %q, want %q", record.Run.Status, assistant.RunStatusCompleted)
	}
	if record.Run.WaitingFor != nil {
		t.Fatalf("WaitingFor = %#v, want nil after resume completion", record.Run.WaitingFor)
	}
	if len(record.WaitRequests) != 1 {
		t.Fatalf("len(WaitRequests) = %d, want 1", len(record.WaitRequests))
	}
}

func TestRunEngineFailsWhenContractKeepsRevising(t *testing.T) {
	t.Parallel()

	repo, dataDir := openEngineTestRepository(t)
	now := time.Date(2026, time.March, 27, 15, 30, 0, 0, time.UTC)
	runtime := &scriptedRuntime{
		steps: map[assistant.AttemptRole][]runtimeStep{
			assistant.AttemptRoleGate: {
				{response: PhaseResponse{Summary: "Gate routed to workflow.", Output: gateJSON("workflow", "This request needs full execution work.")}},
			},
			assistant.AttemptRoleProjectSelector: {
				{response: PhaseResponse{Summary: "Selected the competitor research project.", Output: selectorJSON("competitor-pricing", "Competitor Pricing", "Track repeat competitor pricing research.")}},
			},
			assistant.AttemptRolePlanner: {
				{response: PhaseResponse{Summary: "Planner normalized the request.", Output: plannerJSON("Compare competitor pricing", []string{"Pricing table", "Summary memo"})}},
			},
			assistant.AttemptRoleContractor: {
				{response: PhaseResponse{Summary: "Contract needs revision.", Output: contractJSON("revise", []string{"Pricing table", "Summary memo"}, []string{"Each competitor row includes a direct source URL"}, "Tighten the evidence requirements.")}},
				{response: PhaseResponse{Summary: "Contract still needs revision.", Output: contractJSON("revise", []string{"Pricing table", "Summary memo"}, []string{"Each competitor row includes a direct source URL"}, "Clarify the exact acceptance criteria.")}},
				{response: PhaseResponse{Summary: "Contract remains unresolved.", Output: contractJSON("revise", []string{"Pricing table", "Summary memo"}, []string{"Each competitor row includes a direct source URL"}, "Contract is still not converging.")}},
			},
		},
	}
	engine := NewRunEngine(repo, runtime, &capturingObserver{}, gan.New(gan.Config{MaxGenerationAttempts: 2}), newEngineTestProjectManager(t, dataDir), newEngineTestWikiService(dataDir), fakeMessenger(), fixedClock(now))

	run := assistant.NewRun("Compare competitor pricing and summarize it.", now, 2)
	if err := engine.Start(context.Background(), run); !errors.Is(err, ErrInvalidRunState) {
		t.Fatalf("Start() error = %v, want ErrInvalidRunState", err)
	}

	record, err := repo.GetRunRecord(context.Background(), run.ID)
	if err != nil {
		t.Fatalf("GetRunRecord() error = %v", err)
	}

	if record.Run.Status != assistant.RunStatusFailed {
		t.Fatalf("Run.Status = %q, want %q", record.Run.Status, assistant.RunStatusFailed)
	}
	if len(record.Attempts) != 6 {
		t.Fatalf("len(Attempts) = %d, want 6", len(record.Attempts))
	}
	if got := countAttemptsByRole(record.Attempts, assistant.AttemptRoleContractor); got != maxContractRevisionAttempts {
		t.Fatalf("contract attempts = %d, want %d", got, maxContractRevisionAttempts)
	}
	if record.Run.TaskSpec.AcceptanceContract == nil || record.Run.TaskSpec.AcceptanceContract.Status != assistant.ContractStatusDraft {
		t.Fatalf("AcceptanceContract = %#v, want final draft contract snapshot", record.Run.TaskSpec.AcceptanceContract)
	}
}

func TestRunEngineCancelStopsWaitingRun(t *testing.T) {
	t.Parallel()

	repo, dataDir := openEngineTestRepository(t)
	now := time.Date(2026, time.March, 27, 16, 0, 0, 0, time.UTC)
	runtime := &scriptedRuntime{
		steps: map[assistant.AttemptRole][]runtimeStep{
			assistant.AttemptRoleGate: {
				{response: PhaseResponse{Summary: "Gate routed to workflow.", Output: gateJSON("workflow", "This requires external dashboard work.")}},
			},
			assistant.AttemptRoleProjectSelector: {
				{response: PhaseResponse{Summary: "Selected the dashboard project.", Output: selectorJSON("dashboard-inspection", "Dashboard Inspection", "Inspect and summarize dashboard-related work.")}},
			},
			assistant.AttemptRolePlanner: {
				{response: PhaseResponse{Summary: "Planner needs login approval.", WaitRequest: &assistant.WaitRequest{Kind: assistant.WaitKindApproval, Title: "Approval needed", Prompt: "Approve logging in to the dashboard?"}}},
			},
		},
	}
	engine := NewRunEngine(repo, runtime, &capturingObserver{}, gan.New(gan.Config{MaxGenerationAttempts: 1}), newEngineTestProjectManager(t, dataDir), newEngineTestWikiService(dataDir), fakeMessenger(), fixedClock(now))

	run := assistant.NewRun("Open the dashboard and inspect the numbers.", now, 1)
	if err := engine.Start(context.Background(), run); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	if err := engine.Cancel(context.Background(), run.ID); err != nil {
		t.Fatalf("Cancel() error = %v", err)
	}

	record, err := repo.GetRunRecord(context.Background(), run.ID)
	if err != nil {
		t.Fatalf("GetRunRecord() error = %v", err)
	}
	if record.Run.Status != assistant.RunStatusCancelled {
		t.Fatalf("Run.Status = %q, want %q", record.Run.Status, assistant.RunStatusCancelled)
	}
	if err := engine.Resume(context.Background(), run.ID, map[string]string{"approved": "yes"}); !errors.Is(err, ErrInvalidRunState) {
		t.Fatalf("Resume() error = %v, want ErrInvalidRunState", err)
	}
}

func TestRunEngineGateRoutesToAnswerAndCompletes(t *testing.T) {
	t.Parallel()

	repo, dataDir := openEngineTestRepository(t)
	now := time.Date(2026, time.March, 27, 16, 30, 0, 0, time.UTC)
	runtime := &scriptedRuntime{
		steps: map[assistant.AttemptRole][]runtimeStep{
			assistant.AttemptRoleGate: {
				{response: PhaseResponse{Summary: "Gate routed to answer.", Output: gateJSON("answer", "The request is a read-only follow-up.")}},
			},
			assistant.AttemptRoleAnswer: {
				{response: PhaseResponse{Summary: "Prepared direct answer.", Output: "Top 3 cheapest competitors: A, C, E."}},
			},
			assistant.AttemptRoleReporter: {
				{response: PhaseResponse{Summary: "Delivered final report.", Output: reportJSON("Delivered final report.", "Top 3 cheapest competitors: A, C, E.")}},
			},
		},
	}
	engine := NewRunEngine(repo, runtime, &capturingObserver{}, gan.New(gan.Config{MaxGenerationAttempts: 1}), newEngineTestProjectManager(t, dataDir), newEngineTestWikiService(dataDir), fakeMessenger(), fixedClock(now))

	run := assistant.NewRun("What were the top 3 cheapest competitors from the previous run?", now, 1)
	if err := engine.Start(context.Background(), run); err != nil {
		t.Fatalf("Start() error = %v", err)
	}

	record, err := repo.GetRunRecord(context.Background(), run.ID)
	if err != nil {
		t.Fatalf("GetRunRecord() error = %v", err)
	}
	if record.Run.Status != assistant.RunStatusCompleted {
		t.Fatalf("Run.Status = %q, want %q", record.Run.Status, assistant.RunStatusCompleted)
	}
	if record.Run.GateRoute != assistant.RunRouteAnswer {
		t.Fatalf("GateRoute = %q, want %q", record.Run.GateRoute, assistant.RunRouteAnswer)
	}
	if len(record.Attempts) != 3 {
		t.Fatalf("len(Attempts) = %d, want 3", len(record.Attempts))
	}
	if record.Attempts[0].Role != assistant.AttemptRoleGate || record.Attempts[1].Role != assistant.AttemptRoleAnswer || record.Attempts[2].Role != assistant.AttemptRoleReporter {
		t.Fatalf("attempt roles = %#v, want gate -> answer -> reporter", []assistant.AttemptRole{record.Attempts[0].Role, record.Attempts[1].Role, record.Attempts[2].Role})
	}
	if !artifactsContain(record.Artifacts, "Top 3 cheapest competitors") {
		t.Fatalf("Artifacts = %#v, want persisted answer artifact", record.Artifacts)
	}
}

func TestRunEngineRetriesAnswerAfterTransientExecutorClosure(t *testing.T) {
	t.Parallel()

	repo, dataDir := openEngineTestRepository(t)
	now := time.Date(2026, time.March, 27, 16, 45, 0, 0, time.UTC)
	runtime := &scriptedRuntime{
		steps: map[assistant.AttemptRole][]runtimeStep{
			assistant.AttemptRoleGate: {
				{response: PhaseResponse{Summary: "Gate routed to answer.", Output: gateJSON("answer", "The request is a simple greeting.")}},
			},
			assistant.AttemptRoleAnswer: {
				{err: errors.New("codex app server closed during phase execution")},
				{response: PhaseResponse{Summary: "Greeted the user.", Output: "Hi there!"}},
			},
			assistant.AttemptRoleReporter: {
				{response: PhaseResponse{Summary: "Delivered final report.", Output: reportJSON("Delivered final report.", "Hi there!")}},
			},
		},
	}
	engine := NewRunEngine(repo, runtime, &capturingObserver{}, gan.New(gan.Config{MaxGenerationAttempts: 1}), newEngineTestProjectManager(t, dataDir), newEngineTestWikiService(dataDir), fakeMessenger(), fixedClock(now))

	run := assistant.NewRun("hi", now, 1)
	if err := engine.Start(context.Background(), run); err != nil {
		t.Fatalf("Start() error = %v", err)
	}

	record, err := repo.GetRunRecord(context.Background(), run.ID)
	if err != nil {
		t.Fatalf("GetRunRecord() error = %v", err)
	}
	if record.Run.Status != assistant.RunStatusCompleted {
		t.Fatalf("Run.Status = %q, want %q", record.Run.Status, assistant.RunStatusCompleted)
	}
	if len(record.Attempts) != 4 {
		t.Fatalf("len(Attempts) = %d, want 4", len(record.Attempts))
	}
	if record.Attempts[1].Role != assistant.AttemptRoleAnswer || record.Attempts[2].Role != assistant.AttemptRoleAnswer || record.Attempts[3].Role != assistant.AttemptRoleReporter {
		t.Fatalf("attempt roles = %#v, want gate -> answer -> answer -> reporter", []assistant.AttemptRole{record.Attempts[0].Role, record.Attempts[1].Role, record.Attempts[2].Role, record.Attempts[3].Role})
	}
	if got := runtime.index[assistant.AttemptRoleAnswer]; got != 2 {
		t.Fatalf("answer runtime calls = %d, want 2", got)
	}
}

func TestRunEnginePreservesWorkflowOrderAfterGate(t *testing.T) {
	t.Parallel()

	repo, dataDir := openEngineTestRepository(t)
	now := time.Date(2026, time.March, 27, 17, 0, 0, 0, time.UTC)
	runtime := &orderedRuntime{
		steps: []orderedRuntimeStep{
			{role: assistant.AttemptRoleGate, response: PhaseResponse{Summary: "Gate routed to workflow.", Output: gateJSON("workflow", "This request needs execution.")}},
			{role: assistant.AttemptRoleProjectSelector, response: PhaseResponse{Summary: "Project selected.", Output: selectorJSON("workflow-check", "Workflow Check", "Validate workflow order.")}},
			{role: assistant.AttemptRolePlanner, response: PhaseResponse{Summary: "Planner complete.", Output: plannerJSON("Validate workflow order", []string{"Validation report"})}},
			{role: assistant.AttemptRoleContractor, response: PhaseResponse{Summary: "Contract agreed.", Output: contractJSON("agreed", []string{"Validation report"}, []string{"Workflow executes in order"}, "")}},
			{role: assistant.AttemptRoleGenerator, response: PhaseResponse{Summary: "Generator produced output."}},
			{role: assistant.AttemptRoleEvaluator, response: PhaseResponse{Summary: "Evaluator passed.", Output: evaluatorJSON(true, 95, "Workflow order is correct.", nil, "")}},
			{role: assistant.AttemptRoleReporter, response: PhaseResponse{Summary: "Delivered final report.", Output: reportJSON("Delivered final report.", "Workflow order is correct.")}},
		},
	}
	engine := NewRunEngine(repo, runtime, &capturingObserver{}, gan.New(gan.Config{MaxGenerationAttempts: 1}), newEngineTestProjectManager(t, dataDir), newEngineTestWikiService(dataDir), fakeMessenger(), fixedClock(now))

	run := assistant.NewRun("Validate workflow ordering after gate.", now, 1)
	if err := engine.Start(context.Background(), run); err != nil {
		t.Fatalf("Start() error = %v", err)
	}

	if runtime.index != len(runtime.steps) {
		t.Fatalf("runtime calls = %d, want %d", runtime.index, len(runtime.steps))
	}

	record, err := repo.GetRunRecord(context.Background(), run.ID)
	if err != nil {
		t.Fatalf("GetRunRecord() error = %v", err)
	}
	if record.Run.Status != assistant.RunStatusCompleted {
		t.Fatalf("Run.Status = %q, want %q", record.Run.Status, assistant.RunStatusCompleted)
	}
	wantRoles := []assistant.AttemptRole{
		assistant.AttemptRoleGate,
		assistant.AttemptRoleProjectSelector,
		assistant.AttemptRolePlanner,
		assistant.AttemptRoleContractor,
		assistant.AttemptRoleGenerator,
		assistant.AttemptRoleEvaluator,
		assistant.AttemptRoleReporter,
	}
	if len(record.Attempts) != len(wantRoles) {
		t.Fatalf("len(Attempts) = %d, want %d", len(record.Attempts), len(wantRoles))
	}
	for idx, want := range wantRoles {
		if got := record.Attempts[idx].Role; got != want {
			t.Fatalf("Attempts[%d].Role = %q, want %q", idx, got, want)
		}
	}
}

func TestRunEngineCreatesScheduledRunsBeforeReporting(t *testing.T) {
	t.Parallel()

	repo, dataDir := openEngineTestRepository(t)
	now := time.Date(2026, time.April, 3, 12, 0, 0, 0, time.UTC)
	runtime := &orderedRuntime{
		steps: []orderedRuntimeStep{
			{role: assistant.AttemptRoleGate, response: PhaseResponse{Summary: "Gate routed to workflow.", Output: gateJSON("workflow", "This request needs execution.")}},
			{role: assistant.AttemptRoleProjectSelector, response: PhaseResponse{Summary: "Project selected.", Output: selectorJSON("hospital-outreach", "Hospital Outreach", "Research and contact hospitals.")}},
			{role: assistant.AttemptRolePlanner, response: PhaseResponse{Summary: "Planner complete.", Output: plannerJSONWithSchedule("Research hospitals", []string{"Hospital shortlist"}, []assistant.ScheduleEntry{
				{ScheduledFor: "13:00", Prompt: "Call the first shortlisted hospital."},
				{ScheduledFor: "+90m", Prompt: "Call the second shortlisted hospital."},
			})}},
			{role: assistant.AttemptRoleContractor, response: PhaseResponse{Summary: "Contract agreed.", Output: contractJSON("agreed", []string{"Hospital shortlist"}, []string{"Identify hospitals and schedule follow-up calls"}, "")}},
			{role: assistant.AttemptRoleGenerator, response: PhaseResponse{Summary: "Generator produced output."}},
			{role: assistant.AttemptRoleEvaluator, response: PhaseResponse{Summary: "Evaluator passed.", Output: evaluatorJSON(true, 95, "Workflow is complete.", nil, "")}},
			{role: assistant.AttemptRoleScheduler, response: PhaseResponse{Summary: "Scheduler finalized follow-up calls.", Output: schedulerJSON([]assistant.ScheduleEntry{
				{ScheduledFor: "2026-04-03T13:00:00Z", Prompt: "Call the first shortlisted hospital."},
				{ScheduledFor: "2026-04-03T13:30:00Z", Prompt: "Call the second shortlisted hospital."},
			})}},
			{role: assistant.AttemptRoleReporter, response: PhaseResponse{Summary: "Delivered final report.", Output: reportJSON("Delivered final report.", "Scheduled hospital outreach.")}},
		},
	}
	engine := NewRunEngine(repo, runtime, &capturingObserver{}, gan.New(gan.Config{MaxGenerationAttempts: 1}), newEngineTestProjectManager(t, dataDir), newEngineTestWikiService(dataDir), fakeMessenger(), fixedClock(now))

	run := assistant.NewRun("Research hospitals and then call them later.", now, 1)
	if err := engine.Start(context.Background(), run); err != nil {
		t.Fatalf("Start() error = %v", err)
	}

	record, err := repo.GetRunRecord(context.Background(), run.ID)
	if err != nil {
		t.Fatalf("GetRunRecord() error = %v", err)
	}
	if len(record.ScheduledRuns) != 2 {
		t.Fatalf("len(record.ScheduledRuns) = %d, want 2", len(record.ScheduledRuns))
	}
	if record.Attempts[len(record.Attempts)-1].Role != assistant.AttemptRoleReporter {
		t.Fatalf("attempt roles = %#v, want reporter as final attempt", record.Attempts)
	}
	if record.Attempts[len(record.Attempts)-2].Role != assistant.AttemptRoleScheduler {
		t.Fatalf("attempt roles = %#v, want scheduler before reporter", record.Attempts)
	}
}

func TestEvaluateAutomationSafetyComplianceSoftForBrowserMutating(t *testing.T) {
	t.Parallel()

	policy := &assistant.AutomationSafetyPolicy{
		Profile:     assistant.AutomationSafetyProfileBrowserMutating,
		Enforcement: assistant.AutomationSafetyEnforcementEvaluatorEnforced,
		RateLimits: assistant.AutomationSafetyRateLimits{
			MaxAccountChangingActionsPerRun: 1,
		},
	}
	actions := []assistant.BrowserActionRecord{
		{ActionType: assistant.BrowserActionTypeSubmit, AccountStateChanged: true, OccurredAt: time.Date(2026, time.April, 3, 12, 0, 0, 0, time.UTC)},
		{ActionType: assistant.BrowserActionTypeReply, AccountStateChanged: true, OccurredAt: time.Date(2026, time.April, 3, 12, 1, 0, 0, time.UTC)},
	}
	metrics := &assistant.BrowserRecentActivityMetrics{ReplyActionCount: 2}

	result := evaluateAutomationSafetyCompliance(policy, actions, nil, metrics, true)
	if len(result.Findings) == 0 {
		t.Fatal("Findings = 0, want soft automation safety findings")
	}
	if len(result.HardFindings) == 0 {
		t.Fatal("HardFindings = 0, want deterministic finding details")
	}
	if result.HardBlock {
		t.Fatal("HardBlock = true, want false for browser_mutating evaluator_enforced policy")
	}
}

func TestEvaluateAutomationSafetyComplianceHardBlocksNoActionWithoutEvidence(t *testing.T) {
	t.Parallel()

	policy := &assistant.AutomationSafetyPolicy{
		Profile:     assistant.AutomationSafetyProfileBrowserHighRiskEngagement,
		Enforcement: assistant.AutomationSafetyEnforcementEngineBlocking,
		ModePolicy: assistant.AutomationSafetyModePolicy{
			AllowNoActionSuccess:     true,
			RequireNoActionEvidence:  true,
			NoActionEvidenceRequired: []string{"observed context", "skipped action", "safety reason", "safer next step"},
		},
		RateLimits: assistant.AutomationSafetyRateLimits{
			MaxAccountChangingActionsPerRun: 2,
			MaxRepliesPer24h:                12,
			MinSpacingMinutes:               20,
		},
	}

	result := evaluateAutomationSafetyCompliance(policy, nil, nil, nil, true)
	if len(result.Findings) == 0 {
		t.Fatal("Findings = 0, want missing no-action evidence finding")
	}
	if !result.HardBlock {
		t.Fatal("HardBlock = false, want true for high-risk no-action evidence violation")
	}
	if !strings.Contains(strings.ToLower(result.HardBlockSummary()), "no-action") {
		t.Fatalf("HardBlockSummary = %q, want no-action evidence detail", result.HardBlockSummary())
	}
}

func TestValidateSchedulerAutomationSafetyBlocksWhenReplyCapReached(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, time.April, 5, 10, 0, 0, 0, time.UTC)
	policy := &assistant.AutomationSafetyPolicy{
		Profile:     assistant.AutomationSafetyProfileBrowserHighRiskEngagement,
		Enforcement: assistant.AutomationSafetyEnforcementEngineBlocking,
		RateLimits: assistant.AutomationSafetyRateLimits{
			MaxRepliesPer24h: 12,
		},
	}
	metrics := &assistant.BrowserRecentActivityMetrics{
		ReplyActionCount: 12,
	}

	summary := validateSchedulerAutomationSafety(policy, metrics, now, []time.Time{now.Add(30 * time.Minute)})
	if strings.TrimSpace(summary) == "" {
		t.Fatal("summary is empty, want scheduler hard-limit block")
	}
	if !strings.Contains(strings.ToLower(summary), "max_replies_per_24h") {
		t.Fatalf("summary = %q, want reply-cap reason", summary)
	}
}
func TestRunEngineHardFailsHighRiskWhenMutationLimitExceeded(t *testing.T) {
	t.Parallel()

	repo, dataDir := openEngineTestRepository(t)
	now := time.Date(2026, time.April, 4, 9, 0, 0, 0, time.UTC)
	safetyJSON := `{"profile":"browser_high_risk_engagement","enforcement":"engine_blocking","mode_policy":{"allow_no_action_success":true,"require_no_action_evidence":true,"no_action_evidence_required":["observed context","skipped action","safety reason","safer next step"]},"rate_limits":{"max_account_changing_actions_per_run":1,"max_replies_per_24h":12,"min_spacing_minutes":20}}`
	runtime := &scriptedRuntime{
		steps: map[assistant.AttemptRole][]runtimeStep{
			assistant.AttemptRoleGate: {
				{response: PhaseResponse{Summary: "Gate routed to workflow.", Output: gateJSON("workflow", "This request needs browser execution.")}},
			},
			assistant.AttemptRoleProjectSelector: {
				{response: PhaseResponse{Summary: "Selected project.", Output: selectorJSON("community-outreach", "Community Outreach", "Engage community safely.")}},
			},
			assistant.AttemptRolePlanner: {
				{response: PhaseResponse{Summary: "Planner complete.", Output: plannerJSONWithAutomationSafety("Engage community safely", []string{"Engagement summary"}, safetyJSON, nil)}},
			},
			assistant.AttemptRoleContractor: {
				{response: PhaseResponse{Summary: "Contract agreed.", Output: contractJSON("agreed", []string{"Engagement summary"}, []string{"Respect safety limits"}, "")}},
			},
			assistant.AttemptRoleGenerator: {
				{response: PhaseResponse{Summary: "Generator attempted outreach.", WebSteps: []assistant.WebStep{
					{Title: "Sent first reply", URL: "https://example.com/thread/1", Summary: "Posted one reply.", ActionName: "reply", ActionTarget: "thread-1", ActionValue: "Thanks!", OccurredAt: now.Add(1 * time.Minute)},
					{Title: "Submitted follow-up", URL: "https://example.com/thread/2", Summary: "Posted another reply.", ActionName: "submit", ActionTarget: "thread-2", ActionValue: "Following up", OccurredAt: now.Add(2 * time.Minute)},
				}}},
			},
			assistant.AttemptRoleEvaluator: {
				{response: PhaseResponse{Summary: "Evaluator passed.", Output: evaluatorJSON(true, 92, "Nominal task looked complete.", nil, "")}},
			},
		},
	}
	engine := NewRunEngine(repo, runtime, &capturingObserver{}, gan.New(gan.Config{MaxGenerationAttempts: 2}), newEngineTestProjectManager(t, dataDir), newEngineTestWikiService(dataDir), fakeMessenger(), fixedClock(now))

	run := assistant.NewRun("Reply to two community posts.", now, 2)
	if err := engine.Start(context.Background(), run); err != nil {
		t.Fatalf("Start() error = %v", err)
	}

	record, err := repo.GetRunRecord(context.Background(), run.ID)
	if err != nil {
		t.Fatalf("GetRunRecord() error = %v", err)
	}
	if record.Run.Status != assistant.RunStatusFailed {
		t.Fatalf("Run.Status = %q, want %q", record.Run.Status, assistant.RunStatusFailed)
	}
	if len(record.Evaluations) != 1 {
		t.Fatalf("len(Evaluations) = %d, want 1", len(record.Evaluations))
	}
	if record.Evaluations[0].Passed {
		t.Fatalf("evaluation = %#v, want failed automation-safety evaluation", record.Evaluations[0])
	}
	if !strings.Contains(strings.ToLower(record.Evaluations[0].Summary), "automation safety") {
		t.Fatalf("evaluation summary = %q, want automation safety finding", record.Evaluations[0].Summary)
	}
	if len(record.ScheduledRuns) != 0 {
		t.Fatalf("len(ScheduledRuns) = %d, want 0 after hard fail", len(record.ScheduledRuns))
	}
}

func TestRunEngineHardFailsHighRiskSchedulerFixedShortFollowups(t *testing.T) {
	t.Parallel()

	repo, dataDir := openEngineTestRepository(t)
	now := time.Date(2026, time.April, 5, 11, 0, 0, 0, time.UTC)
	firstFollowUp := now.Add(5 * time.Minute).UTC().Format(time.RFC3339)
	secondFollowUp := now.Add(10 * time.Minute).UTC().Format(time.RFC3339)
	safetyJSON := `{"profile":"browser_high_risk_engagement","enforcement":"engine_blocking","rate_limits":{"max_account_changing_actions_per_run":2,"max_replies_per_24h":12,"min_spacing_minutes":20},"pattern_rules":{"disallow_fixed_short_followups":true}}`
	runtime := &orderedRuntime{
		steps: []orderedRuntimeStep{
			{role: assistant.AttemptRoleGate, response: PhaseResponse{Summary: "Gate routed to workflow.", Output: gateJSON("workflow", "Needs deferred follow-up.")}},
			{role: assistant.AttemptRoleProjectSelector, response: PhaseResponse{Summary: "Project selected.", Output: selectorJSON("community-followup", "Community Follow-Up", "Handle follow-up engagement.")}},
			{role: assistant.AttemptRolePlanner, response: PhaseResponse{Summary: "Planner complete.", Output: plannerJSONWithAutomationSafety("Plan follow-up engagement", []string{"Follow-up plan"}, safetyJSON, []assistant.ScheduleEntry{{ScheduledFor: "+30m", Prompt: "Follow up later."}})}},
			{role: assistant.AttemptRoleContractor, response: PhaseResponse{Summary: "Contract agreed.", Output: contractJSON("agreed", []string{"Follow-up plan"}, []string{"Schedule safe follow-up"}, "")}},
			{role: assistant.AttemptRoleGenerator, response: PhaseResponse{Summary: "Generator complete.", WebSteps: []assistant.WebStep{{Title: "Posted one reply", URL: "https://example.com/thread/9", Summary: "Posted one reply.", ActionName: "reply", ActionTarget: "thread-9", OccurredAt: now.Add(1 * time.Minute)}}}},
			{role: assistant.AttemptRoleEvaluator, response: PhaseResponse{Summary: "Evaluator passed.", Output: evaluatorJSON(true, 95, "Task completed.", nil, "")}},
			{role: assistant.AttemptRoleScheduler, response: PhaseResponse{Summary: "Scheduler emitted fixed short loop.", Output: schedulerJSON([]assistant.ScheduleEntry{{ScheduledFor: firstFollowUp, Prompt: "Reply again."}, {ScheduledFor: secondFollowUp, Prompt: "Reply once more."}})}},
		},
	}
	engine := NewRunEngine(repo, runtime, &capturingObserver{}, gan.New(gan.Config{MaxGenerationAttempts: 1}), newEngineTestProjectManager(t, dataDir), newEngineTestWikiService(dataDir), fakeMessenger(), fixedClock(now))

	run := assistant.NewRun("Schedule high-risk follow-up replies.", now, 1)
	if err := engine.Start(context.Background(), run); err != nil {
		t.Fatalf("Start() error = %v", err)
	}

	record, err := repo.GetRunRecord(context.Background(), run.ID)
	if err != nil {
		t.Fatalf("GetRunRecord() error = %v", err)
	}
	if record.Run.Status != assistant.RunStatusFailed {
		t.Fatalf("Run.Status = %q, want %q", record.Run.Status, assistant.RunStatusFailed)
	}
	if len(record.ScheduledRuns) != 0 {
		t.Fatalf("len(ScheduledRuns) = %d, want 0 after scheduler hard block", len(record.ScheduledRuns))
	}
	if got := record.Attempts[len(record.Attempts)-1].Role; got != assistant.AttemptRoleScheduler {
		t.Fatalf("last attempt role = %q, want %q", got, assistant.AttemptRoleScheduler)
	}
}
func TestRunEngineReportCanEnterWaiting(t *testing.T) {
	t.Parallel()

	repo, dataDir := openEngineTestRepository(t)
	now := time.Date(2026, time.March, 27, 17, 15, 0, 0, time.UTC)
	runtime := &scriptedRuntime{
		steps: map[assistant.AttemptRole][]runtimeStep{
			assistant.AttemptRoleGate: {
				{response: PhaseResponse{Summary: "Gate routed to answer.", Output: gateJSON("answer", "This is a read-only request.")}},
			},
			assistant.AttemptRoleAnswer: {
				{response: PhaseResponse{Summary: "Prepared direct answer.", Output: "Here is the answer."}},
			},
			assistant.AttemptRoleReporter: {
				{response: PhaseResponse{Summary: "Need confirmation before sending.", WaitRequest: &assistant.WaitRequest{
					Kind:   assistant.WaitKindApproval,
					Title:  "Confirm report delivery",
					Prompt: "Approve sending the final report card.",
				}}},
			},
		},
	}
	engine := NewRunEngine(repo, runtime, &capturingObserver{}, gan.New(gan.Config{MaxGenerationAttempts: 1}), newEngineTestProjectManager(t, dataDir), newEngineTestWikiService(dataDir), fakeMessenger(), fixedClock(now))

	run := assistant.NewRun("Answer a quick follow-up.", now, 1)
	if err := engine.Start(context.Background(), run); err != nil {
		t.Fatalf("Start() error = %v", err)
	}

	record, err := repo.GetRunRecord(context.Background(), run.ID)
	if err != nil {
		t.Fatalf("GetRunRecord() error = %v", err)
	}
	if record.Run.Status != assistant.RunStatusWaiting {
		t.Fatalf("Run.Status = %q, want %q", record.Run.Status, assistant.RunStatusWaiting)
	}
	if record.Run.WaitingFor == nil || record.Run.WaitingFor.Title != "Confirm report delivery" {
		t.Fatalf("WaitingFor = %#v, want report wait request", record.Run.WaitingFor)
	}
}

func TestRunEngineReportFailurePreventsCompletion(t *testing.T) {
	t.Parallel()

	repo, dataDir := openEngineTestRepository(t)
	now := time.Date(2026, time.March, 27, 17, 30, 0, 0, time.UTC)
	runtime := &scriptedRuntime{
		steps: map[assistant.AttemptRole][]runtimeStep{
			assistant.AttemptRoleGate: {
				{response: PhaseResponse{Summary: "Gate routed to answer.", Output: gateJSON("answer", "This is a read-only request.")}},
			},
			assistant.AttemptRoleAnswer: {
				{response: PhaseResponse{Summary: "Prepared direct answer.", Output: "Here is the answer."}},
			},
			assistant.AttemptRoleReporter: {
				{response: PhaseResponse{Summary: "Report failed.", Output: `{"summary":"Report failed.","delivery_status":"wait","message_preview":"","report_payload":"","needs_user_input":false,"wait_kind":"","wait_title":"","wait_prompt":"","wait_risk_summary":""}`}},
			},
		},
	}
	engine := NewRunEngine(repo, runtime, &capturingObserver{}, gan.New(gan.Config{MaxGenerationAttempts: 1}), newEngineTestProjectManager(t, dataDir), newEngineTestWikiService(dataDir), fakeMessenger(), fixedClock(now))

	run := assistant.NewRun("Answer a quick follow-up.", now, 1)
	err := engine.Start(context.Background(), run)
	if err == nil {
		t.Fatal("Start() error = nil, want report failure")
	}

	record, getErr := repo.GetRunRecord(context.Background(), run.ID)
	if getErr != nil {
		t.Fatalf("GetRunRecord() error = %v", getErr)
	}
	if record.Run.Status != assistant.RunStatusFailed {
		t.Fatalf("Run.Status = %q, want %q", record.Run.Status, assistant.RunStatusFailed)
	}
}

func TestRunEngineMarksReportingBeforeReporterSetupForAnswerRuns(t *testing.T) {
	t.Parallel()

	repo, dataDir := openEngineTestRepository(t)
	now := time.Date(2026, time.March, 27, 17, 45, 0, 0, time.UTC)
	runtime := &scriptedRuntime{
		steps: map[assistant.AttemptRole][]runtimeStep{
			assistant.AttemptRoleGate: {
				{response: PhaseResponse{Summary: "Gate routed to answer.", Output: gateJSON("answer", "This is a read-only request.")}},
			},
			assistant.AttemptRoleAnswer: {
				{response: PhaseResponse{Summary: "Prepared direct answer.", Output: "Hi! How can I help you today?"}},
			},
		},
	}
	messenger := fakeMessenger()
	messenger.withAccountErr = errors.New("agent-message config is invalid")
	engine := NewRunEngine(repo, runtime, &capturingObserver{}, gan.New(gan.Config{MaxGenerationAttempts: 1}), newEngineTestProjectManager(t, dataDir), newEngineTestWikiService(dataDir), messenger, fixedClock(now))

	run := assistant.NewRun("reply hi to me", now, 1)
	err := engine.Start(context.Background(), run)
	if !errors.Is(err, messenger.withAccountErr) {
		t.Fatalf("Start() error = %v, want %v", err, messenger.withAccountErr)
	}

	record, getErr := repo.GetRunRecord(context.Background(), run.ID)
	if getErr != nil {
		t.Fatalf("GetRunRecord() error = %v", getErr)
	}
	if record.Run.Status != assistant.RunStatusFailed {
		t.Fatalf("Run.Status = %q, want %q", record.Run.Status, assistant.RunStatusFailed)
	}
	if record.Run.Phase != assistant.RunPhaseFailed {
		t.Fatalf("Run.Phase = %q, want %q", record.Run.Phase, assistant.RunPhaseFailed)
	}
	if len(record.Attempts) != 2 {
		t.Fatalf("len(Attempts) = %d, want 2", len(record.Attempts))
	}
	if got := record.Attempts[len(record.Attempts)-1].Role; got != assistant.AttemptRoleAnswer {
		t.Fatalf("last attempt role = %q, want %q", got, assistant.AttemptRoleAnswer)
	}
	if record.Run.CompletedAt != nil {
		t.Fatalf("CompletedAt = %v, want nil for failed run", record.Run.CompletedAt)
	}
}

type scriptedRuntime struct {
	steps map[assistant.AttemptRole][]runtimeStep
	index map[assistant.AttemptRole]int
}

type runtimeStep struct {
	expectCritique    string
	expectResumeInput map[string]string
	response          PhaseResponse
	err               error
}

func (r *scriptedRuntime) Execute(_ context.Context, role assistant.AttemptRole, request PhaseRequest) (PhaseResponse, error) {
	if r.index == nil {
		r.index = make(map[assistant.AttemptRole]int)
	}
	roleSteps := r.steps[role]
	idx := r.index[role]
	if idx >= len(roleSteps) {
		return PhaseResponse{}, fmt.Errorf("unexpected runtime call for role %s", role)
	}
	step := roleSteps[idx]
	r.index[role] = idx + 1

	if step.expectCritique != "" && request.Critique != step.expectCritique {
		return PhaseResponse{}, fmt.Errorf("critique = %q, want %q", request.Critique, step.expectCritique)
	}
	if len(step.expectResumeInput) > 0 {
		for key, want := range step.expectResumeInput {
			if got := request.ResumeInput[key]; got != want {
				return PhaseResponse{}, fmt.Errorf("resume input %q = %q, want %q", key, got, want)
			}
		}
	}
	return step.response, step.err
}

func (r *scriptedRuntime) Close() error {
	return nil
}

type orderedRuntime struct {
	steps []orderedRuntimeStep
	index int
}

type orderedRuntimeStep struct {
	role     assistant.AttemptRole
	response PhaseResponse
	err      error
}

func (r *orderedRuntime) Execute(_ context.Context, role assistant.AttemptRole, _ PhaseRequest) (PhaseResponse, error) {
	if r.index >= len(r.steps) {
		return PhaseResponse{}, fmt.Errorf("unexpected runtime call for role %s", role)
	}
	step := r.steps[r.index]
	r.index++
	if step.role != role {
		return PhaseResponse{}, fmt.Errorf("runtime role at step %d = %s, want %s", r.index-1, role, step.role)
	}
	return step.response, step.err
}

func (r *orderedRuntime) Close() error {
	return nil
}

type capturingObserver struct {
	events []assistant.RunEvent
}

func (o *capturingObserver) Publish(_ context.Context, event assistant.RunEvent) error {
	o.events = append(o.events, event)
	return nil
}

func openEngineTestRepository(t *testing.T) (*store.SQLiteRepository, string) {
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
	return repo, dataDir
}

func newEngineTestProjectManager(t *testing.T, dataDir string) *project.Manager {
	t.Helper()

	manager := project.NewManager(dataDir, filepath.Join(dataDir, "projects"))
	if err := manager.EnsureBaseScaffold(); err != nil {
		t.Fatalf("EnsureBaseScaffold() error = %v", err)
	}
	return manager
}

func newEngineTestWikiService(dataDir string) *wiki.Service {
	return wiki.NewService(filepath.Join(dataDir, "projects"), time.Now)
}

func fixedClock(base time.Time) func() time.Time {
	current := base.Add(-time.Second)
	return func() time.Time {
		current = current.Add(time.Second)
		return current
	}
}

func plannerJSON(goal string, deliverables []string) string {
	return fmt.Sprintf(`{"goal":%q,"deliverables":["%s"],"constraints":[],"tools_allowed":["agent-browser"],"tools_required":["agent-browser"],"done_definition":["Produce the requested deliverables"],"evidence_required":["Capture source evidence"],"risk_flags":[],"max_generation_attempts":2}`, goal, stringsJoin(deliverables))
}

func plannerJSONWithSchedule(goal string, deliverables []string, entries []assistant.ScheduleEntry) string {
	scheduleJSON := "null"
	if len(entries) > 0 {
		parts := make([]string, 0, len(entries))
		for _, entry := range entries {
			parts = append(parts, fmt.Sprintf(`{"scheduled_for":%q,"prompt":%q}`, entry.ScheduledFor, entry.Prompt))
		}
		scheduleJSON = fmt.Sprintf(`{"entries":[%s]}`, strings.Join(parts, ","))
	}
	return fmt.Sprintf(`{"goal":%q,"deliverables":["%s"],"constraints":[],"tools_allowed":["agent-browser"],"tools_required":["agent-browser"],"done_definition":["Produce the requested deliverables"],"evidence_required":["Capture source evidence"],"risk_flags":[],"max_generation_attempts":2,"schedule_plan":%s}`, goal, stringsJoin(deliverables), scheduleJSON)
}

func plannerJSONWithAutomationSafety(goal string, deliverables []string, automationSafetyJSON string, entries []assistant.ScheduleEntry) string {
	scheduleJSON := "null"
	if len(entries) > 0 {
		parts := make([]string, 0, len(entries))
		for _, entry := range entries {
			parts = append(parts, fmt.Sprintf(`{"scheduled_for":%q,"prompt":%q}`, entry.ScheduledFor, entry.Prompt))
		}
		scheduleJSON = fmt.Sprintf(`{"entries":[%s]}`, strings.Join(parts, ","))
	}
	automationSafetyJSON = strings.TrimSpace(automationSafetyJSON)
	if automationSafetyJSON == "" {
		automationSafetyJSON = "null"
	}
	return fmt.Sprintf(`{"goal":%q,"deliverables":["%s"],"constraints":[],"tools_allowed":["agent-browser"],"tools_required":["agent-browser"],"done_definition":["Produce the requested deliverables"],"evidence_required":["Capture source evidence"],"risk_flags":["external-side-effect"],"automation_safety":%s,"max_generation_attempts":2,"schedule_plan":%s}`,
		goal,
		stringsJoin(deliverables),
		automationSafetyJSON,
		scheduleJSON,
	)
}
func contractJSON(decision string, deliverables []string, acceptanceCriteria []string, revisionNotes string) string {
	return fmt.Sprintf(`{"decision":%q,"summary":"Contract review completed.","deliverables":["%s"],"acceptance_criteria":["%s"],"evidence_required":["Capture source evidence"],"constraints":[],"out_of_scope":[],"revision_notes":%q}`, decision, stringsJoin(deliverables), stringsJoin(acceptanceCriteria), revisionNotes)
}

func selectorJSON(slug, name, description string) string {
	return fmt.Sprintf(`{"project_slug":%q,"project_name":%q,"project_description":%q,"summary":"Selected project %s."}`, slug, name, description, slug)
}

func gateJSON(route, reason string) string {
	return fmt.Sprintf(`{"route":%q,"reason":%q,"summary":"Gate routed to %s."}`, route, reason, route)
}

func evaluatorJSON(passed bool, score int, summary string, missing []string, nextAction string) string {
	missingValue := "[]"
	if len(missing) > 0 {
		missingValue = fmt.Sprintf(`["%s"]`, stringsJoin(missing))
	}
	return fmt.Sprintf(`{"passed":%t,"score":%d,"summary":%q,"missing_requirements":%s,"incorrect_claims":[],"evidence_checked":["artifacts"],"next_action_for_generator":%q}`, passed, score, summary, missingValue, nextAction)
}

func reportJSON(summary, preview string) string {
	payload := `{"root":"screen","elements":{"screen":{"type":"Text","props":{"text":"Delivered final report."},"children":[]}}}`
	return fmt.Sprintf(`{"summary":%q,"delivery_status":"sent","message_preview":%q,"report_payload":%q,"needs_user_input":false,"wait_kind":"","wait_title":"","wait_prompt":"","wait_risk_summary":""}`, summary, preview, payload)
}

func schedulerJSON(entries []assistant.ScheduleEntry) string {
	parts := make([]string, 0, len(entries))
	for _, entry := range entries {
		parts = append(parts, fmt.Sprintf(`{"scheduled_for":%q,"prompt":%q}`, entry.ScheduledFor, entry.Prompt))
	}
	return fmt.Sprintf(`{"entries":[%s]}`, strings.Join(parts, ","))
}

func stringsJoin(values []string) string {
	switch len(values) {
	case 0:
		return ""
	case 1:
		return values[0]
	default:
		return values[0] + `","` + values[1]
	}
}

type fakeAgentMessageService struct {
	account        agentmessage.ChatAccount
	withAccountErr error
}

func fakeMessenger() *fakeAgentMessageService {
	return &fakeAgentMessageService{
		account: agentmessage.ChatAccount{
			Name:   "cva-chat_test",
			Master: "supervisor",
		},
	}
}

func (f *fakeAgentMessageService) WithChatAccount(_ context.Context, chatID string, fn func(agentmessage.ChatAccount) error) error {
	if f.withAccountErr != nil {
		return f.withAccountErr
	}
	account := f.account
	account.ChatID = chatID
	return fn(account)
}

func (f *fakeAgentMessageService) CatalogPrompt(context.Context, string) (string, error) {
	return "catalog prompt", nil
}

func (f *fakeAgentMessageService) SendJSONRender(context.Context, string, string) error {
	return nil
}

func (f *fakeAgentMessageService) ReadReplies(context.Context, string) ([]agentmessage.IncomingMessage, error) {
	return nil, nil
}

func (f *fakeAgentMessageService) ReactToMessage(context.Context, string, string, string) error {
	return nil
}

func artifactsContain(artifacts []assistant.Artifact, want string) bool {
	for _, artifact := range artifacts {
		if strings.Contains(artifact.Content, want) {
			return true
		}
	}
	return false
}
