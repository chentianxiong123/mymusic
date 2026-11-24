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
	// 纯事件驱动：SetInputRegion 只让歌词带接收鼠标（其余穿透到下层窗口）。
	// 鼠标进入歌词带 → Hovered() 触发展开（全窗可交互）；移出 → 恢复歌词带。
	// Linux 下 X11 SHAPE 是真 input shape（零轮询），这是 mygo 官方桌宠方案。
	a.lyricHot = false
	a.lyricWin.SetInputRegion(lyricBandRegion())
}

// lyricBandRegion 挂件"歌词带"输入区（窗口 DIP 坐标）：只让中间歌词文字条
// 接收鼠标，其余区域穿透（官方桌宠方案 SetInputRegion）。
// 窗口 560×130，歌词居中；上下各留 20px 让控制条/空白区平时穿透。
func lyricBandRegion() []mygo.Rectangle {
	w, h := int(560*uiScale), int(130*uiScale)
	bandH := int(58 * uiScale)
	return []mygo.Rectangle{{X: 0, Y: (h - bandH) / 2, Width: w, Height: bandH}}
}

// lyricView 挂件内容：透明底 + 当前歌词行（真歌词 KTV 定位）+ 下一行预览 + 进度。
func (a *app) lyricView(c *ui.Context) {
	// 挂件透明：mygo ui.View 根默认 theme.Background（不透明），显式置透明
	// 才对（对齐主线 subtitle-bar 无背景；白字靠 text-shadow 投影兜底）。
	c.Root().Background(ui.Transparent)
	// 纯事件驱动聚焦：Hovered() 在鼠标进入/离开歌词带时自动重建视图（mygo ui 保证）。
	// 用同一个承载内容的 Column 读 Hovered —— 保证命中区与实际内容一致。
	// 状态变化时用 AfterFunc(0) 投递到下一 tick 切 input region，避免在 view 构建中
	// 直接改窗口属性（onMain 在主线程同步执行，构建中改可能干扰布局）。
	root := ui.Column(c).Fill().Center().Gap(6 * uiScale).Padding(16).DragWindow()
	hot := root.Hovered()
	if hot != a.lyricHot {
		a.lyricHot = hot
		w := a.lyricWin
		time.AfterFunc(0, func() {
			if w == nil {
				return
			}
			if hot {
				w.SetInputRegion(nil) // 整窗可交互：控制条/拖拽
			} else {
				w.SetInputRegion(lyricBandRegion()) // 恢复：只有歌词带接收鼠标
			}
		})
	}
	// DragWindow：整条作为窗口拖拽句柄（学主线 subtitle.js pointerdown/move 拖动定位）；
	// 只有聚焦态（整窗 region）才拖得动，穿透态鼠标直接穿过。
	root.Children(func() {
		// 聚焦后浮现控制按钮面板（学主线 subtitle-bar group-hover 浮现控制；
		// 复用 dock 控件：上一首/播放/下一首/收藏/关闭）
		if a.lyricHot {
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
				if playCircle(c, a.playing, 32*uiScale).Clicked() {
					a.togglePlay()
				}
				if iconButton2(c, icNext, 26, clrMuted).Clicked() {
					go a.next()
				}
				if heartButton(c, a.isFav(), 26*uiScale, func() {}).Clicked() {
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
