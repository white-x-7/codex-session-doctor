package repair

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPlanForReportsUnreadableRollouts(t *testing.T) {
	dir := t.TempDir()

	// 用一个同名目录冒充 rollout 文件，让读取必然失败。
	unreadable := filepath.Join(dir, "rollout-broken.jsonl")
	if err := os.Mkdir(unreadable, 0o755); err != nil {
		t.Fatalf("创建目录失败：%v", err)
	}

	plans, warnings := PlanFor([]string{unreadable})
	if len(plans) != 0 {
		t.Fatalf("不可读的文件不应进入修复计划，实际 %d 条", len(plans))
	}
	if len(warnings) != 1 {
		t.Fatalf("应当产生 1 条警告，实际 %d 条：%v", len(warnings), warnings)
	}
	if !strings.Contains(warnings[0], "rollout-broken.jsonl") {
		t.Errorf("警告里应当指出具体文件：%s", warnings[0])
	}
}

func TestPlanForKeepsOnlyRolloutsNeedingRepair(t *testing.T) {
	dir := t.TempDir()
	broken := filepath.Join(dir, "rollout-2026-09-23T00-00-00-01a.jsonl")
	clean := filepath.Join(dir, "rollout-2026-09-23T00-00-01-01b.jsonl")

	writeFile(t, broken, strings.Join([]string{
		`{"type":"session_meta","payload":{"session_id":"01a"}}`,
		`{"type":"response_item","payload":{"type":"reasoning","content":[{"type":"reasoning_text","text":"x"}],"encrypted_content":"8ec8e468-0"}}`,
	}, "\n")+"\n")
	writeFile(t, clean, strings.Join([]string{
		`{"type":"session_meta","payload":{"session_id":"01b"}}`,
		`{"type":"response_item","payload":{"type":"reasoning","content":[],"encrypted_content":"gAAAAABofficial"}}`,
	}, "\n")+"\n")

	plans, warnings := PlanFor([]string{broken, clean})
	if len(warnings) != 0 {
		t.Fatalf("不应有警告：%v", warnings)
	}
	if len(plans) != 1 || filepath.Base(plans[0].Path) != filepath.Base(broken) {
		t.Fatalf("只应保留需要修复的那一个，实际 %+v", plans)
	}
	if plans[0].Stats.WithContent != 1 || plans[0].Stats.ForeignEncrypted != 1 {
		t.Errorf("统计不符：%+v", plans[0].Stats)
	}
}

func TestAllRolloutPathsScansBothTrees(t *testing.T) {
	home := t.TempDir()
	current := filepath.Join(home, "sessions", "2026", "09")
	archived := filepath.Join(home, "archived_sessions")
	for _, dir := range []string{current, archived} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatalf("创建目录失败：%v", err)
		}
	}
	writeFile(t, filepath.Join(current, "rollout-2026-09-23T00-00-00-01a.jsonl"), "{}\n")
	writeFile(t, filepath.Join(archived, "rollout-2026-09-01T00-00-00-01b.jsonl"), "{}\n")
	// 不匹配的文件名不应被收集。
	writeFile(t, filepath.Join(current, "notes.jsonl"), "{}\n")

	paths, warning := AllRolloutPaths(home)
	if warning != "" {
		t.Fatalf("没有 state 数据库时不应告警：%s", warning)
	}
	if len(paths) != 2 {
		t.Fatalf("应当收集到 2 个 rollout，实际 %d 个：%v", len(paths), paths)
	}
}

// writeFile 写入测试文件。
func writeFile(t *testing.T, path string, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("写入 %s 失败：%v", path, err)
	}
}
