// Package task defines the Task data structure and the work queue that
// manages task lifecycle: submission, dispatch to workers, and completion.
package task

import (
	"sync"
	"time"
)

// TaskStatus represents the current state of a task.
type TaskStatus string

const (
	StatusPending    TaskStatus = "pending"
	StatusRunning    TaskStatus = "running"
	StatusCompleted  TaskStatus = "completed"
	StatusFailed     TaskStatus = "failed"
	StatusCancelled  TaskStatus = "cancelled"
)

// Task represents a unit of work for the agent.
type Task struct {
	ID          string     `json:"id"`
	Instruction string     `json:"instruction"`
	Status      TaskStatus `json:"status"`
	Result      string     `json:"result,omitempty"`
	Error       string     `json:"error,omitempty"`
	Steps       int        `json:"steps"` // execution steps taken
	CreatedAt   time.Time  `json:"created_at"`
	StartedAt   time.Time  `json:"started_at,omitempty"`
	FinishedAt  time.Time  `json:"finished_at,omitempty"`
}

// New creates a new Task with the given instruction.
func New(id, instruction string) *Task {
	return &Task{
		ID:          id,
		Instruction: instruction,
		Status:      StatusPending,
		CreatedAt:   time.Now(),
	}
}

// SetResult marks a task as completed with the given result.
func (t *Task) SetResult(result string, steps int) {
	t.Status = StatusCompleted
	t.Result = result
	t.Steps = steps
	t.FinishedAt = time.Now()
}

// SetError marks a task as failed.
func (t *Task) SetError(err error, steps int) {
	t.Status = StatusFailed
	t.Error = err.Error()
	t.Steps = steps
	t.FinishedAt = time.Now()
}

// SetCancelled marks a task as cancelled.
func (t *Task) SetCancelled() {
	t.Status = StatusCancelled
	t.FinishedAt = time.Now()
}

// Start marks a task as running.
func (t *Task) Start() {
	t.Status = StatusRunning
	t.StartedAt = time.Now()
}

// Queue is a bounded work queue for tasks. It supports concurrent submission
// and consumption with a configurable worker pool.
type Queue struct {
	ch       chan *Task
	inFlight map[string]*Task
	mu       sync.Mutex
	wg       sync.WaitGroup
	closed   bool
}

// NewQueue creates a task queue with the given buffer size.
func NewQueue(size int) *Queue {
	if size <= 0 {
		size = 256
	}
	return &Queue{
		ch:       make(chan *Task, size),
		inFlight: make(map[string]*Task),
	}
}

// Submit adds a task to the queue. Returns false if the queue is full.
func (q *Queue) Submit(t *Task) bool {
	q.mu.Lock()
	if q.closed {
		q.mu.Unlock()
		return false
	}
	q.mu.Unlock()

	select {
	case q.ch <- t:
		return true
	default:
		return false
	}
}

// Chan returns the underlying channel for consumption with select.
func (q *Queue) Chan() <-chan *Task {
	return q.ch
}

// Track marks a task as in-flight.
func (q *Queue) Track(t *Task) {
	q.mu.Lock()
	q.inFlight[t.ID] = t
	q.mu.Unlock()
}

// Untrack removes a task from in-flight tracking.
func (q *Queue) Untrack(t *Task) {
	q.mu.Lock()
	delete(q.inFlight, t.ID)
	q.mu.Unlock()
}

// InFlight returns the number of currently running tasks.
func (q *Queue) InFlight() int {
	q.mu.Lock()
	defer q.mu.Unlock()
	return len(q.inFlight)
}

// Len returns the number of tasks waiting in the queue.
func (q *Queue) Len() int {
	return len(q.ch)
}

// Close stops accepting new tasks and waits for pending tasks to drain.
func (q *Queue) Close() {
	q.mu.Lock()
	q.closed = true
	q.mu.Unlock()
	close(q.ch)
}
