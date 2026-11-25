//go:build native

// 播放控制面板 + 队列/收藏/设置页 + 播放器控制方法（seek/volume/loop/next/prev/fav）。
package main

import (
	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"homecast/bilibili"
)

// setTab 切换导航页。
func (a *app) gridCard(c *ui.Context, coverKey, coverURL, title, subtitle string, size float32, onTap func()) {
	// 卡片填满格宽（正方形，AspectRatio(1)）——消空隙（用户：太疏），不再固定 size
	ui.Column(c).WidthPercent(100).Gap(6 * uiScale).Children(func() {
		box := ui.Box(c).WidthPercent(100).AspectRatio(1).Radius(12 * uiScale)
		box.Draw(func(p *ui.Painter, b ui.Rect) {
			// 无图时用 clrTrack 占位（比 clrCard 浅，避免看起来全黑）
			if img, ok := a.covers[coverKey]; ok {
				p.Clip(b, 12, func() { p.Image(img, b, ui.Cover) })
			} else {
				p.FillRadii(b, [4]float32{12, 12, 12, 12}, clrTrack)
			}
		})
		// 懒加载：只有滚进可视区才下载封面（搜索结果几十首不再一次性全下）
		if coverURL != "" && box.InView() {
			a.loadCover(coverKey, coverURL)
		}
		// hover 不改封面（不加遮罩、不加描边），只让下方标题变粉
		// 单击触发 onTap（最近播放=播放；歌单=打开歌单）。列表行（songRow）才是双击播放。
		if box.Clicked() && onTap != nil {
			onTap()
		}
		// 标题（主）+ 副标题（次）：hover 变粉（同最近播放效果）
		titleCol := clrText
		if box.Hovered() {
			titleCol = clrPink
		}
		ui.Text(c, title).FontSize(12 * uiScale).TextColor(titleCol).MaxLines(1)
		if subtitle != "" {
			ui.Text(c, subtitle).FontSize(10 * uiScale).TextColor(clrMuted).MaxLines(1)
		}
	})
}

func (a *app) currentToItem() *bilibili.SearchItem {
	if a.current == nil {
		return nil
	}
	return &bilibili.SearchItem{BVID: a.current.bvid, Title: a.current.title, Author: a.current.author, Pic: a.current.pic}
}

// queueRow 队列行：序号 + 封面 + 标题/作者 + 当前播放高亮。
// 交互对齐搜索行（listRow）：单击展开/收起完整标题，双击播放（不是单击就播）。
// queueRow 队列行（复用 songRow）：序号 + 当前播放高亮 + ✕ 移除；单击播放 + 右键菜单。
func (a *app) openCtxMenu(item *bilibili.SearchItem) {
	a.ctxItem = item
	a.ctxPosSet = false // 重置：ctxMenu 首帧记录一次鼠标位置，之后固定不跟鼠标
	a.win.Invalidate()
}

// ctxMenu 右键菜单浮层（对齐主线 contextmenu.js 四项：播放/下一首播放/收藏/复制链接）
func (a *app) ctxMenu(c *ui.Context) {
	item := *a.ctxItem
	// Overlay 层（mygo ui 专用置顶浮层，对齐 nowPlaying；否则被 Overlay 层盖住不置顶）
	ui.Overlay(c, func() {
		// 背景：点外部关闭 + 记录鼠标窗口位置（bg 从 0,0 铺满 → PointerPosition = 窗口坐标）
		bg := ui.Box(c).Absolute().Left(0).Top(0).Right(0).Bottom(0)
		bg.Draw(func(p *ui.Painter, r ui.Rect) {})
		mx, my, _ := bg.PointerPosition()
		// 首帧记录一次鼠标窗口位置（bg 从 0,0 铺满 → PointerPosition = 窗口坐标），
		// 之后固定不跟鼠标（原生右键菜单行为）。
		if !a.ctxPosSet {
			a.ctxX, a.ctxY = mx, my
			a.ctxPosSet = true
		}
		if bg.Clicked() {
			a.ctxItem = nil
			return
		}
		items := []struct {
			label string
			fn    func()
		}{
			{"播放", func() { go a.play(item); a.ctxItem = nil }},
			{"下一首播放", func() { a.playNext(item); a.ctxItem = nil }},
			{"收藏 / 取消收藏", func() { a.toggleFavItem(item); a.ctxItem = nil }},
			{"添加到歌单", func() { a.addToPick = true }}, // ctxItem 保留：浮层以它为目标准歌
			{"歌词源", func() { a.lyricSongPick = true }}, // ctxItem 保留：绑定弹窗以它为歌
			{"复制链接", func() { a.copyLink(item); a.ctxItem = nil }},
		}
		w := float32(160)
		h := float32(len(items))*38 + 8
		// 菜单固定在右键点击位置（a.ctxX/ctxY = 打开那一刻记录一次，不跟鼠标）
		lx, ly := a.ctxX, a.ctxY
		// 贴边防溢出
		if lx+w > float32(a.winW) {
			lx = float32(a.winW) - w
		}
		if ly+h > float32(a.winH) {
			ly = float32(a.winH) - h
		}
		menu := ui.Box(c).Absolute().Left(lx).Top(ly).Width(w).Height(h).Radius(12 * uiScale).Background(clrCard).PaddingY(4 * uiScale)
		menu.Draw(func(p *ui.Painter, r ui.Rect) {
			p.Shadow(r, 16, 0, 10, 36, 0, ui.RGBA(0, 0, 0, 0.45))
			p.FillRadii(r, [4]float32{12, 12, 12, 12}, clrCard)
		})
		menu.Children(func() {
			ui.Column(c).Gap(0 * uiScale).Children(func() {
				for _, it := range items {
					it := it
					row := ui.Box(c).Fill().Height(38 * uiScale).PaddingX(12 * uiScale)
					if row.Hovered() {
						row.Background(clrField)
					}
					row.Children(func() {
						ui.Row(c).AlignSelf(ui.Stretch).AlignItems(ui.Center).Children(func() {
							ui.Text(c, it.label).FontSize(13 * uiScale).TextColor(clrText)
						})
					})
					if row.Clicked() {
						it.fn()
						a.win.Invalidate()
					}
				}
			})
		})
	})
}

// playNext 下一首播放（对齐主线 play-next：插到当前歌后面 → 这里等价"立即播完这首接它"，
// MyGo 用 SetQueue 语义：取当前队列，把 item 插到 playIndex+1，重新 SetQueue）
func (a *app) playNext(item bilibili.SearchItem) {
	// 对齐主线：右键"下一首播放"= play-next → hcState.cmd('play') → Pb.Play。
	// 主线 Pb.Play 语义=去重后入队尾+定位播放（无"插到当前歌后"的插队概念）；
	// 旧版自创 SetQueue 手动插队（与主线不一致）。直接走 SSOT Play。
	go a.pb.Play(toSong(item))
}

// copyLink 复制 B站链接到剪贴板（mygo Clipboard.WriteText）。
func (a *app) copyLink(item bilibili.SearchItem) {
	url := "https://www.bilibili.com/video/" + item.BVID
	mygo.Clipboard.WriteText(url)
	a.status = "已复制：" + url
	a.win.Invalidate()
}
