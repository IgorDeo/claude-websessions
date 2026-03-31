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

	// Agent Teams events
	EventTeamDiscovered EventType = "team_discovered"
	EventTeamMemberJoin EventType = "team_member_join"
	EventTaskCreated    EventType = "task_created"
	EventTaskUpdated    EventType = "task_updated"
	EventTaskCompleted  EventType = "task_completed"
	EventTeamMessage    EventType = "team_message"
)

type SessionEvent struct {
	SessionID string
	Type      EventType
	Timestamp time.Time
	Message   string
	TeamName  string            // non-empty for team-related events
	Metadata  map[string]string // additional key-value data (task_id, from, to, etc.)
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
