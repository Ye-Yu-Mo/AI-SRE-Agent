package action

import (
	"encoding/json"
	"testing"
	"time"
)

func TestActionType_Constants(t *testing.T) {
	// 所有定义的 action type 必须有对应字符串
	types := map[ActionType]string{
		ActionServerInspect:     "server.inspect",
		ActionServerHealth:      "server.health",
		ActionServerResources:   "server.resources",
		ActionServiceList:       "service.list",
		ActionServiceStatus:     "service.status",
		ActionServiceLogs:       "service.logs",
		ActionServiceStart:      "service.start",
		ActionServiceStop:       "service.stop",
		ActionServiceRestart:    "service.restart",
		ActionServiceReload:     "service.reload",
		ActionDockerList:        "docker.list",
		ActionDockerLogs:        "docker.logs",
		ActionDockerRestart:     "docker.restart",
		ActionDockerStart:       "docker.start",
		ActionDockerStop:        "docker.stop",
	}

	for at, expected := range types {
		if string(at) != expected {
			t.Errorf("ActionType %s: string value = %q, want %q", at, string(at), expected)
		}
	}
}

func TestAction_Validate_ValidReadAction(t *testing.T) {
	a := Action{
		ID:     "act_001",
		Type:   ActionServiceList,
		Target: Target{Kind: "systemd_service", Name: "nginx"},
		Risk:   RiskLow,
	}
	if err := a.Validate(); err != nil {
		t.Errorf("valid read action should pass: %v", err)
	}
}

func TestAction_Validate_ValidWriteAction(t *testing.T) {
	a := Action{
		ID:     "act_002",
		Type:   ActionServiceRestart,
		Target: Target{Kind: "systemd_service", Name: "nginx"},
		Risk:   RiskMedium,
	}
	if err := a.Validate(); err != nil {
		t.Errorf("valid write action should pass: %v", err)
	}
}

func TestAction_Validate_EmptyID(t *testing.T) {
	a := Action{
		Type:   ActionServiceRestart,
		Target: Target{Kind: "systemd_service", Name: "nginx"},
	}
	if err := a.Validate(); err == nil {
		t.Error("action with empty ID should fail")
	}
}

func TestAction_Validate_EmptyType(t *testing.T) {
	a := Action{
		ID:     "act_001",
		Target: Target{Kind: "systemd_service", Name: "nginx"},
	}
	if err := a.Validate(); err == nil {
		t.Error("action with empty type should fail")
	}
}

func TestAction_Validate_EmptyTargetName(t *testing.T) {
	a := Action{
		ID:   "act_001",
		Type: ActionServiceRestart,
		Target: Target{Kind: "systemd_service"},
	}
	if err := a.Validate(); err == nil {
		t.Error("action with empty target name should fail")
	}
}

func TestAction_Validate_InvalidType(t *testing.T) {
	a := Action{
		ID:     "act_001",
		Type:   ActionType("service.delete"),
		Target: Target{Kind: "systemd_service", Name: "nginx"},
	}
	if err := a.Validate(); err == nil {
		t.Error("action with unsupported type should fail")
	}
}

func TestAction_Validate_HighRiskWriteRequiresApproval(t *testing.T) {
	a := Action{
		ID:     "act_003",
		Type:   ActionServiceRestart,
		Target: Target{Kind: "systemd_service", Name: "postgresql"},
		Risk:   RiskHigh,
	}
	if err := a.Validate(); err != nil {
		t.Errorf("valid high risk action should pass: %v", err)
	}
	// 高风险默认 requires_approval
	if !a.RequiresApproval {
		t.Error("high risk action should have RequiresApproval=true")
	}
}

func TestAction_JSON_Roundtrip(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	a := Action{
		ID:        "act_123",
		Type:      ActionServiceRestart,
		Target:    Target{Kind: "systemd_service", Name: "nginx"},
		ServerID:  "srv_abc",
		Risk:      RiskMedium,
		CreatedBy: "ai-agent",
		CreatedAt: now,
	}

	data, err := json.Marshal(a)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var b Action
	if err := json.Unmarshal(data, &b); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if b.ID != a.ID || b.Type != a.Type || b.Target.Name != "nginx" || b.Risk != RiskMedium || b.ServerID != "srv_abc" {
		t.Errorf("roundtrip mismatch: got %+v", b)
	}
	if !b.CreatedAt.Equal(now) {
		t.Errorf("CreatedAt mismatch: got %v, want %v", b.CreatedAt, now)
	}
}

func TestAction_IsReadOnly(t *testing.T) {
	tests := []struct {
		atype    ActionType
		readonly bool
	}{
		{ActionServerInspect, true},
		{ActionServerHealth, true},
		{ActionServiceList, true},
		{ActionServiceLogs, true},
		{ActionDockerList, true},
		{ActionDockerLogs, true},
		{ActionServiceRestart, false},
		{ActionServiceStart, false},
		{ActionServiceStop, false},
		{ActionDockerRestart, false},
	}

	for _, tt := range tests {
		a := Action{ID: "x", Type: tt.atype, Target: Target{Kind: "test", Name: "test"}}
		got := a.IsReadOnly()
		if got != tt.readonly {
			t.Errorf("%s: IsReadOnly() = %v, want %v", tt.atype, got, tt.readonly)
		}
	}
}
