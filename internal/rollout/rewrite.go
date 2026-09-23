package rollout

import (
	"bytes"
	"fmt"
	"os"
)

// Result 记录一次重写的改动量。
type Result struct {
	// ContentEmptied 是被清空明文 content 的推理项数量。
	ContentEmptied int
	// EncryptedStripped 是被移除的非官方 encrypted_content 数量。
	EncryptedStripped int
	// LinesDropped 是被整行删除的推理项数量（仅在 --drop-foreign-reasoning 下出现）。
	LinesDropped int
	// PaddingBytes 是为了保持字节长度而补入的空格数。
	PaddingBytes int
}

// SizeChanged 报告行数变化导致文件长度改变。
func (r Result) SizeChanged() bool {
	return r.LinesDropped > 0
}

// PadLine 在行尾补空格，使其长度恰好等于 target。
//
// JSONL 的行尾空白不影响解析，因此补空格可以保持后续所有字节偏移不变。
func PadLine(line []byte, target int) ([]byte, error) {
	if len(line) > target {
		return nil, fmt.Errorf("无法保持字节长度：新行 %d 字节，原行 %d 字节", len(line), target)
	}
	if len(line) == target {
		return line, nil
	}
	padded := make([]byte, 0, target)
	padded = append(padded, line...)
	for len(padded) < target {
		padded = append(padded, ' ')
	}
	return padded, nil
}

// Rewrite 让 rollout 里的推理项可以被官方接口回放。
//
// content 里的明文推理会被置为空数组，非官方的 encrypted_content 会被移除，
// 两者都通过补空格保持原始字节长度，从而不破坏 Codex 的历史分页偏移。
//
// dropForeign 为真时改成整行删除带非官方加密内容的推理项，此时文件长度会变化，
// 调用方必须刷新历史投影缓存。
func Rewrite(path string, dropForeign bool) (Result, error) {
	var result Result
	raw, err := os.ReadFile(path)
	if err != nil {
		return result, err
	}
	originalSize := len(raw)
	parts := SplitLines(raw)
	out := make([][]byte, 0, len(parts))

	for _, part := range parts {
		stripped := bytes.TrimSpace(part)
		if len(stripped) == 0 {
			out = append(out, part)
			continue
		}
		obj, ok := DecodeObject(stripped)
		if !ok {
			out = append(out, part)
			continue
		}
		payload, ok := Payload(obj)
		if !ok || payload["type"] != ReasoningType {
			out = append(out, part)
			continue
		}

		content, hasContent := payload["content"].([]any)
		contentPresent := hasContent && len(content) > 0
		foreign := IsForeignEncryptedContent(payload["encrypted_content"])
		if !contentPresent && !foreign {
			out = append(out, part)
			continue
		}
		if dropForeign && foreign {
			result.LinesDropped++
			continue
		}

		body, cr := SplitTrailingCR(part)
		bodyLimit := len(body)

		if contentPresent {
			payload["content"] = []any{}
		}
		if foreign {
			delete(payload, "encrypted_content")
		}
		encoded, err := EncodeObject(obj)
		if err != nil {
			return result, fmt.Errorf("%s: 序列化失败: %w", path, err)
		}
		if len(encoded) > bodyLimit {
			// 这里宁可直接报错，也不写出长度变化的文件：
			// 字节长度一旦变化，Codex 缓存的分页偏移就会失效。
			return result, fmt.Errorf(
				"%s: 无法保持字节长度（原 %d 字节，重写后 %d 字节）；"+
					"请改用 --drop-foreign-reasoning 并接受刷新历史投影",
				path, bodyLimit, len(encoded))
		}
		padded, err := PadLine(encoded, bodyLimit)
		if err != nil {
			return result, fmt.Errorf("%s: %w", path, err)
		}
		result.PaddingBytes += bodyLimit - len(encoded)
		if contentPresent {
			result.ContentEmptied++
		}
		if foreign {
			result.EncryptedStripped++
		}
		out = append(out, append(padded, cr...))
	}

	newRaw := JoinLines(out)
	if !result.SizeChanged() && len(newRaw) != originalSize {
		return result, fmt.Errorf(
			"%s: 拒绝写入，字节长度会变化（%d -> %d）", path, originalSize, len(newRaw))
	}
	if !bytes.Equal(newRaw, raw) {
		if err := os.WriteFile(path, newRaw, 0o644); err != nil {
			return result, err
		}
	}
	return result, nil
}
