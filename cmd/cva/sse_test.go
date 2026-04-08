package main

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/siisee11/CodexVirtualAssistant/internal/assistant"
)

func TestStreamSSEHandlesLargeEventLine(t *testing.T) {
	t.Parallel()

	summary := strings.Repeat("x", 128*1024)
	event := assistant.RunEvent{
		ID:        "evt_large",
		RunID:     "run_large",
		Type:      assistant.EventTypeReasoning,
		Summary:   summary,
		CreatedAt: time.Unix(0, 0).UTC(),
	}

	stream := fmt.Sprintf(
		"event: run_event\ndata: {\"id\":\"%s\",\"run_id\":\"%s\",\"type\":\"%s\",\"summary\":\"%s\",\"created_at\":\"%s\"}\n\n",
		event.ID,
		event.RunID,
		event.Type,
		summary,
		event.CreatedAt.Format(time.RFC3339),
	)

	var got assistant.RunEvent
	if err := streamSSE(strings.NewReader(stream), func(ev assistant.RunEvent) bool {
		got = ev
		return false
	}); err != nil {
		t.Fatalf("streamSSE() error = %v", err)
	}

	if got.ID != event.ID {
		t.Fatalf("event id = %q, want %q", got.ID, event.ID)
	}
	if got.Summary != summary {
		t.Fatalf("summary length = %d, want %d", len(got.Summary), len(summary))
	}
}
