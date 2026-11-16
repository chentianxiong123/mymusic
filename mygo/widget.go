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
		if a.lyricMotionCancel != nil {
			a.lyricMotionCancel()
			a.lyricMotionCancel = nil
		}
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
	// 默认整窗鼠标穿透；聚焦用 X11 全局鼠标移动事件（事件驱动、零轮询）。
	// 缓存挂件屏幕矩形（OnMove/OnResize 更新），供事件回调判断鼠标是否在挂件上。
	a.lyricHot = false
	if a.lyricHoverTimer != nil {
		a.lyricHoverTimer.Stop()
		a.lyricHoverTimer = nil
	}
	a.lyricWin.SetIgnoreMouseEvents(true)
	a.setLyricRect(200, 80, 560, 130)
	w := a.lyricWin
	updateRect := func() {
		b := w.Bounds()
		a.setLyricRect(b.X, b.Y, b.Width, b.Height)
	}
	w.OnMove(updateRect)
	w.OnResize(updateRect)
	// 事件驱动：注册 X11 全局鼠标移动回调（root window XSelectInput + XNextEvent）。
	// 进入挂件 → AfterFunc 1 秒取消穿透；移出 → 恢复穿透。零轮询。
	a.lyricMotionCancel = addX11MotionCB(a.lyricMotionCB)
}

func (a *app) setLyricRect(x, y, w, h int) {
	a.lyricRectMu.Lock()
	a.lyricRect = mygo.Rectangle{X: x, Y: y, Width: w, Height: h}
	a.lyricRectMu.Unlock()
}

func (a *app) getLyricRect() mygo.Rectangle {
	a.lyricRectMu.Lock()
	defer a.lyricRectMu.Unlock()
	return a.lyricRect
}

// lyricMotionCB X11 全局鼠标移动回调（在 XNextEvent goroutine，非主线程）。
// 判断鼠标是否在挂件矩形内：进入 → 启动 1 秒 AfterFunc 取消穿透；移出 → 取消
// 定时器并恢复穿透。SetIgnoreMouseEvents/Invalidate 内部投递主线程，线程安全。
func (a *app) lyricMotionCB(x, y int32) {
	if a.lyricWin == nil {
		return
	}
	r := a.getLyricRect()
	inside := x >= int32(r.X) && x <= int32(r.X+r.Width) && y >= int32(r.Y) && y <= int32(r.Y+r.Height)
	if inside {
		if !a.lyricHot && a.lyricHoverTimer == nil {
			a.lyricHoverTimer = time.AfterFunc(time.Second, func() {
				if a.lyricWin != nil {
					a.lyricHot = true
					a.lyricWin.SetIgnoreMouseEvents(false) // 取消穿透：可拖拽 + 控制按钮
					a.win.Invalidate()
				}
			})
		}
		return
	}
	// 移出挂件矩形
	if a.lyricHoverTimer != nil {
		a.lyricHoverTimer.Stop()
		a.lyricHoverTimer = nil
	}
	if a.lyricHot {
		a.lyricHot = false
		if a.lyricWin != nil {
			a.lyricWin.SetIgnoreMouseEvents(true) // 恢复穿透
		}
		a.win.Invalidate()
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
					if a.lyricMotionCancel != nil {
						a.lyricMotionCancel()
						a.lyricMotionCancel = nil
					}
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
