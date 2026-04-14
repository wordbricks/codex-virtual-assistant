package main

import (
	"context"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/siisee11/CodexVirtualAssistant/internal/assistant"
	"github.com/siisee11/CodexVirtualAssistant/internal/store"
)

func TestChatPickerModelSelectsChat(t *testing.T) {
	t.Parallel()

	chats := []assistant.Chat{
		{ID: "chat_1", Title: "One", Status: assistant.RunStatusCompleted},
		{ID: "chat_2", Title: "Two", Status: assistant.RunStatusWaiting},
	}

	model := newChatPickerModel(context.Background(), chats)
	updated, _ := model.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	model = updated.(chatPickerModel)

	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyDown})
	model = updated.(chatPickerModel)
	if model.selected != 1 {
		t.Fatalf("selected = %d, want 1", model.selected)
	}

	updated, cmd := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model = updated.(chatPickerModel)
	if !model.confirmed {
		t.Fatal("confirmed = false, want true")
	}
	if cmd == nil {
		t.Fatal("enter cmd = nil, want quit command")
	}
}

func TestChatPickerModelPreviewIncludesLatestRun(t *testing.T) {
	t.Parallel()

	now := time.Unix(100, 0).UTC()
	model := newChatPickerModel(context.Background(), []assistant.Chat{{
		ID:          "chat_1",
		RootRunID:   "run_1",
		LatestRunID: "run_2",
		Title:       "Release train",
		Status:      assistant.RunStatusWaiting,
		UpdatedAt:   now,
	}})
	model.width = 100
	model.height = 24
	model.ready = true

	view := model.renderPreview()
	if !strings.Contains(view, "Latest run: run_2") {
		t.Fatalf("renderPreview() = %q, want latest run", view)
	}
	if !strings.Contains(view, "Title: Release train") {
		t.Fatalf("renderPreview() = %q, want title", view)
	}
}

func TestChatViewerModelRendersRecord(t *testing.T) {
	t.Parallel()

	rec := &store.ChatRecord{
		Chat: assistant.Chat{
			ID:        "chat_1",
			Title:     "Release train",
			Status:    assistant.RunStatusCompleted,
			UpdatedAt: time.Unix(100, 0).UTC(),
		},
		Runs: []store.RunRecord{{
			Run: assistant.Run{
				ID:             "run_1",
				ChatID:         "chat_1",
				Status:         assistant.RunStatusCompleted,
				Phase:          assistant.RunPhaseCompleted,
				UserRequestRaw: "Write release notes",
				CreatedAt:      time.Unix(50, 0).UTC(),
			},
		}},
	}

	model := newChatViewerModel(context.Background(), rec)
	updated, _ := model.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	model = updated.(chatViewerModel)

	view := model.View()
	if !strings.Contains(view, "Chat: chat_1") {
		t.Fatalf("View() = %q, want chat id", view)
	}
	if !strings.Contains(view, "Write release notes") {
		t.Fatalf("View() = %q, want run request", view)
	}
}
