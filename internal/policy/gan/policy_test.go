package gan

import (
	"testing"
	"time"

	"github.com/siisee11/CodexVirtualAssistant/internal/assistant"
)

func TestPolicyDecideEvaluation(t *testing.T) {
	t.Parallel()

	policy := New(Config{MaxGenerationAttempts: 2})
	run := assistant.NewRun("Compare competitor pricing.", time.Date(2026, time.March, 27, 10, 0, 0, 0, time.UTC), 2)

	if directive := policy.DecideEvaluation(run, nil, assistant.Evaluation{Passed: true}); directive != EvaluationDecisionComplete {
		t.Fatalf("directive = %q, want %q", directive, EvaluationDecisionComplete)
	}

	attempts := []assistant.Attempt{
		{Role: assistant.AttemptRolePlanner},
		{Role: assistant.AttemptRoleGenerator},
	}
	if directive := policy.DecideEvaluation(run, attempts, assistant.Evaluation{Passed: false}); directive != EvaluationDecisionRetry {
		t.Fatalf("directive = %q, want %q", directive, EvaluationDecisionRetry)
	}

	attempts = append(attempts, assistant.Attempt{Role: assistant.AttemptRoleGenerator})
	if directive := policy.DecideEvaluation(run, attempts, assistant.Evaluation{Passed: false}); directive != EvaluationDecisionFail {
		t.Fatalf("directive = %q, want %q", directive, EvaluationDecisionFail)
	}
}

func TestPolicyRequiresAcceptedContractBeforeGeneration(t *testing.T) {
	t.Parallel()

	policy := New(Config{MaxGenerationAttempts: 2})
	run := assistant.NewRun("Compare competitor pricing.", time.Date(2026, time.March, 27, 10, 0, 0, 0, time.UTC), 2)

	if policy.CanGenerate(run) {
		t.Fatal("CanGenerate() = true, want false before contract agreement")
	}

	run.TaskSpec.AcceptanceContract = &assistant.AcceptanceContract{
		Status:             assistant.ContractStatusAgreed,
		Summary:            "Contract agreed.",
		Deliverables:       []string{"Pricing table"},
		AcceptanceCriteria: []string{"Include source URLs"},
		EvidenceRequired:   []string{"Stored sources"},
	}
	if !policy.CanGenerate(run) {
		t.Fatal("CanGenerate() = false, want true after contract agreement")
	}
	if policy.CanEvaluate(run, nil) {
		t.Fatal("CanEvaluate() = true, want false without a generator attempt")
	}
	if !policy.CanEvaluate(run, []assistant.Attempt{{Role: assistant.AttemptRoleGenerator}}) {
		t.Fatal("CanEvaluate() = false, want true with contract and generator attempt")
	}
}
