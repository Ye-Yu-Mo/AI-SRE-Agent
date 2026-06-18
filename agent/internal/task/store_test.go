package task

import (
	"encoding/json"
	"sync"
	"testing"
	"time"
)

func TestStore_CreateGet(t *testing.T) {
	s := NewStore()
	task := s.Create("task_001")
	if task.ID != "task_001" {
		t.Errorf("ID = %q, want task_001", task.ID)
	}
	if task.Status != StatusRunning {
		t.Errorf("Status = %q, want running", task.Status)
	}

	got, ok := s.Get("task_001")
	if !ok {
		t.Fatal("task not found")
	}
	if got.ID != "task_001" {
		t.Errorf("Get ID = %q", got.ID)
	}
}

func TestRunner_RunCompletes(t *testing.T) {
	s := NewStore()
	r := &Runner{Store: s}
	taskID := "task_002"
	s.Create(taskID)

	done := make(chan struct{})
	r.Run(taskID, func(task *Task) {
		time.Sleep(50 * time.Millisecond)
		r.UpdateProgress(taskID, "build", "3/7")
		time.Sleep(50 * time.Millisecond)
		result, _ := json.Marshal(map[string]string{"status": "ok"})
		r.MarkSucceeded(taskID, result)
		close(done)
	})

	select {
	case <-done:
		task, ok := s.Get(taskID)
		if !ok {
			t.Fatal("task not found after completion")
		}
		if task.Status != StatusSucceeded {
			t.Errorf("Status = %q, want succeeded", task.Status)
		}
		if task.Step != "build" {
			t.Errorf("Step = %q, want build", task.Step)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("task did not complete in time")
	}
}

func TestRunner_PanicRecovery(t *testing.T) {
	s := NewStore()
	r := &Runner{Store: s}
	taskID := "task_003"
	s.Create(taskID)

	var wg sync.WaitGroup
	wg.Add(1)
	r.Run(taskID, func(task *Task) {
		defer wg.Done()
		panic("something went wrong")
	})

	wg.Wait()
	// 给 goroutine 一点时间写入状态
	time.Sleep(10 * time.Millisecond)

	task, ok := s.Get(taskID)
	if !ok {
		t.Fatal("task not found after panic")
	}
	if task.Status != StatusFailed {
		t.Errorf("Status = %q, want failed", task.Status)
	}
	if task.Error == "" {
		t.Error("Error should not be empty after panic")
	}
}

func TestStore_Concurrency(t *testing.T) {
	s := NewStore()
	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			s.Create("task_c_" + string(rune('0'+n%10)))
		}(i)
	}
	wg.Wait()
}
