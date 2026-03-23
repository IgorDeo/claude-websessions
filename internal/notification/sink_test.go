package notification_test

import (
	"testing"
	"time"

	"github.com/igor-deoalves/websessions/internal/notification"
)

func TestInAppSink_StoresEvents(t *testing.T) {
	sink := notification.NewInAppSink(100)
	event := notification.SessionEvent{SessionID: "s1", Type: notification.EventWaiting, Timestamp: time.Now()}
	if err := sink.Send(event); err != nil {
		t.Fatal(err)
	}
	events := sink.Pending()
	if len(events) != 1 {
		t.Fatalf("expected 1 pending event, got %d", len(events))
	}
}

func TestInAppSink_UnreadCount(t *testing.T) {
	sink := notification.NewInAppSink(100)
	sink.Send(notification.SessionEvent{SessionID: "s1", Type: notification.EventCompleted, Timestamp: time.Now()})
	sink.Send(notification.SessionEvent{SessionID: "s2", Type: notification.EventErrored, Timestamp: time.Now()})
	if sink.UnreadCount() != 2 {
		t.Errorf("expected 2 unread, got %d", sink.UnreadCount())
	}
}
