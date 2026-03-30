package prompting

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/siisee11/CodexVirtualAssistant/internal/assistant"
)

type Bundle struct {
	System string `json:"system"`
	User   string `json:"user"`
}

type PlannerInput struct {
	UserRequestRaw        string
	MaxGenerationAttempts int
	Project               assistant.ProjectContext
}

type ContractInput struct {
	Run assistant.Run
}

type ProjectSelectorInput struct {
	UserRequestRaw string
}

type PlannerOutput struct {
	Goal                  string   `json:"goal"`
	Deliverables          []string `json:"deliverables"`
	Constraints           []string `json:"constraints"`
	ToolsAllowed          []string `json:"tools_allowed"`
	ToolsRequired         []string `json:"tools_required"`
	DoneDefinition        []string `json:"done_definition"`
	EvidenceRequired      []string `json:"evidence_required"`
	RiskFlags             []string `json:"risk_flags"`
	MaxGenerationAttempts int      `json:"max_generation_attempts"`
}

type GeneratorInput struct {
	Run           assistant.Run
	Attempt       assistant.Attempt
	PriorCritique string
}

type EvaluatorInput struct {
	Run       assistant.Run
	Attempt   assistant.Attempt
	Artifacts []assistant.Artifact
	Evidence  []assistant.Evidence
}

type EvaluatorOutput struct {
	Passed                 bool     `json:"passed"`
	Score                  int      `json:"score"`
	Summary                string   `json:"summary"`
	MissingRequirements    []string `json:"missing_requirements"`
	IncorrectClaims        []string `json:"incorrect_claims"`
	EvidenceChecked        []string `json:"evidence_checked"`
	NextActionForGenerator string   `json:"next_action_for_generator"`
}

type ProjectSelectorOutput struct {
	ProjectSlug        string `json:"project_slug"`
	ProjectName        string `json:"project_name"`
	ProjectDescription string `json:"project_description"`
	Summary            string `json:"summary"`
}

type ContractDecision string

const (
	ContractDecisionRevise ContractDecision = "revise"
	ContractDecisionAgreed ContractDecision = "agreed"
	ContractDecisionFail   ContractDecision = "fail"
)

type ContractOutput struct {
	Decision           ContractDecision `json:"decision"`
	Summary            string           `json:"summary"`
	Deliverables       []string         `json:"deliverables"`
	AcceptanceCriteria []string         `json:"acceptance_criteria"`
	EvidenceRequired   []string         `json:"evidence_required"`
	Constraints        []string         `json:"constraints"`
	OutOfScope         []string         `json:"out_of_scope"`
	RevisionNotes      string           `json:"revision_notes"`
}

func BuildProjectSelectorPrompt(input ProjectSelectorInput) Bundle {
	return Bundle{
		System: strings.TrimSpace(`
You are the project selector for a WTL GAN-policy based assistant.
Before deciding, inspect the existing project descriptions by reading files that match projects/*/PROJECT.md in the current working directory.
Choose the existing project that best matches the user's request.
If no existing project is a good fit, create a new concise project identity in your response.
If the request is just a simple question, one-off instruction, or low-context task, use the reserved project slug "no_project".
Return one strict JSON object and nothing else.
Do not wrap the JSON in markdown fences.
The JSON object must contain exactly these keys:
- project_slug
- project_name
- project_description
- summary
Use a safe lowercase slug containing only letters, digits, underscores, and hyphens.`),
		User: fmt.Sprintf(
			"User request:\n%s\n\nInspect the available PROJECT.md files yourself, then decide which project should be used for this run.",
			strings.TrimSpace(input.UserRequestRaw),
		),
	}
}

func BuildPlannerPrompt(input PlannerInput) Bundle {
	defaultAttempts := input.MaxGenerationAttempts
	if defaultAttempts <= 0 {
		defaultAttempts = 3
	}

	return Bundle{
		System: strings.TrimSpace(`
You are the planner for a WTL GAN-policy based assistant.
Return one strict JSON object and nothing else.
Do not wrap the JSON in markdown fences.
The JSON object must contain exactly these keys:
- goal
- deliverables
- constraints
- tools_allowed
- tools_required
- done_definition
- evidence_required
- risk_flags
- max_generation_attempts
Prefer "agent-browser" for browser work and keep deliverables and done_definition concrete and evaluator-verifiable.`),
		User: strings.TrimSpace(fmt.Sprintf(
			"%s\n\nUser request:\n%s\n\nDefault max generation attempts: %d\n\nProduce a normalized TaskSpec JSON object.",
			projectPlannerContext(input.Project),
			strings.TrimSpace(input.UserRequestRaw),
			defaultAttempts,
		)),
	}
}

func BuildContractPrompt(input ContractInput) Bundle {
	builder := &strings.Builder{}
	fmt.Fprintf(builder, "Original user request: %s\n", strings.TrimSpace(input.Run.UserRequestRaw))
	fmt.Fprintf(builder, "Goal: %s\n", input.Run.TaskSpec.Goal)
	fmt.Fprintf(builder, "Deliverables: %s\n", strings.Join(input.Run.TaskSpec.Deliverables, "; "))
	fmt.Fprintf(builder, "Constraints: %s\n", strings.Join(input.Run.TaskSpec.Constraints, "; "))
	fmt.Fprintf(builder, "Done definition: %s\n", strings.Join(input.Run.TaskSpec.DoneDefinition, "; "))
	fmt.Fprintf(builder, "Evidence required: %s\n", strings.Join(input.Run.TaskSpec.EvidenceRequired, "; "))
	if input.Run.TaskSpec.AcceptanceContract != nil && strings.TrimSpace(input.Run.TaskSpec.AcceptanceContract.RevisionNotes) != "" {
		fmt.Fprintf(builder, "Previous contract revision notes: %s\n", input.Run.TaskSpec.AcceptanceContract.RevisionNotes)
	}

	return Bundle{
		System: strings.TrimSpace(`
You are the contract phase for a WTL GAN-policy based assistant.
Turn the planner TaskSpec into an explicit acceptance contract that the generator and evaluator will both follow.
Return one strict JSON object and nothing else.
Do not wrap the JSON in markdown fences.
The JSON object must contain exactly these keys:
- decision
- summary
- deliverables
- acceptance_criteria
- evidence_required
- constraints
- out_of_scope
- revision_notes
Use decision="agreed" only when the contract is concrete enough for generation to start.
Use decision="revise" when the contract still needs tightening and explain the gap in revision_notes.
Use decision="fail" only when the task cannot be contracted safely from the available information.`),
		User: strings.TrimSpace(builder.String()),
	}
}

func DecodeProjectSelectorOutput(raw []byte) (assistant.ProjectContext, string, error) {
	var output ProjectSelectorOutput
	if err := json.Unmarshal(raw, &output); err != nil {
		return assistant.ProjectContext{}, "", fmt.Errorf("decode project selector output: %w", err)
	}
	project := assistant.ProjectContext{
		Slug:        strings.TrimSpace(output.ProjectSlug),
		Name:        strings.TrimSpace(output.ProjectName),
		Description: strings.TrimSpace(output.ProjectDescription),
	}
	if project.Slug == "" || project.Name == "" || project.Description == "" {
		return assistant.ProjectContext{}, "", fmt.Errorf("decode project selector output: missing required project fields")
	}
	return project, strings.TrimSpace(output.Summary), nil
}

func DecodePlannerOutput(raw []byte, input PlannerInput) (assistant.TaskSpec, error) {
	var output PlannerOutput
	if err := json.Unmarshal(raw, &output); err != nil {
		return assistant.TaskSpec{}, fmt.Errorf("decode planner output: %w", err)
	}

	return assistant.NormalizeTaskSpec(assistant.TaskSpecDraft{
		Goal:                  output.Goal,
		UserRequestRaw:        input.UserRequestRaw,
		Deliverables:          output.Deliverables,
		Constraints:           output.Constraints,
		ToolsAllowed:          output.ToolsAllowed,
		ToolsRequired:         output.ToolsRequired,
		DoneDefinition:        output.DoneDefinition,
		EvidenceRequired:      output.EvidenceRequired,
		RiskFlags:             output.RiskFlags,
		MaxGenerationAttempts: output.MaxGenerationAttempts,
	}, input.UserRequestRaw, input.MaxGenerationAttempts)
}

func DecodeContractOutput(raw []byte, spec assistant.TaskSpec) (assistant.AcceptanceContract, ContractDecision, error) {
	var output ContractOutput
	if err := json.Unmarshal(raw, &output); err != nil {
		return assistant.AcceptanceContract{}, "", fmt.Errorf("decode contract output: %w", err)
	}

	switch output.Decision {
	case ContractDecisionRevise, ContractDecisionAgreed, ContractDecisionFail:
	default:
		return assistant.AcceptanceContract{}, "", fmt.Errorf("decode contract output: unsupported decision %q", output.Decision)
	}

	status := assistant.ContractStatusDraft
	if output.Decision == ContractDecisionAgreed {
		status = assistant.ContractStatusAgreed
	}

	contract, err := assistant.NormalizeAcceptanceContract(assistant.AcceptanceContractDraft{
		Status:             status,
		Summary:            output.Summary,
		Deliverables:       output.Deliverables,
		AcceptanceCriteria: output.AcceptanceCriteria,
		EvidenceRequired:   output.EvidenceRequired,
		Constraints:        output.Constraints,
		OutOfScope:         output.OutOfScope,
		RevisionNotes:      output.RevisionNotes,
	}, spec)
	if err != nil {
		return assistant.AcceptanceContract{}, "", fmt.Errorf("decode contract output: %w", err)
	}
	return contract, output.Decision, nil
}

func BuildGeneratorPrompt(input GeneratorInput) Bundle {
	builder := &strings.Builder{}
	fmt.Fprintf(builder, "Original user request: %s\n", strings.TrimSpace(input.Run.UserRequestRaw))
	fmt.Fprintf(builder, "Goal: %s\n", input.Run.TaskSpec.Goal)
	appendContractContext(builder, input.Run.TaskSpec)
	if input.PriorCritique != "" {
		fmt.Fprintf(builder, "Evaluator critique to address: %s\n", input.PriorCritique)
	}
	return Bundle{
		System: "You are the generator for a WTL GAN-policy based assistant. Complete the real work and retain evidence. When browser work uses agent-browser, try --auto-connect first so an existing authenticated Chrome session can be reused before falling back to manual login or other session bootstrapping. If you produce or export a browser recording during the task, prefer recording or re-encoding it as WebM instead of MP4 whenever the tool supports that choice.",
		User:   strings.TrimSpace(builder.String()),
	}
}

func BuildEvaluatorPrompt(input EvaluatorInput) Bundle {
	builder := &strings.Builder{}
	fmt.Fprintf(builder, "Original user request: %s\n", strings.TrimSpace(input.Run.UserRequestRaw))
	fmt.Fprintf(builder, "Goal: %s\n", input.Run.TaskSpec.Goal)
	appendContractContext(builder, input.Run.TaskSpec)
	fmt.Fprintf(builder, "Artifacts submitted: %d\n", len(input.Artifacts))
	fmt.Fprintf(builder, "Evidence items submitted: %d\n", len(input.Evidence))
	return Bundle{
		System: "You are the evaluator for a WTL GAN-policy based assistant. Judge completion strictly against the accepted contract and return strict JSON with passed, score, summary, missing_requirements, incorrect_claims, evidence_checked, and next_action_for_generator.",
		User:   strings.TrimSpace(builder.String()),
	}
}

func DecodeEvaluatorOutput(raw []byte, runID, attemptID string, now time.Time) (assistant.Evaluation, error) {
	var output EvaluatorOutput
	if err := json.Unmarshal(raw, &output); err != nil {
		return assistant.Evaluation{}, fmt.Errorf("decode evaluator output: %w", err)
	}

	evaluation := assistant.Evaluation{
		ID:                     assistant.NewID("evaluation", now),
		RunID:                  runID,
		AttemptID:              attemptID,
		Passed:                 output.Passed,
		Score:                  output.Score,
		Summary:                strings.TrimSpace(output.Summary),
		MissingRequirements:    output.MissingRequirements,
		IncorrectClaims:        output.IncorrectClaims,
		EvidenceChecked:        output.EvidenceChecked,
		NextActionForGenerator: strings.TrimSpace(output.NextActionForGenerator),
		CreatedAt:              now.UTC(),
	}
	if err := evaluation.Validate(); err != nil {
		return assistant.Evaluation{}, err
	}
	return evaluation, nil
}

func projectPlannerContext(project assistant.ProjectContext) string {
	if strings.TrimSpace(project.Slug) == "" {
		return "Project context:\n- No project selected yet."
	}
	return fmt.Sprintf(
		"Project context:\n- Slug: %s\n- Name: %s\n- Purpose: %s\n- Workspace: %s",
		project.Slug,
		firstNonEmpty(project.Name, project.Slug),
		project.Description,
		project.WorkspaceDir,
	)
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func appendContractContext(builder *strings.Builder, spec assistant.TaskSpec) {
	if spec.AcceptanceContract != nil && spec.AcceptanceContract.Status == assistant.ContractStatusAgreed {
		fmt.Fprintf(builder, "Accepted contract summary: %s\n", spec.AcceptanceContract.Summary)
		fmt.Fprintf(builder, "Accepted deliverables: %s\n", strings.Join(spec.AcceptanceContract.Deliverables, "; "))
		fmt.Fprintf(builder, "Acceptance criteria: %s\n", strings.Join(spec.AcceptanceContract.AcceptanceCriteria, "; "))
		fmt.Fprintf(builder, "Evidence required: %s\n", strings.Join(spec.AcceptanceContract.EvidenceRequired, "; "))
		if len(spec.AcceptanceContract.Constraints) > 0 {
			fmt.Fprintf(builder, "Accepted constraints: %s\n", strings.Join(spec.AcceptanceContract.Constraints, "; "))
		}
		if len(spec.AcceptanceContract.OutOfScope) > 0 {
			fmt.Fprintf(builder, "Out of scope: %s\n", strings.Join(spec.AcceptanceContract.OutOfScope, "; "))
		}
		return
	}
	fmt.Fprintf(builder, "Done definition: %s\n", strings.Join(spec.DoneDefinition, "; "))
	fmt.Fprintf(builder, "Evidence required: %s\n", strings.Join(spec.EvidenceRequired, "; "))
}
