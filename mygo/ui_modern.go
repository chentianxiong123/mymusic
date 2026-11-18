//go:build native

// 现代化 UI 组件：深色 + 紫渐变，全部用 ui.Painter 自绘（渐变/圆角/阴影/圆角封面）。
package main

import (
	"fmt"
	"image/color"
	"unicode/utf8"

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
	clrGreen     = ui.RGB(0x16, 0xa3, 0x4a) // 在线/成功态（对齐主线 text-green-600）
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
		// 玻璃卡片（歌词候选/抽屉等）：明亮主题必须重设为浅色玻璃，
		// 否则保持深色玻璃（0x18,0x19,0x22）导致候选歌词/文字在明亮下看不清。
		clrCardGlass = ui.RGBA(0xff, 0xff, 0xff, 0.72)
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
func gradButton(c *ui.Context, label string) ui.Element {
	box := ui.Box(c).Height(42 * uiScale).Radius(8 * uiScale).PaddingX(24 * uiScale).PaddingY(9 * uiScale).AlignItems(ui.Center).Justify(ui.Center)
	box.Draw(func(p *ui.Painter, r ui.Rect) {
		p.FillGradient(r, grad(), 8)
		// 按下状态：压暗（反馈"已按"）；hover：提亮（反馈可点）
		if box.Pressed() {
			p.Fill(r, ui.RGBA(0, 0, 0, 0.18), 8)
		} else if box.Hovered() {
			p.Fill(r, ui.RGBA(255, 255, 255, 0.12), 8)
		}
	})
	box.Children(func() {
		ui.Text(c, label).TextColor(ui.RGB(0xff, 0xff, 0xff)).FontSize(15 * uiScale).Bold()
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
		p.Clip(r, 8*uiScale, func() { // 封面小圆角方形（原 size/2=圆形，用户要求改正方形）
			if img, ok := a.covers[bvid]; ok {
				p.Image(img, r, ui.Contain)
			} else {
				p.FillRadii(r, [4]float32{8 * uiScale, 8 * uiScale, 8 * uiScale, 8 * uiScale}, clrTrack)
				// 占位音符：两个圆点 + 两条小柄
				p.Fill(ui.Rect{X: r.X + size*0.38, Y: r.Y + size*0.42, W: size * 0.12, H: size * 0.12}, clrPurple, 0)
				p.Fill(ui.Rect{X: r.X + size*0.62, Y: r.Y + size*0.36, W: size * 0.12, H: size * 0.12}, clrPink, 0)
				p.Line(r.X+size*0.44, r.Y+size*0.54, r.X+size*0.44, r.Y+size*0.32, size*0.05, clrPurple)
				p.Line(r.X+size*0.68, r.Y+size*0.48, r.X+size*0.68, r.Y+size*0.26, size*0.05, clrPink)
			}
		})
		// hover「歌词」标签（对齐主线 group-hover 黑底圆角）
		if hoverLyric && box.Hovered() {
			p.Fill(r, ui.RGBA(0, 0, 0, 0.45), 8*uiScale) // 对齐方形封面小圆角
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
	// MinHeight：正常行高 h；内容超出（如标题展开多行）时自然长高，避免固定值突变跳动
	box := ui.Box(c).Fill().MinHeight(h).Radius(12 * uiScale).PaddingX(12 * uiScale)
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
// listRow 搜索结果行（复用 songRow）：♥ 收藏 + 播放按钮；单击行播放 + 右键菜单。
func (a *app) listRow(c *ui.Context, item bilibili.SearchItem) {
	a.songRow(c, item.BVID, item.Pic, item.Title, item.Author, item.Duration, songRowOpt{
		isFav:   a.isFavItem(item),
		heartFn: func() { a.toggleFavItem(item) },
		trailing: func() {
			// 播放按钮（圆形 —— 播放类统一圆形）
			if playIconButton(c, icPlay, a.resp(34, 30), accent(true)).Clicked() {
				go a.play(item)
			}
		},
	})
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

// progressBarSized 进度条；seek=true 时可拖跳转（原生 SliderBase）。
func (a *app) progressBarSized(c *ui.Context, frac float64, h float32, seek bool) ui.Element {
	// 只读进度（歌词页/挂件）：Box 显示，不可拖
	if !seek {
		box := ui.Box(c).AlignSelf(ui.Stretch).Height(h).Radius(3)
		box.Draw(func(p *ui.Painter, r ui.Rect) {
			p.FillRadii(r, [4]float32{3, 3, 3, 3}, clrTrackBar)
			if frac > 0 {
				fr := ui.Rect{X: r.X, Y: r.Y, W: r.W * float32(frac), H: r.H}
				p.FillGradient(fr, grad(), 3)
				p.Fill(ui.Rect{X: fr.X + fr.W - 5, Y: r.Y - 3, W: 10, H: 10}, ui.RGB(0xff, 0xff, 0xff), 5)
			}
		})
		return box
	}
	// 可拖 seek（playerBar）：改用 mygo 原生 SliderBase（flagDraggable，绑定 &a.progress
	// 双向）；之前自定义 Box+PointerPosition 拖不动。拖动中标记 scrubbing 暂停 pbLoop
	// 覆盖 a.progress（避免 thumb 跳回），Changed 时 seek。
	pad := h + 3
	s := ui.SliderBase(c, &a.progress, 0, 1).AlignSelf(ui.Stretch).Height(h + 2*pad).PaddingX(pad)
	a.scrubbing = s.Dragging()
	s.Draw(func(p *ui.Painter, r ui.Rect) {
		track := ui.Rect{X: r.X, Y: r.Y + (r.H-h)/2, W: r.W, H: h}
		p.FillRadii(track, [4]float32{3, 3, 3, 3}, clrTrackBar)
		if a.progress > 0 {
			fr := ui.Rect{X: track.X, Y: track.Y, W: track.W * float32(a.progress), H: h}
			p.FillGradient(fr, grad(), 3)
		}
		// 圆形 thumb 始终显示（对齐主线进度滑块）
		cx := track.X + track.W*float32(a.progress)
		cy := track.Y + h/2
		th := ui.Rect{X: cx - 5, Y: cy - 5, W: 10, H: 10}
		p.FillRadii(th, [4]float32{5, 5, 5, 5}, ui.RGB(0xff, 0xff, 0xff))
		p.FillRadii(ui.Rect{X: th.X + 1, Y: th.Y + 1, W: th.W - 2, H: th.H - 2}, [4]float32{4, 4, 4, 4}, clrPink)
	})
	if s.Changed() {
		a.seek(a.progress) // 拖动改 a.progress 后跳转
	}
	return s
}

// playCircle 大号圆形播放/暂停按钮。
func playCircle(c *ui.Context, playing bool, size float32) ui.Element {
	// 播放按钮 = 圆形（音乐播放器通用语汇，主操作视觉焦点）
	box := ui.Box(c).Size(size, size).Radius(size / 2)
	box.Draw(func(p *ui.Painter, r ui.Rect) {
		p.FillGradient(r, grad(), r.H/2)
		// 按下：压暗；hover：提亮（播放主按钮反馈）
		if box.Pressed() {
			p.Fill(r, ui.RGBA(0, 0, 0, 0.18), r.H/2)
		} else if box.Hovered() {
			p.Fill(r, ui.RGBA(255, 255, 255, 0.12), r.H/2)
		}
		if playing {
			p.FillRadii(ui.Rect{X: r.X + r.W*0.34, Y: r.Y + r.H*0.26, W: r.W * 0.10, H: r.H * 0.48}, [4]float32{2, 2, 2, 2}, ui.RGB(0xff, 0xff, 0xff))
			p.FillRadii(ui.Rect{X: r.X + r.W*0.56, Y: r.Y + r.H*0.26, W: r.W * 0.10, H: r.H * 0.48}, [4]float32{2, 2, 2, 2}, ui.RGB(0xff, 0xff, 0xff))
		} else {
			p.FillPath(playTriangle(r), ui.RGB(0xff, 0xff, 0xff))
		}
	})
	return box
}

// iconButton 方形图标按钮（歌词挂件开关等）。
func iconButton(c *ui.Context, glyph string) ui.Element {
	box := ui.Box(c).Size(34, 34).Radius(8).AlignItems(ui.Center).Justify(ui.Center)
	box.Draw(func(p *ui.Painter, r ui.Rect) {
		bg := clrCard
		if box.Hovered() {
			bg = clrCardHov
		}
		p.Fill(r, bg, 8)
	})
	box.Children(func() {
		ui.Text(c, glyph).TextColor(clrMuted).FontSize(14 * uiScale)
	})
	return box
}

// qualityLabel 当前音质标签（对齐主线 d-quality 三档）。
func (a *app) qualityLabel() string {
	return []string{"64k", "128k", "192k"}[a.quality]
}

// qualityChip 音质小按钮（对齐主线 d-quality：点击循环切换）。
func qualityChip(c *ui.Context, label string, on bool) ui.Element {
	box := ui.Box(c).Height(26 * uiScale).Radius(8 * uiScale).PaddingX(8 * uiScale).AlignItems(ui.Center).Justify(ui.Center)
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
		ui.Text(c, label).FontSize(12 * uiScale).TextColor(col)
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

// lyricCaption 字幕条已随"网页字幕条"功能一并移除（mygo 非 web，不需要 web 版字幕条）。

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

// statusBadge 状态徽标（圆点 + 文字，对齐主线 speaker/cast 在线态）。
// speaker 在线/离线、cast 设备状态统一复用，避免各写一套圆点+文字。
func statusBadge(c *ui.Context, text string, col ui.Color) {
	ui.Row(c).AlignItems(ui.Center).Gap(4 * uiScale).Children(func() {
		ui.Box(c).Size(6*uiScale, 6*uiScale).Radius(3 * uiScale).Background(col)
		ui.Text(c, text).FontSize(10 * uiScale).TextColor(col)
	})
}

// songRow 全站统一歌曲行（搜索/队列/收藏/歌单详情复用一套，杜绝每处自己写）。
// 交互：单击播放 + 右键菜单（不做单击展开，避免行跳动）。
// opt: idx!=nil 显示序号；isCur 高亮当前播放；isFav/favFn 显示 ♥；rmFn 显示 ✕；
// trailing 追加额外控件（如搜索行播放按钮）；author/dur 空则不显示。
// songRowOpt songRow 的可选配置（命名类型，调用方用字段名按需填）。
type songRowOpt struct {
	idx      *int   // 非 nil 显示序号（队列）
	isCur    bool   // 当前播放高亮（队列）
	isFav    bool   // ♥ 状态（配合 heartFn）
	heartFn  func() // ♥ 点击回调（搜索/收藏）
	rmFn     func() // ✕ 点击回调（队列/歌单详情）
	trailing func() // 追加额外控件（搜索行播放按钮）
}

func (a *app) songRow(c *ui.Context, bvid, pic, title, author, dur string, opt songRowOpt) {
	item := bilibili.SearchItem{BVID: bvid, Title: title, Author: author, Pic: pic, Duration: dur}
	box := ui.Box(c).Fill().MinHeight(a.resp(64, 56)).Radius(12 * uiScale).PaddingX(12 * uiScale).Gap(12 * uiScale).Justify(ui.Center)
	if opt.isCur {
		// 当前播放行：卡片 + 粉色描边（对齐主线 queue.js）
		box.Draw(func(p *ui.Painter, r ui.Rect) {
			p.FillRadii(r, [4]float32{12, 12, 12, 12}, clrCard)
			p.Stroke(r, clrPink, 12, 2*uiScale)
		})
	} else {
		box.Background(clrCard)
		if box.Hovered() {
			box.Background(clrCardHov)
		}
	}
	box.Children(func() {
		if box.RightClicked() {
			it := item
			a.openCtxMenu(&it)
		}
		// Row 高度=内容，box 的 Justify(Center) 让 Row 在 box 内垂直居中
		ui.Row(c).AlignItems(ui.Center).Gap(12 * uiScale).Children(func() {
			if opt.idx != nil {
				ui.Text(c, fmt.Sprintf("%02d", *opt.idx+1)).FontSize(13 * uiScale).TextColor(clrMuted).Width(26 * uiScale)
			}
			a.coverWide(c, bvid, pic, a.resp(80, 48), a.resp(56, 36))
			ui.Column(c).Grow(1).Gap(2 * uiScale).Children(func() {
				// 标题（单行）
				ui.Text(c, bilibili.CleanHTML(title)).FontSize(a.resp(14, 13)).Bold().TextColor(clrText).MaxLines(1)
				if author != "" {
					ui.Text(c, author).FontSize(12 * uiScale).TextColor(clrMuted).MaxLines(1)
				}
			})
			if opt.isCur {
				ui.Text(c, "▶ 播放中").FontSize(12 * uiScale).TextColor(clrPink).Bold()
			}
			if dur != "" {
				ui.Text(c, dur).FontSize(12 * uiScale).TextColor(clrMuted)
			}
			if opt.heartFn != nil {
				if iconButton2(c, icHeart, a.resp(34, 30), accent(opt.isFav)).Clicked() {
					opt.heartFn()
				}
			}
			if opt.rmFn != nil {
				if iconButton2(c, icClose, 32, clrMuted).Clicked() {
					opt.rmFn()
				}
			}
			if opt.trailing != nil {
				opt.trailing()
			}
		})
	})
	// 单击触发：标题较长时显示完整 tooltip（不播放）；双击播放（全站统一）
	cleanTitle := bilibili.CleanHTML(title)
	if box.DoubleClicked() {
		go a.play(item)
	} else if box.Clicked() && utf8.RuneCountInString(cleanTitle) > 12 {
		// 单击 + 标题较长 → 记录 tooltip，全局 Overlay 层渲染
		a.tipTitle = cleanTitle
		a.win.Invalidate()
	}
}

// searchBtn 搜索栏内的渐变按钮（小圆角 8，不做成胶囊；按钮贴搜索框右边缘）。
func searchBtn(c *ui.Context, label string) ui.Element {
	box := ui.Box(c).Height(36 * uiScale).Radius(18 * uiScale).PaddingX(20 * uiScale).AlignItems(ui.Center).Justify(ui.Center)
	box.Draw(func(p *ui.Painter, r ui.Rect) {
		p.FillGradient(r, grad(), r.H/2)
		if box.Pressed() {
			p.Fill(r, ui.RGBA(0, 0, 0, 0.18), r.H/2)
		} else if box.Hovered() {
			p.Fill(r, ui.RGBA(255, 255, 255, 0.12), r.H/2)
		}
	})
	box.Children(func() {
		ui.Text(c, label).FontSize(14 * uiScale).Bold().TextColor(ui.RGB(0xff, 0xff, 0xff))
	})
	return box
}

// playIconButton 列表行内的播放图标按钮（圆形 —— 播放类图标统一圆形，
// 与主控 playCircle 一致；其它操作按钮如 ♥ ✕ 用方形 iconButton2）。
func playIconButton(c *ui.Context, s *ui.SVG, size float32, col ui.Color) ui.Element {
	box := ui.Box(c).Size(size, size).Radius(size / 2)
	box.Draw(func(p *ui.Painter, r ui.Rect) {
		if box.Pressed() {
			p.Fill(r, clrTrackBar, r.H/2)
		} else if box.Hovered() {
			p.Fill(r, clrCardHov, r.H/2)
		}
		p.Icon(s, r, col)
	})
	return box
}
