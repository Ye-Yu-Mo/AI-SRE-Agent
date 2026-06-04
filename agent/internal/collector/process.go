package collector

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

type ProcessInfo struct {
	PID     int    `json:"pid"`
	Name    string `json:"name"`
	Cmdline string `json:"cmdline"`
	State   string `json:"state"`
}

func Processes(procRoot string) []ProcessInfo {
	entries, err := os.ReadDir(procRoot)
	if err != nil {
		return nil
	}

	var procs []ProcessInfo
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		pid, err := strconv.Atoi(e.Name())
		if err != nil {
			continue
		}

		statData, _ := os.ReadFile(filepath.Join(procRoot, e.Name(), "stat"))
		cmdlineData, _ := os.ReadFile(filepath.Join(procRoot, e.Name(), "cmdline"))

		p := ProcessInfo{PID: pid}

		// /proc/[pid]/stat: pid (name) state ...
		if len(statData) > 0 {
			stat := string(statData)
			// 提取 name: 在第一个 ( 和 最后一个 ) 之间
			start := strings.Index(stat, "(")
			end := strings.LastIndex(stat, ")")
			if start >= 0 && end > start {
				p.Name = stat[start+1 : end]
				// state 在 ) 后面的第一个字符
				rest := stat[end+1:]
				fields := strings.Fields(rest)
				if len(fields) > 0 {
					p.State = fields[0]
				}
			}
		}

		// /proc/[pid]/cmdline: \x00 分隔
		if len(cmdlineData) > 0 {
			args := strings.Split(string(cmdlineData), "\x00")
			var cleanArgs []string
			for _, a := range args {
				if a != "" {
					cleanArgs = append(cleanArgs, a)
				}
			}
			p.Cmdline = strings.Join(cleanArgs, " ")
		}

		procs = append(procs, p)
	}
	return procs
}
