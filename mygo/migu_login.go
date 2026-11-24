//go:build native

// migu_login.go —— 咪咕音乐扫码登录（参考公开协议自行实现）。
// passport.migu.cn 返回 JPEG 二维码图片 + qrc_sessionid；轮询 qrcquery 拿 token，
// 再用 token-validate 换 WEB Cookie。
package main

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const (
	miguPassport = "https://passport.migu.cn"
	miguGateway  = "https://c.musicapp.migu.cn"
	miguQRSource = "204040"
	miguWebSource = "220029"
	miguUA       = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 Chrome/120.0 Safari/537.36"
)

type miguAuth struct {
	client *http.Client
	cookies map[string]string
}

func newMiguAuth() *miguAuth {
	return &miguAuth{client: &http.Client{Timeout: 10 * time.Second}, cookies: map[string]string{}}
}

func (m *miguAuth) cookieHeader() string {
	parts := make([]string, 0, len(m.cookies))
	for k, v := range m.cookies {
		parts = append(parts, k+"="+v)
	}
	return strings.Join(parts, "; ")
}

func (m *miguAuth) request(u, referer string) ([]byte, http.Header, error) {
	req, _ := http.NewRequest(http.MethodGet, u, nil)
	req.Header.Set("User-Agent", miguUA)
	req.Header.Set("X-Requested-With", "XMLHttpRequest")
	if referer != "" {
		req.Header.Set("Referer", referer)
	}
	if c := m.cookieHeader(); c != "" {
		req.Header.Set("Cookie", c)
	}
	resp, err := m.client.Do(req)
	if err != nil {
		return nil, nil, err
	}
	defer resp.Body.Close()
	for _, c := range resp.Cookies() {
		if c.Name == "mgnd_session_id" || c.Name == "mgnd_session_create" || c.Name == "mgnd_session_last_access" {
			m.cookies[c.Name] = c.Value
		}
	}
	body, err := io.ReadAll(resp.Body)
	return body, resp.Header, err
}

// createMiguQR 生成二维码（返回 JPEG 图片字节放 s.Image，直接显示）。
func createMiguQR() (*qrSession, error) {
	m := newMiguAuth()
	login := miguPassport + "/login?" + url.Values{
		"sourceid": {miguQRSource}, "apptype": {"2"}, "forceAuthn": {"true"},
		"isPassive": {"false"}, "display": {"wap"}, "relayState": {""}, "logintype": {"3"},
	}.Encode()
	_, _, _ = m.request(login, "") // 预热拿 session cookies
	u := miguPassport + "/api/qrcWeb/qrcLogin?" + url.Values{"sourceid": {miguQRSource}}.Encode()
	body, _, err := m.request(u, login)
	if err != nil {
		return nil, err
	}
	var out struct {
		Status int `json:"status"`
		Result struct {
			QrcURL     string `json:"qrcUrl"`
			SessionID  string `json:"qrc_sessionid"`
		} `json:"result"`
	}
	if err := json.Unmarshal(body, &out); err != nil {
		return nil, err
	}
	if out.Status != 2000 || out.Result.QrcURL == "" || out.Result.SessionID == "" {
		return nil, fmt.Errorf("生成二维码失败（status=%d）", out.Status)
	}
	// data:image/jpeg;base64,... → 原始图片
	imgB64 := out.Result.QrcURL
	if i := strings.Index(imgB64, ","); i >= 0 {
		imgB64 = imgB64[i+1:]
	}
	img, err := base64.StdEncoding.DecodeString(imgB64)
	if err != nil {
		return nil, err
	}
	return &qrSession{
		Source: "migu", Key: out.Result.SessionID, Status: qrWaiting, Image: img,
	}, nil
}

// checkMiguQR 轮询 + token 换 Cookie。
func checkMiguQR(s *qrSession) error {
	m := newMiguAuth()
	// 预热（复用 session cookies）
	login := miguPassport + "/login?" + url.Values{
		"sourceid": {miguQRSource}, "apptype": {"2"}, "logintype": {"3"},
	}.Encode()
	_, _, _ = m.request(login, "")
	u := miguPassport + "/api/qrcWeb/qrcquery?" + url.Values{"qrc_sessionid": {s.Key}}.Encode()
	body, _, err := m.request(u, login)
	if err != nil {
		return err
	}
	var out struct {
		Status  int    `json:"status"`
		Message string `json:"message"`
		Result  struct {
			Token string `json:"token"`
		} `json:"result"`
	}
	if err := json.Unmarshal(body, &out); err != nil {
		return err
	}
	switch out.Status {
	case 2000:
		if out.Result.Token == "" {
			s.Status = qrFailed
			return nil
		}
		cookie, err := miguTradeToken(out.Result.Token)
		if err != nil {
			s.Status = qrFailed
			s.Cookie = err.Error()
			return nil
		}
		s.Status = qrSuccess
		s.Cookie = cookie
	case 4073:
		s.Status = qrScanned
	case 4074:
		s.Status = qrWaiting
	case 4072:
		s.Status = qrExpired
	default:
		s.Status = qrFailed
	}
	return nil
}

// miguTradeToken 用 passport token 换 WEB Cookie。
func miguTradeToken(token string) (string, error) {
	u := miguGateway + "/user/h5/token-validate/v3.0?" + url.Values{
		"token": {token}, "type": {"2"}, "sourceId": {miguWebSource}, "activityId": {"MUSIC-WWW"},
	}.Encode()
	req, _ := http.NewRequest(http.MethodGet, u, nil)
	req.Header.Set("User-Agent", miguUA)
	req.Header.Set("Referer", "https://music.migu.cn/")
	cl := &http.Client{Timeout: 10 * time.Second}
	resp, err := cl.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	var out struct {
		Code string `json:"code"`
	}
	_ = json.Unmarshal(body, &out)
	if out.Code != "000000" {
		return "", fmt.Errorf("token 兑换失败（code=%s）", out.Code)
	}
	parts := []string{}
	for _, c := range resp.Cookies() {
		if c.Name != "" {
			parts = append(parts, c.Name+"="+c.Value)
		}
	}
	if len(parts) == 0 {
		return "", fmt.Errorf("未获得咪咕 Cookie")
	}
	return strings.Join(parts, "; "), nil
}

