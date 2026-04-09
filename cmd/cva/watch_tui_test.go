package main

import (
	"context"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/siisee11/CodexVirtualAssistant/internal/assistant"
	"github.com/siisee11/CodexVirtualAssistant/internal/store"
)

type fakeWatchRunLoader struct {
	chats []assistant.Chat
	runs  map[string]*store.RunRecord
}

func (f *fakeWatchRunLoader) ListChats(context.Context) ([]assistant.Chat, error) {
	return f.chats, nil
}

func (f *fakeWatchRunLoader) GetRun(_ context.Context, runID string) (*store.RunRecord, error) {
	return f.runs[runID], nil
}

func TestSelectWatchOutputMode(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		jsonMode  bool
		stdinTTY  bool
		stdoutTTY bool
		want      watchOutputMode
	}{
		{name: "json wins", jsonMode: true, stdinTTY: true, stdoutTTY: true, want: watchOutputModeJSON},
		{name: "interactive uses tui", stdinTTY: true, stdoutTTY: true, want: watchOutputModeTUI},
		{name: "non tty uses text", stdoutTTY: true, want: watchOutputModeText},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := selectWatchOutputMode(tc.jsonMode, tc.stdinTTY, tc.stdoutTTY); got != tc.want {
				t.Fatalf("selectWatchOutputMode(%v, %v, %v) = %q, want %q", tc.jsonMode, tc.stdinTTY, tc.stdoutTTY, got, tc.want)
			}
		})
	}
}

func TestBuildWatchPreviewPrefersWaitingPrompt(t *testing.T) {
	t.Parallel()

	record := &store.RunRecord{
		Run: assistant.Run{
			ID:             "run_wait",
			UserRequestRaw: "deploy release",
			WaitingFor: &assistant.WaitRequest{
				Prompt: "Approve production deploy?",
			},
		},
		Events: []assistant.RunEvent{
			{Summary: "Older event summary"},
		},
	}

	got := buildWatchPreview(record, assistant.Chat{Title: "Deploy chat"})
	if got != "Approve production deploy?" {
		t.Fatalf("buildWatchPreview() = %q, want waiting prompt", got)
	}
}

func TestLoadWatchRunItemsBuildsLatestPreview(t *testing.T) {
	t.Parallel()

	now := time.Unix(100, 0).UTC()
	loader := &fakeWatchRunLoader{
		chats: []assistant.Chat{
			{
				ID:          "chat_2",
				Title:       "Second chat",
				LatestRunID: "run_2",
				UpdatedAt:   now,
			},
			{
				ID:          "chat_1",
				Title:       "First chat",
				LatestRunID: "run_1",
				UpdatedAt:   now.Add(-time.Minute),
			},
		},
		runs: map[string]*store.RunRecord{
			"run_1": {
				Run: assistant.Run{
					ID:             "run_1",
					UserRequestRaw: "write release notes",
				},
				Events: []assistant.RunEvent{{Summary: "Completed and stored the release notes."}},
			},
			"run_2": {
				Run: assistant.Run{
					ID:             "run_2",
					UserRequestRaw: "investigate failing CI",
				},
				Events: []assistant.RunEvent{{Summary: "Waiting for CI rerun confirmation."}},
			},
		},
	}

	items, err := loadWatchRunItems(context.Background(), loader)
	if err != nil {
		t.Fatalf("loadWatchRunItems() error = %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("len(items) = %d, want 2", len(items))
	}
	if items[0].Run.ID != "run_2" {
		t.Fatalf("items[0].Run.ID = %q, want run_2", items[0].Run.ID)
	}
	if items[0].Preview != "Waiting for CI rerun confirmation." {
		t.Fatalf("items[0].Preview = %q", items[0].Preview)
	}
}

func TestWatchPickerModelSelectsRun(t *testing.T) {
	t.Parallel()

	items := []watchRunItem{
		{Chat: assistant.Chat{Title: "One"}, Run: assistant.Run{ID: "run_1", Status: assistant.RunStatusCompleted, Phase: assistant.RunPhaseCompleted}},
		{Chat: assistant.Chat{Title: "Two"}, Run: assistant.Run{ID: "run_2", Status: assistant.RunStatusWaiting, Phase: assistant.RunPhaseWaiting}},
	}

	model := newWatchPickerModel(context.Background(), items)
	updated, _ := model.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	model = updated.(watchPickerModel)

	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyDown})
	model = updated.(watchPickerModel)
	if model.selected != 1 {
		t.Fatalf("selected = %d, want 1", model.selected)
	}

	updated, cmd := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model = updated.(watchPickerModel)
	if !model.confirmed {
		t.Fatal("confirmed = false, want true")
	}
	if cmd == nil {
		t.Fatal("enter cmd = nil, want quit command")
	}
}
