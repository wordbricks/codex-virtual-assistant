package wtl

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/siisee11/CodexVirtualAssistant/internal/assistant"
)

type HeuristicPhaseExecutor struct {
	now func() time.Time
}

func NewHeuristicPhaseExecutor(now func() time.Time) *HeuristicPhaseExecutor {
	if now == nil {
		now = time.Now
	}
	return &HeuristicPhaseExecutor{now: now}
}

func (e *HeuristicPhaseExecutor) RunPhase(_ context.Context, request CodexPhaseRequest) (CodexPhaseResult, error) {
	switch request.Role {
	case assistant.AttemptRoleProjectSelector:
		return e.runProjectSelector(request)
	case assistant.AttemptRolePlanner:
		return e.runPlanner(request)
	case assistant.AttemptRoleContractor:
		return e.runContractor(request)
	case assistant.AttemptRoleGenerator:
		return e.runGenerator(request)
	case assistant.AttemptRoleEvaluator:
		return e.runEvaluator(request)
	default:
		return CodexPhaseResult{}, fmt.Errorf("heuristic executor: unsupported role %q", request.Role)
	}
}

func (e *HeuristicPhaseExecutor) runProjectSelector(request CodexPhaseRequest) (CodexPhaseResult, error) {
	output, err := json.Marshal(map[string]any{
		"project_slug":        "no_project",
		"project_name":        "No Project",
		"project_description": "Use this project for simple questions, one-off requests, and tasks that do not need long-lived project memory.",
		"summary":             "Selected the reserved no_project workspace.",
	})
	if err != nil {
		return CodexPhaseResult{}, err
	}
	return CodexPhaseResult{
		Summary:      "Selected the reserved no_project workspace.",
		Output:       string(output),
		Observations: []string{"Project selection completed using the local heuristic executor."},
	}, nil
}

func (e *HeuristicPhaseExecutor) Close() error {
	return nil
}

func (e *HeuristicPhaseExecutor) runPlanner(request CodexPhaseRequest) (CodexPhaseResult, error) {
	userRequest := request.Prompt
	if marker := "User request:\n"; strings.Contains(request.Prompt, marker) {
		userRequest = strings.SplitN(strings.SplitN(request.Prompt, marker, 2)[1], "\n\n", 2)[0]
	}

	spec, err := assistant.NormalizeTaskSpec(assistant.TaskSpecDraft{
		UserRequestRaw: strings.TrimSpace(userRequest),
	}, userRequest, 3)
	if err != nil {
		return CodexPhaseResult{}, err
	}

	output, err := json.Marshal(map[string]any{
		"goal":                    spec.Goal,
		"deliverables":            spec.Deliverables,
		"constraints":             spec.Constraints,
		"tools_allowed":           spec.ToolsAllowed,
		"tools_required":          spec.ToolsRequired,
		"done_definition":         spec.DoneDefinition,
		"evidence_required":       spec.EvidenceRequired,
		"risk_flags":              spec.RiskFlags,
		"max_generation_attempts": spec.MaxGenerationAttempts,
	})
	if err != nil {
		return CodexPhaseResult{}, err
	}

	return CodexPhaseResult{
		Summary:      "Planner normalized the request into a structured TaskSpec.",
		Output:       string(output),
		Observations: []string{"Task planning completed using the current request and configured defaults."},
	}, nil
}

func (e *HeuristicPhaseExecutor) runGenerator(request CodexPhaseRequest) (CodexPhaseResult, error) {
	if shouldRequestApproval(request) {
		return CodexPhaseResult{
			Summary: "Generator paused for approval or login information before touching the external service.",
			WaitRequest: &assistant.WaitRequest{
				Kind:        waitKindForRequest(request),
				Title:       "Approval or input needed",
				Prompt:      "Please confirm the external action or provide the missing login/context so the assistant can continue.",
				RiskSummary: "The task appears to involve login, approval, or an external update.",
			},
		}, nil
	}

	artifactContent := buildArtifactContent(request)
	now := e.now().UTC()
	return CodexPhaseResult{
		Summary: "Generator collected evidence and drafted the requested result package.",
		Output:  "Generated a first-pass result package from the current task specification.",
		Artifacts: []assistant.Artifact{
			{
				Kind:      assistant.ArtifactKindReport,
				Title:     "Assistant draft result",
				MIMEType:  "text/markdown",
				Content:   artifactContent,
				CreatedAt: now,
			},
		},
		Observations: []string{
			"Prepared a synthesized result draft for the current task goal.",
			"Captured browser-oriented execution notes for evaluator review.",
		},
		ToolRuns: []CodexToolRun{
			{
				Name:          "agent-browser snapshot",
				InputSummary:  "Inspect the relevant browser surface for the current task.",
				OutputSummary: "Captured a browser snapshot and extracted the visible summary text.",
				StartedAt:     now,
				FinishedAt:    now.Add(2 * time.Second),
			},
		},
		BrowserSteps: []AgentBrowserStep{
			{
				Title:   "Prepared a browser research checkpoint",
				URL:     "about:blank",
				Summary: "The assistant created an execution checkpoint that can be evaluated and extended in later runs.",
				Action: AgentBrowserAction{
					Name:   "snapshot",
					Target: "current working surface",
				},
				ObservedText: []string{
					"Goal: " + request.TaskSpec.Goal,
					"Deliverables: " + strings.Join(request.TaskSpec.Deliverables, ", "),
				},
				OccurredAt: now.Add(3 * time.Second),
			},
		},
	}, nil
}

func (e *HeuristicPhaseExecutor) runContractor(request CodexPhaseRequest) (CodexPhaseResult, error) {
	output, err := json.Marshal(map[string]any{
		"decision":            "agreed",
		"summary":             "The acceptance contract is concrete enough to start generation.",
		"deliverables":        request.TaskSpec.Deliverables,
		"acceptance_criteria": request.TaskSpec.DoneDefinition,
		"evidence_required":   request.TaskSpec.EvidenceRequired,
		"constraints":         request.TaskSpec.Constraints,
		"out_of_scope":        []string{},
		"revision_notes":      "",
	})
	if err != nil {
		return CodexPhaseResult{}, err
	}
	return CodexPhaseResult{
		Summary:      "Contract phase agreed the acceptance criteria for this run.",
		Output:       string(output),
		Observations: []string{"Acceptance contract completed using the local heuristic executor."},
	}, nil
}

func (e *HeuristicPhaseExecutor) runEvaluator(request CodexPhaseRequest) (CodexPhaseResult, error) {
	output, err := json.Marshal(map[string]any{
		"passed":                    true,
		"score":                     90,
		"summary":                   "The generated result includes a draft deliverable and supporting browser evidence.",
		"missing_requirements":      []string{},
		"incorrect_claims":          []string{},
		"evidence_checked":          []string{"Stored artifacts", "Stored evidence", "Stored browser steps"},
		"next_action_for_generator": "",
	})
	if err != nil {
		return CodexPhaseResult{}, err
	}
	return CodexPhaseResult{
		Summary: "Evaluator accepted the current result package.",
		Output:  string(output),
	}, nil
}

func shouldRequestApproval(request CodexPhaseRequest) bool {
	if len(request.ResumeInput) > 0 {
		return false
	}
	for _, flag := range request.TaskSpec.RiskFlags {
		if flag == "authentication-required" || flag == "external-side-effect" {
			return true
		}
	}
	return false
}

func waitKindForRequest(request CodexPhaseRequest) assistant.WaitKind {
	for _, flag := range request.TaskSpec.RiskFlags {
		if flag == "authentication-required" {
			return assistant.WaitKindAuthentication
		}
	}
	return assistant.WaitKindApproval
}

func buildArtifactContent(request CodexPhaseRequest) string {
	lines := []string{
		"# Assistant Draft Result",
		"",
		"Goal: " + request.TaskSpec.Goal,
		"",
		"Deliverables:",
	}
	for _, deliverable := range request.TaskSpec.Deliverables {
		lines = append(lines, "- "+deliverable)
	}
	if strings.TrimSpace(request.Critique) != "" {
		lines = append(lines, "", "Evaluator critique addressed:", "- "+request.Critique)
	}
	if len(request.ResumeInput) > 0 {
		lines = append(lines, "", "Resume input:")
		keys := make([]string, 0, len(request.ResumeInput))
		for key := range request.ResumeInput {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			lines = append(lines, fmt.Sprintf("- %s: %s", key, request.ResumeInput[key]))
		}
	}
	lines = append(lines, "", "This is a local heuristic execution result wired for the product API surface.")
	return strings.Join(lines, "\n")
}
