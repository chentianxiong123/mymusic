//go:build native

// qr_login.go —— 音源插件扫码登录（参考协议自行实现：B站 / 网易云）。
// 不依赖第三方聚合库：两个平台各两个 HTTP 端点，状态机统一。
// 成功后把 Cookie 写入对应音源插件的 settings（source/<name>/cookie）。
package main

import (
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	qrcode "github.com/skip2/go-qrcode"

	"github.com/egoist/mygo/ui"

	"mymusic-mygo/plugin"
)

// qrStatus 登录状态（统一两个平台的状态码）。
type qrStatus int

const (
	qrWaiting qrStatus = iota
	qrScanned
	qrSuccess
	qrExpired
	qrFailed
)

func (s qrStatus) text() string {
	switch s {
	case qrWaiting:
		return "等待扫码"
	case qrScanned:
		return "已扫码，请在手机上确认"
	case qrSuccess:
		return "登录成功"
	case qrExpired:
		return "二维码已过期"
	}
	return "登录失败"
}

// qrSession 当前扫码会话。
type qrSession struct {
	Source string
	Key    string
	URL    string
	Status qrStatus
	Cookie string
	Image  []byte // 服务器直接返回的二维码图片（QQ；其余平台为空用 URL 本地生成）
}

var qrHTTP = &http.Client{Timeout: 10 * time.Second}

const biliUA = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0 Safari/537.36"

// createQR 生成二维码会话（src = bilibili / netease）。
func createQR(src string) (*qrSession, error) {
	switch src {
	case "bilibili":
		return createBiliQR()
	case "netease":
		return createNeteaseQR()
	case "kugou":
		return createKugouQR()
	case "qqmusic":
		s, png, err := createQQQR()
		if s != nil {
			s.Image = png
		}
		return s, err
	case "migu":
		return createMiguQR()
	}
	return nil, fmt.Errorf("该音源不支持扫码登录")
}

// checkQR 轮询扫码状态。
func checkQR(s *qrSession) error {
	if s == nil {
		return fmt.Errorf("无登录会话")
	}
	switch s.Source {
	case "bilibili":
		return checkBiliQR(s)
	case "netease":
		return checkNeteaseQR(s)
	case "kugou":
		return checkKugouQR(s)
	case "qqmusic":
		return checkQQQR(s)
	case "migu":
		return checkMiguQR(s)
	}
	return fmt.Errorf("该音源不支持扫码登录")
}

// ---- B站：x/passport-login/web/qrcode/{generate,poll} ----

func createBiliQR() (*qrSession, error) {
	req, _ := http.NewRequest(http.MethodGet, "https://passport.bilibili.com/x/passport-login/web/qrcode/generate", nil)
	req.Header.Set("User-Agent", biliUA)
	req.Header.Set("Referer", "https://www.bilibili.com/")
	resp, err := qrHTTP.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	var out struct {
		Code int    `json:"code"`
		Data struct {
			URL string `json:"url"`
			Key string `json:"qrcode_key"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, err
	}
	if out.Code != 0 || out.Data.Key == "" || out.Data.URL == "" {
		return nil, fmt.Errorf("生成二维码失败（code=%d）", out.Code)
	}
	return &qrSession{Source: "bilibili", Key: out.Data.Key, URL: out.Data.URL, Status: qrWaiting}, nil
}

func checkBiliQR(s *qrSession) error {
	u := "https://passport.bilibili.com/x/passport-login/web/qrcode/poll?qrcode_key=" + url.QueryEscape(s.Key)
	req, _ := http.NewRequest(http.MethodGet, u, nil)
	req.Header.Set("User-Agent", biliUA)
	req.Header.Set("Referer", "https://www.bilibili.com/")
	resp, err := qrHTTP.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	var out struct {
		Data struct {
			Code int `json:"code"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return err
	}
	switch out.Data.Code {
	case 0:
		s.Status = qrSuccess
		// Set-Cookie 提取（SESSDATA / bili_jct / DedeUserID 等）
		parts := make([]string, 0)
		for _, c := range resp.Cookies() {
			if c.Name != "" {
				parts = append(parts, c.Name+"="+c.Value)
			}
		}
		sort.Strings(parts)
		s.Cookie = strings.Join(parts, "; ")
	case 86038:
		s.Status = qrExpired
	case 86090:
		s.Status = qrScanned
	case 86101:
		s.Status = qrWaiting
	default:
		s.Status = qrFailed
	}
	return nil
}

// ---- 网易云：/api/login/qrcode/unikey + /api/login/qrcode/client/login ----

const neteaseUA = "Mozilla/5.0 (Windows NT 10.0; WOW64) AppleWebKit/537.36 (KHTML, like Gecko) Safari/537.36 Chrome/91.0.4472.164 NeteaseMusicDesktop/3.0.18.203152"

func neteasePost(u string, form url.Values) ([]byte, error) {
	req, err := http.NewRequest(http.MethodPost, u, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", neteaseUA)
	req.Header.Set("Referer", "https://music.163.com/")
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := qrHTTP.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	return io.ReadAll(resp.Body)
}

func createNeteaseQR() (*qrSession, error) {
	body, err := neteasePost("https://interface.music.163.com/api/login/qrcode/unikey", url.Values{"type": {"3"}})
	if err != nil {
		return nil, err
	}
	var out struct {
		Code   int    `json:"code"`
		UniKey string `json:"unikey"`
	}
	if err := json.Unmarshal(body, &out); err != nil {
		return nil, err
	}
	if out.Code != 200 || out.UniKey == "" {
		return nil, fmt.Errorf("生成二维码失败（code=%d）", out.Code)
	}
	return &qrSession{
		Source: "netease",
		Key:    out.UniKey,
		URL:    "https://music.163.com/login?codekey=" + url.QueryEscape(out.UniKey),
		Status: qrWaiting,
	}, nil
}

func checkNeteaseQR(s *qrSession) error {
	body, err := neteasePost("https://interface.music.163.com/api/login/qrcode/client/login", url.Values{
		"key":  {s.Key},
		"type": {"3"},
	})
	if err != nil {
		return err
	}
	var out struct {
		Code   int    `json:"code"`
		Cookie string `json:"cookie"`
	}
	if err := json.Unmarshal(body, &out); err != nil {
		return err
	}
	switch out.Code {
	case 803:
		s.Status = qrSuccess
		s.Cookie = out.Cookie
	case 802:
		s.Status = qrScanned
	case 801:
		s.Status = qrWaiting
	case 800:
		s.Status = qrExpired
	default:
		s.Status = qrFailed
	}
	return nil
}

// qrPNG 生成二维码 PNG（skip2/go-qrcode）。
func qrPNG(text string, size int) ([]byte, error) {
	return qrcode.Encode(text, qrcode.Medium, size)
}

// applySourceCookie 统一：落库 + 热更新当前客户端（扫码/粘贴共用）。
func (a *app) applySourceCookie(src, cookie string) {
	_ = plugin.SetSetting("source", src, "cookie", cookie)
	a.hotUpdateCookie(src, cookie)
	a.enqueueLoginVerify(src)
}

// hotUpdateCookie 热更新已构造的客户端（不重启可见；JS 插件每次 getURL 都读 config，无需动）。
func (a *app) hotUpdateCookie(src, cookie string) {
	switch src {
	case "bilibili":
		a.bili.Cookie = cookie
	case "netease":
		// Go 内置网易云源：用 setter 更新，避免重启才生效
		if a.neteaseSrc != nil {
			a.neteaseSrc.SetCookie(cookie)
		}
	}
	// 其它 JS 插件：每次 getURL/search 都从 ctx.config 读，无需热更新。
}

// applyQRCookie 登录成功后写入音源插件 Cookie 设置 + 热更新客户端。
func (a *app) applyQRCookie(src, cookie string) {
	a.applySourceCookie(src, cookie)
}

// logoutSource 退出登录：清持久 cookie + 热更新客户端为空 + 停未完成的扫码。
func (a *app) logoutSource(src string) {
	a.applySourceCookie(src, "")
	a.loginCookieBuf = ""
	if a.qrLoginSession != nil && a.qrLoginSession.Source == src {
		a.stopQRLogin()
	}
	a.showLoginDialog = false
	a.win.Invalidate()
	_ = src
}

// ---- UI ----

// qrLoginSection 音源详情页的登录区块（仅支持扫码的平台显示按钮）。
func (a *app) qrLoginSection(c *ui.Context, src string) {
	// 只给有登录概念的源显示（免登录源不显示）
	if !sourceNeedsLogin(src) {
		return
	}
	cookie := plugin.GetSetting("source", src, "cookie", "")
	chk := getLoginCheck(src)
	state := loginStateLabel(cookie, chk)
	stateColor := clrMuted
	if chk.Valid || (cookie != "" && !chk.Checked) {
		stateColor = clrPink
	} else if chk.Checked && !chk.Valid {
		stateColor = clrRed // 过期：红
	}
	ui.Row(c).AlignSelf(ui.Stretch).AlignItems(ui.Center).Gap(10 * uiScale).PaddingX(4 * uiScale).Children(func() {
		ui.Text(c, "登录").FontSize(14 * uiScale).Bold().TextColor(clrText)
		ui.Spacer(c).Grow(1)
		ui.Text(c, state).FontSize(12 * uiScale).TextColor(stateColor)
		if chip(c, "登录方式 ›").Clicked() {
			a.loginDialogSrc = src
			a.loginCookieBuf = plugin.GetSetting("source", src, "cookie", "")
			a.showLoginDialog = true
			a.win.Invalidate()
		}
	})
}

// sourceNeedsLogin 哪些源有登录概念（其余免登录不显示入口）。
func sourceNeedsLogin(src string) bool {
	switch src {
	case "bilibili", "netease", "kugou", "qqmusic", "migu", "kuwo":
		return true
	}
	return false
}

// sourceHasQR 哪些源有扫码协议。
func sourceHasQR(src string) bool {
	switch src {
	case "bilibili", "netease", "kugou", "qqmusic", "migu":
		return true
	}
	return false
}

// loginMethodDialog 登录方式弹窗：扫码（支持的）+ Cookie/Token 粘贴（全部）。
func (a *app) loginMethodDialog(c *ui.Context) {
	src := a.loginDialogSrc
	if !a.showLoginDialog || src == "" {
		return
	}
	lx, ly, pw, ph := a.dialogRect(360, 300)
	ui.Overlay(c, func() {
		bg := ui.Box(c).Absolute().Left(0).Top(0).Right(0).Bottom(0)
		bg.Draw(func(p *ui.Painter, r ui.Rect) { p.Fill(r, ui.RGBA(0, 0, 0, 0.55), 0) })
		if bg.Clicked() {
			a.showLoginDialog = false
			a.win.Invalidate()
		}
		panel := ui.Box(c).Absolute().Left(lx).Top(ly).Width(pw).Height(ph).Radius(18 * uiScale).Background(clrCard)
		panel.Draw(func(p *ui.Painter, r ui.Rect) {
			p.Shadow(r, 24, 0, 14, 48, 0, ui.RGBA(0, 0, 0, 0.6))
			p.FillRadii(r, [4]float32{18, 18, 18, 18}, clrCard)
		})
		panel.Children(func() {
			ui.Column(c).Fill().Padding(18 * uiScale).Gap(12 * uiScale).Children(func() {
				ui.Row(c).AlignSelf(ui.Stretch).AlignItems(ui.Center).Gap(9 * uiScale).Children(func() {
					ui.Box(c).Size(4*uiScale, 16*uiScale).Radius(2*uiScale).Background(clrPink)
					ui.Text(c, sourceShortName(src)+" 登录").FontSize(16 * uiScale).Bold().TextColor(clrText)
					ui.Spacer(c).Grow(1)
					if chip(c, "×").Clicked() {
						a.showLoginDialog = false
						a.win.Invalidate()
					}
				})
				if sourceHasQR(src) {
					if chip(c, "扫码登录").Clicked() {
						a.showLoginDialog = false
						a.startQRLogin(src)
					}
				}
				ui.Text(c, "或粘贴登录 Cookie / Token").FontSize(12 * uiScale).TextColor(clrMuted)
				ui.TextInput(c, &a.loginCookieBuf).Grow(1).Height(38 * uiScale).FontSize(12 * uiScale).TextColor(clrText).Placeholder("Cookie / Token")
				ui.Spacer(c).Grow(1)
				ui.Row(c).AlignSelf(ui.Stretch).Gap(10 * uiScale).Children(func() {
					if chip(c, "取消").Clicked() {
						a.showLoginDialog = false
						a.win.Invalidate()
					}
					logged := plugin.GetSetting("source", src, "cookie", "") != ""
					if logged && chip(c, "退出登录").Clicked() {
						a.logoutSource(src)
					}
					if gradButton(c, "保存").Clicked() {
						a.applySourceCookie(src, a.loginCookieBuf)
						a.showLoginDialog = false
						a.win.Invalidate()
					}
				})
			})
		})
	})
}

// startQRLogin 生成二维码 + 启动轮询（关闭弹窗即停）。
func (a *app) startQRLogin(src string) {
	s, err := createQR(src)
	if err != nil {
		return
	}
	a.qrLoginSession = s
	a.qrLoginImg = nil
	if len(s.Image) > 0 {
		// QQ：二维码图片由 ptqrshow 直接返回
		a.qrLoginImg = qqQRImage(s.Image)
	} else if s.URL != "" {
		png, err := qrPNG(s.URL, 256)
		if err == nil {
			if img, derr := ui.DecodeImage(png); derr == nil {
				a.qrLoginImg = img
			}
		}
	}
	if a.qrLoginStop != nil {
		close(a.qrLoginStop)
	}
	stop := make(chan struct{})
	a.qrLoginStop = stop
	go func() {
		t := time.NewTicker(2 * time.Second)
		defer t.Stop()
		for {
			select {
			case <-stop:
				return
			case <-t.C:
				cur := a.qrLoginSession
				if cur == nil || cur.Status == qrSuccess || cur.Status == qrExpired {
					return
				}
				if err := checkQR(cur); err != nil {
					continue
				}
				a.win.Update(func() {
					if a.qrLoginSession == nil || a.qrLoginSession != cur {
						return
					}
					if cur.Status == qrSuccess {
						a.applyQRCookie(cur.Source, cur.Cookie)
					}
					a.win.Invalidate()
				})
				if cur.Status == qrSuccess || cur.Status == qrExpired {
					return
				}
			}
		}
	}()
	a.win.Invalidate()
}

// qrLoginDialog 扫码登录弹窗（响应式 dialogRect，Overlay 置顶）。
func (a *app) qrLoginDialog(c *ui.Context) {
	s := a.qrLoginSession
	if s == nil {
		return
	}
	lx, ly, pw, ph := a.dialogRect(320, 400)
	ui.Overlay(c, func() {
		bg := ui.Box(c).Absolute().Left(0).Top(0).Right(0).Bottom(0)
		bg.Draw(func(p *ui.Painter, r ui.Rect) { p.Fill(r, ui.RGBA(0, 0, 0, 0.55), 0) })
		if bg.Clicked() {
			a.stopQRLogin()
		}
		panel := ui.Box(c).Absolute().Left(lx).Top(ly).Width(pw).Height(ph).Radius(18 * uiScale).Background(clrCard)
		panel.Draw(func(p *ui.Painter, r ui.Rect) {
			p.Shadow(r, 24, 0, 14, 48, 0, ui.RGBA(0, 0, 0, 0.6))
			p.FillRadii(r, [4]float32{18, 18, 18, 18}, clrCard)
		})
		panel.Children(func() {
			ui.Column(c).Fill().Padding(18 * uiScale).Gap(12 * uiScale).AlignItems(ui.Center).Children(func() {
				ui.Row(c).AlignSelf(ui.Stretch).AlignItems(ui.Center).Gap(9 * uiScale).Children(func() {
					ui.Box(c).Size(4*uiScale, 16*uiScale).Radius(2*uiScale).Background(clrPink)
					ui.Text(c, "扫码登录").FontSize(16 * uiScale).Bold().TextColor(clrText)
					ui.Spacer(c).Grow(1)
					if chip(c, "×").Clicked() {
						a.stopQRLogin()
					}
				})
				if a.qrLoginImg != nil {
					ui.Image(c, a.qrLoginImg).Size(200*uiScale, 200*uiScale)
				} else {
					ui.Text(c, "二维码生成中…").FontSize(12 * uiScale).TextColor(clrMuted)
				}
				ui.Text(c, s.Status.text()).FontSize(13 * uiScale).TextColor(clrPink)
				if s.Status == qrSuccess {
					if chip(c, "完成").Clicked() {
						a.stopQRLogin()
					}
				} else if s.Status == qrExpired {
					if chip(c, "刷新").Clicked() {
						a.startQRLogin(s.Source)
					}
				}
			})
		})
	})
}

// stopQRLogin 关闭弹窗 + 停止轮询。
func (a *app) stopQRLogin() {
	if a.qrLoginStop != nil {
		close(a.qrLoginStop)
		a.qrLoginStop = nil
	}
	a.qrLoginSession = nil
	a.qrLoginImg = nil
	a.win.Invalidate()
}

// ---- 酷狗：/v2/qrcode + /v2/get_userinfo_qrcode（签名 MD5(key+params+key)） ----

const (
	kgAppID    = "3116"
	kgVer      = "11440"
	kgSignKey  = "NVPh5oo715z5DIWAeQlhMDsWXXQV4hwt"
	kgUA       = "Android15-1070-11083-46-0-DiscoveryDRADProtocol-wifi"
)

// kgSign 酷狗参数签名：排序 key=value 拼接，前后包 sign key 取 MD5。
func kgSign(params map[string]string) string {
	pairs := make([]string, 0, len(params))
	for k, v := range params {
		pairs = append(pairs, k+"="+v)
	}
	sort.Strings(pairs)
	sum := md5sum(kgSignKey + strings.Join(pairs, "") + kgSignKey)
	return sum
}

func md5sum(s string) string {
	h := md5.Sum([]byte(s))
	return hex.EncodeToString(h[:])
}

func kgRandom(n int) string {
	const chars = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZ"
	b := make([]byte, n)
	for i := range b {
		b[i] = chars[time.Now().UnixNano()%int64(len(chars))]
		time.Sleep(time.Millisecond)
	}
	return string(b)
}

// kgRequest 酷狗带签名的 GET（返回 JSON bytes）。
func kgRequest(api string, params map[string]string) ([]byte, error) {
	ct := strconv.FormatInt(time.Now().Unix(), 10)
	p := map[string]string{
		"dfid":       "-",
		"mid":        "-",
		"uuid":       "-",
		"appid":      kgAppID,
		"clientver":  kgVer,
		"clienttime": ct,
	}
	for k, v := range params {
		p[k] = v
	}
	p["signature"] = kgSign(p)
	q := url.Values{}
	for k, v := range p {
		q.Set(k, v)
	}
	req, _ := http.NewRequest(http.MethodGet, api+"?"+q.Encode(), nil)
	req.Header.Set("User-Agent", kgUA)
	req.Header.Set("dfid", p["dfid"])
	req.Header.Set("clienttime", ct)
	req.Header.Set("mid", p["mid"])
	req.Header.Set("kg-rc", "1")
	cl := &http.Client{Timeout: 10 * time.Second}
	resp, err := cl.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	return io.ReadAll(resp.Body)
}

func createKugouQR() (*qrSession, error) {
	body, err := kgRequest("https://login-user.kugou.com/v2/qrcode", map[string]string{
		"type": "1", "plat": "4", "srcappid": "2919",
		"qrcode_txt": "https://h5.kugou.com/apps/loginQRCode/html/index.html?appid=" + kgAppID + "&",
	})
	if err != nil {
		return nil, err
	}
	var out struct {
		Status int `json:"status"`
		Data   struct{ QRCode string `json:"qrcode"` } `json:"data"`
	}
	if err := json.Unmarshal(body, &out); err != nil {
		return nil, err
	}
	if out.Data.QRCode == "" {
		return nil, fmt.Errorf("生成二维码失败（status=%d）", out.Status)
	}
	return &qrSession{
		Source: "kugou", Key: out.Data.QRCode,
		URL: "https://h5.kugou.com/apps/loginQRCode/html/index.html?qrcode=" + url.QueryEscape(out.Data.QRCode),
		Status: qrWaiting,
	}, nil
}

func checkKugouQR(s *qrSession) error {
	body, err := kgRequest("https://login-user.kugou.com/v2/get_userinfo_qrcode", map[string]string{
		"plat": "4", "appid": kgAppID, "srcappid": "2919", "qrcode": s.Key,
	})
	if err != nil {
		return err
	}
	var out struct {
		Data struct {
			Status int    `json:"status"`
			Token  string `json:"token"`
			UserID any    `json:"userid"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &out); err != nil {
		return err
	}
	switch out.Data.Status {
	case 4:
		s.Status = qrSuccess
		// userid 可能是 number 或 string
		uid := ""
		switch v := out.Data.UserID.(type) {
		case string:
			uid = v
		case float64:
			uid = strconv.FormatInt(int64(v), 10)
		}
		if out.Data.Token == "" || uid == "" || uid == "0" {
			s.Status = qrFailed
			return nil
		}
		s.Cookie = "token=" + out.Data.Token + "; userid=" + uid
	case 2, 3:
		s.Status = qrScanned
	case 0, 1:
		s.Status = qrWaiting
	case -1, 5, 6:
		s.Status = qrExpired
	default:
		s.Status = qrFailed
	}
	return nil
}
