package rollout

import (
	"bytes"
	"os"
)

// Stats 描述一个 rollout 里推理项的兼容性问题数量。
type Stats struct {
	// Total 是推理项总数。
	Total int
	// WithContent 是仍带明文 content 的推理项数量（官方接口要求为空数组）。
	WithContent int
	// ForeignEncrypted 是带非官方 encrypted_content 的推理项数量。
	ForeignEncrypted int
}

// NeedsRepair 判断该 rollout 是否需要修复。
func NeedsRepair(stats Stats) bool {
	return stats.WithContent > 0 || stats.ForeignEncrypted > 0
}

// Scan 统计一个 rollout 中不兼容的推理项。
func Scan(path string) (Stats, error) {
	var stats Stats
	raw, err := os.ReadFile(path)
	if err != nil {
		return stats, err
	}
	for _, part := range SplitLines(raw) {
		line := bytes.TrimSpace(part)
		if len(line) == 0 {
			continue
		}
		obj, ok := DecodeObject(line)
		if !ok {
			continue
		}
		payload, ok := Payload(obj)
		if !ok || payload["type"] != ReasoningType {
			continue
		}
		stats.Total++
		if content, ok := payload["content"].([]any); ok && len(content) > 0 {
			stats.WithContent++
		}
		if IsForeignEncryptedContent(payload["encrypted_content"]) {
			stats.ForeignEncrypted++
		}
	}
	return stats, nil
}
