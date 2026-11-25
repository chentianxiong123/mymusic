//go:build native

package plugin

import "testing"

// TestCookieSharedAcrossCapabilities 登录态按平台共享：source 写入后 lyric/recommend 同名插件读到同一份。
func TestCookieSharedAcrossCapabilities(t *testing.T) {
	fs := &fakeStore{}
	SetSettingStore(fs)
	t.Cleanup(func() { SetSettingStore(nil) })

	if err := SetCookie("netease", "MUSIC_U=abc"); err != nil {
		t.Fatal(err)
	}
	// 所有能力读同一份（GetSetting 的 type 参数被忽略）
	if got := GetSetting("source", "netease", "cookie", ""); got != "MUSIC_U=abc" {
		t.Fatalf("source 读: %q", got)
	}
	if got := GetSetting("lyric", "netease", "cookie", ""); got != "MUSIC_U=abc" {
		t.Fatalf("lyric 读: %q", got)
	}
	if got := GetSetting("recommend", "netease", "cookie", ""); got != "MUSIC_U=abc" {
		t.Fatalf("recommend 读: %q", got)
	}
	ClearCookie("netease")
	if HasCookie("netease") {
		t.Fatal("ClearCookie 后仍有登录态")
	}
}

// TestLegacySettingMigration 老键 plugin:source/<name>/<key> 自动迁移到 plugin:<name>/<key>。
func TestLegacySettingMigration(t *testing.T) {
	fs := &fakeStore{m: map[string]string{
		"plugin:source/netease/cookie": "LEGACY_COOKIE",
		"plugin:source/netease/quality": "320k",
	}}
	SetSettingStore(fs)
	t.Cleanup(func() { SetSettingStore(nil) })

	if got := GetSetting("source", "netease", "cookie", ""); got != "LEGACY_COOKIE" {
		t.Fatalf("cookie 迁移: %q", got)
	}
	if got := GetSetting("lyric", "netease", "cookie", ""); got != "LEGACY_COOKIE" {
		t.Fatalf("跨能力读迁移后的 cookie: %q", got)
	}
	if got := GetSetting("source", "netease", "quality", ""); got != "320k" {
		t.Fatalf("quality 迁移: %q", got)
	}
	if got := fs.m["plugin:source/netease/cookie"]; got != "" {
		t.Fatalf("老键应被清空: %q", got)
	}
}
