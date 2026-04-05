package app

import (
	"context"
	"testing"

	"github.com/siisee11/CodexVirtualAssistant/internal/agentmessage"
	"github.com/siisee11/CodexVirtualAssistant/internal/assistant"
	"github.com/siisee11/CodexVirtualAssistant/internal/store"
)

func TestReplyBridgeProcessesLatestReplyOnFirstPoll(t *testing.T) {
	t.Parallel()

	runs := &fakeReplyRuns{
		chats: []assistant.Chat{{ID: "chat_1", LatestRunID: "run_1"}},
		records: map[string]store.ChatRecord{
			"chat_1": {
				Chat: assistant.Chat{ID: "chat_1", LatestRunID: "run_1"},
				Runs: []store.RunRecord{{Run: assistant.Run{ID: "run_1", Status: assistant.RunStatusCompleted}}},
			},
		},
	}
	messenger := &fakeReplyMessenger{
		repliesByChat: map[string][]agentmessage.IncomingMessage{
			"chat_1": {{ID: "msg_1", Sender: "supervisor", Text: "old reply"}},
		},
	}
	bridge := newReplyBridge(runs, messenger, 0)

	if err := bridge.pollOnce(context.Background()); err != nil {
		t.Fatalf("pollOnce() error = %v", err)
	}
	if len(messenger.reactions) != 1 || messenger.reactions[0] != "chat_1:msg_1:👀" {
		t.Fatalf("reactions = %#v, want one reaction for the latest reply", messenger.reactions)
	}
	if len(runs.created) != 1 || runs.created[0].request != "old reply" {
		t.Fatalf("created = %#v, want initial latest reply to be processed", runs.created)
	}
}

func TestReplyBridgeReactsThenCreatesFollowUpForCompletedRun(t *testing.T) {
	t.Parallel()

	runs := &fakeReplyRuns{
		chats: []assistant.Chat{{ID: "chat_1", LatestRunID: "run_1"}},
		records: map[string]store.ChatRecord{
			"chat_1": {
				Chat: assistant.Chat{ID: "chat_1", LatestRunID: "run_1"},
				Runs: []store.RunRecord{{Run: assistant.Run{ID: "run_1", Status: assistant.RunStatusCompleted}}},
			},
		},
	}
	messenger := &fakeReplyMessenger{
		repliesByChat: map[string][]agentmessage.IncomingMessage{
			"chat_1": nil,
		},
	}
	bridge := newReplyBridge(runs, messenger, 0)
	if err := bridge.pollOnce(context.Background()); err != nil {
		t.Fatalf("first pollOnce() error = %v", err)
	}

	messenger.repliesByChat["chat_1"] = []agentmessage.IncomingMessage{
		{ID: "msg_2", Sender: "supervisor", Text: "new follow-up"},
	}
	if err := bridge.pollOnce(context.Background()); err != nil {
		t.Fatalf("second pollOnce() error = %v", err)
	}

	if len(messenger.reactions) != 1 {
		t.Fatalf("reactions = %#v, want one reaction", messenger.reactions)
	}
	if messenger.reactions[0] != "chat_1:msg_2:👀" {
		t.Fatalf("reaction = %q, want reaction before processing", messenger.reactions[0])
	}
	if len(runs.created) != 1 || runs.created[0].parentRunID != "run_1" || runs.created[0].request != "new follow-up" {
		t.Fatalf("created = %#v, want follow-up run", runs.created)
	}
}

func TestReplyBridgeResumesWaitingRun(t *testing.T) {
	t.Parallel()

	runs := &fakeReplyRuns{
		chats: []assistant.Chat{{ID: "chat_1", LatestRunID: "run_wait"}},
		records: map[string]store.ChatRecord{
			"chat_1": {
				Chat: assistant.Chat{ID: "chat_1", LatestRunID: "run_wait"},
				Runs: []store.RunRecord{{Run: assistant.Run{ID: "run_wait", Status: assistant.RunStatusWaiting}}},
			},
		},
	}
	messenger := &fakeReplyMessenger{
		repliesByChat: map[string][]agentmessage.IncomingMessage{
			"chat_1": nil,
		},
	}
	bridge := newReplyBridge(runs, messenger, 0)
	if err := bridge.pollOnce(context.Background()); err != nil {
		t.Fatalf("first pollOnce() error = %v", err)
	}

	messenger.repliesByChat["chat_1"] = []agentmessage.IncomingMessage{
		{ID: "msg_2", Sender: "supervisor", Text: "resume please"},
	}
	if err := bridge.pollOnce(context.Background()); err != nil {
		t.Fatalf("second pollOnce() error = %v", err)
	}

	if len(runs.resumed) != 1 || runs.resumed[0].runID != "run_wait" || runs.resumed[0].input["reply"] != "resume please" {
		t.Fatalf("resumed = %#v, want waiting run resume", runs.resumed)
	}
}

type fakeReplyRuns struct {
	chats   []assistant.Chat
	records map[string]store.ChatRecord
	created []struct {
		request     string
		parentRunID string
	}
	resumed []struct {
		runID string
		input map[string]string
	}
}

func (f *fakeReplyRuns) ListChats(context.Context) ([]assistant.Chat, error) {
	return append([]assistant.Chat(nil), f.chats...), nil
}

func (f *fakeReplyRuns) GetChatRecord(_ context.Context, chatID string) (store.ChatRecord, error) {
	return f.records[chatID], nil
}

func (f *fakeReplyRuns) ResumeRun(_ context.Context, runID string, input map[string]string) error {
	f.resumed = append(f.resumed, struct {
		runID string
		input map[string]string
	}{runID: runID, input: input})
	return nil
}

func (f *fakeReplyRuns) CreateRun(_ context.Context, request string, _ int, parentRunID string) (assistant.Run, error) {
	f.created = append(f.created, struct {
		request     string
		parentRunID string
	}{request: request, parentRunID: parentRunID})
	return assistant.Run{ID: "new_run"}, nil
}

type fakeReplyMessenger struct {
	repliesByChat map[string][]agentmessage.IncomingMessage
	reactions     []string
}

func (*fakeReplyMessenger) WithChatAccount(context.Context, string, func(agentmessage.ChatAccount) error) error {
	return nil
}

func (*fakeReplyMessenger) CatalogPrompt(context.Context, string) (string, error) {
	return "", nil
}

func (*fakeReplyMessenger) SendJSONRender(context.Context, string, string) error {
	return nil
}

func (f *fakeReplyMessenger) ReadReplies(_ context.Context, chatID string) ([]agentmessage.IncomingMessage, error) {
	return append([]agentmessage.IncomingMessage(nil), f.repliesByChat[chatID]...), nil
}

func (f *fakeReplyMessenger) ReactToMessage(_ context.Context, chatID, messageID, emoji string) error {
	f.reactions = append(f.reactions, chatID+":"+messageID+":"+emoji)
	return nil
}
