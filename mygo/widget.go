//go:build native

package main

import (
	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

// toggleLyric 开/关桌面歌词挂件。桌面歌词是 MyGo 原生 UI 最能替代
// 原项目 cgo(GTK/Win32) 挂件的地方：无边框 + 透明 + 置顶 + 不进任务栏
// + 鼠标穿透，一行选项就够，无需任何 cgo。
func (a *app) toggleLyric() {
	if a.lyricWin != nil {
		a.lyricWin.Close()
		a.lyricWin = nil
		a.lyricShown = false
		return
	}
	a.lyricShown = true
	a.lyricWin = mygo.NewWindow(mygo.WindowOptions{
		Title:         "歌词",
		Frameless:     true,
		Transparent:   true,
		AlwaysOnTop:   true,
		SkipTaskbar:   true,
		DisableShadow: true,
		Width:         560,
		Height:        130,
		X:             200,
		Y:             80,
		Content:       ui.View(a.lyricView),
	})
	// 鼠标穿透：点到歌词等于点到桌面。
	a.lyricWin.SetIgnoreMouseEvents(true)
}

// lyricView 挂件内容：透明底 + 当前歌词行（真歌词 KTV 定位）+ 下一行预览 + 进度。
func (a *app) lyricView(c *ui.Context) {
	ui.Column(c).Fill().Center().Gap(6).Padding(16).Children(func() {
		cur, idx := a.lyricAt(a.posSec)
		text := cur
		if text == "" {
			text = a.lyricText
		}
		if text == "" {
			text = "未在播放"
		}
		// 字号档位（对齐设置页 14/16/18/22/26 + 挂件放大 1.5x）
		size := float32([]int{14, 16, 18, 22, 26}[a.lyricSize]) * 1.5
		ui.Text(c, text).FontSize(size).Bold().MaxLines(1).TextAlign(ui.Center).TextColor(ui.RGB(0xff, 0xff, 0xff))
		// 下一行预览（对齐主线挂件双行语义）
		if idx >= 0 && idx+1 < len(a.lyricLines) {
			if nxt := lyricLineText(a.lyricLines[idx+1]); nxt != "" {
				ui.Text(c, nxt).FontSize(size * 0.55).MaxLines(1).TextAlign(ui.Center).TextColor(ui.RGBA(0xff, 0xff, 0xff, 0.55))
			}
		}
		ui.Progress(c, a.progress)
	})
}
