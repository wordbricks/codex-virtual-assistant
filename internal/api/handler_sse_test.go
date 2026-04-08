package api

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/siisee11/CodexVirtualAssistant/internal/assistant"
)

func TestWriteSSEEventSplitsLargePayloadAcrossDataLines(t *testing.T) {
	t.Parallel()

	event := assistant.RunEvent{
		ID:        "evt_large",
		RunID:     "run_large",
		Type:      assistant.EventTypeReasoning,
		Summary:   strings.Repeat("x", 40*1024),
		CreatedAt: time.Unix(0, 0).UTC(),
	}

	var buf bytes.Buffer
	if err := writeSSEEvent(&buf, event); err != nil {
		t.Fatalf("writeSSEEvent() error = %v", err)
	}

	output := buf.String()
	if !strings.HasPrefix(output, "event: run_event\n") {
		t.Fatalf("output = %q, want event prefix", output[:min(len(output), 64)])
	}
	if strings.Count(output, "\ndata: ") < 2 {
		t.Fatalf("data line count = %d, want at least 2", strings.Count(output, "\ndata: ")+1)
	}
	if !strings.HasSuffix(output, "\n\n") {
		t.Fatalf("output missing SSE event terminator")
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
