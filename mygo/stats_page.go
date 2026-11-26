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

			// 清空统计（固定在列表底部、整行居中；必须二次确认）
			ui.Row(c).AlignSelf(ui.Stretch).Justify(ui.Center).PaddingY(10 * uiScale).Children(func() {
				clear := ui.Box(c).Height(34 * uiScale).PaddingX(16 * uiScale).
					Radius(8 * uiScale).Background(clrCard).Justify(ui.Center)
				if clear.Clicked() {
					a.statsConfirmClear = true
					a.win.Invalidate()
				}
				clear.Children(func() {
					ui.Text(c, "清空统计").FontSize(12 * uiScale).TextColor(clrMuted)
				})
			})
		})
	})
	if a.statsConfirmClear {
		a.statsClearConfirmDialog(c)
	}
}

// statsClearConfirmDialog 清空播放统计二次确认（响应式 dialogRect）。
func (a *app) statsClearConfirmDialog(c *ui.Context) {
	lx, ly, pw, ph := a.dialogRect(340*uiScale, 180*uiScale)
	ui.Overlay(c, func() {
		bg := ui.Box(c).Absolute().Left(0).Top(0).Right(0).Bottom(0)
		bg.Draw(func(p *ui.Painter, r ui.Rect) { p.Fill(r, ui.RGBA(0, 0, 0, 0.45), 0) })
		if bg.Clicked() {
			a.statsConfirmClear = false
			a.win.Invalidate()
		}
		box := ui.Box(c).Absolute().Left(lx).Top(ly).Width(pw).Height(ph).Radius(14*uiScale).Background(clrCard)
		box.Draw(func(p *ui.Painter, r ui.Rect) { p.Fill(r, clrCard, 14) })
		box.Children(func() {
			ui.Column(c).Fill().Padding(18*uiScale).Gap(12*uiScale).AlignItems(ui.Center).Children(func() {
				ui.Text(c, "清空播放统计？").FontSize(16*uiScale).Bold().TextColor(clrText)
				ui.Text(c, "所有播放次数将被清除，不可恢复").FontSize(12*uiScale).TextColor(clrMuted)
				ui.Spacer(c).Grow(1)
				ui.Row(c).Gap(12*uiScale).Children(func() {
					if chip(c, "取消").Clicked() {
						a.statsConfirmClear = false
						a.win.Invalidate()
					}
					if chip(c, "确认清空").Clicked() {
						a.savePlayStats(playStats{})
						a.statsConfirmClear = false
						a.win.Invalidate()
					}
				})
			})
		})
	})
}

// statRow 统计页一行（铺满整行，左对齐）：序号（固定宽）+ 名称（Grow 截断）+ 次数（右）。
// 关键：Box 上必须 Justify(ui.Center) 才能让 Row 里的文字垂直居中
//（只给 Row 设 AlignItems(Center) 无效，实测文字 Y=0 贴顶）。
func statRow(c *ui.Context, index, name, count string, clickable bool, onClick func()) bool {
	row := ui.Box(c).AlignSelf(ui.Stretch).Height(38 * uiScale).Radius(8 * uiScale).
		PaddingX(10 * uiScale).Background(clrCard).Justify(ui.Center)
	row.Children(func() {
		ui.Row(c).AlignSelf(ui.Stretch).AlignItems(ui.Center).Gap(8 * uiScale).Children(func() {
			if index != "" {
				ui.Text(c, index).FontSize(11*uiScale).TextColor(clrMuted).Width(24*uiScale).MaxLines(1)
			}
			ui.Text(c, name).FontSize(12*uiScale).TextColor(clrText).Grow(1).MaxLines(1)
			if count != "" {
				ui.Text(c, count).FontSize(11*uiScale).TextColor(clrMuted).MaxLines(1)
			}
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

