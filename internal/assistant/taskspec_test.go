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
