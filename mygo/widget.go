//go:build native && !android

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
		X:             200,
		Y:             80,
		Content:       ui.View(a.lyricView),
	})
	// 整窗始终可交互（不设 SetInputRegion）：之前只让中间歌词带接收鼠标，
	// DragWindow 的按下常落在 region 切换窗口期导致"拖不动"。
	// 挂件窗口小 + 透明背景，整窗可拖不影响使用；控制条仍只在 hover 时浮现。
	a.lyricHot = true // 一开挂件就露出控制条，用户能直接点/拖
}

// lyricView 挂件内容：透明底 + 当前歌词行（真歌词 KTV 定位）+ 下一行预览 + 进度。
func (a *app) lyricView(c *ui.Context) {
	// 挂件透明：mygo ui.View 根默认 theme.Background（不透明），显式置透明
	// 才对（对齐主线 subtitle-bar 无背景；白字靠 text-shadow 投影兜底）。
	c.Root().Background(ui.Transparent)
	// 整窗可拖：DragWindow 挂在填充整个窗口的 Column 上。
	// 控制条常显（用户反馈 hover 浮现太快消失、不好点），不依赖 hover。
	root := ui.Column(c).Fill().Center().Gap(6 * uiScale).Padding(16).DragWindow()
	root.Children(func() {
		// 控制条常显：上一首/播放/下一首/收藏/关闭（用户：面板别太快消失）
		ctrlRow := ui.Row(c).Absolute().Left(0).Right(0).Justify(ui.Center).Gap(8 * uiScale)
		if a.lyricCtrlBottom {
			ctrlRow.Bottom(2)
		} else {
			ctrlRow.Top(2)
		}
		ctrlRow.Children(func() {
			if iconButton2(c, icPrev, 26, clrMuted).Clicked() {
				go a.prev()
			}
			if playCircle(c, a.playing, 32).Clicked() {
				a.togglePlay()
			}
			if iconButton2(c, icNext, 26, clrMuted).Clicked() {
				go a.next()
			}
			if heartButton(c, a.isFav(), 26, func() {}).Clicked() {
				a.toggleFav()
			}
			if iconButton2(c, icClose, 26, clrMuted).Clicked() {
				a.lyricWin.Close()
				a.lyricWin = nil
				a.lyricShown = false
				a.win.Invalidate()
			}
		})
		cur, idx := a.lyricAt(a.posSec)
		text := cur
		if text == "" {
			text = a.lyricText
		}
		if text == "" {
			text = "未在播放"
		}
		// 字号档位（对齐设置页 14/16/18/22/26）× 挂件 1.5 基线 × 系统 uiScale
		// （之前写死 *1.5 没跟系统缩放 → 桌面歌词字体不被 150% 放大）
		size := float32([]int{14, 16, 18, 22, 26}[a.lyricSize]) * 1.5 * uiScale
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
