// codex-session-doctor 修复 Codex 的会话历史，让被第三方接口推进过的会话
// 能够重新在官方接口下续聊。
package main

import (
	"os"

	"github.com/white-x-7/codex-session-doctor/internal/cli"
)

func main() {
	os.Exit(cli.Run(cli.Env{
		Args:   os.Args[1:],
		Stdout: os.Stdout,
		Stderr: os.Stderr,
		Stdin:  os.Stdin,
	}))
}
