package cli

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	_ "modernc.org/sqlite"
)

const (
	singleID  = "01a0c369-caf2-71c1-bdf1-a1f56cd46f00"
	segmentID = "01a0c7fb-21c6-75e1-b1a2-eca31fd2b521"
	parentID  = "01a0c36a-72cf-7ae2-a0af-7dc38897d981"
	keepID    = "01a0ffff-0000-7000-8000-000000000000"
	foreign   = "8ec8e468-e6a3-4513-a1f0-9fbb4f189148-0"
	official  = "gAAAAABofficial-example"
)

// fixture 是一个模拟的 Codex 主目录。
type fixture struct {
	home        string
	singlePath  string
	segmentPath string
}

// newFixture 建好 state 数据库、两个 rollout 与一个历史投影库。
func newFixture(t *testing.T) fixture {
	t.Helper()
	home := t.TempDir()
	sessions := filepath.Join(home, "sessions", "2026", "09")
	if err := os.MkdirAll(sessions, 0o755); err != nil {
		t.Fatalf("创建 sessions 目录失败：%v", err)
	}

	singlePath := filepath.Join(sessions, "rollout-2026-09-21T18-01-25-"+singleID+".jsonl")
	writeFile(t, singlePath, rollout(singleID, 1))

	segmentPath := filepath.Join(sessions, "rollout-2026-09-22T09-21-19-"+parentID+"_"+segmentID+".jsonl")
	writeFile(t, segmentPath, rollout(parentID, 2))

	path := filepath.Join(home, "state_5.sqlite")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatalf("打开 state 数据库失败：%v", err)
	}
	if _, err := db.Exec("CREATE TABLE threads (id TEXT PRIMARY KEY, rollout_path TEXT)"); err != nil {
		t.Fatalf("建表失败：%v", err)
	}
	for _, row := range [][2]string{{singleID, singlePath}, {parentID, segmentPath}} {
		if _, err := db.Exec("INSERT INTO threads VALUES (?, ?)", row[0], row[1]); err != nil {
			t.Fatalf("插入 threads 失败：%v", err)
		}
	}
	_ = db.Close()

	projection := filepath.Join(home, "thread_history_1.sqlite")
	pdb, err := sql.Open("sqlite", projection)
	if err != nil {
		t.Fatalf("打开历史库失败：%v", err)
	}
	for _, table := range []string{"thread_items", "thread_turns", "thread_realtime_items", "thread_history_projection_state"} {
		if _, err := pdb.Exec("CREATE TABLE " + table + " (thread_id TEXT, value TEXT)"); err != nil {
			t.Fatalf("建投影表失败：%v", err)
		}
		for _, id := range []string{singleID, parentID, segmentID, keepID} {
			if _, err := pdb.Exec("INSERT INTO "+table+" VALUES (?, 'old')", id); err != nil {
				t.Fatalf("写入投影行失败：%v", err)
			}
		}
	}
	_ = pdb.Close()

	return fixture{home: home, singlePath: singlePath, segmentPath: segmentPath}
}

// run 执行一次 CLI。
func (f fixture) run(t *testing.T, running bool, args ...string) (int, string, string) {
	t.Helper()
	var stdout, stderr bytes.Buffer
	code := Run(Env{
		Args:      args,
		Stdout:    &stdout,
		Stderr:    &stderr,
		Stdin:     strings.NewReader(""),
		Home:      f.home,
		IsRunning: func(string) bool { return running },
	})
	return code, stdout.String(), stderr.String()
}

func TestDoctorReportsPendingRepairs(t *testing.T) {
	f := newFixture(t)
	code, stdout, stderr := f.run(t, false, "doctor")
	if code != ExitOK {
		t.Fatalf("doctor 应当成功，实际退出码 %d，stderr=%s", code, stderr)
	}
	if !strings.Contains(stdout, "rollout 总数 : 2 个（其中需要修复：2 个）") {
		t.Errorf("doctor 未报告待修数量：\n%s", stdout)
	}
	if !strings.Contains(stdout, "进程状态     : 未运行") {
		t.Errorf("doctor 未报告进程状态：\n%s", stdout)
	}
}

func TestDryRunDoesNotTouchFiles(t *testing.T) {
	f := newFixture(t)
	before := readFile(t, f.singlePath)

	code, stdout, stderr := f.run(t, false, "repair", singleID, "--dry-run")
	if code != ExitOK {
		t.Fatalf("预演应当成功，实际退出码 %d，stderr=%s", code, stderr)
	}
	if !strings.Contains(stdout, "预演完成：没有修改任何文件。") {
		t.Errorf("缺少预演提示：\n%s", stdout)
	}
	if readFile(t, f.singlePath) != before {
		t.Error("预演不应改动文件")
	}
}

func TestRepairKeepsByteLengthAndRefreshesProjection(t *testing.T) {
	f := newFixture(t)
	sizeBefore := fileSize(t, f.singlePath)

	code, stdout, stderr := f.run(t, false, "repair", singleID, "-y")
	if code != ExitOK {
		t.Fatalf("修复应当成功，实际退出码 %d，stderr=%s", code, stderr)
	}
	if !strings.Contains(stdout, "字节长度保持不变") {
		t.Errorf("未报告字节长度保持：\n%s", stdout)
	}
	if got := fileSize(t, f.singlePath); got != sizeBefore {
		t.Fatalf("字节长度变化：%d -> %d", sizeBefore, got)
	}
	content := readFile(t, f.singlePath)
	if strings.Contains(content, foreign) {
		t.Error("非官方密文没有被移除")
	}
	if !strings.Contains(content, `"content":[]`) {
		t.Errorf("推理明文没有被清空：\n%s", content)
	}
	if !strings.Contains(content, official) {
		t.Error("官方密文不应被移除")
	}
	if rows := projectionRows(t, f.home, singleID); rows != 0 {
		t.Errorf("命中会话的投影行应被清理，实际剩余 %d 行", rows)
	}
	if rows := projectionRows(t, f.home, keepID); rows == 0 {
		t.Error("无关会话的投影行不应被清理")
	}
}

func TestRepairAllHandlesSegmentedRollout(t *testing.T) {
	f := newFixture(t)
	sizeBefore := fileSize(t, f.segmentPath)

	code, stdout, stderr := f.run(t, false, "repair", "--all", "-y")
	if code != ExitOK {
		t.Fatalf("修复全部应当成功，实际退出码 %d，stderr=%s", code, stderr)
	}
	if !strings.Contains(stdout, "已修复 2 个 rollout") {
		t.Errorf("未报告修复数量：\n%s", stdout)
	}
	if got := fileSize(t, f.segmentPath); got != sizeBefore {
		t.Fatalf("分段 rollout 字节长度变化：%d -> %d", sizeBefore, got)
	}
	for _, id := range []string{parentID, segmentID, singleID} {
		if rows := projectionRows(t, f.home, id); rows != 0 {
			t.Errorf("会话 %s 的投影行应被清理，实际剩余 %d 行", id, rows)
		}
	}
	if rows := projectionRows(t, f.home, keepID); rows == 0 {
		t.Error("无关会话的投影行不应被清理")
	}
}

func TestRepairRefusesWhileCodexIsRunning(t *testing.T) {
	f := newFixture(t)
	before := readFile(t, f.singlePath)

	code, _, stderr := f.run(t, true, "repair", singleID, "-y")
	if code != ExitBusy {
		t.Fatalf("运行中应当拒绝写入并返回 %d，实际 %d", ExitBusy, code)
	}
	if !strings.Contains(stderr, "Codex 仍在运行") {
		t.Errorf("缺少运行中提示：\n%s", stderr)
	}
	if readFile(t, f.singlePath) != before {
		t.Error("被拒绝时不应改动文件")
	}
}

func TestRepairForceProceedsWhileRunning(t *testing.T) {
	f := newFixture(t)
	code, _, stderr := f.run(t, true, "repair", singleID, "-y", "--force")
	if code != ExitOK {
		t.Fatalf("加了 --force 应当继续，实际退出码 %d，stderr=%s", code, stderr)
	}
}

func TestRepairUnknownSession(t *testing.T) {
	f := newFixture(t)
	code, _, stderr := f.run(t, false, "repair", "no-such-session", "-y")
	if code != ExitFailure {
		t.Fatalf("未知会话应返回 %d，实际 %d", ExitFailure, code)
	}
	if !strings.Contains(stderr, "没有匹配到会话") {
		t.Errorf("缺少错误提示：\n%s", stderr)
	}
}

func TestRepairUsageErrors(t *testing.T) {
	f := newFixture(t)
	cases := []struct {
		name string
		args []string
	}{
		{"缺少目标", []string{"repair"}},
		{"同时指定会话与全部", []string{"repair", singleID, "--all"}},
		{"删除整行却不刷新投影", []string{"repair", singleID, "--drop-foreign-reasoning", "--no-refresh-history"}},
	}
	for _, item := range cases {
		code, _, _ := f.run(t, false, item.args...)
		if code != ExitUsage {
			t.Errorf("%s：期望退出码 %d，实际 %d", item.name, ExitUsage, code)
		}
	}
}

func TestRepairNoChangesNeeded(t *testing.T) {
	f := newFixture(t)
	if code, _, stderr := f.run(t, false, "repair", singleID, "-y"); code != ExitOK {
		t.Fatalf("第一次修复失败：%d %s", code, stderr)
	}
	code, stdout, stderr := f.run(t, false, "repair", singleID, "-y")
	if code != ExitOK {
		t.Fatalf("重复修复应当成功，实际 %d，stderr=%s", code, stderr)
	}
	if !strings.Contains(stdout, "无需修复") {
		t.Errorf("重复修复应报告无需修复：\n%s", stdout)
	}
}

func TestVersionAndUnknownCommand(t *testing.T) {
	f := newFixture(t)
	code, stdout, _ := f.run(t, false, "version")
	if code != ExitOK || !strings.Contains(stdout, programName) {
		t.Errorf("version 输出异常：%d %q", code, stdout)
	}
	if code, _, _ := f.run(t, false, "nope"); code != ExitUsage {
		t.Errorf("未知子命令应返回 %d，实际 %d", ExitUsage, code)
	}
}

func TestHelpCommands(t *testing.T) {
	f := newFixture(t)
	cases := []struct {
		name       string
		args       []string
		wantOutput string
		wantCode   int
	}{
		{name: "top level", args: []string{"help"}, wantOutput: "check-update", wantCode: ExitOK},
		{name: "repair topic", args: []string{"help", "repair"}, wantOutput: "--drop-foreign-reasoning", wantCode: ExitOK},
		{name: "repair flag", args: []string{"repair", "--help"}, wantOutput: "用法：", wantCode: ExitOK},
		{name: "doctor flag", args: []string{"doctor", "--help"}, wantOutput: "只读检查", wantCode: ExitOK},
	}
	for _, item := range cases {
		t.Run(item.name, func(t *testing.T) {
			code, stdout, stderr := f.run(t, false, item.args...)
			if code != item.wantCode {
				t.Fatalf("退出码 = %d，stderr=%s", code, stderr)
			}
			if !strings.Contains(stdout, item.wantOutput) {
				t.Fatalf("帮助输出缺少 %q：\n%s", item.wantOutput, stdout)
			}
		})
	}

	if code, _, stderr := f.run(t, false, "help", "no-such-command"); code != ExitUsage || !strings.Contains(stderr, "未知命令") {
		t.Fatalf("未知帮助主题应返回用法错误，实际 code=%d stderr=%s", code, stderr)
	}
}

func TestCheckUpdateCommand(t *testing.T) {
	f := newFixture(t)
	var stdout, stderr bytes.Buffer
	env := Env{
		Args:   []string{"check-update"},
		Home:   f.home,
		Stdout: &stdout,
		Stderr: &stderr,
		UpdateClient: &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
			if request.URL.String() != "https://api.github.com/repos/white-x-7/codex-session-doctor/releases/latest" {
				t.Fatalf("更新检查请求了未知地址：%s", request.URL)
			}
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(strings.NewReader(`{"tag_name":"v0.2.0","html_url":"https://example.test/release"}`)),
				Header:     make(http.Header),
			}, nil
		})},
	}
	if code := Run(env); code != ExitOK {
		t.Fatalf("check-update 应成功，实际 %d，stderr=%s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "发现新版本") || !strings.Contains(stdout.String(), "https://example.test/release") {
		t.Fatalf("更新输出异常：\n%s", stdout.String())
	}
	if strings.Contains(stdout.String(), "来源：release") {
		t.Fatalf("用户可见来源不应保留英文枚举值：\n%s", stdout.String())
	}

	stdout.Reset()
	stderr.Reset()
	env.UpdateClient = &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader(`{"tag_name":"v0.1.0","html_url":"https://example.test/release"}`)),
			Header:     make(http.Header),
		}, nil
	})}
	if code := Run(env); code != ExitOK || !strings.Contains(stdout.String(), "已经是最新版本") {
		t.Fatalf("已是最新版本的输出异常：code=%d stdout=%s stderr=%s", code, stdout.String(), stderr.String())
	}

	stdout.Reset()
	stderr.Reset()
	env.UpdateClient = &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		return nil, fmt.Errorf("测试网络错误")
	})}
	if code := Run(env); code != ExitFailure || !strings.Contains(stderr.String(), "检查更新失败") {
		t.Fatalf("网络错误应返回失败，实际 code=%d stdout=%s stderr=%s", code, stdout.String(), stderr.String())
	}

	stdout.Reset()
	stderr.Reset()
	env.Args = []string{"check-update", "--timeout", "0"}
	if code := Run(env); code != ExitUsage || !strings.Contains(stderr.String(), "1 到 300") {
		t.Fatalf("非法超时应返回用法错误，实际 %d，stderr=%s", code, stderr.String())
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (fn roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return fn(request)
}

// rollout 生成 brokenCount 条待修推理项，外加一条已经兼容官方接口的推理项。
//
// 官方那一行用于验证修复不会误伤本来就是官方格式的历史。
func rollout(sessionID string, brokenCount int) string {
	lines := []string{string(mustJSON(map[string]any{
		"type":    "session_meta",
		"payload": map[string]any{"session_id": sessionID},
	}))}
	lines = append(lines, `{"type":"event_msg","payload":{"type":"user_message","content":"你好"}}`)
	for index := 0; index < brokenCount; index++ {
		payload := map[string]any{
			"type":              "reasoning",
			"id":                "reasoning-broken-" + string(rune('a'+index)),
			"summary":           []any{},
			"content":           []any{map[string]any{"type": "reasoning_text", "text": "thinking"}},
			"encrypted_content": foreign,
		}
		lines = append(lines, string(mustJSON(map[string]any{
			"type":    "response_item",
			"payload": payload,
		})))
	}
	lines = append(lines, string(mustJSON(map[string]any{
		"type": "response_item",
		"payload": map[string]any{
			"type":              "reasoning",
			"id":                "reasoning-official",
			"summary":           []any{},
			"content":           []any{},
			"encrypted_content": official,
		},
	})))
	return strings.Join(lines, "\n") + "\n"
}

// mustJSON 序列化对象，失败时返回空对象。
func mustJSON(obj map[string]any) []byte {
	data, err := json.Marshal(obj)
	if err != nil {
		return []byte("{}")
	}
	return data
}

// projectionRows 统计某个会话在所有投影表里剩下的行数。
func projectionRows(t *testing.T, home string, threadID string) int {
	t.Helper()
	db, err := sql.Open("sqlite", filepath.Join(home, "thread_history_1.sqlite"))
	if err != nil {
		t.Fatalf("打开历史库失败：%v", err)
	}
	defer db.Close()
	total := 0
	for _, table := range []string{"thread_items", "thread_turns", "thread_realtime_items", "thread_history_projection_state"} {
		var count int
		if err := db.QueryRow("SELECT COUNT(*) FROM "+table+" WHERE thread_id = ?", threadID).Scan(&count); err != nil {
			t.Fatalf("统计 %s 失败：%v", table, err)
		}
		total += count
	}
	return total
}

// readFile 读取文件内容。
func readFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("读取 %s 失败：%v", path, err)
	}
	return string(data)
}

// writeFile 写入文件内容。
func writeFile(t *testing.T, path string, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("写入 %s 失败：%v", path, err)
	}
}

// fileSize 返回文件大小。
func fileSize(t *testing.T, path string) int64 {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("读取 %s 失败：%v", path, err)
	}
	return info.Size()
}
