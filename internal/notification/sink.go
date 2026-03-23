package notification

import "sync"

type Sink interface {
	Send(event SessionEvent) error
}

type InAppSink struct {
	mu     sync.Mutex
	events []SessionEvent
	max    int
}

func NewInAppSink(maxEvents int) *InAppSink {
	return &InAppSink{events: make([]SessionEvent, 0), max: maxEvents}
}

func (s *InAppSink) Send(event SessionEvent) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.events = append(s.events, event)
	if len(s.events) > s.max {
		s.events = s.events[len(s.events)-s.max:]
	}
	return nil
}

func (s *InAppSink) Pending() []SessionEvent {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]SessionEvent, len(s.events))
	copy(out, s.events)
	return out
}

func (s *InAppSink) UnreadCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.events)
}

func (s *InAppSink) Clear() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.events = s.events[:0]
}
