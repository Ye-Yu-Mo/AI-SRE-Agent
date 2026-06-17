package main

import (
	"context"
	"crypto/rand"
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
	"github.com/ai-sre/agent/internal/storage"
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
	if _, err := identity.New(cfg.Dir); err != nil {
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
	srv := newServer(cfg, planStore, auditStore, deployStore, ln)

	// 信号处理
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		sig := <-sigCh
		log.Printf("received %v, shutting down", sig)
		srv.Shutdown(context.Background())
	}()

	log.Printf("listening on %s", ln.Addr().String())
	if err := srv.Serve(ln); err != http.ErrServerClosed {
		return fmt.Errorf("http: %w", err)
	}
	return nil
}

type server struct {
	cfg         *Config
	planStore   *plan.Store
	auditStore  *storage.Store
	deployStore *deploy.ReleaseStore
	sysExec     *executor.SystemdExecutor
	dockerExec  *executor.DockerExecutor
}

func newServer(cfg *Config, planStore *plan.Store, auditStore *storage.Store, deployStore *deploy.ReleaseStore, ln net.Listener) *http.Server {
	s := &server{cfg: cfg, planStore: planStore, auditStore: auditStore, deployStore: deployStore,
		sysExec: &executor.SystemdExecutor{}, dockerExec: &executor.DockerExecutor{}}
	mux := http.NewServeMux()

	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		jsonOK(w, map[string]string{"status": "ok"})
	})

	apiMux := http.NewServeMux()
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
	apiMux.HandleFunc("/api/v1/apps/", func(w http.ResponseWriter, r *http.Request) {
		s.handleApp(w, r)
	})

	mux.Handle("/api/", authMiddleware(cfg.Secret, apiMux))

	return &http.Server{Handler: mux}
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
	jsonOK(w, map[string]interface{}{"service": name, "lines": logLines, "total": len(logLines)})
}

func (s *server) handleAudit(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	events, err := s.auditStore.SearchAudit(
		q.Get("server_id"),
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
	jsonOK(w, map[string]interface{}{"container": name, "lines": logs, "total": len(logs)})
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

func (s *server) handleDeployApply(w http.ResponseWriter, r *http.Request) {
	var req struct {
		PlanID   string `json:"plan_id"`
		RepoURL  string `json:"repo_url"`
		Branch   string `json:"branch"`
		Domain   string `json:"domain"`
		AppName  string `json:"app_name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid JSON"}`, 400)
		return
	}

	appName := req.AppName
	if appName == "" {
		appName = strings.TrimSuffix(strings.TrimPrefix(req.RepoURL, "https://github.com/"), ".git")
		appName = strings.ReplaceAll(appName, "/", "-")
	}

	workDir := filepath.Join(s.cfg.Dir, "apps", appName)

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
	if err := deploy.CloneRepo(req.RepoURL, req.Branch, workDir); err != nil {
		jsonOK(w, map[string]interface{}{"status": "failed", "step": "clone", "error": err.Error()})
		return
	}

	// Step 2: detect
	detected := deploy.Detect(workDir)
	if detected.Runtime == deploy.RuntimeUnknown {
		jsonOK(w, map[string]interface{}{"status": "failed", "step": "detect", "error": "no Dockerfile or compose file found"})
		return
	}

	composeFile := "docker-compose.yml"
	for _, f := range detected.Files {
		if f == "compose.yaml" || f == "docker-compose.yaml" {
			composeFile = f
			break
		}
	}

	// Step 3: validate
	validate := deploy.ValidateCompose(workDir, composeFile)
	if len(validate.Risks) > 0 {
		// 有风险但仍然继续
		log.Printf("deploy risks for %s: %v", appName, validate.Risks)
	}

	// Step 4: build
	ctx := r.Context()
	_, _, err := deploy.ComposeBuild(ctx, workDir, composeFile)
	if err != nil {
		jsonOK(w, map[string]interface{}{"status": "failed", "step": "build", "error": err.Error()})
		return
	}

	// Step 5: up
	_, _, err = deploy.ComposeUp(ctx, workDir, composeFile)
	if err != nil {
		jsonOK(w, map[string]interface{}{"status": "failed", "step": "up", "error": err.Error()})
		return
	}

	// Step 6: healthcheck — 尝试多个常见端口
	hc := deploy.HTTPHealthCheck("http://localhost:80", 0, 3*time.Second)
	if hc.Status != deploy.HealthPassing {
		for _, port := range []int{8080, 8888, 3000, 5000} {
			url := fmt.Sprintf("http://localhost:%d", port)
			if hc2 := deploy.HTTPHealthCheck(url, 0, 3*time.Second); hc2.Status == deploy.HealthPassing {
				hc = hc2
				break
			}
		}
	}

	// Step 7: release — 即使 healthcheck 失败也创建（记录状态供后续排查）
	// 读取当前 commit hash
	commitOut, _ := exec.Command("git", "-C", workDir, "rev-parse", "HEAD").Output()
	commit := strings.TrimSpace(string(commitOut))
	if commit == "" {
		commit = req.Branch
	}

	releaseID := "rel_" + randStr(12)
	rel := deploy.Release{
		ID:                releaseID,
		AppID:             appName,
		ServerID:          "srv_remote_01",
		Commit:            commit,
		Image:             appName + ":latest",
		Status:            "active",
		HealthcheckStatus: string(hc.Status),
	}
	s.deployStore.Create(rel)
	s.deployStore.Activate(releaseID)
	s.deployStore.SaveToDisk(filepath.Join(s.cfg.Dir, "data", "releases.jsonl"))

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
		jsonOK(w, map[string]interface{}{"app_name": appName, "release": rel})
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
