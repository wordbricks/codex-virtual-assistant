package api

import (
	"context"
	"testing"
	"time"

	"github.com/siisee11/CodexVirtualAssistant/internal/assistant"
	"github.com/siisee11/CodexVirtualAssistant/internal/store"
)

func TestEventBrokerDispatchesCompletedHooksFromPhaseChange(t *testing.T) {
	t.Parallel()

	broker := NewEventBroker()
	broker.SetSnapshotLoader(fakeRunRecordLoader{
		record: store.RunRecord{
			Run: assistant.Run{
				ID:     "run_completed",
				Status: assistant.RunStatusCompleted,
				Phase:  assistant.RunPhaseCompleted,
			},
		},
	})

	payloads := make(chan HookPayload, 2)
	unregisterPhase := broker.RegisterHook(HookOnPhaseChanged, func(_ context.Context, payload HookPayload) error {
		payloads <- payload
		return nil
	})
	defer unregisterPhase()
	unregisterCompleted := broker.RegisterHook(HookOnRunCompleted, func(_ context.Context, payload HookPayload) error {
		payloads <- payload
		return nil
	})
	defer unregisterCompleted()

	err := broker.Publish(context.Background(), assistant.RunEvent{
		RunID: "run_completed",
		Type:  assistant.EventTypePhaseChanged,
		Phase: assistant.RunPhaseCompleted,
	})
	if err != nil {
		t.Fatalf("Publish() error = %v", err)
	}

	got := waitForHookPayloads(t, payloads, 2)
	if !hasHookName(got, HookOnPhaseChanged) {
		t.Fatalf("hook payloads = %#v, want %q", got, HookOnPhaseChanged)
	}
	if !hasHookName(got, HookOnRunCompleted) {
		t.Fatalf("hook payloads = %#v, want %q", got, HookOnRunCompleted)
	}
	for _, payload := range got {
		if payload.Record == nil {
			t.Fatalf("payload.Record = nil for hook %q, want run snapshot", payload.Name)
		}
		if payload.Record.Run.Status != assistant.RunStatusCompleted {
			t.Fatalf("payload.Record.Run.Status = %q, want %q", payload.Record.Run.Status, assistant.RunStatusCompleted)
		}
	}
}

func TestEventBrokerDispatchesRunExhaustedHookFromSnapshot(t *testing.T) {
	t.Parallel()

	broker := NewEventBroker()
	broker.SetSnapshotLoader(fakeRunRecordLoader{
		record: store.RunRecord{
			Run: assistant.Run{
				ID:     "run_exhausted",
				Status: assistant.RunStatusExhausted,
				Phase:  assistant.RunPhaseFailed,
			},
		},
	})

	payloads := make(chan HookPayload, 1)
	unregister := broker.RegisterHook(HookOnRunExhausted, func(_ context.Context, payload HookPayload) error {
		payloads <- payload
		return nil
	})
	defer unregister()

	err := broker.Publish(context.Background(), assistant.RunEvent{
		RunID: "run_exhausted",
		Type:  assistant.EventTypePhaseChanged,
		Phase: assistant.RunPhaseFailed,
	})
	if err != nil {
		t.Fatalf("Publish() error = %v", err)
	}

	got := waitForHookPayloads(t, payloads, 1)
	if got[0].Name != HookOnRunExhausted {
		t.Fatalf("hook name = %q, want %q", got[0].Name, HookOnRunExhausted)
	}
	if got[0].Record == nil || got[0].Record.Run.Status != assistant.RunStatusExhausted {
		t.Fatalf("payload.Record = %#v, want exhausted snapshot", got[0].Record)
	}
}

func TestEventBrokerDispatchesRunFailedHookFromSnapshot(t *testing.T) {
	t.Parallel()

	broker := NewEventBroker()
	broker.SetSnapshotLoader(fakeRunRecordLoader{
		record: store.RunRecord{
			Run: assistant.Run{
				ID:     "run_failed",
				Status: assistant.RunStatusFailed,
				Phase:  assistant.RunPhaseFailed,
			},
		},
	})

	payloads := make(chan HookPayload, 1)
	unregister := broker.RegisterHook(HookOnRunFailed, func(_ context.Context, payload HookPayload) error {
		payloads <- payload
		return nil
	})
	defer unregister()

	err := broker.Publish(context.Background(), assistant.RunEvent{
		RunID: "run_failed",
		Type:  assistant.EventTypePhaseChanged,
		Phase: assistant.RunPhaseFailed,
	})
	if err != nil {
		t.Fatalf("Publish() error = %v", err)
	}

	got := waitForHookPayloads(t, payloads, 1)
	if got[0].Name != HookOnRunFailed {
		t.Fatalf("hook name = %q, want %q", got[0].Name, HookOnRunFailed)
	}
}

func TestEventBrokerHookUnregisterStopsFutureDispatch(t *testing.T) {
	t.Parallel()

	broker := NewEventBroker()
	payloads := make(chan HookPayload, 1)
	unregister := broker.RegisterHook(HookOnRunStarted, func(_ context.Context, payload HookPayload) error {
		payloads <- payload
		return nil
	})
	unregister()

	err := broker.Publish(context.Background(), assistant.RunEvent{
		RunID: "run_started",
		Type:  assistant.EventTypeRunCreated,
		Phase: assistant.RunPhaseQueued,
	})
	if err != nil {
		t.Fatalf("Publish() error = %v", err)
	}

	select {
	case payload := <-payloads:
		t.Fatalf("received unexpected payload %#v after unregister", payload)
	case <-time.After(150 * time.Millisecond):
	}
}

type fakeRunRecordLoader struct {
	record store.RunRecord
	err    error
}

func (f fakeRunRecordLoader) GetRunRecord(context.Context, string) (store.RunRecord, error) {
	return f.record, f.err
}

func waitForHookPayloads(t *testing.T, ch <-chan HookPayload, want int) []HookPayload {
	t.Helper()

	payloads := make([]HookPayload, 0, want)
	deadline := time.After(2 * time.Second)
	for len(payloads) < want {
		select {
		case payload := <-ch:
			payloads = append(payloads, payload)
		case <-deadline:
			t.Fatalf("timed out waiting for %d hook payloads; got %#v", want, payloads)
		}
	}
	return payloads
}

func hasHookName(payloads []HookPayload, name HookName) bool {
	for _, payload := range payloads {
		if payload.Name == name {
			return true
		}
	}
	return false
}
