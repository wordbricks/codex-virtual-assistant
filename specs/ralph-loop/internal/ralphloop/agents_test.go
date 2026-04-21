package ralphloop

import (
	"context"
	"strings"
	"testing"
)

type fakeCodexClient struct {
	turns   []turnResult
	prompts []string
}

func (client *fakeCodexClient) Initialize(ctx context.Context) error {
	return nil
}

func (client *fakeCodexClient) StartThread(ctx context.Context, options startThreadOptions) (string, error) {
	return "thread_123", nil
}

func (client *fakeCodexClient) RunTurn(ctx context.Context, options runTurnOptions) (turnResult, error) {
	client.prompts = append(client.prompts, options.Prompt)
	if len(client.turns) == 0 {
		return turnResult{Status: "completed", AgentText: completeToken}, nil
	}
	result := client.turns[0]
	client.turns = client.turns[1:]
	return result, nil
}

func (client *fakeCodexClient) CompactThread(ctx context.Context, threadID string) error {
	return nil
}

func (client *fakeCodexClient) Close() error {
	return nil
}

func (client *fakeCodexClient) SetNotificationHandler(handler func(jsonRPCNotification)) {}

func TestRunCodingLoopRecoversAfterTurnIdleTimeout(t *testing.T) {
	client := &fakeCodexClient{
		turns: []turnResult{
			{Status: "failed", CodexErrorInfo: "TurnIdleTimeout"},
			{Status: "completed", AgentText: completeToken},
		},
	}

	result, err := runCodingLoop(context.Background(), codingLoopOptions{
		Client:        client,
		ThreadID:      "thread_123",
		WorktreePath:  t.TempDir(),
		UserPrompt:    "finish the work",
		PlanPath:      "docs/exec-plans/active/example.md",
		MaxIterations: 3,
	})
	if err != nil {
		t.Fatalf("runCodingLoop() error = %v", err)
	}
	if !result.Completed {
		t.Fatal("runCodingLoop() did not complete")
	}
	if result.Iterations != 2 {
		t.Fatalf("iterations = %d, want 2", result.Iterations)
	}
	if len(client.prompts) != 2 {
		t.Fatalf("prompt count = %d, want 2", len(client.prompts))
	}
	if !strings.Contains(client.prompts[1], "previous iteration failed") {
		t.Fatalf("second prompt = %q, want recovery prompt", client.prompts[1])
	}
}
