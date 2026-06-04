package collector

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

type OSInfoResult struct {
	Name      string `json:"name"`
	Version   string `json:"version"`
	VersionID string `json:"version_id"`
	Kernel    string `json:"kernel"`
	Arch      string `json:"arch"`
}

func OSInfo(etcRoot, procRoot string) *OSInfoResult {
	info := &OSInfoResult{
		Name:      runtime.GOOS,
		VersionID: "unknown",
		Arch:      runtime.GOARCH,
	}

	// /etc/os-release
	if data, err := os.ReadFile(filepath.Join(etcRoot, "os-release")); err == nil {
		for _, line := range strings.Split(string(data), "\n") {
			line = strings.TrimSpace(line)
			if line == "" || strings.HasPrefix(line, "#") {
				continue
			}
			parts := strings.SplitN(line, "=", 2)
			if len(parts) != 2 {
				continue
			}
			key := parts[0]
			val := strings.Trim(parts[1], `"`)
			switch key {
			case "NAME":
				info.Name = val
			case "VERSION":
				info.Version = val
			case "VERSION_ID":
				info.VersionID = val
			}
		}
	}

	// /proc/version
	if data, err := os.ReadFile(filepath.Join(procRoot, "version")); err == nil {
		// "Linux version 5.15.0-101-generic ..."
		line := strings.TrimSpace(string(data))
		parts := strings.Fields(line)
		if len(parts) >= 3 {
			info.Kernel = parts[2]
		}
	}

	return info
}
