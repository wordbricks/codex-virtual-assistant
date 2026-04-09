package main

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/siisee11/CodexVirtualAssistant/internal/assistant"
)

type fakeRunTUIClient struct {
	createResp *createRunResponse
	createErr  error
	resumeErr  error
	streamErr  error
	streamData string

	createCalls []createRunCall
	resumeCalls []resumeRunCall
	streamCalls []string
}

type createRunCall struct {
	request     string
	maxAttempts int
	parentRunID string
}

type resumeRunCall struct {
	runID string
	input map[string]string
}

func (f *fakeRunTUIClient) CreateRun(_ context.Context, request string, maxAttempts int, parentRunID string) (*createRunResponse, error) {
	f.createCalls = append(f.createCalls, createRunCall{
		request:     request,
		maxAttempts: maxAttempts,
		parentRunID: parentRunID,
	})
	if f.createErr != nil {
		return nil, f.createErr
	}
	if f.createResp == nil {
		return nil, errors.New("missing create response")
	}
	return f.createResp, nil
}

func (f *fakeRunTUIClient) ResumeRun(_ context.Context, runID string, input map[string]string) error {
	f.resumeCalls = append(f.resumeCalls, resumeRunCall{
		runID: runID,
		input: input,
	})
	return f.resumeErr
}

func (f *fakeRunTUIClient) StreamEvents(_ context.Context, runID string) (io.ReadCloser, error) {
	f.streamCalls = append(f.streamCalls, runID)
	if f.streamErr != nil {
		return nil, f.streamErr
	}
	return io.NopCloser(strings.NewReader(f.streamData)), nil
}

func TestCurrentComposerMode(t *testing.T) {
	t.Parallel()

	run := assistant.Run{
		ID:        "run_1",
		ChatID:    "chat_1",
		Status:    assistant.RunStatusGenerating,
		Phase:     assistant.RunPhaseGenerating,
		CreatedAt: time.Unix(0, 0).UTC(),
	}
	m := newRunTUIModel(context.Background(), nil, run)

	if got := m.currentComposerMode(); got != composerModeLocked {
		t.Fatalf("currentComposerMode() = %v, want %v", got, composerModeLocked)
	}

	m.phase = assistant.RunPhaseWaiting
	if got := m.currentComposerMode(); got != composerModeWaiting {
		t.Fatalf("currentComposerMode(waiting) = %v, want %v", got, composerModeWaiting)
	}

	m.phase = assistant.RunPhaseCompleted
	m.status = assistant.RunStatusCompleted
	if got := m.currentComposerMode(); got != composerModeFollowUp {
		t.Fatalf("currentComposerMode(completed) = %v, want %v", got, composerModeFollowUp)
	}

	m.submitting = true
	if got := m.currentComposerMode(); got != composerModeSubmitting {
		t.Fatalf("currentComposerMode(submitting) = %v, want %v", got, composerModeSubmitting)
	}
}

func TestParseResumeInputFromComposer(t *testing.T) {
	t.Parallel()

	gotKV := parseResumeInputFromComposer("ticket=123 owner=alice")
	if len(gotKV) != 2 || gotKV["ticket"] != "123" || gotKV["owner"] != "alice" {
		t.Fatalf("parseResumeInputFromComposer(kv) = %#v, want key=value map", gotKV)
	}

	gotFreeText := parseResumeInputFromComposer("please proceed with prod deploy")
	if len(gotFreeText) != 1 || gotFreeText["response"] != "please proceed with prod deploy" {
		t.Fatalf("parseResumeInputFromComposer(free text) = %#v, want response map", gotFreeText)
	}
}

func TestHandleRunEventUpdatesWaitingAndTerminalState(t *testing.T) {
	t.Parallel()

	run := assistant.Run{
		ID:        "run_1",
		ChatID:    "chat_1",
		Status:    assistant.RunStatusQueued,
		Phase:     assistant.RunPhaseQueued,
		CreatedAt: time.Unix(0, 0).UTC(),
	}
	m := newRunTUIModel(context.Background(), nil, run)
	m.followLogs = false

	waitEvent := assistant.RunEvent{
		ID:        "event_wait",
		RunID:     run.ID,
		Type:      assistant.EventTypeWaiting,
		Phase:     assistant.RunPhaseWaiting,
		Summary:   "Approval required before continuing.",
		CreatedAt: time.Unix(10, 0).UTC(),
	}
	m.handleRunEvent(waitEvent)
	if m.status != assistant.RunStatusWaiting {
		t.Fatalf("status after waiting event = %q, want %q", m.status, assistant.RunStatusWaiting)
	}
	if m.phase != assistant.RunPhaseWaiting {
		t.Fatalf("phase after waiting event = %q, want %q", m.phase, assistant.RunPhaseWaiting)
	}
	if m.waitingSummary == "" {
		t.Fatalf("waitingSummary empty, want event summary")
	}

	doneEvent := assistant.RunEvent{
		ID:        "event_done",
		RunID:     run.ID,
		Type:      assistant.EventTypePhaseChanged,
		Phase:     assistant.RunPhaseCompleted,
		Summary:   "Run completed.",
		CreatedAt: time.Unix(20, 0).UTC(),
	}
	m.handleRunEvent(doneEvent)
	if m.status != assistant.RunStatusCompleted {
		t.Fatalf("status after completed event = %q, want %q", m.status, assistant.RunStatusCompleted)
	}
	if m.waitingSummary != "" {
		t.Fatalf("waitingSummary = %q, want empty after terminal phase", m.waitingSummary)
	}
	if !m.followLogs {
		t.Fatalf("followLogs = false, want true after terminal phase")
	}
}

func TestSubmitComposerCmdWaitingResumesRun(t *testing.T) {
	t.Parallel()

	client := &fakeRunTUIClient{}
	run := assistant.Run{
		ID:        "run_waiting",
		ChatID:    "chat_1",
		Status:    assistant.RunStatusWaiting,
		Phase:     assistant.RunPhaseWaiting,
		CreatedAt: time.Unix(0, 0).UTC(),
	}

	msg := submitComposerCmd(context.Background(), client, composerModeWaiting, run, "approved=yes", 7)()
	done, ok := msg.(tuiComposerSubmitDoneMsg)
	if !ok {
		t.Fatalf("submitComposerCmd(waiting) returned %T, want tuiComposerSubmitDoneMsg", msg)
	}
	if done.err != nil {
		t.Fatalf("submitComposerCmd(waiting) err = %v", done.err)
	}
	if len(client.resumeCalls) != 1 {
		t.Fatalf("resume calls = %d, want 1", len(client.resumeCalls))
	}
	if client.resumeCalls[0].runID != run.ID {
		t.Fatalf("resume runID = %q, want %q", client.resumeCalls[0].runID, run.ID)
	}
	if client.resumeCalls[0].input["approved"] != "yes" {
		t.Fatalf("resume input = %#v, want approved=yes", client.resumeCalls[0].input)
	}
	if len(client.streamCalls) != 1 || client.streamCalls[0] != run.ID {
		t.Fatalf("stream calls = %#v, want [%q]", client.streamCalls, run.ID)
	}
	if done.streamMsgs == nil {
		t.Fatalf("streamMsgs is nil, want non-nil")
	}
	if done.nextRun == nil || done.nextRun.ID != run.ID {
		t.Fatalf("nextRun = %#v, want resumed run", done.nextRun)
	}
}

func TestSubmitComposerCmdFollowUpCreatesChildRun(t *testing.T) {
	t.Parallel()

	client := &fakeRunTUIClient{
		createResp: &createRunResponse{
			Run: assistant.Run{
				ID:        "run_child",
				ChatID:    "chat_1",
				Status:    assistant.RunStatusQueued,
				Phase:     assistant.RunPhaseQueued,
				CreatedAt: time.Unix(0, 0).UTC(),
			},
		},
	}
	parent := assistant.Run{
		ID:        "run_parent",
		ChatID:    "chat_1",
		Status:    assistant.RunStatusCompleted,
		Phase:     assistant.RunPhaseCompleted,
		CreatedAt: time.Unix(0, 0).UTC(),
	}

	msg := submitComposerCmd(context.Background(), client, composerModeFollowUp, parent, "next task", 11)()
	done, ok := msg.(tuiComposerSubmitDoneMsg)
	if !ok {
		t.Fatalf("submitComposerCmd(follow-up) returned %T, want tuiComposerSubmitDoneMsg", msg)
	}
	if done.err != nil {
		t.Fatalf("submitComposerCmd(follow-up) err = %v", done.err)
	}
	if len(client.createCalls) != 1 {
		t.Fatalf("create calls = %d, want 1", len(client.createCalls))
	}
	call := client.createCalls[0]
	if call.request != "next task" {
		t.Fatalf("create request = %q, want %q", call.request, "next task")
	}
	if call.parentRunID != parent.ID {
		t.Fatalf("create parentRunID = %q, want %q", call.parentRunID, parent.ID)
	}
	if len(client.streamCalls) != 1 || client.streamCalls[0] != "run_child" {
		t.Fatalf("stream calls = %#v, want [run_child]", client.streamCalls)
	}
	if done.nextRun == nil || done.nextRun.ID != "run_child" {
		t.Fatalf("nextRun = %#v, want run_child", done.nextRun)
	}
	if done.streamMsgs == nil {
		t.Fatalf("streamMsgs is nil, want non-nil")
	}
}
