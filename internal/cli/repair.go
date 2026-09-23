package cli

import (
	"bufio"
	"flag"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/white-x-7/codex-session-doctor/internal/repair"
)

// runRepair 实现 repair 子命令。
func runRepair(env Env, args []string) int {
	flags, positionals := splitArgs(args)
	fs := flag.NewFlagSet("repair", flag.ContinueOnError)
	fs.SetOutput(env.Stderr)
	all := fs.Bool("all", false, "修复全部受影响的会话")
	dryRun := fs.Bool("dry-run", false, "只预览，不写入")
	yes := fs.Bool("yes", false, "跳过确认")
	fs.BoolVar(yes, "y", false, "跳过确认")
	force := fs.Bool("force", false, "Codex 运行时也继续")
	dropForeign := fs.Bool("drop-foreign-reasoning", false, "整行删除带非官方加密内容的推理项")
	noRefresh := fs.Bool("no-refresh-history", false, "不清理历史投影缓存")
	fs.Usage = func() { printUsage(env.Stderr) }
	if err := fs.Parse(flags); err != nil {
		return ExitUsage
	}

	if len(positionals) > 1 {
		fmt.Fprintln(env.Stderr, "只能指定一个会话 id 或一个 rollout 文件。")
		return ExitUsage
	}
	session := ""
	if len(positionals) == 1 {
		session = positionals[0]
	}
	if *all && session != "" {
		fmt.Fprintln(env.Stderr, "请二选一：会话 id 或 --all。")
		return ExitUsage
	}
	if !*all && session == "" {
		fmt.Fprintln(env.Stderr, "请指定会话 id，或使用 --all。")
		return ExitUsage
	}
	if *dropForeign && *noRefresh {
		fmt.Fprintln(env.Stderr,
			"--drop-foreign-reasoning 会改变文件长度，必须刷新历史投影，"+
				"请去掉 --no-refresh-history。")
		return ExitUsage
	}

	home, err := resolveHome(env)
	if err != nil {
		fmt.Fprintf(env.Stderr, "无法定位 Codex 主目录：%v\n", err)
		return ExitFailure
	}

	var plans []repair.Plan
	if *all {
		var scanned int
		var warning string
		plans, scanned, warning = repair.ScanAll(home)
		if warning != "" {
			fmt.Fprintf(env.Stderr, "警告：%s\n", warning)
		}
		fmt.Fprintf(env.Stdout, "已扫描 rollout：%d 个\n", scanned)
	} else {
		paths, err := repair.Resolve(home, session)
		if err != nil {
			fmt.Fprintf(env.Stderr, "错误：%v\n", err)
			return ExitFailure
		}
		fmt.Fprintf(env.Stdout, "会话 '%s' 匹配到 rollout：%d 个\n", session, len(paths))
		for _, path := range paths {
			fmt.Fprintf(env.Stdout, "  %s\n", filepath.Base(path))
		}
		plans = repair.PlanFor(paths)
	}

	fmt.Fprintf(env.Stdout, "需要修复的 rollout：%d 个\n", len(plans))
	for _, plan := range plans {
		fmt.Fprintf(env.Stdout,
			"  %s：%d 处推理明文待清空，%d 处非官方加密内容待移除（共 %d 个推理项）\n",
			filepath.Base(plan.Path), plan.Stats.WithContent,
			plan.Stats.ForeignEncrypted, plan.Stats.Total)
	}

	if *dryRun {
		fmt.Fprintln(env.Stdout, "预演完成：没有修改任何文件。")
		return ExitOK
	}
	if len(plans) == 0 {
		fmt.Fprintln(env.Stdout, "无需修复：所有推理项的内容都已经兼容官方接口。")
		return ExitOK
	}

	if runningCheck(env)(home) && !*force {
		fmt.Fprintf(env.Stderr,
			"Codex 仍在运行。\n请先完全退出 Codex（Cmd+Q），然后重新执行：\n"+
				"  %s repair %s\n", programName, repairTarget(session))
		return ExitBusy
	}

	if !*yes {
		fmt.Fprintf(env.Stdout,
			"将重写 %d 个 rollout（清空推理明文、移除非官方加密内容），"+
				"写入前会自动备份。[y/N] ", len(plans))
		reader := bufio.NewReader(env.Stdin)
		answer, _ := reader.ReadString('\n')
		switch strings.ToLower(strings.TrimSpace(answer)) {
		case "y", "yes":
		default:
			fmt.Fprintln(env.Stdout, "已取消。")
			return ExitOK
		}
	}

	summary, err := repair.Apply(plans, repair.ApplyOptions{
		Home:           home,
		RefreshHistory: !*noRefresh,
		DropForeign:    *dropForeign,
	})
	if err != nil {
		fmt.Fprintf(env.Stderr, "错误：修复失败：%v\n", err)
		return ExitFailure
	}
	printSummary(env, summary)
	return ExitOK
}

// repairTarget 在提示里回显用户原本指定的目标。
func repairTarget(session string) string {
	if session == "" {
		return "--all"
	}
	return session
}

// printSummary 输出修复结果。
func printSummary(env Env, summary repair.Summary) {
	fmt.Fprintf(env.Stdout, "已修复 %d 个 rollout：\n", summary.Rollouts)
	fmt.Fprintf(env.Stdout, "  清空推理明文         : %d\n", summary.ContentEmptied)
	fmt.Fprintf(env.Stdout, "  移除非官方加密内容   : %d\n", summary.EncryptedStripped)
	if summary.LinesDropped > 0 {
		fmt.Fprintf(env.Stdout, "  删除的推理行         : %d（文件长度已变化）\n", summary.LinesDropped)
	}
	fmt.Fprintf(env.Stdout, "  字节长度保持不变     : %d 字节填充\n", summary.PaddingBytes)
	if summary.ProjectionRows > 0 {
		fmt.Fprintf(env.Stdout,
			"  历史投影缓存         : 清理 %d 行，Codex 将重新读取\n", summary.ProjectionRows)
	} else {
		fmt.Fprintln(env.Stdout, "  历史投影缓存         : 没有缓存行需要清理")
	}
	for _, warning := range summary.Warnings {
		fmt.Fprintf(env.Stderr, "警告：%s\n", warning)
	}
	if summary.Backup != "" {
		fmt.Fprintf(env.Stdout, "备份：%s\n", summary.Backup)
	}
}
