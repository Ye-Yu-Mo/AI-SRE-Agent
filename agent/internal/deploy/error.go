package deploy

import "fmt"

// DeployError 结构化部署错误，替代裸 exit code。
type DeployError struct {
	Code     string `json:"code"`
	Message  string `json:"message"`
	Category string `json:"category"`
	Suggestion string `json:"suggestion"`
	Raw      string `json:"raw,omitempty"`
}

func (e *DeployError) Error() string {
	return fmt.Sprintf("[%s] %s", e.Code, e.Message)
}

// TranslateError 将进程退出码映射为结构化错误。
func TranslateError(exitCode int, rawOutput string) *DeployError {
	e := &DeployError{Raw: rawOutput}
	switch exitCode {
	case 125:
		e.Code = "BUILD_FAILED"
		e.Category = "docker"
		e.Message = "Docker build failed"
		e.Suggestion = "Check Docker daemon is running and disk space is sufficient"
	case 127:
		e.Code = "CMD_NOT_FOUND"
		e.Category = "system"
		e.Message = "Command not found (docker or docker-compose missing)"
		e.Suggestion = "Install Docker: curl -fsSL https://get.docker.com | sh"
	case 128:
		e.Code = "CLONE_FAILED"
		e.Category = "git"
		e.Message = "Git clone failed — repository not found or network unreachable"
		e.Suggestion = "Verify the repo URL and branch name; check network connectivity"
	case 137:
		e.Code = "OOM_KILLED"
		e.Category = "container"
		e.Message = "Container was killed by OOM killer (out of memory)"
		e.Suggestion = "Increase container memory limit or reduce application memory usage"
	case 1:
		e.Code = "UNKNOWN_ERROR"
		e.Category = "unknown"
		e.Message = rawOutput
		e.Suggestion = "Check the raw output for details"
	default:
		e.Code = "UNKNOWN_ERROR"
		e.Category = "unknown"
		e.Message = rawOutput
		e.Suggestion = "Check the raw output for details"
	}
	return e
}
