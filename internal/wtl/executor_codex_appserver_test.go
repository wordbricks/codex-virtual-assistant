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

func TestPhaseForAttemptRoleSupportsGateAnswerAndReport(t *testing.T) {
	t.Parallel()

	if got := phaseForAttemptRole(assistant.AttemptRoleGate); got != assistant.RunPhaseGating {
		t.Fatalf("phaseForAttemptRole(gate) = %q, want %q", got, assistant.RunPhaseGating)
	}
	if got := phaseForAttemptRole(assistant.AttemptRoleAnswer); got != assistant.RunPhaseAnswering {
		t.Fatalf("phaseForAttemptRole(answer) = %q, want %q", got, assistant.RunPhaseAnswering)
	}
	if got := phaseForAttemptRole(assistant.AttemptRoleReporter); got != assistant.RunPhaseReporting {
		t.Fatalf("phaseForAttemptRole(reporter) = %q, want %q", got, assistant.RunPhaseReporting)
	}
}

func TestPhaseOutputSchemaSupportsGateAnswerAndReport(t *testing.T) {
	t.Parallel()

	gate := phaseOutputSchema(assistant.AttemptRoleGate)
	if gate == nil {
		t.Fatal("phaseOutputSchema(gate) = nil")
	}
	if _, ok := gate["properties"].(map[string]any)["route"]; !ok {
		t.Fatalf("gate schema properties = %#v, want route", gate["properties"])
	}

	answer := phaseOutputSchema(assistant.AttemptRoleAnswer)
	if answer == nil {
		t.Fatal("phaseOutputSchema(answer) = nil")
	}
	properties, ok := answer["properties"].(map[string]any)
	if !ok {
		t.Fatalf("answer schema properties type = %T, want map[string]any", answer["properties"])
	}
	if _, ok := properties["needs_user_input"]; !ok {
		t.Fatalf("answer schema properties = %#v, want needs_user_input", properties)
	}
	if _, ok := properties["wait_prompt"]; !ok {
		t.Fatalf("answer schema properties = %#v, want wait_prompt", properties)
	}

	report := phaseOutputSchema(assistant.AttemptRoleReporter)
	if report == nil {
		t.Fatal("phaseOutputSchema(reporter) = nil")
	}
	reportProperties, ok := report["properties"].(map[string]any)
	if !ok {
		t.Fatalf("report schema properties type = %T, want map[string]any", report["properties"])
	}
	if _, ok := reportProperties["delivery_status"]; !ok {
		t.Fatalf("report schema properties = %#v, want delivery_status", reportProperties)
	}
	if _, ok := reportProperties["report_payload"]; !ok {
		t.Fatalf("report schema properties = %#v, want report_payload", reportProperties)
	}
}
