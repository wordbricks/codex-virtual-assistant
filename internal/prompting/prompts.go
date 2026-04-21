package prompting

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/siisee11/CodexVirtualAssistant/internal/assistant"
	"github.com/siisee11/CodexVirtualAssistant/internal/config"
)

type Bundle struct {
	System string `json:"system"`
	User   string `json:"user"`
}

type PlannerInput struct {
	UserRequestRaw        string
	MaxGenerationAttempts int
	Project               assistant.ProjectContext
	Wiki                  assistant.WikiContext
	AutomationSafety      config.AutomationSafetyConfig
}

type ContractInput struct {
	Run assistant.Run
}

type ParentRunContext struct {
	RunID          string
	UserRequestRaw string
	Project        assistant.ProjectContext
	Wiki           assistant.WikiContext
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
	Goal                  string                            `json:"goal"`
	Deliverables          []string                          `json:"deliverables"`
	Constraints           []string                          `json:"constraints"`
	ToolsAllowed          []string                          `json:"tools_allowed"`
	ToolsRequired         []string                          `json:"tools_required"`
	DoneDefinition        []string                          `json:"done_definition"`
	EvidenceRequired      []string                          `json:"evidence_required"`
	RiskFlags             []string                          `json:"risk_flags"`
	AutomationSafety      *assistant.AutomationSafetyPolicy `json:"automation_safety"`
	MaxGenerationAttempts int                               `json:"max_generation_attempts"`
	SchedulePlan          *assistant.SchedulePlan           `json:"schedule_plan"`
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
	Run            assistant.Run
	Attempt        assistant.Attempt
	Artifacts      []assistant.Artifact
	Evidence       []assistant.Evidence
	RecentActivity *assistant.BrowserRecentActivityMetrics
}

type SchedulerInput struct {
	Run            assistant.Run
	Artifacts      []assistant.Artifact
	Evidence       []assistant.Evidence
	RecentActivity *assistant.BrowserRecentActivityMetrics
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
When a project already has a wiki, also inspect projects/*/wiki/overview.md and projects/*/wiki/index.md before deciding.
Choose the existing project that best matches the user's request.
Prioritize the user's enduring domain, final purpose, and long-lived objective over the immediate workflow shape or task wording.
Do not split work into a different project merely because the request mentions retrospectives, replanning, scheduling, blocker review, or another support workflow if the underlying goal still belongs to an existing domain project.
If a request is about improving or continuing the same real-world effort, keep it in that effort's project even when the current step is analysis, planning, or scheduling rather than direct execution.
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
- automation_safety
- max_generation_attempts
- schedule_plan
Use automation_safety=null when the run is not browser automation or has no automation-safety policy requirements.
When browser automation is relevant, prefer a structured automation_safety object with profile and enforcement, plus optional mode_policy/rate_limits/pattern_rules/text_reuse_policy/cooldown_policy.
Valid automation safety profiles are: none, browser_read_only, browser_mutating, browser_high_risk_engagement.
Valid automation safety mode_policy.allowed_session_modes values are: read_only, single_action, reply_only. Do not invent other session mode names.
Treat social growth, outreach messaging, public comments or replies, follows or connection requests, marketplace inquiries, community engagement, recruiting/networking messages, likes, endorsements, reviews, and repeated application/inquiry submission workflows as high-risk engagement candidates.
Use schedule_plan=null when all work should happen immediately.
If the request includes future or time-distributed work, keep immediate work in the normal TaskSpec fields and place only the deferred work in schedule_plan.entries.
Each schedule_plan entry must contain scheduled_for and prompt.
Prefer "agent-browser" for browser work and keep deliverables and done_definition concrete and evaluator-verifiable.`),
		User: strings.TrimSpace(fmt.Sprintf(
			"%s\n\nUser request:\n%s\n\nDefault max generation attempts: %d\n\nProduce a normalized TaskSpec JSON object.",
			projectPlannerContext(input.Project, input.Wiki, input.AutomationSafety),
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
	appendAutomationSafetyPromptContext(builder, input.Run.TaskSpec.AutomationSafety)
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
When automation safety policy context is present, translate it into explicit acceptance_criteria and evidence_required entries.
If mode_policy.allow_no_action_success=true, include a criterion that no-action completion is acceptable when mutating action is unsafe.
If mode_policy.require_no_action_evidence=true, include evidence requirements for observed context, skipped action, skip reason, and safer next step.
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
	appendWikiContext(builder, input.Run.Project, input.ParentContext)
	appendParentContext(builder, input.ParentContext)
	if strings.TrimSpace(input.Run.TaskSpec.Goal) != "" {
		fmt.Fprintf(builder, "Current run goal: %s\n", strings.TrimSpace(input.Run.TaskSpec.Goal))
	}
	return Bundle{
		System: strings.TrimSpace(`
You are the answer phase for a WTL GAN-policy based assistant.
This phase is read-oriented: prioritize project wiki context, existing run context, stored artifacts, and stored evidence over fresh execution.
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
When required information is missing, the wiki is stale, or the question clearly needs new execution work, set needs_user_input=true and fill wait_* with a confirmation or clarification request that explains workflow execution is needed.`),
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

	inferredProfile := inferAutomationSafetyProfile(input, output)
	resolver := config.Config{AutomationSafety: input.AutomationSafety}
	resolvedPolicy := resolver.ResolveAutomationSafetyPolicy(output.AutomationSafety, inferredProfile, input.Project.Slug)

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
		AutomationSafety:      resolvedPolicy,
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
	appendAutomationSafetyPromptContext(builder, input.Run.TaskSpec.AutomationSafety)
	if input.PriorCritique != "" {
		fmt.Fprintf(builder, "Evaluator critique to address: %s\n", input.PriorCritique)
	}
	return Bundle{
		System: "You are the generator for a WTL GAN-policy based assistant. Complete the real work and retain evidence. Obey any automation safety policy context provided in the task input. Treat no-action as a valid terminal path only when the policy allows it. If no-action is taken and the policy requires no-action evidence, record the observed context, skipped mutating action, safety reason for skipping, and a safer next step. Preserve enough browser action detail for downstream safety metrics, including action type, target/source context, whether external account state changed, and timing/spacing context for mutating actions. When browser work uses agent-browser, run it in foreground/headed mode by default and prefer the current command pattern, for example: agent-browser open <url> --headed, then agent-browser snapshot -i --json before interacting. If a project-specific browser profile and CDP port are available, prefer that profile over saved auth state files and over --auto-connect. Before launching a new Chrome window, first health-check the project CDP endpoint with curl -sS http://localhost:<port>/json/version. If that health check succeeds, do not launch a new Chrome window and do not call agent-browser connect; instead, reuse the existing project Chrome session by passing --cdp <port> directly on every agent-browser command, for example: agent-browser --cdp <port> open about:blank, then agent-browser --cdp <port> snapshot -i --json. Only if the health check fails should you launch Chrome with that profile and fixed remote debugging port. If an agent-browser command that uses --cdp <port> times out even though the CDP health check succeeded, treat that as a stale agent-browser session issue: then run agent-browser close once, retry the same --cdp <port> command, and avoid launching another Chrome window unless the CDP health check stops responding. Reuse the same project profile across runs so login state persists in the profile directory. Persist auth with explicit state files instead. Use explicit auth state files only as a secondary export/import mechanism, and do not rely on --session-name for auth persistence. Use --auto-connect only when the task must attach to the user's already-running Chrome session and a project-specific browser profile cannot be used. If --auto-connect succeeds, immediately save a fresh auth state to a project-local path, then continue future navigation by opening a blank page, running agent-browser state load <path>, and only then opening the target URL. After a successful state save, do not keep relying on --auto-connect in the same task unless the saved state fails and login must be recovered again. When reusing saved state, prefer opening a blank page, running agent-browser state load <path>, and only then opening the target URL instead of relying on --state during the initial open command. When using --auto-connect to attach to a real Google Chrome session, Chrome may show an 'Allow remote debugging?' dialog. If an attach attempt or the first browser command times out during that flow, do not assume the attempt failed. Ask the user to click Allow in Chrome, return a wait_request for approval, and retry after the user confirms approval. If you produce or export a browser recording during the task, prefer recording or re-encoding it as WebM instead of MP4 whenever the tool supports that choice.",
		User:   strings.TrimSpace(builder.String()),
	}
}

func BuildEvaluatorPrompt(input EvaluatorInput) Bundle {
	builder := &strings.Builder{}
	fmt.Fprintf(builder, "Original user request: %s\n", strings.TrimSpace(input.Run.UserRequestRaw))
	fmt.Fprintf(builder, "Goal: %s\n", input.Run.TaskSpec.Goal)
	appendContractContext(builder, input.Run.TaskSpec)
	appendAutomationSafetyPromptContext(builder, input.Run.TaskSpec.AutomationSafety)
	fmt.Fprintf(builder, "Artifacts submitted: %d\n", len(input.Artifacts))
	fmt.Fprintf(builder, "Evidence items submitted: %d\n", len(input.Evidence))
	appendRecentActivityMetrics(builder, input.RecentActivity)
	return Bundle{
		System: "You are the evaluator for a WTL GAN-policy based assistant. Judge completion strictly against the accepted contract and any automation safety policy context. For browser_mutating runs, treat safety violations as failed evaluations with actionable next_action_for_generator guidance. For browser_high_risk_engagement runs, deterministic hard-limit violations and missing required no-action evidence must not pass. Return strict JSON with passed, score, summary, missing_requirements, incorrect_claims, evidence_checked, and next_action_for_generator.",
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
	appendAutomationSafetyPromptContext(builder, input.Run.TaskSpec.AutomationSafety)
	if input.Run.TaskSpec.SchedulePlan != nil {
		fmt.Fprintf(builder, "Planned schedule entries:\n")
		for _, entry := range input.Run.TaskSpec.SchedulePlan.Entries {
			fmt.Fprintf(builder, "- At %s: %s\n", strings.TrimSpace(entry.ScheduledFor), strings.TrimSpace(entry.Prompt))
		}
	}
	appendArtifactHighlights(builder, input.Artifacts, 6)
	appendEvidenceHighlights(builder, input.Evidence, 10)
	appendRecentActivityMetrics(builder, input.RecentActivity)

	return Bundle{
		System: strings.TrimSpace(`
You are the scheduler phase for a WTL GAN-policy based assistant.
Finalize the deferred execution prompts using concrete details already discovered during the run.
Resolve template placeholders into specific names, phone numbers, URLs, or other facts when the evidence supports them.
Honor automation safety policy context. For high-risk browser engagement, avoid fixed short follow-up loops and prefer safer spacing when risk indicators are elevated.
Return one strict JSON object and nothing else.
Do not wrap the JSON in markdown fences.
The JSON object must contain exactly these keys:
- entries
Each entry object must contain exactly:
- scheduled_for
- prompt
Use RFC3339 timestamps for scheduled_for whenever you can resolve them precisely.
When exact follow-up timing is not required and an irregular cooldown window is safer, you may use randexp(min,max) (for example randexp(45m,3h)).
For randexp(min,max), both min and max must be positive durations and max must be greater than min.`),
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

func inferAutomationSafetyProfile(input PlannerInput, output PlannerOutput) assistant.AutomationSafetyProfile {
	request := strings.ToLower(strings.TrimSpace(input.UserRequestRaw + " " + output.Goal + " " + strings.Join(output.Deliverables, " ")))
	browserUsed := usesAgentBrowser(output.ToolsAllowed, output.ToolsRequired)

	if !browserUsed {
		return assistant.AutomationSafetyProfileNone
	}

	highRiskTokens := []string{
		"growth", "outreach", "public comment", "public comments", "reply", "replies", "follow", "follows",
		"connection request", "connection requests", "marketplace inquiry", "marketplace inquiries",
		"community engagement", "recruiting", "networking", "like", "likes", "endorse", "endorsement",
		"dm", "invite", "invites", "application submission", "apply",
	}
	mutationTokens := []string{
		"post", "publish", "submit", "send", "message", "messages", "comment", "reply", "follow",
		"connect", "like", "endorse", "update preference", "save preference", "application",
	}

	riskFlags := make([]string, 0, len(output.RiskFlags))
	for _, flag := range output.RiskFlags {
		riskFlags = append(riskFlags, strings.ToLower(strings.TrimSpace(flag)))
	}

	inferred := assistant.AutomationSafetyProfileBrowserReadOnly
	if containsAnyToken(request, highRiskTokens...) {
		inferred = assistant.AutomationSafetyProfileBrowserHighRiskEngagement
	} else if containsAnyToken(request, mutationTokens...) || containsAnyToken(strings.Join(riskFlags, " "), "external-side-effect") {
		inferred = assistant.AutomationSafetyProfileBrowserMutating
	}

	if output.AutomationSafety != nil && output.AutomationSafety.Profile != "" {
		inferred = maxAutomationSafetyProfile(inferred, output.AutomationSafety.Profile)
	}

	return inferred
}

func usesAgentBrowser(toolsAllowed, toolsRequired []string) bool {
	for _, tool := range toolsAllowed {
		if strings.EqualFold(strings.TrimSpace(tool), "agent-browser") {
			return true
		}
	}
	for _, tool := range toolsRequired {
		if strings.EqualFold(strings.TrimSpace(tool), "agent-browser") {
			return true
		}
	}
	return false
}

func containsAnyToken(value string, fragments ...string) bool {
	for _, fragment := range fragments {
		if strings.Contains(value, strings.ToLower(strings.TrimSpace(fragment))) {
			return true
		}
	}
	return false
}

func maxAutomationSafetyProfile(left, right assistant.AutomationSafetyProfile) assistant.AutomationSafetyProfile {
	if automationSafetyProfileRank(right) > automationSafetyProfileRank(left) {
		return right
	}
	return left
}

func automationSafetyProfileRank(profile assistant.AutomationSafetyProfile) int {
	switch profile {
	case assistant.AutomationSafetyProfileNone:
		return 0
	case assistant.AutomationSafetyProfileBrowserReadOnly:
		return 1
	case assistant.AutomationSafetyProfileBrowserMutating:
		return 2
	case assistant.AutomationSafetyProfileBrowserHighRiskEngagement:
		return 3
	default:
		return -1
	}
}

func projectPlannerContext(project assistant.ProjectContext, wiki assistant.WikiContext, safety config.AutomationSafetyConfig) string {
	builder := &strings.Builder{}
	if strings.TrimSpace(project.Slug) == "" {
		fmt.Fprintf(builder, "Project context:\n- No project selected yet.")
	} else {
		fmt.Fprintf(
			builder,
			"Project context:\n- Slug: %s\n- Name: %s\n- Purpose: %s\n- Workspace: %s",
			project.Slug,
			firstNonEmpty(project.Name, project.Slug),
			project.Description,
			project.WorkspaceDir,
		)
		if strings.TrimSpace(project.WikiDir) != "" {
			fmt.Fprintf(builder, "\n- Wiki: %s", project.WikiDir)
		}
	}

	appendWikiSummary(builder, wiki)
	appendAutomationSafetyPlannerContext(builder, project.Slug, safety)
	return builder.String()
}

func appendAutomationSafetyPlannerContext(builder *strings.Builder, projectSlug string, safety config.AutomationSafetyConfig) {
	fmt.Fprintf(builder, "\n\nAutomation safety config context:\n")
	if len(safety.Defaults) == 0 {
		fmt.Fprintf(builder, "- Global defaults: none configured.\n")
	} else {
		profiles := make([]string, 0, len(safety.Defaults))
		for profile := range safety.Defaults {
			profiles = append(profiles, profile)
		}
		fmt.Fprintf(builder, "- Global defaults configured for profiles: %s.\n", strings.Join(profiles, ", "))
	}

	slug := strings.TrimSpace(projectSlug)
	if slug == "" {
		fmt.Fprintf(builder, "- Project override: unavailable (project slug not set).")
		return
	}
	if override, ok := safety.Projects[slug]; ok {
		fmt.Fprintf(builder, "- Project override for %s: configured.", slug)
		if override.ProfileOverride != "" {
			fmt.Fprintf(builder, " profile_override=%s.", override.ProfileOverride)
		}
		return
	}
	fmt.Fprintf(builder, "- Project override for %s: none configured.", slug)
}

func appendAutomationSafetyPromptContext(builder *strings.Builder, policy *assistant.AutomationSafetyPolicy) {
	fmt.Fprintf(builder, "Automation safety policy context:\n")
	if policy == nil {
		fmt.Fprintf(builder, "- none.\n")
		return
	}

	fmt.Fprintf(builder, "- profile: %s\n", policy.Profile)
	fmt.Fprintf(builder, "- enforcement: %s\n", policy.Enforcement)

	if len(policy.ModePolicy.AllowedSessionModes) > 0 {
		fmt.Fprintf(builder, "- allowed session modes: %s\n", strings.Join(policy.ModePolicy.AllowedSessionModes, ", "))
	}
	fmt.Fprintf(builder, "- allow no-action success: %t\n", policy.ModePolicy.AllowNoActionSuccess)
	fmt.Fprintf(builder, "- require no-action evidence: %t\n", policy.ModePolicy.RequireNoActionEvidence)
	if len(policy.ModePolicy.NoActionEvidenceRequired) > 0 {
		fmt.Fprintf(builder, "- no-action evidence requirements: %s\n", strings.Join(policy.ModePolicy.NoActionEvidenceRequired, "; "))
	}

	rateLimits := make([]string, 0, 3)
	if policy.RateLimits.MaxAccountChangingActionsPerRun > 0 {
		rateLimits = append(
			rateLimits,
			fmt.Sprintf("max_account_changing_actions_per_run=%d", policy.RateLimits.MaxAccountChangingActionsPerRun),
		)
	}
	if policy.RateLimits.MaxRepliesPer24h > 0 {
		rateLimits = append(rateLimits, fmt.Sprintf("max_replies_per_24h=%d", policy.RateLimits.MaxRepliesPer24h))
	}
	if policy.RateLimits.MinSpacingMinutes > 0 {
		rateLimits = append(rateLimits, fmt.Sprintf("min_spacing_minutes=%d", policy.RateLimits.MinSpacingMinutes))
	}
	if len(rateLimits) > 0 {
		fmt.Fprintf(builder, "- rate limits: %s\n", strings.Join(rateLimits, ", "))
	}

	patternRules := make([]string, 0, 3)
	if policy.PatternRules.DisallowDefaultActionTrios {
		patternRules = append(patternRules, "disallow_default_action_trios")
	}
	if policy.PatternRules.DisallowFixedShortFollowup {
		patternRules = append(patternRules, "disallow_fixed_short_followups")
	}
	if policy.PatternRules.RequireSourceDiversity {
		patternRules = append(patternRules, "require_source_diversity")
	}
	if len(patternRules) > 0 {
		fmt.Fprintf(builder, "- pattern rules: %s\n", strings.Join(patternRules, ", "))
	}

	textReuse := make([]string, 0, 3)
	if policy.TextReuse.RejectHighSimilarity {
		textReuse = append(textReuse, "reject_high_similarity")
	}
	if policy.TextReuse.AvoidRepeatedSelfIntro {
		textReuse = append(textReuse, "avoid_repeated_self_intro")
	}
	if policy.TextReuse.RequireTextVariantSupport {
		textReuse = append(textReuse, "require_text_variant_support")
	}
	if len(textReuse) > 0 {
		fmt.Fprintf(builder, "- text reuse policy: %s\n", strings.Join(textReuse, ", "))
	}

	cooldown := make([]string, 0, 2)
	if policy.CooldownPolicy.ForceReadOnlyAfterDenseActivity {
		cooldown = append(cooldown, "force_read_only_after_dense_activity")
	}
	if policy.CooldownPolicy.PreferLongerCooldownAfterBlockedRuns {
		cooldown = append(cooldown, "prefer_longer_cooldown_after_blocked_runs")
	}
	if len(cooldown) > 0 {
		fmt.Fprintf(builder, "- cooldown policy: %s\n", strings.Join(cooldown, ", "))
	}
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
	if strings.TrimSpace(parent.Project.Slug) != "" {
		fmt.Fprintf(builder, "Parent project: %s\n", strings.TrimSpace(parent.Project.Slug))
	}
	if parent.Wiki.Enabled {
		fmt.Fprintf(builder, "Parent project wiki is available.\n")
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

func appendWikiContext(builder *strings.Builder, project assistant.ProjectContext, parent *ParentRunContext) {
	if strings.TrimSpace(project.Slug) != "" && project.Slug != "no_project" {
		fmt.Fprintf(builder, "Current project: %s\n", project.Slug)
	}
	if parent != nil && parent.Wiki.Enabled {
		fmt.Fprintf(builder, "Parent wiki context follows.\n")
		appendWikiSummary(builder, parent.Wiki)
		return
	}
	fmt.Fprintf(builder, "Project wiki context: none.\n")
}

func appendWikiSummary(builder *strings.Builder, wiki assistant.WikiContext) {
	if !wiki.Enabled {
		return
	}
	fmt.Fprintf(builder, "\nProject wiki context:\n")
	if summary := strings.TrimSpace(wiki.OverviewSummary); summary != "" {
		fmt.Fprintf(builder, "- Overview: %s\n", summary)
	}
	if summary := strings.TrimSpace(wiki.IndexSummary); summary != "" {
		fmt.Fprintf(builder, "- Index: %s\n", summary)
	}
	if summary := strings.TrimSpace(wiki.OpenQuestionsSummary); summary != "" {
		fmt.Fprintf(builder, "- Open questions: %s\n", summary)
	}
	if len(wiki.RecentLogEntries) > 0 {
		fmt.Fprintf(builder, "- Recent log entries:\n")
		for _, entry := range wiki.RecentLogEntries {
			fmt.Fprintf(builder, "  - %s\n", entry)
		}
	}
	if len(wiki.RelevantPages) > 0 {
		fmt.Fprintf(builder, "- Relevant pages:\n")
		for _, page := range wiki.RelevantPages {
			fmt.Fprintf(builder, "  - [%s] %s (%s)\n", firstNonEmpty(page.PageType, "page"), page.Title, page.Path)
		}
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

func appendRecentActivityMetrics(builder *strings.Builder, metrics *assistant.BrowserRecentActivityMetrics) {
	if metrics == nil {
		fmt.Fprintf(builder, "Recent browser activity metrics: unavailable.\n")
		return
	}
	fmt.Fprintf(builder, "Recent browser activity metrics:\n")
	fmt.Fprintf(builder, "- window: %s to %s\n", metrics.WindowStart.Format(time.RFC3339), metrics.WindowEnd.Format(time.RFC3339))
	fmt.Fprintf(builder, "- total_action_count=%d\n", metrics.TotalActionCount)
	fmt.Fprintf(builder, "- mutating_action_count=%d\n", metrics.MutatingActionCount)
	fmt.Fprintf(builder, "- reply_action_count=%d\n", metrics.ReplyActionCount)
	fmt.Fprintf(builder, "- recent_mutation_density=%.4f\n", metrics.RecentMutationDensity)
	fmt.Fprintf(builder, "- source_path_concentration=%.4f\n", metrics.SourcePathConcentration)
	fmt.Fprintf(builder, "- repeated_action_sequence_score=%.4f\n", metrics.RepeatedActionSequenceScore)
	fmt.Fprintf(builder, "- text_reuse_risk_score=%.4f\n", metrics.TextReuseRiskScore)
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
