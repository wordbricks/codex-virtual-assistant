package store

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"sort"
	"strings"
	"time"

	"github.com/siisee11/CodexVirtualAssistant/internal/assistant"
	"github.com/siisee11/CodexVirtualAssistant/internal/config"
)

var ErrNotFound = errors.New("store: not found")

type RunRecord struct {
	Run           assistant.Run            `json:"run"`
	Events        []assistant.RunEvent     `json:"events"`
	Attempts      []assistant.Attempt      `json:"attempts"`
	Artifacts     []assistant.Artifact     `json:"artifacts"`
	Evidence      []assistant.Evidence     `json:"evidence"`
	Evaluations   []assistant.Evaluation   `json:"evaluations"`
	ToolCalls     []assistant.ToolCall     `json:"tool_calls"`
	WebSteps      []assistant.WebStep      `json:"web_steps"`
	WaitRequests  []assistant.WaitRequest  `json:"wait_requests"`
	ScheduledRuns []assistant.ScheduledRun `json:"scheduled_runs"`
}

type ChatRecord struct {
	Chat assistant.Chat `json:"chat"`
	Runs []RunRecord    `json:"runs"`
}

type SQLiteRepository struct {
	path       string
	sqlitePath string
}

func OpenSQLite(cfg config.Config) (*SQLiteRepository, error) {
	if err := EnsureScaffold(cfg); err != nil {
		return nil, err
	}
	return OpenSQLitePath(cfg.DatabasePath)
}

func OpenSQLitePath(path string) (*SQLiteRepository, error) {
	sqlitePath, err := exec.LookPath("sqlite3")
	if err != nil {
		return nil, fmt.Errorf("find sqlite3: %w", err)
	}

	repo := &SQLiteRepository{
		path:       path,
		sqlitePath: sqlitePath,
	}
	if err := repo.migrate(context.Background()); err != nil {
		return nil, err
	}
	return repo, nil
}

func (r *SQLiteRepository) Close() error {
	return nil
}

func (r *SQLiteRepository) SaveRun(ctx context.Context, run assistant.Run) error {
	if err := run.Validate(); err != nil {
		return err
	}

	taskSpecJSON, err := marshalJSON(run.TaskSpec)
	if err != nil {
		return err
	}
	projectJSON, err := marshalJSON(run.Project)
	if err != nil {
		return err
	}

	script := fmt.Sprintf(`
INSERT INTO runs (
	id,
	chat_id,
	parent_run_id,
	status,
	phase,
	gate_route,
	gate_reason,
	gate_decided_at,
	project_json,
	user_request_raw,
	task_spec_json,
	attempt_count,
	max_generation_attempts,
	created_at,
	updated_at,
	completed_at
) VALUES (
	%s,
	%s,
	%s,
	%s,
	%s,
	%s,
	%s,
	%s,
	%s,
	%s,
	%s,
	%d,
	%d,
	%s,
	%s,
	%s
)
ON CONFLICT(id) DO UPDATE SET
	chat_id = excluded.chat_id,
	status = excluded.status,
	phase = excluded.phase,
	parent_run_id = excluded.parent_run_id,
	gate_route = excluded.gate_route,
	gate_reason = excluded.gate_reason,
	gate_decided_at = excluded.gate_decided_at,
	project_json = excluded.project_json,
	user_request_raw = excluded.user_request_raw,
	task_spec_json = excluded.task_spec_json,
	attempt_count = excluded.attempt_count,
	max_generation_attempts = excluded.max_generation_attempts,
	updated_at = excluded.updated_at,
	completed_at = excluded.completed_at;
`, sqlText(run.ID), sqlText(run.ChatID), sqlNullableText(run.ParentRunID), sqlText(string(run.Status)), sqlText(string(run.Phase)), sqlText(string(run.GateRoute)), sqlText(run.GateReason), sqlNullableTime(run.GateDecidedAt), sqlText(projectJSON), sqlText(run.UserRequestRaw), sqlText(taskSpecJSON), run.AttemptCount, run.MaxGenerationAttempts, sqlTime(run.CreatedAt), sqlTime(run.UpdatedAt), sqlNullableTime(run.CompletedAt))

	return r.exec(ctx, script)
}

func (r *SQLiteRepository) AddRunEvent(ctx context.Context, event assistant.RunEvent) error {
	script := fmt.Sprintf(`
INSERT INTO run_events (id, run_id, type, phase, summary, created_at)
VALUES (%s, %s, %s, %s, %s, %s);
`, sqlText(event.ID), sqlText(event.RunID), sqlText(string(event.Type)), sqlText(string(event.Phase)), sqlText(event.Summary), sqlTime(event.CreatedAt))
	return r.exec(ctx, script)
}

func (r *SQLiteRepository) AddAttempt(ctx context.Context, attempt assistant.Attempt) error {
	script := fmt.Sprintf(`
INSERT INTO attempts (id, run_id, sequence, role, input_summary, output_summary, critique, started_at, finished_at)
VALUES (%s, %s, %d, %s, %s, %s, %s, %s, %s);
`, sqlText(attempt.ID), sqlText(attempt.RunID), attempt.Sequence, sqlText(string(attempt.Role)), sqlText(attempt.InputSummary), sqlText(attempt.OutputSummary), sqlText(attempt.Critique), sqlTime(attempt.StartedAt), sqlNullableTime(attempt.FinishedAt))
	return r.exec(ctx, script)
}

func (r *SQLiteRepository) AddArtifact(ctx context.Context, artifact assistant.Artifact) error {
	script := fmt.Sprintf(`
INSERT INTO artifacts (id, run_id, attempt_id, kind, title, mime_type, path, content, source_url, created_at)
VALUES (%s, %s, %s, %s, %s, %s, %s, %s, %s, %s);
`, sqlText(artifact.ID), sqlText(artifact.RunID), sqlText(artifact.AttemptID), sqlText(string(artifact.Kind)), sqlText(artifact.Title), sqlText(artifact.MIMEType), sqlText(artifact.Path), sqlText(artifact.Content), sqlText(artifact.SourceURL), sqlTime(artifact.CreatedAt))
	return r.exec(ctx, script)
}

func (r *SQLiteRepository) AddEvaluation(ctx context.Context, evaluation assistant.Evaluation) error {
	if err := evaluation.Validate(); err != nil {
		return err
	}

	missingRequirementsJSON, err := marshalJSON(evaluation.MissingRequirements)
	if err != nil {
		return err
	}
	incorrectClaimsJSON, err := marshalJSON(evaluation.IncorrectClaims)
	if err != nil {
		return err
	}
	evidenceCheckedJSON, err := marshalJSON(evaluation.EvidenceChecked)
	if err != nil {
		return err
	}

	script := fmt.Sprintf(`
INSERT INTO evaluations (
	id,
	run_id,
	attempt_id,
	passed,
	score,
	summary,
	missing_requirements_json,
	incorrect_claims_json,
	evidence_checked_json,
	next_action_for_generator,
	created_at
) VALUES (
	%s,
	%s,
	%s,
	%s,
	%d,
	%s,
	%s,
	%s,
	%s,
	%s,
	%s
);
`, sqlText(evaluation.ID), sqlText(evaluation.RunID), sqlText(evaluation.AttemptID), sqlBool(evaluation.Passed), evaluation.Score, sqlText(evaluation.Summary), sqlText(missingRequirementsJSON), sqlText(incorrectClaimsJSON), sqlText(evidenceCheckedJSON), sqlText(evaluation.NextActionForGenerator), sqlTime(evaluation.CreatedAt))
	return r.exec(ctx, script)
}

func (r *SQLiteRepository) AddEvidence(ctx context.Context, evidence assistant.Evidence) error {
	script := fmt.Sprintf(`
INSERT INTO evidences (id, run_id, attempt_id, kind, summary, detail, created_at)
VALUES (%s, %s, %s, %s, %s, %s, %s);
`, sqlText(evidence.ID), sqlText(evidence.RunID), sqlText(evidence.AttemptID), sqlText(string(evidence.Kind)), sqlText(evidence.Summary), sqlText(evidence.Detail), sqlTime(evidence.CreatedAt))
	return r.exec(ctx, script)
}

func (r *SQLiteRepository) AddToolCall(ctx context.Context, toolCall assistant.ToolCall) error {
	script := fmt.Sprintf(`
INSERT INTO tool_calls (id, run_id, attempt_id, tool_name, input_summary, output_summary, started_at, finished_at)
VALUES (%s, %s, %s, %s, %s, %s, %s, %s);
`, sqlText(toolCall.ID), sqlText(toolCall.RunID), sqlText(toolCall.AttemptID), sqlText(toolCall.ToolName), sqlText(toolCall.InputSummary), sqlText(toolCall.OutputSummary), sqlTime(toolCall.StartedAt), sqlTime(toolCall.FinishedAt))
	return r.exec(ctx, script)
}

func (r *SQLiteRepository) AddWebStep(ctx context.Context, webStep assistant.WebStep) error {
	script := fmt.Sprintf(`
INSERT INTO web_steps (id, run_id, attempt_id, title, url, summary, occurred_at)
VALUES (%s, %s, %s, %s, %s, %s, %s);
`, sqlText(webStep.ID), sqlText(webStep.RunID), sqlText(webStep.AttemptID), sqlText(webStep.Title), sqlText(webStep.URL), sqlText(webStep.Summary), sqlTime(webStep.OccurredAt))
	return r.exec(ctx, script)
}

func (r *SQLiteRepository) AddWaitRequest(ctx context.Context, waitRequest assistant.WaitRequest) error {
	script := fmt.Sprintf(`
INSERT INTO wait_requests (id, run_id, kind, title, prompt, risk_summary, created_at)
VALUES (%s, %s, %s, %s, %s, %s, %s);
`, sqlText(waitRequest.ID), sqlText(waitRequest.RunID), sqlText(string(waitRequest.Kind)), sqlText(waitRequest.Title), sqlText(waitRequest.Prompt), sqlText(waitRequest.RiskSummary), sqlTime(waitRequest.CreatedAt))
	return r.exec(ctx, script)
}

func (r *SQLiteRepository) SaveScheduledRun(ctx context.Context, scheduledRun assistant.ScheduledRun) error {
	if err := scheduledRun.Validate(); err != nil {
		return err
	}

	script := fmt.Sprintf(`
INSERT INTO scheduled_runs (
	id,
	chat_id,
	parent_run_id,
	user_request_raw,
	max_generation_attempts,
	scheduled_for,
	status,
	run_id,
	error_message,
	created_at,
	triggered_at
) VALUES (
	%s,
	%s,
	%s,
	%s,
	%d,
	%s,
	%s,
	%s,
	%s,
	%s,
	%s
)
ON CONFLICT(id) DO UPDATE SET
	chat_id = excluded.chat_id,
	parent_run_id = excluded.parent_run_id,
	user_request_raw = excluded.user_request_raw,
	max_generation_attempts = excluded.max_generation_attempts,
	scheduled_for = excluded.scheduled_for,
	status = excluded.status,
	run_id = excluded.run_id,
	error_message = excluded.error_message,
	created_at = excluded.created_at,
	triggered_at = excluded.triggered_at;
`, sqlText(scheduledRun.ID), sqlText(scheduledRun.ChatID), sqlText(scheduledRun.ParentRunID), sqlText(scheduledRun.UserRequestRaw), scheduledRun.MaxGenerationAttempts, sqlTime(scheduledRun.ScheduledFor), sqlText(string(scheduledRun.Status)), sqlNullableText(scheduledRun.RunID), sqlText(scheduledRun.ErrorMessage), sqlTime(scheduledRun.CreatedAt), sqlNullableTime(scheduledRun.TriggeredAt))

	return r.exec(ctx, script)
}

func (r *SQLiteRepository) GetScheduledRun(ctx context.Context, scheduledRunID string) (assistant.ScheduledRun, error) {
	rows, err := queryRows[scheduledRunRow](ctx, r, fmt.Sprintf(`
SELECT
	id,
	chat_id,
	parent_run_id,
	user_request_raw,
	max_generation_attempts,
	scheduled_for,
	status,
	COALESCE(run_id, '') AS run_id,
	COALESCE(error_message, '') AS error_message,
	created_at,
	COALESCE(triggered_at, '') AS triggered_at
FROM scheduled_runs
WHERE id = %s
LIMIT 1;
`, sqlText(scheduledRunID)))
	if err != nil {
		return assistant.ScheduledRun{}, err
	}
	if len(rows) == 0 {
		return assistant.ScheduledRun{}, ErrNotFound
	}
	return rows[0].toAssistantScheduledRun()
}

func (r *SQLiteRepository) GetRun(ctx context.Context, runID string) (assistant.Run, error) {
	rows, err := queryRows[runRow](ctx, r, fmt.Sprintf(`
SELECT
	id,
	COALESCE(NULLIF(chat_id, ''), id) AS chat_id,
	COALESCE(parent_run_id, '') AS parent_run_id,
	status,
	phase,
	COALESCE(gate_route, '') AS gate_route,
	COALESCE(gate_reason, '') AS gate_reason,
	COALESCE(gate_decided_at, '') AS gate_decided_at,
	COALESCE(project_json, '{}') AS project_json,
	user_request_raw,
	task_spec_json,
	attempt_count,
	max_generation_attempts,
	created_at,
	updated_at,
	COALESCE(completed_at, '') AS completed_at
FROM runs
WHERE id = %s
LIMIT 1;
`, sqlText(runID)))
	if err != nil {
		return assistant.Run{}, err
	}
	if len(rows) == 0 {
		return assistant.Run{}, ErrNotFound
	}
	return rows[0].toAssistantRun()
}

func (r *SQLiteRepository) ListRuns(ctx context.Context) ([]assistant.Run, error) {
	rows, err := queryRows[runRow](ctx, r, `
SELECT
	id,
	COALESCE(NULLIF(chat_id, ''), id) AS chat_id,
	COALESCE(parent_run_id, '') AS parent_run_id,
	status,
	phase,
	COALESCE(gate_route, '') AS gate_route,
	COALESCE(gate_reason, '') AS gate_reason,
	COALESCE(gate_decided_at, '') AS gate_decided_at,
	COALESCE(project_json, '{}') AS project_json,
	user_request_raw,
	task_spec_json,
	attempt_count,
	max_generation_attempts,
	created_at,
	updated_at,
	COALESCE(completed_at, '') AS completed_at
FROM runs
ORDER BY created_at ASC, updated_at ASC;
`)
	if err != nil {
		return nil, err
	}
	runs := make([]assistant.Run, 0, len(rows))
	for _, row := range rows {
		run, err := row.toAssistantRun()
		if err != nil {
			return nil, err
		}
		runs = append(runs, run)
	}
	return runs, nil
}

func (r *SQLiteRepository) ListRunsByChat(ctx context.Context, chatID string) ([]assistant.Run, error) {
	rows, err := queryRows[runRow](ctx, r, fmt.Sprintf(`
SELECT
	id,
	COALESCE(NULLIF(chat_id, ''), id) AS chat_id,
	COALESCE(parent_run_id, '') AS parent_run_id,
	status,
	phase,
	COALESCE(gate_route, '') AS gate_route,
	COALESCE(gate_reason, '') AS gate_reason,
	COALESCE(gate_decided_at, '') AS gate_decided_at,
	COALESCE(project_json, '{}') AS project_json,
	user_request_raw,
	task_spec_json,
	attempt_count,
	max_generation_attempts,
	created_at,
	updated_at,
	COALESCE(completed_at, '') AS completed_at
FROM runs
WHERE COALESCE(NULLIF(chat_id, ''), id) = %s
ORDER BY created_at ASC, updated_at ASC;
`, sqlText(chatID)))
	if err != nil {
		return nil, err
	}
	runs := make([]assistant.Run, 0, len(rows))
	for _, row := range rows {
		run, err := row.toAssistantRun()
		if err != nil {
			return nil, err
		}
		runs = append(runs, run)
	}
	return runs, nil
}

func (r *SQLiteRepository) ListRunEvents(ctx context.Context, runID string) ([]assistant.RunEvent, error) {
	rows, err := queryRows[runEventRow](ctx, r, fmt.Sprintf(`
SELECT id, run_id, type, COALESCE(phase, '') AS phase, summary, created_at
FROM run_events
WHERE run_id = %s
ORDER BY created_at ASC;
`, sqlText(runID)))
	if err != nil {
		return nil, err
	}
	events := make([]assistant.RunEvent, 0, len(rows))
	for _, row := range rows {
		event, err := row.toAssistantRunEvent()
		if err != nil {
			return nil, err
		}
		events = append(events, event)
	}
	return events, nil
}

func (r *SQLiteRepository) ListAttempts(ctx context.Context, runID string) ([]assistant.Attempt, error) {
	rows, err := queryRows[attemptRow](ctx, r, fmt.Sprintf(`
SELECT id, run_id, sequence, role, input_summary, output_summary, COALESCE(critique, '') AS critique, started_at, COALESCE(finished_at, '') AS finished_at
FROM attempts
WHERE run_id = %s
ORDER BY sequence ASC, started_at ASC;
`, sqlText(runID)))
	if err != nil {
		return nil, err
	}
	attempts := make([]assistant.Attempt, 0, len(rows))
	for _, row := range rows {
		attempt, err := row.toAssistantAttempt()
		if err != nil {
			return nil, err
		}
		attempts = append(attempts, attempt)
	}
	return attempts, nil
}

func (r *SQLiteRepository) ListArtifacts(ctx context.Context, runID string) ([]assistant.Artifact, error) {
	rows, err := queryRows[artifactRow](ctx, r, fmt.Sprintf(`
SELECT id, run_id, attempt_id, kind, title, mime_type, COALESCE(path, '') AS path, COALESCE(content, '') AS content, COALESCE(source_url, '') AS source_url, created_at
FROM artifacts
WHERE run_id = %s
ORDER BY created_at ASC;
`, sqlText(runID)))
	if err != nil {
		return nil, err
	}
	artifacts := make([]assistant.Artifact, 0, len(rows))
	for _, row := range rows {
		artifact, err := row.toAssistantArtifact()
		if err != nil {
			return nil, err
		}
		artifacts = append(artifacts, artifact)
	}
	return artifacts, nil
}

func (r *SQLiteRepository) ListEvidence(ctx context.Context, runID string) ([]assistant.Evidence, error) {
	rows, err := queryRows[evidenceRow](ctx, r, fmt.Sprintf(`
SELECT id, run_id, attempt_id, kind, summary, COALESCE(detail, '') AS detail, created_at
FROM evidences
WHERE run_id = %s
ORDER BY created_at ASC;
`, sqlText(runID)))
	if err != nil {
		return nil, err
	}
	evidence := make([]assistant.Evidence, 0, len(rows))
	for _, row := range rows {
		item, err := row.toAssistantEvidence()
		if err != nil {
			return nil, err
		}
		evidence = append(evidence, item)
	}
	return evidence, nil
}

func (r *SQLiteRepository) ListEvaluations(ctx context.Context, runID string) ([]assistant.Evaluation, error) {
	rows, err := queryRows[evaluationRow](ctx, r, fmt.Sprintf(`
SELECT
	id,
	run_id,
	attempt_id,
	passed,
	score,
	summary,
	missing_requirements_json,
	incorrect_claims_json,
	evidence_checked_json,
	next_action_for_generator,
	created_at
FROM evaluations
WHERE run_id = %s
ORDER BY created_at ASC;
`, sqlText(runID)))
	if err != nil {
		return nil, err
	}
	evaluations := make([]assistant.Evaluation, 0, len(rows))
	for _, row := range rows {
		evaluation, err := row.toAssistantEvaluation()
		if err != nil {
			return nil, err
		}
		evaluations = append(evaluations, evaluation)
	}
	return evaluations, nil
}

func (r *SQLiteRepository) ListToolCalls(ctx context.Context, runID string) ([]assistant.ToolCall, error) {
	rows, err := queryRows[toolCallRow](ctx, r, fmt.Sprintf(`
SELECT id, run_id, attempt_id, tool_name, input_summary, output_summary, started_at, finished_at
FROM tool_calls
WHERE run_id = %s
ORDER BY started_at ASC;
`, sqlText(runID)))
	if err != nil {
		return nil, err
	}
	toolCalls := make([]assistant.ToolCall, 0, len(rows))
	for _, row := range rows {
		toolCall, err := row.toAssistantToolCall()
		if err != nil {
			return nil, err
		}
		toolCalls = append(toolCalls, toolCall)
	}
	return toolCalls, nil
}

func (r *SQLiteRepository) ListWebSteps(ctx context.Context, runID string) ([]assistant.WebStep, error) {
	rows, err := queryRows[webStepRow](ctx, r, fmt.Sprintf(`
SELECT id, run_id, attempt_id, title, COALESCE(url, '') AS url, summary, occurred_at
FROM web_steps
WHERE run_id = %s
ORDER BY occurred_at ASC;
`, sqlText(runID)))
	if err != nil {
		return nil, err
	}
	webSteps := make([]assistant.WebStep, 0, len(rows))
	for _, row := range rows {
		item, err := row.toAssistantWebStep()
		if err != nil {
			return nil, err
		}
		webSteps = append(webSteps, item)
	}
	return webSteps, nil
}

func (r *SQLiteRepository) ListWaitRequests(ctx context.Context, runID string) ([]assistant.WaitRequest, error) {
	rows, err := queryRows[waitRequestRow](ctx, r, fmt.Sprintf(`
SELECT id, run_id, kind, title, prompt, COALESCE(risk_summary, '') AS risk_summary, created_at
FROM wait_requests
WHERE run_id = %s
ORDER BY created_at ASC;
`, sqlText(runID)))
	if err != nil {
		return nil, err
	}
	waitRequests := make([]assistant.WaitRequest, 0, len(rows))
	for _, row := range rows {
		waitRequest, err := row.toAssistantWaitRequest()
		if err != nil {
			return nil, err
		}
		waitRequests = append(waitRequests, waitRequest)
	}
	return waitRequests, nil
}

func (r *SQLiteRepository) ListPendingScheduledRuns(ctx context.Context, before time.Time) ([]assistant.ScheduledRun, error) {
	return r.listScheduledRunsWithQuery(ctx, fmt.Sprintf(`
SELECT
	id,
	chat_id,
	parent_run_id,
	user_request_raw,
	max_generation_attempts,
	scheduled_for,
	status,
	COALESCE(run_id, '') AS run_id,
	COALESCE(error_message, '') AS error_message,
	created_at,
	COALESCE(triggered_at, '') AS triggered_at
FROM scheduled_runs
WHERE status = %s
	AND scheduled_for <= %s
ORDER BY scheduled_for ASC, created_at ASC;
`, sqlText(string(assistant.ScheduledRunStatusPending)), sqlTime(before.UTC())))
}

func (r *SQLiteRepository) ListScheduledRunsByParent(ctx context.Context, parentRunID string) ([]assistant.ScheduledRun, error) {
	return r.listScheduledRunsWithQuery(ctx, fmt.Sprintf(`
SELECT
	id,
	chat_id,
	parent_run_id,
	user_request_raw,
	max_generation_attempts,
	scheduled_for,
	status,
	COALESCE(run_id, '') AS run_id,
	COALESCE(error_message, '') AS error_message,
	created_at,
	COALESCE(triggered_at, '') AS triggered_at
FROM scheduled_runs
WHERE parent_run_id = %s
ORDER BY scheduled_for ASC, created_at ASC;
`, sqlText(parentRunID)))
}

func (r *SQLiteRepository) ListScheduledRunsByChat(ctx context.Context, chatID string) ([]assistant.ScheduledRun, error) {
	return r.listScheduledRunsWithQuery(ctx, fmt.Sprintf(`
SELECT
	id,
	chat_id,
	parent_run_id,
	user_request_raw,
	max_generation_attempts,
	scheduled_for,
	status,
	COALESCE(run_id, '') AS run_id,
	COALESCE(error_message, '') AS error_message,
	created_at,
	COALESCE(triggered_at, '') AS triggered_at
FROM scheduled_runs
WHERE chat_id = %s
ORDER BY scheduled_for ASC, created_at ASC;
`, sqlText(chatID)))
}

func (r *SQLiteRepository) ListScheduledRuns(ctx context.Context, chatID string, status assistant.ScheduledRunStatus) ([]assistant.ScheduledRun, error) {
	clauses := []string{"1 = 1"}
	if trimmed := strings.TrimSpace(chatID); trimmed != "" {
		clauses = append(clauses, fmt.Sprintf("chat_id = %s", sqlText(trimmed)))
	}
	if trimmed := strings.TrimSpace(string(status)); trimmed != "" {
		clauses = append(clauses, fmt.Sprintf("status = %s", sqlText(trimmed)))
	}
	return r.listScheduledRunsWithQuery(ctx, fmt.Sprintf(`
SELECT
	id,
	chat_id,
	parent_run_id,
	user_request_raw,
	max_generation_attempts,
	scheduled_for,
	status,
	COALESCE(run_id, '') AS run_id,
	COALESCE(error_message, '') AS error_message,
	created_at,
	COALESCE(triggered_at, '') AS triggered_at
FROM scheduled_runs
WHERE %s
ORDER BY scheduled_for ASC, created_at ASC;
`, strings.Join(clauses, " AND ")))
}

func (r *SQLiteRepository) UpdateScheduledRunTriggered(ctx context.Context, scheduledRunID, runID string, triggeredAt time.Time) error {
	script := fmt.Sprintf(`
UPDATE scheduled_runs
SET status = %s,
	run_id = %s,
	error_message = '',
	triggered_at = %s
WHERE id = %s;
`, sqlText(string(assistant.ScheduledRunStatusTriggered)), sqlText(runID), sqlTime(triggeredAt.UTC()), sqlText(scheduledRunID))
	return r.exec(ctx, script)
}

func (r *SQLiteRepository) UpdateScheduledRunStatus(ctx context.Context, scheduledRunID string, status assistant.ScheduledRunStatus, errorMessage string) error {
	triggeredAtExpr := "triggered_at"
	if status != assistant.ScheduledRunStatusTriggered {
		triggeredAtExpr = "NULL"
	}
	script := fmt.Sprintf(`
UPDATE scheduled_runs
SET status = %s,
	error_message = %s,
	run_id = CASE WHEN %s = 'triggered' THEN run_id ELSE NULL END,
	triggered_at = %s
WHERE id = %s;
`, sqlText(string(status)), sqlText(strings.TrimSpace(errorMessage)), sqlText(string(status)), triggeredAtExpr, sqlText(scheduledRunID))
	return r.exec(ctx, script)
}

func (r *SQLiteRepository) listScheduledRunsWithQuery(ctx context.Context, query string) ([]assistant.ScheduledRun, error) {
	rows, err := queryRows[scheduledRunRow](ctx, r, query)
	if err != nil {
		return nil, err
	}
	scheduledRuns := make([]assistant.ScheduledRun, 0, len(rows))
	for _, row := range rows {
		scheduledRun, err := row.toAssistantScheduledRun()
		if err != nil {
			return nil, err
		}
		scheduledRuns = append(scheduledRuns, scheduledRun)
	}
	return scheduledRuns, nil
}

func (r *SQLiteRepository) GetRunRecord(ctx context.Context, runID string) (RunRecord, error) {
	run, err := r.GetRun(ctx, runID)
	if err != nil {
		return RunRecord{}, err
	}
	events, err := r.ListRunEvents(ctx, runID)
	if err != nil {
		return RunRecord{}, err
	}
	attempts, err := r.ListAttempts(ctx, runID)
	if err != nil {
		return RunRecord{}, err
	}
	artifacts, err := r.ListArtifacts(ctx, runID)
	if err != nil {
		return RunRecord{}, err
	}
	evidence, err := r.ListEvidence(ctx, runID)
	if err != nil {
		return RunRecord{}, err
	}
	evaluations, err := r.ListEvaluations(ctx, runID)
	if err != nil {
		return RunRecord{}, err
	}
	toolCalls, err := r.ListToolCalls(ctx, runID)
	if err != nil {
		return RunRecord{}, err
	}
	webSteps, err := r.ListWebSteps(ctx, runID)
	if err != nil {
		return RunRecord{}, err
	}
	waitRequests, err := r.ListWaitRequests(ctx, runID)
	if err != nil {
		return RunRecord{}, err
	}
	scheduledRuns, err := r.ListScheduledRunsByParent(ctx, runID)
	if err != nil {
		return RunRecord{}, err
	}

	run.AttemptCount = len(attempts)
	if len(evaluations) > 0 {
		latest := evaluations[len(evaluations)-1]
		run.LatestEvaluation = &latest
	}
	if run.Status == assistant.RunStatusWaiting && len(waitRequests) > 0 {
		latest := waitRequests[len(waitRequests)-1]
		run.WaitingFor = &latest
	}

	return RunRecord{
		Run:           run,
		Events:        events,
		Attempts:      attempts,
		Artifacts:     artifacts,
		Evidence:      evidence,
		Evaluations:   evaluations,
		ToolCalls:     toolCalls,
		WebSteps:      webSteps,
		WaitRequests:  waitRequests,
		ScheduledRuns: scheduledRuns,
	}, nil
}

func (r *SQLiteRepository) ListChats(ctx context.Context) ([]assistant.Chat, error) {
	runs, err := r.ListRuns(ctx)
	if err != nil {
		return nil, err
	}
	chats := make([]assistant.Chat, 0)
	byID := make(map[string]int)
	for _, run := range runs {
		chatID := strings.TrimSpace(run.ChatID)
		if chatID == "" {
			chatID = run.ID
		}
		if idx, ok := byID[chatID]; ok {
			chat := chats[idx]
			if run.CreatedAt.Before(chat.CreatedAt) {
				chat.CreatedAt = run.CreatedAt
				chat.RootRunID = run.ID
				chat.Title = chatTitle(run)
			}
			if run.CreatedAt.After(chat.UpdatedAt) || run.UpdatedAt.After(chat.UpdatedAt) {
				chat.LatestRunID = run.ID
				chat.Status = run.Status
				if run.UpdatedAt.After(chat.UpdatedAt) {
					chat.UpdatedAt = run.UpdatedAt
				}
			}
			chats[idx] = chat
			continue
		}
		byID[chatID] = len(chats)
		chats = append(chats, assistant.Chat{
			ID:          chatID,
			RootRunID:   run.ID,
			LatestRunID: run.ID,
			Title:       chatTitle(run),
			Status:      run.Status,
			CreatedAt:   run.CreatedAt,
			UpdatedAt:   run.UpdatedAt,
		})
	}

	sort.Slice(chats, func(i, j int) bool {
		if chats[i].UpdatedAt.Equal(chats[j].UpdatedAt) {
			return chats[i].CreatedAt.After(chats[j].CreatedAt)
		}
		return chats[i].UpdatedAt.After(chats[j].UpdatedAt)
	})
	return chats, nil
}

func (r *SQLiteRepository) GetChatRecord(ctx context.Context, chatID string) (ChatRecord, error) {
	chatID = strings.TrimSpace(chatID)
	if chatID == "" {
		return ChatRecord{}, ErrNotFound
	}

	run, err := r.GetRun(ctx, chatID)
	if err == nil {
		chatID = firstNonEmpty(strings.TrimSpace(run.ChatID), run.ID)
	} else if !errors.Is(err, ErrNotFound) {
		return ChatRecord{}, err
	}

	runs, err := r.ListRunsByChat(ctx, chatID)
	if err != nil {
		return ChatRecord{}, err
	}
	if len(runs) == 0 {
		return ChatRecord{}, ErrNotFound
	}

	records := make([]RunRecord, 0, len(runs))
	for _, run := range runs {
		record, err := r.GetRunRecord(ctx, run.ID)
		if err != nil {
			return ChatRecord{}, err
		}
		records = append(records, record)
	}

	chats, err := r.ListChats(ctx)
	if err != nil {
		return ChatRecord{}, err
	}
	for _, chat := range chats {
		if chat.ID == chatID {
			return ChatRecord{Chat: chat, Runs: records}, nil
		}
	}
	return ChatRecord{}, ErrNotFound
}

func (r *SQLiteRepository) migrate(ctx context.Context) error {
	if err := r.exec(ctx, schemaSQL); err != nil {
		return err
	}
	if err := r.ensureRunColumns(ctx); err != nil {
		return err
	}
	return r.backfillRunChatIDs(ctx)
}

func (r *SQLiteRepository) ensureRunColumns(ctx context.Context) error {
	type runColumn struct {
		name string
		ddl  string
	}
	columns := []runColumn{
		{
			name: "chat_id",
			ddl:  `ALTER TABLE runs ADD COLUMN chat_id TEXT NOT NULL DEFAULT '';`,
		},
		{
			name: "project_json",
			ddl:  `ALTER TABLE runs ADD COLUMN project_json TEXT NOT NULL DEFAULT '{}';`,
		},
		{
			name: "parent_run_id",
			ddl:  `ALTER TABLE runs ADD COLUMN parent_run_id TEXT;`,
		},
		{
			name: "gate_route",
			ddl:  `ALTER TABLE runs ADD COLUMN gate_route TEXT NOT NULL DEFAULT '';`,
		},
		{
			name: "gate_reason",
			ddl:  `ALTER TABLE runs ADD COLUMN gate_reason TEXT NOT NULL DEFAULT '';`,
		},
		{
			name: "gate_decided_at",
			ddl:  `ALTER TABLE runs ADD COLUMN gate_decided_at TEXT;`,
		},
	}
	for _, column := range columns {
		exists, err := r.tableColumnExists(ctx, "runs", column.name)
		if err != nil {
			return err
		}
		if exists {
			continue
		}
		if err := r.exec(ctx, column.ddl); err != nil {
			return err
		}
	}
	return r.exec(ctx, `
CREATE INDEX IF NOT EXISTS idx_runs_parent_run_id ON runs(parent_run_id);
CREATE INDEX IF NOT EXISTS idx_runs_chat_id_created_at ON runs(chat_id, created_at);
`)
}

func (r *SQLiteRepository) backfillRunChatIDs(ctx context.Context) error {
	type runChatRow struct {
		ID          string `json:"id"`
		ChatID      string `json:"chat_id"`
		ParentRunID string `json:"parent_run_id"`
	}

	rows, err := queryRows[runChatRow](ctx, r, `
SELECT
	id,
	COALESCE(chat_id, '') AS chat_id,
	COALESCE(parent_run_id, '') AS parent_run_id
FROM runs
ORDER BY created_at ASC, updated_at ASC;
`)
	if err != nil {
		return err
	}
	if len(rows) == 0 {
		return nil
	}

	index := make(map[string]runChatRow, len(rows))
	for _, row := range rows {
		index[row.ID] = row
	}

	resolved := make(map[string]string, len(rows))
	var resolve func(string) string
	resolve = func(runID string) string {
		if runID == "" {
			return ""
		}
		if chatID := strings.TrimSpace(resolved[runID]); chatID != "" {
			return chatID
		}
		row, ok := index[runID]
		if !ok {
			return ""
		}
		if chatID := strings.TrimSpace(row.ChatID); chatID != "" {
			resolved[runID] = chatID
			return chatID
		}
		if strings.TrimSpace(row.ParentRunID) == "" {
			resolved[runID] = row.ID
			return row.ID
		}
		chatID := resolve(row.ParentRunID)
		if chatID == "" {
			chatID = row.ID
		}
		resolved[runID] = chatID
		return chatID
	}

	for _, row := range rows {
		if strings.TrimSpace(row.ChatID) != "" {
			continue
		}
		chatID := resolve(row.ID)
		if chatID == "" {
			continue
		}
		if err := r.exec(ctx, fmt.Sprintf(`
UPDATE runs
SET chat_id = %s
WHERE id = %s;
`, sqlText(chatID), sqlText(row.ID))); err != nil {
			return err
		}
	}
	return nil
}

func (r *SQLiteRepository) exec(ctx context.Context, script string) error {
	_, err := r.run(ctx, false, script)
	return err
}

func (r *SQLiteRepository) run(ctx context.Context, jsonMode bool, script string) ([]byte, error) {
	parts := []string{}
	if jsonMode {
		parts = append(parts, ".mode json")
	}
	parts = append(parts, "PRAGMA foreign_keys = ON;")
	parts = append(parts, script)
	fullScript := strings.Join(parts, "\n")

	var lastErr error
	for attempt := 0; attempt < 8; attempt++ {
		cmd := exec.CommandContext(ctx, r.sqlitePath, "-batch", r.path)
		cmd.Stdin = strings.NewReader(fullScript)
		output, err := cmd.CombinedOutput()
		if err == nil {
			return output, nil
		}

		message := strings.TrimSpace(string(output))
		lastErr = fmt.Errorf("sqlite3 %s: %w: %s", r.path, err, message)
		if !isSQLiteBusy(message) {
			return nil, lastErr
		}

		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(15 * time.Millisecond):
		}
	}
	return nil, lastErr
}

func queryRows[T any](ctx context.Context, repo *SQLiteRepository, query string) ([]T, error) {
	output, err := repo.run(ctx, true, query)
	if err != nil {
		return nil, err
	}
	return unmarshalRows[T](output)
}

type pragmaColumnRow struct {
	Name string `json:"name"`
}

func (r *SQLiteRepository) tableColumnExists(ctx context.Context, tableName, columnName string) (bool, error) {
	rows, err := queryRows[pragmaColumnRow](ctx, r, fmt.Sprintf(`PRAGMA table_info(%s);`, tableName))
	if err != nil {
		return false, err
	}
	for _, row := range rows {
		if strings.EqualFold(strings.TrimSpace(row.Name), strings.TrimSpace(columnName)) {
			return true, nil
		}
	}
	return false, nil
}

func chatTitle(run assistant.Run) string {
	return firstNonEmpty(strings.TrimSpace(run.TaskSpec.Goal), strings.TrimSpace(run.UserRequestRaw))
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func sqlText(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "''") + "'"
}

func sqlNullableText(value string) string {
	if strings.TrimSpace(value) == "" {
		return "NULL"
	}
	return sqlText(value)
}

func sqlBool(value bool) string {
	if value {
		return "1"
	}
	return "0"
}

func sqlTime(value time.Time) string {
	return sqlText(value.UTC().Format(time.RFC3339Nano))
}

func sqlNullableTime(value *time.Time) string {
	if value == nil {
		return "NULL"
	}
	return sqlTime(*value)
}

func isSQLiteBusy(message string) bool {
	lower := strings.ToLower(message)
	return strings.Contains(lower, "database is locked") || strings.Contains(lower, "database is busy")
}
