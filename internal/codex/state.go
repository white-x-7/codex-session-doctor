package codex

import (
	"database/sql"
	"net/url"
	"time"

	// 纯 Go 的 SQLite 驱动，不需要 cgo。
	_ "modernc.org/sqlite"
)

// 打开失败时的重试次数与间隔，用于容忍 Codex 短暂重建数据库的瞬间。
const (
	openRetries = 3
	openDelay   = 500 * time.Millisecond
)

// OpenReadOnly 以只读方式打开 state_5.sqlite。
//
// Codex 在运行或刚退出时可能短暂锁住该文件，这里重试若干次后返回错误，
// 让调用方降级处理，而不是直接失败。
func OpenReadOnly(path string) (*sql.DB, error) {
	return openWithRetry(path, "mode=ro")
}

// OpenReadWrite 以读写方式打开数据库，用于清理历史投影缓存。
//
// 使用 mode=rw 而不是 rwc：数据库不存在时应当报错，而不是凭空创建一个空的
// thread_history_*.sqlite 干扰 Codex。
func OpenReadWrite(path string) (*sql.DB, error) {
	return openWithRetry(path, "mode=rw&_pragma=busy_timeout(5000)")
}

func openWithRetry(path string, params string) (*sql.DB, error) {
	var lastErr error
	for attempt := 0; attempt < openRetries; attempt++ {
		db, err := sql.Open("sqlite", dsn(path, params))
		if err != nil {
			lastErr = err
			time.Sleep(openDelay)
			continue
		}
		if pingErr := db.Ping(); pingErr == nil {
			return db, nil
		} else {
			lastErr = pingErr
			_ = db.Close()
		}
		time.Sleep(openDelay)
	}
	return nil, lastErr
}

// dsn 构造 SQLite 的 file: URI，避免路径里的特殊字符被误解析。
func dsn(path string, params string) string {
	u := url.URL{Scheme: "file", Path: path, RawQuery: params}
	return u.String()
}

// RolloutPaths 返回 threads 表里记录的全部 rollout 路径。
func RolloutPaths(db *sql.DB) ([]string, error) {
	return scanPaths(db, "SELECT rollout_path FROM threads")
}

// RolloutPathsForSession 返回与会话 id 或文件名匹配的 rollout 路径。
func RolloutPathsForSession(db *sql.DB, session string) ([]string, error) {
	return scanPaths(db,
		"SELECT rollout_path FROM threads WHERE id = ? OR rollout_path LIKE ?",
		session, "%"+session+"%")
}

// scanPaths 执行一条返回 rollout_path 的查询并收集非空结果。
func scanPaths(db *sql.DB, query string, args ...any) ([]string, error) {
	rows, err := db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var paths []string
	for rows.Next() {
		var raw sql.NullString
		if err := rows.Scan(&raw); err != nil {
			return nil, err
		}
		if raw.Valid && raw.String != "" {
			paths = append(paths, raw.String)
		}
	}
	return paths, rows.Err()
}
