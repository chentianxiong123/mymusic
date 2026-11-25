//go:build native

// account.go —— 平台级登录态（按平台名共享，不按能力类型细分）。
//
// 背景：插件按能力拆成 source/lyric/recommend 后，登录态存 plugin:source/<平台>/cookie，
// 歌词/推荐插件读不到同一份，用户得重复登录。此层把登录态收敛到平台命名空间：
//
//   plugin:account/<platform>/cookie    ← 唯一真相
//   plugin:source/<platform>/cookie     ← 老用户兼容：首次读时迁移到此键
//
// 调用方一律用本包的 GetCookie/SetCookie/ClearCookie，不直接碰 GetSetting("source"...)。
// 能力类型（source/lyric/recommend）仍需各自的设置声明（UI 显示输入框），但存储统一走这里。
package plugin

const accountPrefix = "account/"

func accountCookieKey(platform string) string {
	return settingKeyPrefix + accountPrefix + platform + "/cookie"
}

func legacyCookieKey(platform string) string {
	return settingKeyPrefix + "source/" + platform + "/cookie"
}

// GetCookie 读平台登录态（新键优先；老键存在则迁移并清除老键）。
func GetCookie(platform string) string {
	settingsMu.RLock()
	store := settingStore
	settingsMu.RUnlock()
	if store == nil {
		return ""
	}
	if v, ok := store.Get(accountCookieKey(platform)); ok && v != "" {
		return v
	}
	// 兼容迁移：老用户存在 source/<platform>/cookie，搬到平台级键。
	if v, ok := store.Get(legacyCookieKey(platform)); ok && v != "" {
		_ = store.Set(accountCookieKey(platform), v)
		_ = store.Set(legacyCookieKey(platform), "") // 清空老键，避免双写分歧
		return v
	}
	return ""
}

// SetCookie 写平台登录态（新键；同时清 source 老键避免旧值残留）。
func SetCookie(platform, cookie string) error {
	settingsMu.RLock()
	store := settingStore
	settingsMu.RUnlock()
	if store == nil {
		return nil
	}
	if err := store.Set(accountCookieKey(platform), cookie); err != nil {
		return err
	}
	_ = store.Set(legacyCookieKey(platform), "") // 双写兼容：老键保持空
	return nil
}

// ClearCookie 退出登录：清平台登录态（新键 + 老键都空）。
func ClearCookie(platform string) {
	settingsMu.RLock()
	store := settingStore
	settingsMu.RUnlock()
	if store == nil {
		return
	}
	_ = store.Set(accountCookieKey(platform), "")
	_ = store.Set(legacyCookieKey(platform), "")
}

// HasCookie 是否已登录（非空即登录态；有效性校验在登录域 login_verify）。
func HasCookie(platform string) bool { return GetCookie(platform) != "" }
