//go:build native

package main

import (
	"fmt"
	"homecast/bilibili"

	"github.com/egoist/mygo/ui"
)

func (a *app) favPage(c *ui.Context) {
	// 歌单详情（点歌单卡进入）优先
	if a.songlistOpen != 0 {
		a.songlistDetail(c)
		return
	}
	// 整页滚动条（对齐主线）：整页内容包一个 Scroll，不做组件内部滚动条
	ui.Scroll(c).Grow(1).Scrollbars(false).Children(func() {
		ui.Column(c).Fill().PaddingX(a.resp(24, 12)).PaddingY(a.resp(16, 10)).Gap(14 * uiScale).Children(func() {
			// ① 喜欢（原收藏）区
			ui.Row(c).AlignSelf(ui.Stretch).AlignItems(ui.Center).Gap(10 * uiScale).Children(func() {
				ui.Text(c, "我喜欢的").FontSize(20 * uiScale).Bold().TextColor(clrText)
				ui.Text(c, fmt.Sprintf("%d 首", len(a.favorites))).FontSize(13 * uiScale).TextColor(clrMuted)
				ui.Spacer(c).Grow(1)
				if len(a.favorites) > 0 {
					if chip(c, "播放全部").Clicked() {
						a.playAll(a.favorites, 0)
					}
				}
			})
			if len(a.favorites) == 0 {
				ui.Column(c).Fill().Center().Gap(12 * uiScale).PaddingY(20 * uiScale).Children(func() {
					icon(c, icHeart, 44, ui.RGBA(0xff, 0xff, 0xff, 0.08))
					ui.Text(c, "喜欢的歌还空着").FontSize(17 * uiScale).Bold().TextColor(clrText)
					ui.Text(c, "听歌时点卡片上的 ♥ 收藏").FontSize(13 * uiScale).TextColor(clrMuted)
				})
			} else {
				ui.Column(c).Gap(6 * uiScale).Children(func() {
					favPage := a.pg["fav"]
					for _, item := range pageSlice(a.favorites, favPage) {
						a.favRow(c, item)
					}
					pager(c, favPage, pageCount(len(a.favorites)), func(p int) {
						a.pg["fav"] = p
						a.win.Invalidate()
					})
				})
			}
			// ② 歌单区（对齐主线 songlist_grid.html）
			ui.Row(c).AlignSelf(ui.Stretch).AlignItems(ui.Center).Gap(10 * uiScale).PaddingY(6 * uiScale).Children(func() {
				ui.Text(c, "我的歌单").FontSize(20 * uiScale).Bold().TextColor(clrText)
				ui.Text(c, fmt.Sprintf("%d 份", len(a.songlists))).FontSize(13 * uiScale).TextColor(clrMuted)
				ui.Spacer(c).Grow(1)
				if chip(c, "新建歌单").Clicked() {
					a.songlistNewOpen = true
					a.songlistNameInput = ""
					a.win.Invalidate()
				}
			})
			if a.songlistNewOpen {
				a.songlistNewRow(c)
			}
			if len(a.songlists) == 0 && !a.songlistNewOpen {
				ui.Column(c).Fill().Center().Gap(12 * uiScale).PaddingY(20 * uiScale).Children(func() {
					icon(c, icList, 44, ui.RGBA(0xff, 0xff, 0xff, 0.08))
					ui.Text(c, "还没有歌单").FontSize(17 * uiScale).Bold().TextColor(clrText)
					ui.Text(c, "把喜欢的歌归类收藏").FontSize(13 * uiScale).TextColor(clrMuted)
				})
			} else if len(a.songlists) > 0 {
				cols := a.songlistGridCols()
				tracks := make([]ui.Track, cols)
				for i := range tracks {
					tracks[i] = ui.Fr(1)
				}
				ui.Grid(c).ColumnTracks(tracks...).Gap(8 * uiScale).Fill().Children(func() {
					for _, g := range a.songlists {
						a.songlistCard(c, g)
					}
				})
			}
		})
	})
}

// favRow 收藏行。
// favRow 收藏行（复用 songRow）：♥ 取消收藏；单击播放 + 右键菜单。
func (a *app) favRow(c *ui.Context, item bilibili.SearchItem) {
	a.songRow(c, item.BVID, item.Pic, item.Title, item.Author, item.Duration, songRowOpt{
		isFav: true,
		heartFn: func() {
			a.svc.fav.Remove(item.BVID)
			a.refreshFavs()
			a.win.Invalidate()
		},
	})
}

// settingsPage 设置页：全量对齐主线 settings.js 渲染的 11 个区块
// （主题模式/网页字幕条/音质/播放模式/歌词字号/歌词背景/歌词时间/字幕字号/歌曲缓存/歌词缓存/数据/关于）。
