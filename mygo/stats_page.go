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

			// 常听歌手（固定高行 + 单行截断，长名字不撑爆格子）
			artists := a.topArtists(10)
			if len(artists) > 0 {
				ui.Text(c, "常听歌手").FontSize(15 * uiScale).Bold().TextColor(clrText).Padding(8*uiScale, 0, 0, 0)
				for i, ar := range artists {
					statRow(c, fmt.Sprintf("%d.", i+1), ar.Name, fmt.Sprintf("%d 次", ar.Count), false, nil)
				}
			}

			// 常听歌曲（点击直接播放；固定高行 + 单行截断）
			songs := a.topSongs(20)
			if len(songs) > 0 {
				ui.Text(c, "常听歌曲").FontSize(15 * uiScale).Bold().TextColor(clrText).Padding(8*uiScale, 0, 0, 0)
				for i, x := range songs {
					x := x
					statRow(c, fmt.Sprintf("%d.", i+1), x.Title+" — "+x.Artist, fmt.Sprintf("%d 次", x.Count), true, func() {
						a.PlayRef(x.Source, x.ID, x.Title, x.Artist, x.Cover)
					})
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

// statRow 统计页一行：序号（固定宽）+ 名称（Grow 单行截断）+ 次数（右对齐固定），
// 整行固定高度（不随文字换行膨胀），可选点击。
func statRow(c *ui.Context, index, name, count string, clickable bool, onClick func()) bool {
	row := ui.Box(c).AlignSelf(ui.Stretch).Height(38 * uiScale).Radius(8 * uiScale).
		Padding(0, 10*uiScale).Background(clrCard)
	row.Children(func() {
		ui.Row(c).AlignSelf(ui.Stretch).AlignItems(ui.Center).Gap(8 * uiScale).Children(func() {
			ui.Text(c, index).FontSize(11*uiScale).TextColor(clrMuted).Width(24*uiScale).MaxLines(1)
			ui.Text(c, name).FontSize(12*uiScale).TextColor(clrText).Grow(1).MaxLines(1)
			ui.Text(c, count).FontSize(11*uiScale).TextColor(clrMuted).MaxLines(1)
		})
	})
	if clickable && row.Clicked() {
		if onClick != nil {
			onClick()
		}
		return true
	}
	return false
}
