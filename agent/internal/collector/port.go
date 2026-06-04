package collector

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

type PortInfo struct {
	Port     int    `json:"port"`
	Protocol string `json:"protocol"`
	State    string `json:"state"`
	Inode    uint64 `json:"-"`
}

// ListeningPorts 返回所有监听中的 TCP/TCP6 端口
func ListeningPorts(procRoot string) []PortInfo {
	var ports []PortInfo

	for _, f := range []string{"tcp", "tcp6"} {
		data, err := os.ReadFile(filepath.Join(procRoot, "net", f))
		if err != nil {
			continue
		}
		ports = append(ports, parseNetFile(string(data), f)...)
	}
	return ports
}

func parseNetFile(content, proto string) []PortInfo {
	var ports []PortInfo
	lines := strings.Split(content, "\n")
	for i, line := range lines {
		if i == 0 || strings.TrimSpace(line) == "" {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 4 {
			continue
		}

		// local_address 格式: HEX:HEX (IP:PORT)
		local := fields[1]
		parts := strings.Split(local, ":")
		if len(parts) < 2 {
			continue
		}
		portHex := parts[len(parts)-1]
		port, err := strconv.ParseInt(portHex, 16, 64)
		if err != nil {
			continue
		}

		state := "unknown"
		if len(fields) >= 4 {
			// st 字段: 0A = LISTEN
			st, _ := strconv.ParseInt(fields[3], 16, 64)
			switch st {
			case 0x0A:
				state = "LISTEN"
			case 0x01:
				state = "ESTABLISHED"
			case 0x06:
				state = "TIME_WAIT"
			}
		}

		protocol := "tcp"
		if proto == "tcp6" {
			protocol = "tcp6"
		}

		ports = append(ports, PortInfo{
			Port:     int(port),
			Protocol: protocol,
			State:    state,
		})
	}
	return ports
}

// PortProcessMapping 将端口映射到进程名
func PortProcessMapping(procRoot string) []map[string]interface{} {
	// 读所有进程的 fd 目录，找 socket inode
	inodeProc := map[uint64]string{}
	entries, _ := os.ReadDir(procRoot)
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		pid := e.Name()
		fdDir := filepath.Join(procRoot, pid, "fd")
		fds, err := os.ReadDir(fdDir)
		if err != nil {
			continue
		}
		for _, fd := range fds {
			link, err := os.Readlink(filepath.Join(fdDir, fd.Name()))
			if err != nil {
				continue
			}
			// socket:[12345]
			if strings.HasPrefix(link, "socket:[") {
				inodeStr := strings.TrimSuffix(strings.TrimPrefix(link, "socket:["), "]")
				inode, _ := strconv.ParseUint(inodeStr, 10, 64)
				if inode > 0 {
					// 读取进程名
					stat, _ := os.ReadFile(filepath.Join(procRoot, pid, "stat"))
					name := extractProcName(string(stat))
					inodeProc[inode] = name
				}
			}
		}
	}

	ports := ListeningPorts(procRoot)
	var result []map[string]interface{}
	for _, p := range ports {
		procName := inodeProc[p.Inode]
		if procName == "" {
			procName = "-"
		}
		result = append(result, map[string]interface{}{
			"port":     p.Port,
			"protocol": p.Protocol,
			"state":    p.State,
			"process":  procName,
		})
	}
	return result
}

func extractProcName(stat string) string {
	start := strings.Index(stat, "(")
	end := strings.LastIndex(stat, ")")
	if start >= 0 && end > start {
		return stat[start+1 : end]
	}
	return ""
}

// formatPortMap 辅助：确保 port info 不报 unused import
var _ = fmt.Sprintf // suppress unused import
