package wtl

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/siisee11/CodexVirtualAssistant/internal/assistant"
)

type CodexPhaseExecutor interface {
	RunPhase(context.Context, CodexPhaseRequest) (CodexPhaseResult, error)
	Close() error
}

type CodexPhaseRequest struct {
	Role        assistant.AttemptRole    `json:"role"`
	Model       string                   `json:"model"`
	SessionName string                   `json:"session_name"`
	Prompt      string                   `json:"prompt"`
	RunID       string                   `json:"run_id"`
	AttemptID   string                   `json:"attempt_id"`
	WorkingDir  string                   `json:"working_dir"`
	UserRequest string                   `json:"user_request"`
	Project     assistant.ProjectContext `json:"project"`
	TaskSpec    assistant.TaskSpec       `json:"task_spec"`
	Critique    string                   `json:"critique,omitempty"`
	ResumeInput map[string]string        `json:"resume_input,omitempty"`
	ProjectSlug string                   `json:"project_slug,omitempty"`
	Tools       []string                 `json:"tools"`
	LiveEmit    func(assistant.RunEvent) `json:"-"`
}

type CodexPhaseResult struct {
	Summary      string                 `json:"summary"`
	Output       string                 `json:"output"`
	Artifacts    []assistant.Artifact   `json:"artifacts"`
	Observations []string               `json:"observations"`
	ToolRuns     []CodexToolRun         `json:"tool_runs"`
	BrowserSteps []AgentBrowserStep     `json:"browser_steps"`
	WaitRequest  *assistant.WaitRequest `json:"wait_request,omitempty"`
}

type CodexToolRun struct {
	ID            string    `json:"id,omitempty"`
	Name          string    `json:"name"`
	InputSummary  string    `json:"input_summary"`
	OutputSummary string    `json:"output_summary"`
	StartedAt     time.Time `json:"started_at"`
	FinishedAt    time.Time `json:"finished_at"`
}

type AgentBrowserStep struct {
	Title          string             `json:"title"`
	URL            string             `json:"url,omitempty"`
	Summary        string             `json:"summary"`
	Action         AgentBrowserAction `json:"action"`
	ObservedText   []string           `json:"observed_text,omitempty"`
	ScreenshotPath string             `json:"screenshot_path,omitempty"`
	ScreenshotNote string             `json:"screenshot_note,omitempty"`
	OccurredAt     time.Time          `json:"occurred_at"`
}

type AgentBrowserAction struct {
	Name    string `json:"name"`
	Target  string `json:"target,omitempty"`
	Ref     string `json:"ref,omitempty"`
	Value   string `json:"value,omitempty"`
	Session string `json:"session,omitempty"`
}

type CodexRuntime struct {
	executor    CodexPhaseExecutor
	model       string
	now         func() time.Time
	sessionName func(run assistant.Run, role assistant.AttemptRole) string
}

func NewCodexRuntime(executor CodexPhaseExecutor, model string, now func() time.Time) *CodexRuntime {
	if now == nil {
		now = time.Now
	}
	return &CodexRuntime{
		executor: executor,
		model:    firstNonEmpty(model, "gpt-5.4"),
		now:      now,
		sessionName: func(run assistant.Run, role assistant.AttemptRole) string {
			return fmt.Sprintf("%s-%s", run.ID, role)
		},
	}
}

func (r *CodexRuntime) Execute(ctx context.Context, role assistant.AttemptRole, request PhaseRequest) (PhaseResponse, error) {
	result, err := r.executor.RunPhase(ctx, CodexPhaseRequest{
		Role:        role,
		Model:       r.model,
		SessionName: r.sessionName(request.Run, role),
		Prompt:      composePhasePrompt(request),
		RunID:       request.Run.ID,
		AttemptID:   request.Attempt.ID,
		WorkingDir:  firstNonEmpty(request.WorkingDir, request.Run.Project.WorkspaceDir),
		UserRequest: request.Run.UserRequestRaw,
		Project:     request.Run.Project,
		TaskSpec:    request.Run.TaskSpec,
		Critique:    request.Critique,
		ResumeInput: request.ResumeInput,
		ProjectSlug: request.Run.Project.Slug,
		Tools:       phaseTools(request.Run, role),
		LiveEmit:    request.LiveEmit,
	})
	if err != nil {
		return PhaseResponse{}, err
	}

	response := PhaseResponse{
		Summary:     strings.TrimSpace(firstNonEmpty(result.Summary, summarizeOutput(result.Output))),
		Output:      result.Output,
		Artifacts:   append([]assistant.Artifact{}, result.Artifacts...),
		Evidence:    make([]assistant.Evidence, 0, len(result.Observations)+len(result.ToolRuns)+len(result.BrowserSteps)),
		ToolCalls:   make([]assistant.ToolCall, 0, len(result.ToolRuns)),
		WebSteps:    make([]assistant.WebStep, 0, len(result.BrowserSteps)),
		WaitRequest: result.WaitRequest,
	}

	for _, observation := range result.Observations {
		if strings.TrimSpace(observation) == "" {
			continue
		}
		response.Evidence = append(response.Evidence, assistant.Evidence{
			Kind:      assistant.EvidenceKindObservation,
			Summary:   summarizeOutput(observation),
			Detail:    strings.TrimSpace(observation),
			CreatedAt: r.now().UTC(),
		})
	}

	for _, toolRun := range result.ToolRuns {
		response.ToolCalls = append(response.ToolCalls, assistant.ToolCall{
			ID:            strings.TrimSpace(toolRun.ID),
			ToolName:      strings.TrimSpace(toolRun.Name),
			InputSummary:  strings.TrimSpace(toolRun.InputSummary),
			OutputSummary: strings.TrimSpace(toolRun.OutputSummary),
			StartedAt:     normalizeTime(toolRun.StartedAt, r.now()),
			FinishedAt:    normalizeTime(toolRun.FinishedAt, r.now()),
		})
		response.Evidence = append(response.Evidence, assistant.Evidence{
			Kind:      assistant.EvidenceKindToolCall,
			Summary:   summarizeOutput(fmt.Sprintf("%s: %s", toolRun.Name, toolRun.OutputSummary)),
			Detail:    strings.TrimSpace(fmt.Sprintf("Tool %s\nInput: %s\nOutput: %s", toolRun.Name, toolRun.InputSummary, toolRun.OutputSummary)),
			CreatedAt: normalizeTime(toolRun.FinishedAt, r.now()),
		})
	}

	for _, step := range result.BrowserSteps {
		response.WebSteps = append(response.WebSteps, assistant.WebStep{
			Title:         step.Title,
			URL:           step.URL,
			Summary:       strings.TrimSpace(step.Summary),
			ActionName:    strings.TrimSpace(step.Action.Name),
			ActionTarget:  strings.TrimSpace(step.Action.Target),
			ActionRef:     strings.TrimSpace(step.Action.Ref),
			ActionValue:   strings.TrimSpace(step.Action.Value),
			ActionSession: strings.TrimSpace(step.Action.Session),
			OccurredAt:    normalizeTime(step.OccurredAt, r.now()),
		})
		response.Evidence = append(response.Evidence, assistant.Evidence{
			Kind:      assistant.EvidenceKindWebStep,
			Summary:   summarizeOutput(step.Summary),
			Detail:    strings.TrimSpace(browserEvidenceDetail(step)),
			CreatedAt: normalizeTime(step.OccurredAt, r.now()),
		})
		if strings.TrimSpace(step.ScreenshotPath) != "" {
			response.Artifacts = append(response.Artifacts, assistant.Artifact{
				Kind:      assistant.ArtifactKindScreenshot,
				Title:     firstNonEmpty(step.ScreenshotNote, step.Title),
				MIMEType:  "image/png",
				Path:      step.ScreenshotPath,
				Content:   "",
				SourceURL: step.URL,
				CreatedAt: normalizeTime(step.OccurredAt, r.now()),
			})
		}
	}

	return response, nil
}

func (r *CodexRuntime) Close() error {
	if r.executor == nil {
		return nil
	}
	return r.executor.Close()
}

func composePhasePrompt(request PhaseRequest) string {
	parts := []string{strings.TrimSpace(request.Prompt.System), strings.TrimSpace(request.Prompt.User)}
	if len(request.ResumeInput) > 0 {
		pairs := make([]string, 0, len(request.ResumeInput))
		for key, value := range request.ResumeInput {
			pairs = append(pairs, fmt.Sprintf("%s=%s", key, value))
		}
		parts = append(parts, "Resume input:\n"+strings.Join(pairs, "\n"))
	}
	return strings.TrimSpace(strings.Join(parts, "\n\n"))
}

func phaseTools(run assistant.Run, role assistant.AttemptRole) []string {
	if role == assistant.AttemptRoleProjectSelector {
		return []string{"shell", "filesystem", "project-selection", "schedule-management"}
	}
	if role == assistant.AttemptRoleGate {
		return []string{"stored-parent-run", "stored-artifacts", "stored-evidence", "stored-evaluations", "schedule-management"}
	}
	if role == assistant.AttemptRoleAnswer {
		return []string{"stored-parent-run", "stored-artifacts", "stored-evidence", "stored-evaluations", "schedule-management"}
	}
	if role == assistant.AttemptRoleContractor {
		return []string{"stored-plan", "schedule-management"}
	}
	if role == assistant.AttemptRoleReporter {
		return []string{"stored-artifacts", "stored-evidence", "stored-evaluations", "schedule-management"}
	}
	if role == assistant.AttemptRoleWikiIngest {
		return []string{"shell", "filesystem", "project-wiki", "stored-artifacts", "stored-evidence", "stored-evaluations"}
	}
	tools := append([]string{}, run.TaskSpec.ToolsAllowed...)
	tools = append(tools, "schedule-management")
	if role == assistant.AttemptRoleEvaluator {
		tools = append(tools, "stored-artifacts", "stored-evidence")
	}
	return tools
}

func browserEvidenceDetail(step AgentBrowserStep) string {
	parts := []string{
		fmt.Sprintf("Action: %s", step.Action.Name),
		fmt.Sprintf("Target: %s", firstNonEmpty(step.Action.Target, step.Action.Ref, "(none)")),
	}
	if strings.TrimSpace(step.URL) != "" {
		parts = append(parts, "URL: "+step.URL)
	}
	if strings.TrimSpace(step.Summary) != "" {
		parts = append(parts, "Summary: "+step.Summary)
	}
	if len(step.ObservedText) > 0 {
		parts = append(parts, "Observed text: "+strings.Join(step.ObservedText, " | "))
	}
	return strings.Join(parts, "\n")
}
