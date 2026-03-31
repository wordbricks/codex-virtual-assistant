package store

import (
	"context"
	"errors"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/siisee11/CodexVirtualAssistant/internal/assistant"
	"github.com/siisee11/CodexVirtualAssistant/internal/config"
)

func TestSQLiteRepositoryRoundTripsRunRecord(t *testing.T) {
	t.Parallel()

	repo := openTestRepository(t)
	ctx := context.Background()
	now := time.Date(2026, time.March, 27, 12, 0, 0, 0, time.UTC)
	finishedAt := now.Add(2 * time.Minute)
	completedAt := now.Add(10 * time.Minute)
	gateDecidedAt := now.Add(15 * time.Second)

	run := assistant.NewRun("Research five competitor pricing pages and summarize the findings.", now, 3)
	run.ParentRunID = "run_parent_seed"
	run.Status = assistant.RunStatusWaiting
	run.Phase = assistant.RunPhaseWaiting
	run.GateRoute = assistant.RunRouteWorkflow
	run.GateReason = "Requires multi-step execution and fresh evidence gathering."
	run.GateDecidedAt = &gateDecidedAt
	run.UpdatedAt = now.Add(time.Minute)
	run.CompletedAt = &completedAt
	run.TaskSpec = assistant.NewDefaultTaskSpec(run.UserRequestRaw, 3)
	if err := repo.SaveRun(ctx, run); err != nil {
		t.Fatalf("SaveRun() error = %v", err)
	}

	event := assistant.RunEvent{
		ID:        assistant.NewID("event", now),
		RunID:     run.ID,
		Type:      assistant.EventTypePhaseChanged,
		Phase:     assistant.RunPhasePlanning,
		Summary:   "Planner started normalizing the request.",
		CreatedAt: now.Add(10 * time.Second),
	}
	if err := repo.AddRunEvent(ctx, event); err != nil {
		t.Fatalf("AddRunEvent() error = %v", err)
	}

	attempt := assistant.Attempt{
		ID:            assistant.NewID("attempt", now),
		RunID:         run.ID,
		Sequence:      1,
		Role:          assistant.AttemptRolePlanner,
		InputSummary:  "User request and default planner constraints.",
		OutputSummary: "Generated normalized TaskSpec JSON.",
		Critique:      "",
		StartedAt:     now.Add(20 * time.Second),
		FinishedAt:    &finishedAt,
	}
	if err := repo.AddAttempt(ctx, attempt); err != nil {
		t.Fatalf("AddAttempt() error = %v", err)
	}

	run.AttemptCount = 1
	if err := repo.SaveRun(ctx, run); err != nil {
		t.Fatalf("SaveRun() after attempt error = %v", err)
	}

	artifact := assistant.Artifact{
		ID:        assistant.NewID("artifact", now),
		RunID:     run.ID,
		AttemptID: attempt.ID,
		Kind:      assistant.ArtifactKindTable,
		Title:     "Competitor pricing comparison",
		MIMEType:  "text/markdown",
		Content:   "| Vendor | Entry Price |\n| --- | --- |\n| A | $49 |",
		CreatedAt: now.Add(30 * time.Second),
	}
	if err := repo.AddArtifact(ctx, artifact); err != nil {
		t.Fatalf("AddArtifact() error = %v", err)
	}

	evidence := assistant.Evidence{
		ID:        assistant.NewID("evidence", now),
		RunID:     run.ID,
		AttemptID: attempt.ID,
		Kind:      assistant.EvidenceKindObservation,
		Summary:   "Observed the vendor pricing card on the competitor site.",
		Detail:    "Pricing card showed the starter plan price and billing cadence.",
		CreatedAt: now.Add(35 * time.Second),
	}
	if err := repo.AddEvidence(ctx, evidence); err != nil {
		t.Fatalf("AddEvidence() error = %v", err)
	}

	evaluation := assistant.Evaluation{
		ID:                     assistant.NewID("evaluation", now),
		RunID:                  run.ID,
		AttemptID:              attempt.ID,
		Passed:                 false,
		Score:                  72,
		Summary:                "The pricing table exists, but source evidence is incomplete.",
		MissingRequirements:    []string{"Include direct source URLs for each competitor."},
		IncorrectClaims:        []string{},
		EvidenceChecked:        []string{"Visited vendor pages", "Draft markdown table"},
		NextActionForGenerator: "Revisit each pricing page and attach source URLs.",
		CreatedAt:              now.Add(40 * time.Second),
	}
	if err := repo.AddEvaluation(ctx, evaluation); err != nil {
		t.Fatalf("AddEvaluation() error = %v", err)
	}

	toolCall := assistant.ToolCall{
		ID:            assistant.NewID("tool", now),
		RunID:         run.ID,
		AttemptID:     attempt.ID,
		ToolName:      "agent-browser snapshot",
		InputSummary:  "Snapshot competitor pricing page",
		OutputSummary: "Collected pricing cards and page title",
		StartedAt:     now.Add(50 * time.Second),
		FinishedAt:    now.Add(55 * time.Second),
	}
	if err := repo.AddToolCall(ctx, toolCall); err != nil {
		t.Fatalf("AddToolCall() error = %v", err)
	}

	webStep := assistant.WebStep{
		ID:         assistant.NewID("webstep", now),
		RunID:      run.ID,
		AttemptID:  attempt.ID,
		Title:      "Viewed competitor pricing page",
		URL:        "https://example.com/pricing",
		Summary:    "The pricing page displayed entry plan details and CTA buttons.",
		OccurredAt: now.Add(58 * time.Second),
	}
	if err := repo.AddWebStep(ctx, webStep); err != nil {
		t.Fatalf("AddWebStep() error = %v", err)
	}

	waitRequest := assistant.WaitRequest{
		ID:          assistant.NewID("wait", now),
		RunID:       run.ID,
		Kind:        assistant.WaitKindClarification,
		Title:       "Need clarification on target competitors",
		Prompt:      "Should the comparison include only direct SaaS competitors?",
		RiskSummary: "Proceeding without clarification could compare the wrong companies.",
		CreatedAt:   now.Add(time.Minute),
	}
	if err := repo.AddWaitRequest(ctx, waitRequest); err != nil {
		t.Fatalf("AddWaitRequest() error = %v", err)
	}

	record, err := repo.GetRunRecord(ctx, run.ID)
	if err != nil {
		t.Fatalf("GetRunRecord() error = %v", err)
	}

	if record.Run.ID != run.ID {
		t.Fatalf("Run.ID = %q, want %q", record.Run.ID, run.ID)
	}
	if record.Run.ParentRunID != run.ParentRunID {
		t.Fatalf("ParentRunID = %q, want %q", record.Run.ParentRunID, run.ParentRunID)
	}
	if record.Run.GateRoute != run.GateRoute {
		t.Fatalf("GateRoute = %q, want %q", record.Run.GateRoute, run.GateRoute)
	}
	if record.Run.GateReason != run.GateReason {
		t.Fatalf("GateReason = %q, want %q", record.Run.GateReason, run.GateReason)
	}
	if record.Run.GateDecidedAt == nil || !record.Run.GateDecidedAt.Equal(gateDecidedAt) {
		t.Fatalf("GateDecidedAt = %#v, want %s", record.Run.GateDecidedAt, gateDecidedAt.Format(time.RFC3339Nano))
	}
	if record.Run.TaskSpec.Goal == "" {
		t.Fatal("TaskSpec.Goal is empty after hydration")
	}
	if record.Run.AttemptCount != 1 {
		t.Fatalf("AttemptCount = %d, want 1", record.Run.AttemptCount)
	}
	if record.Run.LatestEvaluation == nil || record.Run.LatestEvaluation.ID != evaluation.ID {
		t.Fatalf("LatestEvaluation = %#v, want %q", record.Run.LatestEvaluation, evaluation.ID)
	}
	if record.Run.WaitingFor == nil || record.Run.WaitingFor.ID != waitRequest.ID {
		t.Fatalf("WaitingFor = %#v, want %q", record.Run.WaitingFor, waitRequest.ID)
	}
	if len(record.Events) != 1 || len(record.Attempts) != 1 || len(record.Artifacts) != 1 || len(record.Evidence) != 1 || len(record.Evaluations) != 1 || len(record.ToolCalls) != 1 || len(record.WebSteps) != 1 || len(record.WaitRequests) != 1 {
		t.Fatalf("unexpected record counts: %#v", record)
	}
}

func TestSQLiteRepositoryGetRunReturnsNotFound(t *testing.T) {
	t.Parallel()

	repo := openTestRepository(t)
	_, err := repo.GetRun(context.Background(), "missing")
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("GetRun() error = %v, want ErrNotFound", err)
	}
}

func TestSQLiteRepositoryMigratesLegacyRunsTableForGateAndFollowupFields(t *testing.T) {
	t.Parallel()

	sqlitePath, err := exec.LookPath("sqlite3")
	if err != nil {
		t.Fatalf("find sqlite3: %v", err)
	}

	dataDir := t.TempDir()
	dbPath := filepath.Join(dataDir, "legacy.db")
	legacySchema := `
CREATE TABLE runs (
	id TEXT PRIMARY KEY,
	status TEXT NOT NULL,
	phase TEXT NOT NULL,
	user_request_raw TEXT NOT NULL,
	task_spec_json TEXT NOT NULL,
	attempt_count INTEGER NOT NULL DEFAULT 0,
	max_generation_attempts INTEGER NOT NULL,
	created_at TEXT NOT NULL,
	updated_at TEXT NOT NULL,
	completed_at TEXT
);
`
	cmd := exec.Command(sqlitePath, "-batch", dbPath)
	cmd.Stdin = strings.NewReader(legacySchema)
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("create legacy schema: %v: %s", err, string(output))
	}

	repo, err := OpenSQLitePath(dbPath)
	if err != nil {
		t.Fatalf("OpenSQLitePath() migration error = %v", err)
	}

	ctx := context.Background()
	for _, column := range []string{"project_json", "parent_run_id", "gate_route", "gate_reason", "gate_decided_at"} {
		exists, err := repo.tableColumnExists(ctx, "runs", column)
		if err != nil {
			t.Fatalf("tableColumnExists(%s) error = %v", column, err)
		}
		if !exists {
			t.Fatalf("column %s missing after migration", column)
		}
	}
}

func openTestRepository(t *testing.T) *SQLiteRepository {
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

	repo, err := OpenSQLite(cfg)
	if err != nil {
		t.Fatalf("OpenSQLite() error = %v", err)
	}
	return repo
}
