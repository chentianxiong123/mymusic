//go:build native

// 现代化 UI 组件：深色 + 紫渐变，全部用 ui.Painter 自绘（渐变/圆角/阴影/圆角封面）。
package main

import (
	"image/color"

	"github.com/egoist/mygo/ui"

	"homecast/bilibili"
)

// 配色（默认深色 + 紫粉渐变；主题模式切换时 applyTheme 重设，对齐主线 THEMES 三态）
var (
	clrBg        = ui.RGB(0x0e, 0x0f, 0x14)        // 窗口背景（近黑）
	clrCard      = ui.RGB(0x18, 0x19, 0x22)        // 卡片
	clrCardGlass = ui.RGBA(0x18, 0x19, 0x22, 0.72) // 玻璃卡片（对齐主线 bg-gray-900/70 半透明）
	clrCardHov   = ui.RGB(0x22, 0x23, 0x2e)        // 卡片 hover
	clrField     = ui.RGB(0x1b, 0x1c, 0x27)        // 输入框
	clrTrack     = ui.RGB(0x2b, 0x2c, 0x37)        // 进度条轨道 / 占位
	clrText      = ui.RGB(0xf2, 0xf2, 0xf6)
	clrMuted     = ui.RGB(0x8a, 0x8b, 0x97)
	clrPurple    = ui.RGB(0x8b, 0x5c, 0xf6)
	clrPink      = ui.RGB(0xec, 0x48, 0x99)
	clrBar       = ui.RGB(0x14, 0x15, 0x1d) // 播放条底色
	clrTrackBar  = ui.RGB(0x33, 0x34, 0x40) // 进度条轨道
)

// applyTheme 按主题模式重设配色（对齐主线 THEMES：auto 跟随系统黑暗偏好 / light / dark）。
// 自绘 UI 每帧由 view 开头调用；themeMode: 0 auto 1 light 2 dark。
func applyTheme(mode int) bool {
	if mode == 1 {
		// 明亮（Light）：白底卡片 + 深字
		clrBg = ui.RGB(0xf6, 0xf6, 0xfa)
		clrCard = ui.RGB(0xff, 0xff, 0xff)
		clrCardHov = ui.RGB(0xee, 0xee, 0xf4)
		clrField = ui.RGB(0xef, 0xef, 0xf5)
		clrTrack = ui.RGB(0xd8, 0xd8, 0xe2)
		clrText = ui.RGB(0x1a, 0x1a, 0x22)
		clrMuted = ui.RGB(0x6b, 0x6b, 0x78)
		clrBar = ui.RGB(0xff, 0xff, 0xff)
		clrTrackBar = ui.RGB(0xd0, 0xd0, 0xda)
		return true
	}
	// 深色（dark / auto 默认深色）
	clrBg = ui.RGB(0x0e, 0x0f, 0x14)
	clrCard = ui.RGB(0x18, 0x19, 0x22)
	clrCardHov = ui.RGB(0x22, 0x23, 0x2e)
	clrField = ui.RGB(0x1b, 0x1c, 0x27)
	clrTrack = ui.RGB(0x2b, 0x2c, 0x37)
	clrText = ui.RGB(0xf2, 0xf2, 0xf6)
	clrMuted = ui.RGB(0x8a, 0x8b, 0x97)
	clrBar = ui.RGB(0x14, 0x15, 0x1d)
	clrTrackBar = ui.RGB(0x33, 0x34, 0x40)
	return false
}

func grad() ui.LinearGradient {
	return ui.LinearGradient{From: clrPurple, To: clrPink, Angle: 90}
}

// gradButton 紫渐变圆角按钮。
// playBtn 播放按钮（对齐主线结果行 hx-play：粉紫渐变圆角 + 播放三角 + "播放"）。
func playBtn(c *ui.Context) ui.Element {
	box := ui.Box(c).Height(34).Radius(17).PaddingX(14).AlignItems(ui.Center).Justify(ui.Center).
		Draw(func(p *ui.Painter, r ui.Rect) {
			p.FillGradient(r, grad(), r.H/2)
		})
	box.Children(func() {
		ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
			// 白色播放三角
			icon(c, icPlay, 15, ui.RGB(0xff, 0xff, 0xff))
			ui.Text(c, "播放").TextColor(ui.RGB(0xff, 0xff, 0xff)).FontSize(14).Bold()
		})
	})
	return box
}

func gradButton(c *ui.Context, label string) ui.Element {
	box := ui.Box(c).Height(42).Radius(21).PaddingX(24).PaddingY(9).AlignItems(ui.Center).Justify(ui.Center).
		Draw(func(p *ui.Painter, r ui.Rect) {
			p.FillGradient(r, grad(), r.H/2)
		})
	box.Children(func() {
		ui.Text(c, label).TextColor(ui.RGB(0xff, 0xff, 0xff)).FontSize(15).Bold()
	})
	return box
}

// cover 圆角封面图（网络加载 + 占位）。url 为空显示占位。
func (a *app) cover(c *ui.Context, bvid, url string, size float32) ui.Element {
	return a.coverTagged(c, bvid, url, size, false)
}

// coverTagged 圆角封面（hoverLyric=true 时 hover 显示「歌词」标签，对齐主线 d-cover group-hover）。
func (a *app) coverTagged(c *ui.Context, bvid, url string, size float32, hoverLyric bool) ui.Element {
	box := ui.Box(c).Size(size, size)
	box.Draw(func(p *ui.Painter, r ui.Rect) {
		p.Clip(r, size/2, func() {
			if img, ok := a.covers[bvid]; ok {
				p.Image(img, r, ui.Contain)
			} else {
				p.FillRadii(r, [4]float32{size / 2, size / 2, size / 2, size / 2}, clrTrack)
				// 占位音符：两个圆点 + 两条小柄
				p.Fill(ui.Rect{X: r.X + size*0.38, Y: r.Y + size*0.42, W: size * 0.12, H: size * 0.12}, clrPurple, 0)
				p.Fill(ui.Rect{X: r.X + size*0.62, Y: r.Y + size*0.36, W: size * 0.12, H: size * 0.12}, clrPink, 0)
				p.Line(r.X+size*0.44, r.Y+size*0.54, r.X+size*0.44, r.Y+size*0.32, size*0.05, clrPurple)
				p.Line(r.X+size*0.68, r.Y+size*0.48, r.X+size*0.68, r.Y+size*0.26, size*0.05, clrPink)
			}
		})
		// hover「歌词」标签（对齐主线 group-hover 黑底圆角）
		if hoverLyric && box.Hovered() {
			p.Fill(r, ui.RGBA(0, 0, 0, 0.45), size/2)
			lbl := "歌词"
			// 用 painter 居中画小标签
			w, h := size*0.5, size*0.28
			lx, ly := r.X+(r.W-w)/2, r.Y+(r.H-h)/2
			p.FillRadii(ui.Rect{X: lx, Y: ly, W: w, H: h}, [4]float32{h / 2, h / 2, h / 2, h / 2}, ui.RGBA(0, 0, 0, 0.6))
			p.Text(lx+w/2, ly+h/2, lbl, size*0.11, ui.RGB(0xff, 0xff, 0xff))
		}
		// EQ 跳动频谱（对齐主线 d-eq：封面底部黑条 + 5 条粉色柱）
		if a.eqPhase >= 0 {
			barH := size * 0.22
			// 底部黑半透明条
			p.Fill(ui.Rect{X: r.X, Y: r.Y + r.H - barH, W: r.W, H: barH}, ui.RGBA(0, 0, 0, 0.30), 0)
			bw := size * 0.06
			gap := size * 0.045
			n := 5
			total := bw*float32(n) + gap*float32(n-1)
			startX := r.X + (r.W-total)/2
			for i := 0; i < n; i++ {
				ph := (a.eqPhase + i*2) % 6
				f := float32(0.55)
				switch ph {
				case 0:
					f = 0.25
				case 2:
					f = 0.85
				case 4:
					f = 0.45
				case 5:
					f = 0.7
				}
				bh := barH * 0.8 * f
				bx := startX + bw*float32(i) + gap*float32(i)
				// 柱底对齐黑条底部内边距（barH-bh)/2
				by := r.Y + r.H - (barH-bh)/2 - bh
				p.Fill(ui.Rect{X: bx, Y: by, W: bw, H: bh}, clrPink, bw/2)
			}
		}
	})
	if url != "" {
		a.loadCover(bvid, url)
	}
	return box
}

// rowShell 统一列表行容器：定高圆角卡 + 内容行 Fill 垂直居中。
// 所有列表行（搜索/收藏/队列）复用，保证内部组件与容器对齐/居中统一。
func (a *app) rowShell(c *ui.Context, h float32, fn func()) ui.Element {
	box := ui.Box(c).Fill().Height(h).Radius(12).PaddingX(12)
	box.Draw(func(p *ui.Painter, r ui.Rect) {
		if box.Hovered() {
			p.Fill(r, clrCardHov, 12)
		}
	})
	box.Children(func() {
		// AlignSelf(Stretch)：内容行填满行高 + AlignItems(Center) 垂直居中
		// —— 否则 Row 高度=内容，在定高 box 顶部对齐（不居中）
		ui.Row(c).AlignSelf(ui.Stretch).Gap(a.resp(12, 8)).AlignItems(ui.Center).Children(fn)
	})
	return box
}

// listRow 搜索结果列表行：小封面 + 标题/作者 + 收藏 + 播放。点击行播放。
func (a *app) listRow(c *ui.Context, item bilibili.SearchItem) {
	// 标题展开态（对齐主线 hx-title 点击展开/收起完整标题）
	expanded := a.expandedTitle == item.BVID
	btnHit := false // 点击了行内 ♥/播放按钮（收藏/播放独立生效，不触发展开标题）
	rowH := float32(68)
	if expanded {
		rowH = 84
	}
	box := a.rowShell(c, rowH, func() {
		// 封面（对齐主线 w-12 h-9 sm:w-20 sm:h-14 = 48×36 / 80×56 横向）
		a.coverWide(c, item.BVID, item.Pic, a.resp(80, 48), a.resp(56, 36))
		ui.Column(c).Grow(1).Gap(2).Children(func() {
			title := bilibili.CleanHTML(item.Title)
			tb := ui.Text(c, title).FontSize(a.resp(15, 14)).Bold().TextColor(clrText)
			if expanded {
				tb.MaxLines(0)
			} else {
				tb.MaxLines(1)
			}
			ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
				ui.Text(c, item.Author).FontSize(13).TextColor(clrMuted).MaxLines(1)
				if item.Duration != "" {
					ui.Text(c, item.Duration).FontSize(12).TextColor(clrMuted)
				}
			})
		})
		if iconButton2(c, icHeart, a.resp(34, 30), accent(a.isFavItem(item))).Clicked() {
			a.toggleFavItem(item)
			btnHit = true
		}
		// 播放按钮始终显示（对齐主线结果行 hx-play：粉紫渐变圆角 + ▶ + 播放）
		if playBtn(c).Clicked() {
			go a.play(item)
			btnHit = true
		}
	})
	// 展开标题仅在点击行主体（非 ♥/播放按钮）时触发，避免点收藏却展开（收藏独立生效）
	if box.Clicked() && !btnHit {
		if a.expandedTitle == item.BVID {
			a.expandedTitle = ""
		} else {
			a.expandedTitle = item.BVID
		}
		a.win.Invalidate()
	}
	// 右键 → 更多菜单（对齐主线 contextmenu）
	if btn, ok := box.PressedButton(); ok && btn == 2 {
		it := item
		a.openCtxMenu(&it)
	}
}

// playTriangle 返回一个播放三角 Path（中心偏右）。
func playTriangle(r ui.Rect) *ui.Path {
	p := &ui.Path{}
	p.MoveTo(r.X+r.W*0.36, r.Y+r.H*0.30)
	p.LineTo(r.X+r.W*0.72, r.Y+r.H*0.50)
	p.LineTo(r.X+r.W*0.36, r.Y+r.H*0.70)
	p.Close()
	return p
}

// progressBar 渐变进度条（底部播放条，可 seek）。
func (a *app) progressBar(c *ui.Context, frac float64) ui.Element {
	return a.progressBarSized(c, frac, 5, true)
}

// progressBarSized 进度条；seek=true 时点击/拖动跳转。
func (a *app) progressBarSized(c *ui.Context, frac float64, h float32, seek bool) ui.Element {
	box := ui.Box(c).AlignSelf(ui.Stretch).Height(h).Radius(h / 2)
	box.Draw(func(p *ui.Painter, r ui.Rect) {
		p.FillRadii(r, [4]float32{r.H / 2, r.H / 2, r.H / 2, r.H / 2}, clrTrackBar)
		if frac > 0 {
			fr := ui.Rect{X: r.X, Y: r.Y, W: r.W * float32(frac), H: r.H}
			p.FillGradient(fr, grad(), r.H/2)
			p.Fill(ui.Rect{X: fr.X + fr.W - 5, Y: r.Y - 3, W: 10, H: 10}, ui.RGB(0xff, 0xff, 0xff), 5)
		}
		if seek {
			if x, _, over := box.PointerPosition(); over && (box.Pressed() || box.Dragging()) {
				a.seek(float64(x / r.W))
			}
		}
	})
	return box
}

// playCircle 大号圆形播放/暂停按钮。
func playCircle(c *ui.Context, playing bool, size float32) ui.Element {
	box := ui.Box(c).Size(size, size).Radius(size / 2)
	box.Draw(func(p *ui.Painter, r ui.Rect) {
		p.FillGradient(r, grad(), r.H/2)
		if playing {
			p.FillRadii(ui.Rect{X: r.X + r.W*0.34, Y: r.Y + r.H*0.26, W: r.W * 0.10, H: r.H * 0.48}, [4]float32{2, 2, 2, 2}, ui.RGB(0xff, 0xff, 0xff))
			p.FillRadii(ui.Rect{X: r.X + r.W*0.56, Y: r.Y + r.H*0.26, W: r.W * 0.10, H: r.H * 0.48}, [4]float32{2, 2, 2, 2}, ui.RGB(0xff, 0xff, 0xff))
		} else {
			p.FillPath(playTriangle(r), ui.RGB(0xff, 0xff, 0xff))
		}
	})
	return box
}

// iconButton 圆形图标按钮（歌词挂件开关等）。
func iconButton(c *ui.Context, glyph string) ui.Element {
	box := ui.Box(c).Size(34, 34).Radius(17).AlignItems(ui.Center).Justify(ui.Center)
	box.Draw(func(p *ui.Painter, r ui.Rect) {
		bg := clrCard
		if box.Hovered() {
			bg = clrCardHov
		}
		p.Fill(r, bg, r.H/2)
	})
	box.Children(func() {
		ui.Text(c, glyph).TextColor(clrMuted).FontSize(14)
	})
	return box
}

// qualityLabel 当前音质标签（对齐主线 d-quality 三档）。
func (a *app) qualityLabel() string {
	return []string{"64k", "128k", "192k"}[a.quality]
}

// qualityChip 音质小按钮（对齐主线 d-quality：点击循环切换）。
func qualityChip(c *ui.Context, label string, on bool) ui.Element {
	box := ui.Box(c).Height(26).Radius(8).PaddingX(8).AlignItems(ui.Center).Justify(ui.Center)
	box.Draw(func(p *ui.Painter, r ui.Rect) {
		bg := ui.RGBA(0, 0, 0, 0)
		if on || box.Hovered() {
			bg = clrField
		}
		p.Fill(r, bg, 8)
	})
	box.Children(func() {
		col := clrMuted
		if on {
			col = clrPink
		}
		ui.Text(c, label).FontSize(12).TextColor(col)
	})
	return box
}

// coverWide 4:3 横向封面（对齐主线 results.html 封面 w-12 h-9 / sm:w-20 h-14 + hover 播放遮罩）。
func (a *app) coverWide(c *ui.Context, bvid, url string, w, h float32) ui.Element {
	box := ui.Box(c).Size(w, h)
	box.Draw(func(p *ui.Painter, r ui.Rect) {
		p.Clip(r, 10, func() {
			img, ok := a.covers[bvid]
			if ok {
				p.Image(img, r, ui.Cover)
			} else {
				p.FillRadii(r, [4]float32{10, 10, 10, 10}, clrTrack)
			}
		})
		// hover 播放遮罩（对齐主线 group-hover：黑底半透明 + 播放三角）
		if box.Hovered() {
			p.Clip(r, 10, func() {
				p.Fill(r, ui.RGBA(0, 0, 0, 0.35), 0)
				cx, cy := r.X+r.W/2, r.Y+r.H/2
				tr := ui.Rect{X: cx - 8, Y: cy - 10, W: 16, H: 20}
				pp := &ui.Path{}
				pp.MoveTo(tr.X, tr.Y)
				pp.LineTo(tr.X+tr.W, tr.Y+tr.H/2)
				pp.LineTo(tr.X, tr.Y+tr.H)
				pp.Close()
				p.FillPath(pp, ui.RGB(0xff, 0xff, 0xff))
			})
		}
	})
	if url != "" {
		a.loadCover(bvid, url)
	}
	return box
}

// lyricCaption 字幕条（对齐主线 subtitle-bar）：当前歌词行（真歌词）+ 关闭按钮。
func (a *app) lyricCaption(c *ui.Context) {
	if !a.captionShown {
		return
	}
	// 歌词行文本：有真歌词用真词（对齐主线 subtitle-bar 走 lyric.js），否则回退标题
	line, _ := a.lyricAt(a.posSec)
	sub := line
	if sub == "" {
		sub = a.lyricText
	}
	if sub == "" {
		sub = "♪ " + a.current.title
	}
	// 透明字幕条（对齐主线 subtitle-bar：无背景，只歌词文字 + 悬停浮现关闭按钮）
	box := ui.Box(c).AlignSelf(ui.Stretch).Height(a.resp(40, 34)).PaddingX(a.resp(16, 12))
	box.Children(func() {
		ui.Row(c).AlignSelf(ui.Stretch).AlignItems(ui.Center).Gap(12).Children(func() {
			// 歌词文字：有阴影投影保证深/浅背景下都可读（对齐 subtitle-bar text-white/95）
			// 居中（对齐 subtitle-bar justify-center：内部歌词文字水平居中）
			ui.Text(c, sub).FontSize(a.capFontSize()).Bold().
				TextColor(ui.RGB(0xff, 0xff, 0xff)).MaxLines(1).Grow(1).TextAlign(ui.Center)
			// 关闭按钮：仅悬停浮现（对齐 subtitle-x group-hover）
			if box.Hovered() {
				if iconButton(c, "✕").Clicked() {
					a.toggleCaption()
				}
			}
		})
	})
}

// toggleCaption 字幕条开关。
func (a *app) toggleCaption() {
	a.captionShown = !a.captionShown
	a.win.Invalidate()
}

// toggleMute 静音/恢复（MyGo 本地 audio 音量）。
func (a *app) toggleMute() {
	if a.volume > 0 {
		a.lastVolume = a.volume
		a.muted = true
		a.setVolume(0)
	} else {
		if a.lastVolume <= 0 {
			a.lastVolume = 0.8
		}
		a.muted = false
		a.setVolume(a.lastVolume)
	}
}

var _ = color.RGBA{}
