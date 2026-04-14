package main

import (
	"context"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/siisee11/CodexVirtualAssistant/internal/assistant"
)

func TestSchedulePickerModelSelectsRun(t *testing.T) {
	t.Parallel()

	now := time.Unix(100, 0).UTC()
	items := []assistant.ScheduledRun{
		{ID: "scheduled_1", Status: assistant.ScheduledRunStatusPending, ScheduledFor: now},
		{ID: "scheduled_2", Status: assistant.ScheduledRunStatusTriggered, ScheduledFor: now.Add(time.Hour)},
	}

	model := newSchedulePickerModel(context.Background(), items)
	updated, _ := model.Update(tea.WindowSizeMsg{Width: 100, Height: 28})
	model = updated.(schedulePickerModel)

	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyDown})
	model = updated.(schedulePickerModel)
	if model.selected != 1 {
		t.Fatalf("selected = %d, want 1", model.selected)
	}

	updated, cmd := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model = updated.(schedulePickerModel)
	if !model.confirmed {
		t.Fatal("confirmed = false, want true")
	}
	if cmd == nil {
		t.Fatal("enter cmd = nil, want quit command")
	}
}

func TestBuildScheduledRunPreviewContentIncludesKeyDetails(t *testing.T) {
	t.Parallel()

	now := time.Unix(100, 0).UTC()
	triggeredAt := now.Add(2 * time.Minute)
	item := assistant.ScheduledRun{
		ID:                    "scheduled_preview",
		Status:                assistant.ScheduledRunStatusTriggered,
		ChatID:                "chat_123",
		ParentRunID:           "run_parent",
		UserRequestRaw:        "Re-evaluate the overnight schedule and keep only canonical slots.",
		MaxGenerationAttempts: 3,
		ScheduledFor:          now.Add(time.Hour),
		CreatedAt:             now,
		TriggeredAt:           &triggeredAt,
		RunID:                 "run_child",
		ErrorMessage:          "transient scheduler conflict",
	}

	view := buildScheduledRunPreviewContent(item, 120)
	for _, fragment := range []string{
		"scheduled_preview",
		"chat_123",
		"run_parent",
		"run_child",
		"transient scheduler conflict",
		"Re-evaluate the overnight schedule",
	} {
		if !strings.Contains(view, fragment) {
			t.Fatalf("renderPreview() missing %q in %q", fragment, view)
		}
	}
}

func TestSchedulePickerFiltersItemsFromTypedQuery(t *testing.T) {
	t.Parallel()

	now := time.Unix(100, 0).UTC()
	items := []assistant.ScheduledRun{
		{
			ID:             "scheduled_alpha",
			Status:         assistant.ScheduledRunStatusPending,
			ChatID:         "chat_growth",
			ParentRunID:    "run_alpha",
			UserRequestRaw: "Review growth schedule",
			ScheduledFor:   now,
			CreatedAt:      now,
		},
		{
			ID:             "scheduled_beta",
			Status:         assistant.ScheduledRunStatusCancelled,
			ChatID:         "chat_ops",
			ParentRunID:    "run_beta",
			UserRequestRaw: "Retry ops workflow",
			ScheduledFor:   now.Add(time.Hour),
			CreatedAt:      now,
		},
	}

	model := newSchedulePickerModel(context.Background(), items)
	updated, _ := model.Update(tea.WindowSizeMsg{Width: 100, Height: 28})
	model = updated.(schedulePickerModel)

	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("g")})
	model = updated.(schedulePickerModel)
	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("r")})
	model = updated.(schedulePickerModel)

	if model.filterQuery != "gr" {
		t.Fatalf("filterQuery = %q, want %q", model.filterQuery, "gr")
	}
	if len(model.items) != 1 {
		t.Fatalf("len(filtered items) = %d, want 1", len(model.items))
	}
	if model.items[0].ID != "scheduled_alpha" {
		t.Fatalf("filtered item = %q, want scheduled_alpha", model.items[0].ID)
	}

	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyBackspace})
	model = updated.(schedulePickerModel)
	if model.filterQuery != "g" {
		t.Fatalf("filterQuery after backspace = %q, want %q", model.filterQuery, "g")
	}

	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyCtrlU})
	model = updated.(schedulePickerModel)
	if model.filterQuery != "" {
		t.Fatalf("filterQuery after ctrl+u = %q, want empty", model.filterQuery)
	}
	if len(model.items) != 2 {
		t.Fatalf("len(items) after clear = %d, want 2", len(model.items))
	}
}
