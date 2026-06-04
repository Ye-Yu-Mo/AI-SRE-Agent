package action

import (
	"encoding/json"
	"testing"
	"time"
)

func TestPlanStatus_Constants(t *testing.T) {
	statuses := map[PlanStatus]string{
		PlanPending:    "pending",
		PlanApproved:   "approved",
		PlanRunning:    "running",
		PlanSucceeded:  "succeeded",
		PlanFailed:     "failed",
		PlanRolledBack: "rolled_back",
		PlanCancelled:  "cancelled",
		PlanExpired:    "expired",
	}
	for s, expected := range statuses {
		if string(s) != expected {
			t.Errorf("PlanStatus %s: string = %q, want %q", s, string(s), expected)
		}
	}
}

func TestPlan_Validate_Valid(t *testing.T) {
	p := Plan{
		ID:         "plan_001",
		Intent:     "restart nginx",
		ServerID:   "srv_abc",
		Risk:       RiskMedium,
		Status:     PlanPending,
		CreatedAt:  time.Now().UTC(),
		ExpiresAt:  time.Now().UTC().Add(10 * time.Minute),
		Steps: []ActionStep{
			{Step: 1, Action: Action{ID: "act_001", Type: ActionServiceRestart, Target: Target{Kind: "systemd_service", Name: "nginx"}, Risk: RiskMedium}},
		},
	}
	if err := p.Validate(); err != nil {
		t.Errorf("valid plan should pass: %v", err)
	}
}

func TestPlan_Validate_EmptyIntent(t *testing.T) {
	p := Plan{ID: "plan_001", ServerID: "srv_abc"}
	if err := p.Validate(); err == nil {
		t.Error("plan with empty intent should fail")
	}
}

func TestPlan_Validate_EmptySteps(t *testing.T) {
	p := Plan{ID: "plan_001", Intent: "restart nginx", ServerID: "srv_abc"}
	if err := p.Validate(); err == nil {
		t.Error("plan with no steps should fail")
	}
}

func TestPlan_Validate_InvalidStepAction(t *testing.T) {
	p := Plan{
		ID:       "plan_001",
		Intent:   "bad action",
		ServerID: "srv_abc",
		Steps: []ActionStep{
			{Step: 1, Action: Action{ID: "", Type: ActionType("bad"), Target: Target{}}},
		},
	}
	if err := p.Validate(); err == nil {
		t.Error("plan with invalid step action should fail")
	}
}

func TestPlan_IsExpired(t *testing.T) {
	future := Plan{ID: "p1", Intent: "x", ServerID: "s", ExpiresAt: time.Now().UTC().Add(time.Hour)}
	if future.IsExpired() {
		t.Error("future plan should not be expired")
	}

	past := Plan{ID: "p2", Intent: "x", ServerID: "s", ExpiresAt: time.Now().UTC().Add(-time.Hour)}
	if !past.IsExpired() {
		t.Error("past plan should be expired")
	}
}

func TestPlan_CanTransition(t *testing.T) {
	tests := []struct {
		from   PlanStatus
		to     PlanStatus
		expect bool
	}{
		{PlanPending, PlanApproved, true},
		{PlanPending, PlanCancelled, true},
		{PlanPending, PlanExpired, true},
		{PlanPending, PlanRunning, false},  // 不能跳过审批
		{PlanPending, PlanSucceeded, false}, // 不能跳过执行
		{PlanApproved, PlanRunning, true},
		{PlanApproved, PlanCancelled, true},
		{PlanApproved, PlanSucceeded, false}, // 不能跳过执行
		{PlanRunning, PlanSucceeded, true},
		{PlanRunning, PlanFailed, true},
		{PlanRunning, PlanPending, false},  // 不能回到 pending
		{PlanSucceeded, PlanRolledBack, true},
		{PlanSucceeded, PlanRunning, false}, // 已完成不能重跑
		{PlanFailed, PlanRunning, false},    // 失败不能直接重跑
		{PlanCancelled, PlanRunning, false}, // 取消后不能重跑
		{PlanExpired, PlanPending, false},   // 过期不能复活
	}

	for _, tt := range tests {
		p := Plan{ID: "p", Intent: "x", ServerID: "s", Status: tt.from}
		got := p.CanTransitionTo(tt.to)
		if got != tt.expect {
			t.Errorf("CanTransition(%s → %s) = %v, want %v", tt.from, tt.to, got, tt.expect)
		}
	}
}

func TestPlan_JSON_Roundtrip(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	p := Plan{
		ID:               "plan_123",
		Intent:           "Deploy app to production",
		ServerID:         "srv_prod_01",
		Risk:             RiskHigh,
		Status:           PlanPending,
		RequiresApproval: true,
		CreatedAt:        now,
		ExpiresAt:        now.Add(10 * time.Minute),
		Steps: []ActionStep{
			{Step: 1, Action: Action{ID: "act_1", Type: ActionServiceRestart, Target: Target{Kind: "systemd_service", Name: "nginx"}, Risk: RiskMedium}},
			{Step: 2, Action: Action{ID: "act_2", Type: ActionServiceStart, Target: Target{Kind: "systemd_service", Name: "app"}, Risk: RiskMedium}},
		},
		Rollback: &RollbackContract{Available: true, Strategy: "previous_release"},
	}

	data, err := json.Marshal(p)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var q Plan
	if err := json.Unmarshal(data, &q); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if q.Intent != p.Intent || q.Risk != RiskHigh || q.RequiresApproval != true {
		t.Errorf("roundtrip mismatch: %+v", q)
	}
	if len(q.Steps) != 2 || q.Steps[0].Step != 1 || q.Steps[1].Step != 2 {
		t.Errorf("steps mismatch: got %d steps", len(q.Steps))
	}
	if q.Rollback == nil || q.Rollback.Strategy != "previous_release" {
		t.Errorf("rollback mismatch: %+v", q.Rollback)
	}
	if len(q.Steps[0].Action.Target.Name) == 0 {
		t.Error("step action target is empty after roundtrip")
	}
}

func TestRollbackContract_Defaults(t *testing.T) {
	rc := RollbackContract{}
	if rc.Available {
		t.Error("default rollback should not be available")
	}
	if rc.Strategy != "" && rc.Strategy != "none" {
		t.Errorf("default strategy should be empty or 'none', got %q", rc.Strategy)
	}
}
