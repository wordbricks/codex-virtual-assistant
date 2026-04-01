package assistant

import (
	"strings"
	"testing"
	"time"
)

func TestNewRunInitializesWTLDefaults(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, time.March, 27, 9, 30, 0, 0, time.UTC)
	run := NewRun("Research 5 competitor pricing pages", now, 4)

	if !strings.HasPrefix(run.ID, "run_") {
		t.Fatalf("run ID = %q, want prefix run_", run.ID)
	}
	if run.ChatID != run.ID {
		t.Fatalf("ChatID = %q, want %q", run.ChatID, run.ID)
	}
	if run.Status != RunStatusQueued {
		t.Fatalf("Status = %q, want %q", run.Status, RunStatusQueued)
	}
	if run.Phase != RunPhaseQueued {
		t.Fatalf("Phase = %q, want %q", run.Phase, RunPhaseQueued)
	}
	if run.TaskSpec.UserRequestRaw != run.UserRequestRaw {
		t.Fatalf("TaskSpec.UserRequestRaw = %q, want %q", run.TaskSpec.UserRequestRaw, run.UserRequestRaw)
	}
	if run.MaxGenerationAttempts != 4 {
		t.Fatalf("MaxGenerationAttempts = %d, want 4", run.MaxGenerationAttempts)
	}
	if err := run.Validate(); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
}

func TestEvaluationValidateRejectsInvalidScore(t *testing.T) {
	t.Parallel()

	evaluation := Evaluation{
		RunID:     "run_123",
		AttemptID: "attempt_123",
		Score:     101,
		Summary:   "Missing final spreadsheet output.",
		CreatedAt: time.Now().UTC(),
	}

	if err := evaluation.Validate(); err == nil {
		t.Fatal("Validate() error = nil, want invalid score error")
	}
}

func TestRunValidateRejectsInvalidGateMetadata(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, time.March, 31, 9, 30, 0, 0, time.UTC)

	run := NewRun("Answer a quick follow-up question using previous evidence.", now, 2)
	run.GateRoute = RunRoute("invalid")
	if err := run.Validate(); err == nil {
		t.Fatal("Validate() error = nil, want invalid gate route error")
	}

	run = NewRun("Answer a quick follow-up question using previous evidence.", now, 2)
	run.GateReason = "Should be answer-only."
	if err := run.Validate(); err == nil {
		t.Fatal("Validate() error = nil, want gate metadata requires route error")
	}
}
