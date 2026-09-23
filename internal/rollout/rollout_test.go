package rollout

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestIsForeignEncryptedContent(t *testing.T) {
	cases := []struct {
		name  string
		value any
		want  bool
	}{
		{"缺失字段", nil, false},
		{"官方密文", "gAAAAABofficial", false},
		{"第三方占位符", "8ec8e468-e6a3-4513-a1f0-9fbb4f189148-0", true},
		{"空字符串", "", true},
		{"非字符串", 42, true},
	}
	for _, item := range cases {
		if got := IsForeignEncryptedContent(item.value); got != item.want {
			t.Errorf("%s：期望 %v，实际 %v", item.name, item.want, got)
		}
	}
}

func TestThreadIDsIncludesParentAndSegment(t *testing.T) {
	dir := t.TempDir()
	parent := "01parent"
	segment := "01segment"
	path := filepath.Join(dir, "rollout-2026-09-23T00-00-00-"+parent+"_"+segment+".jsonl")
	writeFile(t, path, jsonLine(map[string]any{
		"type":    "session_meta",
		"payload": map[string]any{"session_id": parent, "id": parent},
	}))

	got := ThreadIDs(path)
	want := []string{parent, segment}
	if len(got) != len(want) {
		t.Fatalf("期望 %d 个 id（%v），实际 %v", len(want), want, got)
	}
	for index := range want {
		if got[index] != want[index] {
			t.Fatalf("第 %d 个 id：期望 %s，实际 %s", index, want[index], got[index])
		}
	}
}

func TestThreadIDsWithoutSegment(t *testing.T) {
	dir := t.TempDir()
	parent := "01a0c369-caf2-71c1-bdf1-a1f56cd46f00"
	path := filepath.Join(dir, "rollout-2026-09-21T18-01-25-"+parent+".jsonl")
	writeFile(t, path, jsonLine(map[string]any{
		"type":    "session_meta",
		"payload": map[string]any{"session_id": parent},
	}))

	got := ThreadIDs(path)
	if len(got) != 1 || got[0] != parent {
		t.Fatalf("期望只有父会话 id %s，实际 %v", parent, got)
	}
}

func TestScanCountsReasoningProblems(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "rollout.jsonl")
	lines := []map[string]any{
		{"type": "session_meta", "payload": map[string]any{"session_id": "s1"}},
		reasoningLine([]any{map[string]any{"type": "reasoning_text", "text": "x"}}, nil),
		reasoningLine([]any{}, "8ec8e468-0000-0000-0000-000000000000-0"),
		reasoningLine([]any{}, "gAAAAABofficial"),
	}
	writeFile(t, path, joinLines(lines...))

	stats, err := Scan(path)
	if err != nil {
		t.Fatalf("扫描失败：%v", err)
	}
	if stats.Total != 3 || stats.WithContent != 1 || stats.ForeignEncrypted != 1 {
		t.Fatalf("统计不符：%+v", stats)
	}
	if !NeedsRepair(stats) {
		t.Fatal("应当判定为需要修复")
	}

	clean := filepath.Join(dir, "clean.jsonl")
	writeFile(t, clean, joinLines(
		map[string]any{"type": "session_meta", "payload": map[string]any{"session_id": "s1"}},
		reasoningLine([]any{}, "gAAAAABofficial"),
	))
	cleanStats, err := Scan(clean)
	if err != nil {
		t.Fatalf("扫描失败：%v", err)
	}
	if NeedsRepair(cleanStats) {
		t.Fatalf("干净的 rollout 不应判定为需要修复：%+v", cleanStats)
	}
}

// reasoningLine 构造一条推理项。
func reasoningLine(content []any, encrypted any) map[string]any {
	payload := map[string]any{
		"type":    "reasoning",
		"id":      "reasoning-1",
		"summary": []any{},
		"content": content,
	}
	if encrypted != nil {
		payload["encrypted_content"] = encrypted
	}
	return map[string]any{"type": "response_item", "payload": payload}
}

// jsonLine 序列化一个对象为单行 JSON。
func jsonLine(obj map[string]any) string {
	data, _ := json.Marshal(obj)
	return string(data) + "\n"
}

// joinLines 把多个对象拼成 JSONL 文本。
func joinLines(objs ...map[string]any) string {
	out := ""
	for _, obj := range objs {
		out += jsonLine(obj)
	}
	return out
}

// writeFile 写入测试文件，失败即终止用例。
func writeFile(t *testing.T, path string, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("写入 %s 失败：%v", path, err)
	}
}
