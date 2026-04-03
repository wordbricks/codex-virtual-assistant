package scheduler

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/siisee11/CodexVirtualAssistant/internal/assistant"
)

func TestSchedulerRunOnceTriggersDueRuns(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, time.April, 3, 13, 0, 0, 0, time.UTC)
	repo := &fakeRepository{
		pending: []assistant.ScheduledRun{
			{
				ID:                    "scheduled_1",
				ChatID:                "chat_1",
				ParentRunID:           "run_parent",
				UserRequestRaw:        "Call hospital A.",
				MaxGenerationAttempts: 2,
				ScheduledFor:          now.Add(-time.Minute),
				Status:                assistant.ScheduledRunStatusPending,
				CreatedAt:             now.Add(-time.Hour),
			},
		},
	}
	runs := &fakeRunCreator{
		run: assistant.Run{ID: "run_created"},
	}
	publisher := &fakePublisher{}

	s := New(repo, runs, publisher, time.Minute, func() time.Time { return now })
	if err := s.RunOnce(context.Background()); err != nil {
		t.Fatalf("RunOnce() error = %v", err)
	}

	if len(runs.calls) != 1 {
		t.Fatalf("len(runs.calls) = %d, want 1", len(runs.calls))
	}
	if len(repo.triggered) != 1 || repo.triggered[0].scheduledRunID != "scheduled_1" {
		t.Fatalf("triggered = %#v, want scheduled_1", repo.triggered)
	}
	if len(publisher.events) != 1 || publisher.events[0].Type != assistant.EventTypeScheduleTriggered {
		t.Fatalf("events = %#v, want one schedule_triggered event", publisher.events)
	}
}

func TestSchedulerRunOnceMarksFailures(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, time.April, 3, 13, 0, 0, 0, time.UTC)
	repo := &fakeRepository{
		pending: []assistant.ScheduledRun{
			{
				ID:                    "scheduled_2",
				ChatID:                "chat_1",
				ParentRunID:           "run_parent",
				UserRequestRaw:        "Call hospital B.",
				MaxGenerationAttempts: 2,
				ScheduledFor:          now.Add(-time.Minute),
				Status:                assistant.ScheduledRunStatusPending,
				CreatedAt:             now.Add(-time.Hour),
			},
		},
	}
	runs := &fakeRunCreator{err: errors.New("boom")}
	publisher := &fakePublisher{}

	s := New(repo, runs, publisher, time.Minute, func() time.Time { return now })
	if err := s.RunOnce(context.Background()); err != nil {
		t.Fatalf("RunOnce() error = %v", err)
	}

	if len(repo.statusUpdates) != 1 || repo.statusUpdates[0].status != assistant.ScheduledRunStatusFailed {
		t.Fatalf("statusUpdates = %#v, want failed update", repo.statusUpdates)
	}
	if len(publisher.events) != 1 || publisher.events[0].Type != assistant.EventTypeScheduleFailed {
		t.Fatalf("events = %#v, want one schedule_failed event", publisher.events)
	}
}

type fakeRepository struct {
	pending       []assistant.ScheduledRun
	triggered     []triggeredUpdate
	statusUpdates []statusUpdate
}

type triggeredUpdate struct {
	scheduledRunID string
	runID          string
}

type statusUpdate struct {
	scheduledRunID string
	status         assistant.ScheduledRunStatus
	message        string
}

func (f *fakeRepository) ListPendingScheduledRuns(context.Context, time.Time) ([]assistant.ScheduledRun, error) {
	return append([]assistant.ScheduledRun(nil), f.pending...), nil
}

func (f *fakeRepository) UpdateScheduledRunTriggered(_ context.Context, scheduledRunID, runID string, _ time.Time) error {
	f.triggered = append(f.triggered, triggeredUpdate{scheduledRunID: scheduledRunID, runID: runID})
	return nil
}

func (f *fakeRepository) UpdateScheduledRunStatus(_ context.Context, scheduledRunID string, status assistant.ScheduledRunStatus, message string) error {
	f.statusUpdates = append(f.statusUpdates, statusUpdate{scheduledRunID: scheduledRunID, status: status, message: message})
	return nil
}

func (f *fakeRepository) AddRunEvent(context.Context, assistant.RunEvent) error {
	return nil
}

type fakeRunCreator struct {
	run   assistant.Run
	err   error
	calls []string
}

func (f *fakeRunCreator) CreateRun(_ context.Context, userRequest string, _ int, _ string) (assistant.Run, error) {
	f.calls = append(f.calls, userRequest)
	if f.err != nil {
		return assistant.Run{}, f.err
	}
	return f.run, nil
}

type fakePublisher struct {
	events []assistant.RunEvent
}

func (f *fakePublisher) Publish(_ context.Context, event assistant.RunEvent) error {
	f.events = append(f.events, event)
	return nil
}
