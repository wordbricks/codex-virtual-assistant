package wtl

import (
	"testing"
	"time"

	"github.com/siisee11/CodexVirtualAssistant/internal/assistant"
)

func TestCollectReasoningSkipsEmptyStructuredContent(t *testing.T) {
	t.Parallel()

	session := newAppServerTurnSession(AppServerPhaseExecutorConfig{}, func() time.Time {
		return time.Date(2026, time.March, 29, 3, 0, 0, 0, time.UTC)
	})
	session.runID = "run_123"
	var emitted []assistant.RunEvent
	session.liveEmit = func(event assistant.RunEvent) {
		emitted = append(emitted, event)
	}

	session.collectReasoning(map[string]any{
		"id":      "reasoning_1",
		"type":    "reasoning",
		"content": []any{},
	})

	if len(emitted) != 0 {
		t.Fatalf("collectReasoning() emitted %#v, want no events for empty content", emitted)
	}
}

func TestCollectReasoningExtractsTextContent(t *testing.T) {
	t.Parallel()

	session := newAppServerTurnSession(AppServerPhaseExecutorConfig{}, func() time.Time {
		return time.Date(2026, time.March, 29, 3, 1, 0, 0, time.UTC)
	})
	session.runID = "run_123"
	session.attemptID = "attempt_123"
	session.attemptRole = assistant.AttemptRolePlanner

	var emitted []assistant.RunEvent
	session.liveEmit = func(event assistant.RunEvent) {
		emitted = append(emitted, event)
	}

	session.collectReasoning(map[string]any{
		"id":   "reasoning_2",
		"type": "reasoning",
		"content": []any{
			map[string]any{"text": "Plan the task carefully."},
		},
	})

	if len(emitted) != 1 {
		t.Fatalf("len(emitted) = %d, want 1", len(emitted))
	}
	if emitted[0].Type != assistant.EventTypeReasoning {
		t.Fatalf("event type = %q, want %q", emitted[0].Type, assistant.EventTypeReasoning)
	}
	if emitted[0].Summary != "Plan the task carefully." {
		t.Fatalf("summary = %q, want extracted reasoning text", emitted[0].Summary)
	}
}

func TestIsMeaningfulReasoningText(t *testing.T) {
	t.Parallel()

	cases := []struct {
		value string
		want  bool
	}{
		{value: "", want: false},
		{value: "   ", want: false},
		{value: "[]", want: false},
		{value: "{}", want: false},
		{value: "null", want: false},
		{value: `""`, want: false},
		{value: "Planner normalized the task.", want: true},
	}

	for _, tc := range cases {
		if got := isMeaningfulReasoningText(tc.value); got != tc.want {
			t.Fatalf("isMeaningfulReasoningText(%q) = %t, want %t", tc.value, got, tc.want)
		}
	}
}
