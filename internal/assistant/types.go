package assistant

import (
	"errors"
	"fmt"
	"strings"
	"time"
)

type RunStatus string

const (
	RunStatusQueued           RunStatus = "queued"
	RunStatusGating           RunStatus = "gating"
	RunStatusAnswering        RunStatus = "answering"
	RunStatusSelectingProject RunStatus = "selecting_project"
	RunStatusPlanning         RunStatus = "planning"
	RunStatusContracting      RunStatus = "contracting"
	RunStatusGenerating       RunStatus = "generating"
	RunStatusEvaluating       RunStatus = "evaluating"
	RunStatusScheduling       RunStatus = "scheduling"
	RunStatusWikiIngesting    RunStatus = "wiki_ingesting"
	RunStatusReporting        RunStatus = "reporting"
	RunStatusWaiting          RunStatus = "waiting"
	RunStatusCompleted        RunStatus = "completed"
	RunStatusFailed           RunStatus = "failed"
	RunStatusExhausted        RunStatus = "exhausted"
	RunStatusCancelled        RunStatus = "cancelled"
)

type RunPhase string

const (
	RunPhaseQueued           RunPhase = "queued"
	RunPhaseGating           RunPhase = "gating"
	RunPhaseAnswering        RunPhase = "answering"
	RunPhaseSelectingProject RunPhase = "selecting_project"
	RunPhasePlanning         RunPhase = "planning"
	RunPhaseContracting      RunPhase = "contracting"
	RunPhaseGenerating       RunPhase = "generating"
	RunPhaseEvaluating       RunPhase = "evaluating"
	RunPhaseScheduling       RunPhase = "scheduling"
	RunPhaseWikiIngesting    RunPhase = "wiki_ingesting"
	RunPhaseReporting        RunPhase = "reporting"
	RunPhaseWaiting          RunPhase = "waiting"
	RunPhaseCompleted        RunPhase = "completed"
	RunPhaseFailed           RunPhase = "failed"
	RunPhaseCancelled        RunPhase = "cancelled"
)

type AttemptRole string

const (
	AttemptRoleGate            AttemptRole = "gate"
	AttemptRoleAnswer          AttemptRole = "answer"
	AttemptRoleProjectSelector AttemptRole = "project_selector"
	AttemptRolePlanner         AttemptRole = "planner"
	AttemptRoleContractor      AttemptRole = "contractor"
	AttemptRoleGenerator       AttemptRole = "generator"
	AttemptRoleEvaluator       AttemptRole = "evaluator"
	AttemptRoleScheduler       AttemptRole = "scheduler"
	AttemptRoleWikiIngest      AttemptRole = "wiki_ingest"
	AttemptRoleReporter        AttemptRole = "reporter"
)

type RunRoute string

const (
	RunRouteWorkflow RunRoute = "workflow"
	RunRouteAnswer   RunRoute = "answer"
)

type ContractStatus string

const (
	ContractStatusDraft  ContractStatus = "draft"
	ContractStatusAgreed ContractStatus = "agreed"
)

type AutomationSafetyProfile string

const (
	AutomationSafetyProfileNone                      AutomationSafetyProfile = "none"
	AutomationSafetyProfileBrowserReadOnly           AutomationSafetyProfile = "browser_read_only"
	AutomationSafetyProfileBrowserMutating           AutomationSafetyProfile = "browser_mutating"
	AutomationSafetyProfileBrowserHighRiskEngagement AutomationSafetyProfile = "browser_high_risk_engagement"
)

type AutomationSafetyEnforcement string

const (
	AutomationSafetyEnforcementAdvisory          AutomationSafetyEnforcement = "advisory"
	AutomationSafetyEnforcementEvaluatorEnforced AutomationSafetyEnforcement = "evaluator_enforced"
	AutomationSafetyEnforcementEngineBlocking    AutomationSafetyEnforcement = "engine_blocking"
)

type AutomationSafetyModePolicy struct {
	AllowedSessionModes      []string `json:"allowed_session_modes,omitempty"`
	AllowNoActionSuccess     bool     `json:"allow_no_action_success,omitempty"`
	RequireNoActionEvidence  bool     `json:"require_no_action_evidence,omitempty"`
	NoActionEvidenceRequired []string `json:"no_action_evidence_required,omitempty"`
}

type AutomationSafetyRateLimits struct {
	MaxAccountChangingActionsPerRun int `json:"max_account_changing_actions_per_run,omitempty"`
	MaxRepliesPer24h                int `json:"max_replies_per_24h,omitempty"`
	MinSpacingMinutes               int `json:"min_spacing_minutes,omitempty"`
}

type AutomationSafetyPatternRules struct {
	DisallowDefaultActionTrios bool `json:"disallow_default_action_trios,omitempty"`
	DisallowFixedShortFollowup bool `json:"disallow_fixed_short_followups,omitempty"`
	RequireSourceDiversity     bool `json:"require_source_diversity,omitempty"`
}

type AutomationSafetyTextReusePolicy struct {
	RejectHighSimilarity      bool `json:"reject_high_similarity,omitempty"`
	AvoidRepeatedSelfIntro    bool `json:"avoid_repeated_self_intro,omitempty"`
	RequireTextVariantSupport bool `json:"require_text_variant_support,omitempty"`
}

type AutomationSafetyCooldownPolicy struct {
	ForceReadOnlyAfterDenseActivity      bool `json:"force_read_only_after_dense_activity,omitempty"`
	PreferLongerCooldownAfterBlockedRuns bool `json:"prefer_longer_cooldown_after_blocked_runs,omitempty"`
}

type AutomationSafetyPolicy struct {
	Profile        AutomationSafetyProfile         `json:"profile"`
	Enforcement    AutomationSafetyEnforcement     `json:"enforcement"`
	ModePolicy     AutomationSafetyModePolicy      `json:"mode_policy,omitempty"`
	RateLimits     AutomationSafetyRateLimits      `json:"rate_limits,omitempty"`
	PatternRules   AutomationSafetyPatternRules    `json:"pattern_rules,omitempty"`
	TextReuse      AutomationSafetyTextReusePolicy `json:"text_reuse_policy,omitempty"`
	CooldownPolicy AutomationSafetyCooldownPolicy  `json:"cooldown_policy,omitempty"`
}

type AcceptanceContract struct {
	Status             ContractStatus `json:"status"`
	Summary            string         `json:"summary"`
	Deliverables       []string       `json:"deliverables"`
	AcceptanceCriteria []string       `json:"acceptance_criteria"`
	EvidenceRequired   []string       `json:"evidence_required"`
	Constraints        []string       `json:"constraints"`
	OutOfScope         []string       `json:"out_of_scope"`
	RevisionNotes      string         `json:"revision_notes,omitempty"`
}

type ProjectContext struct {
	Slug              string `json:"slug"`
	Name              string `json:"name"`
	Description       string `json:"description"`
	WorkspaceDir      string `json:"workspace_dir"`
	WikiDir           string `json:"wiki_dir,omitempty"`
	BrowserProfileDir string `json:"browser_profile_dir,omitempty"`
	BrowserCDPPort    int    `json:"browser_cdp_port,omitempty"`
}

type WikiPageMeta struct {
	Path       string   `json:"path"`
	Title      string   `json:"title"`
	PageType   string   `json:"page_type"`
	UpdatedAt  string   `json:"updated_at,omitempty"`
	Status     string   `json:"status,omitempty"`
	Confidence string   `json:"confidence,omitempty"`
	SourceRefs []string `json:"source_refs,omitempty"`
	Related    []string `json:"related,omitempty"`
}

type WikiContext struct {
	Enabled              bool           `json:"enabled"`
	OverviewSummary      string         `json:"overview_summary,omitempty"`
	IndexSummary         string         `json:"index_summary,omitempty"`
	OpenQuestionsSummary string         `json:"open_questions_summary,omitempty"`
	RecentLogEntries     []string       `json:"recent_log_entries,omitempty"`
	RelevantPages        []WikiPageMeta `json:"relevant_pages,omitempty"`
}

type ArtifactKind string

const (
	ArtifactKindReport     ArtifactKind = "report"
	ArtifactKindTable      ArtifactKind = "table"
	ArtifactKindDocument   ArtifactKind = "document"
	ArtifactKindEvidence   ArtifactKind = "evidence"
	ArtifactKindExport     ArtifactKind = "export"
	ArtifactKindScreenshot ArtifactKind = "screenshot"
)

type EvidenceKind string

const (
	EvidenceKindToolCall    EvidenceKind = "tool_call"
	EvidenceKindWebStep     EvidenceKind = "web_step"
	EvidenceKindObservation EvidenceKind = "observation"
)

type WaitKind string

const (
	WaitKindApproval       WaitKind = "approval"
	WaitKindCredential     WaitKind = "credential"
	WaitKindClarification  WaitKind = "clarification"
	WaitKindAuthentication WaitKind = "authentication"
)

type EventType string

const (
	EventTypeRunCreated        EventType = "run_created"
	EventTypePhaseChanged      EventType = "phase_changed"
	EventTypeAttemptLogged     EventType = "attempt_logged"
	EventTypeWaiting           EventType = "waiting"
	EventTypeArtifactAdded     EventType = "artifact_added"
	EventTypeEvaluation        EventType = "evaluation_recorded"
	EventTypeReasoning         EventType = "reasoning"
	EventTypeToolCallStart     EventType = "tool_call_started"
	EventTypeToolCallEnd       EventType = "tool_call_completed"
	EventTypeScheduleCreated   EventType = "schedule_created"
	EventTypeScheduleTriggered EventType = "schedule_triggered"
	EventTypeScheduleFailed    EventType = "schedule_failed"
)

type Run struct {
	ID                    string         `json:"id"`
	ChatID                string         `json:"chat_id"`
	ParentRunID           string         `json:"parent_run_id,omitempty"`
	Status                RunStatus      `json:"status"`
	Phase                 RunPhase       `json:"phase"`
	GateRoute             RunRoute       `json:"gate_route,omitempty"`
	GateReason            string         `json:"gate_reason,omitempty"`
	GateDecidedAt         *time.Time     `json:"gate_decided_at,omitempty"`
	Project               ProjectContext `json:"project"`
	UserRequestRaw        string         `json:"user_request_raw"`
	TaskSpec              TaskSpec       `json:"task_spec"`
	AttemptCount          int            `json:"attempt_count"`
	MaxGenerationAttempts int            `json:"max_generation_attempts"`
	LatestEvaluation      *Evaluation    `json:"latest_evaluation,omitempty"`
	WaitingFor            *WaitRequest   `json:"waiting_for,omitempty"`
	CreatedAt             time.Time      `json:"created_at"`
	UpdatedAt             time.Time      `json:"updated_at"`
	CompletedAt           *time.Time     `json:"completed_at,omitempty"`
}

type Chat struct {
	ID          string    `json:"id"`
	RootRunID   string    `json:"root_run_id"`
	LatestRunID string    `json:"latest_run_id"`
	Title       string    `json:"title"`
	Status      RunStatus `json:"status"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

type TaskSpec struct {
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
	AcceptanceContract    *AcceptanceContract     `json:"acceptance_contract,omitempty"`
}

type Attempt struct {
	ID            string      `json:"id"`
	RunID         string      `json:"run_id"`
	Sequence      int         `json:"sequence"`
	Role          AttemptRole `json:"role"`
	InputSummary  string      `json:"input_summary"`
	OutputSummary string      `json:"output_summary"`
	Critique      string      `json:"critique,omitempty"`
	StartedAt     time.Time   `json:"started_at"`
	FinishedAt    *time.Time  `json:"finished_at,omitempty"`
}

type Evaluation struct {
	ID                     string    `json:"id"`
	RunID                  string    `json:"run_id"`
	AttemptID              string    `json:"attempt_id"`
	Passed                 bool      `json:"passed"`
	Score                  int       `json:"score"`
	Summary                string    `json:"summary"`
	MissingRequirements    []string  `json:"missing_requirements"`
	IncorrectClaims        []string  `json:"incorrect_claims"`
	EvidenceChecked        []string  `json:"evidence_checked"`
	NextActionForGenerator string    `json:"next_action_for_generator"`
	CreatedAt              time.Time `json:"created_at"`
}

type Artifact struct {
	ID        string       `json:"id"`
	RunID     string       `json:"run_id"`
	AttemptID string       `json:"attempt_id"`
	Kind      ArtifactKind `json:"kind"`
	Title     string       `json:"title"`
	MIMEType  string       `json:"mime_type"`
	Path      string       `json:"path,omitempty"`
	URL       string       `json:"url,omitempty"`
	Content   string       `json:"content,omitempty"`
	SourceURL string       `json:"source_url,omitempty"`
	CreatedAt time.Time    `json:"created_at"`
}

type Evidence struct {
	ID        string       `json:"id"`
	RunID     string       `json:"run_id"`
	AttemptID string       `json:"attempt_id"`
	Kind      EvidenceKind `json:"kind"`
	Summary   string       `json:"summary"`
	Detail    string       `json:"detail,omitempty"`
	CreatedAt time.Time    `json:"created_at"`
}

type ToolCall struct {
	ID            string    `json:"id"`
	RunID         string    `json:"run_id"`
	AttemptID     string    `json:"attempt_id"`
	ToolName      string    `json:"tool_name"`
	InputSummary  string    `json:"input_summary"`
	OutputSummary string    `json:"output_summary"`
	StartedAt     time.Time `json:"started_at"`
	FinishedAt    time.Time `json:"finished_at"`
}

type WebStep struct {
	ID         string    `json:"id"`
	RunID      string    `json:"run_id"`
	AttemptID  string    `json:"attempt_id"`
	Title      string    `json:"title"`
	URL        string    `json:"url,omitempty"`
	Summary    string    `json:"summary"`
	OccurredAt time.Time `json:"occurred_at"`
}

type WaitRequest struct {
	ID          string    `json:"id"`
	RunID       string    `json:"run_id"`
	Kind        WaitKind  `json:"kind"`
	Title       string    `json:"title"`
	Prompt      string    `json:"prompt"`
	RiskSummary string    `json:"risk_summary,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
}

type RunEvent struct {
	ID        string         `json:"id"`
	RunID     string         `json:"run_id"`
	Type      EventType      `json:"type"`
	Phase     RunPhase       `json:"phase,omitempty"`
	Summary   string         `json:"summary"`
	Data      map[string]any `json:"data,omitempty"`
	CreatedAt time.Time      `json:"created_at"`
}

func NewRun(userRequest string, now time.Time, maxGenerationAttempts int) Run {
	if maxGenerationAttempts <= 0 {
		maxGenerationAttempts = 3
	}
	id := NewID("run", now)
	chatID := NewChatID(now)
	return Run{
		ID:                    id,
		ChatID:                chatID,
		Status:                RunStatusQueued,
		Phase:                 RunPhaseQueued,
		UserRequestRaw:        userRequest,
		MaxGenerationAttempts: maxGenerationAttempts,
		TaskSpec:              NewDefaultTaskSpec(userRequest, maxGenerationAttempts),
		CreatedAt:             now.UTC(),
		UpdatedAt:             now.UTC(),
	}
}

func (r Run) Validate() error {
	switch {
	case r.ID == "":
		return errors.New("assistant: run id is required")
	case r.ChatID == "":
		return errors.New("assistant: chat id is required")
	case r.ParentRunID != "" && r.ParentRunID == r.ID:
		return errors.New("assistant: parent run id cannot match run id")
	case r.UserRequestRaw == "":
		return errors.New("assistant: user request is required")
	case r.MaxGenerationAttempts <= 0:
		return errors.New("assistant: max generation attempts must be positive")
	case r.CreatedAt.IsZero():
		return errors.New("assistant: created timestamp is required")
	case r.UpdatedAt.IsZero():
		return errors.New("assistant: updated timestamp is required")
	}
	switch r.GateRoute {
	case "", RunRouteWorkflow, RunRouteAnswer:
	default:
		return errors.New("assistant: gate route is invalid")
	}
	if r.GateRoute == "" && (r.GateReason != "" || r.GateDecidedAt != nil) {
		return errors.New("assistant: gate metadata requires gate route")
	}
	if err := r.TaskSpec.Validate(); err != nil {
		return fmt.Errorf("assistant: task spec invalid: %w", err)
	}
	return nil
}

func (s TaskSpec) Validate() error {
	switch {
	case s.Goal == "":
		return errors.New("task spec goal is required")
	case s.UserRequestRaw == "":
		return errors.New("task spec user request is required")
	case len(s.Deliverables) == 0:
		return errors.New("task spec deliverables are required")
	case len(s.ToolsAllowed) == 0:
		return errors.New("task spec tools allowed are required")
	case len(s.ToolsRequired) == 0:
		return errors.New("task spec tools required are required")
	case len(s.DoneDefinition) == 0:
		return errors.New("task spec done definition is required")
	case len(s.EvidenceRequired) == 0:
		return errors.New("task spec evidence requirements are required")
	case s.MaxGenerationAttempts <= 0:
		return errors.New("task spec max generation attempts must be positive")
	default:
		if s.AutomationSafety != nil {
			if err := s.AutomationSafety.Validate(); err != nil {
				return fmt.Errorf("task spec automation safety invalid: %w", err)
			}
		}
		if s.SchedulePlan != nil {
			if err := s.SchedulePlan.Validate(); err != nil {
				return fmt.Errorf("task spec schedule plan invalid: %w", err)
			}
		}
		if s.AcceptanceContract != nil {
			if err := s.AcceptanceContract.Validate(); err != nil {
				return fmt.Errorf("task spec acceptance contract invalid: %w", err)
			}
		}
		return nil
	}
}

func (p AutomationSafetyPolicy) Validate() error {
	switch p.Profile {
	case AutomationSafetyProfileNone,
		AutomationSafetyProfileBrowserReadOnly,
		AutomationSafetyProfileBrowserMutating,
		AutomationSafetyProfileBrowserHighRiskEngagement:
	default:
		return errors.New("automation safety profile is invalid")
	}

	switch p.Enforcement {
	case AutomationSafetyEnforcementAdvisory,
		AutomationSafetyEnforcementEvaluatorEnforced,
		AutomationSafetyEnforcementEngineBlocking:
	default:
		return errors.New("automation safety enforcement is invalid")
	}

	if p.Enforcement == AutomationSafetyEnforcementEngineBlocking &&
		p.Profile != AutomationSafetyProfileBrowserHighRiskEngagement {
		return errors.New("automation safety engine_blocking is only valid for browser_high_risk_engagement")
	}

	for _, mode := range p.ModePolicy.AllowedSessionModes {
		switch mode {
		case "read_only", "single_action", "reply_only":
		default:
			return fmt.Errorf("automation safety session mode %q is invalid", mode)
		}
	}

	for _, requirement := range p.ModePolicy.NoActionEvidenceRequired {
		if strings.TrimSpace(requirement) == "" {
			return errors.New("automation safety no-action evidence requirement must be non-empty")
		}
	}
	if p.ModePolicy.RequireNoActionEvidence && len(p.ModePolicy.NoActionEvidenceRequired) == 0 {
		return errors.New("automation safety no-action evidence requirements are required when require_no_action_evidence is true")
	}

	if p.RateLimits.MaxAccountChangingActionsPerRun < 0 ||
		p.RateLimits.MaxRepliesPer24h < 0 ||
		p.RateLimits.MinSpacingMinutes < 0 {
		return errors.New("automation safety rate limits cannot be negative")
	}

	if p.Profile == AutomationSafetyProfileBrowserHighRiskEngagement {
		if p.RateLimits.MaxAccountChangingActionsPerRun == 0 {
			return errors.New("automation safety max_account_changing_actions_per_run is required for high-risk engagement")
		}
		if p.RateLimits.MaxRepliesPer24h == 0 {
			return errors.New("automation safety max_replies_per_24h is required for high-risk engagement")
		}
		if p.RateLimits.MinSpacingMinutes == 0 {
			return errors.New("automation safety min_spacing_minutes is required for high-risk engagement")
		}
	}

	return nil
}

func (c AcceptanceContract) Validate() error {
	switch c.Status {
	case ContractStatusDraft, ContractStatusAgreed:
	default:
		return errors.New("acceptance contract status is required")
	}
	switch {
	case c.Summary == "":
		return errors.New("acceptance contract summary is required")
	case len(c.Deliverables) == 0:
		return errors.New("acceptance contract deliverables are required")
	case len(c.AcceptanceCriteria) == 0:
		return errors.New("acceptance contract acceptance criteria are required")
	case len(c.EvidenceRequired) == 0:
		return errors.New("acceptance contract evidence requirements are required")
	default:
		return nil
	}
}

func (s TaskSpec) HasAcceptedContract() bool {
	return s.AcceptanceContract != nil && s.AcceptanceContract.Status == ContractStatusAgreed
}

func (e Evaluation) Validate() error {
	switch {
	case e.RunID == "":
		return errors.New("evaluation run id is required")
	case e.AttemptID == "":
		return errors.New("evaluation attempt id is required")
	case e.Score < 0 || e.Score > 100:
		return errors.New("evaluation score must be between 0 and 100")
	case e.Summary == "":
		return errors.New("evaluation summary is required")
	case e.CreatedAt.IsZero():
		return errors.New("evaluation created timestamp is required")
	default:
		return nil
	}
}

func AllRunStatuses() []RunStatus {
	return []RunStatus{
		RunStatusQueued,
		RunStatusGating,
		RunStatusAnswering,
		RunStatusSelectingProject,
		RunStatusPlanning,
		RunStatusContracting,
		RunStatusGenerating,
		RunStatusEvaluating,
		RunStatusScheduling,
		RunStatusWikiIngesting,
		RunStatusReporting,
		RunStatusWaiting,
		RunStatusCompleted,
		RunStatusFailed,
		RunStatusExhausted,
		RunStatusCancelled,
	}
}

func AllRunPhases() []RunPhase {
	return []RunPhase{
		RunPhaseQueued,
		RunPhaseGating,
		RunPhaseAnswering,
		RunPhaseSelectingProject,
		RunPhasePlanning,
		RunPhaseContracting,
		RunPhaseGenerating,
		RunPhaseEvaluating,
		RunPhaseScheduling,
		RunPhaseWikiIngesting,
		RunPhaseReporting,
		RunPhaseWaiting,
		RunPhaseCompleted,
		RunPhaseFailed,
		RunPhaseCancelled,
	}
}
