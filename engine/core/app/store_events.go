package app

import (
	"sync"
	"time"

	"github.com/loongxjin/forksync/engine/core/eventbus"
	"github.com/loongxjin/forksync/engine/core/repo"
	"github.com/loongxjin/forksync/engine/pkg/types"
)

// eventCoalesceWindow is how long to wait before publishing after the last
// mutation. Rapid consecutive Updates (e.g. syncer saving workflow steps)
// reset the timer, so they produce at most one event per window instead of
// one per step. This prevents the frontend from getting a burst of 20+
// repos_changed events in 130ms that cause React re-render jank.
const eventCoalesceWindow = 300 * time.Millisecond

// eventsStore wraps a repo.Store and publishes an EventReposChanged event to
// the bus after every mutating operation (Add/Update/Remove). Reads (List/Get/
// GetByName) pass through untouched. This is the single hook point that turns
// store writes into frontend push notifications, so individual call sites
// (syncer, resolver, handlers) don't need to know about the bus.
//
// Publishers are coalesced: rapid mutations reset a 300ms timer before the
// event is actually published, so the frontend gets at most one repos_changed
// per window regardless of how many store operations happen back-to-back.
type eventsStore struct {
	inner repo.Store
	bus   *eventbus.Bus

	// coalescing timer state
	timerMu sync.Mutex
	timer   *time.Timer
}

// wrapStoreWithEvents returns a repo.Store that publishes EventReposChanged
// after every mutation. Returns the inner store unchanged if bus is nil (e.g.
// in tests that don't care about events).
func wrapStoreWithEvents(inner repo.Store, bus *eventbus.Bus) repo.Store {
	if bus == nil {
		return inner
	}
	return &eventsStore{inner: inner, bus: bus}
}

// coalescedPublish fires an EventReposChanged on the bus, but coalesces rapid
// calls: if called again within eventCoalesceWindow of the last call, the
// timer is reset so only one event is published after the burst settles.
func (s *eventsStore) coalescedPublish() {
	s.timerMu.Lock()
	defer s.timerMu.Unlock()
	if s.timer != nil {
		s.timer.Stop()
	}
	s.timer = time.AfterFunc(eventCoalesceWindow, func() {
		s.bus.Publish(eventbus.Event{Type: eventbus.EventReposChanged})
	})
}

func (s *eventsStore) List() ([]types.Repo, error) { return s.inner.List() }
func (s *eventsStore) Get(id string) (types.Repo, bool) {
	return s.inner.Get(id)
}
func (s *eventsStore) GetByName(name string) (types.Repo, bool) {
	return s.inner.GetByName(name)
}

func (s *eventsStore) Add(r types.Repo) error {
	if err := s.inner.Add(r); err != nil {
		return err
	}
	s.coalescedPublish()
	return nil
}

func (s *eventsStore) Update(r types.Repo) error {
	if err := s.inner.Update(r); err != nil {
		return err
	}
	s.coalescedPublish()
	return nil
}

func (s *eventsStore) Remove(id string) error {
	if err := s.inner.Remove(id); err != nil {
		return err
	}
	s.coalescedPublish()
	return nil
}
