package cli

import (
	"context"
	"flag"
	"fmt"
	"time"

	"github.com/white-x-7/codex-session-doctor/internal/update"
)

// runCheckUpdate 实现 check-update 子命令。
func runCheckUpdate(env Env, args []string) int {
	if hasHelpFlag(args) {
		printCheckUpdateUsage(env.Stdout)
		return ExitOK
	}
	fs := flag.NewFlagSet("check-update", flag.ContinueOnError)
	fs.SetOutput(env.Stderr)
	timeoutSeconds := fs.Int("timeout", 10, "网络超时时间（秒）")
	fs.Usage = func() { printCheckUpdateUsage(env.Stderr) }
	if err := fs.Parse(args); err != nil {
		return ExitUsage
	}
	if fs.NArg() > 0 {
		fmt.Fprintf(env.Stderr, "check-update 不接受位置参数：%v\n", fs.Args())
		return ExitUsage
	}
	if *timeoutSeconds < 1 || *timeoutSeconds > 300 {
		fmt.Fprintln(env.Stderr, "--timeout 必须是 1 到 300 之间的整数。")
		return ExitUsage
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(*timeoutSeconds)*time.Second)
	defer cancel()
	result, err := update.Check(ctx, env.UpdateClient, Version)
	if err != nil {
		fmt.Fprintf(env.Stderr, "检查更新失败：%v\n", err)
		return ExitFailure
	}

	fmt.Fprintf(env.Stdout, "当前版本：%s\n", result.Current)
	fmt.Fprintf(env.Stdout, "最新版本：%s（来源：%s）\n", result.Latest, result.Source)
	if result.UpdateAvailable {
		fmt.Fprintln(env.Stdout, "发现新版本。工具不会自动下载或安装，请打开下面的链接查看更新：")
		fmt.Fprintf(env.Stdout, "%s\n", result.URL)
	} else {
		fmt.Fprintln(env.Stdout, "当前已经是最新版本。")
	}
	return ExitOK
}
