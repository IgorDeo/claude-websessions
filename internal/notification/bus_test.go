package notification_test

import (
	"sync"
	"testing"
	"time"

	"github.com/IgorDeo/claude-websessions/internal/notification"
)

func TestBus_SubscribeAndPublish(t *testing.T) {
	bus := notification.NewBus()
	var received []notification.SessionEvent
	var mu sync.Mutex
	bus.Subscribe(func(e notification.SessionEvent) {
		mu.Lock()
		received = append(received, e)
		mu.Unlock()
	})
	bus.Publish(notification.SessionEvent{SessionID: "s1", Type: notification.EventCompleted, Timestamp: time.Now()})
	time.Sleep(50 * time.Millisecond)
	mu.Lock()
	defer mu.Unlock()
	if len(received) != 1 {
		t.Fatalf("expected 1 event, got %d", len(received))
	}
	if received[0].SessionID != "s1" {
		t.Errorf("expected session s1, got %s", received[0].SessionID)
	}
}

func TestBus_MultipleSubscribers(t *testing.T) {
	bus := notification.NewBus()
	var count1, count2 int
	var mu sync.Mutex
	bus.Subscribe(func(e notification.SessionEvent) { mu.Lock(); count1++; mu.Unlock() })
	bus.Subscribe(func(e notification.SessionEvent) { mu.Lock(); count2++; mu.Unlock() })
	bus.Publish(notification.SessionEvent{SessionID: "s1", Type: notification.EventErrored, Timestamp: time.Now()})
	time.Sleep(50 * time.Millisecond)
	mu.Lock()
	defer mu.Unlock()
	if count1 != 1 || count2 != 1 {
		t.Errorf("expected both subscribers to receive 1 event, got %d and %d", count1, count2)
	}
}
