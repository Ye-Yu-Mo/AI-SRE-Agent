package collector

import (
	"testing"
)

func TestOSInfo(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "os-release", `NAME="Ubuntu"
VERSION="22.04.4 LTS (Jammy Jellyfish)"
ID=ubuntu
VERSION_ID="22.04"
`)
	writeFile(t, dir, "version", "Linux version 5.15.0-101-generic (buildd@lcy02-amd64-101) (gcc (Ubuntu 11.4.0-1ubuntu1~22.04) 11.4.0, GNU ld (GNU Binutils for Ubuntu) 2.38) #111-Ubuntu SMP Tue Mar 5 20:16:58 UTC 2024")

	info := OSInfo(dir, dir)
	if info.Name != "Ubuntu" {
		t.Errorf("Name = %q, want Ubuntu", info.Name)
	}
	if info.VersionID != "22.04" {
		t.Errorf("VersionID = %q, want 22.04", info.VersionID)
	}
	if info.Version != "22.04.4 LTS (Jammy Jellyfish)" {
		t.Errorf("Version = %q", info.Version)
	}
	if info.Kernel != "5.15.0-101-generic" {
		t.Errorf("Kernel = %q, want 5.15.0-101-generic", info.Kernel)
	}
}

func TestOSInfo_Fallback(t *testing.T) {
	dir := t.TempDir()
	// 没有 os-release 和 version 文件 → 用 runtime.GOOS/GOARCH
	info := OSInfo(dir, dir)
	if info.Name == "" {
		t.Error("Name should not be empty when files are missing")
	}
	// 至少 Kernel 和 Arch 应该有值（来自 runtime）
	if info.Arch == "" {
		t.Error("Arch should not be empty")
	}
}
