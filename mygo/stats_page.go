//go:build native

// stats_page.go —— 播放统计页（按歌/歌手展示，数据来自 play_stats.go）。
package main

import (
	"fmt"

	"github.com/egoist/mygo/ui"
)

func (a *app) statsPage(c *ui.Context) {
	ui.Scroll(c).Fill().Scrollbars(false).Children(func() {
		ui.Column(c).Gap(10 * uiScale).Padding(4*uiScale, a.resp(24, 12), 24*uiScale, a.resp(24, 12)).Children(func() {
			ui.Text(c, "播放统计").FontSize(20 * uiScale).Bold().TextColor(clrText)
			s := a.loadPlayStats()
			total := 0
			for _, x := range s.Songs {
				total += x.Count
			}
			ui.Text(c, fmt.Sprintf("累计播放 %d 次", total)).
				FontSize(12 * uiScale).TextColor(clrMuted)

			// 常听歌手
			artists := a.topArtists(10)
			if len(artists) > 0 {
				ui.Text(c, "常听歌手").FontSize(15 * uiScale).Bold().TextColor(clrText).Padding(8*uiScale, 0, 0, 0)
				for i, ar := range artists {
					chip(c, fmt.Sprintf("%d. %s — %d 次", i+1, ar.Name, ar.Count))
				}
			}

			// 常听歌曲（点击直接播放）
			songs := a.topSongs(20)
			if len(songs) > 0 {
				ui.Text(c, "常听歌曲").FontSize(15 * uiScale).Bold().TextColor(clrText).Padding(8*uiScale, 0, 0, 0)
				for i, x := range songs {
					x := x
					label := fmt.Sprintf("%d. %s — %s · %d 次", i+1, x.Title, x.Artist, x.Count)
					if chip(c, label).Clicked() {
						a.PlayRef(x.Source, x.ID, x.Title, x.Artist, x.Cover)
					}
				}
			}
			if len(s.Songs) == 0 {
				ui.Text(c, "还没有播放记录").FontSize(13 * uiScale).TextColor(clrMuted)
			}

			// 清空统计
			clear := ui.Box(c).AlignSelf(ui.Start).Height(34 * uiScale).PaddingX(14 * uiScale).
				Radius(8 * uiScale).Background(clrCard)
			if clear.Clicked() {
				a.savePlayStats(playStats{})
			}
			clear.Children(func() {
				ui.Text(c, "清空统计").FontSize(12 * uiScale).TextColor(clrMuted).AlignSelf(ui.Center)
			})
		})
	})
}

