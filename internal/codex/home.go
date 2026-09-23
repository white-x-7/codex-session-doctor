// Package codex 负责定位 Codex 本地目录，并读取它维护的数据库与进程状态。
package codex

import (
	"os"
	"path/filepath"
)

const (
	// EnvHome 指向一个模拟的 Codex 主目录，供测试和隔离运行使用。
	EnvHome = "CODEX_SESSION_DOCTOR_HOME"
	// EnvCodexHome 是 Codex 自身支持的主目录覆盖变量。
	EnvCodexHome = "CODEX_HOME"
)

// Home 返回 Codex 主目录，默认是 ~/.codex。
//
// 优先级：EnvHome > EnvCodexHome > ~/.codex。
func Home() (string, error) {
	if v := os.Getenv(EnvHome); v != "" {
		return v, nil
	}
	if v := os.Getenv(EnvCodexHome); v != "" {
		return v, nil
	}
	dir, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, ".codex"), nil
}

// Isolated 报告当前是否运行在隔离主目录下。
//
// 隔离模式下不检查真实桌面进程，避免测试误判成 Codex 正在运行。
func Isolated() bool {
	return os.Getenv(EnvHome) != ""
}

// StateDB 返回 Codex 的 state 数据库路径（state_5.sqlite）。
func StateDB(home string) string {
	return filepath.Join(home, "state_5.sqlite")
}

// BackupDir 返回本工具的备份根目录。
func BackupDir(home string) string {
	return filepath.Join(home, "backups", "codex-session-doctor")
}
