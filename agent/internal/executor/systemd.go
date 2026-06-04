package executor

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"github.com/ai-sre/agent/internal/action"
)

type SystemdExecutor struct{}

func (e *SystemdExecutor) Execute(ctx context.Context, act action.Action) (*ActionResult, error) {
	var systemctlCmd string
	switch act.Type {
	case action.ActionServiceStart:
		systemctlCmd = "start"
	case action.ActionServiceStop:
		systemctlCmd = "stop"
	case action.ActionServiceRestart:
		systemctlCmd = "restart"
	case action.ActionServiceReload:
		systemctlCmd = "reload"
	default:
		return nil, fmt.Errorf("systemd executor: unsupported action type %s", act.Type)
	}

	// 采集 before state
	before := captureServiceState(act.Target.Name)

	cmd := exec.CommandContext(ctx, "systemctl", systemctlCmd, act.Target.Name)
	stdout, err := cmd.Output()

	result := &ActionResult{
		ActionID:    act.ID,
		BeforeState: before,
	}

	if err != nil {
		result.Success = false
		result.Stderr = err.Error()
		if exitErr, ok := err.(*exec.ExitError); ok {
			result.Stderr = string(exitErr.Stderr)
		}
	} else {
		result.Success = true
		result.Stdout = strings.TrimSpace(string(stdout))
	}

	// 采集 after state
	time.Sleep(500 * time.Millisecond)
	after := captureServiceState(act.Target.Name)
	result.AfterState = after

	return result, nil
}

func captureServiceState(svcName string) map[string]string {
	out, err := exec.Command(
		"systemctl", "show", svcName,
		"--property=ActiveState",
		"--property=SubState",
		"--property=ActiveEnterTimestamp",
	).Output()
	if err != nil {
		return map[string]string{"error": err.Error()}
	}

	state := map[string]string{}
	for _, line := range strings.Split(string(out), "\n") {
		parts := strings.SplitN(line, "=", 2)
		if len(parts) == 2 {
			state[parts[0]] = strings.TrimSpace(parts[1])
		}
	}
	return state
}

type ActionResult struct {
	ActionID    string
	Success     bool
	Stdout      string
	Stderr      string
	BeforeState map[string]string
	AfterState  map[string]string
}
