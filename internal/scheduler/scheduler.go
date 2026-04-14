package scheduler

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/siisee11/CodexVirtualAssistant/internal/assistant"
)

type Repository interface {
	ListPendingScheduledRuns(context.Context, time.Time) ([]assistant.ScheduledRun, error)
	SaveScheduledRun(context.Context, assistant.ScheduledRun) error
	UpdateScheduledRunTriggered(context.Context, string, string, time.Time) error
	UpdateScheduledRunStatus(context.Context, string, assistant.ScheduledRunStatus, string) error
	AddRunEvent(context.Context, assistant.RunEvent) error
}

type RunCreator interface {
	CreateRun(context.Context, string, int, string) (assistant.Run, error)
}

type EventPublisher interface {
	Publish(context.Context, assistant.RunEvent) error
}

type Scheduler struct {
	repo     Repository
	runs     RunCreator
	events   EventPublisher
	interval time.Duration
	now      func() time.Time
}

func New(repo Repository, runs RunCreator, events EventPublisher, interval time.Duration, now func() time.Time) *Scheduler {
	if interval <= 0 {
		interval = 30 * time.Second
	}
	if now == nil {
		now = time.Now
	}
	return &Scheduler{
		repo:     repo,
		runs:     runs,
		events:   events,
		interval: interval,
		now:      now,
	}
}

func (s *Scheduler) Run(ctx context.Context) error {
	if err := s.RunOnce(ctx); err != nil {
		return err
	}

	ticker := time.NewTicker(s.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			if err := s.RunOnce(ctx); err != nil {
				return err
			}
		}
	}
}

func (s *Scheduler) RunOnce(ctx context.Context) error {
	if s.repo == nil || s.runs == nil {
		return nil
	}
	dueRuns, err := s.repo.ListPendingScheduledRuns(ctx, s.now().UTC())
	if err != nil {
		return err
	}
	for _, scheduledRun := range dueRuns {
		if err := s.triggerScheduledRun(ctx, scheduledRun); err != nil {
			return err
		}
	}
	return nil
}

func (s *Scheduler) triggerScheduledRun(ctx context.Context, scheduledRun assistant.ScheduledRun) error {
	createdRun, err := s.runs.CreateRun(ctx, scheduledRun.UserRequestRaw, scheduledRun.MaxGenerationAttempts, scheduledRun.ParentRunID)
	if err != nil {
		message := strings.TrimSpace(err.Error())
		if strings.TrimSpace(scheduledRun.CronExpr) != "" {
			nextScheduledFor, nextErr := assistant.NextCronOccurrence(scheduledRun.CronExpr, s.now().In(time.Local))
			if nextErr != nil {
				return nextErr
			}
			triggeredAt := s.now().UTC()
			scheduledRun.Status = assistant.ScheduledRunStatusPending
			scheduledRun.RunID = ""
			scheduledRun.ErrorMessage = message
			scheduledRun.TriggeredAt = &triggeredAt
			scheduledRun.ScheduledFor = nextScheduledFor
			if saveErr := s.repo.SaveScheduledRun(ctx, scheduledRun); saveErr != nil {
				return saveErr
			}
		} else {
			if updateErr := s.repo.UpdateScheduledRunStatus(ctx, scheduledRun.ID, assistant.ScheduledRunStatusFailed, message); updateErr != nil {
				return updateErr
			}
		}
		return s.publish(ctx, assistant.RunEvent{
			ID:        assistant.NewID("event", s.now().UTC()),
			RunID:     scheduledRun.ParentRunID,
			Type:      assistant.EventTypeScheduleFailed,
			Phase:     assistant.RunPhaseScheduling,
			Summary:   firstNonEmpty(message, "Scheduled run failed to trigger."),
			CreatedAt: s.now().UTC(),
			Data: map[string]any{
				"scheduled_run_id": scheduledRun.ID,
				"error_message":    message,
			},
		})
	}

	triggeredAt := s.now().UTC()
	if strings.TrimSpace(scheduledRun.CronExpr) != "" {
		nextScheduledFor, err := assistant.NextCronOccurrence(scheduledRun.CronExpr, s.now().In(time.Local))
		if err != nil {
			return err
		}
		scheduledRun.Status = assistant.ScheduledRunStatusPending
		scheduledRun.RunID = createdRun.ID
		scheduledRun.ErrorMessage = ""
		scheduledRun.TriggeredAt = &triggeredAt
		scheduledRun.ScheduledFor = nextScheduledFor
		if err := s.repo.SaveScheduledRun(ctx, scheduledRun); err != nil {
			return err
		}
	} else {
		if err := s.repo.UpdateScheduledRunTriggered(ctx, scheduledRun.ID, createdRun.ID, triggeredAt); err != nil {
			return err
		}
	}
	return s.publish(ctx, assistant.RunEvent{
		ID:        assistant.NewID("event", triggeredAt),
		RunID:     scheduledRun.ParentRunID,
		Type:      assistant.EventTypeScheduleTriggered,
		Phase:     assistant.RunPhaseScheduling,
		Summary:   fmt.Sprintf("Triggered scheduled run %s.", scheduledRun.ID),
		CreatedAt: triggeredAt,
		Data: map[string]any{
			"scheduled_run_id": scheduledRun.ID,
			"created_run_id":   createdRun.ID,
		},
	})
}

func (s *Scheduler) publish(ctx context.Context, event assistant.RunEvent) error {
	if s.repo != nil {
		if err := s.repo.AddRunEvent(ctx, event); err != nil {
			return err
		}
	}
	if s.events == nil {
		return nil
	}
	return s.events.Publish(ctx, event)
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
