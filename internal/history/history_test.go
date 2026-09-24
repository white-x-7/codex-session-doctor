package history

import (
	"database/sql"
	"os"
	"path/filepath"
	"testing"

	_ "modernc.org/sqlite"
)

func TestDBPathPicksNewestVersion(t *testing.T) {
	home := t.TempDir()
	for _, name := range []string{"thread_history_1.sqlite", "thread_history_10.sqlite", "thread_history_2.sqlite"} {
		writeEmptyFile(t, filepath.Join(home, name))
	}
	if got := filepath.Base(DBPath(home)); got != "thread_history_10.sqlite" {
		t.Fatalf("期望选中版本号最大的库，实际 %s", got)
	}

	empty := t.TempDir()
	if got := DBPath(empty); got != "" {
		t.Fatalf("没有历史库时应返回空字符串，实际 %s", got)
	}
}

func TestRefreshRemovesOnlyMatchingIDs(t *testing.T) {
	home := t.TempDir()
	dbPath := filepath.Join(home, "thread_history_1.sqlite")
	createProjectionDB(t, dbPath, []string{"parent", "segment", "keep"})

	snapDir := filepath.Join(home, "snapshot")
	if err := os.MkdirAll(snapDir, 0o755); err != nil {
		t.Fatalf("创建快照目录失败：%v", err)
	}

	deleted, warnings := Refresh(dbPath, snapDir, []string{"parent", "segment"})
	if len(warnings) != 0 {
		t.Fatalf("不应有警告：%v", warnings)
	}
	if want := len(Tables) * 2; deleted != want {
		t.Fatalf("期望删除 %d 行，实际 %d", want, deleted)
	}

	remaining := queryIDs(t, dbPath, "thread_items")
	if len(remaining) != 1 || remaining[0] != "keep" {
		t.Fatalf("只应保留无关行，实际 %v", remaining)
	}

	// 侧车文件与数据库本体都要进快照，否则回滚会丢掉未合并的事务。
	if _, err := os.Stat(filepath.Join(snapDir, "thread_history_1.sqlite")); err != nil {
		t.Fatalf("快照里缺少数据库本体：%v", err)
	}
	writeEmptyFile(t, dbPath+"-wal")
	if _, warnings := Refresh(dbPath, snapDir, []string{"keep"}); len(warnings) != 0 {
		t.Fatalf("第二次刷新不应有警告：%v", warnings)
	}
	if _, err := os.Stat(filepath.Join(snapDir, "thread_history_1.sqlite-wal")); err != nil {
		t.Fatalf("快照里缺少 -wal 侧车文件：%v", err)
	}
}

func TestRefreshWithoutBackupLeavesDatabaseUntouched(t *testing.T) {
	home := t.TempDir()
	dbPath := filepath.Join(home, "thread_history_1.sqlite")
	createProjectionDB(t, dbPath, []string{"only"})

	// 备份目录不可用时（这里给了一个不存在的父路径作为文件），必须放弃清理。
	blocker := filepath.Join(home, "blocker")
	writeEmptyFile(t, blocker)
	deleted, warnings := Refresh(dbPath, blocker, []string{"only"})
	if deleted != 0 || len(warnings) == 0 {
		t.Fatalf("备份失败时应当放弃清理并给出警告：deleted=%d warnings=%v", deleted, warnings)
	}
	if remaining := queryIDs(t, dbPath, "thread_items"); len(remaining) != 1 {
		t.Fatalf("数据库不应被改动，实际剩余 %v", remaining)
	}
}

// createProjectionDB 建一个包含全部投影表的历史库。
func createProjectionDB(t *testing.T, path string, ids []string) {
	t.Helper()
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatalf("打开数据库失败：%v", err)
	}
	defer db.Close()
	for _, table := range Tables {
		if _, err := db.Exec("CREATE TABLE " + table + " (thread_id TEXT, value TEXT)"); err != nil {
			t.Fatalf("建表 %s 失败：%v", table, err)
		}
		for _, id := range ids {
			if _, err := db.Exec("INSERT INTO "+table+" VALUES (?, ?)", id, "old"); err != nil {
				t.Fatalf("插入 %s 失败：%v", table, err)
			}
		}
	}
}

// queryIDs 读取某个投影表里剩下的 thread_id。
func queryIDs(t *testing.T, path string, table string) []string {
	t.Helper()
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatalf("打开数据库失败：%v", err)
	}
	defer db.Close()
	rows, err := db.Query("SELECT thread_id FROM " + table + " ORDER BY thread_id")
	if err != nil {
		t.Fatalf("查询失败：%v", err)
	}
	defer rows.Close()
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			t.Fatalf("扫描失败：%v", err)
		}
		ids = append(ids, id)
	}
	return ids
}

// writeEmptyFile 创建一个空文件。
func writeEmptyFile(t *testing.T, path string) {
	t.Helper()
	if err := os.WriteFile(path, nil, 0o644); err != nil {
		t.Fatalf("创建 %s 失败：%v", path, err)
	}
}
