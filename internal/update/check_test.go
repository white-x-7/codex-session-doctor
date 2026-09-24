package update

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (fn roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return fn(request)
}

func jsonResponse(status int, body string) *http.Response {
	return &http.Response{
		StatusCode: status,
		Body:       io.NopCloser(strings.NewReader(body)),
		Header:     make(http.Header),
	}
}

func TestCompareSupportsStableAndPrereleaseVersions(t *testing.T) {
	cases := []struct {
		name    string
		current string
		latest  string
		want    int
	}{
		{name: "older with v prefix", current: "v0.1.0", latest: "0.2.0", want: -1},
		{name: "same", current: "0.2.0", latest: "v0.2.0", want: 0},
		{name: "newer", current: "0.3.0", latest: "0.2.0", want: 1},
		{name: "stable beats prerelease", current: "1.0.0", latest: "1.0.0-rc.1", want: 1},
		{name: "prerelease ordering", current: "1.0.0-rc.1", latest: "1.0.0-rc.2", want: -1},
	}
	for _, item := range cases {
		t.Run(item.name, func(t *testing.T) {
			got, err := Compare(item.current, item.latest)
			if err != nil {
				t.Fatalf("Compare 返回错误：%v", err)
			}
			if got != item.want {
				t.Fatalf("Compare(%q, %q) = %d，期望 %d", item.current, item.latest, got, item.want)
			}
		})
	}
}

func TestCompareRejectsInvalidVersion(t *testing.T) {
	for _, item := range [][2]string{{"0.1", "0.2.0"}, {"0.1.0", "latest"}, {"0.01.0", "0.2.0"}, {"0.1.0", "0.2.0-"}, {"0.1.0+", "0.2.0"}, {"0.1.0", "0.2.0-rc_1"}} {
		if _, err := Compare(item[0], item[1]); err == nil {
			t.Errorf("Compare(%q, %q) 应拒绝非法版本", item[0], item[1])
		}
	}
}

func TestCheckReadsLatestRelease(t *testing.T) {
	client := &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		if request.URL.String() != LatestReleaseEndpoint {
			t.Fatalf("请求了未知地址：%s", request.URL)
		}
		if request.Header.Get("Accept") == "" || request.Header.Get("User-Agent") == "" {
			t.Fatal("更新请求缺少必要请求头")
		}
		return jsonResponse(http.StatusOK, `{"tag_name":"v0.2.0","html_url":"https://github.com/white-x-7/codex-session-doctor/releases/tag/v0.2.0"}`), nil
	})}

	result, err := Check(context.Background(), client, "0.1.0")
	if err != nil {
		t.Fatalf("Check 返回错误：%v", err)
	}
	if !result.UpdateAvailable || result.Latest != "v0.2.0" || result.Source != "release" {
		t.Fatalf("更新结果异常：%+v", result)
	}
}

func TestCheckFallsBackToTagsWhenNoRelease(t *testing.T) {
	client := &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		switch request.URL.String() {
		case LatestReleaseEndpoint:
			return jsonResponse(http.StatusNotFound, `{}`), nil
		case TagsEndpoint:
			return jsonResponse(http.StatusOK, `[{"name":"not-a-version"},{"name":"v0.3.0-rc.1"},{"name":"v0.2.0"}]`), nil
		default:
			t.Fatalf("请求了未知地址：%s", request.URL)
			return nil, nil
		}
	})}

	result, err := Check(context.Background(), client, "0.2.0")
	if err != nil {
		t.Fatalf("Check 返回错误：%v", err)
	}
	if !result.UpdateAvailable || result.Latest != "v0.3.0-rc.1" || result.Source != "tag" {
		t.Fatalf("tags 回退结果异常：%+v", result)
	}
}

func TestCheckRejectsServerErrorAndOversizedResponse(t *testing.T) {
	serverErrorClient := &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		return jsonResponse(http.StatusInternalServerError, `{"message":"temporary"}`), nil
	})}
	if _, err := Check(context.Background(), serverErrorClient, "0.1.0"); err == nil {
		t.Fatal("服务器错误应返回错误")
	}

	largeBody := bytes.Repeat([]byte("x"), maxResponseSize+1)
	largeClient := &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		return jsonResponse(http.StatusOK, string(largeBody)), nil
	})}
	if _, err := Check(context.Background(), largeClient, "0.1.0"); err == nil {
		t.Fatal("超大响应应返回错误")
	}
}

func TestCheckHonorsContextTimeout(t *testing.T) {
	client := &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		<-request.Context().Done()
		return nil, request.Context().Err()
	})}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	_, err := Check(ctx, client, "0.1.0")
	if err == nil || !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("超时错误异常：%v", err)
	}
}
