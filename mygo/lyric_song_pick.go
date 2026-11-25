//go:build native

// lyric_song_pick.go —— 歌级歌词绑定弹窗（右键菜单「歌词源」入口）。
// 颗粒度：这首歌用哪个歌词插件（歌级 > 渠道级 > 插件自带 > 默认）。
package main

import (
	"github.com/egoist/mygo/ui"
	"mymusic-mygo/plugin"
)

func (a *app) lyricSongPickDialog(c *ui.Context) {
	item := a.ctxItem
	if item == nil {
		a.lyricSongPick = false
		return
	}
	ui.Overlay(c, func() {
		lx, ly, pw, ph := a.dialogRect(360*uiScale, 420*uiScale)
		bg := ui.Box(c).Absolute().Left(0).Top(0).Right(0).Bottom(0)
		bg.Draw(func(p *ui.Painter, r ui.Rect) {})
		if bg.Clicked() {
			a.lyricSongPick = false
		}
		box := ui.Box(c).Absolute().Left(lx).Top(ly).Width(pw).Height(ph).
			Radius(16 * uiScale).Background(clrCard).Padding(16 * uiScale)
		box.Draw(func(p *ui.Painter, r ui.Rect) {
			p.Shadow(r, 16, 0, 10, 36, 0, ui.RGBA(0, 0, 0, 0.45))
			p.FillRadii(r, [4]float32{16, 16, 16, 16}, clrCard)
		})
		box.Children(func() {
			ui.Column(c).Fill().Gap(8 * uiScale).Children(func() {
				ui.Text(c, "这首歌的歌词源").FontSize(16 * uiScale).Bold().TextColor(clrText)
				ui.Text(c, item.Title).FontSize(12 * uiScale).TextColor(clrMuted)
				src := item.Typename
				if src == "" {
					src = "bilibili"
				}
				bound := a.lyricBoundForSong(src, item.BVID)
				ui.Scroll(c).Grow(1).Scrollbars(false).Children(func() {
					for _, name := range a.lyricBindOptions() {
						name := name
						isCur := name == bound
						btn := ui.Box(c).AlignSelf(ui.Stretch).Height(40 * uiScale).
							PaddingX(12 * uiScale).Radius(8 * uiScale)
						if isCur {
							btn.Background(clrCardHov)
						}
						if btn.Clicked() {
							a.setLyricBoundForSong(src, item.BVID, name)
							a.lyricSongPick = false
							a.ctxItem = nil
							// 当前正在播这首歌 → 立刻重取词
							if a.current != nil && a.current.bvid == item.BVID {
								a.loadLyricAsync(item.BVID)
							}
						}
						btn.Children(func() {
							ui.Row(c).AlignItems(ui.Center).Gap(8 * uiScale).Children(func() {
								ui.Text(c, name).FontSize(13 * uiScale).
									TextColor(map[bool]ui.Color{true: clrPink, false: clrText}[isCur])
								if plugin.Lyric(name) != nil {
									ui.Text(c, "已启用").FontSize(10 * uiScale).TextColor(clrMuted)
								}
								ui.Spacer(c).Grow(1)
								if isCur {
									ui.Text(c, "✓").FontSize(13 * uiScale).TextColor(clrPink)
								}
							})
						})
					}
				})
				// 恢复渠道级（删除歌级绑定）
				if _, ok := a.songLyricBinding(src, item.BVID); ok {
					reset := ui.Box(c).AlignSelf(ui.Stretch).Height(36 * uiScale).Radius(8 * uiScale)
					if reset.Clicked() {
						a.clearLyricBoundForSong(src, item.BVID)
						a.lyricSongPick = false
						a.ctxItem = nil
						if a.current != nil && a.current.bvid == item.BVID {
							a.loadLyricAsync(item.BVID)
						}
					}
					reset.Children(func() {
						ui.Text(c, "跟随音源设置").FontSize(12 * uiScale).TextColor(clrMuted).AlignSelf(ui.Center)
					})
				}
			})
		})
	})
}
