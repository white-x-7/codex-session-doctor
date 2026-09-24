package cli

import (
	"fmt"
	"io"
)

// runHelp 输出顶层或指定命令的帮助。
func runHelp(env Env, args []string) int {
	flags, positionals := splitArgs(args)
	for _, flag := range flags {
		if flag != "-h" && flag != "--help" {
			fmt.Fprintf(env.Stderr, "help 不支持选项：%s\n", flag)
			return ExitUsage
		}
	}
	if len(positionals) == 0 {
		printUsage(env.Stdout)
		return ExitOK
	}
	if len(positionals) > 1 {
		fmt.Fprintln(env.Stderr, "help 最多接受一个命令名。")
		return ExitUsage
	}
	switch positionals[0] {
	case "help":
		printUsage(env.Stdout)
	case "doctor":
		printDoctorUsage(env.Stdout)
	case "repair":
		printRepairUsage(env.Stdout)
	case "check-update":
		printCheckUpdateUsage(env.Stdout)
	case "version":
		fmt.Fprintf(env.Stdout, "%s %s\n", programName, Version)
	default:
		fmt.Fprintf(env.Stderr, "未知命令：%s\n\n", positionals[0])
		printUsage(env.Stderr)
		return ExitUsage
	}
	return ExitOK
}

func hasHelpFlag(args []string) bool {
	for _, arg := range args {
		if arg == "-h" || arg == "--help" {
			return true
		}
	}
	return false
}

func printDoctorUsage(w io.Writer) {
	fmt.Fprint(w, `用法：
  codex-session-doctor doctor

只读检查 Codex 主目录、进程状态、state 数据库、rollout 数量和历史投影库。
不会修改任何文件。
`)
}

func printRepairUsage(w io.Writer) {
	fmt.Fprint(w, `用法：
  codex-session-doctor repair <会话id|rollout文件名> [选项]
  codex-session-doctor repair --all [选项]

选项：
  --dry-run                 只预览要修的内容，不改动任何文件
  -y, --yes                 跳过确认提示
  --force                   即使检测到 Codex 正在运行也继续写入
  --drop-foreign-reasoning  整行删除带非官方加密内容的推理项
  --no-refresh-history      不清理历史投影缓存（仅调试用）
`)
}

func printCheckUpdateUsage(w io.Writer) {
	fmt.Fprint(w, `用法：
  codex-session-doctor check-update [选项]

选项：
  --timeout <秒>            网络超时时间，范围 1-300，默认 10 秒

该命令只访问 codex-session-doctor 的 GitHub release/tag API，
只读检查版本，不下载、不安装、不修改本地文件。
`)
}
