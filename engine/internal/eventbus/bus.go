// Package eventbus provides a lightweight in-process pub/sub bus used to
// notify the frontend (via the /stream/events WebSocket) that repo or history
// state has changed, so the renderer can stop polling.
//
// The bus is intentionally minimal: buffered channels per subscriber, a mutex
// over the subscriber set, and non-blocking publishes (a slow subscriber's
// buffer drops the oldest event rather than blocking the publisher). This is a
// local, single-process bus; there is no network/serialization here.
package eventbus

import (
	"sync"
	"sync/atomic"
)

// EventType identifies what kind of state changed.
type EventType string

const (
	// EventReposChanged is published whenever a repo is added, updated, or
	// removed (status transitions, workflow progress, etc.).
	EventReposChanged EventType = "repos_changed"
	// EventHistoryChanged is published whenever a history record is inserted,
	// its summary updated, or the history is cleaned up.
	EventHistoryChanged EventType = "history_changed"
)

// Event is a single published notification. Payload is left opaque (the
// subscriber re-reads the current state from its store) to keep the bus small
// and avoid duplicating store serialization logic.
type Event struct {
	Type EventType
}

// subscriberBufferSize is the per-subscriber channel buffer. The publisher
// drops the oldest pending event when full (non-blocking send) so a slow
// subscriber never blocks state changes.
const subscriberBufferSize = 32

// Bus is a concurrency-safe in-process event bus.
type Bus struct {
	mu          sync.RWMutex
	subscribers map[chan Event]struct{}
	closed      atomic.Bool
}

// New returns an empty, ready-to-use bus.
func New() *Bus {
	return &Bus{subscribers: make(map[chan Event]struct{})}
}

// Subscribe returns a channel that receives every published Event. The caller
// must call the returned cancel function to unsubscribe and release the buffer
// when done (e.g. when a WebSocket disconnects). Returns nil + a no-op cancel
// if the bus is already closed.
func (b *Bus) Subscribe() (<-chan Event, func()) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.closed.Load() {
		// Closed bus: return a nil channel (never receives) + no-op cancel.
		return nil, func() {}
	}
	ch := make(chan Event, subscriberBufferSize)
	b.subscribers[ch] = struct{}{}
	return ch, func() {
		b.mu.Lock()
		defer b.mu.Unlock()
		if _, ok := b.subscribers[ch]; ok {
			delete(b.subscribers, ch)
			close(ch)
		}
	}
}

// Publish broadcasts ev to all subscribers. Non-blocking: if a subscriber's
// buffer is full, the oldest pending event is dropped to keep state-change
// propagation from stalling behind a slow consumer.
func (b *Bus) Publish(ev Event) {
	if b.closed.Load() {
		return
	}
	b.mu.RLock()
	defer b.mu.RUnlock()
	for ch := range b.subscribers {
		select {
		case ch <- ev:
		default:
			// Buffer full: drop the oldest so the newest has room. A dropped
			// event is acceptable because subscribers re-read current state on
			// every event they DO receive — they only need a poke, not every
			// intermediate transition.
			select {
			case <-ch:
			default:
			}
			select {
			case ch <- ev:
			default:
			}
		}
	}
}

// Close shuts down the bus: all subscriber channels are closed and future
// Subscribe/Publish calls are no-ops. Safe to call multiple times.
func (b *Bus) Close() {
	if !b.closed.CompareAndSwap(false, true) {
		return
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	for ch := range b.subscribers {
		close(ch)
		delete(b.subscribers, ch)
	}
}
