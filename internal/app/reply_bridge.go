package app

import (
	"context"
	"errors"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/siisee11/CodexVirtualAssistant/internal/agentmessage"
	"github.com/siisee11/CodexVirtualAssistant/internal/assistant"
	"github.com/siisee11/CodexVirtualAssistant/internal/store"
)

const (
	replyReactionEmoji       = "👀"
	defaultReplyPollInterval = 5 * time.Second
)

type replyRunService interface {
	ListChats(context.Context) ([]assistant.Chat, error)
	GetChatRecord(context.Context, string) (store.ChatRecord, error)
	ResumeRun(context.Context, string, map[string]string) error
	CreateRun(context.Context, string, int, string) (assistant.Run, error)
}

type replyBridge struct {
	runs       replyRunService
	messenger  agentmessage.Service
	pollEvery  time.Duration
	mu         sync.Mutex
	lastSeenID map[string]string
}

func newReplyBridge(runs replyRunService, messenger agentmessage.Service, pollEvery time.Duration) *replyBridge {
	if runs == nil || messenger == nil {
		return nil
	}
	if pollEvery <= 0 {
		pollEvery = defaultReplyPollInterval
	}
	return &replyBridge{
		runs:       runs,
		messenger:  messenger,
		pollEvery:  pollEvery,
		lastSeenID: map[string]string{},
	}
}

func (b *replyBridge) Run(ctx context.Context) error {
	if b == nil {
		return nil
	}
	ticker := time.NewTicker(b.pollEvery)
	defer ticker.Stop()

	for {
		if err := b.pollOnce(ctx); err != nil && !errors.Is(err, context.Canceled) {
			log.Printf("agent-message reply poll failed: %v", err)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}

func (b *replyBridge) pollOnce(ctx context.Context) error {
	chats, err := b.runs.ListChats(ctx)
	if err != nil {
		return err
	}
	for _, chat := range chats {
		if err := b.pollChat(ctx, chat.ID); err != nil && !errors.Is(err, context.Canceled) {
			log.Printf("agent-message reply poll failed for chat %s: %v", chat.ID, err)
		}
	}
	return nil
}

func (b *replyBridge) pollChat(ctx context.Context, chatID string) error {
	replies, err := b.messenger.ReadReplies(ctx, chatID)
	if err != nil {
		if errors.Is(err, agentmessage.ErrNoMasterRecipient) {
			return nil
		}
		return err
	}
	if len(replies) == 0 {
		return nil
	}

	lastSeen, seenBefore := b.getLastSeen(chatID)
	if !seenBefore {
		b.setLastSeen(chatID, replies[len(replies)-1].ID)
		return nil
	}

	newReplies, ok := messagesAfter(replies, lastSeen)
	if !ok {
		b.setLastSeen(chatID, replies[len(replies)-1].ID)
		return nil
	}
	for _, reply := range newReplies {
		if err := b.messenger.ReactToMessage(ctx, chatID, reply.ID, replyReactionEmoji); err != nil {
			return err
		}
		if err := b.processReply(ctx, chatID, reply.Text); err != nil {
			return err
		}
		b.setLastSeen(chatID, reply.ID)
	}
	return nil
}

func (b *replyBridge) processReply(ctx context.Context, chatID, text string) error {
	record, err := b.runs.GetChatRecord(ctx, chatID)
	if err != nil {
		return err
	}
	latest, ok := latestRunRecord(record)
	if !ok {
		return nil
	}

	replyText := strings.TrimSpace(text)
	if replyText == "" {
		return nil
	}
	if latest.Run.Status == assistant.RunStatusWaiting {
		return b.runs.ResumeRun(ctx, latest.Run.ID, map[string]string{"reply": replyText})
	}
	_, err = b.runs.CreateRun(ctx, replyText, 0, latest.Run.ID)
	return err
}

func latestRunRecord(record store.ChatRecord) (store.RunRecord, bool) {
	if len(record.Runs) == 0 {
		return store.RunRecord{}, false
	}
	latestID := strings.TrimSpace(record.Chat.LatestRunID)
	if latestID != "" {
		for _, run := range record.Runs {
			if run.Run.ID == latestID {
				return run, true
			}
		}
	}
	return record.Runs[len(record.Runs)-1], true
}

func messagesAfter(messages []agentmessage.IncomingMessage, lastSeenID string) ([]agentmessage.IncomingMessage, bool) {
	if len(messages) == 0 {
		return nil, true
	}
	if strings.TrimSpace(lastSeenID) == "" {
		return messages, true
	}
	for idx, message := range messages {
		if message.ID == strings.TrimSpace(lastSeenID) {
			return append([]agentmessage.IncomingMessage(nil), messages[idx+1:]...), true
		}
	}
	return nil, false
}

func (b *replyBridge) getLastSeen(chatID string) (string, bool) {
	b.mu.Lock()
	defer b.mu.Unlock()
	value, ok := b.lastSeenID[strings.TrimSpace(chatID)]
	return value, ok
}

func (b *replyBridge) setLastSeen(chatID, messageID string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.lastSeenID[strings.TrimSpace(chatID)] = strings.TrimSpace(messageID)
}
