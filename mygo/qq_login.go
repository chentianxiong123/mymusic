//go:build native

// qq_login.go —— QQ 音乐扫码登录（参考公开协议自行实现）。
// 流程：ptlogin 登录页预热 → ptqrshow 取二维码图片+qrsig → ptqrlogin 轮询 →
// 成功后跟随跳转链拿 p_skey → OAuth authorize 换 code → musicu.fcg 登录拿 Cookie。
package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"
)

const (
	qqAppID     = "716027609"
	qqDaid      = "383"
	qqThirdAid  = "100497308"
	qqOAuthJump = "https://graph.qq.com/oauth2.0/login_jump"
	qqQRPage    = "https://xui.ptlogin2.qq.com/cgi-bin/xlogin"
	qqQRShow    = "https://xui.ptlogin2.qq.com/ssl/ptqrshow"
	qqQRCheck   = "https://xui.ptlogin2.qq.com/ssl/ptqrlogin"
	qqAuthorize = "https://graph.qq.com/oauth2.0/authorize"
	qqMusicAPI  = "https://u.y.qq.com/cgi-bin/musicu.fcg"
	qqUA        = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 Chrome/120.0 Safari/537.36"
)

// qqAuth 会话态（cookiejar 跨请求保持）。
type qqAuth struct {
	client *http.Client
	jar    http.CookieJar
	qrsig  string
	loginSig string
}

func qqHash33(s string) int {
	h := 0
	for _, c := range s {
		h += (h << 5) + int(c)
	}
	return h & 0x7fffffff
}

func newQQClient() *qqAuth {
	jar, _ := cookiejar.New(nil)
	return &qqAuth{
		jar: jar,
		client: &http.Client{Jar: jar, Timeout: 15 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		}},
	}
}

func (q *qqAuth) get(u, referer string) (*http.Response, error) {
	req, _ := http.NewRequest(http.MethodGet, u, nil)
	req.Header.Set("User-Agent", qqUA)
	if referer != "" {
		req.Header.Set("Referer", referer)
	}
	return q.client.Do(req)
}

func (q *qqAuth) jarCookie(rawURL, name string) string {
	u, err := url.Parse(rawURL)
	if err != nil {
		return ""
	}
	for _, c := range q.jar.Cookies(u) {
		if c.Name == name {
			return c.Value
		}
	}
	return ""
}

// createQQQR 生成二维码：返回二维码图片字节（直接显示，不用 URL 再编码）。
func createQQQR() (*qrSession, []byte, error) {
	q := newQQClient()
	// 预热登录页（拿 pt_login_sig）
	lp := url.Values{
		"appid": {qqAppID}, "daid": {qqDaid}, "style": {"33"},
		"login_text": {"登录"}, "hide_title_bar": {"1"}, "hide_border": {"1"},
		"target": {"self"}, "s_url": {qqOAuthJump}, "pt_3rd_aid": {qqThirdAid},
	}
	if resp, err := q.get(qqQRPage+"?"+lp.Encode(), ""); err == nil {
		resp.Body.Close()
	}
	q.loginSig = q.jarCookie("https://xui.ptlogin2.qq.com/", "pt_login_sig")

	// ptqrshow 拿二维码图片
	p := url.Values{
		"appid": {qqAppID}, "e": {"2"}, "l": {"M"}, "s": {"3"}, "d": {"72"},
		"v": {"4"}, "t": {fmt.Sprintf("%.6f", float64(time.Now().UnixNano())/1e9)},
		"daid": {qqDaid}, "pt_3rd_aid": {qqThirdAid}, "u1": {qqOAuthJump},
	}
	resp, err := q.get(qqQRShow+"?"+p.Encode(), "https://xui.ptlogin2.qq.com/")
	if err != nil {
		return nil, nil, err
	}
	defer resp.Body.Close()
	img, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, nil, err
	}
	q.qrsig = q.jarCookie("https://xui.ptlogin2.qq.com/", "qrsig")
	for _, c := range resp.Cookies() {
		if c.Name == "qrsig" {
			q.qrsig = c.Value
		}
	}
	if q.qrsig == "" {
		return nil, nil, fmt.Errorf("二维码缺少 qrsig")
	}
	// 把会话态塞进 session 字段（check 时重建 client）
	state := url.Values{"qrsig": {q.qrsig}, "login_sig": {q.loginSig}}
	return &qrSession{
		Source: "qq", Key: state.Encode(), URL: "", Status: qrWaiting,
	}, img, nil
}

var qqCheckRe = regexp.MustCompile(`'([^']*)'`)

// checkQQQR 轮询扫码状态；成功时完成 OAuth 换 Cookie。
func checkQQQR(s *qrSession) error {
	vals, err := url.ParseQuery(s.Key)
	if err != nil {
		return err
	}
	qrsig := vals.Get("qrsig")
	if qrsig == "" {
		return fmt.Errorf("qrsig为空")
	}
	q := newQQClient()
	// 重建 cookiejar：ptqrlogin 需要 qrsig/pt_login_sig
	ub, _ := url.Parse("https://xui.ptlogin2.qq.com/")
	cookies := []*http.Cookie{
		{Name: "qrsig", Value: qrsig, Domain: ub.Host, Path: "/"},
	}
	if ls := vals.Get("login_sig"); ls != "" {
		cookies = append(cookies, &http.Cookie{Name: "pt_login_sig", Value: ls, Domain: ub.Host, Path: "/"})
	}
	q.jar.SetCookies(ub, cookies)

	p := url.Values{
		"u1": {qqOAuthJump}, "ptqrtoken": {strconv.Itoa(qqHash33(qrsig))},
		"ptredirect": {"0"}, "h": {"1"}, "t": {"1"}, "g": {"1"},
		"from_ui": {"1"}, "ptlang": {"2052"},
		"action": {fmt.Sprintf("0-0-%d", time.Now().UnixMilli())},
		"js_ver": {"26071711"}, "js_type": {"1"},
		"login_sig": {vals.Get("login_sig")}, "pt_uistyle": {"40"},
		"aid": {qqAppID}, "daid": {qqDaid}, "pt_3rd_aid": {qqThirdAid},
		"pt_js_version": {"c1987b96"},
	}
	resp, err := q.get(qqQRCheck+"?"+p.Encode(), "https://xui.ptlogin2.qq.com/")
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	m := qqCheckRe.FindAllStringSubmatch(string(body), -1)
	if len(m) < 5 {
		s.Status = qrFailed
		return nil
	}
	code, msg, redirect := m[0][1], m[4][1], m[2][1]
	switch code {
	case "0":
		s.Status = qrSuccess
	case "65":
		s.Status = qrExpired
	case "66":
		s.Status = qrWaiting
	case "67":
		s.Status = qrScanned
	default:
		s.Status = qrFailed
		s.Cookie = msg // 错误信息带出去（UI 可显示）
		return nil
	}
	if s.Status != qrSuccess {
		return nil
	}
	// 成功后走 OAuth 换 QQ音乐 Cookie
	cookie, err := qqFinishLogin(q, redirect)
	if err != nil {
		s.Status = qrFailed
		s.Cookie = err.Error()
		return nil
	}
	s.Cookie = cookie
	return nil
}

// qqFinishLogin 跟随跳转拿 p_skey → OAuth authorize → musicu.fcg 登录。
func qqFinishLogin(q *qqAuth, redirect string) (string, error) {
	cur := redirect
	for i := 0; i < 10 && cur != ""; i++ {
		resp, err := q.get(cur, "https://xui.ptlogin2.qq.com/")
		if err != nil {
			return "", err
		}
		io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
		resp.Body.Close()
		loc := resp.Header.Get("Location")
		if loc == "" {
			break
		}
		if strings.HasPrefix(loc, "/") {
			u, _ := url.Parse(cur)
			loc = u.Scheme + "://" + u.Host + loc
		}
		cur = loc
	}
	pskey := q.jarCookie("https://graph.qq.com/", "p_skey")
	if pskey == "" {
		pskey = q.jarCookie("https://y.qq.com/", "p_skey")
	}
	if pskey == "" {
		return "", fmt.Errorf("缺少 p_skey")
	}
	// OAuth authorize（用 hash 算法同 gtk）
	h := 5381
	for _, c := range pskey {
		h += (h << 5) + int(c)
	}
	gtk := strconv.Itoa(h & 0x7fffffff)
	redirectURI := "https://y.qq.com/portal/wx_redirect.html?login_type=1&surl=" + url.QueryEscape("https://y.qq.com/")
	f := url.Values{
		"response_type": {"code"}, "client_id": {qqThirdAid},
		"redirect_uri": {redirectURI}, "scope": {"get_user_info"},
		"state": {"state"}, "from_ptlogin": {"1"}, "src": {"1"},
		"update_auth": {"1"}, "openapi": {"1010_1030"},
		"g_tk": {gtk}, "auth_time": {strconv.FormatInt(time.Now().UnixMilli(), 10)},
	}
	resp, err := q.get(qqAuthorize+"?"+f.Encode(), "https://graph.qq.com/")
	if err != nil {
		return "", err
	}
	loc := resp.Header.Get("Location")
	resp.Body.Close()
	code := ""
	if loc != "" {
		u, _ := url.Parse(loc)
		code = u.Query().Get("code")
	}
	if code == "" {
		return "", fmt.Errorf("OAuth 未返回 code")
	}
	// musicu.fcg 登录换 Cookie
	payload, _ := json.Marshal(map[string]any{
		"comm": map[string]any{"g_tk": 5381, "platform": "yqq", "ct": 24, "cv": 0},
		"req": map[string]any{
			"module": "QQConnectLogin.LoginServer", "method": "QQLogin",
			"param": map[string]string{"code": code},
		},
	})
	req, _ := http.NewRequest(http.MethodPost, qqMusicAPI, strings.NewReader(string(payload)))
	req.Header.Set("User-Agent", qqUA)
	req.Header.Set("Referer", "https://y.qq.com/")
	req.Header.Set("Content-Type", "application/json")
	resp2, err := q.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp2.Body.Close()
	// 从 cookiejar 收集 y.qq.com 域 Cookie
	parts := []string{}
	for _, c := range q.jar.Cookies(&url.URL{Scheme: "https", Host: "y.qq.com"}) {
		parts = append(parts, c.Name+"="+c.Value)
	}
	if len(parts) == 0 {
		return "", fmt.Errorf("未获得 QQ音乐 Cookie")
	}
	return strings.Join(parts, "; "), nil
}

// qqQRImage 返回二维码 ImageSource（ptqrshow 直接给 PNG，不需要 qrcode 库）。
func qqQRImage(png []byte) ui.ImageSource {
	img, err := ui.DecodeImage(png)
	if err != nil {
		return nil
	}
	return img
}
