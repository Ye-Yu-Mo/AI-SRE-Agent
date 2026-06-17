package graph

import (
	"os"
	"os/exec"
	"strconv"
	"strings"

	"github.com/ai-sre/agent/internal/collector"
)

type Node struct {
	ID     string `json:"id"`
	Kind   string `json:"kind"`
	Name   string `json:"name"`
	Status string `json:"status,omitempty"`
}

type Edge struct {
	From string `json:"from"`
	To   string `json:"to"`
	Type string `json:"type"`
}

type Graph struct {
	Nodes []Node `json:"nodes"`
	Edges []Edge `json:"edges"`
}

// Build 构建服务器的 State Graph
func Build() *Graph {
	g := &Graph{}

	// 1. Docker 容器节点
	containers, _ := collector.DockerList()
	for _, c := range containers {
		id := "container_" + c.Name
		g.Nodes = append(g.Nodes, Node{ID: id, Kind: "Container", Name: c.Name, Status: c.Status})
	}

	// 2. 端口节点 + 容器映射
	ports := collector.ListeningPorts("/proc")
	for _, p := range ports {
		if p.State != "LISTEN" {
			continue
		}
		pid := "port_" + strconv.Itoa(int(p.Port)) + "_" + p.Protocol
		g.Nodes = append(g.Nodes, Node{ID: pid, Kind: "Port", Name: strconv.Itoa(int(p.Port)) + "/" + p.Protocol, Status: p.State})

		// 通过端口匹配容器
		for _, c := range containers {
			if containerHasPort(c, int(p.Port)) {
				g.Edges = append(g.Edges, Edge{From: "container_" + c.Name, To: pid, Type: "listens_on"})
			}
		}
	}

	// 3. 反向代理检测（Nginx / Caddy）
	proxyPorts := detectReverseProxy()
	for _, pp := range proxyPorts {
		pid := "proxy_" + pp.name
		g.Nodes = append(g.Nodes, Node{ID: pid, Kind: "ReverseProxy", Name: pp.name, Status: "running"})

		// 代理 → upstream 端口
		for _, up := range pp.upstreams {
			portID := "port_" + up + "_tcp"
			g.Edges = append(g.Edges, Edge{From: pid, To: portID, Type: "proxies_to"})
		}
	}

	// 4. Docker Compose 项目 → 容器
	composeProjects := detectComposeProjects(containers)
	for proj, ctns := range composeProjects {
		pid := "app_" + proj
		g.Nodes = append(g.Nodes, Node{ID: pid, Kind: "Application", Name: proj})
		for _, cn := range ctns {
			g.Edges = append(g.Edges, Edge{From: pid, To: "container_" + cn, Type: "runs"})
		}
	}

	return g
}

func containerHasPort(c collector.DockerContainer, port int) bool {
	portStr := ":" + strconv.Itoa(port) + "->"
	for _, p := range c.Ports {
		if strings.Contains(p, portStr) || strings.Contains(p, ":"+strconv.Itoa(port)+"/") {
			return true
		}
	}
	return false
}

type proxyInfo struct {
	name      string
	upstreams []string
}

func detectReverseProxy() []proxyInfo {
	var proxies []proxyInfo

	// 检查 Nginx
	for _, path := range []string{"/etc/nginx/nginx.conf", "/etc/nginx/sites-enabled/default", "/etc/nginx/conf.d/default.conf"} {
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		upstreams := parseNginxUpstreams(string(data))
		if len(upstreams) > 0 {
			proxies = append(proxies, proxyInfo{name: "nginx", upstreams: upstreams})
			break
		}
	}

	// 检查 Caddy
	if data, err := os.ReadFile("/etc/caddy/Caddyfile"); err == nil {
		upstreams := parseCaddyUpstreams(string(data))
		if len(upstreams) > 0 {
			proxies = append(proxies, proxyInfo{name: "caddy", upstreams: upstreams})
		}
	}

	return proxies
}

func parseNginxUpstreams(config string) []string {
	var ports []string
	for _, line := range strings.Split(config, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "#") {
			continue
		}
		// proxy_pass http://127.0.0.1:3000;
		if strings.Contains(line, "proxy_pass") {
			parts := strings.Split(line, ":")
			if len(parts) >= 3 {
				port := strings.TrimRight(strings.TrimSpace(parts[len(parts)-1]), "; ")
				if _, err := strconv.Atoi(port); err == nil {
					ports = append(ports, port)
				}
			}
		}
		// upstream block: server 127.0.0.1:3000;
		if strings.Contains(line, "server ") && strings.Contains(line, ":") {
			parts := strings.Split(line, ":")
			if len(parts) >= 3 {
				port := strings.TrimRight(strings.TrimSpace(parts[len(parts)-1]), "; ")
				if _, err := strconv.Atoi(port); err == nil {
					ports = append(ports, port)
				}
			}
		}
	}
	return ports
}

func parseCaddyUpstreams(config string) []string {
	var ports []string
	for _, line := range strings.Split(config, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "#") {
			continue
		}
		// reverse_proxy :3000 或 reverse_proxy localhost:3000
		if strings.Contains(line, "reverse_proxy") {
			parts := strings.Fields(line)
			for i, p := range parts {
				if p == "reverse_proxy" && i+1 < len(parts) {
					target := strings.TrimRight(parts[i+1], "{} ")
					target = strings.TrimPrefix(target, "localhost:")
					target = strings.TrimPrefix(target, "127.0.0.1:")
					target = strings.TrimPrefix(target, ":")
					if _, err := strconv.Atoi(target); err == nil {
						ports = append(ports, target)
					}
				}
			}
		}
	}
	return ports
}

func detectComposeProjects(containers []collector.DockerContainer) map[string][]string {
	projects := map[string][]string{}
	for _, c := range containers {
		parts := strings.SplitN(c.Name, "_", 2)
		if len(parts) >= 1 {
			proj := parts[0]
			projects[proj] = append(projects[proj], c.Name)
		}
	}
	return projects
}

// suppress unused import warning
var _ = exec.Command
