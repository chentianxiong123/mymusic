//go:build native

package plugin

import "testing"

// TestCookieSharedAcrossCapabilities 登录态按平台共享：source 写入后 lyric/recommend 同名插件读到同一份。
func TestCookieSharedAcrossCapabilities(t *testing.T) {
	fs := &fakeStore{}
	SetSettingStore(fs)
	t.Cleanup(func() { SetSettingStore(nil) })

	// 音源插件写入 cookie（走 account 层）
	if err := SetCookie("netease", "MUSIC_U=abc"); err != nil {
		t.Fatal(err)
	}
	// 歌词/推荐插件同名 key 读同一份（不按 pluginType 隔离）
	if got := GetCookie("netease"); got != "MUSIC_U=abc" {
		t.Fatalf("共享读取: got %q", got)
	}
	// 旧 type 命名空间下不应有残留（避免双写分歧）
	if got := GetSetting("source", "netease", "cookie", "OLD"); got != "" {
		t.Fatalf("老键应被清空，got %q", got)
	}
	// 退出清除
	ClearCookie("netease")
	if HasCookie("netease") {
		t.Fatal("ClearCookie 后仍有登录态")
	}
}

// TestCookieLegacyMigration 老用户老键自动迁移到平台级键。
func TestCookieLegacyMigration(t *testing.T) {
	fs := &fakeStore{}
	SetSettingStore(fs)
	t.Cleanup(func() { SetSettingStore(nil) })

	// 模拟老用户：只存在 plugin:source/netease/cookie
	_ = SetSetting("source", "netease", "cookie", "LEGACY_COOKIE")

	got := GetCookie("netease")
	if got != "LEGACY_COOKIE" {
		t.Fatalf("迁移失败: got %q", got)
	}
	// 再读一次仍稳定（老键已清，新键保留）
	if got := GetCookie("netease"); got != "LEGACY_COOKIE" {
		t.Fatalf("迁移后二次读: got %q", got)
	}
}
