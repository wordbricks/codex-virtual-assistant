package api

import (
	"context"
	"log"
	"sync"

	"github.com/siisee11/CodexVirtualAssistant/internal/assistant"
	"github.com/siisee11/CodexVirtualAssistant/internal/store"
)

type HookName string

const (
	HookOnRunStarted   HookName = "onRunStarted"
	HookOnPhaseChanged HookName = "onPhaseChanged"
	HookOnWaitEntered  HookName = "onWaitEntered"
	HookOnRunCompleted HookName = "onRunCompleted"
	HookOnRunExhausted HookName = "onRunExhausted"
)

type HookPayload struct {
	Name   HookName           `json:"name"`
	Event  assistant.RunEvent `json:"event"`
	Record *store.RunRecord   `json:"record,omitempty"`
}

type HookFunc func(context.Context, HookPayload) error

type runRecordLoader interface {
	GetRunRecord(context.Context, string) (store.RunRecord, error)
}

type registeredHook struct {
	id   int
	fn   HookFunc
	name HookName
}

type EventBroker struct {
	mu          sync.RWMutex
	subscribers map[string]map[chan assistant.RunEvent]struct{}
	hooks       map[HookName][]registeredHook
	nextHookID  int
	loader      runRecordLoader
}

func NewEventBroker() *EventBroker {
	return &EventBroker{
		subscribers: map[string]map[chan assistant.RunEvent]struct{}{},
		hooks:       map[HookName][]registeredHook{},
	}
}

func (b *EventBroker) SetSnapshotLoader(loader runRecordLoader) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.loader = loader
}

func (b *EventBroker) RegisterHook(name HookName, hook HookFunc) func() {
	if hook == nil {
		return func() {}
	}

	b.mu.Lock()
	id := b.nextHookID
	b.nextHookID++
	b.hooks[name] = append(b.hooks[name], registeredHook{
		id:   id,
		name: name,
		fn:   hook,
	})
	b.mu.Unlock()

	return func() {
		b.mu.Lock()
		defer b.mu.Unlock()

		registered := b.hooks[name]
		for idx := range registered {
			if registered[idx].id != id {
				continue
			}
			b.hooks[name] = append(registered[:idx], registered[idx+1:]...)
			if len(b.hooks[name]) == 0 {
				delete(b.hooks, name)
			}
			return
		}
	}
}

func (b *EventBroker) Publish(ctx context.Context, event assistant.RunEvent) error {
	b.mu.RLock()
	subscribers := make([]chan assistant.RunEvent, 0, len(b.subscribers[event.RunID]))
	for ch := range b.subscribers[event.RunID] {
		subscribers = append(subscribers, ch)
	}
	loader := b.loader
	b.mu.RUnlock()

	for _, ch := range subscribers {
		select {
		case ch <- event:
		default:
		}
	}

	payloads := b.hookPayloads(ctx, loader, event)
	for _, payload := range payloads {
		b.dispatchHooks(context.WithoutCancel(ctx), payload)
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

func (b *EventBroker) hookPayloads(ctx context.Context, loader runRecordLoader, event assistant.RunEvent) []HookPayload {
	names := []HookName{}
	switch event.Type {
	case assistant.EventTypeRunCreated:
		names = append(names, HookOnRunStarted)
	case assistant.EventTypePhaseChanged:
		names = append(names, HookOnPhaseChanged)
	case assistant.EventTypeWaiting:
		names = append(names, HookOnWaitEntered)
	}

	var snapshot *store.RunRecord
	if loader != nil && b.needsRunSnapshot(event) {
		record, err := loader.GetRunRecord(ctx, event.RunID)
		if err != nil {
			log.Printf("event hook snapshot load failed for run %s: %v", event.RunID, err)
		} else {
			snapshot = &record
			switch record.Run.Status {
			case assistant.RunStatusCompleted:
				names = append(names, HookOnRunCompleted)
			case assistant.RunStatusExhausted:
				names = append(names, HookOnRunExhausted)
			}
		}
	}

	if snapshot == nil && event.Type == assistant.EventTypePhaseChanged && event.Phase == assistant.RunPhaseCompleted {
		names = append(names, HookOnRunCompleted)
	}

	payloads := make([]HookPayload, 0, len(names))
	for _, name := range names {
		payloads = append(payloads, HookPayload{
			Name:   name,
			Event:  event,
			Record: snapshot,
		})
	}
	return payloads
}

func (b *EventBroker) needsRunSnapshot(event assistant.RunEvent) bool {
	return event.Type == assistant.EventTypePhaseChanged || event.Type == assistant.EventTypeWaiting
}

func (b *EventBroker) dispatchHooks(ctx context.Context, payload HookPayload) {
	b.mu.RLock()
	registered := append([]registeredHook(nil), b.hooks[payload.Name]...)
	b.mu.RUnlock()

	for _, hook := range registered {
		go func(fn HookFunc, payload HookPayload) {
			defer func() {
				if recovered := recover(); recovered != nil {
					log.Printf("event hook %s panicked for run %s: %v", payload.Name, payload.Event.RunID, recovered)
				}
			}()
			if err := fn(ctx, payload); err != nil {
				log.Printf("event hook %s failed for run %s: %v", payload.Name, payload.Event.RunID, err)
			}
		}(hook.fn, payload)
	}
}
