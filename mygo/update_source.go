//go:build native

// update_source 运行期更新源（用户自填）：GitHub owner/name 或自建 URL。
//
// 关键设计：校验公钥由 mygo build 从 mygo.json 的 updates.publicKey 烙进二进制，
// 与"源"无关——mygo.Updater.Check 永远用那把公钥验签名，所以换源不掉安全性。
// 构建期的 feed（mygo.json 的 github/url）只用于打开更新开关；一旦 SetSource 生效，
// 实际去哪拿包完全由这里决定（见 mygo.Updater.Check）。
package main

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"runtime"
	"strings"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/netproxy"
)

// 更新源类型（设置里的分段选择）。
const (
	kindGitHub = "github"
	kindURL    = "url"
)

// updateTarget 本平台 manifest 后缀：mygo build 出 update-<target>.json
// （如 update-linux-amd64.json / update-windows-amd64.json）。
func updateTarget() string { return runtime.GOOS + "-" + runtime.GOARCH }

// updateClient 运行期自定义更新源，实现 mygo.UpdateSource。
type updateClient struct {
	kind  string // kindGitHub / kindURL
	githu string // github 模式：owner/name
	base  string // url 模式：源根，如 https://example.com/mygo
	proxy string // 自定义代理地址；空=跟随系统代理
}

// httpClient 下载用 client。默认跟随系统代理（netproxy）；填了自定义代理则覆盖。
// 不设 Client.Timeout：更新包可能很大，超时交由调用方的 ctx 控制。
func (s updateClient) httpClient() *http.Client {
	tr := netproxy.Transport().Clone()
	if u, err := url.Parse(s.proxy); err == nil && u.Host != "" {
		tr.Proxy = http.ProxyURL(u)
	}
	return &http.Client{Transport: tr}
}

// manifestURL 本平台 manifest 的绝对地址。
func (s updateClient) manifestURL() string {
	name := "update-" + updateTarget() + ".json"
	switch s.kind {
	case kindGitHub:
		// GitHub 把 /releases/latest/download/<asset> 重定向到真实 CDN 地址。
		return fmt.Sprintf("https://github.com/%s/releases/latest/download/%s",
			strings.TrimSpace(s.githu), name)
	default:
		return strings.TrimRight(s.base, "/") + "/" + name
	}
}

// Manifest 返回运行平台的 manifest，供 Updater.Check 校验签名+比对版本。
func (s updateClient) Manifest(ctx context.Context, _ bool) (io.ReadCloser, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, s.manifestURL(), nil)
	if err != nil {
		return nil, err
	}
	resp, err := s.httpClient().Do(req)
	if err != nil {
		return nil, fmt.Errorf("取更新清单 %s: %w", s.manifestURL(), err)
	}
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		return nil, fmt.Errorf("更新清单 HTTP %d：%s", resp.StatusCode, s.manifestURL())
	}
	return resp.Body, nil
}

// Open 按 manifest 里记的 URL 取更新包/增量。相对 URL 按 base 补全。
func (s updateClient) Open(ctx context.Context, u string) (io.ReadCloser, error) {
	if !strings.HasPrefix(u, "http://") && !strings.HasPrefix(u, "https://") {
		u = strings.TrimRight(s.base, "/") + "/" + strings.TrimLeft(u, "/")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	resp, err := s.httpClient().Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		return nil, fmt.Errorf("更新包 HTTP %d：%s", resp.StatusCode, u)
	}
	return resp.Body, nil
}

// applyUpdateSource 把用户配置接入 mygo.Updater（运行期生效，不改构建期）。
// kind 为空 = 不覆盖，回落到构建期 feed（mygo.json 的 github/url）。
func applyUpdateSource(kind, githu, base, proxy string) {
	if kind == "" {
		mygo.Updater.SetSource(nil)
		return
	}
	mygo.Updater.SetSource(updateClient{kind: kind, githu: githu, base: base, proxy: proxy})
}
