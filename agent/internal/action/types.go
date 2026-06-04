package action

import (
	"fmt"
	"time"
)

// ActionType 预定义的 typed action 常量。
// AI 只能请求这些 action，不能生成任意 shell 命令。
type ActionType string

const (
	// Read-only
	ActionServerInspect   ActionType = "server.inspect"
	ActionServerHealth    ActionType = "server.health"
	ActionServerResources ActionType = "server.resources"

	// Service read
	ActionServiceList   ActionType = "service.list"
	ActionServiceStatus ActionType = "service.status"
	ActionServiceLogs   ActionType = "service.logs"

	// Service write
	ActionServiceStart   ActionType = "service.start"
	ActionServiceStop    ActionType = "service.stop"
	ActionServiceRestart ActionType = "service.restart"
	ActionServiceReload  ActionType = "service.reload"

	// Docker read
	ActionDockerList ActionType = "docker.list"
	ActionDockerLogs ActionType = "docker.logs"

	// Docker write
	ActionDockerRestart ActionType = "docker.restart"
	ActionDockerStart   ActionType = "docker.start"
	ActionDockerStop    ActionType = "docker.stop"
)

// validTypes 白名单：只有在这张表里的 action type 才能通过 Validate。
var validTypes = map[ActionType]bool{
	ActionServerInspect:     true,
	ActionServerHealth:      true,
	ActionServerResources:   true,
	ActionServiceList:       true,
	ActionServiceStatus:     true,
	ActionServiceLogs:       true,
	ActionServiceStart:      true,
	ActionServiceStop:       true,
	ActionServiceRestart:    true,
	ActionServiceReload:     true,
	ActionDockerList:        true,
	ActionDockerLogs:        true,
	ActionDockerRestart:     true,
	ActionDockerStart:       true,
	ActionDockerStop:        true,
}

// readOnlyTypes 只读 action 集合。
var readOnlyTypes = map[ActionType]bool{
	ActionServerInspect:     true,
	ActionServerHealth:      true,
	ActionServerResources:   true,
	ActionServiceList:       true,
	ActionServiceStatus:     true,
	ActionServiceLogs:       true,
	ActionDockerList:        true,
	ActionDockerLogs:        true,
}

type RiskLevel string

const (
	RiskLow      RiskLevel = "low"
	RiskMedium   RiskLevel = "medium"
	RiskHigh     RiskLevel = "high"
	RiskCritical RiskLevel = "critical"
)

type Target struct {
	Kind string `json:"kind"` // "systemd_service" | "docker_container"
	Name string `json:"name"`
}

// Action 是系统唯一允许执行的操作单位。
type Action struct {
	ID               string    `json:"action_id"`
	Type             ActionType `json:"type"`
	Target           Target    `json:"target"`
	ServerID         string    `json:"server_id,omitempty"`
	Risk             RiskLevel  `json:"risk"`
	RequiresApproval bool      `json:"requires_approval"`
	CreatedBy        string    `json:"created_by,omitempty"`
	CreatedAt        time.Time `json:"created_at"`
}

// Validate 校验 action 是否合法。
func (a *Action) Validate() error {
	if a.ID == "" {
		return fmt.Errorf("action ID is empty")
	}
	if a.Type == "" {
		return fmt.Errorf("action type is empty")
	}
	if !validTypes[a.Type] {
		return fmt.Errorf("unsupported action type: %s", a.Type)
	}
	if a.Target.Name == "" {
		return fmt.Errorf("target name is empty")
	}
	// 高风险写操作默认要求审批
	if !a.RequiresApproval && !a.IsReadOnly() && (a.Risk == RiskHigh || a.Risk == RiskCritical) {
		a.RequiresApproval = true
	}
	return nil
}

// IsReadOnly 判断 action 是否有副作用。
func (a *Action) IsReadOnly() bool {
	return readOnlyTypes[a.Type]
}

// PlanStatus 定义 plan 的生命周期状态。
type PlanStatus string

const (
	PlanPending    PlanStatus = "pending"
	PlanApproved   PlanStatus = "approved"
	PlanRunning    PlanStatus = "running"
	PlanSucceeded  PlanStatus = "succeeded"
	PlanFailed     PlanStatus = "failed"
	PlanRolledBack PlanStatus = "rolled_back"
	PlanCancelled  PlanStatus = "cancelled"
	PlanExpired    PlanStatus = "expired"
)

// validTransitions 状态机：每个状态允许跃迁到哪些状态。
var validTransitions = map[PlanStatus]map[PlanStatus]bool{
	PlanPending:    {PlanApproved: true, PlanCancelled: true, PlanExpired: true},
	PlanApproved:   {PlanRunning: true, PlanCancelled: true},
	PlanRunning:    {PlanSucceeded: true, PlanFailed: true},
	PlanSucceeded:  {PlanRolledBack: true},
	PlanFailed:     {},
	PlanRolledBack: {},
	PlanCancelled:  {},
	PlanExpired:    {},
}

// ActionStep 是 plan 中的一个有序步骤。
type ActionStep struct {
	Step   int    `json:"step"`
	Action Action `json:"action"`
}

// RollbackContract 定义回滚策略。
type RollbackContract struct {
	Available bool   `json:"available"`
	Strategy  string `json:"strategy"` // "previous_release" | "none"
}

// Plan 是一组 action 的有序集合，必须审批后才能执行。
type Plan struct {
	ID               string          `json:"plan_id"`
	Intent           string          `json:"intent"`
	ServerID         string          `json:"server_id"`
	Risk             RiskLevel       `json:"risk"`
	Status           PlanStatus      `json:"status"`
	RequiresApproval bool            `json:"requires_approval"`
	CreatedAt        time.Time       `json:"created_at"`
	ExpiresAt        time.Time       `json:"expires_at,omitempty"`
	Steps            []ActionStep    `json:"steps"`
	Rollback         *RollbackContract `json:"rollback,omitempty"`
}

// Validate 校验 plan 是否合法。
func (p *Plan) Validate() error {
	if p.Intent == "" {
		return fmt.Errorf("plan intent is empty")
	}
	if p.ServerID == "" {
		return fmt.Errorf("plan server_id is empty")
	}
	if len(p.Steps) == 0 {
		return fmt.Errorf("plan has no steps")
	}
	for _, s := range p.Steps {
		if err := s.Action.Validate(); err != nil {
			return fmt.Errorf("step %d: %w", s.Step, err)
		}
	}
	return nil
}

// IsExpired 判断 plan 是否已过期。
func (p *Plan) IsExpired() bool {
	return !p.ExpiresAt.IsZero() && time.Now().UTC().After(p.ExpiresAt)
}

// CanTransitionTo 检查状态跃迁是否合法。
func (p *Plan) CanTransitionTo(target PlanStatus) bool {
	return validTransitions[p.Status][target]
}
