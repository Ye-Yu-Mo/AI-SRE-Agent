package deploy

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
)

const caddyfilePath = "/etc/caddy/Caddyfile"

// ConfigureCaddy 为 domain 创建或更新 Caddy 反向代理 route。
// 若 caddy 二进制不存在，返回错误（不自动安装）。
func ConfigureCaddy(domain, upstreamPort string) error {
	if _, err := exec.LookPath("caddy"); err != nil {
		return fmt.Errorf("caddy not installed: %w", err)
	}

	block := generateCaddyBlock(domain, upstreamPort)

	// 读现有 Caddyfile
	data, err := os.ReadFile(caddyfilePath)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("read Caddyfile: %w", err)
	}

	content := string(data)
	currentBlock := extractCaddyBlock(content, domain)

	if currentBlock != "" {
		// 更新已有 block
		content = strings.Replace(content, currentBlock, block, 1)
	} else {
		// 追加新 block
		if content != "" && !strings.HasSuffix(content, "\n") {
			content += "\n"
		}
		content += block + "\n"
	}

	// 备份
	os.WriteFile(caddyfilePath+".bak", data, 0644)
	if err := os.WriteFile(caddyfilePath, []byte(content), 0644); err != nil {
		return fmt.Errorf("write Caddyfile: %w", err)
	}

	// reload
	return exec.Command("caddy", "reload", "--config", caddyfilePath).Run()
}

func generateCaddyBlock(domain, upstreamPort string) string {
	return fmt.Sprintf("%s {\n\treverse_proxy localhost:%s\n}", domain, upstreamPort)
}

func extractCaddyBlock(content, domain string) string {
	// 找到以 domain { 开头的 block，匹配到对应的 }
	start := strings.Index(content, domain+" {")
	if start < 0 {
		return ""
	}
	// 找到匹配的 }
	depth := 0
	end := start
	for i := start; i < len(content); i++ {
		if content[i] == '{' {
			depth++
		}
		if content[i] == '}' {
			depth--
			if depth == 0 {
				end = i + 1
				break
			}
		}
	}
	return content[start:end]
}

// RemoveCaddyRoute 删除 domain 对应的 Caddy route。
func RemoveCaddyRoute(domain string) error {
	return removeCaddyRouteFile(caddyfilePath, domain)
}

func removeCaddyRouteFile(path, domain string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil // 文件不存在 = 已删除
		}
		return err
	}

	content := string(data)
	block := extractCaddyBlock(content, domain)
	if block == "" {
		return nil // block 不存在 = 已删除
	}

	content = strings.Replace(content, block, "", 1)
	// 清理多余空行
	content = strings.TrimSpace(content) + "\n"

	// 备份后写回
	os.WriteFile(path+".bak", data, 0644)
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		return fmt.Errorf("write Caddyfile: %w", err)
	}

	// reload
	if _, err := exec.LookPath("caddy"); err == nil {
		return exec.Command("caddy", "reload", "--config", path).Run()
	}
	return nil
}
