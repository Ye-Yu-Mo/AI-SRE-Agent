package executor

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"github.com/ai-sre/agent/internal/action"
)

type DockerExecutor struct{}

func (e *DockerExecutor) Execute(ctx context.Context, act action.Action) (*ActionResult, error) {
	var dockerCmd string
	switch act.Type {
	case action.ActionDockerStart:
		dockerCmd = "start"
	case action.ActionDockerStop:
		dockerCmd = "stop"
	case action.ActionDockerRestart:
		dockerCmd = "restart"
	default:
		return nil, fmt.Errorf("docker executor: unsupported action type %s", act.Type)
	}

	before := captureDockerState(act.Target.Name)
	cmd := exec.CommandContext(ctx, "docker", dockerCmd, act.Target.Name)
	stdout, err := cmd.Output()

	result := &ActionResult{
		ActionID:    act.ID,
		BeforeState: before,
	}
	if err != nil {
		result.Success = false
		result.Stderr = err.Error()
	} else {
		result.Success = true
		result.Stdout = strings.TrimSpace(string(stdout))
	}

	time.Sleep(500 * time.Millisecond)
	result.AfterState = captureDockerState(act.Target.Name)
	return result, nil
}

func captureDockerState(name string) map[string]string {
	out, err := exec.Command("docker", "inspect",
		"--format", `{{.State.Status}} {{.State.StartedAt}}`,
		name,
	).Output()
	if err != nil {
		return map[string]string{"error": err.Error()}
	}
	parts := strings.SplitN(strings.TrimSpace(string(out)), " ", 2)
	state := map[string]string{}
	if len(parts) >= 1 {
		state["State"] = parts[0]
	}
	if len(parts) >= 2 {
		state["StartedAt"] = parts[1]
	}
	return state
}
