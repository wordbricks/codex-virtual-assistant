package gan

import (
	"time"

	"github.com/siisee11/CodexVirtualAssistant/internal/assistant"
)

type Config struct {
	MaxGenerationAttempts int
}

type Policy struct {
	config Config
}

type EvaluationDecision string

const (
	EvaluationDecisionRetry    EvaluationDecision = "retry"
	EvaluationDecisionComplete EvaluationDecision = "complete"
	EvaluationDecisionFail     EvaluationDecision = "fail"
)

func New(config Config) Policy {
	if config.MaxGenerationAttempts <= 0 {
		config.MaxGenerationAttempts = 3
	}
	return Policy{config: config}
}

func (p Policy) Config() Config {
	return p.config
}

func (p Policy) InitialRun(userRequest string, now time.Time) assistant.Run {
	return assistant.NewRun(userRequest, now, p.config.MaxGenerationAttempts)
}

func (p Policy) CountGenerationAttempts(attempts []assistant.Attempt) int {
	total := 0
	for _, attempt := range attempts {
		if attempt.Role == assistant.AttemptRoleGenerator {
			total++
		}
	}
	return total
}

func (p Policy) HasAcceptedContract(run assistant.Run) bool {
	return run.TaskSpec.HasAcceptedContract()
}

func (p Policy) CanGenerate(run assistant.Run) bool {
	return p.HasAcceptedContract(run)
}

func (p Policy) CanEvaluate(run assistant.Run, attempts []assistant.Attempt) bool {
	return p.HasAcceptedContract(run) && p.CountGenerationAttempts(attempts) > 0
}

func (p Policy) DecideEvaluation(run assistant.Run, attempts []assistant.Attempt, evaluation assistant.Evaluation) EvaluationDecision {
	if evaluation.Passed {
		return EvaluationDecisionComplete
	}
	if p.CountGenerationAttempts(attempts) >= run.MaxGenerationAttempts {
		return EvaluationDecisionFail
	}
	return EvaluationDecisionRetry
}
