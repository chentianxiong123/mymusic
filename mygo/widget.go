//go:build native

package main

import (
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

// lyricHandleW 挂件左边缘把手宽（DIP）：默认只有这条接收鼠标（其余穿透到
// 下层窗口/桌面），鼠标进把手 1 秒 → 全窗口可交互（拖拽+控制），移出恢复穿透。
const lyricHandleW = 12

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
	// 默认鼠标穿透（事件驱动聚焦，无轮询）：SetInputRegion 只保留左边缘把手
	// 区接收鼠标，其余穿透到下层；鼠标进把手满 1 秒 → 全窗口可交互，移出恢复。
	a.lyricHot = false
	a.lyricHoverStart = time.Time{}
	if a.lyricHoverTimer != nil {
		a.lyricHoverTimer.Stop()
		a.lyricHoverTimer = nil
	}
	a.lyricWin.SetInputRegion([]mygo.Rectangle{{X: 0, Y: 0, Width: lyricHandleW, Height: 130}})
}

// lyricView 挂件内容：透明底 + 当前歌词行（真歌词 KTV 定位）+ 下一行预览 + 进度。
func (a *app) lyricView(c *ui.Context) {
	// 挂件透明：mygo ui.View 根默认 theme.Background（不透明），显式置透明
	// 才对（对齐主线 subtitle-bar 无背景；白字靠 text-shadow 投影兜底）。
	c.Root().Background(ui.Transparent)
	// 把手聚焦（事件驱动，无轮询）：穿透下窗口收不到鼠标事件，靠 SetInputRegion
	// 保留把手区接收鼠标；Hovered 在把手区为真 → AfterFunc 满 1 秒取消穿透；
	// 移出把手/窗口 → 恢复穿透（SetInputRegion 只留把手）。
	if c.Root().Hovered() {
		if a.lyricHoverStart.IsZero() {
			a.lyricHoverStart = time.Now()
			a.lyricHoverTimer = time.AfterFunc(time.Second, func() {
				if a.lyricWin != nil && !a.lyricHot {
					a.lyricHot = true
					a.lyricWin.SetInputRegion(nil) // 全窗口可交互（拖拽 + 控制按钮）
					a.win.Invalidate()
				}
			})
		}
	} else {
		a.lyricHoverStart = time.Time{}
		if a.lyricHoverTimer != nil {
			a.lyricHoverTimer.Stop()
			a.lyricHoverTimer = nil
		}
		if a.lyricHot {
			a.lyricHot = false
			a.lyricWin.SetInputRegion([]mygo.Rectangle{{X: 0, Y: 0, Width: lyricHandleW, Height: 130}}) // 恢复穿透
			a.win.Invalidate()
		}
	}
	// DragWindow：整条作为窗口拖拽句柄（学主线 subtitle.js pointerdown/move 拖动定位）；
	// 只有聚焦态（全窗口可交互）才拖得动，穿透态仅把手区可触发聚焦。
	ui.Column(c).Fill().Center().Gap(6 * uiScale).Padding(16).DragWindow().Children(func() {
		// 左边缘把手指示（穿透时提示拖动入口；聚焦后高亮）
		ui.Box(c).Absolute().Left(3).Top(0).Bottom(0).Width(3).Radius(2).
			Background(ui.RGBA(1, 1, 1, 0.22))
		// 聚焦后浮现控制按钮面板（学主线 subtitle-bar group-hover 浮现控制；
		// 复用 dock 控件：上一首/播放/下一首/收藏/关闭）
		if a.lyricHot {
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
