package notification

import (
	"sync"
	"time"
)

type EventType string

const (
	EventCompleted EventType = "completed"
	EventErrored   EventType = "errored"
	EventWaiting   EventType = "waiting"
)

type SessionEvent struct {
	SessionID string
	Type      EventType
	Timestamp time.Time
	Message   string
}

type SubscriberFunc func(SessionEvent)

type Bus struct {
	mu          sync.RWMutex
	subscribers []SubscriberFunc
}

func NewBus() *Bus { return &Bus{} }

func (b *Bus) Subscribe(fn SubscriberFunc) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.subscribers = append(b.subscribers, fn)
}

func (b *Bus) Publish(e SessionEvent) {
	b.mu.RLock()
	subs := make([]SubscriberFunc, len(b.subscribers))
	copy(subs, b.subscribers)
	b.mu.RUnlock()
	for _, fn := range subs {
		go fn(e)
	}
}
