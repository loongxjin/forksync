// Package summarizer provides async AI-generated summaries for sync history.
// It uses a single-worker queue model to process summarization tasks.
package summarizer

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/loongxjin/forksync/engine/internal/agent"
	"github.com/loongxjin/forksync/engine/internal/config"
	"github.com/loongxjin/forksync/engine/internal/history"
	"github.com/loongxjin/forksync/engine/pkg/types"
)

// Task represents a summarization job.
type Task struct {
	HistoryID int64
	RepoName  string
	Commits   []CommitInfo
	Language  string // "zh" or "en"
}

// Summarizer manages async summarization of sync history records.
type Summarizer struct {
	mu            sync.Mutex
	queue         chan Task
	historyStore  *history.Store
	agentRegistry *agent.Registry
	executor      *Executor
	config        *config.Config
	logger        *log.Logger
	closeOnce     sync.Once
	closed        bool
	wg            sync.WaitGroup // tracks in-flight tasks
}

// NewSummarizer creates a new Summarizer with a buffered task queue.
func NewSummarizer(historyStore *history.Store, agentRegistry *agent.Registry, cfg *config.Config) *Summarizer {
	executor := NewExecutor()
	// Allow override via sync.summary_timeout config (default: 3m)
	if cfg != nil && cfg.Sync.SummaryTimeout != "" {
		if d, err := time.ParseDuration(cfg.Sync.SummaryTimeout); err == nil && d > 0 {
			executor = NewExecutorWithTimeout(d)
		}
	}
	return &Summarizer{
		queue:         make(chan Task, 10),
		historyStore:  historyStore,
		agentRegistry: agentRegistry,
		executor:      executor,
		config:        cfg,
		logger:        log.Default(),
	}
}

// SetLogger sets a custom logger.
func (s *Summarizer) SetLogger(logger *log.Logger) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.logger = logger
}

// Start launches the worker goroutine. Call once during app startup.
func (s *Summarizer) Start() {
	go s.worker()
}

// Enqueue adds a summarization task to the queue.
// Non-blocking: if the queue is full or the Summarizer is stopped, the task is dropped.
func (s *Summarizer) Enqueue(task Task) {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		s.logger.Printf("[summarizer] stopped, dropping summary task for %s (history #%d)", task.RepoName, task.HistoryID)
		_ = s.historyStore.UpdateSummary(task.HistoryID, "", string(types.SummaryStatusFailed))
		return
	}
	select {
	case s.queue <- task:
		s.wg.Add(1)
		s.mu.Unlock()
		s.logger.Printf("[summarizer] enqueued summary task for %s (history #%d)", task.RepoName, task.HistoryID)
	default:
		s.mu.Unlock()
		s.logger.Printf("[summarizer] queue full, dropping summary task for %s", task.RepoName)
		// Mark as failed since status was already set to pending at record time
		_ = s.historyStore.UpdateSummary(task.HistoryID, "", string(types.SummaryStatusFailed))
	}
}

// worker processes tasks from the queue one at a time.
func (s *Summarizer) worker() {
	for task := range s.queue {
		s.processTask(task)
		s.wg.Done()
	}
}

// processTask handles a single summarization task.
func (s *Summarizer) processTask(task Task) {
	s.logger.Printf("[summarizer] processing summary for %s (history #%d)", task.RepoName, task.HistoryID)

	// Update status to generating
	_ = s.historyStore.UpdateSummary(task.HistoryID, "", string(types.SummaryStatusGenerating))

	// Determine which agent to use
	agentName := ""
	if s.config != nil {
		agentName = s.config.Sync.SummaryAgent
	}
	if agentName == "" {
		// Auto-select: use preferred or first available
		if prov, err := s.agentRegistry.GetPreferred(); err == nil {
			agentName = prov.Name()
		}
	}

	if agentName == "" {
		s.failTask(task, "no agent available for summarization")
		return
	}

	if !IsAgentAvailable(agentName) {
		s.failTask(task, fmt.Sprintf("agent %q is not installed", agentName))
		return
	}

	// Generate summary
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	summary, err := s.executor.Summarize(ctx, task.Commits, task.Language, agentName)
	if err != nil {
		s.failTask(task, err.Error())
		return
	}

	// Success
	if err := s.historyStore.UpdateSummary(task.HistoryID, summary, string(types.SummaryStatusDone)); err != nil {
		s.logger.Printf("[summarizer] failed to save summary for %s: %v", task.RepoName, err)
		return
	}

	s.logger.Printf("[summarizer] summary generated for %s (history #%d) by %s", task.RepoName, task.HistoryID, agentName)
}

// failTask marks a task as failed.
func (s *Summarizer) failTask(task Task, errMsg string) {
	_ = s.historyStore.UpdateSummary(task.HistoryID, "", string(types.SummaryStatusFailed))
	s.logger.Printf("[summarizer] summary failed for %s (history #%d): %s", task.RepoName, task.HistoryID, errMsg)
}

// WaitIdle blocks until all enqueued tasks have been processed.
// It should be called before Stop to ensure no in-flight work is lost.
func (s *Summarizer) WaitIdle(ctx context.Context) {
	done := make(chan struct{})
	go func() {
		s.wg.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-ctx.Done():
		s.logger.Printf("[summarizer] WaitIdle timed out, some tasks may not have completed")
	}
}

// Stop signals the worker to stop (non-blocking).
// Safe to call multiple times.
func (s *Summarizer) Stop() {
	s.closeOnce.Do(func() {
		s.mu.Lock()
		s.closed = true
		close(s.queue)
		s.mu.Unlock()
	})
}

// StopAndWait stops the summarizer and waits for in-flight tasks to complete.
func (s *Summarizer) StopAndWait(ctx context.Context) {
	s.Stop()
	// The worker goroutine will automatically drain remaining tasks from the
	// closed channel and call wg.Done() for each. We just wait for it to finish.
	done := make(chan struct{})
	go func() {
		s.wg.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-ctx.Done():
		s.logger.Printf("[summarizer] StopAndWait timed out")
	}
}
