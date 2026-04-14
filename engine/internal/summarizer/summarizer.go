// Package summarizer provides async AI-generated summaries for sync history.
// It uses a single-worker queue model to process summarization tasks.
package summarizer

import (
	"context"
	"fmt"
	"log"
	"sync"
	"sync/atomic"

	"github.com/loongxjin/forksync/engine/internal/agent"
	"github.com/loongxjin/forksync/engine/internal/config"
	"github.com/loongxjin/forksync/engine/internal/history"
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
	closed        atomic.Bool
}

// NewSummarizer creates a new Summarizer with a buffered task queue.
func NewSummarizer(historyStore *history.Store, agentRegistry *agent.Registry, cfg *config.Config) *Summarizer {
	return &Summarizer{
		queue:         make(chan Task, 10),
		historyStore:  historyStore,
		agentRegistry: agentRegistry,
		executor:      NewExecutor(),
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
	if s.closed.Load() {
		s.logger.Printf("[summarizer] stopped, dropping summary task for %s (history #%d)", task.RepoName, task.HistoryID)
		_ = s.historyStore.UpdateSummary(task.HistoryID, "", "failed")
		return
	}
	select {
	case s.queue <- task:
		s.logger.Printf("[summarizer] enqueued summary task for %s (history #%d)", task.RepoName, task.HistoryID)
	default:
		s.logger.Printf("[summarizer] queue full, dropping summary task for %s", task.RepoName)
		// Mark as failed since status was already set to pending at record time
		_ = s.historyStore.UpdateSummary(task.HistoryID, "", "failed")
	}
}

// worker processes tasks from the queue one at a time.
func (s *Summarizer) worker() {
	for task := range s.queue {
		s.processTask(task)
	}
}

// processTask handles a single summarization task.
func (s *Summarizer) processTask(task Task) {
	s.logger.Printf("[summarizer] processing summary for %s (history #%d)", task.RepoName, task.HistoryID)

	// Update status to generating
	_ = s.historyStore.UpdateSummary(task.HistoryID, "", "generating")

	// Determine which agent to use
	agentName := s.config.Sync.SummaryAgent
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
	if err := s.historyStore.UpdateSummary(task.HistoryID, summary, "done"); err != nil {
		s.logger.Printf("[summarizer] failed to save summary for %s: %v", task.RepoName, err)
		return
	}

	s.logger.Printf("[summarizer] summary generated for %s (history #%d) by %s", task.RepoName, task.HistoryID, agentName)
}

// failTask marks a task as failed.
func (s *Summarizer) failTask(task Task, errMsg string) {
	_ = s.historyStore.UpdateSummary(task.HistoryID, "", "failed")
	s.logger.Printf("[summarizer] summary failed for %s (history #%d): %s", task.RepoName, task.HistoryID, errMsg)
}

// RetrySummarize retries a failed summarization for a specific history record.
func (s *Summarizer) RetrySummarize(task Task) error {
	// Only retry failed records
	record, err := s.historyStore.GetByID(task.HistoryID)
	if err != nil {
		return fmt.Errorf("get record: %w", err)
	}
	if record.SummaryStatus != "failed" && record.SummaryStatus != "" {
		return fmt.Errorf("record %d is not in failed state (current: %s)", task.HistoryID, record.SummaryStatus)
	}

	s.Enqueue(task)
	return nil
}

// Stop signals the worker to stop (non-blocking).
// Safe to call multiple times.
func (s *Summarizer) Stop() {
	s.closeOnce.Do(func() {
		s.closed.Store(true)
		close(s.queue)
	})
}
