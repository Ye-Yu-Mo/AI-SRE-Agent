package plan

import (
	"testing"
	"time"

	"github.com/ai-sre/agent/internal/action"
)

func TestStore_CreateAndGet(t *testing.T) {
	s := NewStore()
	p := &action.Plan{
		ID:       "plan_test_001",
		Intent:   "restart nginx",
		ServerID: "srv_abc",
		Risk:     action.RiskMedium,
		Status:   action.PlanPending,
		Steps: []action.ActionStep{
			{Step: 1, Action: action.Action{
				ID: "act_001", Type: action.ActionServiceRestart,
				Target: action.Target{Kind: "systemd_service", Name: "nginx"},
			}},
		},
		CreatedAt: time.Now().UTC(),
		ExpiresAt: time.Now().UTC().Add(10 * time.Minute),
	}

	s.Create(p)

	got, ok := s.Get("plan_test_001")
	if !ok {
		t.Fatal("plan not found after create")
	}
	if got.Intent != "restart nginx" {
		t.Errorf("Intent = %q, want %q", got.Intent, "restart nginx")
	}
	if got.Status != action.PlanPending {
		t.Errorf("Status = %s, want pending", got.Status)
	}
}

func TestStore_Get_Missing(t *testing.T) {
	s := NewStore()
	_, ok := s.Get("nonexistent")
	if ok {
		t.Error("expected false for missing plan")
	}
}

func TestStore_UpdateStatus(t *testing.T) {
	s := NewStore()
	p := &action.Plan{
		ID:       "plan_test_002",
		Intent:   "restart something",
		ServerID: "srv_abc",
		Status:   action.PlanPending,
		Steps:    []action.ActionStep{{Step: 1, Action: action.Action{ID: "a", Type: action.ActionServiceRestart, Target: action.Target{Kind: "s", Name: "x"}}}},
	}
	s.Create(p)

	// pending → approved
	if err := s.UpdateStatus("plan_test_002", action.PlanApproved); err != nil {
		t.Fatalf("UpdateStatus pending→approved: %v", err)
	}
	// approved → running
	if err := s.UpdateStatus("plan_test_002", action.PlanRunning); err != nil {
		t.Fatalf("UpdateStatus approved→running: %v", err)
	}

	got, _ := s.Get("plan_test_002")
	if got.Status != action.PlanRunning {
		t.Errorf("Status = %s, want running", got.Status)
	}
}

func TestStore_UpdateStatus_InvalidTransition(t *testing.T) {
	s := NewStore()
	p := &action.Plan{
		ID: "plan_test_003", Intent: "x", ServerID: "s",
		Status: action.PlanSucceeded,
		Steps:  []action.ActionStep{{Step: 1, Action: action.Action{ID: "a", Type: action.ActionServiceRestart, Target: action.Target{Kind: "s", Name: "x"}}}},
	}
	s.Create(p)

	err := s.UpdateStatus("plan_test_003", action.PlanRunning)
	if err == nil {
		t.Error("expected error: cannot transition succeeded → running")
	}
}

func TestStore_List(t *testing.T) {
	s := NewStore()
	for i := 0; i < 3; i++ {
		p := &action.Plan{
			ID: "plan_" + string(rune('a'+i)), Intent: "x", ServerID: "srv",
			Status: action.PlanPending,
			Steps:  []action.ActionStep{{Step: 1, Action: action.Action{ID: "a", Type: action.ActionServiceRestart, Target: action.Target{Kind: "s", Name: "x"}}}},
		}
		s.Create(p)
	}

	list := s.List()
	if len(list) != 3 {
		t.Errorf("List() = %d plans, want 3", len(list))
	}
}
