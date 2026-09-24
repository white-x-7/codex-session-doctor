// Package history 处理 Codex 的历史分页投影缓存（thread_history_*.sqlite）。
package history

import (
	"database/sql"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"

	"github.com/white-x-7/codex-session-doctor/internal/backup"
	"github.com/white-x-7/codex-session-doctor/internal/codex"
)

// Tables 是缓存已解析 rollout 的表，按 thread_id 分片。
//
// Codex 会用这些行里的字节偏移定位 rollout，因此重写 rollout 之后必须清掉
// 对应行，让 Codex 重新解析并重算偏移。
var Tables = []string{
	"thread_items",
	"thread_turns",
	"thread_realtime_items",
	"thread_history_projection_state",
}

// 从 thread_history_<版本>.sqlite 里取出版本号。
var namePattern = regexp.MustCompile(`^thread_history_(\d+)\.sqlite$`)

// DBPath 返回版本号最新的 thread_history_*.sqlite，找不到时返回空字符串。
func DBPath(home string) string {
	matches, err := filepath.Glob(filepath.Join(home, "thread_history_*.sqlite"))
	if err != nil || len(matches) == 0 {
		return ""
	}
	sort.Slice(matches, func(i, j int) bool {
		left, right := versionOf(matches[i]), versionOf(matches[j])
		if left != right {
			return left < right
		}
		return matches[i] < matches[j]
	})
	return matches[len(matches)-1]
}

// versionOf 解析文件名里的版本号，解析失败返回 -1。
func versionOf(path string) int {
	match := namePattern.FindStringSubmatch(filepath.Base(path))
	if len(match) != 2 {
		return -1
	}
	value, err := strconv.Atoi(match[1])
	if err != nil {
		return -1
	}
	return value
}

// Refresh 删除命中会话的缓存投影行。
//
// 返回删除的行数与警告列表。警告不阻断流程：此时 rollout 本身已经修好，
// 最差情况只是 Codex 需要重新解析一次历史。
//
// 备份失败时直接返回警告并保持数据库原样，避免在没有回滚点的情况下改动它。
func Refresh(dbPath string, snapDir string, ids []string) (int, []string) {
	if dbPath == "" || len(ids) == 0 {
		return 0, nil
	}
	if _, err := os.Stat(dbPath); err != nil {
		return 0, nil
	}
	if snapDir != "" {
		if err := backup.CopyWithSidecars(dbPath, snapDir); err != nil {
			return 0, []string{"历史库备份失败，已跳过投影清理：" + err.Error()}
		}
	}

	db, err := codex.OpenReadWrite(dbPath)
	if err != nil {
		return 0, []string{"无法打开 " + filepath.Base(dbPath) + "：" + err.Error()}
	}
	defer db.Close()

	present, err := existingTables(db)
	if err != nil {
		return 0, []string{"读取 " + filepath.Base(dbPath) + " 失败：" + err.Error()}
	}

	tx, err := db.Begin()
	if err != nil {
		return 0, []string{"无法开始事务：" + err.Error()}
	}
	deleted := 0
	for _, table := range Tables {
		if !present[table] {
			continue
		}
		for _, id := range ids {
			result, err := tx.Exec("DELETE FROM "+table+" WHERE thread_id = ?", id)
			if err != nil {
				_ = tx.Rollback()
				return 0, []string{"清理历史投影失败：" + err.Error()}
			}
			if affected, err := result.RowsAffected(); err == nil && affected > 0 {
				deleted += int(affected)
			}
		}
	}
	if err := tx.Commit(); err != nil {
		_ = tx.Rollback()
		return 0, []string{"提交历史投影清理失败：" + err.Error()}
	}
	return deleted, nil
}

// existingTables 返回数据库里存在的表名集合。
func existingTables(db *sql.DB) (map[string]bool, error) {
	rows, err := db.Query("SELECT name FROM sqlite_master WHERE type='table'")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	present := map[string]bool{}
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, err
		}
		present[name] = true
	}
	return present, rows.Err()
}
