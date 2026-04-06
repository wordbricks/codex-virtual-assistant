package wtl

import (
	"context"
	"testing"
	"time"

	"github.com/siisee11/CodexVirtualAssistant/internal/assistant"
	"github.com/siisee11/CodexVirtualAssistant/internal/prompting"
)

func TestCodexRuntimeMapsToolAndBrowserEvidence(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, time.March, 27, 17, 0, 0, 0, time.UTC)
	executor := &fakeCodexExecutor{
		result: CodexPhaseResult{
			Summary:      "Collected competitor pricing evidence.",
			Output:       "Completed the first research pass.",
			Observations: []string{"Observed a starter price card for Vendor A."},
			ToolRuns: []CodexToolRun{
				{
					Name:          "agent-browser open",
					InputSummary:  "Open the vendor pricing page",
					OutputSummary: "Loaded the pricing page successfully",
					StartedAt:     now,
					FinishedAt:    now.Add(2 * time.Second),
				},
			},
			BrowserSteps: []AgentBrowserStep{
				{
					Title:   "Viewed pricing page",
					URL:     "https://example.com/pricing",
					Summary: "The pricing page shows a starter plan card.",
					Action: AgentBrowserAction{
						Name:   "open",
						Target: "pricing page",
					},
					ObservedText:   []string{"Starter plan", "$49 per month"},
					ScreenshotPath: "data/artifacts/pricing.png",
					ScreenshotNote: "Pricing page screenshot",
					OccurredAt:     now.Add(3 * time.Second),
				},
			},
		},
	}

	runtime := NewCodexRuntime(executor, "gpt-5.4", func() time.Time { return now })
	response, err := runtime.Execute(context.Background(), assistant.AttemptRoleGenerator, PhaseRequest{
		Run: assistant.Run{
			ID: "run_123",
			TaskSpec: assistant.TaskSpec{
				ToolsAllowed: []string{"agent-browser"},
			},
		},
		Attempt: assistant.Attempt{ID: "attempt_123"},
		Prompt: prompting.Bundle{
			System: "system prompt",
			User:   "user prompt",
		},
	})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	if len(response.ToolCalls) != 1 {
		t.Fatalf("len(ToolCalls) = %d, want 1", len(response.ToolCalls))
	}
	if len(response.WebSteps) != 1 {
		t.Fatalf("len(WebSteps) = %d, want 1", len(response.WebSteps))
	}
	if len(response.Evidence) != 3 {
		t.Fatalf("len(Evidence) = %d, want 3", len(response.Evidence))
	}
	if len(response.Artifacts) != 1 || response.Artifacts[0].Kind != assistant.ArtifactKindScreenshot {
		t.Fatalf("Artifacts = %#v, want derived screenshot artifact", response.Artifacts)
	}
	if executor.request.Model != "gpt-5.4" {
		t.Fatalf("Model = %q, want gpt-5.4", executor.request.Model)
	}
	if executor.request.SessionName == "" {
		t.Fatal("SessionName is empty")
	}
}

func TestCodexRuntimeCarriesResumeInputIntoPrompt(t *testing.T) {
	t.Parallel()

	executor := &fakeCodexExecutor{
		result: CodexPhaseResult{Output: "{}", Summary: "ok"},
	}
	runtime := NewCodexRuntime(executor, "", time.Now)

	_, err := runtime.Execute(context.Background(), assistant.AttemptRolePlanner, PhaseRequest{
		Run: assistant.Run{
			ID: "run_456",
			TaskSpec: assistant.TaskSpec{
				ToolsAllowed: []string{"agent-browser"},
			},
		},
		Attempt: assistant.Attempt{ID: "attempt_456"},
		Prompt: prompting.Bundle{
			System: "planner system",
			User:   "planner user",
		},
		ResumeInput: map[string]string{"scope": "direct competitors only"},
	})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	if executor.request.ResumeInput["scope"] != "direct competitors only" {
		t.Fatalf("ResumeInput = %#v", executor.request.ResumeInput)
	}
	if executor.request.Tools[0] != "agent-browser" {
		t.Fatalf("Tools = %#v", executor.request.Tools)
	}
}

func TestCodexRuntimeUsesReadOnlyContextToolsForGateAndAnswer(t *testing.T) {
	t.Parallel()

	executor := &fakeCodexExecutor{
		result: CodexPhaseResult{Output: "{}", Summary: "ok"},
	}
	runtime := NewCodexRuntime(executor, "", time.Now)

	_, err := runtime.Execute(context.Background(), assistant.AttemptRoleGate, PhaseRequest{
		Run:     assistant.Run{ID: "run_gate"},
		Attempt: assistant.Attempt{ID: "attempt_gate"},
		Prompt:  prompting.Bundle{System: "gate system", User: "gate user"},
	})
	if err != nil {
		t.Fatalf("Execute(gate) error = %v", err)
	}
	if len(executor.request.Tools) == 0 || executor.request.Tools[0] != "stored-parent-run" {
		t.Fatalf("gate tools = %#v, want stored parent context tools", executor.request.Tools)
	}

	_, err = runtime.Execute(context.Background(), assistant.AttemptRoleAnswer, PhaseRequest{
		Run:     assistant.Run{ID: "run_answer"},
		Attempt: assistant.Attempt{ID: "attempt_answer"},
		Prompt:  prompting.Bundle{System: "answer system", User: "answer user"},
	})
	if err != nil {
		t.Fatalf("Execute(answer) error = %v", err)
	}
	if len(executor.request.Tools) == 0 || executor.request.Tools[0] != "stored-parent-run" {
		t.Fatalf("answer tools = %#v, want stored parent context tools", executor.request.Tools)
	}
}

func TestCodexRuntimeUsesStoredContextToolsForReporter(t *testing.T) {
	t.Parallel()

	executor := &fakeCodexExecutor{
		result: CodexPhaseResult{Output: "{}", Summary: "ok"},
	}
	runtime := NewCodexRuntime(executor, "", time.Now)

	_, err := runtime.Execute(context.Background(), assistant.AttemptRoleReporter, PhaseRequest{
		Run:     assistant.Run{ID: "run_report"},
		Attempt: assistant.Attempt{ID: "attempt_report"},
		Prompt:  prompting.Bundle{System: "report system", User: "report user"},
	})
	if err != nil {
		t.Fatalf("Execute(reporter) error = %v", err)
	}
	if len(executor.request.Tools) == 0 || executor.request.Tools[0] != "stored-artifacts" {
		t.Fatalf("reporter tools = %#v, want stored context tools", executor.request.Tools)
	}
}

func TestCodexRuntimeUsesStoredContextToolsForScheduler(t *testing.T) {
	t.Parallel()

	executor := &fakeCodexExecutor{
		result: CodexPhaseResult{Output: "{}", Summary: "ok"},
	}
	runtime := NewCodexRuntime(executor, "", time.Now)

	_, err := runtime.Execute(context.Background(), assistant.AttemptRoleScheduler, PhaseRequest{
		Run:     assistant.Run{ID: "run_schedule"},
		Attempt: assistant.Attempt{ID: "attempt_schedule"},
		Prompt:  prompting.Bundle{System: "schedule system", User: "schedule user"},
	})
	if err != nil {
		t.Fatalf("Execute(scheduler) error = %v", err)
	}
	if len(executor.request.Tools) == 0 || executor.request.Tools[0] != "stored-plan" {
		t.Fatalf("scheduler tools = %#v, want stored schedule context tools", executor.request.Tools)
	}
}

type fakeCodexExecutor struct {
	request CodexPhaseRequest
	result  CodexPhaseResult
	err     error
}

func (f *fakeCodexExecutor) RunPhase(_ context.Context, request CodexPhaseRequest) (CodexPhaseResult, error) {
	f.request = request
	return f.result, f.err
}

func (f *fakeCodexExecutor) Close() error {
	return nil
}
