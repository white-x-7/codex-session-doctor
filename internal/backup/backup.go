// Package backup 负责在改动任何文件之前生成可回滚的快照。
package backup

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Entry 是 manifest.json 里记录的一个文件。
type Entry struct {
	SHA256 string `json:"sha256"`
	Backup string `json:"backup"`
}

// NewDir 返回备份根目录下一个带时间戳的快照目录，并按需创建它。
func NewDir(root string, label string) (string, error) {
	dir := filepath.Join(root, fmt.Sprintf("%s-%s", label, time.Now().Format("20060102-150405")))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	return dir, nil
}

// Snapshot 把 files 复制到 dir 下（保持相对 root 的目录结构），并写入 manifest.json。
func Snapshot(root string, dir string, files []string) error {
	manifest := make(map[string]Entry, len(files))
	for _, source := range files {
		rel, err := filepath.Rel(root, source)
		if err != nil || rel == "" || rel == "." || strings.HasPrefix(rel, "..") {
			rel = filepath.Base(source)
		}
		target := filepath.Join(dir, rel)
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		if err := CopyFile(source, target); err != nil {
			return err
		}
		digest, err := SHA256(source)
		if err != nil {
			return err
		}
		manifest[source] = Entry{SHA256: digest, Backup: target}
	}
	payload, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, "manifest.json"), append(payload, '\n'), 0o644)
}

// CopyWithSidecars 复制 SQLite 数据库及其 -wal / -shm 边车文件。
//
// 边车文件里可能还有尚未合并进主库的事务，必须一起复制才能完整回滚。
func CopyWithSidecars(dbPath string, dir string) error {
	var copied bool
	for _, suffix := range []string{"", "-wal", "-shm"} {
		source := dbPath + suffix
		if _, err := os.Stat(source); err != nil {
			continue
		}
		if err := CopyFile(source, filepath.Join(dir, filepath.Base(source))); err != nil {
			return err
		}
		copied = true
	}
	if !copied {
		return fmt.Errorf("没有找到可备份的数据库文件：%s", dbPath)
	}
	return nil
}

// CopyFile 复制单个文件并保留文件权限。
func CopyFile(source string, target string) error {
	in, err := os.Open(source)
	if err != nil {
		return err
	}
	defer in.Close()

	info, err := in.Stat()
	if err != nil {
		return err
	}
	out, err := os.OpenFile(target, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, info.Mode().Perm())
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		_ = out.Close()
		return err
	}
	return out.Close()
}

// SHA256 返回文件的十六进制摘要。
func SHA256(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()
	hasher := sha256.New()
	if _, err := io.Copy(hasher, file); err != nil {
		return "", err
	}
	return hex.EncodeToString(hasher.Sum(nil)), nil
}
