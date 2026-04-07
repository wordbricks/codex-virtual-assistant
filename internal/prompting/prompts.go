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

type ParentRunContext struct {
	RunID          string
	UserRequestRaw string
	Summary        string
	Artifacts      []assistant.Artifact
	Evidence       []assistant.Evidence
}

type GateInput struct {
	Run           assistant.Run
	ParentContext *ParentRunContext
}

type ProjectSelectorInput struct {
	UserRequestRaw string
}

type PlannerOutput struct {
	Goal                  string                  `json:"goal"`
	Deliverables          []string                `json:"deliverables"`
	Constraints           []string                `json:"constraints"`
	ToolsAllowed          []string                `json:"tools_allowed"`
	ToolsRequired         []string                `json:"tools_required"`
	DoneDefinition        []string                `json:"done_definition"`
	EvidenceRequired      []string                `json:"evidence_required"`
	RiskFlags             []string                `json:"risk_flags"`
	MaxGenerationAttempts int                     `json:"max_generation_attempts"`
	SchedulePlan          *assistant.SchedulePlan `json:"schedule_plan"`
}

type GateOutput struct {
	Route   string `json:"route"`
	Reason  string `json:"reason"`
	Summary string `json:"summary"`
}

type GeneratorInput struct {
	Run           assistant.Run
	Attempt       assistant.Attempt
	PriorCritique string
}

type AnswerInput struct {
	Run           assistant.Run
	ParentContext *ParentRunContext
}

type AnswerOutput struct {
	Summary         string `json:"summary"`
	Output          string `json:"output"`
	NeedsUserInput  bool   `json:"needs_user_input"`
	WaitKind        string `json:"wait_kind"`
	WaitTitle       string `json:"wait_title"`
	WaitPrompt      string `json:"wait_prompt"`
	WaitRiskSummary string `json:"wait_risk_summary"`
}

type EvaluatorInput struct {
	Run       assistant.Run
	Attempt   assistant.Attempt
	Artifacts []assistant.Artifact
	Evidence  []assistant.Evidence
}

type SchedulerInput struct {
	Run       assistant.Run
	Artifacts []assistant.Artifact
	Evidence  []assistant.Evidence
}

type SchedulerOutput struct {
	Entries []assistant.ScheduleEntry `json:"entries"`
}

type ReportInput struct {
	Run                 assistant.Run
	Artifacts           []assistant.Artifact
	Evidence            []assistant.Evidence
	ToolCalls           []assistant.ToolCall
	LatestEvaluation    *assistant.Evaluation
	ChatAccountUsername string
	MasterUsername      string
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

type ReportOutput struct {
	Summary         string `json:"summary"`
	DeliveryStatus  string `json:"delivery_status"`
	MessagePreview  string `json:"message_preview"`
	ReportPayload   string `json:"report_payload"`
	NeedsUserInput  bool   `json:"needs_user_input"`
	WaitKind        string `json:"wait_kind"`
	WaitTitle       string `json:"wait_title"`
	WaitPrompt      string `json:"wait_prompt"`
	WaitRiskSummary string `json:"wait_risk_summary"`
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
 - schedule_plan
Use schedule_plan=null when all work should happen immediately.
If the request includes future or time-distributed work, keep immediate work in the normal TaskSpec fields and place only the deferred work in schedule_plan.entries.
Each schedule_plan entry must contain scheduled_for and prompt.
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

func BuildGatePrompt(input GateInput) Bundle {
	builder := &strings.Builder{}
	fmt.Fprintf(builder, "Original user request: %s\n", strings.TrimSpace(input.Run.UserRequestRaw))
	appendParentContext(builder, input.ParentContext)
	return Bundle{
		System: strings.TrimSpace(`
You are the gate phase for a WTL GAN-policy based assistant.
Decide whether this run should go to:
- route="answer" for a read-oriented response that can be completed from existing context and evidence, or
- route="workflow" for full execution work (project selection + planning + contracting + generating + evaluating).
Return one strict JSON object and nothing else.
Do not wrap the JSON in markdown fences.
The JSON object must contain exactly these keys:
- route
- reason
- summary
Use only "answer" or "workflow" for route.
Choose "answer" only when no new side-effecting execution is required.`),
		User: strings.TrimSpace(builder.String()),
	}
}

func BuildAnswerPrompt(input AnswerInput) Bundle {
	builder := &strings.Builder{}
	fmt.Fprintf(builder, "Original user request: %s\n", strings.TrimSpace(input.Run.UserRequestRaw))
	appendParentContext(builder, input.ParentContext)
	if strings.TrimSpace(input.Run.TaskSpec.Goal) != "" {
		fmt.Fprintf(builder, "Current run goal: %s\n", strings.TrimSpace(input.Run.TaskSpec.Goal))
	}
	return Bundle{
		System: strings.TrimSpace(`
You are the answer phase for a WTL GAN-policy based assistant.
This phase is read-oriented: prioritize existing run context, stored artifacts, and stored evidence over fresh execution.
Return one strict JSON object and nothing else.
Do not wrap the JSON in markdown fences.
The JSON object must contain exactly these keys:
- summary
- output
- needs_user_input
- wait_kind
- wait_title
- wait_prompt
- wait_risk_summary
When you can answer now, set needs_user_input=false and leave wait_* fields empty strings.
When required information is missing, set needs_user_input=true and fill wait_* for a clarification or authentication request.`),
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

func DecodeGateOutput(raw []byte) (assistant.RunRoute, string, string, error) {
	var output GateOutput
	if err := json.Unmarshal(raw, &output); err != nil {
		return "", "", "", fmt.Errorf("decode gate output: %w", err)
	}

	route := assistant.RunRoute(strings.TrimSpace(output.Route))
	switch route {
	case assistant.RunRouteAnswer, assistant.RunRouteWorkflow:
	default:
		return "", "", "", fmt.Errorf("decode gate output: unsupported route %q", output.Route)
	}

	reason := strings.TrimSpace(output.Reason)
	if reason == "" {
		return "", "", "", fmt.Errorf("decode gate output: reason is required")
	}
	summary := strings.TrimSpace(firstNonEmpty(output.Summary, reason))
	return route, reason, summary, nil
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
		SchedulePlan:          output.SchedulePlan,
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
		System: "You are the generator for a WTL GAN-policy based assistant. Complete the real work and retain evidence. When browser work uses agent-browser, run it in foreground/headed mode by default and prefer the current command pattern, for example: agent-browser open <url> --headed, then agent-browser snapshot -i --json before interacting. If a project-specific browser profile and CDP port are available, prefer that profile over saved auth state files and over --auto-connect. Before launching a new Chrome window, first health-check the project CDP endpoint with curl -sS http://localhost:<port>/json/version. If that health check succeeds, do not launch a new Chrome window and do not call agent-browser close first; connect immediately with agent-browser connect http://localhost:<port> and reuse the existing project Chrome session. Only if the health check fails should you launch Chrome with that profile and fixed remote debugging port. If agent-browser connect times out even though the CDP health check succeeded, treat that as a stale agent-browser session issue: then run agent-browser close once, reconnect, and avoid launching another Chrome window unless the CDP health check stops responding. Reuse the same project profile across runs so login state persists in the profile directory. Persist auth with explicit state files instead. Use explicit auth state files only as a secondary export/import mechanism, and do not rely on --session-name for auth persistence. Use --auto-connect only when the task must attach to the user's already-running Chrome session and a project-specific browser profile cannot be used. If --auto-connect succeeds, immediately save a fresh auth state to a project-local path, then continue future navigation by opening a blank page, running agent-browser state load <path>, and only then opening the target URL. After a successful state save, do not keep relying on --auto-connect in the same task unless the saved state fails and login must be recovered again. When reusing saved state, prefer opening a blank page, running agent-browser state load <path>, and only then opening the target URL instead of relying on --state during the initial open command. When using --auto-connect to attach to a real Google Chrome session, Chrome may show an 'Allow remote debugging?' dialog. If an attach attempt or the first browser command times out during that flow, do not assume the attempt failed. Ask the user to click Allow in Chrome, return a wait_request for approval, and retry after the user confirms approval. If you produce or export a browser recording during the task, prefer recording or re-encoding it as WebM instead of MP4 whenever the tool supports that choice.",
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

func BuildReportPrompt(input ReportInput) Bundle {
	builder := &strings.Builder{}
	fmt.Fprintf(builder, "Original user request: %s\n", strings.TrimSpace(input.Run.UserRequestRaw))
	fmt.Fprintf(builder, "Chat id: %s\n", strings.TrimSpace(input.Run.ChatID))
	fmt.Fprintf(builder, "Chat account username: %s\n", strings.TrimSpace(input.ChatAccountUsername))
	if strings.TrimSpace(input.MasterUsername) != "" {
		fmt.Fprintf(builder, "Configured recipient username: %s\n", strings.TrimSpace(input.MasterUsername))
	}
	fmt.Fprintf(builder, "Goal: %s\n", strings.TrimSpace(input.Run.TaskSpec.Goal))
	appendContractContext(builder, input.Run.TaskSpec)
	if input.LatestEvaluation != nil {
		fmt.Fprintf(builder, "Latest evaluation summary: %s\n", strings.TrimSpace(input.LatestEvaluation.Summary))
	}
	appendArtifactHighlights(builder, input.Artifacts, 6)
	appendEvidenceHighlights(builder, input.Evidence, 10)
	appendToolCallHighlights(builder, input.ToolCalls, 8)

	return Bundle{
		System: strings.TrimSpace(`
You are the report phase for a WTL GAN-policy based assistant.
Your job is to deliver the final user-facing result through agent-message before the run can complete.
Before composing the message, call agent-message catalog prompt and use it as the source of truth for the json_render component catalog and constraints.
Then send exactly one final report with agent-message send --kind json_render '<json-object>'.
Return one strict JSON object and nothing else.
Do not wrap the JSON in markdown fences.
The JSON object must contain exactly these keys:
- summary
- delivery_status
- message_preview
- report_payload
- needs_user_input
- wait_kind
- wait_title
- wait_prompt
- wait_risk_summary
Use delivery_status="sent" only after the final report was successfully sent.
Use needs_user_input=true only when you must safely pause for missing information or configuration.
When needs_user_input=false, report_payload must be the exact JSON object string that was sent and message_preview must summarize the visible user-facing message.`),
		User: strings.TrimSpace(builder.String()),
	}
}

func BuildSchedulerPrompt(input SchedulerInput) Bundle {
	builder := &strings.Builder{}
	fmt.Fprintf(builder, "Original user request: %s\n", strings.TrimSpace(input.Run.UserRequestRaw))
	fmt.Fprintf(builder, "Goal: %s\n", strings.TrimSpace(input.Run.TaskSpec.Goal))
	if input.Run.TaskSpec.SchedulePlan != nil {
		fmt.Fprintf(builder, "Planned schedule entries:\n")
		for _, entry := range input.Run.TaskSpec.SchedulePlan.Entries {
			fmt.Fprintf(builder, "- At %s: %s\n", strings.TrimSpace(entry.ScheduledFor), strings.TrimSpace(entry.Prompt))
		}
	}
	appendArtifactHighlights(builder, input.Artifacts, 6)
	appendEvidenceHighlights(builder, input.Evidence, 10)

	return Bundle{
		System: strings.TrimSpace(`
You are the scheduler phase for a WTL GAN-policy based assistant.
Finalize the deferred execution prompts using concrete details already discovered during the run.
Resolve template placeholders into specific names, phone numbers, URLs, or other facts when the evidence supports them.
Return one strict JSON object and nothing else.
Do not wrap the JSON in markdown fences.
The JSON object must contain exactly these keys:
- entries
Each entry object must contain exactly:
- scheduled_for
- prompt
Use RFC3339 timestamps for scheduled_for whenever you can resolve them precisely.`),
		User: strings.TrimSpace(builder.String()),
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

func DecodeAnswerOutput(raw []byte) (AnswerOutput, error) {
	var output AnswerOutput
	if err := json.Unmarshal(raw, &output); err != nil {
		return AnswerOutput{}, fmt.Errorf("decode answer output: %w", err)
	}
	output.Summary = strings.TrimSpace(output.Summary)
	output.Output = strings.TrimSpace(output.Output)
	output.WaitKind = strings.TrimSpace(output.WaitKind)
	output.WaitTitle = strings.TrimSpace(output.WaitTitle)
	output.WaitPrompt = strings.TrimSpace(output.WaitPrompt)
	output.WaitRiskSummary = strings.TrimSpace(output.WaitRiskSummary)

	if output.NeedsUserInput {
		if output.WaitKind == "" || output.WaitPrompt == "" {
			return AnswerOutput{}, fmt.Errorf("decode answer output: wait_kind and wait_prompt are required when needs_user_input is true")
		}
		return output, nil
	}
	if output.Output == "" {
		return AnswerOutput{}, fmt.Errorf("decode answer output: output is required when needs_user_input is false")
	}
	return output, nil
}

func DecodeReportOutput(raw []byte) (ReportOutput, error) {
	var output ReportOutput
	if err := json.Unmarshal(raw, &output); err != nil {
		return ReportOutput{}, fmt.Errorf("decode report output: %w", err)
	}

	output.Summary = strings.TrimSpace(output.Summary)
	output.DeliveryStatus = strings.TrimSpace(output.DeliveryStatus)
	output.MessagePreview = strings.TrimSpace(output.MessagePreview)
	output.ReportPayload = strings.TrimSpace(output.ReportPayload)
	output.WaitKind = strings.TrimSpace(output.WaitKind)
	output.WaitTitle = strings.TrimSpace(output.WaitTitle)
	output.WaitPrompt = strings.TrimSpace(output.WaitPrompt)
	output.WaitRiskSummary = strings.TrimSpace(output.WaitRiskSummary)

	if output.NeedsUserInput {
		if output.WaitKind == "" || output.WaitPrompt == "" {
			return ReportOutput{}, fmt.Errorf("decode report output: wait_kind and wait_prompt are required when needs_user_input is true")
		}
		return output, nil
	}
	if output.DeliveryStatus != "sent" {
		return ReportOutput{}, fmt.Errorf("decode report output: delivery_status must be sent when no wait is requested")
	}
	if output.ReportPayload == "" {
		return ReportOutput{}, fmt.Errorf("decode report output: report_payload is required when delivery succeeds")
	}
	if output.MessagePreview == "" {
		return ReportOutput{}, fmt.Errorf("decode report output: message_preview is required when delivery succeeds")
	}
	return output, nil
}

func DecodeSchedulerOutput(raw []byte) ([]assistant.ScheduleEntry, error) {
	var output SchedulerOutput
	if err := json.Unmarshal(raw, &output); err != nil {
		return nil, fmt.Errorf("decode scheduler output: %w", err)
	}
	plan := assistant.SchedulePlan{Entries: output.Entries}
	if err := plan.Validate(); err != nil {
		return nil, fmt.Errorf("decode scheduler output: %w", err)
	}
	return output.Entries, nil
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
	if spec.SchedulePlan != nil && len(spec.SchedulePlan.Entries) > 0 {
		fmt.Fprintf(builder, "Planned schedule entries:\n")
		for _, entry := range spec.SchedulePlan.Entries {
			fmt.Fprintf(builder, "- At %s: %s\n", strings.TrimSpace(entry.ScheduledFor), strings.TrimSpace(entry.Prompt))
		}
	}
}

func appendParentContext(builder *strings.Builder, parent *ParentRunContext) {
	if parent == nil || strings.TrimSpace(parent.RunID) == "" {
		fmt.Fprintf(builder, "Parent run context: none.\n")
		return
	}

	fmt.Fprintf(builder, "Parent run id: %s\n", strings.TrimSpace(parent.RunID))
	if request := strings.TrimSpace(parent.UserRequestRaw); request != "" {
		fmt.Fprintf(builder, "Parent user request: %s\n", request)
	}
	if summary := strings.TrimSpace(parent.Summary); summary != "" {
		fmt.Fprintf(builder, "Parent summary: %s\n", summary)
	}

	if len(parent.Artifacts) == 0 {
		fmt.Fprintf(builder, "Parent artifacts: none.\n")
	} else {
		fmt.Fprintf(builder, "Parent artifacts (most recent first, capped to 5):\n")
		artifacts := parent.Artifacts
		for idx := len(artifacts) - 1; idx >= 0 && idx >= len(artifacts)-5; idx-- {
			artifact := artifacts[idx]
			fmt.Fprintf(
				builder,
				"- [%s] %s (%s)%s\n",
				artifact.Kind,
				firstNonEmpty(artifact.Title, artifact.ID),
				artifact.MIMEType,
				formatOptionalSuffix(firstNonEmpty(artifact.SourceURL, artifact.Path)),
			)
		}
	}

	if len(parent.Evidence) == 0 {
		fmt.Fprintf(builder, "Parent evidence: none.\n")
		return
	}
	fmt.Fprintf(builder, "Parent evidence highlights (most recent first, capped to 8):\n")
	evidence := parent.Evidence
	for idx := len(evidence) - 1; idx >= 0 && idx >= len(evidence)-8; idx-- {
		item := evidence[idx]
		fmt.Fprintf(builder, "- [%s] %s\n", item.Kind, firstNonEmpty(item.Summary, item.Detail))
	}
}

func appendArtifactHighlights(builder *strings.Builder, artifacts []assistant.Artifact, limit int) {
	if len(artifacts) == 0 {
		fmt.Fprintf(builder, "Artifacts: none.\n")
		return
	}
	fmt.Fprintf(builder, "Artifacts (most recent first, capped to %d):\n", limit)
	for idx := len(artifacts) - 1; idx >= 0 && idx >= len(artifacts)-limit; idx-- {
		artifact := artifacts[idx]
		fmt.Fprintf(builder, "- [%s] %s (%s)\n", artifact.Kind, firstNonEmpty(artifact.Title, artifact.ID), artifact.MIMEType)
	}
}

func appendEvidenceHighlights(builder *strings.Builder, evidence []assistant.Evidence, limit int) {
	if len(evidence) == 0 {
		fmt.Fprintf(builder, "Evidence: none.\n")
		return
	}
	fmt.Fprintf(builder, "Evidence highlights (most recent first, capped to %d):\n", limit)
	for idx := len(evidence) - 1; idx >= 0 && idx >= len(evidence)-limit; idx-- {
		item := evidence[idx]
		fmt.Fprintf(builder, "- [%s] %s\n", item.Kind, firstNonEmpty(item.Summary, item.Detail))
	}
}

func appendToolCallHighlights(builder *strings.Builder, toolCalls []assistant.ToolCall, limit int) {
	if len(toolCalls) == 0 {
		fmt.Fprintf(builder, "Recorded tool calls: none.\n")
		return
	}
	fmt.Fprintf(builder, "Recorded tool calls (most recent first, capped to %d):\n", limit)
	for idx := len(toolCalls) - 1; idx >= 0 && idx >= len(toolCalls)-limit; idx-- {
		toolCall := toolCalls[idx]
		fmt.Fprintf(builder, "- %s: %s\n", toolCall.ToolName, firstNonEmpty(toolCall.OutputSummary, toolCall.InputSummary))
	}
}

func formatOptionalSuffix(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	return " | " + value
}
