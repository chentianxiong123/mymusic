//go:build native

// login_verify_test.go —— 登录态文案/缓存逻辑测试（不碰外网）。
package main

import "testing"

func TestLoginStateLabel(t *testing.T) {
	cases := []struct {
		cookie string
		chk    loginCheck
		want   string
	}{
		{"", loginCheck{}, "未登录"},
		{"x=1", loginCheck{}, "已登录"},                              // 无法校验的平台
		{"x=1", loginCheck{Checked: true, Valid: true}, "已登录"},      // 校验通过无名
		{"x=1", loginCheck{Checked: true, Valid: false}, "已过期"},    // 过期
		{"x=1", loginCheck{Checked: true, Valid: true, Name: "小明"}, "已登录 · 小明"},
		{"x=1", loginCheck{Checked: true, Valid: true, Name: "很长很长的账号昵称超过十二字"}, "已登录 · 很长很长的账号昵称超过十…"},
	}
	for _, c := range cases {
		if got := loginStateLabel(c.cookie, c.chk); got != c.want {
			t.Errorf("label(%q,%+v) = %q, want %q", c.cookie, c.chk, got, c.want)
		}
	}
}

func TestLoginCheckCache(t *testing.T) {
	setLoginCheck("bilibili", loginCheck{Checked: true, Valid: false})
	got := getLoginCheck("bilibili")
	if !got.Checked || got.Valid {
		t.Fatalf("cache 读写错: %+v", got)
	}
	if empty := getLoginCheck("netease"); empty.Checked {
		t.Fatalf("未校验源应零值: %+v", empty)
	}
}
