package assistant

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
)

type ScheduledRunStatus string

const (
	ScheduledRunStatusPending   ScheduledRunStatus = "pending"
	ScheduledRunStatusTriggered ScheduledRunStatus = "triggered"
	ScheduledRunStatusCancelled ScheduledRunStatus = "cancelled"
	ScheduledRunStatusFailed    ScheduledRunStatus = "failed"
)

type ScheduleEntry struct {
	ScheduledFor string `json:"scheduled_for"`
	Prompt       string `json:"prompt"`
}

type SchedulePlan struct {
	Entries []ScheduleEntry `json:"entries"`
}

type ScheduledRun struct {
	ID                    string             `json:"id"`
	ChatID                string             `json:"chat_id"`
	ParentRunID           string             `json:"parent_run_id"`
	UserRequestRaw        string             `json:"user_request_raw"`
	MaxGenerationAttempts int                `json:"max_generation_attempts"`
	ScheduledFor          time.Time          `json:"scheduled_for"`
	Status                ScheduledRunStatus `json:"status"`
	RunID                 string             `json:"run_id,omitempty"`
	ErrorMessage          string             `json:"error_message,omitempty"`
	CreatedAt             time.Time          `json:"created_at"`
	TriggeredAt           *time.Time         `json:"triggered_at,omitempty"`
}

func (p SchedulePlan) Validate() error {
	if len(p.Entries) == 0 {
		return errors.New("schedule plan entries are required")
	}
	for idx, entry := range p.Entries {
		if err := entry.Validate(); err != nil {
			return fmt.Errorf("entry %d invalid: %w", idx, err)
		}
	}
	return nil
}

func (e ScheduleEntry) Validate() error {
	switch {
	case strings.TrimSpace(e.ScheduledFor) == "":
		return errors.New("scheduled_for is required")
	case strings.TrimSpace(e.Prompt) == "":
		return errors.New("prompt is required")
	default:
		return nil
	}
}

func (r ScheduledRun) Validate() error {
	switch {
	case strings.TrimSpace(r.ID) == "":
		return errors.New("scheduled run id is required")
	case strings.TrimSpace(r.ChatID) == "":
		return errors.New("scheduled run chat id is required")
	case strings.TrimSpace(r.ParentRunID) == "":
		return errors.New("scheduled run parent run id is required")
	case strings.TrimSpace(r.UserRequestRaw) == "":
		return errors.New("scheduled run user request is required")
	case r.MaxGenerationAttempts <= 0:
		return errors.New("scheduled run max generation attempts must be positive")
	case r.ScheduledFor.IsZero():
		return errors.New("scheduled run scheduled_for is required")
	case r.CreatedAt.IsZero():
		return errors.New("scheduled run created_at is required")
	}

	switch r.Status {
	case ScheduledRunStatusPending, ScheduledRunStatusTriggered, ScheduledRunStatusCancelled, ScheduledRunStatusFailed:
	default:
		return errors.New("scheduled run status is invalid")
	}

	if r.Status == ScheduledRunStatusTriggered && strings.TrimSpace(r.RunID) == "" {
		return errors.New("scheduled run run id is required when triggered")
	}
	return nil
}

func AllScheduledRunStatuses() []ScheduledRunStatus {
	return []ScheduledRunStatus{
		ScheduledRunStatusPending,
		ScheduledRunStatusTriggered,
		ScheduledRunStatusCancelled,
		ScheduledRunStatusFailed,
	}
}

func ParseScheduledFor(raw string, now time.Time) (time.Time, error) {
	value := strings.TrimSpace(raw)
	if value == "" {
		return time.Time{}, errors.New("scheduled_for is required")
	}
	if now.IsZero() {
		now = time.Now().UTC()
	}
	now = now.UTC()

	if parsed, err := time.Parse(time.RFC3339, value); err == nil {
		return parsed.UTC(), nil
	}
	if parsed, err := time.Parse(time.RFC3339Nano, value); err == nil {
		return parsed.UTC(), nil
	}

	if strings.HasPrefix(value, "+") || strings.HasPrefix(value, "-") {
		duration, err := time.ParseDuration(value)
		if err != nil {
			return time.Time{}, fmt.Errorf("parse relative schedule %q: %w", value, err)
		}
		return now.Add(duration).UTC(), nil
	}

	if parsed, ok, err := parseClockTime(value, now); err != nil {
		return time.Time{}, err
	} else if ok {
		return parsed.UTC(), nil
	}

	return time.Time{}, fmt.Errorf("unsupported scheduled_for value %q", value)
}

func parseClockTime(raw string, now time.Time) (time.Time, bool, error) {
	parts := strings.Split(raw, ":")
	if len(parts) != 2 && len(parts) != 3 {
		return time.Time{}, false, nil
	}

	hour, err := strconv.Atoi(parts[0])
	if err != nil {
		return time.Time{}, false, fmt.Errorf("parse hour from %q: %w", raw, err)
	}
	minute, err := strconv.Atoi(parts[1])
	if err != nil {
		return time.Time{}, false, fmt.Errorf("parse minute from %q: %w", raw, err)
	}
	second := 0
	if len(parts) == 3 {
		second, err = strconv.Atoi(parts[2])
		if err != nil {
			return time.Time{}, false, fmt.Errorf("parse second from %q: %w", raw, err)
		}
	}
	if hour < 0 || hour > 23 || minute < 0 || minute > 59 || second < 0 || second > 59 {
		return time.Time{}, false, fmt.Errorf("clock time %q is out of range", raw)
	}

	candidate := time.Date(now.Year(), now.Month(), now.Day(), hour, minute, second, 0, time.UTC)
	if candidate.Before(now) {
		candidate = candidate.Add(24 * time.Hour)
	}
	return candidate, true, nil
}
