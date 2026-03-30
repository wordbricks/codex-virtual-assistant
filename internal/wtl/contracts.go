package wtl

import (
	"context"

	"github.com/siisee11/CodexVirtualAssistant/internal/assistant"
	"github.com/siisee11/CodexVirtualAssistant/internal/prompting"
)

type Directive string

const (
	DirectiveContinue Directive = "continue"
	DirectiveWait     Directive = "wait"
	DirectiveRetry    Directive = "retry"
	DirectiveComplete Directive = "complete"
	DirectiveFail     Directive = "fail"
)

type PhaseRequest struct {
	Run         assistant.Run
	Attempt     assistant.Attempt
	Critique    string
	ResumeInput map[string]string
	Prompt      prompting.Bundle
	WorkingDir  string
	LiveEmit    func(assistant.RunEvent)
}

type PhaseResponse struct {
	Summary     string
	Output      string
	Artifacts   []assistant.Artifact
	Evidence    []assistant.Evidence
	ToolCalls   []assistant.ToolCall
	WebSteps    []assistant.WebStep
	WaitRequest *assistant.WaitRequest
}

type Runtime interface {
	Execute(context.Context, assistant.AttemptRole, PhaseRequest) (PhaseResponse, error)
	Close() error
}

type Observer interface {
	Publish(context.Context, assistant.RunEvent) error
}

type Engine interface {
	Start(context.Context, assistant.Run) error
	Resume(context.Context, string, map[string]string) error
	Cancel(context.Context, string) error
}
