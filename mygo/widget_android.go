//go:build native && android

package main

// 安卓歌词开关：不创建第二个 mygo 窗口（移动端只有单 WebView 窗口，
// mygo.NewWindow 在安卓上会起一个盖住主页面的新窗口）。改走 Kotlin
// 悬浮歌词 Service（widgetoverlay），与主页面互不干扰。

import "mymusic-mygo/widgetoverlay"

// toggleLyric 安卓版：开/关悬浮歌词 Service。
// 未获"显示在其他应用上层"权限时 TryStart 会拉起权限设置页，
// 返回 false 表示需要用户授权（按钮状态由调用方按 a.androidLyricOn 渲染）。
func (a *app) toggleLyric() {
	if a.androidLyricOn {
		_ = widgetoverlay.Stop()
		a.androidLyricOn = false
		a.savePref("mygo_androlyric", 0)
	} else if widgetoverlay.TryStart() {
		a.androidLyricOn = true
		a.savePref("mygo_androlyric", 1)
	}
	a.win.Invalidate()
}
