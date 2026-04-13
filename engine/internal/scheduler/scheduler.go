package scheduler

import (
	"context"
	"sync"
	"time"

	"github.com/loongxjin/forksync/engine/internal/config"
	"github.com/loongxjin/forksync/engine/internal/notify"
	syncpkg "github.com/loongxjin/forksync/engine/internal/sync"
)

// DefaultInterval is the default sync interval.
const DefaultInterval = 30 * time.Minute

// Scheduler periodically syncs managed repositories.
type Scheduler struct {
	syncer   *syncpkg.Syncer
	notifier *notify.Notifier
	interval time.Duration
	stopCh   chan struct{}
	mu       sync.Mutex
	running  bool
}

// NewScheduler creates a new Scheduler.
func NewScheduler(syncer *syncpkg.Syncer, notifier *notify.Notifier, cfg *config.Config) *Scheduler {
	interval := DefaultInterval
	if cfg != nil && cfg.Sync.DefaultInterval != "" {
		if d, err := time.ParseDuration(cfg.Sync.DefaultInterval); err == nil {
			interval = d
		}
	}

	return &Scheduler{
		syncer:   syncer,
		notifier: notifier,
		interval: interval,
		stopCh:   make(chan struct{}),
	}
}

// Start begins the scheduler loop.
func (s *Scheduler) Start(ctx context.Context) {
	s.mu.Lock()
	if s.running {
		s.mu.Unlock()
		return
	}
	s.running = true
	s.mu.Unlock()

	go func() {
		defer func() {
			s.mu.Lock()
			s.running = false
			s.mu.Unlock()
		}()

		ticker := time.NewTicker(s.interval)
		defer ticker.Stop()

		// Run once immediately on start
		s.runSync(ctx)

		for {
			select {
			case <-ctx.Done():
				return
			case <-s.stopCh:
				return
			case <-ticker.C:
				s.runSync(ctx)
			}
		}
	}()
}

// Stop stops the scheduler.
func (s *Scheduler) Stop() {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.running {
		close(s.stopCh)
		s.stopCh = make(chan struct{})
	}
}

// IsRunning returns whether the scheduler is currently running.
func (s *Scheduler) IsRunning() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.running
}

func (s *Scheduler) runSync(ctx context.Context) {
	results := s.syncer.SyncAll(ctx)
	for _, r := range results {
		if s.notifier == nil {
			continue
		}
		switch r.Status {
		case "synced":
			if r.CommitsPulled > 0 {
				s.notifier.NotifySyncSuccess(r.RepoName, r.CommitsPulled)
			}
		case "conflict":
			s.notifier.NotifyConflict(r.RepoName, len(r.ConflictFiles))
		case "error":
			s.notifier.NotifyError(r.RepoName, r.ErrorMessage)
		}
	}
}
