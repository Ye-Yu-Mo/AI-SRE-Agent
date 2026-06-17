package collector

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os/exec"
	"strings"
)

type DockerContainer struct {
	Name   string   `json:"name"`
	Image  string   `json:"image"`
	Status string   `json:"status"`
	Ports  []string `json:"ports,omitempty"`
}

// DockerList 返回所有 Docker 容器列表
func DockerList() ([]DockerContainer, error) {
	cmd := exec.Command("docker", "ps", "-a",
		"--format", `{"Names":"{{.Names}}","Image":"{{.Image}}","Status":"{{.Status}}","Ports":"{{.Ports}}","CreatedAt":"{{.CreatedAt}}"}`,
	)
	out, err := cmd.Output()
	if err != nil {
		return nil, err
	}
	return parseDockerPS(strings.NewReader(string(out))), nil
}

// DockerLogs 返回指定容器的最近 N 行日志
func DockerLogs(containerName string, lines int) ([]string, error) {
	cmd := exec.Command("docker", "logs", "--tail", fmt.Sprint(lines), containerName)
	out, _ := cmd.Output()
	logLines := strings.Split(strings.TrimSpace(string(out)), "\n")
	if len(logLines) == 1 && logLines[0] == "" {
		return nil, nil
	}
	return logLines, nil
}

func parseDockerPS(r io.Reader) []DockerContainer {
	var containers []DockerContainer
	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var c dockerPsLine
		if err := json.Unmarshal([]byte(line), &c); err != nil {
			continue
		}
		ports := strings.Split(c.Ports, ", ")
		var cleanPorts []string
		for _, p := range ports {
			if p != "" {
				cleanPorts = append(cleanPorts, strings.TrimSpace(p))
			}
		}
		containers = append(containers, DockerContainer{
			Name:   c.Names,
			Image:  c.Image,
			Status: c.Status,
			Ports:  cleanPorts,
		})
	}
	return containers
}

type dockerPsLine struct {
	Names     string `json:"Names"`
	Image     string `json:"Image"`
	Status    string `json:"Status"`
	Ports     string `json:"Ports"`
	CreatedAt string `json:"CreatedAt"`
}
