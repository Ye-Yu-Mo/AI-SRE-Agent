package executor

import "strings"

// dangerousPatterns 危险命令关键词匹配表。
var dangerousPatterns = []string{
	"rm -rf /",
	"rm -rf /var",
	"rm -rf /etc",
	"rm -rf ~",
	"mkfs.",
	"dd if=",
	"dd if =",
	"iptables -f",
	"iptables --flush",
	"ufw disable",
	"userdel",
	"passwd",
	"chmod -r 777 /",
	"chmod 777 /",
	"chown -r",
	"docker volume rm",
	"docker rm -f",
	"> /dev/sd",
	"() { :|:& };:",
	"fork bomb",
	":(){ :|:& };:",
	"reboot",
	"shutdown",
	"halt",
	"poweroff",
	"/etc/shadow",
	"/root/.ssh",
}

// IsDangerous 检查命令是否在黑名单中。
func IsDangerous(cmd string) bool {
	lower := strings.ToLower(cmd)
	for _, p := range dangerousPatterns {
		if strings.Contains(lower, p) {
			return true
		}
	}
	return false
}
