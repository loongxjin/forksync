package app

import (
	"github.com/loongxjin/forksync/engine/internal/eventbus"
	"github.com/loongxjin/forksync/engine/internal/repo"
	"github.com/loongxjin/forksync/engine/pkg/types"
)

// eventsStore wraps a repo.Store and publishes an EventReposChanged event to
// the bus after every mutating operation (Add/Update/Remove). Reads (List/Get/
// GetByName) pass through untouched. This is the single hook point that turns
// store writes into frontend push notifications, so individual call sites
// (syncer, resolver, handlers) don't need to know about the bus.
type eventsStore struct {
	inner repo.Store
	bus   *eventbus.Bus
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
	s.bus.Publish(eventbus.Event{Type: eventbus.EventReposChanged})
	return nil
}

func (s *eventsStore) Update(r types.Repo) error {
	if err := s.inner.Update(r); err != nil {
		return err
	}
	s.bus.Publish(eventbus.Event{Type: eventbus.EventReposChanged})
	return nil
}

func (s *eventsStore) Remove(id string) error {
	if err := s.inner.Remove(id); err != nil {
		return err
	}
	s.bus.Publish(eventbus.Event{Type: eventbus.EventReposChanged})
	return nil
}
