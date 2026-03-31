package wtl

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/siisee11/CodexVirtualAssistant/internal/assistant"
	"github.com/siisee11/CodexVirtualAssistant/internal/config"
	"github.com/siisee11/CodexVirtualAssistant/internal/policy/gan"
	"github.com/siisee11/CodexVirtualAssistant/internal/project"
	"github.com/siisee11/CodexVirtualAssistant/internal/store"
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
		},
	}
	observer := &capturingObserver{}
	projectManager := newEngineTestProjectManager(t, dataDir)
	engine := NewRunEngine(repo, runtime, observer, gan.New(gan.Config{MaxGenerationAttempts: 2}), projectManager, fixedClock(now))

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
	if len(record.Attempts) != 8 {
		t.Fatalf("len(Attempts) = %d, want 8", len(record.Attempts))
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
	if len(record.Artifacts) != 2 {
		t.Fatalf("len(Artifacts) = %d, want 2", len(record.Artifacts))
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
		},
	}
	engine := NewRunEngine(repo, runtime, &capturingObserver{}, gan.New(gan.Config{MaxGenerationAttempts: 2}), newEngineTestProjectManager(t, dataDir), fixedClock(now))

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
	engine := NewRunEngine(repo, runtime, &capturingObserver{}, gan.New(gan.Config{MaxGenerationAttempts: 1}), newEngineTestProjectManager(t, dataDir), fixedClock(now))

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
		},
	}
	engine := NewRunEngine(repo, runtime, &capturingObserver{}, gan.New(gan.Config{MaxGenerationAttempts: 1}), newEngineTestProjectManager(t, dataDir), fixedClock(now))

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
	if len(record.Attempts) != 2 {
		t.Fatalf("len(Attempts) = %d, want 2", len(record.Attempts))
	}
	if record.Attempts[0].Role != assistant.AttemptRoleGate || record.Attempts[1].Role != assistant.AttemptRoleAnswer {
		t.Fatalf("attempt roles = %#v, want gate -> answer", []assistant.AttemptRole{record.Attempts[0].Role, record.Attempts[1].Role})
	}
	if len(record.Artifacts) == 0 || !strings.Contains(record.Artifacts[len(record.Artifacts)-1].Content, "Top 3 cheapest competitors") {
		t.Fatalf("Artifacts = %#v, want persisted answer artifact", record.Artifacts)
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
