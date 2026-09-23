// Package rollout 解析并修复 Codex 的会话记录文件（rollout JSONL）。
package rollout

import (
	"bufio"
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
)

const (
	// ReasoningType 是 rollout 里推理项的类型名。
	ReasoningType = "reasoning"
	// OpenAIPrefix 是官方加密内容的前缀（Fernet 格式）。
	OpenAIPrefix = "gAAAAA"
)

// IsForeignEncryptedContent 判断 encrypted_content 是否不可能来自官方接口。
//
// 官方写入的是 gAAAAA 开头的 Fernet 密文；第三方服务或兼容网关写入的
// 占位符（例如 8ec8e468-...-0）无法被官方接口校验，必须清除。
func IsForeignEncryptedContent(value any) bool {
	if value == nil {
		return false
	}
	text, ok := value.(string)
	if !ok {
		return true
	}
	return !strings.HasPrefix(text, OpenAIPrefix)
}

// SplitLines 按换行切分文件内容，保留末尾空段，便于原样拼回。
func SplitLines(raw []byte) [][]byte {
	return bytes.Split(raw, []byte("\n"))
}

// JoinLines 把切分后的内容拼回原始形态，保留文件末尾的换行。
func JoinLines(parts [][]byte) []byte {
	return bytes.Join(parts, []byte("\n"))
}

// SplitTrailingCR 分离行尾的 \r，返回正文与需要回填的字节。
func SplitTrailingCR(part []byte) (body []byte, cr []byte) {
	if len(part) > 0 && part[len(part)-1] == '\r' {
		return part[:len(part)-1], []byte("\r")
	}
	return part, nil
}

// DecodeObject 解析一行 JSON，并使用 json.Number 保留数字的原始写法。
//
// 用 map[string]any 直接解析会把整数变成 float64，大整数会丢失精度并改变
// 字面量；UseNumber 可以避免这种破坏。
func DecodeObject(line []byte) (map[string]any, bool) {
	decoder := json.NewDecoder(bytes.NewReader(line))
	decoder.UseNumber()
	var obj map[string]any
	if err := decoder.Decode(&obj); err != nil {
		return nil, false
	}
	return obj, true
}

// EncodeObject 紧凑序列化一个对象，关闭 HTML 转义以贴近原始写法。
func EncodeObject(obj map[string]any) ([]byte, error) {
	var buf bytes.Buffer
	encoder := json.NewEncoder(&buf)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(obj); err != nil {
		return nil, err
	}
	return bytes.TrimRight(buf.Bytes(), "\n"), nil
}

// Payload 取出 rollout 行的 payload 字段。
func Payload(obj map[string]any) (map[string]any, bool) {
	value, ok := obj["payload"].(map[string]any)
	if !ok {
		return nil, false
	}
	return value, true
}

// ThreadID 读取 rollout 第一行 session_meta 里的会话 id。
func ThreadID(path string) (string, bool) {
	file, err := os.Open(path)
	if err != nil {
		return "", false
	}
	defer file.Close()

	reader := bufio.NewReader(file)
	first, err := reader.ReadBytes('\n')
	if err != nil && len(first) == 0 {
		return "", false
	}
	obj, ok := DecodeObject(bytes.TrimSpace(first))
	if !ok {
		return "", false
	}
	payload, ok := Payload(obj)
	if !ok {
		return "", false
	}
	for _, key := range []string{"session_id", "id"} {
		if value, ok := payload[key].(string); ok && value != "" {
			return value, true
		}
	}
	return "", false
}

// ThreadIDs 返回这个 rollout 归属的全部会话 id。
//
// 续写过的会话会写成 rollout-<时间>-<会话id>_<分段id>.jsonl。Codex 为会话 id
// 和每个续写分段分别维护分页游标，因此重写文件后必须同时刷新这些 id 的缓存，
// 否则未更新的分段游标会指向被截短的文件，续聊时报
// "cutoff byte offset is past the source rollout"。
func ThreadIDs(path string) []string {
	var ids []string
	seen := map[string]bool{}
	add := func(id string) {
		if id != "" && !seen[id] {
			seen[id] = true
			ids = append(ids, id)
		}
	}

	if parent, ok := ThreadID(path); ok {
		add(parent)
	}
	base := filepath.Base(path)
	base = strings.TrimSuffix(base, ".jsonl")
	if index := strings.LastIndex(base, "_"); index >= 0 {
		add(base[index+1:])
	}
	return ids
}
