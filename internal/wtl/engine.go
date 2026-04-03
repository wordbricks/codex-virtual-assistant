package wtl

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/siisee11/CodexVirtualAssistant/internal/agentmessage"
	"github.com/siisee11/CodexVirtualAssistant/internal/assistant"
	"github.com/siisee11/CodexVirtualAssistant/internal/policy/gan"
	"github.com/siisee11/CodexVirtualAssistant/internal/prompting"
	"github.com/siisee11/CodexVirtualAssistant/internal/store"
)

var ErrInvalidRunState = errors.New("wtl: invalid run state")

type Repository interface {
	SaveRun(context.Context, assistant.Run) error
	AddRunEvent(context.Context, assistant.RunEvent) error
	AddAttempt(context.Context, assistant.Attempt) error
	AddArtifact(context.Context, assistant.Artifact) error
	AddEvidence(context.Context, assistant.Evidence) error
	AddEvaluation(context.Context, assistant.Evaluation) error
	AddToolCall(context.Context, assistant.ToolCall) error
	AddWebStep(context.Context, assistant.WebStep) error
	AddWaitRequest(context.Context, assistant.WaitRequest) error
	GetRun(context.Context, string) (assistant.Run, error)
	GetRunRecord(context.Context, string) (store.RunRecord, error)
}

type ProjectManager interface {
	SelectionRoot() string
	EnsureProject(assistant.ProjectContext) (assistant.ProjectContext, error)
}

type RunEngine struct {
	repo      Repository
	runtime   Runtime
	observer  Observer
	policy    gan.Policy
	projects  ProjectManager
	messenger agentmessage.Service
	now       func() time.Time
}

func NewRunEngine(repo Repository, runtime Runtime, observer Observer, policy gan.Policy, projects ProjectManager, messenger agentmessage.Service, now func() time.Time) *RunEngine {
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
		messenger: messenger,
		now:       now,
	}
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
			role = assistant.AttemptRoleReporter
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
				role = assistant.AttemptRoleReporter
				resumeInput = nil
			case DirectiveRetry:
				role = assistant.AttemptRoleGenerator
				resumeInput = nil
			default:
				return fmt.Errorf("wtl: unsupported directive %q", directive)
			}
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
		return e.publishEvent(ctx, record.Run.ID, assistant.EventTypePhaseChanged, assistant.RunPhaseSelectingProject, firstNonEmpty(summary, "Gate selected workflow mode. Starting project selection."))
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
	return e.publishEvent(ctx, record.Run.ID, assistant.EventTypePhaseChanged, assistant.RunPhaseReporting, firstNonEmpty(summary, "Answer completed. Starting final report delivery."))
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

	prompt := prompting.BuildEvaluatorPrompt(prompting.EvaluatorInput{
		Run:       record.Run,
		Attempt:   *generatorAttempt,
		Artifacts: record.Artifacts,
		Evidence:  record.Evidence,
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
	if err := e.repo.AddEvaluation(ctx, evaluation); err != nil {
		return "", err
	}
	record.Run.LatestEvaluation = &evaluation
	if err := e.publishEvent(ctx, record.Run.ID, assistant.EventTypeEvaluation, assistant.RunPhaseEvaluating, evaluation.Summary); err != nil {
		return "", err
	}

	updatedRecord, err := e.repo.GetRunRecord(ctx, record.Run.ID)
	if err != nil {
		return "", err
	}
	switch e.policy.DecideEvaluation(updatedRecord.Run, updatedRecord.Attempts, evaluation) {
	case gan.EvaluationDecisionComplete:
		if err := e.publishEvent(ctx, updatedRecord.Run.ID, assistant.EventTypePhaseChanged, assistant.RunPhaseReporting, firstNonEmpty(evaluation.Summary, "Evaluation passed. Starting final report delivery.")); err != nil {
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
		return "", err
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

	attempt.OutputSummary = firstNonEmpty(strings.TrimSpace(response.Summary), summarizeOutput(response.Output))
	if err := e.repo.AddAttempt(ctx, attempt); err != nil {
		return assistant.Attempt{}, PhaseResponse{}, err
	}
	if err := e.persistPhaseResponse(ctx, run.ID, attempt.ID, response, finishedAt); err != nil {
		return assistant.Attempt{}, PhaseResponse{}, err
	}
	return attempt, response, nil
}

func (e *RunEngine) persistPhaseResponse(ctx context.Context, runID, attemptID string, response PhaseResponse, now time.Time) error {
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

func (e *RunEngine) publishEvent(ctx context.Context, runID string, eventType assistant.EventType, phase assistant.RunPhase, summary string) error {
	event := assistant.RunEvent{
		ID:        assistant.NewID("event", e.now()),
		RunID:     runID,
		Type:      eventType,
		Phase:     phase,
		Summary:   summary,
		CreatedAt: e.now().UTC(),
	}
	if err := e.repo.AddRunEvent(ctx, event); err != nil {
		return err
	}
	return e.observer.Publish(ctx, event)
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
