//go:build native

package main

import (
	"time"

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
	// 默认整窗鼠标穿透；聚焦用全局光标轮询 lyricHoverLoop。
	// 为什么轮询：穿透窗口收不到任何鼠标事件（GDK 不投 hover、Xlib root motion
	// 真鼠标走 GNOME XInput2 收不到、xgb 无 XInput2 包）。查全局光标位置
	// mygo.Screen.CursorScreenPoint() 是真鼠标唯一可靠方式（X server 维护光标
	// 位置，不分输入路径）。代价 100ms 一次 XQueryPointer，可忽略。
	a.lyricHot = false
	a.lyricHoverStart = time.Time{}
	a.lyricWin.SetIgnoreMouseEvents(true)
	if !a.lyricLooping {
		a.lyricLooping = true
		go a.lyricHoverLoop()
	}
}

// lyricHoverMs 悬停浮现控制条的时长（对齐主线 widget.go HoverMs = 1200ms）
const lyricHoverMs = 1200 * time.Millisecond

// lyricHoverLoop 全局光标轮询（150ms，对齐主线 g_timeout_add(150)）：鼠标在挂件
// 矩形内持续 lyricHoverMs（1.2 秒，对齐主线 HoverMs）→ 取消穿透（可拖拽 + 控制
// 按钮）；移出 → 恢复穿透。与主线 widget.go 同源方案（CursorInside 也走
// XQueryPointer），性能损耗主线已真机验证可接受。
func (a *app) lyricHoverLoop() {
	defer func() { a.lyricLooping = false }()
	for a.lyricWin != nil {
		time.Sleep(150 * time.Millisecond)
		w := a.lyricWin
		if w == nil {
			return
		}
		p := mygo.Screen.CursorScreenPoint()
		wx, wy := w.Position()
		ww, wh := w.Size()
		inside := p.X >= wx && p.X <= wx+ww && p.Y >= wy && p.Y <= wy+wh
		now := time.Now()
		if inside {
			if a.lyricHoverStart.IsZero() {
				a.lyricHoverStart = now
			}
			if !a.lyricHot && now.Sub(a.lyricHoverStart) >= lyricHoverMs {
				a.lyricHot = true
				w.SetIgnoreMouseEvents(false) // 取消穿透：可拖拽 + 控制按钮
				a.win.Invalidate()
			}
		} else {
			a.lyricHoverStart = time.Time{}
			if a.lyricHot {
				a.lyricHot = false
				w.SetIgnoreMouseEvents(true) // 恢复穿透
				a.win.Invalidate()
			}
		}
	}
}

// lyricView 挂件内容：透明底 + 当前歌词行（真歌词 KTV 定位）+ 下一行预览 + 进度。
func (a *app) lyricView(c *ui.Context) {
	// 挂件透明：mygo ui.View 根默认 theme.Background（不透明），显式置透明
	// 才对（对齐主线 subtitle-bar 无背景；白字靠 text-shadow 投影兜底）。
	c.Root().Background(ui.Transparent)
	// DragWindow：整条作为窗口拖拽句柄（学主线 subtitle.js pointerdown/move 拖动定位）；
	// 只有聚焦态（SetIgnoreMouseEvents(false)）才拖得动，穿透态鼠标直接穿过。
	ui.Column(c).Fill().Center().Gap(6 * uiScale).Padding(16).DragWindow().Children(func() {
		// 聚焦后浮现控制按钮面板（学主线 subtitle-bar group-hover 浮现控制；
		// 复用 dock 控件：上一首/播放/下一首/收藏/关闭）
		if a.lyricHot {
			ui.Row(c).Absolute().Top(2).Left(0).Right(0).Justify(ui.Center).Gap(8 * uiScale).Children(func() {
				if iconButton2(c, icPrev, 26*uiScale, clrMuted).Clicked() {
					go a.prev()
				}
				if playCircle(c, a.playing, 32*uiScale).Clicked() {
					a.togglePlay()
				}
				if iconButton2(c, icNext, 26*uiScale, clrMuted).Clicked() {
					go a.next()
				}
				if heartButton(c, a.isFav(), 26*uiScale, func() {}).Clicked() {
					a.toggleFav()
				}
				if iconButton2(c, icClose, 26*uiScale, clrMuted).Clicked() {
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
