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
	if record.Run.ChatID != run.ChatID {
		t.Fatalf("ChatID = %q, want %q", record.Run.ChatID, run.ChatID)
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

func TestSQLiteRepositoryScheduledRunsRoundTrip(t *testing.T) {
	t.Parallel()

	repo := openTestRepository(t)
	ctx := context.Background()
	now := time.Date(2026, time.April, 3, 12, 0, 0, 0, time.UTC)

	parent := assistant.NewRun("Research hospitals and schedule follow-up calls.", now, 3)
	if err := repo.SaveRun(ctx, parent); err != nil {
		t.Fatalf("SaveRun(parent) error = %v", err)
	}

	first := assistant.ScheduledRun{
		ID:                    "scheduled_one",
		ChatID:                parent.ChatID,
		ParentRunID:           parent.ID,
		UserRequestRaw:        "Call hospital A.",
		MaxGenerationAttempts: 2,
		ScheduledFor:          now.Add(30 * time.Minute),
		Status:                assistant.ScheduledRunStatusPending,
		CreatedAt:             now,
	}
	second := assistant.ScheduledRun{
		ID:                    "scheduled_two",
		ChatID:                parent.ChatID,
		ParentRunID:           parent.ID,
		UserRequestRaw:        "Call hospital B.",
		MaxGenerationAttempts: 2,
		ScheduledFor:          now.Add(90 * time.Minute),
		Status:                assistant.ScheduledRunStatusPending,
		CreatedAt:             now.Add(time.Minute),
	}

	if err := repo.SaveScheduledRun(ctx, first); err != nil {
		t.Fatalf("SaveScheduledRun(first) error = %v", err)
	}
	if err := repo.SaveScheduledRun(ctx, second); err != nil {
		t.Fatalf("SaveScheduledRun(second) error = %v", err)
	}

	pending, err := repo.ListPendingScheduledRuns(ctx, now.Add(time.Hour))
	if err != nil {
		t.Fatalf("ListPendingScheduledRuns() error = %v", err)
	}
	if len(pending) != 1 || pending[0].ID != first.ID {
		t.Fatalf("pending = %#v, want only first scheduled run", pending)
	}

	childRun := assistant.NewRun("Call hospital A.", now.Add(31*time.Minute), 2)
	childRun.ParentRunID = parent.ID
	childRun.ChatID = parent.ChatID
	if err := repo.SaveRun(ctx, childRun); err != nil {
		t.Fatalf("SaveRun(child) error = %v", err)
	}

	triggeredAt := now.Add(31 * time.Minute)
	if err := repo.UpdateScheduledRunTriggered(ctx, first.ID, childRun.ID, triggeredAt); err != nil {
		t.Fatalf("UpdateScheduledRunTriggered() error = %v", err)
	}
	if err := repo.UpdateScheduledRunStatus(ctx, second.ID, assistant.ScheduledRunStatusCancelled, ""); err != nil {
		t.Fatalf("UpdateScheduledRunStatus() error = %v", err)
	}

	record, err := repo.GetRunRecord(ctx, parent.ID)
	if err != nil {
		t.Fatalf("GetRunRecord() error = %v", err)
	}
	if len(record.ScheduledRuns) != 2 {
		t.Fatalf("len(record.ScheduledRuns) = %d, want 2", len(record.ScheduledRuns))
	}

	updatedFirst, err := repo.GetScheduledRun(ctx, first.ID)
	if err != nil {
		t.Fatalf("GetScheduledRun(first) error = %v", err)
	}
	if updatedFirst.Status != assistant.ScheduledRunStatusTriggered || updatedFirst.RunID != childRun.ID {
		t.Fatalf("updatedFirst = %#v, want triggered %s", updatedFirst, childRun.ID)
	}
	if updatedFirst.TriggeredAt == nil || !updatedFirst.TriggeredAt.Equal(triggeredAt) {
		t.Fatalf("TriggeredAt = %#v, want %s", updatedFirst.TriggeredAt, triggeredAt)
	}

	byChat, err := repo.ListScheduledRunsByChat(ctx, parent.ChatID)
	if err != nil {
		t.Fatalf("ListScheduledRunsByChat() error = %v", err)
	}
	if len(byChat) != 2 {
		t.Fatalf("len(byChat) = %d, want 2", len(byChat))
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
	for _, column := range []string{"chat_id", "project_json", "parent_run_id", "gate_route", "gate_reason", "gate_decided_at"} {
		exists, err := repo.tableColumnExists(ctx, "runs", column)
		if err != nil {
			t.Fatalf("tableColumnExists(%s) error = %v", column, err)
		}
		if !exists {
			t.Fatalf("column %s missing after migration", column)
		}
	}
}

func TestSQLiteRepositoryListsChatsAndExpandsChatRecord(t *testing.T) {
	t.Parallel()

	repo := openTestRepository(t)
	ctx := context.Background()
	now := time.Date(2026, time.April, 1, 8, 0, 0, 0, time.UTC)

	root := assistant.NewRun("Initial request", now, 2)
	root.Status = assistant.RunStatusCompleted
	root.Phase = assistant.RunPhaseCompleted
	root.CompletedAt = ptrTimeStore(now.Add(2 * time.Minute))
	root.UpdatedAt = now.Add(2 * time.Minute)
	if err := repo.SaveRun(ctx, root); err != nil {
		t.Fatalf("SaveRun(root) error = %v", err)
	}

	followUp := assistant.NewRun("Follow-up request", now.Add(3*time.Minute), 2)
	followUp.ChatID = root.ChatID
	followUp.ParentRunID = root.ID
	followUp.Status = assistant.RunStatusCompleted
	followUp.Phase = assistant.RunPhaseCompleted
	followUp.CompletedAt = ptrTimeStore(now.Add(5 * time.Minute))
	followUp.UpdatedAt = now.Add(5 * time.Minute)
	if err := repo.SaveRun(ctx, followUp); err != nil {
		t.Fatalf("SaveRun(followUp) error = %v", err)
	}

	chats, err := repo.ListChats(ctx)
	if err != nil {
		t.Fatalf("ListChats() error = %v", err)
	}
	if len(chats) != 1 {
		t.Fatalf("len(chats) = %d, want 1", len(chats))
	}
	if chats[0].ID != root.ChatID {
		t.Fatalf("chat ID = %q, want %q", chats[0].ID, root.ChatID)
	}
	if chats[0].RootRunID != root.ID {
		t.Fatalf("RootRunID = %q, want %q", chats[0].RootRunID, root.ID)
	}
	if chats[0].LatestRunID != followUp.ID {
		t.Fatalf("LatestRunID = %q, want %q", chats[0].LatestRunID, followUp.ID)
	}

	record, err := repo.GetChatRecord(ctx, root.ChatID)
	if err != nil {
		t.Fatalf("GetChatRecord() error = %v", err)
	}
	if record.Chat.ID != root.ChatID {
		t.Fatalf("record.Chat.ID = %q, want %q", record.Chat.ID, root.ChatID)
	}
	if len(record.Runs) != 2 {
		t.Fatalf("len(record.Runs) = %d, want 2", len(record.Runs))
	}
	if record.Runs[0].Run.ID != root.ID || record.Runs[1].Run.ID != followUp.ID {
		t.Fatalf("chat runs = %#v, want root then follow-up", record.Runs)
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

func ptrTimeStore(v time.Time) *time.Time {
	return &v
}
