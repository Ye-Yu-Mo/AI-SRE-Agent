package risk

import (
	"strings"

	"github.com/ai-sre/agent/internal/action"
)

type Decision string

const (
	DecisionAllow            Decision = "allow"
	DecisionDeny             Decision = "deny"
	DecisionApprovalRequired Decision = "approval_required"
	DecisionReadOnlyOnly     Decision = "read_only_only"
)

func (d Decision) IsDenied() bool {
	return d == DecisionDeny
}

func (d Decision) RequiresApproval() bool {
	return d == DecisionApprovalRequired
}

type Result struct {
	Level    action.RiskLevel
	Decision Decision
	Reason   string
}

// Classify 根据 action type、target 和 environment 返回风险分级和决策。
// M2 阶段使用硬编码查找表，M4 替换为可配置策略引擎。
func Classify(act action.Action, env string) Result {
	if env == "" {
		env = "production"
	}

	// 只读 action 一律 low + allow
	if act.IsReadOnly() {
		return Result{Level: action.RiskLow, Decision: DecisionAllow}
	}

	// 未知 action type → critical + deny
	if !isValidType(act.Type) {
		return Result{Level: action.RiskCritical, Decision: DecisionDeny, Reason: "unsupported action type"}
	}

	// 写 action 按 target 和 environment 分级
	level := classifyWriteAction(act)
	decision := decide(level, env)

	return Result{Level: level, Decision: decision}
}

func classifyWriteAction(act action.Action) action.RiskLevel {
	svc := strings.ToLower(act.Target.Name)

	// stop 数据库是不可逆破坏性操作 → critical（在 plan 创建阶段直接 deny）
	// restart 数据库仍是 high（有恢复路径）
	if isStopAction(act.Type) && isDatabaseService(svc) {
		return action.RiskCritical
	}

	// 数据库 restart → high
	if isDatabaseService(svc) {
		return action.RiskHigh
	}

	// 其余 systemd/docker 写操作 → medium
	return action.RiskMedium
}

func decide(level action.RiskLevel, env string) Decision {
	switch level {
	case action.RiskLow:
		return DecisionAllow
	case action.RiskMedium:
		if env == "production" {
			return DecisionApprovalRequired
		}
		return DecisionAllow
	case action.RiskHigh:
		if env == "production" || env == "staging" {
			return DecisionApprovalRequired
		}
		return DecisionAllow
	case action.RiskCritical:
		return DecisionDeny
	default:
		return DecisionAllow
	}
}

func isDatabaseService(name string) bool {
	dbs := []string{"postgresql", "postgres", "mysql", "mariadb", "mongod", "mongodb", "redis", "redis-server"}
	for _, d := range dbs {
		if name == d {
			return true
		}
	}
	return false
}

func isStopAction(t action.ActionType) bool {
	return t == action.ActionServiceStop || t == action.ActionDockerStop
}

func isSystemdAction(t action.ActionType) bool {
	switch t {
	case action.ActionServiceStart, action.ActionServiceStop, action.ActionServiceRestart, action.ActionServiceReload:
		return true
	}
	return false
}

func isDockerAction(t action.ActionType) bool {
	switch t {
	case action.ActionDockerStart, action.ActionDockerStop, action.ActionDockerRestart:
		return true
	}
	return false
}

func isValidType(t action.ActionType) bool {
	// action package has its own validation, but we double-check here for defense in depth
	switch t {
	case
		action.ActionServerInspect, action.ActionServerHealth, action.ActionServerResources,
		action.ActionServiceList, action.ActionServiceStatus, action.ActionServiceLogs,
		action.ActionServiceStart, action.ActionServiceStop, action.ActionServiceRestart, action.ActionServiceReload,
		action.ActionDockerList, action.ActionDockerLogs,
		action.ActionDockerRestart, action.ActionDockerStart, action.ActionDockerStop:
		return true
	}
	return false
}
