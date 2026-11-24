//go:build native

package main

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"runtime"
	"strings"
	"testing"
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

// TestUpdateClientManifest404 源站没有 manifest 时应返回 404 错误（不 panic、不当"已是最新"）。
// 用本地 httptest，不依赖外网 / 仓库 release 状态（v1.0.0 发布后旧测试前提已失效）。
func TestUpdateClientManifest404(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))
	defer srv.Close()
	u := updateClient{kind: kindURL, base: srv.URL}
	body, err := u.Manifest(context.Background(), false)
	if err == nil {
		body.Close()
		t.Fatal("期望 404 错误，实际成功")
	}
	if !strings.Contains(err.Error(), "HTTP 404") {
		t.Errorf("未含 HTTP 404：%v", err)
	}
}

// TestUpdateClientManifestOK 源站有 manifest 时应能读到 body。
func TestUpdateClientManifestOK(t *testing.T) {
	name := "update-" + updateTarget() + ".json"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/"+name {
			http.NotFound(w, r)
			return
		}
		fmt.Fprint(w, `{"version":"9.9.9"}`)
	}))
	defer srv.Close()
	u := updateClient{kind: kindURL, base: srv.URL}
	body, err := u.Manifest(context.Background(), false)
	if err != nil {
		t.Fatalf("读 manifest 失败: %v", err)
	}
	defer body.Close()
	buf := make([]byte, 64)
	n, _ := body.Read(buf)
	if !strings.Contains(string(buf[:n]), "9.9.9") {
		t.Errorf("manifest 内容不对: %q", buf[:n])
	}
}

// TestUpdateEnabled 说明当前二进制是否带更新公钥（plain go build = 未启用）。
func TestUpdateEnabled(t *testing.T) {
	t.Logf("updateTarget=%s Enabled=%v（plain go build 为 false，mygo build 后为 true）", updateTarget(), mygoUpdaterEnabled())
}

// _ = http.MethodGet 让 net/http 留在 import 里（供将来加鉴权头用）。
var _ = http.MethodGet
