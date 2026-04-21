package wtl

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/siisee11/CodexVirtualAssistant/internal/agentmessage"
	"github.com/siisee11/CodexVirtualAssistant/internal/assistant"
	"github.com/siisee11/CodexVirtualAssistant/internal/config"
	"github.com/siisee11/CodexVirtualAssistant/internal/policy/gan"
	"github.com/siisee11/CodexVirtualAssistant/internal/prompting"
	"github.com/siisee11/CodexVirtualAssistant/internal/safety"
	"github.com/siisee11/CodexVirtualAssistant/internal/store"
	"github.com/siisee11/CodexVirtualAssistant/internal/wiki"
)

var ErrInvalidRunState = errors.New("wtl: invalid run state")

const maxContractRevisionAttempts = 3

type Repository interface {
	SaveRun(context.Context, assistant.Run) error
	AddRunEvent(context.Context, assistant.RunEvent) error
	AddAttempt(context.Context, assistant.Attempt) error
	AddArtifact(context.Context, assistant.Artifact) error
	AddEvidence(context.Context, assistant.Evidence) error
	AddEvaluation(context.Context, assistant.Evaluation) error
	AddToolCall(context.Context, assistant.ToolCall) error
	AddWebStep(context.Context, assistant.WebStep) error
	AddBrowserAction(context.Context, assistant.BrowserActionRecord) error
	AddWaitRequest(context.Context, assistant.WaitRequest) error
	GetRun(context.Context, string) (assistant.Run, error)
	GetRunRecord(context.Context, string) (store.RunRecord, error)
	ListBrowserActionsByProject(context.Context, string, time.Time, time.Time) ([]assistant.BrowserActionRecord, error)
	SaveScheduledRun(context.Context, assistant.ScheduledRun) error
}

type ProjectManager interface {
	SelectionRoot() string
	EnsureProject(assistant.ProjectContext) (assistant.ProjectContext, error)
}

type WikiManager interface {
	EnsureProjectScaffold(assistant.ProjectContext) error
	LoadContext(assistant.ProjectContext) (assistant.WikiContext, error)
	LintProject(assistant.ProjectContext) (wiki.LintReport, error)
}

type RunEngine struct {
	repo             Repository
	runtime          Runtime
	observer         Observer
	policy           gan.Policy
	projects         ProjectManager
	wiki             WikiManager
	messenger        agentmessage.Service
	automationSafety config.AutomationSafetyConfig
	now              func() time.Time
}

func NewRunEngine(repo Repository, runtime Runtime, observer Observer, policy gan.Policy, projects ProjectManager, wikiManager WikiManager, messenger agentmessage.Service, now func() time.Time) *RunEngine {
	if observer == nil {
		observer = noopObserver{}
	}
	if now == nil {
		now = time.Now
	}
	return &RunEngine{
		repo:      repo,
		runtime:   runtime,
		observer:  observer,
		policy:    policy,
		projects:  projects,
		wiki:      wikiManager,
		messenger: messenger,
		now:       now,
	}
}

func (e *RunEngine) SetAutomationSafetyConfig(cfg config.AutomationSafetyConfig) {
	e.automationSafety = cfg
}

func (e *RunEngine) Start(ctx context.Context, run assistant.Run) error {
	if err := run.Validate(); err != nil {
		return err
	}

	_, err := e.repo.GetRun(ctx, run.ID)
	switch {
	case err == nil:
		return fmt.Errorf("%w: run %s already exists", ErrInvalidRunState, run.ID)
	case !errors.Is(err, store.ErrNotFound):
		return err
	}

	if strings.TrimSpace(run.Project.Slug) != "" {
		if e.projects == nil {
			return fmt.Errorf("%w: explicit project requires project manager", ErrInvalidRunState)
		}
		projectSelection, err := e.projects.EnsureProject(run.Project)
		if err != nil {
			return fmt.Errorf("wtl: explicit project could not be prepared: %w", err)
		}
		run.Project = projectSelection
	}

	run.Status = assistant.RunStatusQueued
	run.Phase = assistant.RunPhaseQueued
	run.WaitingFor = nil
	run.LatestEvaluation = nil
	run.GateRoute = ""
	run.GateReason = ""
	run.GateDecidedAt = nil
	run.AttemptCount = 0
	run.CreatedAt = run.CreatedAt.UTC()
	run.UpdatedAt = run.CreatedAt
	if err := e.repo.SaveRun(ctx, run); err != nil {
		return err
	}
	if err := e.publishEvent(ctx, run.ID, assistant.EventTypeRunCreated, assistant.RunPhaseQueued, "Run created from the user request."); err != nil {
		return err
	}

	return e.continueRun(ctx, run.ID, assistant.AttemptRoleGate, nil)
}

func (e *RunEngine) Resume(ctx context.Context, runID string, input map[string]string) error {
	record, err := e.repo.GetRunRecord(ctx, runID)
	if err != nil {
		return err
	}
	if record.Run.Status != assistant.RunStatusWaiting {
		return fmt.Errorf("%w: run %s is not waiting", ErrInvalidRunState, runID)
	}
	if record.Run.Status == assistant.RunStatusCancelled {
		return fmt.Errorf("%w: run %s is cancelled", ErrInvalidRunState, runID)
	}

	role := assistant.AttemptRoleGate
	if len(record.Attempts) > 0 {
		role = record.Attempts[len(record.Attempts)-1].Role
	}

	record.Run.WaitingFor = nil
	record.Run.UpdatedAt = e.now().UTC()
	if err := e.repo.SaveRun(ctx, record.Run); err != nil {
		return err
	}
	if err := e.publishEvent(ctx, runID, assistant.EventTypePhaseChanged, record.Run.Phase, "External input received and the run resumed."); err != nil {
		return err
	}
	return e.continueRun(ctx, runID, role, input)
}

func (e *RunEngine) Cancel(ctx context.Context, runID string) error {
	run, err := e.repo.GetRun(ctx, runID)
	if err != nil {
		return err
	}
	if isTerminalStatus(run.Status) {
		return nil
	}

	now := e.now().UTC()
	run.Status = assistant.RunStatusCancelled
	run.Phase = assistant.RunPhaseCancelled
	run.WaitingFor = nil
	run.UpdatedAt = now
	if err := e.repo.SaveRun(ctx, run); err != nil {
		return err
	}
	return e.publishEvent(ctx, run.ID, assistant.EventTypePhaseChanged, assistant.RunPhaseCancelled, "Run cancelled.")
}

func (e *RunEngine) continueRun(ctx context.Context, runID string, role assistant.AttemptRole, resumeInput map[string]string) error {
	for {
		record, err := e.repo.GetRunRecord(ctx, runID)
		if err != nil {
			return err
		}
		if isTerminalStatus(record.Run.Status) {
			return nil
		}

		switch role {
		case assistant.AttemptRoleGate:
			if err := e.executeGate(ctx, &record, resumeInput); err != nil {
				return err
			}
			record, err = e.repo.GetRunRecord(ctx, runID)
			if err != nil {
				return err
			}
			if record.Run.Status == assistant.RunStatusWaiting || isTerminalStatus(record.Run.Status) {
				return nil
			}
			if record.Run.GateRoute == assistant.RunRouteAnswer {
				role = assistant.AttemptRoleAnswer
			} else if strings.TrimSpace(record.Run.Project.Slug) != "" {
				role = assistant.AttemptRolePlanner
			} else {
				role = assistant.AttemptRoleProjectSelector
			}
			resumeInput = nil
		case assistant.AttemptRoleAnswer:
			if err := e.executeAnswer(ctx, &record, resumeInput); err != nil {
				return err
			}
			record, err = e.repo.GetRunRecord(ctx, runID)
			if err != nil {
				return err
			}
			if record.Run.Status == assistant.RunStatusWaiting || isTerminalStatus(record.Run.Status) {
				return nil
			}
			role = assistant.AttemptRoleScheduler
			resumeInput = nil
		case assistant.AttemptRoleProjectSelector:
			if err := e.executeProjectSelector(ctx, &record, resumeInput); err != nil {
				return err
			}
			record, err = e.repo.GetRunRecord(ctx, runID)
			if err != nil {
				return err
			}
			if record.Run.Status == assistant.RunStatusWaiting || isTerminalStatus(record.Run.Status) {
				return nil
			}
			role = assistant.AttemptRolePlanner
			resumeInput = nil
		case assistant.AttemptRolePlanner:
			if err := e.executePlanner(ctx, &record, resumeInput); err != nil {
				return err
			}
			record, err = e.repo.GetRunRecord(ctx, runID)
			if err != nil {
				return err
			}
			if record.Run.Status == assistant.RunStatusWaiting || isTerminalStatus(record.Run.Status) {
				return nil
			}
			role = assistant.AttemptRoleContractor
			resumeInput = nil
		case assistant.AttemptRoleContractor:
			advance, err := e.executeContract(ctx, &record, resumeInput)
			if err != nil {
				return err
			}
			record, err = e.repo.GetRunRecord(ctx, runID)
			if err != nil {
				return err
			}
			if record.Run.Status == assistant.RunStatusWaiting || isTerminalStatus(record.Run.Status) {
				return nil
			}
			if advance {
				role = assistant.AttemptRoleGenerator
			} else {
				role = assistant.AttemptRoleContractor
			}
			resumeInput = nil
		case assistant.AttemptRoleGenerator:
			if err := e.executeGenerator(ctx, &record, resumeInput); err != nil {
				return err
			}
			record, err = e.repo.GetRunRecord(ctx, runID)
			if err != nil {
				return err
			}
			if record.Run.Status == assistant.RunStatusWaiting || isTerminalStatus(record.Run.Status) {
				return nil
			}
			role = assistant.AttemptRoleEvaluator
			resumeInput = nil
		case assistant.AttemptRoleEvaluator:
			directive, err := e.executeEvaluator(ctx, &record, resumeInput)
			if err != nil {
				return err
			}
			record, err = e.repo.GetRunRecord(ctx, runID)
			if err != nil {
				return err
			}
			switch directive {
			case DirectiveWait:
				return nil
			case DirectiveFail:
				return nil
			case DirectiveComplete:
				role = assistant.AttemptRoleScheduler
				resumeInput = nil
			case DirectiveContinue:
				role = assistant.AttemptRoleScheduler
				resumeInput = nil
			case DirectiveRetry:
				role = assistant.AttemptRoleGenerator
				resumeInput = nil
			default:
				return fmt.Errorf("wtl: unsupported directive %q", directive)
			}
		case assistant.AttemptRoleScheduler:
			if err := e.executeScheduler(ctx, &record, resumeInput); err != nil {
				return err
			}
			record, err = e.repo.GetRunRecord(ctx, runID)
			if err != nil {
				return err
			}
			if record.Run.Status == assistant.RunStatusWaiting || isTerminalStatus(record.Run.Status) {
				return nil
			}
			role = assistant.AttemptRoleWikiIngest
			resumeInput = nil
		case assistant.AttemptRoleWikiIngest:
			if err := e.executeWikiIngest(ctx, &record); err != nil {
				return err
			}
			record, err = e.repo.GetRunRecord(ctx, runID)
			if err != nil {
				return err
			}
			if record.Run.Status == assistant.RunStatusWaiting || isTerminalStatus(record.Run.Status) {
				return nil
			}
			role = assistant.AttemptRoleReporter
			resumeInput = nil
		case assistant.AttemptRoleReporter:
			directive, err := e.executeReporter(ctx, &record, resumeInput)
			if err != nil {
				return err
			}
			switch directive {
			case DirectiveWait, DirectiveComplete, DirectiveFail:
				return nil
			default:
				return fmt.Errorf("wtl: unsupported reporter directive %q", directive)
			}
		default:
			return fmt.Errorf("wtl: unsupported role %q", role)
		}
	}
}

func (e *RunEngine) executeGate(ctx context.Context, record *store.RunRecord, resumeInput map[string]string) error {
	now := e.now().UTC()
	record.Run.Status = assistant.RunStatusGating
	record.Run.Phase = assistant.RunPhaseGating
	record.Run.WaitingFor = nil
	record.Run.UpdatedAt = now
	if err := e.repo.SaveRun(ctx, record.Run); err != nil {
		return err
	}
	if err := e.publishEvent(ctx, record.Run.ID, assistant.EventTypePhaseChanged, assistant.RunPhaseGating, "Gating the run to decide between answer mode and workflow execution."); err != nil {
		return err
	}

	parentContext, err := e.parentContextForRun(ctx, record.Run)
	if err != nil {
		return e.failRun(ctx, record.Run, fmt.Sprintf("Gate could not load parent run context: %v", err), err)
	}
	prompt := prompting.BuildGatePrompt(prompting.GateInput{
		Run:           record.Run,
		ParentContext: parentContext,
	})

	attempt, response, err := e.executeAttempt(ctx, record.Run, record.Attempts, assistant.AttemptRoleGate, prompt, "", resumeInput, record.Run.Project.WorkspaceDir)
	if err != nil {
		return e.failRun(ctx, record.Run, fmt.Sprintf("Gate execution failed: %v", err), err)
	}
	if response.WaitRequest != nil {
		return e.enterWaiting(ctx, record.Run, attempt, response.WaitRequest)
	}

	route, reason, summary, err := prompting.DecodeGateOutput([]byte(response.Output))
	if err != nil {
		return e.failRun(ctx, record.Run, fmt.Sprintf("Gate output could not be decoded: %v", err), err)
	}

	decidedAt := e.now().UTC()
	record.Run.GateRoute = route
	record.Run.GateReason = reason
	record.Run.GateDecidedAt = &decidedAt
	record.Run.AttemptCount = len(record.Attempts) + 1
	record.Run.UpdatedAt = decidedAt
	if err := e.repo.SaveRun(ctx, record.Run); err != nil {
		return err
	}

	switch route {
	case assistant.RunRouteAnswer:
		return e.publishEvent(ctx, record.Run.ID, assistant.EventTypePhaseChanged, assistant.RunPhaseAnswering, firstNonEmpty(summary, "Gate selected answer mode for this run."))
	case assistant.RunRouteWorkflow:
		defaultSummary := "Gate selected workflow mode. Starting project selection."
		if strings.TrimSpace(record.Run.Project.Slug) != "" {
			defaultSummary = fmt.Sprintf("Gate selected workflow mode with preselected project %s. Starting planning.", record.Run.Project.Slug)
		}
		return e.publishEvent(ctx, record.Run.ID, assistant.EventTypePhaseChanged, assistant.RunPhaseSelectingProject, firstNonEmpty(summary, defaultSummary))
	default:
		return e.failRun(ctx, record.Run, fmt.Sprintf("Gate selected an unsupported route %q.", route), ErrInvalidRunState)
	}
}

func (e *RunEngine) executeAnswer(ctx context.Context, record *store.RunRecord, resumeInput map[string]string) error {
	now := e.now().UTC()
	record.Run.Status = assistant.RunStatusAnswering
	record.Run.Phase = assistant.RunPhaseAnswering
	record.Run.WaitingFor = nil
	record.Run.UpdatedAt = now
	if err := e.repo.SaveRun(ctx, record.Run); err != nil {
		return err
	}
	if err := e.publishEvent(ctx, record.Run.ID, assistant.EventTypePhaseChanged, assistant.RunPhaseAnswering, "Preparing a read-oriented answer using available run context and evidence."); err != nil {
		return err
	}

	parentContext, err := e.parentContextForRun(ctx, record.Run)
	if err != nil {
		return e.failRun(ctx, record.Run, fmt.Sprintf("Answer phase could not load parent run context: %v", err), err)
	}
	if strings.TrimSpace(record.Run.Project.Slug) == "" && parentContext != nil && strings.TrimSpace(parentContext.Project.Slug) != "" {
		record.Run.Project = parentContext.Project
		record.Run.UpdatedAt = e.now().UTC()
		if err := e.repo.SaveRun(ctx, record.Run); err != nil {
			return err
		}
	}
	prompt := prompting.BuildAnswerPrompt(prompting.AnswerInput{
		Run:           record.Run,
		ParentContext: parentContext,
	})

	attempt, response, err := e.executeAttempt(ctx, record.Run, record.Attempts, assistant.AttemptRoleAnswer, prompt, "", resumeInput, record.Run.Project.WorkspaceDir)
	if err != nil && isTransientPhaseExecutionError(err) {
		latestRecord, reloadErr := e.repo.GetRunRecord(ctx, record.Run.ID)
		if reloadErr != nil {
			return e.failRun(ctx, record.Run, fmt.Sprintf("Answer phase execution failed: %v", err), err)
		}
		attempt, response, err = e.executeAttempt(ctx, latestRecord.Run, latestRecord.Attempts, assistant.AttemptRoleAnswer, prompt, "", resumeInput, latestRecord.Run.Project.WorkspaceDir)
	}
	if err != nil {
		return e.failRun(ctx, record.Run, fmt.Sprintf("Answer phase execution failed: %v", err), err)
	}
	if response.WaitRequest != nil {
		return e.enterWaiting(ctx, record.Run, attempt, response.WaitRequest)
	}

	summary := strings.TrimSpace(response.Summary)
	output := strings.TrimSpace(response.Output)
	decoded, decodeErr := prompting.DecodeAnswerOutput([]byte(response.Output))
	if decodeErr == nil {
		if decoded.NeedsUserInput {
			return e.enterWaiting(ctx, record.Run, attempt, &assistant.WaitRequest{
				Kind:        assistant.WaitKind(decoded.WaitKind),
				Title:       decoded.WaitTitle,
				Prompt:      decoded.WaitPrompt,
				RiskSummary: decoded.WaitRiskSummary,
			})
		}
		summary = firstNonEmpty(decoded.Summary, summary)
		output = firstNonEmpty(decoded.Output, output)
	}

	if output == "" {
		return e.failRun(ctx, record.Run, "Answer phase completed without an answer output.", ErrInvalidRunState)
	}
	if len(response.Artifacts) == 0 {
		now := e.now().UTC()
		if err := e.repo.AddArtifact(ctx, assistant.Artifact{
			ID:        assistant.NewID("artifact", now),
			RunID:     record.Run.ID,
			AttemptID: attempt.ID,
			Kind:      assistant.ArtifactKindReport,
			Title:     "Assistant answer",
			MIMEType:  "text/markdown",
			Content:   output,
			CreatedAt: now,
		}); err != nil {
			return err
		}
	}

	record.Run.AttemptCount = len(record.Attempts) + 1
	record.Run.UpdatedAt = e.now().UTC()
	if err := e.repo.SaveRun(ctx, record.Run); err != nil {
		return err
	}
	return e.publishEvent(ctx, record.Run.ID, assistant.EventTypePhaseChanged, assistant.RunPhaseScheduling, firstNonEmpty(summary, "Answer completed. Finalizing scheduled work and wiki memory."))
}

func (e *RunEngine) executeProjectSelector(ctx context.Context, record *store.RunRecord, resumeInput map[string]string) error {
	if e.projects == nil {
		return e.failRun(ctx, record.Run, "Project selection is not configured.", ErrInvalidRunState)
	}
	now := e.now().UTC()
	record.Run.Status = assistant.RunStatusSelectingProject
	record.Run.Phase = assistant.RunPhaseSelectingProject
	record.Run.WaitingFor = nil
	record.Run.UpdatedAt = now
	if err := e.repo.SaveRun(ctx, record.Run); err != nil {
		return err
	}
	if err := e.publishEvent(ctx, record.Run.ID, assistant.EventTypePhaseChanged, assistant.RunPhaseSelectingProject, "Selecting the project workspace for this request."); err != nil {
		return err
	}

	prompt := prompting.BuildProjectSelectorPrompt(prompting.ProjectSelectorInput{
		UserRequestRaw: record.Run.UserRequestRaw,
	})

	attempt, response, err := e.executeAttempt(ctx, record.Run, record.Attempts, assistant.AttemptRoleProjectSelector, prompt, "", resumeInput, e.projects.SelectionRoot())
	if err != nil {
		return e.failRun(ctx, record.Run, fmt.Sprintf("Project selection failed: %v", err), err)
	}
	if response.WaitRequest != nil {
		return e.enterWaiting(ctx, record.Run, attempt, response.WaitRequest)
	}

	projectSelection, summary, err := prompting.DecodeProjectSelectorOutput([]byte(response.Output))
	if err != nil {
		return e.failRun(ctx, record.Run, fmt.Sprintf("Project selector output could not be decoded: %v", err), err)
	}
	projectSelection, err = e.projects.EnsureProject(projectSelection)
	if err != nil {
		return e.failRun(ctx, record.Run, fmt.Sprintf("Selected project could not be prepared: %v", err), err)
	}

	record.Run.Project = projectSelection
	record.Run.UpdatedAt = e.now().UTC()
	if err := e.repo.SaveRun(ctx, record.Run); err != nil {
		return err
	}
	return e.publishEvent(ctx, record.Run.ID, assistant.EventTypePhaseChanged, assistant.RunPhasePlanning, firstNonEmpty(summary, fmt.Sprintf("Project %s selected. Starting planning.", projectSelection.Slug)))
}

func (e *RunEngine) parentContextForRun(ctx context.Context, run assistant.Run) (*prompting.ParentRunContext, error) {
	parentRunID := strings.TrimSpace(run.ParentRunID)
	if parentRunID == "" {
		return nil, nil
	}
	record, err := e.repo.GetRunRecord(ctx, parentRunID)
	if err != nil {
		return nil, err
	}
	return &prompting.ParentRunContext{
		RunID:          record.Run.ID,
		UserRequestRaw: record.Run.UserRequestRaw,
		Project:        record.Run.Project,
		Wiki:           e.loadWikiContext(record.Run.Project),
		Summary:        parentRunSummary(record),
		Artifacts:      append([]assistant.Artifact{}, record.Artifacts...),
		Evidence:       append([]assistant.Evidence{}, record.Evidence...),
	}, nil
}

func parentRunSummary(record store.RunRecord) string {
	if record.Run.LatestEvaluation != nil && strings.TrimSpace(record.Run.LatestEvaluation.Summary) != "" {
		return record.Run.LatestEvaluation.Summary
	}
	if len(record.Attempts) > 0 {
		latest := record.Attempts[len(record.Attempts)-1]
		if strings.TrimSpace(latest.OutputSummary) != "" {
			return latest.OutputSummary
		}
	}
	if len(record.Events) > 0 {
		latest := record.Events[len(record.Events)-1]
		if strings.TrimSpace(latest.Summary) != "" {
			return latest.Summary
		}
	}
	return ""
}

func (e *RunEngine) executePlanner(ctx context.Context, record *store.RunRecord, resumeInput map[string]string) error {
	now := e.now().UTC()
	record.Run.Status = assistant.RunStatusPlanning
	record.Run.Phase = assistant.RunPhasePlanning
	record.Run.WaitingFor = nil
	record.Run.UpdatedAt = now
	if err := e.repo.SaveRun(ctx, record.Run); err != nil {
		return err
	}
	if err := e.publishEvent(ctx, record.Run.ID, assistant.EventTypePhaseChanged, assistant.RunPhasePlanning, "Planning the task into a structured TaskSpec."); err != nil {
		return err
	}

	prompt := prompting.BuildPlannerPrompt(prompting.PlannerInput{
		UserRequestRaw:        record.Run.UserRequestRaw,
		MaxGenerationAttempts: record.Run.MaxGenerationAttempts,
		Project:               record.Run.Project,
		Wiki:                  e.loadWikiContext(record.Run.Project),
		AutomationSafety:      e.automationSafety,
	})

	attempt, response, err := e.executeAttempt(ctx, record.Run, record.Attempts, assistant.AttemptRolePlanner, prompt, "", resumeInput, record.Run.Project.WorkspaceDir)
	if err != nil {
		return e.failRun(ctx, record.Run, fmt.Sprintf("Planner execution failed: %v", err), err)
	}
	if response.WaitRequest != nil {
		return e.enterWaiting(ctx, record.Run, attempt, response.WaitRequest)
	}

	spec, err := prompting.DecodePlannerOutput([]byte(response.Output), prompting.PlannerInput{
		UserRequestRaw:        record.Run.UserRequestRaw,
		MaxGenerationAttempts: record.Run.MaxGenerationAttempts,
		Project:               record.Run.Project,
		AutomationSafety:      e.automationSafety,
	})
	if err != nil {
		return e.failRun(ctx, record.Run, fmt.Sprintf("Planner output could not be decoded: %v", err), err)
	}

	record.Run.TaskSpec = spec
	record.Run.AttemptCount = len(record.Attempts) + 1
	record.Run.UpdatedAt = e.now().UTC()
	if err := e.repo.SaveRun(ctx, record.Run); err != nil {
		return err
	}
	return e.publishEvent(ctx, record.Run.ID, assistant.EventTypePhaseChanged, assistant.RunPhaseContracting, "Planning complete. Negotiating the acceptance contract before generation starts.")
}

func (e *RunEngine) executeContract(ctx context.Context, record *store.RunRecord, resumeInput map[string]string) (bool, error) {
	now := e.now().UTC()
	record.Run.Status = assistant.RunStatusContracting
	record.Run.Phase = assistant.RunPhaseContracting
	record.Run.WaitingFor = nil
	record.Run.UpdatedAt = now
	if err := e.repo.SaveRun(ctx, record.Run); err != nil {
		return false, err
	}
	if err := e.publishEvent(ctx, record.Run.ID, assistant.EventTypePhaseChanged, assistant.RunPhaseContracting, contractPhaseSummary(record.Run)); err != nil {
		return false, err
	}

	prompt := prompting.BuildContractPrompt(prompting.ContractInput{
		Run: record.Run,
	})

	attempt, response, err := e.executeAttempt(ctx, record.Run, record.Attempts, assistant.AttemptRoleContractor, prompt, "", resumeInput, record.Run.Project.WorkspaceDir)
	if err != nil {
		return false, e.failRun(ctx, record.Run, fmt.Sprintf("Contract execution failed: %v", err), err)
	}
	if response.WaitRequest != nil {
		return false, e.enterWaiting(ctx, record.Run, attempt, response.WaitRequest)
	}

	contract, decision, err := prompting.DecodeContractOutput([]byte(response.Output), record.Run.TaskSpec)
	if err != nil {
		return false, e.failRun(ctx, record.Run, fmt.Sprintf("Contract output could not be decoded: %v", err), err)
	}

	record.Run.TaskSpec.AcceptanceContract = &contract
	record.Run.AttemptCount = len(record.Attempts) + 1
	record.Run.UpdatedAt = e.now().UTC()
	if err := e.repo.SaveRun(ctx, record.Run); err != nil {
		return false, err
	}

	switch decision {
	case prompting.ContractDecisionAgreed:
		if err := e.publishEvent(ctx, record.Run.ID, assistant.EventTypePhaseChanged, assistant.RunPhaseGenerating, "Acceptance contract agreed. Starting the first generation attempt."); err != nil {
			return false, err
		}
		return true, nil
	case prompting.ContractDecisionRevise:
		if countAttemptsByRole(record.Attempts, assistant.AttemptRoleContractor)+1 >= maxContractRevisionAttempts {
			return false, e.failRun(ctx, record.Run, firstNonEmpty(
				contract.RevisionNotes,
				fmt.Sprintf("Acceptance contract did not converge after %d revision attempts.", maxContractRevisionAttempts),
			), ErrInvalidRunState)
		}
		if err := e.publishEvent(ctx, record.Run.ID, assistant.EventTypePhaseChanged, assistant.RunPhaseContracting, firstNonEmpty(contract.RevisionNotes, "Acceptance contract needs another revision before generation starts.")); err != nil {
			return false, err
		}
		return false, nil
	case prompting.ContractDecisionFail:
		return false, e.failRun(ctx, record.Run, firstNonEmpty(contract.RevisionNotes, contract.Summary, "Acceptance contract could not be agreed."), ErrInvalidRunState)
	default:
		return false, fmt.Errorf("wtl: unknown contract decision %q", decision)
	}
}

func (e *RunEngine) executeGenerator(ctx context.Context, record *store.RunRecord, resumeInput map[string]string) error {
	if !e.policy.CanGenerate(record.Run) {
		return e.failRun(ctx, record.Run, "Generation cannot start before the acceptance contract is agreed.", ErrInvalidRunState)
	}

	now := e.now().UTC()
	record.Run.Status = assistant.RunStatusGenerating
	record.Run.Phase = assistant.RunPhaseGenerating
	record.Run.WaitingFor = nil
	record.Run.UpdatedAt = now
	if err := e.repo.SaveRun(ctx, record.Run); err != nil {
		return err
	}
	if err := e.publishEvent(ctx, record.Run.ID, assistant.EventTypePhaseChanged, assistant.RunPhaseGenerating, generatorPhaseSummary(record)); err != nil {
		return err
	}

	critique := ""
	if record.Run.LatestEvaluation != nil {
		critique = record.Run.LatestEvaluation.NextActionForGenerator
	}

	prompt := prompting.BuildGeneratorPrompt(prompting.GeneratorInput{
		Run:           record.Run,
		Attempt:       assistant.Attempt{},
		PriorCritique: critique,
	})

	attempt, response, err := e.executeAttempt(ctx, record.Run, record.Attempts, assistant.AttemptRoleGenerator, prompt, critique, resumeInput, record.Run.Project.WorkspaceDir)
	if err != nil {
		return e.failRun(ctx, record.Run, fmt.Sprintf("Generator execution failed: %v", err), err)
	}
	if response.WaitRequest != nil {
		return e.enterWaiting(ctx, record.Run, attempt, response.WaitRequest)
	}

	record.Run.AttemptCount = len(record.Attempts) + 1
	record.Run.UpdatedAt = e.now().UTC()
	return e.repo.SaveRun(ctx, record.Run)
}

func (e *RunEngine) executeEvaluator(ctx context.Context, record *store.RunRecord, resumeInput map[string]string) (Directive, error) {
	if !e.policy.HasAcceptedContract(record.Run) {
		return "", e.failRun(ctx, record.Run, "Evaluation cannot start before the acceptance contract is agreed.", ErrInvalidRunState)
	}

	now := e.now().UTC()
	record.Run.Status = assistant.RunStatusEvaluating
	record.Run.Phase = assistant.RunPhaseEvaluating
	record.Run.WaitingFor = nil
	record.Run.UpdatedAt = now
	if err := e.repo.SaveRun(ctx, record.Run); err != nil {
		return "", err
	}
	if err := e.publishEvent(ctx, record.Run.ID, assistant.EventTypePhaseChanged, assistant.RunPhaseEvaluating, "Evaluating the latest generated result against the done definition."); err != nil {
		return "", err
	}

	generatorAttempt := latestAttemptByRole(record.Attempts, assistant.AttemptRoleGenerator)
	if generatorAttempt == nil {
		return "", e.failRun(ctx, record.Run, "Cannot evaluate because no generator attempt was found.", ErrInvalidRunState)
	}

	metrics, err := e.recentActivityMetrics(ctx, record.Run)
	if err != nil {
		return "", e.failRun(ctx, record.Run, fmt.Sprintf("Could not compute recent browser activity metrics: %v", err), err)
	}

	prompt := prompting.BuildEvaluatorPrompt(prompting.EvaluatorInput{
		Run:            record.Run,
		Attempt:        *generatorAttempt,
		Artifacts:      record.Artifacts,
		Evidence:       record.Evidence,
		RecentActivity: metrics,
	})

	attempt, response, err := e.executeAttempt(ctx, record.Run, record.Attempts, assistant.AttemptRoleEvaluator, prompt, "", resumeInput, record.Run.Project.WorkspaceDir)
	if err != nil {
		return "", e.failRun(ctx, record.Run, fmt.Sprintf("Evaluator execution failed: %v", err), err)
	}
	if response.WaitRequest != nil {
		return DirectiveWait, e.enterWaiting(ctx, record.Run, attempt, response.WaitRequest)
	}

	evaluation, err := prompting.DecodeEvaluatorOutput([]byte(response.Output), record.Run.ID, attempt.ID, e.now())
	if err != nil {
		return "", e.failRun(ctx, record.Run, fmt.Sprintf("Evaluator output could not be decoded: %v", err), err)
	}
	safetyCheck := evaluateAutomationSafetyCompliance(record.Run.TaskSpec.AutomationSafety, record.BrowserActions, record.Evidence, metrics, evaluation.Passed)
	if len(safetyCheck.Findings) > 0 {
		evaluation = mergeAutomationSafetyFindingsIntoEvaluation(evaluation, safetyCheck)
	}
	if err := e.repo.AddEvaluation(ctx, evaluation); err != nil {
		return "", err
	}
	record.Run.LatestEvaluation = &evaluation
	if err := e.publishEvent(ctx, record.Run.ID, assistant.EventTypeEvaluation, assistant.RunPhaseEvaluating, evaluation.Summary); err != nil {
		return "", err
	}
	if safetyCheck.HardBlock {
		summary := firstNonEmpty(safetyCheck.HardBlockSummary(), "Automation safety hard-limit violation blocked the run.")
		if err := e.failRun(ctx, record.Run, summary, nil); err != nil {
			return "", err
		}
		return DirectiveFail, nil
	}

	updatedRecord, err := e.repo.GetRunRecord(ctx, record.Run.ID)
	if err != nil {
		return "", err
	}
	switch e.policy.DecideEvaluation(updatedRecord.Run, updatedRecord.Attempts, evaluation) {
	case gan.EvaluationDecisionComplete:
		if err := e.publishEvent(ctx, updatedRecord.Run.ID, assistant.EventTypePhaseChanged, assistant.RunPhaseScheduling, firstNonEmpty(evaluation.Summary, "Evaluation passed. Finalizing scheduled work and wiki memory.")); err != nil {
			return "", err
		}
		return DirectiveComplete, nil
	case gan.EvaluationDecisionFail:
		return DirectiveFail, e.exhaustRun(ctx, updatedRecord.Run, evaluation.Summary)
	case gan.EvaluationDecisionRetry:
		if err := e.publishEvent(ctx, updatedRecord.Run.ID, assistant.EventTypePhaseChanged, assistant.RunPhaseGenerating, "Evaluation requested another generation attempt with critique."); err != nil {
			return "", err
		}
		return DirectiveRetry, nil
	default:
		return "", fmt.Errorf("wtl: unknown evaluation decision")
	}
}

func (e *RunEngine) executeScheduler(ctx context.Context, record *store.RunRecord, resumeInput map[string]string) error {
	now := e.now().UTC()
	record.Run.Status = assistant.RunStatusScheduling
	record.Run.Phase = assistant.RunPhaseScheduling
	record.Run.WaitingFor = nil
	record.Run.UpdatedAt = now
	if err := e.repo.SaveRun(ctx, record.Run); err != nil {
		return err
	}
	if err := e.publishEvent(ctx, record.Run.ID, assistant.EventTypePhaseChanged, assistant.RunPhaseScheduling, "Finalizing any deferred work items before the final report."); err != nil {
		return err
	}

	if record.Run.TaskSpec.SchedulePlan == nil || len(record.Run.TaskSpec.SchedulePlan.Entries) == 0 {
		return e.publishEvent(ctx, record.Run.ID, assistant.EventTypePhaseChanged, assistant.RunPhaseWikiIngesting, "No deferred work was scheduled. Updating project wiki memory.")
	}

	metrics, err := e.recentActivityMetrics(ctx, record.Run)
	if err != nil {
		return e.failRun(ctx, record.Run, fmt.Sprintf("Could not compute recent browser activity metrics: %v", err), err)
	}

	prompt := prompting.BuildSchedulerPrompt(prompting.SchedulerInput{
		Run:            record.Run,
		Artifacts:      record.Artifacts,
		Evidence:       record.Evidence,
		RecentActivity: metrics,
	})

	attempt, response, err := e.executeAttempt(ctx, record.Run, record.Attempts, assistant.AttemptRoleScheduler, prompt, "", resumeInput, record.Run.Project.WorkspaceDir)
	if err != nil {
		return e.failRun(ctx, record.Run, fmt.Sprintf("Scheduler phase execution failed: %v", err), err)
	}
	if response.WaitRequest != nil {
		return e.enterWaiting(ctx, record.Run, attempt, response.WaitRequest)
	}

	entries, err := prompting.DecodeSchedulerOutput([]byte(response.Output))
	if err != nil {
		return e.failRun(ctx, record.Run, fmt.Sprintf("Scheduler output could not be decoded: %v", err), err)
	}

	scheduledTimes := make([]time.Time, 0, len(entries))
	for idx, entry := range entries {
		scheduledFor, err := assistant.ParseScheduledFor(entry.ScheduledFor, now)
		if err != nil {
			return e.failRun(ctx, record.Run, fmt.Sprintf("Scheduler entry %d could not parse scheduled_for: %v", idx+1, err), err)
		}
		scheduledTimes = append(scheduledTimes, scheduledFor)
	}

	if summary := validateSchedulerAutomationSafety(record.Run.TaskSpec.AutomationSafety, metrics, now, scheduledTimes); summary != "" {
		return e.failRun(ctx, record.Run, summary, nil)
	}

	createdIDs := make([]string, 0, len(entries))
	for idx, entry := range entries {
		scheduledRun := assistant.ScheduledRun{
			ID:                    assistant.NewID("scheduled", now.Add(time.Duration(idx)*time.Millisecond)),
			ChatID:                record.Run.ChatID,
			ParentRunID:           record.Run.ID,
			UserRequestRaw:        strings.TrimSpace(entry.Prompt),
			MaxGenerationAttempts: record.Run.MaxGenerationAttempts,
			ScheduledFor:          scheduledTimes[idx],
			Status:                assistant.ScheduledRunStatusPending,
			CreatedAt:             now.Add(time.Duration(idx) * time.Millisecond),
		}
		if err := e.repo.SaveScheduledRun(ctx, scheduledRun); err != nil {
			return err
		}
		createdIDs = append(createdIDs, scheduledRun.ID)
	}

	record.Run.AttemptCount = len(record.Attempts) + 1
	record.Run.UpdatedAt = e.now().UTC()
	if err := e.repo.SaveRun(ctx, record.Run); err != nil {
		return err
	}
	if err := e.publishEventWithData(ctx, record.Run.ID, assistant.EventTypeScheduleCreated, assistant.RunPhaseScheduling, fmt.Sprintf("Created %d scheduled run(s).", len(createdIDs)), map[string]any{
		"scheduled_run_ids": createdIDs,
		"count":             len(createdIDs),
	}); err != nil {
		return err
	}
	return e.publishEvent(ctx, record.Run.ID, assistant.EventTypePhaseChanged, assistant.RunPhaseWikiIngesting, fmt.Sprintf("Created %d scheduled run(s). Updating project wiki memory.", len(createdIDs)))
}

func (e *RunEngine) executeWikiIngest(ctx context.Context, record *store.RunRecord) error {
	enterReporting := func(summary string) error {
		now := e.now().UTC()
		record.Run.Status = assistant.RunStatusReporting
		record.Run.Phase = assistant.RunPhaseReporting
		record.Run.WaitingFor = nil
		record.Run.UpdatedAt = now
		if err := e.repo.SaveRun(ctx, record.Run); err != nil {
			return err
		}
		return e.publishEvent(ctx, record.Run.ID, assistant.EventTypePhaseChanged, assistant.RunPhaseReporting, summary)
	}

	if e.wiki == nil || strings.TrimSpace(record.Run.Project.Slug) == "" || strings.TrimSpace(record.Run.Project.Slug) == "no_project" || strings.TrimSpace(record.Run.Project.WorkspaceDir) == "" {
		return enterReporting("Wiki ingest skipped. Delivering the final report.")
	}

	now := e.now().UTC()
	record.Run.Status = assistant.RunStatusWikiIngesting
	record.Run.Phase = assistant.RunPhaseWikiIngesting
	record.Run.WaitingFor = nil
	record.Run.UpdatedAt = now
	if err := e.repo.SaveRun(ctx, record.Run); err != nil {
		return err
	}
	if err := e.publishEvent(ctx, record.Run.ID, assistant.EventTypePhaseChanged, assistant.RunPhaseWikiIngesting, "Updating project wiki memory from the run results."); err != nil {
		return err
	}

	latestRecord, err := e.repo.GetRunRecord(ctx, record.Run.ID)
	if err != nil {
		return err
	}
	projectCtx := latestRecord.Run.Project
	if err := e.wiki.EnsureProjectScaffold(projectCtx); err != nil {
		if eventErr := e.publishEvent(ctx, record.Run.ID, assistant.EventTypeReasoning, assistant.RunPhaseWikiIngesting, fmt.Sprintf("Project wiki scaffold validation failed before ingest: %v", err)); eventErr != nil {
			return eventErr
		}
		return enterReporting("Project wiki ingest validation failed. Continuing to the final report.")
	}

	prompt := prompting.BuildWikiIngestPrompt(prompting.WikiIngestInput{
		Run:              latestRecord.Run,
		Artifacts:        latestRecord.Artifacts,
		Evidence:         latestRecord.Evidence,
		ToolCalls:        latestRecord.ToolCalls,
		LatestEvaluation: latestRecord.Run.LatestEvaluation,
		Wiki:             e.loadWikiContext(projectCtx),
	})
	attempt, response, ingestErr := e.executeAttempt(ctx, latestRecord.Run, latestRecord.Attempts, assistant.AttemptRoleWikiIngest, prompt, "", nil, projectCtx.WorkspaceDir)
	if ingestErr != nil {
		if err := e.publishEvent(ctx, record.Run.ID, assistant.EventTypeReasoning, assistant.RunPhaseWikiIngesting, fmt.Sprintf("Project wiki ingest execution failed: %v", ingestErr)); err != nil {
			return err
		}
		return enterReporting("Project wiki ingest execution failed. Continuing to the final report.")
	}
	if response.WaitRequest != nil {
		return e.enterWaiting(ctx, latestRecord.Run, attempt, response.WaitRequest)
	}
	if err := e.wiki.EnsureProjectScaffold(projectCtx); err != nil {
		if eventErr := e.publishEvent(ctx, record.Run.ID, assistant.EventTypeReasoning, assistant.RunPhaseWikiIngesting, fmt.Sprintf("Project wiki scaffold validation failed after ingest: %v", err)); eventErr != nil {
			return eventErr
		}
		return enterReporting("Project wiki ingest validation failed. Continuing to the final report.")
	}
	lintReport, lintErr := e.wiki.LintProject(projectCtx)
	if lintErr != nil {
		if err := e.publishEvent(ctx, record.Run.ID, assistant.EventTypeReasoning, assistant.RunPhaseWikiIngesting, fmt.Sprintf("Project wiki lint validation failed: %v", lintErr)); err != nil {
			return err
		}
		return enterReporting("Project wiki ingest lint validation failed. Continuing to the final report.")
	}
	ingestOutput, decodeErr := prompting.DecodeWikiIngestOutput([]byte(response.Output))
	if decodeErr != nil {
		ingestOutput.Summary = firstNonEmpty(response.Summary, "Project wiki memory updated.")
	}
	changedPages := uniqueTrimmedStrings(append(ingestOutput.ChangedPages, lintReport.ReportPath, "open-questions.md", "index.md", "overview.md"))
	if err := e.publishEventWithData(ctx, record.Run.ID, assistant.EventTypeReasoning, assistant.RunPhaseWikiIngesting, firstNonEmpty(ingestOutput.Summary, response.Summary, "Project wiki memory updated."), map[string]any{
		"changed_pages":     changedPages,
		"validation_report": lintReport.ReportPath,
		"validation_notes":  ingestOutput.ValidationNotes,
		"lint_findings":     len(lintReport.Findings),
	}); err != nil {
		return err
	}
	return enterReporting("Project wiki memory updated and validated. Delivering the final report.")
}

func (e *RunEngine) executeReporter(ctx context.Context, record *store.RunRecord, resumeInput map[string]string) (Directive, error) {
	if e.messenger == nil {
		return DirectiveFail, e.failRun(ctx, record.Run, "Report delivery is not configured.", ErrInvalidRunState)
	}

	var directive Directive
	var phaseErr error
	err := e.messenger.WithChatAccount(ctx, record.Run.ChatID, func(account agentmessage.ChatAccount) error {
		now := e.now().UTC()
		record.Run.Status = assistant.RunStatusReporting
		record.Run.Phase = assistant.RunPhaseReporting
		record.Run.WaitingFor = nil
		record.Run.UpdatedAt = now
		if err := e.repo.SaveRun(ctx, record.Run); err != nil {
			return err
		}
		if err := e.publishEvent(ctx, record.Run.ID, assistant.EventTypePhaseChanged, assistant.RunPhaseReporting, "Delivering the final report through agent-message."); err != nil {
			return err
		}

		prompt := prompting.BuildReportPrompt(prompting.ReportInput{
			Run:                 record.Run,
			Artifacts:           record.Artifacts,
			Evidence:            record.Evidence,
			ToolCalls:           record.ToolCalls,
			LatestEvaluation:    record.Run.LatestEvaluation,
			ChatAccountUsername: account.Name,
			MasterUsername:      account.Master,
		})

		attempt, response, err := e.executeAttempt(ctx, record.Run, record.Attempts, assistant.AttemptRoleReporter, prompt, "", resumeInput, record.Run.Project.WorkspaceDir)
		if err != nil {
			phaseErr = e.failRun(ctx, record.Run, fmt.Sprintf("Report phase execution failed: %v", err), err)
			directive = DirectiveFail
			return nil
		}
		if response.WaitRequest != nil {
			phaseErr = e.enterWaiting(ctx, record.Run, attempt, response.WaitRequest)
			directive = DirectiveWait
			return nil
		}

		reportOutput, err := prompting.DecodeReportOutput([]byte(response.Output))
		if err != nil {
			phaseErr = e.failRun(ctx, record.Run, fmt.Sprintf("Report output could not be decoded: %v", err), err)
			directive = DirectiveFail
			return nil
		}
		if reportOutput.NeedsUserInput {
			phaseErr = e.enterWaiting(ctx, record.Run, attempt, &assistant.WaitRequest{
				Kind:        assistant.WaitKind(reportOutput.WaitKind),
				Title:       reportOutput.WaitTitle,
				Prompt:      reportOutput.WaitPrompt,
				RiskSummary: reportOutput.WaitRiskSummary,
			})
			directive = DirectiveWait
			return nil
		}
		if reportOutput.DeliveryStatus != "sent" {
			phaseErr = e.failRun(ctx, record.Run, firstNonEmpty(reportOutput.Summary, "Report delivery did not complete successfully."), ErrInvalidRunState)
			directive = DirectiveFail
			return nil
		}

		latestRecord, err := e.repo.GetRunRecord(ctx, record.Run.ID)
		if err != nil {
			return err
		}

		if !hasReportArtifact(latestRecord.Artifacts, attempt.ID, reportOutput.ReportPayload) {
			now := e.now().UTC()
			if err := e.repo.AddArtifact(ctx, assistant.Artifact{
				ID:        assistant.NewID("artifact", now),
				RunID:     record.Run.ID,
				AttemptID: attempt.ID,
				Kind:      assistant.ArtifactKindReport,
				Title:     "Final report payload",
				MIMEType:  "application/json",
				Content:   reportOutput.ReportPayload,
				CreatedAt: now,
			}); err != nil {
				return err
			}
		}

		latestRecord.Run.AttemptCount = len(latestRecord.Attempts)
		latestRecord.Run.UpdatedAt = e.now().UTC()
		phaseErr = e.completeRun(ctx, latestRecord.Run, firstNonEmpty(reportOutput.Summary, "Final report delivered."))
		directive = DirectiveComplete
		return nil
	})
	if err != nil {
		return DirectiveFail, e.failRun(ctx, record.Run, fmt.Sprintf("Report delivery setup failed: %v", err), err)
	}
	return directive, phaseErr
}

func (e *RunEngine) executeAttempt(ctx context.Context, run assistant.Run, existingAttempts []assistant.Attempt, role assistant.AttemptRole, prompt prompting.Bundle, critique string, resumeInput map[string]string, workingDir string) (assistant.Attempt, PhaseResponse, error) {
	startedAt := e.now().UTC()
	attempt := assistant.Attempt{
		ID:           assistant.NewID("attempt", startedAt),
		RunID:        run.ID,
		Sequence:     len(existingAttempts) + 1,
		Role:         role,
		InputSummary: summarizePrompt(prompt),
		Critique:     critique,
		StartedAt:    startedAt,
	}

	response, err := e.runtime.Execute(ctx, role, PhaseRequest{
		Run:         run,
		Attempt:     attempt,
		Critique:    critique,
		ResumeInput: resumeInput,
		Prompt:      prompt,
		WorkingDir:  workingDir,
		LiveEmit: func(event assistant.RunEvent) {
			if event.RunID == "" {
				event.RunID = run.ID
			}
			if event.ID == "" {
				event.ID = assistant.NewID("event", e.now().UTC())
			}
			if event.CreatedAt.IsZero() {
				event.CreatedAt = e.now().UTC()
			}
			_ = e.observer.Publish(ctx, event)
		},
	})

	finishedAt := e.now().UTC()
	attempt.FinishedAt = &finishedAt
	if err != nil {
		attempt.OutputSummary = err.Error()
		if saveErr := e.repo.AddAttempt(ctx, attempt); saveErr != nil {
			return assistant.Attempt{}, PhaseResponse{}, saveErr
		}
		return attempt, PhaseResponse{}, err
	}

	attempt.OutputSummary = summarizeAttemptResponse(response)
	if err := e.repo.AddAttempt(ctx, attempt); err != nil {
		return assistant.Attempt{}, PhaseResponse{}, err
	}
	if err := e.persistPhaseResponse(ctx, run.ID, run.Project.Slug, attempt.ID, response, finishedAt); err != nil {
		return assistant.Attempt{}, PhaseResponse{}, err
	}
	return attempt, response, nil
}

func (e *RunEngine) persistPhaseResponse(ctx context.Context, runID, projectSlug, attemptID string, response PhaseResponse, now time.Time) error {
	for _, artifact := range response.Artifacts {
		artifact.ID = firstNonEmpty(artifact.ID, assistant.NewID("artifact", now))
		artifact.RunID = runID
		artifact.AttemptID = attemptID
		artifact.CreatedAt = normalizeTime(artifact.CreatedAt, now)
		if err := e.repo.AddArtifact(ctx, artifact); err != nil {
			return err
		}
	}
	for _, evidence := range response.Evidence {
		evidence.ID = firstNonEmpty(evidence.ID, assistant.NewID("evidence", now))
		evidence.RunID = runID
		evidence.AttemptID = attemptID
		evidence.CreatedAt = normalizeTime(evidence.CreatedAt, now)
		if err := e.repo.AddEvidence(ctx, evidence); err != nil {
			return err
		}
	}
	for _, toolCall := range response.ToolCalls {
		toolCall.ID = firstNonEmpty(toolCall.ID, assistant.NewID("tool", now))
		toolCall.RunID = runID
		toolCall.AttemptID = attemptID
		toolCall.StartedAt = normalizeTime(toolCall.StartedAt, now)
		toolCall.FinishedAt = normalizeTime(toolCall.FinishedAt, now)
		if err := e.repo.AddToolCall(ctx, toolCall); err != nil {
			return err
		}
	}
	for _, webStep := range response.WebSteps {
		webStep.ID = firstNonEmpty(webStep.ID, assistant.NewID("webstep", now))
		webStep.RunID = runID
		webStep.AttemptID = attemptID
		webStep.OccurredAt = normalizeTime(webStep.OccurredAt, now)
		if err := e.repo.AddWebStep(ctx, webStep); err != nil {
			return err
		}

		actionRecord, ok := safety.BrowserActionRecordFromWebStep(projectSlug, webStep)
		if !ok {
			continue
		}
		actionRecord.ID = assistant.NewID("browser_action", webStep.OccurredAt)
		actionRecord.RunID = runID
		actionRecord.AttemptID = attemptID
		actionRecord.OccurredAt = webStep.OccurredAt
		if err := e.repo.AddBrowserAction(ctx, actionRecord); err != nil {
			return err
		}
	}
	return nil
}

func (e *RunEngine) enterWaiting(ctx context.Context, run assistant.Run, attempt assistant.Attempt, waitRequest *assistant.WaitRequest) error {
	now := e.now().UTC()
	wait := *waitRequest
	wait.ID = firstNonEmpty(wait.ID, assistant.NewID("wait", now))
	wait.RunID = run.ID
	wait.CreatedAt = normalizeTime(wait.CreatedAt, now)
	if err := e.repo.AddWaitRequest(ctx, wait); err != nil {
		return err
	}

	run.Status = assistant.RunStatusWaiting
	run.Phase = assistant.RunPhaseWaiting
	run.WaitingFor = &wait
	run.AttemptCount = attempt.Sequence
	run.UpdatedAt = now
	if err := e.repo.SaveRun(ctx, run); err != nil {
		return err
	}
	return e.publishEvent(ctx, run.ID, assistant.EventTypeWaiting, assistant.RunPhaseWaiting, wait.Title)
}

func (e *RunEngine) completeRun(ctx context.Context, run assistant.Run, summary string) error {
	now := e.now().UTC()
	run.Status = assistant.RunStatusCompleted
	run.Phase = assistant.RunPhaseCompleted
	run.WaitingFor = nil
	run.UpdatedAt = now
	run.CompletedAt = &now
	if err := e.repo.SaveRun(ctx, run); err != nil {
		return err
	}
	return e.publishEvent(ctx, run.ID, assistant.EventTypePhaseChanged, assistant.RunPhaseCompleted, firstNonEmpty(summary, "Run completed."))
}

func (e *RunEngine) exhaustRun(ctx context.Context, run assistant.Run, summary string) error {
	now := e.now().UTC()
	run.Status = assistant.RunStatusExhausted
	run.Phase = assistant.RunPhaseFailed
	run.WaitingFor = nil
	run.UpdatedAt = now
	if err := e.repo.SaveRun(ctx, run); err != nil {
		return err
	}
	return e.publishEvent(ctx, run.ID, assistant.EventTypePhaseChanged, assistant.RunPhaseFailed, firstNonEmpty(summary, "Run exhausted the available generation attempts."))
}

func (e *RunEngine) failRun(ctx context.Context, run assistant.Run, summary string, cause error) error {
	now := e.now().UTC()
	run.Status = assistant.RunStatusFailed
	run.Phase = assistant.RunPhaseFailed
	run.WaitingFor = nil
	run.UpdatedAt = now
	if err := e.repo.SaveRun(ctx, run); err != nil {
		return err
	}
	if eventErr := e.publishEvent(ctx, run.ID, assistant.EventTypePhaseChanged, assistant.RunPhaseFailed, summary); eventErr != nil {
		return eventErr
	}
	return cause
}

func (e *RunEngine) loadWikiContext(project assistant.ProjectContext) assistant.WikiContext {
	if e.wiki == nil || strings.TrimSpace(project.Slug) == "" {
		return assistant.WikiContext{}
	}
	context, err := e.wiki.LoadContext(project)
	if err != nil {
		return assistant.WikiContext{}
	}
	return context
}

func (e *RunEngine) publishEvent(ctx context.Context, runID string, eventType assistant.EventType, phase assistant.RunPhase, summary string) error {
	return e.publishEventWithData(ctx, runID, eventType, phase, summary, nil)
}

func (e *RunEngine) publishEventWithData(ctx context.Context, runID string, eventType assistant.EventType, phase assistant.RunPhase, summary string, data map[string]any) error {
	event := assistant.RunEvent{
		ID:        assistant.NewID("event", e.now()),
		RunID:     runID,
		Type:      eventType,
		Phase:     phase,
		Summary:   summary,
		Data:      data,
		CreatedAt: e.now().UTC(),
	}
	if err := e.repo.AddRunEvent(ctx, event); err != nil {
		return err
	}
	return e.observer.Publish(ctx, event)
}

type automationSafetyCheckResult struct {
	Findings     []string
	HardFindings []string
	NextActions  []string
	HardBlock    bool
}

func (r automationSafetyCheckResult) HardBlockSummary() string {
	if len(r.HardFindings) == 0 {
		return ""
	}
	return "Automation safety hard-limit violation: " + strings.Join(r.HardFindings, "; ")
}

func evaluateAutomationSafetyCompliance(
	policy *assistant.AutomationSafetyPolicy,
	actions []assistant.BrowserActionRecord,
	evidence []assistant.Evidence,
	metrics *assistant.BrowserRecentActivityMetrics,
	evaluationPassed bool,
) automationSafetyCheckResult {
	result := automationSafetyCheckResult{}
	if policy == nil {
		return result
	}
	if policy.Profile != assistant.AutomationSafetyProfileBrowserMutating &&
		policy.Profile != assistant.AutomationSafetyProfileBrowserHighRiskEngagement {
		return result
	}

	mutatingCount, replyCount, mutatingTimes, actionTypes := countMutatingAndReplyActions(actions)
	if limit := policy.RateLimits.MaxAccountChangingActionsPerRun; limit > 0 && mutatingCount > limit {
		finding := fmt.Sprintf("account-changing actions in this run (%d) exceed max_account_changing_actions_per_run=%d", mutatingCount, limit)
		result.Findings = append(result.Findings, finding)
		result.HardFindings = append(result.HardFindings, finding)
		result.NextActions = append(result.NextActions, "Reduce mutating browser actions to stay within per-run limits.")
	}
	if limit := policy.RateLimits.MaxRepliesPer24h; limit > 0 {
		observedReplies := replyCount
		if metrics != nil && metrics.ReplyActionCount > observedReplies {
			observedReplies = metrics.ReplyActionCount
		}
		if observedReplies > limit {
			finding := fmt.Sprintf("rolling reply count in the last 24h (%d) exceeds max_replies_per_24h=%d", observedReplies, limit)
			result.Findings = append(result.Findings, finding)
			result.HardFindings = append(result.HardFindings, finding)
			result.NextActions = append(result.NextActions, "Pause reply generation and continue with read-only observation until reply volume cools down.")
		}
	}
	if minSpacing := policy.RateLimits.MinSpacingMinutes; minSpacing > 0 {
		if ok, observed := violatesMinimumSpacing(mutatingTimes, minSpacing); ok {
			finding := fmt.Sprintf("mutating action spacing (%s) is below min_spacing_minutes=%d", observed, minSpacing)
			result.Findings = append(result.Findings, finding)
			result.HardFindings = append(result.HardFindings, finding)
			result.NextActions = append(result.NextActions, "Increase spacing between mutating actions before retrying.")
		}
	}
	if policy.PatternRules.DisallowDefaultActionTrios && hasDefaultActionTrio(actionTypes) {
		finding := "default mutating action trio pattern was detected in a single run"
		result.Findings = append(result.Findings, finding)
		result.HardFindings = append(result.HardFindings, finding)
		result.NextActions = append(result.NextActions, "Use a less repetitive action pattern and avoid bundled engage/reply/submit loops.")
	}

	if policy.PatternRules.RequireSourceDiversity && metrics != nil && metrics.SourcePathConcentration >= 0.85 {
		finding := fmt.Sprintf("source_path_concentration=%.2f is too high for require_source_diversity", metrics.SourcePathConcentration)
		result.Findings = append(result.Findings, finding)
		result.NextActions = append(result.NextActions, "Use more diverse source paths before additional mutating actions.")
	}
	if policy.TextReuse.RejectHighSimilarity && metrics != nil && metrics.TextReuseRiskScore >= 0.60 {
		finding := fmt.Sprintf("text_reuse_risk_score=%.2f indicates repeated messaging", metrics.TextReuseRiskScore)
		result.Findings = append(result.Findings, finding)
		result.NextActions = append(result.NextActions, "Rewrite outbound text with stronger variation before retrying.")
	}

	if evaluationPassed && policy.ModePolicy.AllowNoActionSuccess && policy.ModePolicy.RequireNoActionEvidence && mutatingCount == 0 {
		missing := missingNoActionEvidence(policy.ModePolicy.NoActionEvidenceRequired, evidence)
		if len(missing) > 0 {
			finding := fmt.Sprintf("no-action success is missing required evidence: %s", strings.Join(missing, "; "))
			result.Findings = append(result.Findings, finding)
			result.HardFindings = append(result.HardFindings, finding)
			result.NextActions = append(result.NextActions, "Record observed context, skipped action, safety reason, and safer next step before passing no-action outcomes.")
		}
	}

	if isHighRiskEngineBlockingPolicy(policy) && len(result.HardFindings) > 0 {
		result.HardBlock = true
	}
	result.Findings = uniqueTrimmedStrings(result.Findings)
	result.HardFindings = uniqueTrimmedStrings(result.HardFindings)
	result.NextActions = uniqueTrimmedStrings(result.NextActions)
	return result
}

func mergeAutomationSafetyFindingsIntoEvaluation(evaluation assistant.Evaluation, check automationSafetyCheckResult) assistant.Evaluation {
	if len(check.Findings) == 0 {
		return evaluation
	}
	evaluation.Passed = false
	if check.HardBlock {
		if evaluation.Score > 20 {
			evaluation.Score = 20
		}
	} else if evaluation.Score > 55 {
		evaluation.Score = 55
	}

	missing := append([]string{}, evaluation.MissingRequirements...)
	for _, finding := range check.Findings {
		missing = append(missing, "Automation safety: "+finding)
	}
	evaluation.MissingRequirements = uniqueTrimmedStrings(missing)

	summaryPrefix := "Automation safety findings: " + strings.Join(check.Findings, "; ")
	evaluation.Summary = firstNonEmpty(summaryPrefix, evaluation.Summary)

	nextActions := append([]string{}, check.NextActions...)
	if strings.TrimSpace(evaluation.NextActionForGenerator) != "" {
		nextActions = append(nextActions, evaluation.NextActionForGenerator)
	}
	evaluation.NextActionForGenerator = strings.Join(uniqueTrimmedStrings(nextActions), " ")
	return evaluation
}

func validateSchedulerAutomationSafety(
	policy *assistant.AutomationSafetyPolicy,
	metrics *assistant.BrowserRecentActivityMetrics,
	now time.Time,
	scheduledTimes []time.Time,
) string {
	if !isHighRiskEngineBlockingPolicy(policy) {
		return ""
	}
	if len(scheduledTimes) == 0 {
		return ""
	}

	if limit := policy.RateLimits.MaxRepliesPer24h; limit > 0 && metrics != nil && metrics.ReplyActionCount >= limit {
		return fmt.Sprintf("Automation safety hard-limit violation: rolling reply count (%d) reached max_replies_per_24h=%d, so no additional high-risk follow-up can be scheduled.", metrics.ReplyActionCount, limit)
	}

	sorted := append([]time.Time(nil), scheduledTimes...)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].Before(sorted[j])
	})

	if minSpacing := policy.RateLimits.MinSpacingMinutes; minSpacing > 0 {
		minSpacingDuration := time.Duration(minSpacing) * time.Minute
		for _, scheduledFor := range sorted {
			if scheduledFor.Sub(now) < minSpacingDuration {
				return fmt.Sprintf("Automation safety hard-limit violation: scheduled follow-up at %s is below min_spacing_minutes=%d from now.", scheduledFor.UTC().Format(time.RFC3339), minSpacing)
			}
		}
		for idx := 1; idx < len(sorted); idx++ {
			gap := sorted[idx].Sub(sorted[idx-1])
			if gap < minSpacingDuration {
				return fmt.Sprintf("Automation safety hard-limit violation: follow-up spacing (%s) is below min_spacing_minutes=%d.", gap.Round(time.Second), minSpacing)
			}
		}

		if policy.PatternRules.DisallowFixedShortFollowup && len(sorted) >= 2 {
			intervals := make([]time.Duration, 0, len(sorted)-1)
			for idx := 1; idx < len(sorted); idx++ {
				intervals = append(intervals, sorted[idx].Sub(sorted[idx-1]))
			}
			if len(intervals) > 0 {
				first := intervals[0]
				allEqual := true
				allShort := first <= 2*minSpacingDuration
				for _, interval := range intervals[1:] {
					if interval != first {
						allEqual = false
					}
					if interval > 2*minSpacingDuration {
						allShort = false
					}
				}
				if allEqual && allShort {
					return fmt.Sprintf("Automation safety hard-limit violation: fixed short follow-up cadence (%s) is disallowed for high-risk scheduling.", first.Round(time.Second))
				}
			}
		}
	}

	return ""
}

func isHighRiskEngineBlockingPolicy(policy *assistant.AutomationSafetyPolicy) bool {
	if policy == nil {
		return false
	}
	return policy.Profile == assistant.AutomationSafetyProfileBrowserHighRiskEngagement &&
		policy.Enforcement == assistant.AutomationSafetyEnforcementEngineBlocking
}

func countMutatingAndReplyActions(actions []assistant.BrowserActionRecord) (int, int, []time.Time, []assistant.BrowserActionType) {
	mutatingCount := 0
	replyCount := 0
	mutatingTimes := make([]time.Time, 0, len(actions))
	actionTypes := make([]assistant.BrowserActionType, 0, len(actions))
	for _, action := range actions {
		actionTypes = append(actionTypes, action.ActionType)
		if action.ActionType == assistant.BrowserActionTypeReply {
			replyCount++
		}
		if action.AccountStateChanged {
			mutatingCount++
			mutatingTimes = append(mutatingTimes, action.OccurredAt.UTC())
		}
	}
	sort.Slice(mutatingTimes, func(i, j int) bool {
		return mutatingTimes[i].Before(mutatingTimes[j])
	})
	return mutatingCount, replyCount, mutatingTimes, actionTypes
}

func violatesMinimumSpacing(mutatingTimes []time.Time, minSpacingMinutes int) (bool, string) {
	if len(mutatingTimes) < 2 || minSpacingMinutes <= 0 {
		return false, ""
	}
	threshold := time.Duration(minSpacingMinutes) * time.Minute
	for idx := 1; idx < len(mutatingTimes); idx++ {
		gap := mutatingTimes[idx].Sub(mutatingTimes[idx-1])
		if gap < threshold {
			return true, gap.Round(time.Second).String()
		}
	}
	return false, ""
}

func hasDefaultActionTrio(actionTypes []assistant.BrowserActionType) bool {
	hasEngage := false
	hasReply := false
	hasSubmitLike := false
	for _, actionType := range actionTypes {
		switch actionType {
		case assistant.BrowserActionTypeEngage:
			hasEngage = true
		case assistant.BrowserActionTypeReply:
			hasReply = true
		case assistant.BrowserActionTypeSubmit, assistant.BrowserActionTypeInput:
			hasSubmitLike = true
		}
	}
	return hasEngage && hasReply && hasSubmitLike
}

func missingNoActionEvidence(requirements []string, evidence []assistant.Evidence) []string {
	if len(requirements) == 0 {
		return nil
	}
	corpus := strings.ToLower(strings.TrimSpace(evidenceCorpus(evidence)))
	if corpus == "" {
		return append([]string{}, requirements...)
	}
	missing := make([]string, 0, len(requirements))
	for _, requirement := range requirements {
		req := strings.TrimSpace(requirement)
		if req == "" {
			continue
		}
		if !noActionRequirementSatisfied(strings.ToLower(req), corpus) {
			missing = append(missing, req)
		}
	}
	return missing
}

func evidenceCorpus(evidence []assistant.Evidence) string {
	parts := make([]string, 0, len(evidence)*2)
	for _, item := range evidence {
		if strings.TrimSpace(item.Summary) != "" {
			parts = append(parts, item.Summary)
		}
		if strings.TrimSpace(item.Detail) != "" {
			parts = append(parts, item.Detail)
		}
	}
	return strings.Join(parts, "\n")
}

func noActionRequirementSatisfied(requirement string, corpus string) bool {
	if strings.Contains(corpus, requirement) {
		return true
	}
	tokens := significantTokens(requirement)
	if len(tokens) == 0 {
		return false
	}
	hits := 0
	for _, token := range tokens {
		if strings.Contains(corpus, token) {
			hits++
		}
	}
	if len(tokens) <= 2 {
		return hits >= 1
	}
	return hits >= 2
}

func significantTokens(value string) []string {
	stopwords := map[string]struct{}{
		"what": {}, "that": {}, "made": {}, "action": {}, "was": {}, "for": {}, "and": {},
		"the": {}, "this": {}, "with": {}, "have": {}, "from": {}, "next": {}, "step": {},
		"should": {}, "happen": {}, "safe": {}, "safer": {},
	}
	parts := strings.Fields(strings.ToLower(value))
	tokens := make([]string, 0, len(parts))
	for _, part := range parts {
		trimmed := strings.Trim(part, ".,;:!?()[]{}\"'`")
		if len(trimmed) < 4 {
			continue
		}
		if _, skip := stopwords[trimmed]; skip {
			continue
		}
		tokens = append(tokens, trimmed)
	}
	return uniqueTrimmedStrings(tokens)
}

func uniqueTrimmedStrings(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	result := make([]string, 0, len(values))
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed == "" {
			continue
		}
		if _, ok := seen[trimmed]; ok {
			continue
		}
		seen[trimmed] = struct{}{}
		result = append(result, trimmed)
	}
	return result
}

func summarizePrompt(bundle prompting.Bundle) string {
	parts := []string{strings.TrimSpace(bundle.System), strings.TrimSpace(bundle.User)}
	text := strings.TrimSpace(strings.Join(parts, "\n"))
	if len(text) > 240 {
		return text[:240]
	}
	return text
}

func summarizeOutput(value string) string {
	trimmed := strings.TrimSpace(value)
	if len(trimmed) > 240 {
		return trimmed[:240]
	}
	return trimmed
}

func summarizeAttemptResponse(response PhaseResponse) string {
	summary := strings.TrimSpace(response.Summary)
	output := strings.TrimSpace(response.Output)
	if output != "" {
		shortOutput := summarizeOutput(output)
		if summary == "" || summary == shortOutput {
			return summarizeAttemptOutput(output)
		}
	}
	return firstNonEmpty(summary, summarizeAttemptOutput(output))
}

func summarizeAttemptOutput(value string) string {
	const limit = 1200

	trimmed := strings.TrimSpace(value)
	runes := []rune(trimmed)
	if len(runes) > limit {
		return strings.TrimSpace(string(runes[:limit-3])) + "..."
	}
	return trimmed
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func isTransientPhaseExecutionError(err error) bool {
	if err == nil {
		return false
	}
	message := strings.ToLower(strings.TrimSpace(err.Error()))
	return strings.Contains(message, "codex app server closed during phase execution") ||
		strings.Contains(message, "codex app server closed")
}

func (e *RunEngine) recentActivityMetrics(ctx context.Context, run assistant.Run) (*assistant.BrowserRecentActivityMetrics, error) {
	projectSlug := strings.TrimSpace(run.Project.Slug)
	if projectSlug == "" {
		return nil, nil
	}
	windowEnd := e.now().UTC()
	windowStart := windowEnd.Add(-24 * time.Hour)
	actions, err := e.repo.ListBrowserActionsByProject(ctx, projectSlug, windowStart, windowEnd)
	if err != nil {
		return nil, err
	}
	metrics := safety.ComputeRecentActivityMetrics(actions, windowStart, windowEnd)
	return &metrics, nil
}

func normalizeTime(value time.Time, fallback time.Time) time.Time {
	if value.IsZero() {
		return fallback.UTC()
	}
	return value.UTC()
}

func latestAttemptByRole(attempts []assistant.Attempt, role assistant.AttemptRole) *assistant.Attempt {
	for idx := len(attempts) - 1; idx >= 0; idx-- {
		if attempts[idx].Role == role {
			attempt := attempts[idx]
			return &attempt
		}
	}
	return nil
}

func countAttemptsByRole(attempts []assistant.Attempt, role assistant.AttemptRole) int {
	count := 0
	for _, attempt := range attempts {
		if attempt.Role == role {
			count++
		}
	}
	return count
}

func hasReportArtifact(artifacts []assistant.Artifact, attemptID string, content string) bool {
	for _, artifact := range artifacts {
		if artifact.AttemptID != attemptID {
			continue
		}
		if artifact.Kind == assistant.ArtifactKindReport && strings.TrimSpace(artifact.Content) == strings.TrimSpace(content) {
			return true
		}
	}
	return false
}

func generatorPhaseSummary(record *store.RunRecord) string {
	attempt := 1
	if record.Run.LatestEvaluation != nil {
		attempt = len(record.Attempts) + 1
	}
	if record.Run.LatestEvaluation != nil && strings.TrimSpace(record.Run.LatestEvaluation.NextActionForGenerator) != "" {
		return fmt.Sprintf("Generator retry %d is addressing evaluator critique.", attempt)
	}
	return fmt.Sprintf("Generator attempt %d is producing work artifacts.", attempt)
}

func contractPhaseSummary(run assistant.Run) string {
	if run.TaskSpec.AcceptanceContract != nil && strings.TrimSpace(run.TaskSpec.AcceptanceContract.RevisionNotes) != "" {
		return "Revising the acceptance contract to remove ambiguity before generation."
	}
	return "Negotiating the acceptance contract that will gate generation and evaluation."
}

func isTerminalStatus(status assistant.RunStatus) bool {
	switch status {
	case assistant.RunStatusCompleted, assistant.RunStatusFailed, assistant.RunStatusExhausted, assistant.RunStatusCancelled:
		return true
	default:
		return false
	}
}

type noopObserver struct{}

func (noopObserver) Publish(context.Context, assistant.RunEvent) error {
	return nil
}
