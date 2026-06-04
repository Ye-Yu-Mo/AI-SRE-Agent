package service

import (
	"bytes"
	"fmt"
	"text/template"
)

type UnitConfig struct {
	BinaryPath string
	DataDir    string
	User       string
	Group      string
	Port       int
	Secret     string
	EnvFile    string
}

const unitTemplate = `[Unit]
Description=AI Server Agent
Documentation=https://github.com/ai-sre/agent
After=network.target

[Service]
Type=simple
User={{.User}}
Group={{.Group}}
{{- if .EnvFile}}
EnvironmentFile={{.EnvFile}}
{{- end}}
ExecStart={{.BinaryPath}} serve --dir {{.DataDir}} --port {{.Port}}
Restart=always
RestartSec=5
NoNewPrivileges=true
ProtectSystem=strict
ProtectHome=true
ReadWritePaths={{.DataDir}}
PrivateTmp=true

[Install]
WantedBy=multi-user.target
`

var tmpl = template.Must(template.New("unit").Parse(unitTemplate))

func RenderUnit(cfg UnitConfig) string {
	if cfg.DataDir == "" {
		cfg.DataDir = "/var/lib/ai-server-agent"
	}
	if cfg.Port == 0 {
		cfg.Port = 9090
	}
	if cfg.BinaryPath == "" {
		cfg.BinaryPath = "/usr/local/bin/ai-server-agent"
	}
	if cfg.User == "" {
		cfg.User = "ai-server-agent"
	}
	if cfg.Group == "" {
		cfg.Group = "ai-server-agent"
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, cfg); err != nil {
		panic(fmt.Sprintf("service: unit template render: %v", err))
	}
	return buf.String()
}
