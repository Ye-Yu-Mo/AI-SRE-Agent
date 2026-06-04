package executor

import (
	"testing"
)

func TestIsDangerous_BlockedCommands(t *testing.T) {
	dangerous := []string{
		"rm -rf /",
		"rm -rf /var",
		"sudo rm -rf /app",
		"mkfs.ext4 /dev/sda",
		"dd if=/dev/zero of=/dev/sda",
		"iptables -F",
		"ufw disable",
		"userdel root",
		"passwd root",
		"chmod -R 777 /",
		"chown -R root:root /",
		"docker volume rm production_db",
		"docker rm -f $(docker ps -aq)",
		"> /dev/sda",
		":() { :|:& };:", // fork bomb
		"reboot",
		"shutdown -h now",
		"halt",
		"poweroff",
	}

	for _, cmd := range dangerous {
		if !IsDangerous(cmd) {
			t.Errorf("dangerous command not detected: %q", cmd)
		}
	}
}

func TestIsDangerous_SafeCommands(t *testing.T) {
	safe := []string{
		"systemctl restart nginx",
		"docker ps",
		"docker restart web-1",
		"cat /etc/hostname",
		"ls /var/log",
		"df -h",
		"free -m",
		"journalctl -u nginx -n 50",
		"docker logs web-1 --tail 50",
		"curl http://localhost/health",
		"",
	}

	for _, cmd := range safe {
		if IsDangerous(cmd) {
			t.Errorf("safe command flagged as dangerous: %q", cmd)
		}
	}
}

func TestIsDangerous_PathTraversal(t *testing.T) {
	blocked := []string{
		"cat /etc/shadow",
		"cat /root/.ssh/id_rsa",
		"cp /etc/shadow /tmp/",
		"curl file:///etc/shadow",
	}

	for _, cmd := range blocked {
		if !IsDangerous(cmd) {
			t.Errorf("sensitive file access not detected: %q", cmd)
		}
	}
}
