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
		Width:         int(560 * uiScale),
		Height:        int(130 * uiScale),
		X:             int(200 * uiScale),
		Y:             int(80 * uiScale),
		Content:       ui.View(a.lyricView),
	})
	// 可交互（学主线 subtitle-bar：pointerdown 拖拽 + 悬停浮现关闭；不再鼠标穿透）
	a.lyricWin.SetIgnoreMouseEvents(false)
}

// lyricView 挂件内容：透明底 + 当前歌词行（真歌词 KTV 定位）+ 下一行预览 + 进度。
func (a *app) lyricView(c *ui.Context) {
	// 挂件透明：mygo ui.View 根默认 theme.Background（不透明），显式置透明
	// 才对（对齐主线 subtitle-bar 无背景；白字靠 text-shadow 投影兜底）。
	c.Root().Background(ui.Transparent)
	// DragWindow：整条作为窗口拖拽句柄（学主线 subtitle.js pointerdown/move 拖动定位）
	ui.Column(c).Fill().Center().Gap(6 * uiScale).Padding(16).DragWindow().Children(func() {
		// 悬停浮现控制按钮面板（学主线 subtitle-bar group-hover 浮现控制；
		// 复用 dock 控件：上一首/播放/下一首/收藏/关闭）
		if c.Root().Hovered() {
			ui.Row(c).Absolute().Top(2).Left(0).Right(0).Justify(ui.Center).Gap(8 * uiScale).Children(func() {
				if iconButton2(c, icPrev, 26, clrMuted).Clicked() {
					go a.prev()
				}
				if playCircle(c, a.playing, 32).Clicked() {
					a.togglePlay()
				}
				if iconButton2(c, icNext, 26, clrMuted).Clicked() {
					go a.next()
				}
				if iconButton2(c, icHeart, 26, accent(a.isFav())).Clicked() {
					a.toggleFav()
				}
				if iconButton2(c, icMsg, 26, accent(a.captionShown)).Clicked() {
					a.toggleCaption()
				}
				if iconButton2(c, icClose, 26, clrMuted).Clicked() {
					a.lyricWin.Close()
					a.lyricWin = nil
					a.lyricShown = false
					a.win.Invalidate()
				}
			})
		}
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
		ui.Text(c, text).FontSize(size).Bold().MaxLines(1).TextAlign(ui.Center).TextColor(ui.RGB(0xff, 0xff, 0xff)).
			TextShadow(0, 1, 3, ui.RGBA(0, 0, 0, 0.95))
		// 下一行预览（对齐主线挂件双行语义）
		if idx >= 0 && idx+1 < len(a.lyricLines) {
			if nxt := lyricLineText(a.lyricLines[idx+1]); nxt != "" {
				ui.Text(c, nxt).FontSize(size*0.55).MaxLines(1).TextAlign(ui.Center).TextColor(ui.RGBA(0xff, 0xff, 0xff, 0.55)).
					TextShadow(0, 1, 3, ui.RGBA(0, 0, 0, 0.8))
			}
		}
		ui.Progress(c, a.progress)
	})
}
