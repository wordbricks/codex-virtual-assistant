package safety

import (
	"math"
	"testing"
	"time"

	"github.com/siisee11/CodexVirtualAssistant/internal/assistant"
)

func TestBrowserActionRecordFromWebStepClassifiesMutationAndFingerprint(t *testing.T) {
	t.Parallel()

	step := assistant.WebStep{
		Title:        "Submitted reply",
		URL:          "https://example.com/posts/123?ref=feed",
		Summary:      "Posted a reply to the public thread.",
		ActionName:   "reply",
		ActionTarget: "thread-123",
		ActionValue:  "Thanks for sharing this update",
		OccurredAt:   time.Date(2026, time.April, 19, 10, 0, 0, 0, time.UTC),
	}

	record, ok := BrowserActionRecordFromWebStep("community-growth", step)
	if !ok {
		t.Fatal("BrowserActionRecordFromWebStep() ok = false, want true")
	}
	if record.ProjectSlug != "community-growth" {
		t.Fatalf("ProjectSlug = %q, want community-growth", record.ProjectSlug)
	}
	if record.ActionType != assistant.BrowserActionTypeReply {
		t.Fatalf("ActionType = %q, want %q", record.ActionType, assistant.BrowserActionTypeReply)
	}
	if !record.AccountStateChanged {
		t.Fatal("AccountStateChanged = false, want true")
	}
	if record.SourceContext != "example.com/posts/123" {
		t.Fatalf("SourceContext = %q, want example.com/posts/123", record.SourceContext)
	}
	if record.TextFingerprint == "" {
		t.Fatal("TextFingerprint is empty, want fingerprint for reply payload")
	}
}

func TestComputeRecentActivityMetricsCalculatesWindowScores(t *testing.T) {
	t.Parallel()

	windowStart := time.Date(2026, time.April, 18, 12, 0, 0, 0, time.UTC)
	windowEnd := windowStart.Add(24 * time.Hour)

	records := []assistant.BrowserActionRecord{
		{
			ActionType:          assistant.BrowserActionTypeReply,
			SourceContext:       "example.com/a",
			AccountStateChanged: true,
			TextFingerprint:     "fp-1",
			OccurredAt:          windowStart.Add(1 * time.Hour),
		},
		{
			ActionType:          assistant.BrowserActionTypeReply,
			SourceContext:       "example.com/a",
			AccountStateChanged: true,
			TextFingerprint:     "fp-1",
			OccurredAt:          windowStart.Add(2 * time.Hour),
		},
		{
			ActionType:          assistant.BrowserActionTypeReply,
			SourceContext:       "example.com/b",
			AccountStateChanged: true,
			TextFingerprint:     "fp-2",
			OccurredAt:          windowStart.Add(3 * time.Hour),
		},
		{
			ActionType:          assistant.BrowserActionTypeNavigate,
			SourceContext:       "example.com/a",
			AccountStateChanged: false,
			OccurredAt:          windowStart.Add(4 * time.Hour),
		},
		{
			ActionType:          assistant.BrowserActionTypeSubmit,
			SourceContext:       "example.com/a",
			AccountStateChanged: true,
			TextFingerprint:     "fp-3",
			OccurredAt:          windowStart.Add(5 * time.Hour),
		},
		{
			ActionType:          assistant.BrowserActionTypeSubmit,
			SourceContext:       "example.com/outside",
			AccountStateChanged: true,
			OccurredAt:          windowEnd.Add(1 * time.Minute),
		},
	}

	metrics := ComputeRecentActivityMetrics(records, windowStart, windowEnd)
	if metrics.TotalActionCount != 5 {
		t.Fatalf("TotalActionCount = %d, want 5", metrics.TotalActionCount)
	}
	if metrics.MutatingActionCount != 4 {
		t.Fatalf("MutatingActionCount = %d, want 4", metrics.MutatingActionCount)
	}
	if metrics.ReplyActionCount != 3 {
		t.Fatalf("ReplyActionCount = %d, want 3", metrics.ReplyActionCount)
	}
	if !approximatelyEqual(metrics.RecentMutationDensity, 4.0/24.0) {
		t.Fatalf("RecentMutationDensity = %f, want %f", metrics.RecentMutationDensity, 4.0/24.0)
	}
	if !approximatelyEqual(metrics.SourcePathConcentration, 4.0/5.0) {
		t.Fatalf("SourcePathConcentration = %f, want %f", metrics.SourcePathConcentration, 4.0/5.0)
	}
	if !approximatelyEqual(metrics.RepeatedActionSequenceScore, 0.25) {
		t.Fatalf("RepeatedActionSequenceScore = %f, want 0.25", metrics.RepeatedActionSequenceScore)
	}
	if !approximatelyEqual(metrics.TextReuseRiskScore, 1.0/4.0) {
		t.Fatalf("TextReuseRiskScore = %f, want %f", metrics.TextReuseRiskScore, 1.0/4.0)
	}
}

func approximatelyEqual(got, want float64) bool {
	return math.Abs(got-want) < 1e-9
}
