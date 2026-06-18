package task

import (
	"encoding/json"
	"sync"
	"time"
)

type Status string

const (
	StatusRunning   Status = "running"
	StatusSucceeded Status = "succeeded"
	StatusFailed    Status = "failed"
)

type Task struct {
	ID        string          `json:"task_id"`
	Status    Status          `json:"status"`
	Progress  string          `json:"progress,omitempty"`  // e.g. "3/7"
	Step      string          `json:"step,omitempty"`      // current step name
	Result    json.RawMessage `json:"result,omitempty"`    // final result JSON
	Error     string          `json:"error,omitempty"`
	CreatedAt string          `json:"created_at"`
	UpdatedAt string          `json:"updated_at,omitempty"`
}

type Store struct {
	mu    sync.RWMutex
	tasks map[string]*Task
}

func NewStore() *Store {
	return &Store{tasks: make(map[string]*Task)}
}

func (s *Store) Create(id string) *Task {
	s.mu.Lock()
	defer s.mu.Unlock()
	t := &Task{ID: id, Status: StatusRunning, CreatedAt: time.Now().UTC().Format(time.RFC3339)}
	s.tasks[id] = t
	return t
}

func (s *Store) Get(id string) (*Task, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	t, ok := s.tasks[id]
	return t, ok
}

// Runner 执行部署逻辑，更新 task 状态和进度。
type Runner struct {
	Store *Store
}

// Run 在 goroutine 中执行 fn，自动捕获 panic 并更新 task 状态。
func (r *Runner) Run(taskID string, fn func(task *Task)) {
	go func() {
		defer func() {
			if rec := recover(); rec != nil {
				if t, ok := r.Store.Get(taskID); ok {
					t.Status = StatusFailed
					t.Error = "panic: " + stringify(rec)
					t.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
				}
			}
		}()
		if t, ok := r.Store.Get(taskID); ok {
			fn(t)
		}
	}()
}

// MarkFailed 标记 task 失败。
func (r *Runner) MarkFailed(taskID, errMsg string) {
	r.Store.mu.Lock()
	defer r.Store.mu.Unlock()
	if t, ok := r.Store.tasks[taskID]; ok {
		t.Status = StatusFailed
		t.Error = errMsg
		t.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
	}
}

// MarkSucceeded 标记 task 成功。
func (r *Runner) MarkSucceeded(taskID string, result json.RawMessage) {
	r.Store.mu.Lock()
	defer r.Store.mu.Unlock()
	if t, ok := r.Store.tasks[taskID]; ok {
		t.Status = StatusSucceeded
		t.Result = result
		t.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
	}
}

// UpdateProgress 更新 task 进度和当前步骤。
func (r *Runner) UpdateProgress(taskID, step, progress string) {
	r.Store.mu.Lock()
	defer r.Store.mu.Unlock()
	if t, ok := r.Store.tasks[taskID]; ok {
		t.Step = step
		t.Progress = progress
		t.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
	}
}

func stringify(v interface{}) string {
	b, _ := json.Marshal(v)
	return string(b)
}
