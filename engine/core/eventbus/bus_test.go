package eventbus

import (
	"testing"
	"time"
)

func TestBusSubscribePublish(t *testing.T) {
	b := New()
	ch, cancel := b.Subscribe()
	defer cancel()

	b.Publish(Event{Type: EventReposChanged})

	select {
	case ev := <-ch:
		if ev.Type != EventReposChanged {
			t.Fatalf("got %q, want repos_changed", ev.Type)
		}
	case <-time.After(100 * time.Millisecond):
		t.Fatal("did not receive published event")
	}
}

func TestBusMultipleSubscribers(t *testing.T) {
	b := New()
	ch1, cancel1 := b.Subscribe()
	defer cancel1()
	ch2, cancel2 := b.Subscribe()
	defer cancel2()

	b.Publish(Event{Type: EventHistoryChanged})

	for i, ch := range []<-chan Event{ch1, ch2} {
		select {
		case ev := <-ch:
			if ev.Type != EventHistoryChanged {
				t.Fatalf("subscriber %d got %q", i, ev.Type)
			}
		case <-time.After(100 * time.Millisecond):
			t.Fatalf("subscriber %d did not receive", i)
		}
	}
}

func TestBusUnsubscribeStopsDelivery(t *testing.T) {
	b := New()
	ch, cancel := b.Subscribe()
	cancel()

	b.Publish(Event{Type: EventReposChanged})

	// After cancel, the channel should either be closed (read returns zero
	// value) or empty — but it must NOT receive the event published after
	// unsubscribe.
	select {
	case ev, ok := <-ch:
		if ok && ev.Type != "" {
			t.Fatalf("received event after unsubscribe: %+v", ev)
		}
	case <-time.After(50 * time.Millisecond):
		// empty + open is also acceptable (cancel may race), but no delivery.
	}
}

func TestBusCloseStopsPublish(t *testing.T) {
	b := New()
	ch, cancel := b.Subscribe()
	_ = cancel // Close supersedes per-subscriber cancel.

	b.Close()
	b.Publish(Event{Type: EventReposChanged})

	// After Close the subscriber channel is closed; a read returns the zero
	// value immediately.
	ev, ok := <-ch
	if ok {
		t.Fatalf("expected closed channel, got event %+v", ev)
	}
}

func TestBusCloseIdempotent(t *testing.T) {
	b := New()
	b.Close()
	b.Close() // must not panic
}

func TestBusSlowSubscriberDoesNotBlockPublisher(t *testing.T) {
	b := New()
	// A subscriber whose buffer we never drain.
	_, cancel := b.Subscribe()
	defer cancel()

	// Publish more than the buffer; Publish must not block.
	for i := 0; i < subscriberBufferSize+10; i++ {
		b.Publish(Event{Type: EventReposChanged})
	}
	// If we got here, Publish never blocked.
}
