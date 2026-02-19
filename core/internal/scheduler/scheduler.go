package scheduler

import (
	"context"
	"fmt"
	"log/slog"
	"sync"

	"github.com/robfig/cron/v3"
)

// WakeFunc is called when a scheduled job fires.
type WakeFunc func(agentID, message string)

// Scheduler manages cron-based agent wake-up schedules.
type Scheduler struct {
	mu     sync.Mutex
	cron   *cron.Cron
	jobs   map[string][]cron.EntryID // agent_id → entry IDs
	wakeFn WakeFunc
	logger *slog.Logger
}

// New creates a new scheduler.
func New(wakeFn WakeFunc, logger *slog.Logger) *Scheduler {
	if logger == nil {
		logger = slog.Default()
	}
	return &Scheduler{
		cron:   cron.New(),
		jobs:   make(map[string][]cron.EntryID),
		wakeFn: wakeFn,
		logger: logger,
	}
}

// Start begins the cron scheduler. Blocks until context is cancelled.
func (s *Scheduler) Start(ctx context.Context) error {
	s.cron.Start()
	s.logger.Info("scheduler started")

	<-ctx.Done()
	s.cron.Stop()
	s.logger.Info("scheduler stopped")
	return ctx.Err()
}

// RegisterAgent adds a cron schedule for an agent.
// The schedule should be a standard cron expression (5 fields) or a predefined schedule like @every 1h.
func (s *Scheduler) RegisterAgent(agentID, schedule string) error {
	return s.AddJob(agentID, schedule, "Scheduled wake-up")
}

// AddJob adds a scheduled job for an agent with a custom message.
func (s *Scheduler) AddJob(agentID, schedule, message string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	id, err := s.cron.AddFunc(schedule, func() {
		s.logger.Info("cron fired", "agent", agentID, "message", message)
		s.wakeFn(agentID, message)
	})
	if err != nil {
		return fmt.Errorf("scheduler: invalid schedule %q: %w", schedule, err)
	}

	s.jobs[agentID] = append(s.jobs[agentID], id)
	s.logger.Info("job registered", "agent", agentID, "schedule", schedule)
	return nil
}

// RemoveAgent removes all scheduled jobs for an agent.
func (s *Scheduler) RemoveAgent(agentID string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	for _, id := range s.jobs[agentID] {
		s.cron.Remove(id)
	}
	delete(s.jobs, agentID)
}

// RemoveJob removes a specific job by entry ID.
func (s *Scheduler) RemoveJob(entryID cron.EntryID) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.cron.Remove(entryID)
}

// ListJobs returns all entry IDs for an agent.
func (s *Scheduler) ListJobs(agentID string) []cron.EntryID {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.jobs[agentID]
}

// JobCount returns the total number of scheduled jobs.
func (s *Scheduler) JobCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	total := 0
	for _, ids := range s.jobs {
		total += len(ids)
	}
	return total
}
