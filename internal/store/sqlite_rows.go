package store

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/siisee11/CodexVirtualAssistant/internal/assistant"
)

type runRow struct {
	ID                    string `json:"id"`
	ChatID                string `json:"chat_id"`
	ParentRunID           string `json:"parent_run_id"`
	Status                string `json:"status"`
	Phase                 string `json:"phase"`
	GateRoute             string `json:"gate_route"`
	GateReason            string `json:"gate_reason"`
	GateDecidedAt         string `json:"gate_decided_at"`
	ProjectJSON           string `json:"project_json"`
	UserRequestRaw        string `json:"user_request_raw"`
	TaskSpecJSON          string `json:"task_spec_json"`
	AttemptCount          int    `json:"attempt_count"`
	MaxGenerationAttempts int    `json:"max_generation_attempts"`
	CreatedAt             string `json:"created_at"`
	UpdatedAt             string `json:"updated_at"`
	CompletedAt           string `json:"completed_at"`
}

type runEventRow struct {
	ID        string `json:"id"`
	RunID     string `json:"run_id"`
	Type      string `json:"type"`
	Phase     string `json:"phase"`
	Summary   string `json:"summary"`
	CreatedAt string `json:"created_at"`
}

type attemptRow struct {
	ID            string `json:"id"`
	RunID         string `json:"run_id"`
	Sequence      int    `json:"sequence"`
	Role          string `json:"role"`
	InputSummary  string `json:"input_summary"`
	OutputSummary string `json:"output_summary"`
	Critique      string `json:"critique"`
	StartedAt     string `json:"started_at"`
	FinishedAt    string `json:"finished_at"`
}

type artifactRow struct {
	ID        string `json:"id"`
	RunID     string `json:"run_id"`
	AttemptID string `json:"attempt_id"`
	Kind      string `json:"kind"`
	Title     string `json:"title"`
	MIMEType  string `json:"mime_type"`
	Path      string `json:"path"`
	Content   string `json:"content"`
	SourceURL string `json:"source_url"`
	CreatedAt string `json:"created_at"`
}

type evidenceRow struct {
	ID        string `json:"id"`
	RunID     string `json:"run_id"`
	AttemptID string `json:"attempt_id"`
	Kind      string `json:"kind"`
	Summary   string `json:"summary"`
	Detail    string `json:"detail"`
	CreatedAt string `json:"created_at"`
}

type evaluationRow struct {
	ID                      string `json:"id"`
	RunID                   string `json:"run_id"`
	AttemptID               string `json:"attempt_id"`
	Passed                  int    `json:"passed"`
	Score                   int    `json:"score"`
	Summary                 string `json:"summary"`
	MissingRequirementsJSON string `json:"missing_requirements_json"`
	IncorrectClaimsJSON     string `json:"incorrect_claims_json"`
	EvidenceCheckedJSON     string `json:"evidence_checked_json"`
	NextActionForGenerator  string `json:"next_action_for_generator"`
	CreatedAt               string `json:"created_at"`
}

type toolCallRow struct {
	ID            string `json:"id"`
	RunID         string `json:"run_id"`
	AttemptID     string `json:"attempt_id"`
	ToolName      string `json:"tool_name"`
	InputSummary  string `json:"input_summary"`
	OutputSummary string `json:"output_summary"`
	StartedAt     string `json:"started_at"`
	FinishedAt    string `json:"finished_at"`
}

type webStepRow struct {
	ID         string `json:"id"`
	RunID      string `json:"run_id"`
	AttemptID  string `json:"attempt_id"`
	Title      string `json:"title"`
	URL        string `json:"url"`
	Summary    string `json:"summary"`
	OccurredAt string `json:"occurred_at"`
}

type waitRequestRow struct {
	ID          string `json:"id"`
	RunID       string `json:"run_id"`
	Kind        string `json:"kind"`
	Title       string `json:"title"`
	Prompt      string `json:"prompt"`
	RiskSummary string `json:"risk_summary"`
	CreatedAt   string `json:"created_at"`
}

type scheduledRunRow struct {
	ID                    string `json:"id"`
	ChatID                string `json:"chat_id"`
	ParentRunID           string `json:"parent_run_id"`
	UserRequestRaw        string `json:"user_request_raw"`
	MaxGenerationAttempts int    `json:"max_generation_attempts"`
	ScheduledFor          string `json:"scheduled_for"`
	Status                string `json:"status"`
	RunID                 string `json:"run_id"`
	ErrorMessage          string `json:"error_message"`
	CreatedAt             string `json:"created_at"`
	TriggeredAt           string `json:"triggered_at"`
}

func (r runRow) toAssistantRun() (assistant.Run, error) {
	createdAt, err := parseTime(r.CreatedAt)
	if err != nil {
		return assistant.Run{}, err
	}
	updatedAt, err := parseTime(r.UpdatedAt)
	if err != nil {
		return assistant.Run{}, err
	}

	var taskSpec assistant.TaskSpec
	if err := json.Unmarshal([]byte(r.TaskSpecJSON), &taskSpec); err != nil {
		return assistant.Run{}, fmt.Errorf("decode task spec: %w", err)
	}
	var project assistant.ProjectContext
	if strings.TrimSpace(r.ProjectJSON) != "" {
		if err := json.Unmarshal([]byte(r.ProjectJSON), &project); err != nil {
			return assistant.Run{}, fmt.Errorf("decode project context: %w", err)
		}
	}

	run := assistant.Run{
		ID:                    r.ID,
		ChatID:                strings.TrimSpace(r.ChatID),
		ParentRunID:           strings.TrimSpace(r.ParentRunID),
		Status:                assistant.RunStatus(r.Status),
		Phase:                 assistant.RunPhase(r.Phase),
		GateRoute:             assistant.RunRoute(strings.TrimSpace(r.GateRoute)),
		GateReason:            strings.TrimSpace(r.GateReason),
		Project:               project,
		UserRequestRaw:        r.UserRequestRaw,
		TaskSpec:              taskSpec,
		AttemptCount:          r.AttemptCount,
		MaxGenerationAttempts: r.MaxGenerationAttempts,
		CreatedAt:             createdAt,
		UpdatedAt:             updatedAt,
	}
	if run.ChatID == "" {
		run.ChatID = run.ID
	}
	if strings.TrimSpace(r.CompletedAt) != "" {
		completedAt, err := parseTime(r.CompletedAt)
		if err != nil {
			return assistant.Run{}, err
		}
		run.CompletedAt = &completedAt
	}
	if strings.TrimSpace(r.GateDecidedAt) != "" {
		gateDecidedAt, err := parseTime(r.GateDecidedAt)
		if err != nil {
			return assistant.Run{}, err
		}
		run.GateDecidedAt = &gateDecidedAt
	}
	return run, nil
}

func (r runEventRow) toAssistantRunEvent() (assistant.RunEvent, error) {
	createdAt, err := parseTime(r.CreatedAt)
	if err != nil {
		return assistant.RunEvent{}, err
	}
	return assistant.RunEvent{
		ID:        r.ID,
		RunID:     r.RunID,
		Type:      assistant.EventType(r.Type),
		Phase:     assistant.RunPhase(r.Phase),
		Summary:   r.Summary,
		CreatedAt: createdAt,
	}, nil
}

func (r attemptRow) toAssistantAttempt() (assistant.Attempt, error) {
	startedAt, err := parseTime(r.StartedAt)
	if err != nil {
		return assistant.Attempt{}, err
	}
	attempt := assistant.Attempt{
		ID:            r.ID,
		RunID:         r.RunID,
		Sequence:      r.Sequence,
		Role:          assistant.AttemptRole(r.Role),
		InputSummary:  r.InputSummary,
		OutputSummary: r.OutputSummary,
		Critique:      r.Critique,
		StartedAt:     startedAt,
	}
	if strings.TrimSpace(r.FinishedAt) != "" {
		finishedAt, err := parseTime(r.FinishedAt)
		if err != nil {
			return assistant.Attempt{}, err
		}
		attempt.FinishedAt = &finishedAt
	}
	return attempt, nil
}

func (r artifactRow) toAssistantArtifact() (assistant.Artifact, error) {
	createdAt, err := parseTime(r.CreatedAt)
	if err != nil {
		return assistant.Artifact{}, err
	}
	return assistant.Artifact{
		ID:        r.ID,
		RunID:     r.RunID,
		AttemptID: r.AttemptID,
		Kind:      assistant.ArtifactKind(r.Kind),
		Title:     r.Title,
		MIMEType:  r.MIMEType,
		Path:      r.Path,
		Content:   r.Content,
		SourceURL: r.SourceURL,
		CreatedAt: createdAt,
	}, nil
}

func (r evidenceRow) toAssistantEvidence() (assistant.Evidence, error) {
	createdAt, err := parseTime(r.CreatedAt)
	if err != nil {
		return assistant.Evidence{}, err
	}
	return assistant.Evidence{
		ID:        r.ID,
		RunID:     r.RunID,
		AttemptID: r.AttemptID,
		Kind:      assistant.EvidenceKind(r.Kind),
		Summary:   r.Summary,
		Detail:    r.Detail,
		CreatedAt: createdAt,
	}, nil
}

func (r evaluationRow) toAssistantEvaluation() (assistant.Evaluation, error) {
	createdAt, err := parseTime(r.CreatedAt)
	if err != nil {
		return assistant.Evaluation{}, err
	}

	var missingRequirements []string
	if err := json.Unmarshal([]byte(r.MissingRequirementsJSON), &missingRequirements); err != nil {
		return assistant.Evaluation{}, fmt.Errorf("decode missing requirements: %w", err)
	}
	var incorrectClaims []string
	if err := json.Unmarshal([]byte(r.IncorrectClaimsJSON), &incorrectClaims); err != nil {
		return assistant.Evaluation{}, fmt.Errorf("decode incorrect claims: %w", err)
	}
	var evidenceChecked []string
	if err := json.Unmarshal([]byte(r.EvidenceCheckedJSON), &evidenceChecked); err != nil {
		return assistant.Evaluation{}, fmt.Errorf("decode evidence checked: %w", err)
	}

	return assistant.Evaluation{
		ID:                     r.ID,
		RunID:                  r.RunID,
		AttemptID:              r.AttemptID,
		Passed:                 r.Passed == 1,
		Score:                  r.Score,
		Summary:                r.Summary,
		MissingRequirements:    missingRequirements,
		IncorrectClaims:        incorrectClaims,
		EvidenceChecked:        evidenceChecked,
		NextActionForGenerator: r.NextActionForGenerator,
		CreatedAt:              createdAt,
	}, nil
}

func (r toolCallRow) toAssistantToolCall() (assistant.ToolCall, error) {
	startedAt, err := parseTime(r.StartedAt)
	if err != nil {
		return assistant.ToolCall{}, err
	}
	finishedAt, err := parseTime(r.FinishedAt)
	if err != nil {
		return assistant.ToolCall{}, err
	}
	return assistant.ToolCall{
		ID:            r.ID,
		RunID:         r.RunID,
		AttemptID:     r.AttemptID,
		ToolName:      r.ToolName,
		InputSummary:  r.InputSummary,
		OutputSummary: r.OutputSummary,
		StartedAt:     startedAt,
		FinishedAt:    finishedAt,
	}, nil
}

func (r webStepRow) toAssistantWebStep() (assistant.WebStep, error) {
	occurredAt, err := parseTime(r.OccurredAt)
	if err != nil {
		return assistant.WebStep{}, err
	}
	return assistant.WebStep{
		ID:         r.ID,
		RunID:      r.RunID,
		AttemptID:  r.AttemptID,
		Title:      r.Title,
		URL:        r.URL,
		Summary:    r.Summary,
		OccurredAt: occurredAt,
	}, nil
}

func (r waitRequestRow) toAssistantWaitRequest() (assistant.WaitRequest, error) {
	createdAt, err := parseTime(r.CreatedAt)
	if err != nil {
		return assistant.WaitRequest{}, err
	}
	return assistant.WaitRequest{
		ID:          r.ID,
		RunID:       r.RunID,
		Kind:        assistant.WaitKind(r.Kind),
		Title:       r.Title,
		Prompt:      r.Prompt,
		RiskSummary: r.RiskSummary,
		CreatedAt:   createdAt,
	}, nil
}

func (r scheduledRunRow) toAssistantScheduledRun() (assistant.ScheduledRun, error) {
	scheduledFor, err := parseTime(r.ScheduledFor)
	if err != nil {
		return assistant.ScheduledRun{}, err
	}
	createdAt, err := parseTime(r.CreatedAt)
	if err != nil {
		return assistant.ScheduledRun{}, err
	}
	scheduledRun := assistant.ScheduledRun{
		ID:                    r.ID,
		ChatID:                r.ChatID,
		ParentRunID:           r.ParentRunID,
		UserRequestRaw:        r.UserRequestRaw,
		MaxGenerationAttempts: r.MaxGenerationAttempts,
		ScheduledFor:          scheduledFor,
		Status:                assistant.ScheduledRunStatus(r.Status),
		RunID:                 strings.TrimSpace(r.RunID),
		ErrorMessage:          strings.TrimSpace(r.ErrorMessage),
		CreatedAt:             createdAt,
	}
	if strings.TrimSpace(r.TriggeredAt) != "" {
		triggeredAt, err := parseTime(r.TriggeredAt)
		if err != nil {
			return assistant.ScheduledRun{}, err
		}
		scheduledRun.TriggeredAt = &triggeredAt
	}
	return scheduledRun, nil
}

func marshalJSON(value any) (string, error) {
	bytes, err := json.Marshal(value)
	if err != nil {
		return "", err
	}
	return string(bytes), nil
}

func unmarshalRows[T any](output []byte) ([]T, error) {
	trimmed := strings.TrimSpace(string(output))
	if trimmed == "" {
		return []T{}, nil
	}
	var rows []T
	if err := json.Unmarshal([]byte(trimmed), &rows); err != nil {
		return nil, fmt.Errorf("decode sqlite rows: %w", err)
	}
	return rows, nil
}

func parseTime(value string) (time.Time, error) {
	parsed, err := time.Parse(time.RFC3339Nano, value)
	if err != nil {
		return time.Time{}, fmt.Errorf("parse time %q: %w", value, err)
	}
	return parsed, nil
}
