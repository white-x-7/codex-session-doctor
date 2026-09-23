package cli

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/white-x-7/codex-session-doctor/internal/codex"
	"github.com/white-x-7/codex-session-doctor/internal/history"
	"github.com/white-x-7/codex-session-doctor/internal/repair"
)

// runDoctor 实现 doctor 子命令：只读地报告当前环境与待修会话数量。
func runDoctor(env Env, args []string) int {
	flags, positionals := splitArgs(args)
	fs := flag.NewFlagSet("doctor", flag.ContinueOnError)
	fs.SetOutput(env.Stderr)
	fs.Usage = func() { printUsage(env.Stderr) }
	if err := fs.Parse(flags); err != nil {
		return ExitUsage
	}
	if len(positionals) > 0 {
		fmt.Fprintf(env.Stderr, "doctor 不接受参数：%v\n", positionals)
		return ExitUsage
	}

	home, err := resolveHome(env)
	if err != nil {
		fmt.Fprintf(env.Stderr, "无法定位 Codex 主目录：%v\n", err)
		return ExitFailure
	}

	fmt.Fprintf(env.Stdout, "Codex 主目录 : %s\n", home)
	if runningCheck(env)(home) {
		fmt.Fprintln(env.Stdout, "进程状态     : 运行中（修复前请完全退出 Codex）")
	} else {
		fmt.Fprintln(env.Stdout, "进程状态     : 未运行")
	}

	dbPath := codex.StateDB(home)
	if _, err := os.Stat(dbPath); err != nil {
		fmt.Fprintln(env.Stdout, "state 数据库 : 未找到（仅按 sessions 目录扫描）")
	} else if db, openErr := codex.OpenReadOnly(dbPath); openErr != nil {
		fmt.Fprintf(env.Stdout, "state 数据库 : 暂时无法读取（%v）\n", openErr)
	} else {
		_ = db.Close()
		fmt.Fprintf(env.Stdout, "state 数据库 : 可读（%s）\n", filepath.Base(dbPath))
	}

	plans, scanned, warning := repair.ScanAll(home)
	fmt.Fprintf(env.Stdout, "rollout 总数 : %d 个（其中需要修复：%d 个）\n", scanned, len(plans))
	for _, plan := range plans {
		fmt.Fprintf(env.Stdout,
			"  %s：%d 处推理明文，%d 处非官方加密内容\n",
			filepath.Base(plan.Path), plan.Stats.WithContent, plan.Stats.ForeignEncrypted)
	}

	if db := history.DBPath(home); db != "" {
		fmt.Fprintf(env.Stdout, "历史投影库   : %s\n", filepath.Base(db))
	} else {
		fmt.Fprintln(env.Stdout, "历史投影库   : 未找到（首次续聊时由 Codex 自动创建）")
	}
	fmt.Fprintf(env.Stdout, "备份目录     : %s\n", codex.BackupDir(home))

	if warning != "" {
		fmt.Fprintf(env.Stderr, "警告：%s\n", warning)
	}
	return ExitOK
}
