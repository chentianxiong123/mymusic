//go:build native

// login_verify.go —— 登录态有效性校验（Cookie 非空 ≠ 有效）。
// 实测端点（2026-11-25，无 Cookie/假 Cookie 均返回明确未登录）：
//   B站   https://api.bilibili.com/x/web-interface/nav → code=0 && isLogin + uname
//   网易云 https://music.163.com/api/w/nuser/account/get → account != null + nickname
// 其余平台（酷狗/QQ/咪咕/酷我）没有可靠的"用 WEB Cookie 校验"端点：
//   咪咕 token-validate 用的是 passport token，登录成功后已换成 WEB Cookie，无法复用。
//   → 诚实降级为「已登录（未校验）」，不猜、不误报过期。
package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"

	"mymusic-mygo/plugin"
)

type loginCheck struct {
	Checked bool   // 是否已校验（false=平台无可靠端点，仅凭 Cookie 非空）
	Valid   bool   // Cookie 是否有效
	Name    string // 账号名（校验成功时）
}

var loginChecks = struct {
	sync.Mutex
	m map[string]loginCheck
}{m: map[string]loginCheck{}}

func setLoginCheck(src string, c loginCheck) {
	loginChecks.Lock()
	loginChecks.m[src] = c
	loginChecks.Unlock()
}

func getLoginCheck(src string) loginCheck {
	loginChecks.Lock()
	defer loginChecks.Unlock()
	return loginChecks.m[src]
}

var verifyHTTP = &http.Client{Timeout: 8 * time.Second}

func verifyRequest(u, cookie, referer string) ([]byte, error) {
	req, _ := http.NewRequest(http.MethodGet, u, nil)
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 Chrome/120.0 Safari/537.36")
	req.Header.Set("Referer", referer)
	if cookie != "" {
		req.Header.Set("Cookie", cookie)
	}
	resp, err := verifyHTTP.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	return io.ReadAll(resp.Body)
}

// verifyLoginCookie 校验 Cookie 是否有效（同步，调用方放 goroutine）。
func verifyLoginCookie(src, cookie string) loginCheck {
	if cookie == "" {
		return loginCheck{Checked: true, Valid: false}
	}
	switch src {
	case "bilibili":
		body, err := verifyRequest("https://api.bilibili.com/x/web-interface/nav", cookie, "https://www.bilibili.com/")
		if err != nil {
			return loginCheck{} // 网络失败 → 不误判过期
		}
		var r struct {
			Code int `json:"code"`
			Data struct {
				IsLogin bool   `json:"isLogin"`
				Uname   string `json:"uname"`
			} `json:"data"`
		}
		if json.Unmarshal(body, &r) != nil {
			return loginCheck{}
		}
		return loginCheck{Checked: true, Valid: r.Code == 0 && r.Data.IsLogin, Name: r.Data.Uname}

	case "netease":
		body, err := verifyRequest("https://music.163.com/api/w/nuser/account/get", cookie, "https://music.163.com/")
		if err != nil {
			return loginCheck{}
		}
		var r struct {
			Code    int `json:"code"`
			Account *struct {
				ID int64 `json:"id"`
			} `json:"account"`
			Profile *struct {
				Nickname string `json:"nickname"`
			} `json:"profile"`
		}
		if json.Unmarshal(body, &r) != nil {
			return loginCheck{}
		}
		name := ""
		if r.Profile != nil {
			name = r.Profile.Nickname
		}
		return loginCheck{Checked: true, Valid: r.Code == 200 && r.Account != nil, Name: name}
	}
	// 其它平台：没有可靠的 Cookie 校验端点 → 不校验，不误报。
	return loginCheck{}
}

// enqueueLoginVerify 异步校验并把结果写缓存，完成后重绘。
func (a *app) enqueueLoginVerify(src string) {
	cookie := plugin.GetSetting("source", src, "cookie", "")
	if cookie == "" {
		setLoginCheck(src, loginCheck{Checked: true, Valid: false})
		return
	}
	go func() {
		res := verifyLoginCookie(src, cookie)
		setLoginCheck(src, res)
		a.win.Update(func() { a.win.Invalidate() })
	}()
}

// verifyAllLogins 启动/需要时批量校验所有已登录源（异步，失败不误报）。
func (a *app) verifyAllLogins() {
	for _, src := range []string{"bilibili", "netease", "kugou", "qqmusic", "migu", "kuwo"} {
		a.enqueueLoginVerify(src)
	}
}

// loginStateLabel 登录状态文案（UI 用；纯函数，好测）。
func loginStateLabel(cookie string, c loginCheck) string {
	if cookie == "" {
		return "未登录"
	}
	if !c.Checked {
		return "已登录"
	}
	if !c.Valid {
		return "已过期"
	}
	if c.Name != "" {
		return fmt.Sprintf("已登录 · %s", truncateRunes(c.Name, 12))
	}
	return "已登录"
}

func truncateRunes(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "…"
}

