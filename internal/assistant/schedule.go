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
	CronExpr              string             `json:"cron_expr,omitempty"`
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
	case strings.TrimSpace(r.CronExpr) != "":
		if _, err := ParseCronExpr(r.CronExpr); err != nil {
			return fmt.Errorf("scheduled run cron expr is invalid: %w", err)
		}
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

type CronExpr struct {
	Minute fieldMatcher
	Hour   fieldMatcher
	Dom    fieldMatcher
	Month  fieldMatcher
	Dow    fieldMatcher
}

func ParseCronExpr(raw string) (CronExpr, error) {
	parts := strings.Fields(strings.TrimSpace(raw))
	if len(parts) != 5 {
		return CronExpr{}, fmt.Errorf("cron expr %q must have 5 fields", raw)
	}

	minute, err := parseCronField(parts[0], 0, 59)
	if err != nil {
		return CronExpr{}, fmt.Errorf("minute: %w", err)
	}
	hour, err := parseCronField(parts[1], 0, 23)
	if err != nil {
		return CronExpr{}, fmt.Errorf("hour: %w", err)
	}
	dom, err := parseCronField(parts[2], 1, 31)
	if err != nil {
		return CronExpr{}, fmt.Errorf("day of month: %w", err)
	}
	month, err := parseCronField(parts[3], 1, 12)
	if err != nil {
		return CronExpr{}, fmt.Errorf("month: %w", err)
	}
	dow, err := parseCronField(parts[4], 0, 6)
	if err != nil {
		return CronExpr{}, fmt.Errorf("day of week: %w", err)
	}

	return CronExpr{
		Minute: minute,
		Hour:   hour,
		Dom:    dom,
		Month:  month,
		Dow:    dow,
	}, nil
}

func NextCronOccurrence(raw string, after time.Time) (time.Time, error) {
	expr, err := ParseCronExpr(raw)
	if err != nil {
		return time.Time{}, err
	}
	if after.IsZero() {
		after = time.Now()
	}
	loc := after.Location()
	if loc == nil {
		loc = time.Local
	}
	cursor := after.In(loc).Truncate(time.Minute).Add(time.Minute)
	deadline := cursor.AddDate(5, 0, 0)
	for !cursor.After(deadline) {
		if expr.matches(cursor) {
			return cursor.UTC(), nil
		}
		cursor = cursor.Add(time.Minute)
	}
	return time.Time{}, fmt.Errorf("cron expr %q has no occurrence within 5 years", raw)
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

type fieldMatcher struct {
	any    bool
	values map[int]struct{}
}

func (m fieldMatcher) match(value int) bool {
	if m.any {
		return true
	}
	_, ok := m.values[value]
	return ok
}

func (c CronExpr) matches(ts time.Time) bool {
	if !c.Minute.match(ts.Minute()) ||
		!c.Hour.match(ts.Hour()) ||
		!c.Month.match(int(ts.Month())) {
		return false
	}
	domMatch := c.Dom.match(ts.Day())
	dowMatch := c.Dow.match(int(ts.Weekday()))
	switch {
	case c.Dom.any && c.Dow.any:
		return true
	case c.Dom.any:
		return dowMatch
	case c.Dow.any:
		return domMatch
	default:
		return domMatch || dowMatch
	}
}

func parseCronField(raw string, minValue, maxValue int) (fieldMatcher, error) {
	raw = strings.TrimSpace(raw)
	if raw == "*" {
		return fieldMatcher{any: true}, nil
	}
	values := make(map[int]struct{})
	for _, segment := range strings.Split(raw, ",") {
		segment = strings.TrimSpace(segment)
		if segment == "" {
			return fieldMatcher{}, fmt.Errorf("empty segment in %q", raw)
		}
		base := segment
		step := 1
		if strings.Contains(segment, "/") {
			parts := strings.Split(segment, "/")
			if len(parts) != 2 {
				return fieldMatcher{}, fmt.Errorf("invalid step segment %q", segment)
			}
			base = strings.TrimSpace(parts[0])
			parsedStep, err := strconv.Atoi(strings.TrimSpace(parts[1]))
			if err != nil || parsedStep <= 0 {
				return fieldMatcher{}, fmt.Errorf("invalid step in %q", segment)
			}
			step = parsedStep
		}

		rangeStart, rangeEnd, err := parseCronRange(base, minValue, maxValue)
		if err != nil {
			return fieldMatcher{}, err
		}
		for value := rangeStart; value <= rangeEnd; value += step {
			values[value] = struct{}{}
		}
	}
	if len(values) == 0 {
		return fieldMatcher{}, fmt.Errorf("field %q selects no values", raw)
	}
	return fieldMatcher{values: values}, nil
}

func parseCronRange(raw string, minValue, maxValue int) (int, int, error) {
	raw = strings.TrimSpace(raw)
	switch {
	case raw == "" || raw == "*":
		return minValue, maxValue, nil
	case strings.Contains(raw, "-"):
		parts := strings.Split(raw, "-")
		if len(parts) != 2 {
			return 0, 0, fmt.Errorf("invalid range %q", raw)
		}
		start, err := strconv.Atoi(strings.TrimSpace(parts[0]))
		if err != nil {
			return 0, 0, fmt.Errorf("invalid range start in %q", raw)
		}
		end, err := strconv.Atoi(strings.TrimSpace(parts[1]))
		if err != nil {
			return 0, 0, fmt.Errorf("invalid range end in %q", raw)
		}
		if start > end {
			return 0, 0, fmt.Errorf("descending range %q", raw)
		}
		if start < minValue || end > maxValue {
			return 0, 0, fmt.Errorf("range %q is out of bounds", raw)
		}
		return start, end, nil
	default:
		value, err := strconv.Atoi(raw)
		if err != nil {
			return 0, 0, fmt.Errorf("invalid value %q", raw)
		}
		if value < minValue || value > maxValue {
			return 0, 0, fmt.Errorf("value %q is out of bounds", raw)
		}
		return value, value, nil
	}
}
