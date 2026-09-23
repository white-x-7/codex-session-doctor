package rollout

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const (
	officialBlob = "gAAAAABofficial-example"
	foreignBlob  = "8ec8e468-e6a3-4513-a1f0-9fbb4f189148-0"
)

// buildRollout 构造包含明文推理、伪造密文与官方密文的 rollout。
func buildRollout(t *testing.T) (string, int64) {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "rollout-2026-09-23T00-00-00-01parent.jsonl")

	// 第一条无关消息刻意保留多余空格，用于验证未被触碰的行按字节原样保留。
	untouched := `{"timestamp":"2026-09-23T00:00:00.000Z",  "type":"event_msg",  "payload":{"type":"user_message","content":"你好"}}`
	// 这条推理项带一个大整数（超出 float64 精度），用于验证数字字面量不被改写。
	withBigNumber := `{"type":"response_item","payload":{"type":"reasoning","id":"r1",` +
		`"summary":[],"content":[{"type":"reasoning_text","text":"thinking"}],` +
		`"encrypted_content":"` + foreignBlob + `",` +
		`"token_count":1780000000000123456}}`
	content := strings.Join([]string{
		`{"type":"session_meta","payload":{"session_id":"01parent"}}`,
		untouched,
		withBigNumber,
		jsonLine(reasoningLine([]any{map[string]any{"type": "reasoning_text", "text": "thinking"}}, officialBlob)),
		jsonLine(reasoningLine([]any{}, nil)),
	}, "\n") + "\n"

	writeFile(t, path, content)
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("读取文件大小失败：%v", err)
	}
	return path, info.Size()
}

func TestRewritePreservesByteLength(t *testing.T) {
	path, sizeBefore := buildRollout(t)

	result, err := Rewrite(path, false)
	if err != nil {
		t.Fatalf("修复失败：%v", err)
	}
	if result.ContentEmptied != 2 {
		t.Errorf("期望清空 2 处明文，实际 %d", result.ContentEmptied)
	}
	if result.EncryptedStripped != 1 {
		t.Errorf("期望移除 1 处非官方密文，实际 %d", result.EncryptedStripped)
	}
	if result.LinesDropped != 0 {
		t.Errorf("正常修复不应删除行，实际删除 %d 行", result.LinesDropped)
	}
	if result.SizeChanged() {
		t.Error("正常修复不应改变文件长度")
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("读取文件大小失败：%v", err)
	}
	if info.Size() != sizeBefore {
		t.Fatalf("字节长度变化：%d -> %d", sizeBefore, info.Size())
	}

	lines := readLines(t, path)
	var withForeign, withOfficial, withContent bool
	for _, line := range lines {
		payload := payloadOf(t, line)
		if payload == nil || payload["type"] != ReasoningType {
			continue
		}
		if blob, ok := payload["encrypted_content"].(string); ok {
			if blob == foreignBlob {
				withForeign = true
			}
			if blob == officialBlob {
				withOfficial = true
			}
		}
		if content, ok := payload["content"].([]any); ok && len(content) > 0 {
			withContent = true
		}
	}
	if withForeign {
		t.Error("非官方密文应当被移除")
	}
	if !withOfficial {
		t.Error("官方密文应当保留")
	}
	if withContent {
		t.Error("明文推理内容应当被清空")
	}
}

func TestRewriteKeepsUnrelatedLinesByteForByte(t *testing.T) {
	path, _ := buildRollout(t)
	before := readLines(t, path)

	if _, err := Rewrite(path, false); err != nil {
		t.Fatalf("修复失败：%v", err)
	}
	after := readLines(t, path)
	if before[1] != after[1] {
		t.Errorf("无关行被改写：\n修复前 %s\n修复后 %s", before[1], after[1])
	}
	if !strings.Contains(after[2], "1780000000000123456") {
		t.Errorf("大整数的字面量在重写后丢失：%s", after[2])
	}
}

func TestRewriteIsIdempotent(t *testing.T) {
	path, _ := buildRollout(t)
	if _, err := Rewrite(path, false); err != nil {
		t.Fatalf("第一次修复失败：%v", err)
	}
	first, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("读取失败：%v", err)
	}

	result, err := Rewrite(path, false)
	if err != nil {
		t.Fatalf("第二次修复失败：%v", err)
	}
	if result.ContentEmptied != 0 || result.EncryptedStripped != 0 || result.LinesDropped != 0 {
		t.Errorf("重复修复不应再有改动：%+v", result)
	}
	second, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("读取失败：%v", err)
	}
	if string(first) != string(second) {
		t.Error("重复修复改变了文件内容")
	}
}

func TestRewriteDropForeignRemovesLines(t *testing.T) {
	path, sizeBefore := buildRollout(t)

	result, err := Rewrite(path, true)
	if err != nil {
		t.Fatalf("修复失败：%v", err)
	}
	if result.LinesDropped != 1 {
		t.Errorf("期望删除 1 行，实际 %d", result.LinesDropped)
	}
	if !result.SizeChanged() {
		t.Error("删除整行时应当报告长度已变化")
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("读取文件大小失败：%v", err)
	}
	if info.Size() >= sizeBefore {
		t.Errorf("删除整行后文件应变小：%d -> %d", sizeBefore, info.Size())
	}
}

// readLines 读取文件并按行切分。
func readLines(t *testing.T, path string) []string {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("读取 %s 失败：%v", path, err)
	}
	return strings.Split(strings.TrimSuffix(string(raw), "\n"), "\n")
}

// payloadOf 取出一行 JSON 的 payload。
func payloadOf(t *testing.T, line string) map[string]any {
	t.Helper()
	var obj map[string]any
	if err := json.Unmarshal([]byte(strings.TrimSpace(line)), &obj); err != nil {
		return nil
	}
	payload, _ := obj["payload"].(map[string]any)
	return payload
}
