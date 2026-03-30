package prompting

import (
	"strings"
	"testing"
	"time"

	"github.com/siisee11/CodexVirtualAssistant/internal/assistant"
)

func TestBuildPlannerPromptDeclaresStrictJSONContract(t *testing.T) {
	t.Parallel()

	bundle := BuildPlannerPrompt(PlannerInput{
		UserRequestRaw:        "Research five competitors and build a comparison table.",
		MaxGenerationAttempts: 4,
	})

	if !strings.Contains(bundle.System, "strict JSON object") {
		t.Fatalf("System prompt = %q, want strict JSON instruction", bundle.System)
	}
	if !strings.Contains(bundle.System, "tools_allowed") || !strings.Contains(bundle.System, "done_definition") {
		t.Fatalf("System prompt = %q, want required planner keys", bundle.System)
	}
	if !strings.Contains(bundle.User, "Default max generation attempts: 4") {
		t.Fatalf("User prompt = %q, want attempt count", bundle.User)
	}
}

func TestBuildProjectSelectorPromptRequiresProjectInspection(t *testing.T) {
	t.Parallel()

	bundle := BuildProjectSelectorPrompt(ProjectSelectorInput{
		UserRequestRaw: "Estimate our company's infrastructure costs next quarter.",
	})

	if !strings.Contains(bundle.System, "projects/*/PROJECT.md") {
		t.Fatalf("System prompt = %q, want PROJECT.md inspection guidance", bundle.System)
	}
	if !strings.Contains(bundle.System, "\"no_project\"") {
		t.Fatalf("System prompt = %q, want no_project guidance", bundle.System)
	}
	if !strings.Contains(bundle.System, "project_slug") {
		t.Fatalf("System prompt = %q, want selector schema keys", bundle.System)
	}
}

func TestBuildGeneratorPromptPrefersAgentBrowserAutoConnect(t *testing.T) {
	t.Parallel()

	bundle := BuildGeneratorPrompt(GeneratorInput{
		Run: assistant.Run{
			UserRequestRaw: "Use https://example.com/source and save results to the target list.",
		},
	})

	if !strings.Contains(bundle.System, "--auto-connect") {
		t.Fatalf("System prompt = %q, want agent-browser auto-connect guidance", bundle.System)
	}
	if !strings.Contains(bundle.System, "agent-browser") {
		t.Fatalf("System prompt = %q, want agent-browser guidance", bundle.System)
	}
	if !strings.Contains(strings.ToLower(bundle.System), "webm") {
		t.Fatalf("System prompt = %q, want WebM recording guidance", bundle.System)
	}
	if !strings.Contains(bundle.User, "Original user request: Use https://example.com/source and save results to the target list.") {
		t.Fatalf("User prompt = %q, want original user request context", bundle.User)
	}
}

func TestBuildContractPromptDeclaresStrictJSONContract(t *testing.T) {
	t.Parallel()

	bundle := BuildContractPrompt(ContractInput{
		Run: assistant.Run{
			UserRequestRaw: "Take the cafes from https://www.diningcode.com/list.dc?query=foo and save them into Naver Map.",
			TaskSpec: assistant.TaskSpec{
				Goal:             "Compare competitor pricing",
				Deliverables:     []string{"Pricing table"},
				DoneDefinition:   []string{"Produce the pricing table"},
				EvidenceRequired: []string{"Source URLs"},
			},
		},
	})

	if !strings.Contains(bundle.System, "decision") || !strings.Contains(bundle.System, "acceptance_criteria") {
		t.Fatalf("System prompt = %q, want contract schema keys", bundle.System)
	}
	if !strings.Contains(bundle.System, "strict JSON object") {
		t.Fatalf("System prompt = %q, want strict JSON instruction", bundle.System)
	}
	if !strings.Contains(bundle.User, "Original user request: Take the cafes from https://www.diningcode.com/list.dc?query=foo and save them into Naver Map.") {
		t.Fatalf("User prompt = %q, want original user request context", bundle.User)
	}
}

func TestDecodePlannerOutputNormalizesTaskSpec(t *testing.T) {
	t.Parallel()

	raw := []byte(`{
		"goal":"Compare competitor pricing",
		"deliverables":["Pricing table","Summary memo"],
		"constraints":["Use public sources only"],
		"tools_allowed":[],
		"tools_required":["agent-browser"],
		"done_definition":[],
		"evidence_required":[],
		"risk_flags":["public-web-research"],
		"max_generation_attempts":0
	}`)

	spec, err := DecodePlannerOutput(raw, PlannerInput{
		UserRequestRaw:        "Compare competitor pricing and summarize it.",
		MaxGenerationAttempts: 3,
	})
	if err != nil {
		t.Fatalf("DecodePlannerOutput() error = %v", err)
	}

	if spec.MaxGenerationAttempts != 3 {
		t.Fatalf("MaxGenerationAttempts = %d, want 3", spec.MaxGenerationAttempts)
	}
	if len(spec.ToolsAllowed) == 0 {
		t.Fatal("ToolsAllowed is empty, want normalized defaults")
	}
	if len(spec.DoneDefinition) == 0 || len(spec.EvidenceRequired) == 0 {
		t.Fatal("expected normalized done definition and evidence requirements")
	}
}

func TestDecodeEvaluatorOutputBuildsEvaluation(t *testing.T) {
	t.Parallel()

	raw := []byte(`{
		"passed": false,
		"score": 68,
		"summary": "The output is missing direct source links.",
		"missing_requirements": ["Direct source URLs for each competitor"],
		"incorrect_claims": [],
		"evidence_checked": ["Draft table"],
		"next_action_for_generator": "Collect direct source URLs and update the table."
	}`)

	evaluation, err := DecodeEvaluatorOutput(raw, "run_123", "attempt_456", time.Date(2026, time.March, 27, 10, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("DecodeEvaluatorOutput() error = %v", err)
	}

	if evaluation.RunID != "run_123" || evaluation.AttemptID != "attempt_456" {
		t.Fatalf("evaluation = %#v, want run and attempt ids", evaluation)
	}
	if evaluation.Score != 68 || evaluation.Passed {
		t.Fatalf("evaluation = %#v, want failed score 68", evaluation)
	}
}

func TestDecodeContractOutputBuildsAcceptanceContract(t *testing.T) {
	t.Parallel()

	raw := []byte(`{
		"decision": "agreed",
		"summary": "The contract is concrete enough to start generation.",
		"deliverables": ["Pricing table", "Summary memo"],
		"acceptance_criteria": ["Each competitor has a source URL", "The memo summarizes major price differences"],
		"evidence_required": ["Direct pricing page URLs", "Stored artifact with final table"],
		"constraints": ["Use public sources only"],
		"out_of_scope": ["Private pricing data"],
		"revision_notes": ""
	}`)

	contract, decision, err := DecodeContractOutput(raw, assistant.TaskSpec{
		Goal:             "Compare competitor pricing",
		Deliverables:     []string{"Pricing table", "Summary memo"},
		DoneDefinition:   []string{"Produce deliverables"},
		EvidenceRequired: []string{"Source URLs"},
	})
	if err != nil {
		t.Fatalf("DecodeContractOutput() error = %v", err)
	}
	if decision != ContractDecisionAgreed {
		t.Fatalf("decision = %q, want %q", decision, ContractDecisionAgreed)
	}
	if contract.Status != assistant.ContractStatusAgreed {
		t.Fatalf("contract.Status = %q, want %q", contract.Status, assistant.ContractStatusAgreed)
	}
	if len(contract.AcceptanceCriteria) != 2 {
		t.Fatalf("contract = %#v, want populated acceptance criteria", contract)
	}
}

func TestDecodeProjectSelectorOutputBuildsProjectContext(t *testing.T) {
	t.Parallel()

	raw := []byte(`{
		"project_slug": "infra-cost-estimation",
		"project_name": "Infrastructure Cost Estimation",
		"project_description": "Estimate and track the company's infrastructure spending.",
		"summary": "Selected the existing infrastructure cost estimation project."
	}`)

	project, summary, err := DecodeProjectSelectorOutput(raw)
	if err != nil {
		t.Fatalf("DecodeProjectSelectorOutput() error = %v", err)
	}
	if project.Slug != "infra-cost-estimation" || project.Name == "" || project.Description == "" {
		t.Fatalf("project = %#v, want populated project context", project)
	}
	if summary == "" {
		t.Fatal("summary is empty")
	}
}

func TestBuildEvaluatorPromptIncludesOriginalUserRequest(t *testing.T) {
	t.Parallel()

	bundle := BuildEvaluatorPrompt(EvaluatorInput{
		Run: assistant.Run{
			UserRequestRaw: "Verify the saved list against https://example.com/source.",
			TaskSpec: assistant.TaskSpec{
				Goal:             "Verify the saved list",
				DoneDefinition:   []string{"Compare the saved list to the source"},
				EvidenceRequired: []string{"Source URL", "Saved-list screenshot"},
			},
		},
	})

	if !strings.Contains(bundle.User, "Original user request: Verify the saved list against https://example.com/source.") {
		t.Fatalf("User prompt = %q, want original user request context", bundle.User)
	}
}
