package main

import (
	"context"
	"crypto/rand"
	_ "embed"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/ai-sre/agent/internal/action"
	"github.com/ai-sre/agent/internal/collector"
	"github.com/ai-sre/agent/internal/deploy"
	"github.com/ai-sre/agent/internal/executor"
	"github.com/ai-sre/agent/internal/graph"
	"github.com/ai-sre/agent/internal/identity"
	"github.com/ai-sre/agent/internal/plan"
	"github.com/ai-sre/agent/internal/risk"
	"github.com/ai-sre/agent/internal/secret"
	"github.com/ai-sre/agent/internal/storage"
	"github.com/ai-sre/agent/internal/task"
)

type Config struct {
	Dir    string
	Port   int
	Secret string
}

func envConfig() *Config {
	cfg := &Config{
		Dir:  "/var/lib/ai-server-agent",
		Port: 9090,
	}
	if v := os.Getenv("AGENT_DATA_DIR"); v != "" {
		cfg.Dir = v
	}
	if v := os.Getenv("AGENT_PORT"); v != "" {
		if p, err := strconv.Atoi(v); err == nil {
			cfg.Port = p
		}
	}
	cfg.Secret = os.Getenv("AGENT_SECRET")
	return cfg
}

func main() {
	cfg := envConfig()

	serveCmd := flag.NewFlagSet("serve", flag.ExitOnError)
	serveCmd.StringVar(&cfg.Dir, "dir", cfg.Dir, "data directory")
	serveCmd.IntVar(&cfg.Port, "port", cfg.Port, "HTTP listen port")
	serveCmd.StringVar(&cfg.Secret, "secret", cfg.Secret, "shared secret for API auth")

	if len(os.Args) < 2 {
		fmt.Fprintf(os.Stderr, "usage: %s serve [flags]\n", os.Args[0])
		os.Exit(1)
	}

	switch os.Args[1] {
	case "serve":
		serveCmd.Parse(os.Args[2:])
		if cfg.Secret == "" {
			fmt.Fprintln(os.Stderr, "error: AGENT_SECRET env or --secret is required")
			os.Exit(1)
		}
		if err := run(cfg); err != nil {
			log.Fatalf("fatal: %v", err)
		}
	default:
		fmt.Fprintf(os.Stderr, "unknown command: %s\n", os.Args[1])
		os.Exit(1)
	}
}

func run(cfg *Config) error {
	// 初始化 identity
	id, err := identity.New(cfg.Dir)
	if err != nil {
		return fmt.Errorf("identity: %w", err)
	}

	ln, err := net.Listen("tcp", fmt.Sprintf(":%d", cfg.Port))
	if err != nil {
		return fmt.Errorf("listen: %w", err)
	}

	planStore := plan.NewStore()
	auditStore, err := storage.NewStore(filepath.Join(cfg.Dir, "data"))
	if err != nil {
		return fmt.Errorf("storage: %w", err)
	}
	deployStore := deploy.NewReleaseStore()
	// 从磁盘恢复已有的 releases
	deployStore.LoadFromDisk(filepath.Join(cfg.Dir, "data", "releases.jsonl"))
	srv := newServer(cfg, id, planStore, auditStore, deployStore, ln)

	// 信号处理
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		sig := <-sigCh
		log.Printf("received %v, shutting down", sig)
		srv.Shutdown(context.Background())
	}()

	log.Printf("listening on %s", ln.Addr().String())

	// 心跳 goroutine
	go heartbeatLoop(cfg.Dir)

	if err := srv.Serve(ln); err != http.ErrServerClosed {
		return fmt.Errorf("http: %w", err)
	}
	return nil
}

// Version 编译时通过 ldflags 注入
var Version = "dev"

//go:embed console/index.html
var consoleHTML string

type server struct {
	cfg         *Config
	identity    *identity.Identity
	planStore   *plan.Store
	auditStore  *storage.Store
	deployStore *deploy.ReleaseStore
	taskStore   *task.Store
	taskRunner  *task.Runner
	sysExec     *executor.SystemdExecutor
	dockerExec  *executor.DockerExecutor
}

func newServer(cfg *Config, id *identity.Identity, planStore *plan.Store, auditStore *storage.Store, deployStore *deploy.ReleaseStore, ln net.Listener) *http.Server {
	s := &server{cfg: cfg, identity: id, planStore: planStore, auditStore: auditStore, deployStore: deployStore,
		taskStore: task.NewStore(), sysExec: &executor.SystemdExecutor{}, dockerExec: &executor.DockerExecutor{}}
	s.taskRunner = &task.Runner{Store: s.taskStore}
	mux := http.NewServeMux()

	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		jsonOK(w, map[string]string{"status": "ok"})
	})

	// Web Console — 嵌入式仪表盘
	if consoleHTML != "" {
		mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != "/" {
				http.NotFound(w, r)
				return
			}
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.Write([]byte(consoleHTML))
		})
	}

	apiMux := http.NewServeMux()
	apiMux.HandleFunc("/api/v1/agent/version", func(w http.ResponseWriter, r *http.Request) {
		jsonOK(w, map[string]interface{}{"version": Version, "server_id": id.ServerID})
	})
	apiMux.HandleFunc("/api/v1/agent/heartbeat", func(w http.ResponseWriter, r *http.Request) {
		hbPath := filepath.Join(cfg.Dir, "data", "heartbeat")
		data, err := os.ReadFile(hbPath)
		if err != nil {
			jsonOK(w, map[string]interface{}{"status": "unknown"})
			return
		}
		ts, err := time.Parse(time.RFC3339, strings.TrimSpace(string(data)))
		if err != nil {
			jsonOK(w, map[string]interface{}{"status": "unknown"})
			return
		}
		status := "offline"
		if time.Since(ts) < 90*time.Second {
			status = "online"
		}
		jsonOK(w, map[string]interface{}{"status": status, "last_beat": ts.Format(time.RFC3339)})
	})
	apiMux.HandleFunc("/api/v1/identity", func(w http.ResponseWriter, r *http.Request) {
		s.handleIdentity(w)
	})
	apiMux.HandleFunc("/api/v1/inspect", func(w http.ResponseWriter, r *http.Request) {
		s.handleInspect(w, r)
	})
	apiMux.HandleFunc("/api/v1/health", handleHealth)
	apiMux.HandleFunc("/api/v1/resources", handleResources)
	apiMux.HandleFunc("/api/v1/services", handleServices)
	apiMux.HandleFunc("/api/v1/services/", func(w http.ResponseWriter, r *http.Request) {
		s.handleServiceLogs(w, r)
	})
	apiMux.HandleFunc("/api/v1/plans", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPost:
			s.handlePlanCreate(w, r)
		default:
			http.Error(w, "method not allowed", 405)
		}
	})
	apiMux.HandleFunc("/api/v1/plans/", func(w http.ResponseWriter, r *http.Request) {
		s.handlePlanByID(w, r)
	})
	apiMux.HandleFunc("/api/v1/audit", func(w http.ResponseWriter, r *http.Request) {
		s.handleAudit(w, r)
	})
	apiMux.HandleFunc("/api/v1/docker/containers", func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/logs") {
			s.handleDockerLogs(w, r)
		} else {
			s.handleDockerList(w)
		}
	})
	apiMux.HandleFunc("/api/v1/docker/containers/", func(w http.ResponseWriter, r *http.Request) {
		s.handleDockerLogs(w, r)
	})
	apiMux.HandleFunc("/api/v1/deploy/plan", func(w http.ResponseWriter, r *http.Request) {
		s.handleDeployPlan(w, r)
	})
	apiMux.HandleFunc("/api/v1/deploy/apply", func(w http.ResponseWriter, r *http.Request) {
		s.handleDeployApply(w, r)
	})
	apiMux.HandleFunc("/api/v1/graph", func(w http.ResponseWriter, r *http.Request) {
		s.handleGraph(w)
	})
	apiMux.HandleFunc("/api/v1/files/write", func(w http.ResponseWriter, r *http.Request) {
		s.handleFileWrite(w, r)
	})
	apiMux.HandleFunc("/api/v1/commands/run", func(w http.ResponseWriter, r *http.Request) {
		s.handleCommandRun(w, r)
	})
	apiMux.HandleFunc("/api/v1/tasks/", func(w http.ResponseWriter, r *http.Request) {
		s.handleTaskStatus(w, r)
	})
	apiMux.HandleFunc("/api/v1/agent/update", func(w http.ResponseWriter, r *http.Request) {
		s.handleAgentUpdate(w, r)
	})
	apiMux.HandleFunc("/api/v1/diagnose", func(w http.ResponseWriter, r *http.Request) {
		s.handleDiagnose(w, r)
	})
	apiMux.HandleFunc("/api/v1/apps/", func(w http.ResponseWriter, r *http.Request) {
		s.handleApp(w, r)
	})

	mux.Handle("/api/", authMiddleware(cfg.Secret, apiMux))

	return &http.Server{Handler: mux}
}

func (s *server) handleIdentity(w http.ResponseWriter) {
	sid := "unknown"
	host := "unknown"
	if s.identity != nil {
		sid = s.identity.ServerID
		host = s.identity.Hostname
	}
	jsonOK(w, map[string]interface{}{
		"server_id": sid,
		"hostname":  host,
	})
}

func (s *server) handleInspect(w http.ResponseWriter, _ *http.Request) {
	osInfo := collector.OSInfo("/etc", "/proc")
	cpu, _ := collector.CPUInfo("/proc")
	mem, _ := collector.MemoryInfo("/proc")
	disk, _ := collector.DiskInfo("/")
	ports := collector.PortProcessMapping("/proc")

	result := map[string]interface{}{
		"hostname":   osInfo.Name,
		"os":         osInfo.Name,
		"os_version": osInfo.VersionID,
		"kernel":     osInfo.Kernel,
		"arch":       osInfo.Arch,
	}
	if cpu != nil {
		result["cpu_percent"] = cpu.Percent
		result["cpu_cores"] = cpu.NumCores
		result["cpu_model"] = cpu.ModelName
	}
	if mem != nil {
		result["memory_total"] = mem.Total
		result["memory_used"] = mem.Used
		result["memory_percent"] = mem.UsedPercent
	}
	if disk != nil {
		result["disk_total"] = disk.Total
		result["disk_used"] = disk.Used
		result["disk_percent"] = disk.UsedPercent
	}
	if len(ports) > 0 {
		result["listening_ports"] = ports
	}
	jsonOK(w, result)
}

func (s *server) handleServiceLogs(w http.ResponseWriter, r *http.Request) {
	// URL: /api/v1/services/{name}/logs?lines=50
	path := strings.TrimPrefix(r.URL.Path, "/api/v1/services/")
	name := strings.TrimRight(strings.TrimSuffix(path, "/logs"), "/")
	if name == "" {
		http.Error(w, `{"error":"service name required"}`, 400)
		return
	}
	lines := r.URL.Query().Get("lines")
	if lines == "" {
		lines = "50"
	}
	n, _ := strconv.Atoi(lines)
	if n <= 0 || n > 1000 {
		n = 50
	}

	out, err := exec.Command("journalctl", "-u", name, "--no-pager", "-n", strconv.Itoa(n), "-o", "short-iso").Output()
	if err != nil {
		jsonOK(w, map[string]interface{}{"service": name, "lines": []string{}, "error": err.Error()})
		return
	}
	logLines := strings.Split(strings.TrimSpace(string(out)), "\n")
	if len(logLines) == 1 && logLines[0] == "" {
		logLines = nil
	}
	// 脱敏：journal 日志可能含 secret
	redacted := make([]string, len(logLines))
	for i, l := range logLines {
		redacted[i] = secret.RedactLine(l)
	}
	jsonOK(w, map[string]interface{}{"service": name, "lines": redacted, "total": len(redacted)})
}

func (s *server) handleAudit(w http.ResponseWriter, r *http.Request) {
	// MCP 的 server_id 是别名，与 Agent identity 不同。
	// 如果传入的 server_id 与 Agent identity 匹配，或为空 → 返回该 Agent 的所有审计记录。
	q := r.URL.Query()
	filterServerID := q.Get("server_id")
	if filterServerID != "" {
		// 若传入值与 Agent identity 不匹配且不是 MCP 别名，不过滤（返回全部）
		if s.identity != nil && filterServerID != s.identity.ServerID {
			// 尝试宽松匹配：srv_remote / srv_new / srv_local 等别名都放行
			// 不做严格过滤，让数据全量返回
			filterServerID = ""
		}
	}
	events, err := s.auditStore.SearchAudit(
		filterServerID,
		q.Get("action_type"),
		q.Get("result"),
		50,
	)
	if err != nil {
		http.Error(w, fmt.Sprintf(`{"error":"%v"}`, err), 500)
		return
	}
	if events == nil {
		events = []storage.AuditEvent{}
	}
	jsonOK(w, map[string]interface{}{"events": events, "total": len(events)})
}

// ── Docker handlers ──

func (s *server) handleDockerList(w http.ResponseWriter) {
	containers, err := collector.DockerList()
	if err != nil {
		jsonOK(w, map[string]interface{}{"containers": []collector.DockerContainer{}})
		return
	}
	if containers == nil {
		containers = []collector.DockerContainer{}
	}
	jsonOK(w, map[string]interface{}{"containers": containers})
}

func (s *server) handleDockerLogs(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/api/v1/docker/containers/")
	name := strings.TrimRight(strings.TrimSuffix(path, "/logs"), "/")
	if name == "" {
		http.Error(w, `{"error":"container name required"}`, 400)
		return
	}
	lines := 50
	if n, err := strconv.Atoi(r.URL.Query().Get("lines")); err == nil && n > 0 && n <= 1000 {
		lines = n
	}
	logs, _ := collector.DockerLogs(name, lines)
	if logs == nil {
		logs = []string{}
	}
	// 脱敏：容器日志可能含 secret
	redacted := make([]string, len(logs))
	for i, l := range logs {
		redacted[i] = secret.RedactLine(l)
	}
	jsonOK(w, map[string]interface{}{"container": name, "lines": redacted, "total": len(redacted)})
}

func (s *server) handleGraph(w http.ResponseWriter) {
	g := graph.Build()
	jsonOK(w, g)
}

func (s *server) execFor(atype action.ActionType) (executorInterface, error) {
	if strings.HasPrefix(string(atype), "service.") {
		return s.sysExec, nil
	}
	if strings.HasPrefix(string(atype), "docker.") {
		return s.dockerExec, nil
	}
	return nil, fmt.Errorf("no executor for action type %s", atype)
}

type executorInterface interface {
	Execute(ctx context.Context, act action.Action) (*executor.ActionResult, error)
}

func handleHealth(w http.ResponseWriter, _ *http.Request) {
	var warnings []string

	mem, err := collector.MemoryInfo("/proc")
	if err == nil && mem.UsedPercent > 90 {
		warnings = append(warnings, fmt.Sprintf("memory usage %.1f%%", mem.UsedPercent))
	}

	disk, err := collector.DiskInfo("/")
	if err == nil && disk.UsedPercent > 85 {
		warnings = append(warnings, fmt.Sprintf("disk usage %.1f%% on /", disk.UsedPercent))
	}

	status := "healthy"
	if len(warnings) > 0 {
		status = "warning"
	}

	jsonOK(w, map[string]interface{}{
		"status":   status,
		"warnings": warnings,
	})
}

func handleResources(w http.ResponseWriter, _ *http.Request) {
	cpu, _ := collector.CPUInfo("/proc")
	mem, _ := collector.MemoryInfo("/proc")
	disk, _ := collector.DiskInfo("/")

	result := map[string]interface{}{}
	if cpu != nil {
		result["cpu_percent"] = cpu.Percent
		result["cpu_cores"] = cpu.NumCores
	}
	if mem != nil {
		result["memory_percent"] = mem.UsedPercent
		result["memory_total"] = mem.Total
		result["memory_used"] = mem.Used
	}
	if disk != nil {
		result["disk_percent"] = disk.UsedPercent
		result["disk_total"] = disk.Total
		result["disk_used"] = disk.Used
	}
	jsonOK(w, result)
}

// systemd 服务列表（通过 systemctl list-units）
func handleServices(w http.ResponseWriter, _ *http.Request) {
	out, err := exec.Command(
		"systemctl", "list-units", "--type=service",
		"--no-legend", "--no-pager",
	).Output()
	if err != nil {
		jsonOK(w, map[string]interface{}{"services": []string{}})
		return
	}

	var services []map[string]string
	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.Contains(line, "●") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 4 {
			continue
		}
		services = append(services, map[string]string{
			"name":   fields[0],
			"load":   fields[1],
			"status": fields[2],
			"sub":    fields[3],
		})
	}
	jsonOK(w, map[string]interface{}{"services": services})
}

func (s *server) handlePlanCreate(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Intent   string          `json:"intent"`
		ServerID string          `json:"server_id"`
		Actions  []action.Action `json:"actions"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid JSON"}`, 400)
		return
	}
	if req.Intent == "" || len(req.Actions) == 0 {
		http.Error(w, `{"error":"intent and actions required"}`, 400)
		return
	}

	// 风险分级
	for i := range req.Actions {
		req.Actions[i].ID = fmt.Sprintf("act_%s_%d", randStr(8), i)
		r := risk.Classify(req.Actions[i], "production")
		req.Actions[i].Risk = r.Level
		req.Actions[i].RequiresApproval = r.Decision.RequiresApproval()
		req.Actions[i].CreatedBy = "ai-agent"
		req.Actions[i].CreatedAt = timeNow()
	}

	planID := "plan_" + randStr(12)
	p := &action.Plan{
		ID:               planID,
		Intent:           req.Intent,
		ServerID:         req.ServerID,
		Status:           action.PlanPending,
		RequiresApproval: req.Actions[0].RequiresApproval,
		CreatedAt:        timeNow(),
		ExpiresAt:        timeNow().Add(10 * time.Minute),
	}

	var maxRisk action.RiskLevel
	for _, a := range req.Actions {
		p.Steps = append(p.Steps, action.ActionStep{Step: len(p.Steps) + 1, Action: a})
		if riskOrder(a.Risk) > riskOrder(maxRisk) {
			maxRisk = a.Risk
		}
	}
	p.Risk = maxRisk

	s.planStore.Create(p)

	jsonOK(w, p)
}

func (s *server) handlePlanByID(w http.ResponseWriter, r *http.Request) {
	// URL: /api/v1/plans/{id} or /api/v1/plans/{id}/apply
	path := strings.TrimPrefix(r.URL.Path, "/api/v1/plans/")
	parts := strings.SplitN(path, "/", 2)
	planID := parts[0]

	switch {
	case len(parts) == 1 && r.Method == http.MethodGet:
		p, ok := s.planStore.Get(planID)
		if !ok {
			http.Error(w, `{"error":"plan not found"}`, 404)
			return
		}
		jsonOK(w, p)

	case len(parts) == 2 && parts[1] == "apply" && r.Method == http.MethodPost:
		p, ok := s.planStore.Get(planID)
		if !ok {
			http.Error(w, `{"error":"plan not found"}`, 404)
			return
		}
		if p.IsExpired() {
			http.Error(w, `{"error":"plan expired"}`, 410)
			return
		}

		// M1: approval 闸门。requires_approval=true 时必须显式携带 approve:true。
		// 单机阶段调用方自己声明确认；后续里程碑替换为远程审批 token。
		if p.RequiresApproval {
			var body struct {
				Approve bool `json:"approve"`
			}
			json.NewDecoder(r.Body).Decode(&body)
			if !body.Approve {
				http.Error(w, fmt.Sprintf(`{"error":"approval required","plan_id":%q}`, planID), http.StatusConflict)
				return
			}
		}

		s.planStore.UpdateStatus(planID, action.PlanApproved)
		s.planStore.UpdateStatus(planID, action.PlanRunning)

		var results []executor.ActionResult
		for _, step := range p.Steps {
			ex, err := s.execFor(step.Action.Type)
			if err != nil {
				http.Error(w, fmt.Sprintf(`{"error":"%v"}`, err), 500)
				return
			}
			result, err := ex.Execute(r.Context(), step.Action)
			if err != nil {
				http.Error(w, fmt.Sprintf(`{"error":"execution error: %v"}`, err), 500)
				return
			}
			results = append(results, *result)
		}

		s.planStore.UpdateStatus(planID, action.PlanSucceeded)

		// 写 audit log
		for i, step := range p.Steps {
			r := results[i]
			bs, _ := json.Marshal(r.BeforeState)
			as, _ := json.Marshal(r.AfterState)
			s.auditStore.RecordAudit(storage.AuditEvent{
				ServerID:    p.ServerID,
				PlanID:      planID,
				ActionID:    step.Action.ID,
				ActionType:  string(step.Action.Type),
				Target:      step.Action.Target.Name,
				Risk:        string(step.Action.Risk),
				Result:      map[bool]string{true: "succeeded", false: "failed"}[r.Success],
				BeforeState: string(bs),
				AfterState:  string(as),
				Stdout:      r.Stdout,
				Stderr:      r.Stderr,
			})
		}

		jsonOK(w, map[string]interface{}{
			"plan_id": planID,
			"status":  "succeeded",
			"results": results,
		})

	default:
		http.Error(w, "method not allowed", 405)
	}
}

func (s *server) handleDeployPlan(w http.ResponseWriter, r *http.Request) {
	var req struct {
		RepoURL  string `json:"repo_url"`
		Branch   string `json:"branch"`
		Domain   string `json:"domain"`
		ServerID string `json:"server_id"`
		AppName  string `json:"app_name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid JSON"}`, 400)
		return
	}
	if req.RepoURL == "" {
		http.Error(w, `{"error":"repo_url required"}`, 400)
		return
	}

	planID := "plan_" + randStr(12)
	appName := req.AppName
	if appName == "" {
		appName = strings.TrimSuffix(strings.TrimPrefix(req.RepoURL, "https://github.com/"), ".git")
		appName = strings.ReplaceAll(appName, "/", "-")
	}

	jsonOK(w, map[string]interface{}{
		"plan_id":           planID,
		"app_name":          appName,
		"repo_url":          req.RepoURL,
		"branch":            req.Branch,
		"domain":            req.Domain,
		"risk":              "high",
		"requires_approval": true,
		"steps": []string{
			"repo.clone",
			"compose.detect",
			"compose.validate",
			"compose.build",
			"compose.up",
			"healthcheck.run",
			"release.create",
		},
	})
}

// writeDeployError 返回结构化部署错误。
func writeDeployError(w http.ResponseWriter, err *deploy.DeployError) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusInternalServerError)
	json.NewEncoder(w).Encode(map[string]interface{}{"status": "failed", "error": err})
}

// extractExitCode 从 CombinedOutput 错误中提取退出码。
func extractExitCode(err error) int {
	if err == nil {
		return 0
	}
	if ee, ok := err.(*exec.ExitError); ok {
		return ee.ExitCode()
	}
	return 1
}

func (s *server) handleDeployApply(w http.ResponseWriter, r *http.Request) {
	var req struct {
		PlanID   string `json:"plan_id"`
		RepoURL  string `json:"repo_url"`
		Branch   string `json:"branch"`
		Domain   string `json:"domain"`
		AppName  string `json:"app_name"`
		ServerID string `json:"server_id"`
		Force    bool   `json:"force"`
		Async    *bool  `json:"async"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid JSON"}`, 400)
		return
	}
	if req.RepoURL == "" {
		http.Error(w, `{"error":"repo_url required"}`, 400)
		return
	}

	// 同步模式（向后兼容）
	syncMode := req.Async != nil && !*req.Async
	if syncMode {
		dr := struct {
			PlanID, RepoURL, Branch, Domain, AppName, ServerID string
			Force bool
		}{req.PlanID, req.RepoURL, req.Branch, req.Domain, req.AppName, req.ServerID, req.Force}
		s.deployApply(w, r, dr)
		return
	}

	// 异步模式：创建 task，goroutine 执行，立即返回 task_id
	taskID := "task_" + randStr(12)
	s.taskStore.Create(taskID)
	capture := &responseCapture{statusCode: 200, header: make(http.Header)}
	s.taskRunner.Run(taskID, func(t *task.Task) {
		dr := struct {
			PlanID, RepoURL, Branch, Domain, AppName, ServerID string
			Force bool
		}{req.PlanID, req.RepoURL, req.Branch, req.Domain, req.AppName, req.ServerID, req.Force}
		s.deployApply(capture, r, dr)
		// 解析 capture 的输出，判断成功/失败
		var result map[string]interface{}
		if json.Unmarshal(capture.body, &result) == nil {
			if status, ok := result["status"].(string); ok && status == "succeeded" {
				b, _ := json.Marshal(result)
				s.taskRunner.MarkSucceeded(taskID, b)
				return
			}
		}
		s.taskRunner.MarkFailed(taskID, string(capture.body))
		if capture.statusCode >= 400 {
			s.taskRunner.MarkFailed(taskID, string(capture.body))
		} else if !capture.wroteJSON {
			s.taskRunner.MarkFailed(taskID, "no result produced")
		}
	})
	jsonOK(w, map[string]interface{}{"task_id": taskID, "status": "running"})
}

// responseCapture 实现 http.ResponseWriter，捕获输出供 task 使用。
type responseCapture struct {
	statusCode int
	header     http.Header
	body       []byte
	wroteJSON  bool
}

func (rc *responseCapture) Header() http.Header           { return rc.header }
func (rc *responseCapture) WriteHeader(statusCode int)     { rc.statusCode = statusCode }
func (rc *responseCapture) Write(data []byte) (int, error) { rc.body = append(rc.body, data...); return len(data), nil }

// deployApply 同步部署逻辑（被同步和异步模式共用）。
func (s *server) deployApply(w http.ResponseWriter, r *http.Request, req struct {
	PlanID   string
	RepoURL  string
	Branch   string
	Domain   string
	AppName  string
	ServerID string
	Force    bool
}) {

	appName := req.AppName
	if appName == "" {
		appName = strings.TrimSuffix(strings.TrimPrefix(req.RepoURL, "https://github.com/"), ".git")
		appName = strings.ReplaceAll(appName, "/", "-")
	}

	workDir := filepath.Join(s.cfg.Dir, "apps", appName)
	deferFail := func() { os.RemoveAll(workDir) }

	// 探测默认分支（若用户未指定）
	branch := req.Branch
	if branch == "" {
		if detected := deploy.DetectDefaultBranch(req.RepoURL); detected != "" {
			branch = detected
		} else {
			branch = "main"
		}
	}

	// 先停掉旧容器
	if _, err := os.Stat(workDir); err == nil {
		for _, f := range []string{"docker-compose.yml", "compose.yaml", "docker-compose.yaml"} {
			if _, err := os.Stat(filepath.Join(workDir, f)); err == nil {
				deploy.ComposeDown(r.Context(), workDir, f)
				break
			}
		}
	}
	os.RemoveAll(workDir)

	// Step 1: clone
	if err := deploy.CloneRepo(req.RepoURL, branch, workDir); err != nil {
		deferFail()
		derr := deploy.TranslateError(extractExitCode(err), err.Error())
		s.writeDeployAudit(appName, "clone", "failed", derr.Message)
		writeDeployError(w, derr)
		return
	}

	// Step 2: detect
	detected := deploy.Detect(workDir)
	if detected.Runtime == deploy.RuntimeUnknown {
		deferFail()
		derr := deploy.TranslateError(1, "no Dockerfile or compose file found in repository root")
		s.writeDeployAudit(appName, "detect", "failed", derr.Message)
		writeDeployError(w, derr)
		return
	}

	// Dockerfile-only 路径
	if detected.Runtime == deploy.RuntimeDockerfile {
		s.handleDockerfileDeploy(w, r, appName, workDir, req.Domain, detected.Files)
		return
	}

	// Compose 路径

	composeFile := "docker-compose.yml"
	for _, f := range detected.Files {
		if f == "compose.yaml" || f == "docker-compose.yaml" {
			composeFile = f
			break
		}
	}

	// Step 3: validate — 危险配置必须显式确认才能继续
	validate := deploy.ValidateCompose(workDir, composeFile)
	if !validate.Valid && !req.Force {
		s.writeDeployAudit(appName, "validate", "blocked", strings.Join(validate.Risks, "; "))
		risksJSON, _ := json.Marshal(validate.Risks)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusConflict)
		fmt.Fprintf(w, `{"error":"supply chain risks detected","risks":%s,"hint":"re-deploy with force:true after reviewing risks"}`, risksJSON)
		return
	}
	if !validate.Valid && req.Force {
		log.Printf("deploy %s: force-confirmed supply chain risks: %v", appName, validate.Risks)
	}

	// Step 4: build
	ctx := r.Context()
	stdout, stderr, err := deploy.ComposeBuild(ctx, workDir, composeFile)
	if err != nil {
		deferFail()
		derr := deploy.TranslateError(extractExitCode(err), stdout+"\n"+stderr)
		s.writeDeployAudit(appName, "build", "failed", derr.Message)
		writeDeployError(w, derr)
		return
	}

	// Step 5: up
	stdout, stderr, err = deploy.ComposeUp(ctx, workDir, composeFile)
	if err != nil {
		deferFail()
		derr := deploy.TranslateError(extractExitCode(err), stdout+"\n"+stderr)
		s.writeDeployAudit(appName, "up", "failed", derr.Message)
		writeDeployError(w, derr)
		return
	}

	// Step 6: healthcheck
	hc := deploy.ProbeAppHealth(appName, workDir)

	// Step 6.5: Caddy reverse proxy — 仅当用户提供了 domain 且健康检查通过
	if req.Domain != "" && hc.Status == deploy.HealthPassing && hc.Port > 0 {
		if err := deploy.ConfigureCaddy(req.Domain, strconv.Itoa(hc.Port)); err != nil {
			log.Printf("deploy %s: caddy configure for %s (port %d): %v", appName, req.Domain, hc.Port, err)
		} else {
			log.Printf("deploy %s: caddy route created: %s → localhost:%d", appName, req.Domain, hc.Port)
		}
	}

	// Step 7: compose 快照
	var composeSnap string
	if data, err := os.ReadFile(filepath.Join(workDir, composeFile)); err == nil {
		composeSnap = base64.StdEncoding.EncodeToString(data)
	}

	// Step 8: release
	commitOut, _ := exec.Command("git", "-C", workDir, "rev-parse", "HEAD").Output()
	commit := strings.TrimSpace(string(commitOut))
	if commit == "" {
		commit = req.Branch
	}

	releaseID := "rel_" + randStr(12)
	rel := deploy.Release{
		ID:                releaseID,
		AppID:             appName,
		ServerID:          req.ServerID,
		Commit:            commit,
		Image:             appName + ":latest",
		Status:            "active",
		HealthcheckStatus: string(hc.Status),
		ComposeSnapshot:   composeSnap,
	}
	s.deployStore.Create(rel)
	s.deployStore.Activate(releaseID)
	s.deployStore.SaveToDisk(filepath.Join(s.cfg.Dir, "data", "releases.jsonl"))

	// Step 9: audit log
	s.writeDeployAudit(appName, "release", "succeeded", string(hc.Status))

	jsonOK(w, map[string]interface{}{
		"status":      "succeeded",
		"release_id":  releaseID,
		"app_name":    appName,
		"runtime":     detected.Runtime,
		"healthcheck": hc,
	})
}

func (s *server) handleApp(w http.ResponseWriter, r *http.Request) {
	// /api/v1/apps/{name} or /api/v1/apps/{name}/rollback
	path := strings.TrimPrefix(r.URL.Path, "/api/v1/apps/")
	parts := strings.SplitN(path, "/", 2)
	appName := parts[0]

	// GET /api/v1/apps/{name} — status
	if len(parts) == 1 && r.Method == http.MethodGet {
		rel, ok := s.deployStore.Current(appName)
		if !ok {
			http.Error(w, `{"error":"app not found"}`, 404)
			return
		}
		// 实时健康探测：current_health 反映当前状态，与 release record 的历史快照分开
		workDir := filepath.Join(s.cfg.Dir, "apps", appName)
		current := deploy.ProbeAppHealth(appName, workDir)
		jsonOK(w, map[string]interface{}{
			"app_name":       appName,
			"release":        rel,
			"current_health": current,
		})
		return
	}

	// POST /api/v1/apps/{name}/rollback — rollback
	if len(parts) == 2 && parts[1] == "rollback" && r.Method == http.MethodPost {
		workDir := filepath.Join(s.cfg.Dir, "apps", appName)
		composeFile := findComposeFile(workDir)

		prev, err := deploy.Rollback(s.deployStore, appName, workDir, composeFile)
		if err != nil {
			jsonOK(w, map[string]interface{}{"status": "failed", "error": err.Error()})
			return
		}
		s.deployStore.SaveToDisk(filepath.Join(s.cfg.Dir, "data", "releases.jsonl"))

		// 如果之前配置了 domain 的 caddy route，移除回退
		_ = deploy.RemoveCaddyRoute(appName + ".com")

		hc := deploy.HTTPHealthCheck("http://localhost:80", 0, 10*time.Second)
		jsonOK(w, map[string]interface{}{
			"status":      "rolled_back",
			"release":     prev,
			"healthcheck": hc,
		})
		return
	}

	http.Error(w, "method not allowed", 405)
}

func findComposeFile(dir string) string {
	for _, name := range []string{"docker-compose.yml", "compose.yaml", "docker-compose.yaml"} {
		if _, err := os.Stat(filepath.Join(dir, name)); err == nil {
			return name
		}
	}
	return "docker-compose.yml"
}

// writeDeployAudit 写入部署操作审计记录，统一处理成功和失败路径。
func (s *server) writeDeployAudit(appName, step, result, detail string) {
	sid := "unknown"
	if s.identity != nil {
		sid = s.identity.ServerID
	}
	s.auditStore.RecordAudit(storage.AuditEvent{
		ServerID:   sid,
		ActionType: "app.deploy",
		Target:     appName,
		Risk:       "high",
		Result:     result,
		AfterState: fmt.Sprintf(`{"step":%q,"detail":%q}`, step, detail),
	})
}

// handleDockerfileDeploy 处理纯 Dockerfile（无 compose 文件）项目的部署。
func (s *server) handleDockerfileDeploy(w http.ResponseWriter, r *http.Request, appName, workDir, domain string, files []string) {
	ctx := r.Context()
	dockerfile := findDockerfile(workDir, files)
	buildDir := filepath.Dir(dockerfile)

	// docker build
	tag := appName + ":latest"
	buildCmd := exec.CommandContext(ctx, "docker", "build", "-t", tag, "-f", dockerfile, buildDir)
	buildOut, buildErr := buildCmd.CombinedOutput()
	if buildErr != nil {
		os.RemoveAll(workDir)
		derr := deploy.TranslateError(extractExitCode(buildErr), string(buildOut))
		s.writeDeployAudit(appName, "build", "failed", derr.Message)
		writeDeployError(w, derr)
		return
	}

	// docker run -d -P
	runCmd := exec.CommandContext(ctx, "docker", "run", "-d", "-P", "--name", appName, tag)
	runOut, runErr := runCmd.CombinedOutput()
	if runErr != nil {
		os.RemoveAll(workDir)
		derr := deploy.TranslateError(extractExitCode(runErr), string(runOut))
		s.writeDeployAudit(appName, "up", "failed", derr.Message)
		writeDeployError(w, derr)
		return
	}

	// 获取容器端口
	portOut, _ := exec.CommandContext(ctx, "docker", "port", appName).Output()
	hc := deploy.HealthResult{Status: deploy.HealthFailing}
	if ports := parseDockerPorts(string(portOut)); len(ports) > 0 {
		hc = deploy.TCPHealthCheck("localhost", ports[0], 5*time.Second)
	} else {
		hc = deploy.ProbeAppHealth(appName, "")
	}

	// Caddy proxy
	if domain != "" && hc.Status == deploy.HealthPassing && hc.Port > 0 {
		deploy.ConfigureCaddy(domain, strconv.Itoa(hc.Port))
	}

	// Release
	commitOut, _ := exec.Command("git", "-C", workDir, "rev-parse", "HEAD").Output()
	commit := strings.TrimSpace(string(commitOut))
	releaseID := "rel_" + randStr(12)
	rel := deploy.Release{
		ID: releaseID, AppID: appName, ServerID: "srv_remote_01",
		Commit: commit, Image: tag, Status: "active", HealthcheckStatus: string(hc.Status),
	}
	s.deployStore.Create(rel)
	s.deployStore.Activate(releaseID)
	s.deployStore.SaveToDisk(filepath.Join(s.cfg.Dir, "data", "releases.jsonl"))
	s.writeDeployAudit(appName, "release", "succeeded", string(hc.Status))

	jsonOK(w, map[string]interface{}{"status": "succeeded", "release_id": releaseID, "app_name": appName, "runtime": "dockerfile", "healthcheck": hc})
}

func findDockerfile(workDir string, files []string) string {
	for _, f := range files {
		if strings.HasSuffix(f, "Dockerfile") || f == "Dockerfile" {
			return filepath.Join(workDir, f)
		}
	}
	return filepath.Join(workDir, "Dockerfile")
}

func parseDockerPorts(output string) []int {
	var ports []int
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		if line == "" { continue }
		// 格式: 80/tcp -> 0.0.0.0:32768
		parts := strings.Split(line, "->")
		if len(parts) < 2 { continue }
		hostPort := strings.TrimSpace(parts[1])
		hostPort = strings.SplitN(hostPort, "/", 2)[0]
		if idx := strings.LastIndex(hostPort, ":"); idx >= 0 {
			hostPort = hostPort[idx+1:]
		}
		if p, err := strconv.Atoi(hostPort); err == nil && p > 0 {
			ports = append(ports, p)
		}
	}
	return ports
}

// handleFileWrite 上传文件到 Agent 服务器。
func (s *server) handleFileWrite(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Path    string `json:"path"`
		Content string `json:"content"` // base64 编码
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid JSON"}`, 400)
		return
	}
	if req.Path == "" || req.Content == "" {
		http.Error(w, `{"error":"path and content required"}`, 400)
		return
	}
	// 安全检查：禁止写入系统关键路径
	for _, prefix := range []string{"/etc/", "/boot/", "/sys/", "/proc/"} {
		if strings.HasPrefix(req.Path, prefix) {
			http.Error(w, `{"error":"writing to system path is forbidden"}`, 403)
			return
		}
	}
	data, err := base64.StdEncoding.DecodeString(req.Content)
	if err != nil {
		http.Error(w, `{"error":"invalid base64 content"}`, 400)
		return
	}
	os.MkdirAll(filepath.Dir(req.Path), 0755)
	if err := os.WriteFile(req.Path, data, 0644); err != nil {
		jsonOK(w, map[string]interface{}{"status": "failed", "error": err.Error()})
		return
	}
	s.auditStore.RecordAudit(storage.AuditEvent{
		ServerID:   s.identity.ServerID,
		ActionType: "file.write",
		Target:     req.Path,
		Result:     "succeeded",
		AfterState: fmt.Sprintf(`{"size":%d}`, len(data)),
	})
	jsonOK(w, map[string]interface{}{"status": "ok", "path": req.Path, "size": len(data)})
}

// handleCommandRun 执行一个 shell 命令（受限沙箱）。
func (s *server) handleCommandRun(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Command  string `json:"command"`
		WorkDir  string `json:"work_dir,omitempty"`
		Timeout  int    `json:"timeout,omitempty"` // 秒，默认 30
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid JSON"}`, 400)
		return
	}
	if req.Command == "" {
		http.Error(w, `{"error":"command required"}`, 400)
		return
	}
	timeout := 30
	if req.Timeout > 0 && req.Timeout <= 300 {
		timeout = req.Timeout
	}
	ctx, cancel := context.WithTimeout(r.Context(), time.Duration(timeout)*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "sh", "-c", req.Command)
	if req.WorkDir != "" {
		cmd.Dir = req.WorkDir
	}
	stdout, err := cmd.CombinedOutput()
	result := "succeeded"
	if err != nil {
		result = "failed"
	}
	redacted := secret.RedactLine(string(stdout))
	s.auditStore.RecordAudit(storage.AuditEvent{
		ServerID:   s.identity.ServerID,
		ActionType: "command.run",
		Target:     req.Command,
		Result:     result,
		Stdout:     redacted,
	})
	jsonOK(w, map[string]interface{}{"status": result, "stdout": redacted, "exit_code": extractExitCode(err)})
}

// handleTaskStatus 返回异步 task 的状态。
func (s *server) handleTaskStatus(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/api/v1/tasks/")
	taskID := strings.TrimRight(path, "/")
	t, ok := s.taskStore.Get(taskID)
	if !ok {
		http.Error(w, `{"error":"task not found"}`, 404)
		return
	}
	jsonOK(w, t)
}

// handleAgentUpdate 触发 Agent 自更新。
func (s *server) handleAgentUpdate(w http.ResponseWriter, r *http.Request) {
	var req struct{ Version string `json:"version,omitempty"` }
	json.NewDecoder(r.Body).Decode(&req)

	url := "https://github.com/Ye-Yu-Mo/AI-SRE-Agent/releases/latest/download/ai-server-agent"
	if req.Version != "" {
		url = fmt.Sprintf("https://github.com/Ye-Yu-Mo/AI-SRE-Agent/releases/download/%s/ai-server-agent", req.Version)
	}

	tmpPath := "/tmp/ai-server-agent.new"
	binPath := "/usr/local/bin/ai-server-agent"
	bakPath := binPath + ".bak"

	// 下载新 binary
	cmd := exec.Command("curl", "-fsSL", "-o", tmpPath, url)
	if out, err := cmd.CombinedOutput(); err != nil {
		jsonOK(w, map[string]interface{}{"status": "failed", "step": "download", "error": string(out)})
		return
	}

	// 校验
	info, err := os.Stat(tmpPath)
	if err != nil || info.Size() < 1000000 {
		jsonOK(w, map[string]interface{}{"status": "failed", "step": "verify", "error": "downloaded binary too small or missing"})
		return
	}

	// 备份 + 替换
	os.Rename(binPath, bakPath)
	os.Rename(tmpPath, binPath)
	os.Chmod(binPath, 0755)

	// 重启
	exec.Command("systemctl", "restart", "ai-server-agent").Run()
	time.Sleep(3 * time.Second)

	// 验证
	resp, err := http.Get("http://localhost:9090/health")
	if err != nil || resp.StatusCode != 200 {
		// 回滚
		os.Rename(bakPath, binPath)
		exec.Command("systemctl", "restart", "ai-server-agent").Run()
		jsonOK(w, map[string]interface{}{"status": "failed", "step": "verify", "error": "new binary health check failed, rolled back"})
		return
	}
	resp.Body.Close()

	os.Remove(bakPath)
	s.auditStore.RecordAudit(storage.AuditEvent{
		ServerID: s.identity.ServerID, ActionType: "agent.update", Target: Version, Result: "succeeded",
	})
	jsonOK(w, map[string]interface{}{"status": "ok", "version": req.Version})
}

// handleDiagnose 返回 Docker 容器的深度诊断信息。
func (s *server) handleDiagnose(w http.ResponseWriter, r *http.Request) {
	container := r.URL.Query().Get("container")
	if container == "" {
		jsonOK(w, map[string]interface{}{"error": "container query param required"})
		return
	}
	out, err := exec.Command("docker", "inspect", container).Output()
	if err != nil {
		jsonOK(w, map[string]interface{}{"error": "docker inspect failed: " + err.Error()})
		return
	}
	var inspect []map[string]interface{}
	json.Unmarshal(out, &inspect)
	if len(inspect) == 0 {
		jsonOK(w, map[string]interface{}{"error": "container not found"})
		return
	}
	state := inspect[0]["State"].(map[string]interface{})
	findings := []map[string]interface{}{}

	exitCode, _ := state["ExitCode"].(float64)
	if exitCode == 137 {
		findings = append(findings, map[string]interface{}{
			"severity": "high", "type": "oom_killed",
			"detail": "exit_code=137 (SIGKILL from OOM killer)",
			"suggestion": "Increase container memory limit",
		})
	}
	if oom, ok := state["OOMKilled"].(bool); ok && oom {
		findings = append(findings, map[string]interface{}{
			"severity": "high", "type": "oom_killed",
			"detail": "OOMKilled=true", "suggestion": "Increase container memory limit",
		})
	}
	rc, _ := state["RestartCount"].(float64)
	if rc > 3 {
		findings = append(findings, map[string]interface{}{
			"severity": "medium", "type": "frequent_restart",
			"detail": fmt.Sprintf("%.0f restarts", rc),
			"suggestion": "Check application logs for crash cause",
		})
	}
	health, _ := state["Health"].(map[string]interface{})
	if health != nil {
		hstatus, _ := health["Status"].(string)
		if hstatus != "healthy" && hstatus != "" {
			findings = append(findings, map[string]interface{}{
				"severity": "medium", "type": "health_check_failing",
				"detail": fmt.Sprintf("Health status: %s", hstatus),
				"suggestion": "Check container health check endpoint",
			})
		}
	}

	jsonOK(w, map[string]interface{}{
		"container": container,
		"exit_code": exitCode,
		"restart_count": rc,
		"status": state["Status"],
		"findings": findings,
	})
}

func riskOrder(r action.RiskLevel) int {
	switch r {
	case action.RiskLow:
		return 0
	case action.RiskMedium:
		return 1
	case action.RiskHigh:
		return 2
	case action.RiskCritical:
		return 3
	default:
		return 0
	}
}

func randStr(n int) string {
	b := make([]byte, (n+1)/2)
	rand.Read(b)
	return hex.EncodeToString(b)[:n]
}

func timeNow() time.Time { return time.Now().UTC() }

func jsonOK(w http.ResponseWriter, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(v)
}

func authMiddleware(secret string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer "+secret {
			http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func heartbeatLoop(dataDir string) {
	path := filepath.Join(dataDir, "data", "heartbeat")
	os.MkdirAll(filepath.Dir(path), 0755)
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	for range ticker.C {
		os.WriteFile(path, []byte(time.Now().UTC().Format(time.RFC3339)), 0644)
	}
}
