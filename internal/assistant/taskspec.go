package assistant

import (
	"errors"
	"fmt"
	"slices"
	"strings"
)

var (
	defaultToolsAllowed     = []string{"agent-browser", "codex-app-server"}
	defaultToolsRequired    = []string{"agent-browser"}
	defaultEvidenceRequired = []string{
		"Relevant page titles, URLs, or source references for each material claim.",
		"Concrete artifacts produced during the run, such as summaries, tables, or documents.",
		"Evidence that the final output satisfies the user request and done definition.",
	}
	defaultHighRiskMaxAccountChangingActionsPerRun = 2
	defaultHighRiskMaxRepliesPer24h                = 12
	defaultHighRiskMinSpacingMinutes               = 20
	defaultHighRiskNoActionEvidenceRequired        = []string{
		"What was observed that made action risky.",
		"What account-changing action was skipped.",
		"Why the action was skipped for safety.",
		"What safer follow-up step should happen next.",
	}
)

type TaskSpecDraft struct {
	Goal                  string                  `json:"goal"`
	UserRequestRaw        string                  `json:"user_request_raw"`
	Deliverables          []string                `json:"deliverables"`
	Constraints           []string                `json:"constraints"`
	ToolsAllowed          []string                `json:"tools_allowed"`
	ToolsRequired         []string                `json:"tools_required"`
	DoneDefinition        []string                `json:"done_definition"`
	EvidenceRequired      []string                `json:"evidence_required"`
	RiskFlags             []string                `json:"risk_flags"`
	AutomationSafety      *AutomationSafetyPolicy `json:"automation_safety,omitempty"`
	MaxGenerationAttempts int                     `json:"max_generation_attempts"`
	SchedulePlan          *SchedulePlan           `json:"schedule_plan,omitempty"`
}

type AcceptanceContractDraft struct {
	Status             ContractStatus `json:"status"`
	Summary            string         `json:"summary"`
	Deliverables       []string       `json:"deliverables"`
	AcceptanceCriteria []string       `json:"acceptance_criteria"`
	EvidenceRequired   []string       `json:"evidence_required"`
	Constraints        []string       `json:"constraints"`
	OutOfScope         []string       `json:"out_of_scope"`
	RevisionNotes      string         `json:"revision_notes"`
}

func NewDefaultTaskSpec(userRequest string, maxGenerationAttempts int) TaskSpec {
	spec, err := NormalizeTaskSpec(TaskSpecDraft{
		UserRequestRaw:        userRequest,
		MaxGenerationAttempts: maxGenerationAttempts,
	}, userRequest, maxGenerationAttempts)
	if err != nil {
		panic(fmt.Errorf("assistant: normalize default task spec: %w", err))
	}
	return spec
}

func NormalizeTaskSpec(draft TaskSpecDraft, fallbackUserRequest string, defaultMaxGenerationAttempts int) (TaskSpec, error) {
	userRequest := strings.TrimSpace(firstNonEmpty(draft.UserRequestRaw, fallbackUserRequest))
	if userRequest == "" {
		return TaskSpec{}, errors.New("task spec user request is required")
	}

	maxAttempts := draft.MaxGenerationAttempts
	if maxAttempts <= 0 {
		maxAttempts = defaultMaxGenerationAttempts
	}
	if maxAttempts <= 0 {
		maxAttempts = 3
	}

	goal := strings.TrimSpace(draft.Goal)
	if goal == "" {
		goal = userRequest
	}

	deliverables := cleanList(draft.Deliverables)
	if len(deliverables) == 0 {
		deliverables = []string{fmt.Sprintf("Completed result for: %s", goal)}
	}

	toolsAllowed := cleanList(draft.ToolsAllowed)
	if len(toolsAllowed) == 0 {
		toolsAllowed = slices.Clone(defaultToolsAllowed)
	}

	toolsRequired := cleanList(draft.ToolsRequired)
	if len(toolsRequired) == 0 {
		toolsRequired = slices.Clone(defaultToolsRequired)
	}
	for _, required := range toolsRequired {
		if !slices.Contains(toolsAllowed, required) {
			toolsAllowed = append(toolsAllowed, required)
		}
	}

	doneDefinition := cleanList(draft.DoneDefinition)
	if len(doneDefinition) == 0 {
		for _, deliverable := range deliverables {
			doneDefinition = append(doneDefinition, fmt.Sprintf("Produce deliverable: %s", deliverable))
		}
		doneDefinition = append(doneDefinition, "Support all material claims with verifiable evidence collected during execution.")
	}

	evidenceRequired := cleanList(draft.EvidenceRequired)
	if len(evidenceRequired) == 0 {
		evidenceRequired = slices.Clone(defaultEvidenceRequired)
	}

	riskFlags := cleanList(draft.RiskFlags)
	riskFlags = append(riskFlags, detectRiskFlags(userRequest)...)
	riskFlags = cleanList(riskFlags)

	spec := TaskSpec{
		Goal:                  goal,
		UserRequestRaw:        userRequest,
		Deliverables:          deliverables,
		Constraints:           cleanList(draft.Constraints),
		ToolsAllowed:          toolsAllowed,
		ToolsRequired:         toolsRequired,
		DoneDefinition:        doneDefinition,
		EvidenceRequired:      evidenceRequired,
		RiskFlags:             riskFlags,
		AutomationSafety:      normalizeAutomationSafetyPolicy(draft.AutomationSafety),
		MaxGenerationAttempts: maxAttempts,
		SchedulePlan:          normalizeSchedulePlan(draft.SchedulePlan),
	}

	if err := spec.Validate(); err != nil {
		return TaskSpec{}, err
	}
	return spec, nil
}

func normalizeAutomationSafetyPolicy(policy *AutomationSafetyPolicy) *AutomationSafetyPolicy {
	if policy == nil {
		return nil
	}

	normalized := *policy
	if normalized.Profile == "" {
		normalized.Profile = AutomationSafetyProfileNone
	}
	normalized.ModePolicy.AllowedSessionModes = cleanList(normalized.ModePolicy.AllowedSessionModes)
	normalized.ModePolicy.NoActionEvidenceRequired = cleanList(normalized.ModePolicy.NoActionEvidenceRequired)

	if normalized.Enforcement == "" {
		switch normalized.Profile {
		case AutomationSafetyProfileBrowserReadOnly, AutomationSafetyProfileNone:
			normalized.Enforcement = AutomationSafetyEnforcementAdvisory
		case AutomationSafetyProfileBrowserMutating:
			normalized.Enforcement = AutomationSafetyEnforcementEvaluatorEnforced
		case AutomationSafetyProfileBrowserHighRiskEngagement:
			normalized.Enforcement = AutomationSafetyEnforcementEngineBlocking
		}
	}

	if normalized.Profile == AutomationSafetyProfileBrowserHighRiskEngagement {
		if normalized.RateLimits.MaxAccountChangingActionsPerRun == 0 {
			normalized.RateLimits.MaxAccountChangingActionsPerRun = defaultHighRiskMaxAccountChangingActionsPerRun
		}
		if normalized.RateLimits.MaxRepliesPer24h == 0 {
			normalized.RateLimits.MaxRepliesPer24h = defaultHighRiskMaxRepliesPer24h
		}
		if normalized.RateLimits.MinSpacingMinutes == 0 {
			normalized.RateLimits.MinSpacingMinutes = defaultHighRiskMinSpacingMinutes
		}
		if !normalized.ModePolicy.AllowNoActionSuccess {
			normalized.ModePolicy.AllowNoActionSuccess = true
		}
		if !normalized.ModePolicy.RequireNoActionEvidence {
			normalized.ModePolicy.RequireNoActionEvidence = true
		}
		if len(normalized.ModePolicy.NoActionEvidenceRequired) == 0 {
			normalized.ModePolicy.NoActionEvidenceRequired = slices.Clone(defaultHighRiskNoActionEvidenceRequired)
		}
	}

	return &normalized
}

func normalizeSchedulePlan(plan *SchedulePlan) *SchedulePlan {
	if plan == nil {
		return nil
	}
	entries := make([]ScheduleEntry, 0, len(plan.Entries))
	for _, entry := range plan.Entries {
		normalized := ScheduleEntry{
			ScheduledFor: strings.TrimSpace(entry.ScheduledFor),
			Prompt:       strings.TrimSpace(entry.Prompt),
		}
		if normalized.ScheduledFor == "" && normalized.Prompt == "" {
			continue
		}
		entries = append(entries, normalized)
	}
	if len(entries) == 0 {
		return nil
	}
	return &SchedulePlan{Entries: entries}
}

func NormalizeAcceptanceContract(draft AcceptanceContractDraft, spec TaskSpec) (AcceptanceContract, error) {
	status := draft.Status
	if status == "" {
		status = ContractStatusDraft
	}

	contract := AcceptanceContract{
		Status:             status,
		Summary:            strings.TrimSpace(draft.Summary),
		Deliverables:       cleanList(draft.Deliverables),
		AcceptanceCriteria: cleanList(draft.AcceptanceCriteria),
		EvidenceRequired:   cleanList(draft.EvidenceRequired),
		Constraints:        cleanList(draft.Constraints),
		OutOfScope:         cleanList(draft.OutOfScope),
		RevisionNotes:      strings.TrimSpace(draft.RevisionNotes),
	}

	if contract.Summary == "" {
		contract.Summary = fmt.Sprintf("Acceptance contract for: %s", spec.Goal)
	}
	if len(contract.Deliverables) == 0 {
		contract.Deliverables = slices.Clone(spec.Deliverables)
	}
	if len(contract.AcceptanceCriteria) == 0 {
		contract.AcceptanceCriteria = slices.Clone(spec.DoneDefinition)
	}
	if len(contract.EvidenceRequired) == 0 {
		contract.EvidenceRequired = slices.Clone(spec.EvidenceRequired)
	}
	if len(contract.Constraints) == 0 {
		contract.Constraints = slices.Clone(spec.Constraints)
	}

	if err := contract.Validate(); err != nil {
		return AcceptanceContract{}, err
	}
	return contract, nil
}

func cleanList(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	cleaned := make([]string, 0, len(values))
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed == "" {
			continue
		}
		if _, ok := seen[trimmed]; ok {
			continue
		}
		seen[trimmed] = struct{}{}
		cleaned = append(cleaned, trimmed)
	}
	return cleaned
}

func detectRiskFlags(userRequest string) []string {
	lower := strings.ToLower(userRequest)
	flags := make([]string, 0, 2)

	if containsAny(lower, "login", "log in", "sign in", "password", "credential", "secret", "token", "otp") {
		flags = append(flags, "authentication-required")
	}
	if containsAny(lower, "delete", "remove", "cancel", "archive", "publish", "send", "email", "message", "pay", "purchase", "refund", "approve", "submit", "update", "edit") {
		flags = append(flags, "external-side-effect")
	}

	return flags
}

func containsAny(value string, fragments ...string) bool {
	for _, fragment := range fragments {
		if strings.Contains(value, fragment) {
			return true
		}
	}
	return false
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}
