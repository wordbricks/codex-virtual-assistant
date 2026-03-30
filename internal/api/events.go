package api

import (
	"context"
	"sync"

	"github.com/siisee11/CodexVirtualAssistant/internal/assistant"
)

type EventBroker struct {
	mu          sync.RWMutex
	subscribers map[string]map[chan assistant.RunEvent]struct{}
}

func NewEventBroker() *EventBroker {
	return &EventBroker{
		subscribers: map[string]map[chan assistant.RunEvent]struct{}{},
	}
}

func (b *EventBroker) Publish(_ context.Context, event assistant.RunEvent) error {
	b.mu.RLock()
	defer b.mu.RUnlock()

	for ch := range b.subscribers[event.RunID] {
		select {
		case ch <- event:
		default:
		}
	}
	return nil
}

func (b *EventBroker) Subscribe(runID string) (<-chan assistant.RunEvent, func()) {
	ch := make(chan assistant.RunEvent, 32)

	b.mu.Lock()
	if b.subscribers[runID] == nil {
		b.subscribers[runID] = map[chan assistant.RunEvent]struct{}{}
	}
	b.subscribers[runID][ch] = struct{}{}
	b.mu.Unlock()

	return ch, func() {
		b.mu.Lock()
		defer b.mu.Unlock()
		if set, ok := b.subscribers[runID]; ok {
			delete(set, ch)
			if len(set) == 0 {
				delete(b.subscribers, runID)
			}
		}
		close(ch)
	}
}
