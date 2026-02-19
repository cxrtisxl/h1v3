package scheduler

import (
	"sync"
	"testing"
	"time"
)

func TestAddJob(t *testing.T) {
	var mu sync.Mutex
	var calls []string

	sched := New(func(agentID, message string) {
		mu.Lock()
		calls = append(calls, agentID+":"+message)
		mu.Unlock()
	}, nil)

	err := sched.AddJob("coder", "@every 1s", "test wake")
	if err != nil {
		t.Fatalf("AddJob: %v", err)
	}

	if sched.JobCount() != 1 {
		t.Errorf("JobCount = %d", sched.JobCount())
	}

	// Start cron and wait for it to fire
	sched.cron.Start()
	time.Sleep(1500 * time.Millisecond)
	sched.cron.Stop()

	mu.Lock()
	defer mu.Unlock()
	if len(calls) == 0 {
		t.Error("expected at least one call")
	}
	if calls[0] != "coder:test wake" {
		t.Errorf("call = %q", calls[0])
	}
}

func TestRegisterAgent(t *testing.T) {
	sched := New(func(string, string) {}, nil)
	err := sched.RegisterAgent("coder", "@every 5m")
	if err != nil {
		t.Fatalf("RegisterAgent: %v", err)
	}
	if sched.JobCount() != 1 {
		t.Errorf("JobCount = %d", sched.JobCount())
	}
}

func TestInvalidSchedule(t *testing.T) {
	sched := New(func(string, string) {}, nil)
	err := sched.AddJob("coder", "invalid-cron", "msg")
	if err == nil {
		t.Error("expected error for invalid schedule")
	}
}

func TestRemoveAgent(t *testing.T) {
	sched := New(func(string, string) {}, nil)
	sched.AddJob("coder", "@every 1h", "msg1")
	sched.AddJob("coder", "@every 2h", "msg2")

	if sched.JobCount() != 2 {
		t.Fatalf("JobCount = %d before remove", sched.JobCount())
	}

	sched.RemoveAgent("coder")
	if sched.JobCount() != 0 {
		t.Errorf("JobCount = %d after remove", sched.JobCount())
	}
}

func TestListJobs(t *testing.T) {
	sched := New(func(string, string) {}, nil)
	sched.AddJob("coder", "@every 1h", "msg1")
	sched.AddJob("coder", "@every 2h", "msg2")
	sched.AddJob("reviewer", "@every 3h", "msg3")

	coderJobs := sched.ListJobs("coder")
	if len(coderJobs) != 2 {
		t.Errorf("coder jobs = %d", len(coderJobs))
	}

	reviewerJobs := sched.ListJobs("reviewer")
	if len(reviewerJobs) != 1 {
		t.Errorf("reviewer jobs = %d", len(reviewerJobs))
	}
}
