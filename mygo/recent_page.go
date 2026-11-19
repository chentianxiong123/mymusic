//go:build native

package main

import (
	"fmt"
	"homecast/bilibili"

	"github.com/egoist/mygo/ui"
)

func (a *app) recentPage(c *ui.Context) {
	// 整页滚动条（对齐主线）：整页内容包一个 Scroll，不做组件内部滚动条
	ui.Scroll(c).Grow(1).Scrollbars(false).Children(func() {
		ui.Column(c).Fill().PaddingX(a.resp(24, 12)).PaddingY(a.resp(16, 10)).Gap(14 * uiScale).Children(func() {
			ui.Row(c).AlignSelf(ui.Stretch).AlignItems(ui.Center).Gap(10 * uiScale).Children(func() {
				ui.Text(c, "最近播放").FontSize(20 * uiScale).Bold().TextColor(clrText)
				ui.Text(c, fmt.Sprintf("共 %d 首", len(a.recent))).FontSize(13 * uiScale).TextColor(clrMuted)
				ui.Spacer(c).Grow(1)
				if len(a.recent) > 0 {
					if chip(c, "清空").Clicked() {
						a.recent = nil
						a.saveRecent()
						a.win.Invalidate()
					}
				}
			})
			if len(a.recent) == 0 {
				ui.Column(c).Fill().Center().Gap(10 * uiScale).Children(func() {
					icon(c, icList, 44, ui.RGBA(0xff, 0xff, 0xff, 0.08))
					ui.Text(c, "还没有播放记录").FontSize(17 * uiScale).Bold().TextColor(clrText)
					ui.Text(c, "听过的歌会出现在这里，最多保留 30 首").FontSize(13 * uiScale).TextColor(clrMuted)
					if gradButton(c, "去搜索").Clicked() {
						a.setTab("search")
					}
				})
				return
			}
			// 网格卡片（列数按 tier 对应主线 grid-cols-3 sm:4 md:6）+ 分页
			cols := a.recentGridCols()
			tracks := make([]ui.Track, cols)
			for i := range tracks {
				tracks[i] = ui.Fr(1)
			}
			page := a.pg["recent"]
			ui.Grid(c).ColumnTracks(tracks...).Gap(12 * uiScale).Fill().JustifyItems(ui.Center).Children(func() {
				for _, item := range pageSlice(a.recent, page) {
					a.recentCard(c, item)
				}
			})
			pager(c, page, pageCount(len(a.recent)), func(p int) {
				a.pg["recent"] = p
				a.win.Invalidate()
			})
		})
	})
}

// recentGridCols 最近播放网格列数（对齐主线 grid-cols-3 sm:4 md:6）。
func (a *app) recentGridCols() int {
	switch a.tier() {
	case 0: // 手机（<768）
		return 3
	case 1: // 紧凑（768-1024）
		return 4
	default: // 桌面（>=1024）
		return 6
	}
}

// recentCard 最近播放网格卡片：方形封面为主 + 标题/歌手文字次之（对齐主线 recent.js 卡片）。
// gridCard 网格卡片共享样式（封面盒 + 12 圆角 Clip + hover 遮罩 + 标题/副标题）。
// recentCard / songlistCard 等网格卡片全部复用，避免重复造轮子。
// coverKey=封面缓存键（bvid 或歌单 key）；coverURL 空=不拉取；onTap=点击回调。
// gridCard 网格卡片（最近播放 / 歌单共用）：方形封面 + 标题 + 副标题。
// 整体包在 Column 里作为单一 grid item，避免封面与文字被排成不同格子。
func (a *app) recentCard(c *ui.Context, item bilibili.SearchItem) {
	// 封面（方形，随网格宽度自适应）— 复用 gridCard 共享样式
	size := a.recentCardSize()
	a.gridCard(c, item.BVID, item.Pic, bilibili.CleanHTML(item.Title), item.Author, size, func() {
		go a.play(item)
	})
}

// recentCardSize 网格卡片封面边长（桌面 6 列时 ~120，紧凑/手机随列数自适应）。
func (a *app) recentCardSize() float32 {
	// 窗口宽 / 列数，减去间距（12*2），留封面方边。用固定档位避免过度计算。
	switch a.tier() {
	case 0:
		return a.resp(96, 88)
	case 1:
		return a.resp(112, 96)
	default:
		return a.resp(132, 112)
	}
}

// currentToItem 把当前曲目转回 SearchItem（供 play 复用）。
