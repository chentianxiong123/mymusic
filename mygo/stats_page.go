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
					statRow(c, a.resp(520, 340)*uiScale, fmt.Sprintf("%d.", i+1), ar.Name, fmt.Sprintf("%d 次", ar.Count), false, nil)
				}
			}

			// 常听歌曲（点击直接播放；固定高行 + 单行截断）
			songs := a.topSongs(20)
			if len(songs) > 0 {
				ui.Text(c, "常听歌曲").FontSize(15 * uiScale).Bold().TextColor(clrText).Padding(8*uiScale, 0, 0, 0)
				for i, x := range songs {
					x := x
					statRow(c, a.resp(520, 340)*uiScale, fmt.Sprintf("%d.", i+1), x.Title+" — "+x.Artist, fmt.Sprintf("%d 次", x.Count), true, func() {
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

// statRow 统计页一行：居中气泡（宽度随内容自适应、封顶不超屏），固定高 + 单行截断。
// 文字整体居中（序号 · 名称 · 次数拼一行），长文本 MaxWidth 封顶 + MaxLines(1) 不爆字体。
func statRow(c *ui.Context, maxW float32, index, name, count string, clickable bool, onClick func()) bool {
	label := name
	if index != "" {
		label = index + " " + name
	}
	if count != "" {
		label += " · " + count
	}
	clicked := false
	// 外层整行居中：内容少时气泡按内容宽居中，内容多时气泡封顶仍居中
	ui.Row(c).AlignSelf(ui.Stretch).Justify(ui.Center).Children(func() {
		row := ui.Box(c).Height(34 * uiScale).Radius(8 * uiScale).PaddingX(12 * uiScale).
			Background(clrCard).AlignItems(ui.Center).Justify(ui.Center)
		row.Draw(func(p *ui.Painter, r ui.Rect) {
			bg := clrCard
			if row.Hovered() {
				bg = clrCardHov // 可点行 hover 反馈（跟 chip 一致）
			}
			p.Fill(r, bg, 8)
		})
		row.Children(func() {
			ui.Text(c, label).FontSize(12*uiScale).TextColor(clrText).
				MaxLines(1).MaxWidth(maxW)
		})
		if clickable && row.Clicked() {
			clicked = true
		}
	})
	if clicked && onClick != nil {
		onClick()
	}
	return clicked
}

