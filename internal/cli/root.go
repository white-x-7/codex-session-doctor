// Package cli 实现 codex-session-doctor 的命令行入口。
package cli

import (
	"fmt"
	"io"
	"strings"

	"github.com/white-x-7/codex-session-doctor/internal/codex"
)

// Version 是当前版本号。
const Version = "0.1.0"

// 退出码：与旧版保持一致，便于脚本判断。
const (
	// ExitOK 表示成功。
	ExitOK = 0
	// ExitFailure 表示修复失败。
	ExitFailure = 1
	// ExitUsage 表示参数用法错误。
	ExitUsage = 2
	// ExitBusy 表示 Codex 仍在运行，拒绝写入。
	ExitBusy = 3
)

const programName = "codex-session-doctor"

// Env 汇总一次运行的输入输出与可注入的依赖。
type Env struct {
	// Args 是不含程序名的参数。
	Args []string
	// Stdout / Stderr / Stdin 是输出与交互输入。
	Stdout io.Writer
	Stderr io.Writer
	Stdin  io.Reader
	// Home 覆盖 Codex 主目录，为空时按环境变量解析。
	Home string
	// IsRunning 检测桌面端是否运行，为空时使用真实检测；测试可替换。
	IsRunning func(home string) bool
}

// Run 执行一次命令并返回退出码。
func Run(env Env) int {
	if env.Stdout == nil {
		env.Stdout = io.Discard
	}
	if env.Stderr == nil {
		env.Stderr = io.Discard
	}
	command, rest := nextCommand(env.Args)
	switch command {
	case "":
		printUsage(env.Stdout)
		return ExitOK
	case "help", "-h", "--help":
		printUsage(env.Stdout)
		return ExitOK
	case "version", "-v", "--version":
		fmt.Fprintf(env.Stdout, "%s %s\n", programName, Version)
		return ExitOK
	case "doctor":
		return runDoctor(env, rest)
	case "repair":
		return runRepair(env, rest)
	default:
		fmt.Fprintf(env.Stderr, "未知子命令：%s\n\n", command)
		printUsage(env.Stderr)
		return ExitUsage
	}
}

// resolveHome 解析 Codex 主目录，Env.Home 优先。
func resolveHome(env Env) (string, error) {
	if env.Home != "" {
		return env.Home, nil
	}
	return codex.Home()
}

// runningCheck 返回用于检测桌面端状态的方法。
func runningCheck(env Env) func(string) bool {
	if env.IsRunning != nil {
		return env.IsRunning
	}
	return codex.IsRunning
}

// nextCommand 取出第一个非选项参数作为子命令，其余原样返回。
func nextCommand(args []string) (string, []string) {
	for index, arg := range args {
		if strings.HasPrefix(arg, "-") {
			continue
		}
		rest := make([]string, 0, len(args)-1)
		rest = append(rest, args[:index]...)
		rest = append(rest, args[index+1:]...)
		return arg, rest
	}
	return "", args
}

// splitArgs 把参数拆成选项与位置参数。
//
// 本工具的所有选项都是布尔开关，因此任何以 - 开头的参数都当作开关处理，
// 这样选项写在会话 id 前面或后面都能识别。
func splitArgs(args []string) (flags []string, positionals []string) {
	for _, arg := range args {
		if strings.HasPrefix(arg, "-") && arg != "-" {
			flags = append(flags, arg)
			continue
		}
		positionals = append(positionals, arg)
	}
	return flags, positionals
}

// printUsage 输出帮助信息。
func printUsage(w io.Writer) {
	fmt.Fprint(w, `codex-session-doctor —— 修复 Codex 会话历史，让被第三方接口推进过的会话能重新续聊

用法：
  codex-session-doctor repair <会话id|rollout文件名> [选项]
  codex-session-doctor repair --all [选项]
  codex-session-doctor doctor
  codex-session-doctor version

选项：
  --dry-run                 只预览要修的内容，不改动任何文件
  -y, --yes                 跳过确认提示（仅代表免确认，不代表强制）
  --force                   即使检测到 Codex 正在运行也继续写入
  --drop-foreign-reasoning  整行删除带非官方加密内容的推理项
                            （会改变文件长度，必须刷新历史投影）
  --no-refresh-history      不清理历史投影缓存
                            （仅调试用；正常修复不要加）

退出码：
  0  成功
  1  修复失败
  2  参数用法错误
  3  Codex 正在运行，已拒绝写入

示例：
  codex-session-doctor doctor
  codex-session-doctor repair 01a0c369-caf2-71c1-bdf1-a1f56cd46f00 --dry-run
  codex-session-doctor repair 01a0c369-caf2-71c1-bdf1-a1f56cd46f00 -y
  codex-session-doctor repair --all -y
`)
}
