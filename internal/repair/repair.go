// Package repair 负责扫描、备份并修复 rollout，同时刷新历史分页投影。
package repair

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/white-x-7/codex-session-doctor/internal/backup"
	"github.com/white-x-7/codex-session-doctor/internal/codex"
	"github.com/white-x-7/codex-session-doctor/internal/history"
	"github.com/white-x-7/codex-session-doctor/internal/rollout"
)

// Plan 是一个待修复 rollout 及其问题统计。
type Plan struct {
	Path  string
	Stats rollout.Stats
}

// Summary 是一次修复的结果汇总。
type Summary struct {
	Backup            string
	Rollouts          int
	ContentEmptied    int
	EncryptedStripped int
	LinesDropped      int
	PaddingBytes      int
	ProjectionRows    int
	Warnings          []string
}

// ApplyOptions 控制修复行为。
type ApplyOptions struct {
	// Home 是 Codex 主目录。
	Home string
	// RefreshHistory 为真时清理历史投影缓存，默认应为真。
	RefreshHistory bool
	// DropForeign 为真时整行删除带非官方加密内容的推理项。
	DropForeign bool
}

// ScanAll 扫描全部 rollout，返回需要修复的清单、扫描总数与警告列表。
func ScanAll(home string) ([]Plan, int, []string) {
	paths, warning := AllRolloutPaths(home)
	plans, scanWarnings := PlanFor(paths)
	if warning != "" {
		scanWarnings = append([]string{warning}, scanWarnings...)
	}
	return plans, len(paths), scanWarnings
}

// PlanFor 过滤出真正需要修复的 rollout。
//
// 读不了的文件不能悄悄跳过：用户会以为"没有需要修复的会话"，实际上只是没读到。
// 这类文件作为警告返回。
func PlanFor(paths []string) ([]Plan, []string) {
	var plans []Plan
	var warnings []string
	for _, path := range paths {
		stats, err := rollout.Scan(path)
		if err != nil {
			warnings = append(warnings, fmt.Sprintf(
				"跳过无法读取的 rollout %s：%v", filepath.Base(path), err))
			continue
		}
		if rollout.NeedsRepair(stats) {
			plans = append(plans, Plan{Path: path, Stats: stats})
		}
	}
	return plans, warnings
}

// AllRolloutPaths 返回全部 rollout 文件。
//
// 扫描 sessions 与 archived_sessions 两棵目录；当 state_5.sqlite 读不到时，
// 返回警告让调用方提示用户可能扫得不全。
func AllRolloutPaths(home string) ([]string, string) {
	seen := map[string]bool{}
	var paths []string
	add := func(path string) {
		if path != "" && !seen[path] {
			seen[path] = true
			paths = append(paths, path)
		}
	}

	for _, dirname := range []string{"sessions", "archived_sessions"} {
		root := filepath.Join(home, dirname)
		_ = filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
			if err != nil || entry.IsDir() {
				return nil
			}
			if strings.HasPrefix(entry.Name(), "rollout-") && strings.HasSuffix(entry.Name(), ".jsonl") {
				add(path)
			}
			return nil
		})
	}

	var warning string
	dbPath := codex.StateDB(home)
	if _, err := os.Stat(dbPath); err == nil {
		db, openErr := codex.OpenReadOnly(dbPath)
		if openErr != nil {
			warning = "state_5.sqlite 暂时无法读取，仅按 sessions 目录扫描；" +
				"若结果不全，请完全退出 Codex（Cmd+Q）后重试。"
		} else {
			recorded, queryErr := codex.RolloutPaths(db)
			_ = db.Close()
			if queryErr != nil {
				warning = "state_5.sqlite 暂时无法读取，仅按 sessions 目录扫描；" +
					"若结果不全，请完全退出 Codex（Cmd+Q）后重试。"
			}
			for _, path := range recorded {
				if info, err := os.Stat(path); err == nil && !info.IsDir() {
					add(path)
				}
			}
		}
	}
	sort.Strings(paths)
	return paths, warning
}

// Resolve 按会话 id 或 rollout 文件名找出全部匹配文件。
//
// 同一个会话可能拥有多个 rollout（初始文件加上每次续写），它们共享会话 id，
// 需要一起修复，因此这里返回全部匹配而不是遇到歧义就报错。
func Resolve(home string, session string) ([]string, error) {
	seen := map[string]bool{}
	var matches []string
	add := func(path string) {
		if path == "" || seen[path] {
			return
		}
		info, err := os.Stat(path)
		if err != nil || info.IsDir() {
			return
		}
		seen[path] = true
		matches = append(matches, path)
	}

	paths, _ := AllRolloutPaths(home)
	for _, path := range paths {
		if strings.Contains(filepath.Base(path), session) {
			add(path)
		}
	}

	dbPath := codex.StateDB(home)
	if _, err := os.Stat(dbPath); err == nil {
		if db, openErr := codex.OpenReadOnly(dbPath); openErr == nil {
			recorded, _ := codex.RolloutPathsForSession(db, session)
			_ = db.Close()
			for _, path := range recorded {
				add(path)
			}
		}
	}

	if len(matches) == 0 {
		return nil, fmt.Errorf(
			"没有匹配到会话 '%s' 的 rollout 文件；请填写完整会话 id"+
				"（例如 019fcb99-1c6f-74c2-a5bb-f390a2013653）或 rollout 文件名", session)
	}
	sort.Strings(matches)
	return matches, nil
}

// Apply 备份并修复给定的 rollout，然后刷新历史投影缓存。
func Apply(plans []Plan, opts ApplyOptions) (Summary, error) {
	var summary Summary
	files := make([]string, 0, len(plans))
	for _, plan := range plans {
		files = append(files, plan.Path)
	}
	dir, err := backup.NewDir(codex.BackupDir(opts.Home), "repair")
	if err != nil {
		return summary, err
	}
	if err := backup.Snapshot(opts.Home, dir, files); err != nil {
		return summary, err
	}
	summary.Backup = dir

	ids := map[string]bool{}
	for _, plan := range plans {
		sizeBefore, err := fileSize(plan.Path)
		if err != nil {
			return summary, withBackup(err, dir)
		}
		result, err := rollout.Rewrite(plan.Path, opts.DropForeign)
		if err != nil {
			return summary, withBackup(err, dir)
		}
		if result.ContentEmptied != plan.Stats.WithContent {
			return summary, withBackup(fmt.Errorf(
				"%s：预期清空 %d 处明文，实际 %d 处",
				filepath.Base(plan.Path), plan.Stats.WithContent, result.ContentEmptied), dir)
		}
		handled := result.EncryptedStripped
		if opts.DropForeign {
			handled = result.LinesDropped
		}
		if handled != plan.Stats.ForeignEncrypted {
			return summary, withBackup(fmt.Errorf(
				"%s：预期处理 %d 处非官方加密内容，实际 %d 处",
				filepath.Base(plan.Path), plan.Stats.ForeignEncrypted, handled), dir)
		}
		if !result.SizeChanged() {
			sizeAfter, err := fileSize(plan.Path)
			if err != nil {
				return summary, withBackup(err, dir)
			}
			if sizeAfter != sizeBefore {
				return summary, withBackup(fmt.Errorf(
					"%s：字节长度发生变化（%d -> %d）",
					filepath.Base(plan.Path), sizeBefore, sizeAfter), dir)
			}
		}
		summary.Rollouts++
		summary.ContentEmptied += result.ContentEmptied
		summary.EncryptedStripped += result.EncryptedStripped
		summary.LinesDropped += result.LinesDropped
		summary.PaddingBytes += result.PaddingBytes
		for _, id := range rollout.ThreadIDs(plan.Path) {
			ids[id] = true
		}
	}

	if opts.RefreshHistory {
		unique := make([]string, 0, len(ids))
		for id := range ids {
			unique = append(unique, id)
		}
		sort.Strings(unique)
		deleted, warnings := history.Refresh(history.DBPath(opts.Home), dir, unique)
		summary.ProjectionRows = deleted
		summary.Warnings = append(summary.Warnings, warnings...)
	}
	return summary, nil
}

// withBackup 在错误里附上快照路径，让用户知道从哪里回滚。
//
// 修复是逐个文件进行的，中途报错时前面几个文件可能已经改写，因此错误信息
// 必须带上回滚点。
func withBackup(err error, dir string) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("%w（本次改动的快照在 %s，可从这里回滚）", err, dir)
}

// fileSize 返回文件大小。
func fileSize(path string) (int64, error) {
	info, err := os.Stat(path)
	if err != nil {
		return 0, err
	}
	return info.Size(), nil
}
