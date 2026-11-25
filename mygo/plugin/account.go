//go:build native

// account.go —— 平台登录态（一体化模型：cookie 就是 plugin:<platform>/cookie）。
// 所有能力（音源/歌词/推荐）按平台 key 读同一份设置——身份合并后天然共享，
// 不需要单独的命名空间。旧插件的 plugin:source/<platform>/cookie 由 GetSetting 自动迁移。
package plugin

// GetCookie 读平台登录态（走一体化设置；旧键由 GetSetting 迁移）。
func GetCookie(platform string) string {
	return GetSetting("source", platform, "cookie", "")
}

// SetCookie 写平台登录态。
func SetCookie(platform, cookie string) error {
	return SetSetting("source", platform, "cookie", cookie)
}

// ClearCookie 退出登录（清空）。
func ClearCookie(platform string) {
	_ = SetSetting("source", platform, "cookie", "")
}

// HasCookie 是否已登录（非空即登录态；有效性校验在登录域 login_verify）。
func HasCookie(platform string) bool { return GetCookie(platform) != "" }
