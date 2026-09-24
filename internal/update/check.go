// Package update 检查 codex-session-doctor 的可用版本。
package update

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const (
	// LatestReleaseEndpoint 是 GitHub 官方的最新稳定 release API。
	LatestReleaseEndpoint = "https://api.github.com/repos/white-x-7/codex-session-doctor/releases/latest"
	// TagsEndpoint 用于仓库尚未发布 release 时读取版本标签。
	TagsEndpoint = "https://api.github.com/repos/white-x-7/codex-session-doctor/tags?per_page=30"
	// DefaultTimeout 限制一次更新检查最多占用的时间。
	DefaultTimeout  = 10 * time.Second
	maxResponseSize = 1 << 20
)

// Result 是一次更新检查的结果。
type Result struct {
	Current         string
	Latest          string
	URL             string
	UpdateAvailable bool
	Source          string
}

type releaseResponse struct {
	TagName string `json:"tag_name"`
	HTMLURL string `json:"html_url"`
}

type tagResponse struct {
	Name string `json:"name"`
}

type version struct {
	major      int
	minor      int
	patch      int
	prerelease []string
}

// Check 查询最新稳定 release；仓库没有 release 时回退到版本 tags。
// client 只用于依赖注入和测试，nil 时使用带默认超时的客户端。
func Check(ctx context.Context, client *http.Client, current string) (Result, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	_, hasDeadline := ctx.Deadline()
	if !hasDeadline {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, DefaultTimeout)
		defer cancel()
	}
	if _, err := parseVersion(current); err != nil {
		return Result{}, fmt.Errorf("当前版本无效：%w", err)
	}
	if client == nil {
		client = &http.Client{}
		if !hasDeadline {
			client.Timeout = DefaultTimeout
		}
	}
	// 固定 API 地址不需要跟随重定向，避免请求被带到非 GitHub 主机。
	safeClient := *client
	safeClient.CheckRedirect = func(*http.Request, []*http.Request) error {
		return errors.New("更新 API 不允许重定向")
	}
	client = &safeClient

	latest, releaseURL, err := fetchRelease(ctx, client)
	source := "release"
	if errors.Is(err, errNoRelease) {
		latest, releaseURL, err = fetchTag(ctx, client)
		source = "tag"
	}
	if err != nil {
		return Result{}, err
	}

	comparison, err := Compare(current, latest)
	if err != nil {
		return Result{}, fmt.Errorf("最新版本无效：%w", err)
	}
	return Result{
		Current:         current,
		Latest:          latest,
		URL:             releaseURL,
		UpdateAvailable: comparison < 0,
		Source:          source,
	}, nil
}

// Compare 按 SemVer 比较两个版本，允许可选的 v 前缀和预发布标识。
// 返回 -1 表示 current 较旧，0 表示相同，1 表示 current 较新。
func Compare(current, latest string) (int, error) {
	left, err := parseVersion(current)
	if err != nil {
		return 0, fmt.Errorf("版本 %q 无效：%w", current, err)
	}
	right, err := parseVersion(latest)
	if err != nil {
		return 0, fmt.Errorf("版本 %q 无效：%w", latest, err)
	}
	return compareVersions(left, right), nil
}

var errNoRelease = errors.New("仓库尚未发布稳定 release")

func fetchRelease(ctx context.Context, client *http.Client) (string, string, error) {
	var payload releaseResponse
	status, err := getJSON(ctx, client, LatestReleaseEndpoint, &payload)
	if err != nil {
		return "", "", err
	}
	if status == http.StatusNotFound || strings.TrimSpace(payload.TagName) == "" {
		return "", "", errNoRelease
	}
	if status < http.StatusOK || status >= http.StatusMultipleChoices {
		return "", "", fmt.Errorf("GitHub release API 返回 HTTP %d", status)
	}
	return strings.TrimSpace(payload.TagName), strings.TrimSpace(payload.HTMLURL), nil
}

func fetchTag(ctx context.Context, client *http.Client) (string, string, error) {
	var payload []tagResponse
	status, err := getJSON(ctx, client, TagsEndpoint, &payload)
	if err != nil {
		return "", "", err
	}
	if status < http.StatusOK || status >= http.StatusMultipleChoices {
		return "", "", fmt.Errorf("GitHub tags API 返回 HTTP %d", status)
	}

	var best version
	var bestName string
	for _, tag := range payload {
		name := strings.TrimSpace(tag.Name)
		parsed, parseErr := parseVersion(name)
		if parseErr != nil {
			continue
		}
		if bestName == "" || compareVersions(best, parsed) < 0 {
			best = parsed
			bestName = name
		}
	}
	if bestName == "" {
		return "", "", errors.New("GitHub 仓库没有可识别的版本 tag")
	}
	return bestName, "https://github.com/white-x-7/codex-session-doctor/releases/tag/" + url.PathEscape(bestName), nil
}

func getJSON(ctx context.Context, client *http.Client, endpoint string, target any) (int, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return 0, fmt.Errorf("创建更新请求失败：%w", err)
	}
	request.Header.Set("Accept", "application/vnd.github+json")
	request.Header.Set("User-Agent", "codex-session-doctor")
	response, err := client.Do(request)
	if err != nil {
		return 0, fmt.Errorf("检查更新失败：%w", err)
	}
	defer response.Body.Close()

	limited := io.LimitReader(response.Body, maxResponseSize+1)
	body, err := io.ReadAll(limited)
	if err != nil {
		return response.StatusCode, fmt.Errorf("读取更新响应失败：%w", err)
	}
	if len(body) > maxResponseSize {
		return response.StatusCode, errors.New("更新响应超过 1 MiB，已拒绝解析")
	}
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		if response.StatusCode == http.StatusNotFound {
			return response.StatusCode, nil
		}
		return response.StatusCode, fmt.Errorf("GitHub API 返回 HTTP %d", response.StatusCode)
	}
	if err := json.Unmarshal(body, target); err != nil {
		return response.StatusCode, fmt.Errorf("解析 GitHub 响应失败：%w", err)
	}
	return response.StatusCode, nil
}

func parseVersion(raw string) (version, error) {
	value := strings.TrimSpace(raw)
	value = strings.TrimPrefix(value, "v")
	if value == "" {
		return version{}, errors.New("为空")
	}
	if buildIndex := strings.IndexByte(value, '+'); buildIndex >= 0 {
		build := value[buildIndex+1:]
		if build == "" {
			return version{}, errors.New("构建元数据不能为空")
		}
		for _, identifier := range strings.Split(build, ".") {
			if identifier == "" || !validIdentifier(identifier) {
				return version{}, errors.New("构建元数据标识无效")
			}
		}
		value = value[:buildIndex]
	}
	parts := strings.SplitN(value, "-", 2)
	core := strings.Split(parts[0], ".")
	if len(core) != 3 {
		return version{}, errors.New("需要 major.minor.patch 三段")
	}
	parsed := version{}
	values := []*int{&parsed.major, &parsed.minor, &parsed.patch}
	for index, component := range core {
		if component == "" || !isNumeric(component) || (len(component) > 1 && component[0] == '0') {
			return version{}, errors.New("数字段必须是十进制整数且不能有前导零")
		}
		number, err := strconv.Atoi(component)
		if err != nil || number < 0 {
			return version{}, errors.New("数字段不是非负整数")
		}
		*values[index] = number
	}
	if len(parts) == 2 {
		for _, identifier := range strings.Split(parts[1], ".") {
			if identifier == "" || !validIdentifier(identifier) || (isNumeric(identifier) && len(identifier) > 1 && identifier[0] == '0') {
				return version{}, errors.New("预发布标识无效")
			}
			parsed.prerelease = append(parsed.prerelease, identifier)
		}
	}
	return parsed, nil
}

func compareVersions(left, right version) int {
	for _, pair := range [][2]int{{left.major, right.major}, {left.minor, right.minor}, {left.patch, right.patch}} {
		if pair[0] < pair[1] {
			return -1
		}
		if pair[0] > pair[1] {
			return 1
		}
	}
	if len(left.prerelease) == 0 && len(right.prerelease) == 0 {
		return 0
	}
	if len(left.prerelease) == 0 {
		return 1
	}
	if len(right.prerelease) == 0 {
		return -1
	}
	for index := 0; index < len(left.prerelease) && index < len(right.prerelease); index++ {
		leftID, rightID := left.prerelease[index], right.prerelease[index]
		leftNumeric, rightNumeric := isNumeric(leftID), isNumeric(rightID)
		if leftNumeric && rightNumeric {
			leftNumber, _ := strconv.Atoi(leftID)
			rightNumber, _ := strconv.Atoi(rightID)
			if leftNumber < rightNumber {
				return -1
			}
			if leftNumber > rightNumber {
				return 1
			}
			continue
		}
		if leftNumeric != rightNumeric {
			if leftNumeric {
				return -1
			}
			return 1
		}
		if leftID < rightID {
			return -1
		}
		if leftID > rightID {
			return 1
		}
	}
	if len(left.prerelease) < len(right.prerelease) {
		return -1
	}
	if len(left.prerelease) > len(right.prerelease) {
		return 1
	}
	return 0
}

func isNumeric(value string) bool {
	if value == "" {
		return false
	}
	for _, character := range value {
		if character < '0' || character > '9' {
			return false
		}
	}
	return true
}

func validIdentifier(value string) bool {
	for _, character := range value {
		if (character < '0' || character > '9') &&
			(character < 'A' || character > 'Z') &&
			(character < 'a' || character > 'z') && character != '-' {
			return false
		}
	}
	return value != ""
}
