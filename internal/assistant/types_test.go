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
	if !strings.HasPrefix(run.ChatID, "chat_") {
		t.Fatalf("ChatID = %q, want prefix chat_", run.ChatID)
	}
	if run.ChatID == run.ID {
		t.Fatalf("ChatID = %q, want a distinct short chat id", run.ChatID)
	}
	if len(run.ChatID) >= len(run.ID) {
		t.Fatalf("ChatID length = %d, want shorter than run ID length %d", len(run.ChatID), len(run.ID))
	}
	if len(run.ChatID) > 28 {
		t.Fatalf("ChatID length = %d, want <= 28 so cva-<chatId> fits CLI username limits", len(run.ChatID))
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

func TestLifecycleEnumsIncludeReporting(t *testing.T) {
	t.Parallel()

	if !containsRunStatus(AllRunStatuses(), RunStatusReporting) {
		t.Fatalf("AllRunStatuses() = %#v, want reporting", AllRunStatuses())
	}
	if !containsRunStatus(AllRunStatuses(), RunStatusScheduling) {
		t.Fatalf("AllRunStatuses() = %#v, want scheduling", AllRunStatuses())
	}
	if !containsRunPhase(AllRunPhases(), RunPhaseReporting) {
		t.Fatalf("AllRunPhases() = %#v, want reporting", AllRunPhases())
	}
	if !containsRunPhase(AllRunPhases(), RunPhaseScheduling) {
		t.Fatalf("AllRunPhases() = %#v, want scheduling", AllRunPhases())
	}
}

func TestParseScheduledForSupportsRelativeAndClockTimes(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, time.April, 3, 12, 30, 0, 0, time.UTC)

	relative, err := ParseScheduledFor("+30m", now)
	if err != nil {
		t.Fatalf("ParseScheduledFor(+30m) error = %v", err)
	}
	if want := now.Add(30 * time.Minute); !relative.Equal(want) {
		t.Fatalf("relative = %s, want %s", relative, want)
	}

	clock, err := ParseScheduledFor("13:00", now)
	if err != nil {
		t.Fatalf("ParseScheduledFor(13:00) error = %v", err)
	}
	if want := time.Date(2026, time.April, 3, 13, 0, 0, 0, time.UTC); !clock.Equal(want) {
		t.Fatalf("clock = %s, want %s", clock, want)
	}

	nextDay, err := ParseScheduledFor("11:00", now)
	if err != nil {
		t.Fatalf("ParseScheduledFor(11:00) error = %v", err)
	}
	if want := time.Date(2026, time.April, 4, 11, 0, 0, 0, time.UTC); !nextDay.Equal(want) {
		t.Fatalf("nextDay = %s, want %s", nextDay, want)
	}
}

func TestNextCronOccurrenceSupportsDailyMidnight(t *testing.T) {
	t.Parallel()

	loc := time.FixedZone("PDT", -7*60*60)
	now := time.Date(2026, time.April, 13, 10, 15, 0, 0, loc)

	next, err := NextCronOccurrence("0 0 * * *", now)
	if err != nil {
		t.Fatalf("NextCronOccurrence() error = %v", err)
	}

	want := time.Date(2026, time.April, 14, 7, 0, 0, 0, time.UTC)
	if !next.Equal(want) {
		t.Fatalf("next = %s, want %s", next, want)
	}
}

func TestScheduledRunValidateRequiresTriggeredRunID(t *testing.T) {
	t.Parallel()

	run := ScheduledRun{
		ID:                    "scheduled_123",
		ChatID:                "chat_123",
		ParentRunID:           "run_parent",
		UserRequestRaw:        "Call the first hospital.",
		MaxGenerationAttempts: 2,
		ScheduledFor:          time.Date(2026, time.April, 3, 13, 0, 0, 0, time.UTC),
		Status:                ScheduledRunStatusTriggered,
		CreatedAt:             time.Date(2026, time.April, 3, 12, 0, 0, 0, time.UTC),
	}

	if err := run.Validate(); err == nil {
		t.Fatal("Validate() error = nil, want triggered run id validation")
	}
}

func containsRunStatus(values []RunStatus, want RunStatus) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func containsRunPhase(values []RunPhase, want RunPhase) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func TestTaskSpecValidateRejectsInvalidAutomationSafetyPolicy(t *testing.T) {
	t.Parallel()

	spec := NewDefaultTaskSpec("Collect QA screenshots for release notes.", 2)
	spec.AutomationSafety = &AutomationSafetyPolicy{
		Profile:     AutomationSafetyProfileBrowserReadOnly,
		Enforcement: AutomationSafetyEnforcementEngineBlocking,
	}

	if err := spec.Validate(); err == nil {
		t.Fatal("Validate() error = nil, want automation safety enforcement/profile validation error")
	}
}

func TestTaskSpecValidateRejectsHighRiskPolicyMissingLimits(t *testing.T) {
	t.Parallel()

	spec := NewDefaultTaskSpec("Respond to inbound forum replies.", 2)
	spec.AutomationSafety = &AutomationSafetyPolicy{
		Profile:     AutomationSafetyProfileBrowserHighRiskEngagement,
		Enforcement: AutomationSafetyEnforcementEngineBlocking,
		ModePolicy: AutomationSafetyModePolicy{
			AllowNoActionSuccess:    true,
			RequireNoActionEvidence: true,
			NoActionEvidenceRequired: []string{
				"Observed elevated risk signals.",
			},
		},
	}

	if err := spec.Validate(); err == nil {
		t.Fatal("Validate() error = nil, want high-risk rate limit validation error")
	}
}

func TestTaskSpecValidateAcceptsValidAutomationSafetyPolicy(t *testing.T) {
	t.Parallel()

	spec := NewDefaultTaskSpec("Handle one inbound marketplace response.", 2)
	spec.AutomationSafety = &AutomationSafetyPolicy{
		Profile:     AutomationSafetyProfileBrowserHighRiskEngagement,
		Enforcement: AutomationSafetyEnforcementEngineBlocking,
		ModePolicy: AutomationSafetyModePolicy{
			AllowedSessionModes:      []string{"read_only", "single_action"},
			AllowNoActionSuccess:     true,
			RequireNoActionEvidence:  true,
			NoActionEvidenceRequired: []string{"Observed risk and deferred mutation."},
		},
		RateLimits: AutomationSafetyRateLimits{
			MaxAccountChangingActionsPerRun: 2,
			MaxRepliesPer24h:                12,
			MinSpacingMinutes:               20,
		},
	}

	if err := spec.Validate(); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
}
