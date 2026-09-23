package codex

import (
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

// commandTimeout 限制外部检测命令的等待时间。
const commandTimeout = 5 * time.Second

// IsRunning 判断 Codex 桌面端是否正在运行。
//
// 运行中的进程可能覆盖写 rollout 文件和数据库，因此修复前必须先确认它已退出。
func IsRunning(home string) bool {
	if runtime.GOOS == "windows" {
		return runningOnWindows()
	}
	if socketInUse(filepath.Join(home, "ipc", "ipc.sock")) {
		return true
	}
	// 隔离主目录下只认本目录的 socket，避免读到真实运行的桌面进程。
	if Isolated() {
		return false
	}
	return runningByPattern()
}

// socketInUse 用 lsof 检查 ipc socket 是否被进程占用。
func socketInUse(socket string) bool {
	out, err := runCommand("lsof", socket)
	if err != nil {
		return false
	}
	return strings.TrimSpace(out) != ""
}

// runningByPattern 用 pgrep 匹配桌面端可执行文件与 bundle id。
func runningByPattern() bool {
	patterns := []string{
		"ChatGPT.app/Contents/MacOS/ChatGPT",
		"Codex.app/Contents/MacOS/Codex",
		"com.openai.codex",
	}
	for _, pattern := range patterns {
		out, err := runCommand("pgrep", "-f", pattern)
		if err == nil && strings.TrimSpace(out) != "" {
			return true
		}
	}
	return false
}

// runningOnWindows 用 tasklist 检查桌面端进程。
func runningOnWindows() bool {
	for _, image := range []string{"ChatGPT.exe", "Codex.exe"} {
		out, err := runCommand("tasklist", "/FI", "IMAGENAME eq "+image, "/NH")
		if err != nil {
			continue
		}
		if strings.Contains(strings.ToLower(out), strings.ToLower(image)) {
			return true
		}
	}
	return false
}

// runCommand 执行一个短命令并返回标准输出，超时会强制结束。
func runCommand(name string, args ...string) (string, error) {
	cmd := exec.Command(name, args...)
	type outcome struct {
		out []byte
		err error
	}
	done := make(chan outcome, 1)
	go func() {
		out, err := cmd.Output()
		done <- outcome{out: out, err: err}
	}()
	select {
	case result := <-done:
		return string(result.out), result.err
	case <-time.After(commandTimeout):
		if cmd.Process != nil {
			_ = cmd.Process.Kill()
		}
		return "", exec.ErrNotFound
	}
}
