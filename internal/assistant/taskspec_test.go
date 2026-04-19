package assistant

import (
	"slices"
	"testing"
)

func TestNormalizeTaskSpecAppliesDefaultsAndRiskFlags(t *testing.T) {
	t.Parallel()

	spec, err := NormalizeTaskSpec(TaskSpecDraft{
		UserRequestRaw: "Log in to HubSpot and update the lead status, then send the result summary.",
		Deliverables:   []string{"Lead status update confirmation", "Lead status update confirmation", "Summary message"},
	}, "", 4)
	if err != nil {
		t.Fatalf("NormalizeTaskSpec() error = %v", err)
	}

	if spec.Goal != "Log in to HubSpot and update the lead status, then send the result summary." {
		t.Fatalf("Goal = %q", spec.Goal)
	}
	if len(spec.Deliverables) != 2 {
		t.Fatalf("Deliverables = %#v, want deduplicated values", spec.Deliverables)
	}
	if !slices.Contains(spec.ToolsAllowed, "agent-browser") {
		t.Fatalf("ToolsAllowed = %#v, want agent-browser default", spec.ToolsAllowed)
	}
	if !slices.Contains(spec.RiskFlags, "authentication-required") {
		t.Fatalf("RiskFlags = %#v, want authentication-required", spec.RiskFlags)
	}
	if !slices.Contains(spec.RiskFlags, "external-side-effect") {
		t.Fatalf("RiskFlags = %#v, want external-side-effect", spec.RiskFlags)
	}
	if len(spec.DoneDefinition) == 0 || len(spec.EvidenceRequired) == 0 {
		t.Fatal("expected default done definition and evidence requirements")
	}
}

func TestNewDefaultTaskSpecProducesValidSpec(t *testing.T) {
	t.Parallel()

	spec := NewDefaultTaskSpec("Research five competitor pricing pages and summarize the findings.", 3)
	if err := spec.Validate(); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
}

func TestNormalizeTaskSpecAppliesHighRiskAutomationSafetyDefaults(t *testing.T) {
	t.Parallel()

	spec, err := NormalizeTaskSpec(TaskSpecDraft{
		UserRequestRaw: "Reply to new comments with concise follow-ups.",
		AutomationSafety: &AutomationSafetyPolicy{
			Profile: AutomationSafetyProfileBrowserHighRiskEngagement,
		},
	}, "", 3)
	if err != nil {
		t.Fatalf("NormalizeTaskSpec() error = %v", err)
	}

	if spec.AutomationSafety == nil {
		t.Fatal("AutomationSafety = nil, want normalized policy")
	}
	if spec.AutomationSafety.Enforcement != AutomationSafetyEnforcementEngineBlocking {
		t.Fatalf("Enforcement = %q, want %q", spec.AutomationSafety.Enforcement, AutomationSafetyEnforcementEngineBlocking)
	}
	if spec.AutomationSafety.RateLimits.MaxAccountChangingActionsPerRun != 2 {
		t.Fatalf("MaxAccountChangingActionsPerRun = %d, want 2", spec.AutomationSafety.RateLimits.MaxAccountChangingActionsPerRun)
	}
	if spec.AutomationSafety.RateLimits.MaxRepliesPer24h != 12 {
		t.Fatalf("MaxRepliesPer24h = %d, want 12", spec.AutomationSafety.RateLimits.MaxRepliesPer24h)
	}
	if spec.AutomationSafety.RateLimits.MinSpacingMinutes != 20 {
		t.Fatalf("MinSpacingMinutes = %d, want 20", spec.AutomationSafety.RateLimits.MinSpacingMinutes)
	}
	if !spec.AutomationSafety.ModePolicy.AllowNoActionSuccess {
		t.Fatal("AllowNoActionSuccess = false, want true")
	}
	if !spec.AutomationSafety.ModePolicy.RequireNoActionEvidence {
		t.Fatal("RequireNoActionEvidence = false, want true")
	}
	if len(spec.AutomationSafety.ModePolicy.NoActionEvidenceRequired) == 0 {
		t.Fatal("NoActionEvidenceRequired is empty, want default evidence requirements")
	}
}
