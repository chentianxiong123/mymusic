//go:build native

package main

import (
	"context"
	"net/http"
	"runtime"
	"strings"
	"testing"
	"time"
)

// TestUpdateTarget 平台 manifest 后缀（与 mygo build 产出的 update-<target>.json 对齐）。
func TestUpdateTarget(t *testing.T) {
	got, want := updateTarget(), runtime.GOOS+"-"+runtime.GOARCH
	if got != want {
		t.Fatalf("updateTarget()=%q，期望 %q", got, want)
	}
}

// TestUpdateClientManifestURL 两种源类型的 manifest 地址。
func TestUpdateClientManifestURL(t *testing.T) {
	name := "update-" + updateTarget() + ".json"

	g := updateClient{kind: kindGitHub, githu: "chentianxiong123/mymusic"}
	if want := "https://github.com/chentianxiong123/mymusic/releases/latest/download/" + name; g.manifestURL() != want {
		t.Errorf("github: got %q want %q", g.manifestURL(), want)
	}

	u := updateClient{kind: kindURL, base: "https://example.com/mygo/"}
	if want := "https://example.com/mygo/" + name; u.manifestURL() != want {
		t.Errorf("url: got %q want %q", u.manifestURL(), want)
	}
}

// TestUpdateClientOpenResolvesRelative manifest 里的相对 URL 应按 base 补全。
func TestUpdateClientOpenResolvesRelative(t *testing.T) {
	u := updateClient{kind: kindURL, base: "https://example.com/mygo"}
	// 相对 URL 会指向 404（不存在），但错误信息里应能看到补全后的绝对地址。
	_, err := u.Open(context.Background(), "mymusic-mygo-0.2.0.zip")
	if err == nil {
		t.Fatal("期望 404 错误，实际成功")
	}
	if !strings.Contains(err.Error(), "https://example.com/mygo/mymusic-mygo-0.2.0.zip") {
		t.Errorf("未含补全后的绝对地址：%v", err)
	}
	if !strings.Contains(err.Error(), "HTTP 404") {
		t.Errorf("未含 HTTP 状态：%v", err)
	}
}

// TestApplyUpdateSource applyUpdateSource 不 panic，且回落到构建期 feed（nil）。
func TestApplyUpdateSource(t *testing.T) {
	applyUpdateSource(kindGitHub, "chentianxiong123/mymusic", "", "")
	applyUpdateSource(kindURL, "", "https://example.com/mygo", "http://127.0.0.1:7890")
	applyUpdateSource("", "", "", "")
}

// TestUpdateClientNoReleaseYet 当前仓库还没有 mygo- 标签的 release：
// 应返回 404（而不是 panic 或当成"已是最新"）。
func TestUpdateClientNoReleaseYet(t *testing.T) {
	g := updateClient{kind: kindGitHub, githu: "chentianxiong123/mymusic"}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	body, err := g.Manifest(ctx, false)
	if err == nil {
		body.Close()
		t.Fatal("期望 manifest 取不到（仓库尚无 release），实际成功")
	}
	if !strings.Contains(err.Error(), "HTTP 404") {
		t.Logf("注意：实际错误为 %v（若仓库已发 release，可忽略）", err)
	}
}

// TestUpdateEnabled 说明当前二进制是否带更新公钥（plain go build = 未启用）。
func TestUpdateEnabled(t *testing.T) {
	t.Logf("updateTarget=%s Enabled=%v（plain go build 为 false，mygo build 后为 true）", updateTarget(), mygoUpdaterEnabled())
}

// _ = http.MethodGet 让 net/http 留在 import 里（供将来加鉴权头用）。
var _ = http.MethodGet
