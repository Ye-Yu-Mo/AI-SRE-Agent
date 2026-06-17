package risk

import (
	"testing"

	"github.com/ai-sre/agent/internal/action"
)

func TestClassify_ReadActionsAreLow(t *testing.T) {
	readTypes := []action.ActionType{
		action.ActionServerInspect,
		action.ActionServerHealth,
		action.ActionServerResources,
		action.ActionServiceList,
		action.ActionServiceStatus,
		action.ActionServiceLogs,
		action.ActionDockerList,
		action.ActionDockerLogs,
	}

	for _, at := range readTypes {
		act := action.Action{ID: "x", Type: at, Target: action.Target{Kind: "test", Name: "test"}}
		got := Classify(act, "production")
		if got.Level != action.RiskLow {
			t.Errorf("%s in production: level = %s, want low", at, got.Level)
		}
		if got.Decision != DecisionAllow {
			t.Errorf("%s in production: decision = %s, want allow", at, got.Decision)
		}
	}
}

func TestClassify_ServiceRestart(t *testing.T) {
	tests := []struct {
		service  string
		env      string
		wantRisk action.RiskLevel
		wantDec  Decision
	}{
		{"nginx", "production", action.RiskMedium, DecisionApprovalRequired},
		{"nginx", "staging", action.RiskMedium, DecisionAllow},
		{"nginx", "development", action.RiskMedium, DecisionAllow},
		{"postgresql", "production", action.RiskHigh, DecisionApprovalRequired},
		{"postgresql", "staging", action.RiskHigh, DecisionApprovalRequired},
		{"postgresql", "development", action.RiskHigh, DecisionAllow},
		{"redis", "production", action.RiskHigh, DecisionApprovalRequired},
		{"mysql", "production", action.RiskHigh, DecisionApprovalRequired},
	}

	for _, tt := range tests {
		act := action.Action{
			ID:     "x",
			Type:   action.ActionServiceRestart,
			Target: action.Target{Kind: "systemd_service", Name: tt.service},
		}
		got := Classify(act, tt.env)
		if got.Level != tt.wantRisk {
			t.Errorf("restart %s in %s: level = %s, want %s", tt.service, tt.env, got.Level, tt.wantRisk)
		}
		if got.Decision != tt.wantDec {
			t.Errorf("restart %s in %s: decision = %s, want %s", tt.service, tt.env, got.Decision, tt.wantDec)
		}
	}
}

func TestClassify_DockerRestart(t *testing.T) {
	act := action.Action{
		ID:     "x",
		Type:   action.ActionDockerRestart,
		Target: action.Target{Kind: "docker_container", Name: "web-1"},
	}
	got := Classify(act, "production")
	if got.Level != action.RiskMedium {
		t.Errorf("docker restart in production: level = %s, want medium", got.Level)
	}
	if got.Decision != DecisionApprovalRequired {
		t.Errorf("docker restart in production: decision = %s, want approval_required", got.Decision)
	}
}

func TestClassify_DatabaseActionAlwaysHigh(t *testing.T) {
	dbServices := []string{"postgresql", "postgres", "mysql", "mariadb", "mongod", "redis", "redis-server"}
	for _, svc := range dbServices {
		act := action.Action{
			ID:     "x",
			Type:   action.ActionServiceRestart,
			Target: action.Target{Kind: "systemd_service", Name: svc},
		}
		got := Classify(act, "production")
		if got.Level != action.RiskHigh {
			t.Errorf("restart %s: level = %s, want high", svc, got.Level)
		}
	}
}

func TestClassify_UnknownActionType(t *testing.T) {
	act := action.Action{
		ID:     "x",
		Type:   action.ActionType("unknown.action"),
		Target: action.Target{Kind: "test", Name: "test"},
	}
	got := Classify(act, "production")
	if got.Level != action.RiskCritical {
		t.Errorf("unknown action: level = %s, want critical", got.Level)
	}
	if got.Decision != DecisionDeny {
		t.Errorf("unknown action: decision = %s, want deny", got.Decision)
	}
}

// M1: 停止生产数据库是不可逆的破坏性操作，必须 critical + deny，
// 在 plan 创建阶段就拦下，而不是等审批。restart 仍是 high（可恢复）。
func TestClassify_StopDatabaseIsCriticalDeny(t *testing.T) {
	dbServices := []string{"postgresql", "postgres", "mysql", "mariadb", "mongod", "redis"}
	for _, svc := range dbServices {
		act := action.Action{
			ID:     "x",
			Type:   action.ActionServiceStop,
			Target: action.Target{Kind: "systemd_service", Name: svc},
		}
		got := Classify(act, "production")
		if got.Level != action.RiskCritical {
			t.Errorf("stop %s: level = %s, want critical", svc, got.Level)
		}
		if got.Decision != DecisionDeny {
			t.Errorf("stop %s: decision = %s, want deny", svc, got.Decision)
		}
	}
}

// docker.stop 数据库容器同样 critical + deny。
func TestClassify_StopDatabaseContainerIsCriticalDeny(t *testing.T) {
	act := action.Action{
		ID:     "x",
		Type:   action.ActionDockerStop,
		Target: action.Target{Kind: "docker_container", Name: "postgres"},
	}
	got := Classify(act, "production")
	if got.Level != action.RiskCritical {
		t.Errorf("stop postgres container: level = %s, want critical", got.Level)
	}
	if got.Decision != DecisionDeny {
		t.Errorf("stop postgres container: decision = %s, want deny", got.Decision)
	}
}

// 停止非数据库服务不应升级为 critical——别误伤正常操作。
func TestClassify_StopNonDatabaseStaysMedium(t *testing.T) {
	act := action.Action{
		ID:     "x",
		Type:   action.ActionServiceStop,
		Target: action.Target{Kind: "systemd_service", Name: "nginx"},
	}
	got := Classify(act, "production")
	if got.Level != action.RiskMedium {
		t.Errorf("stop nginx: level = %s, want medium", got.Level)
	}
	if got.Decision != DecisionApprovalRequired {
		t.Errorf("stop nginx: decision = %s, want approval_required", got.Decision)
	}
}

func TestClassify_EmptyEnvironmentDefaultsToProduction(t *testing.T) {
	act := action.Action{
		ID:     "x",
		Type:   action.ActionServiceRestart,
		Target: action.Target{Kind: "systemd_service", Name: "nginx"},
	}
	got := Classify(act, "")
	if got.Level != action.RiskMedium {
		t.Errorf("empty env: level = %s, want medium", got.Level)
	}
}

func TestDecision_Constants(t *testing.T) {
	decisions := map[Decision]string{
		DecisionAllow:             "allow",
		DecisionDeny:              "deny",
		DecisionApprovalRequired:  "approval_required",
		DecisionReadOnlyOnly:      "read_only_only",
	}
	for d, expected := range decisions {
		if string(d) != expected {
			t.Errorf("Decision %s: string = %q, want %q", d, string(d), expected)
		}
	}
}

func TestResult_IsError(t *testing.T) {
	r := Result{Level: action.RiskLow, Decision: DecisionAllow}
	if r.Decision.IsDenied() {
		t.Error("allow decision should not be denied")
	}

	r2 := Result{Level: action.RiskCritical, Decision: DecisionDeny}
	if !r2.Decision.IsDenied() {
		t.Error("deny decision should be denied")
	}

	r3 := Result{Level: action.RiskHigh, Decision: DecisionApprovalRequired}
	if !r3.Decision.RequiresApproval() {
		t.Error("approval_required should require approval")
	}
	if r3.Decision.IsDenied() {
		t.Error("approval_required should not be denied")
	}
}
