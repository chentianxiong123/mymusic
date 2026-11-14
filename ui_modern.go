//go:build native

// 现代化 UI 组件：深色 + 紫渐变，全部用 ui.Painter 自绘（渐变/圆角/阴影/圆角封面）。
package main

import (
	"image/color"

	"github.com/egoist/mygo/ui"

	"mymusic-mygo/bilibili"
)

// 配色（现代深色 + 紫粉渐变，参考用户 htmx 的强调色）
var (
	clrBg       = ui.RGB(0x0e, 0x0f, 0x14) // 窗口背景（近黑）
	clrCard     = ui.RGB(0x18, 0x19, 0x22) // 卡片
	clrCardHov  = ui.RGB(0x22, 0x23, 0x2e) // 卡片 hover
	clrField    = ui.RGB(0x1b, 0x1c, 0x27) // 输入框
	clrTrack    = ui.RGB(0x2b, 0x2c, 0x37) // 进度条轨道 / 占位
	clrText     = ui.RGB(0xf2, 0xf2, 0xf6)
	clrMuted    = ui.RGB(0x8a, 0x8b, 0x97)
	clrPurple   = ui.RGB(0x8b, 0x5c, 0xf6)
	clrPink     = ui.RGB(0xec, 0x48, 0x99)
	clrBar      = ui.RGB(0x14, 0x15, 0x1d) // 播放条底色
	clrTrackBar = ui.RGB(0x33, 0x34, 0x40) // 进度条轨道
)

func grad() ui.LinearGradient {
	return ui.LinearGradient{From: clrPurple, To: clrPink, Angle: 90}
}

// gradButton 紫渐变圆角按钮。
func gradButton(c *ui.Context, label string) ui.Element {
	box := ui.Box(c).Height(42).Radius(21).PaddingX(24).PaddingY(9).
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
	box := ui.Box(c).Size(size, size).Draw(func(p *ui.Painter, r ui.Rect) {
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
	})
	if url != "" {
		a.loadCover(bvid, url)
	}
	return box
}

// listRow 搜索结果列表行：小封面 + 标题/作者 + 收藏 + 播放。点击行播放。
func (a *app) listRow(c *ui.Context, item bilibili.SearchItem) {
	box := ui.Box(c).Fill().Height(60).Radius(12).PaddingX(12).Background(clrCard)
	box.Draw(func(p *ui.Painter, r ui.Rect) {
		if box.Hovered() {
			p.Fill(r, clrCardHov, 12)
		}
	})
	box.Children(func() {
		ui.Row(c).Gap(12).AlignItems(ui.Center).Children(func() {
			a.cover(c, item.BVID, item.Pic, 44)
			ui.Column(c).Grow(1).Gap(2).Children(func() {
				ui.Text(c, bilibili.CleanHTML(item.Title)).FontSize(15).Bold().TextColor(clrText).MaxLines(1)
				ui.Text(c, item.Author).FontSize(13).TextColor(clrMuted).MaxLines(1)
			})
			if iconButton2(c, icHeart, 34, accent(a.isFavItem(item))).Clicked() {
				a.toggleFavItem(item)
			}
			if gradButton(c, "播放").Clicked() {
				go a.play(item)
			}
		})
	})
	if box.Clicked() {
		go a.play(item)
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
	box := ui.Box(c).Size(34, 34).Radius(17)
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

// lyricCaption 字幕条（对齐主线）：当前歌词行 + 关闭按钮。
func (a *app) lyricCaption(c *ui.Context) {
	if !a.captionShown {
		return
	}
	box := ui.Box(c).AlignSelf(ui.Stretch).Height(40).Radius(12).PaddingX(16).Background(clrCard)
	box.Children(func() {
		ui.Row(c).AlignSelf(ui.Stretch).AlignItems(ui.Center).Gap(12).Children(func() {
			ui.Text(c, a.lyricText).FontSize(16).Bold().TextColor(clrText).MaxLines(1)
			ui.Spacer(c).Grow(1)
			if iconButton(c, "✕").Clicked() {
				a.toggleCaption()
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
