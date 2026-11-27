//go:build native

package main

// lyricOn 歌词开关当前状态：桌面=桌面挂件窗口，安卓=悬浮歌词 Service。
func (a *app) lyricOn() bool {
	return a.lyricShown || a.androidLyricOn
}
